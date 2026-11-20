package governance

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/core/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

type inMemoryAuditRepo struct {
	mu   sync.Mutex
	logs map[apiTypes.CompanyID][]AuditLog
}

func newInMemoryAuditRepo() *inMemoryAuditRepo {
	return &inMemoryAuditRepo{
		logs: make(map[apiTypes.CompanyID][]AuditLog),
	}
}

func (r *inMemoryAuditRepo) AppendAuditLog(ctx context.Context, log AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.logs[log.CompanyID]
	log.ID = int64(len(list) + 1)
	r.logs[log.CompanyID] = append(list, log)
	return nil
}

func (r *inMemoryAuditRepo) GetLastAuditLogForCompany(ctx context.Context, companyID apiTypes.CompanyID) (*AuditLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.logs[companyID]
	if len(list) == 0 {
		return nil, nil
	}
	last := list[len(list)-1]
	return &last, nil
}

func (r *inMemoryAuditRepo) IterateAuditLogs(ctx context.Context, companyID apiTypes.CompanyID, fn func(log AuditLog) error) error {
	r.mu.Lock()
	list := append([]AuditLog(nil), r.logs[companyID]...)
	r.mu.Unlock()

	for _, l := range list {
		if err := fn(l); err != nil {
			return err
		}
	}
	return nil
}

func TestCryptographicHashChaining(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryAuditRepo()
	chain := NewAuditChain(repo)

	compA := apiTypes.NewCompanyID()
	compB := apiTypes.NewCompanyID()

	// Append 3 logs for Company A
	log1, err := chain.Append(ctx, AuditLog{
		CompanyID:  compA,
		EntityType: "journal_entry",
		EntityID:   "JE-1",
		Action:     "POST",
		ActorID:    "actor-1",
		Payload:    []byte(`{"amount":"100.00"}`),
	})
	if err != nil {
		t.Fatalf("Append log1 failed: %v", err)
	}
	if log1.PreviousLogHash != GenesisHash {
		t.Fatalf("expected genesis hash on log1, got %s", log1.PreviousLogHash)
	}

	log2, err := chain.Append(ctx, AuditLog{
		CompanyID:  compA,
		EntityType: "journal_entry",
		EntityID:   "JE-2",
		Action:     "POST",
		ActorID:    "actor-2",
		Payload:    []byte(`{"amount":"200.00"}`),
	})
	if err != nil {
		t.Fatalf("Append log2 failed: %v", err)
	}
	if log2.PreviousLogHash != log1.CurrentLogHash {
		t.Fatalf("log2 prev hash (%s) must link to log1 curr hash (%s)", log2.PreviousLogHash, log1.CurrentLogHash)
	}

	// Company B log should start with GenesisHash independently
	logB1, err := chain.Append(ctx, AuditLog{
		CompanyID:  compB,
		EntityType: "journal_entry",
		EntityID:   "JE-B1",
		Action:     "POST",
		ActorID:    "actor-B",
		Payload:    []byte(`{"amount":"50.00"}`),
	})
	if err != nil {
		t.Fatalf("Append logB1 failed: %v", err)
	}
	if logB1.PreviousLogHash != GenesisHash {
		t.Fatalf("expected genesis hash on compB log1, got %s", logB1.PreviousLogHash)
	}

	// Verify integrity of Company A
	validA, err := chain.VerifyAuditIntegrity(ctx, compA)
	if err != nil || !validA {
		t.Fatalf("VerifyAuditIntegrity for compA failed: %v, valid=%v", err, validA)
	}

	// Tamper Detection Test: modify payload of log1 in storage
	repo.mu.Lock()
	repo.logs[compA][0].Payload = []byte(`{"amount":"999.00"}`) // Tampered!
	repo.mu.Unlock()

	validAfterTamper, err := chain.VerifyAuditIntegrity(ctx, compA)
	if validAfterTamper {
		t.Fatalf("expected verification to FAIL after tampering!")
	}
	if !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("expected ErrAuditChainBroken, got %v", err)
	}
}

func TestMakerCheckerAndSoDApprovals(t *testing.T) {
	threshold, _ := types.NewMoneyFromString("1000.00", "USD")
	amountAbove, _ := types.NewMoneyFromString("1500.00", "USD")
	amountBelow, _ := types.NewMoneyFromString("500.00", "USD")

	// 1. Self-approval above threshold prohibited
	err := ValidatePostingApproval("user-1", "user-1", amountAbove, threshold)
	if !errors.Is(err, ErrSelfApprovalProhibited) {
		t.Fatalf("expected ErrSelfApprovalProhibited, got %v", err)
	}

	// 2. Self-approval below threshold allowed
	err = ValidatePostingApproval("user-1", "user-1", amountBelow, threshold)
	if err != nil {
		t.Fatalf("expected approval below threshold to succeed, got %v", err)
	}

	// 3. Different approver above threshold allowed
	err = ValidatePostingApproval("user-1", "manager-2", amountAbove, threshold)
	if err != nil {
		t.Fatalf("expected different approver to succeed, got %v", err)
	}

	// 4. Segregation of duties (reconciliation vs disbursement)
	err = ValidateBankDutySegregation("user-1", "user-1")
	if !errors.Is(err, ErrBankDutySegregationViolation) {
		t.Fatalf("expected ErrBankDutySegregationViolation, got %v", err)
	}
	err = ValidateBankDutySegregation("user-1", "user-2")
	if err != nil {
		t.Fatalf("expected segregated duties to succeed, got %v", err)
	}

	// 5. Annual closing requires Controller
	err = ValidateAnnualClose(true, []Permission{PermissionPost, PermissionClose})
	if !errors.Is(err, ErrFinancialControllerRequired) {
		t.Fatalf("expected ErrFinancialControllerRequired, got %v", err)
	}
	err = ValidateAnnualClose(true, []Permission{PermissionPost, PermissionController})
	if err != nil {
		t.Fatalf("expected controller annual close to pass, got %v", err)
	}
}
