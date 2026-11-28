package governance

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

type inMemoryAuditRepo struct {
	mu   sync.Mutex
	logs map[apiTypes.CompanyID][]AuditLog
}

type nonAtomicAuditRepo struct {
	AuditLogRepository
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

func (r *inMemoryAuditRepo) AppendAuditLogAtomic(ctx context.Context, log AuditLog) (AuditLog, error) {
	if err := ctx.Err(); err != nil {
		return AuditLog{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.logs[log.CompanyID]
	log.ID = int64(len(list) + 1)
	log.SequenceNumber = uint64(len(list) + 1)
	log.PreviousLogHash = GenesisHash
	if len(list) > 0 {
		last := list[len(list)-1]
		log.PreviousLogHash = last.CurrentLogHash
	}
	log.CurrentLogHash = CalculateHash(log)
	r.logs[log.CompanyID] = append(list, log)
	return log, nil
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

func TestAuditChainHashIncludesReason(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryAuditRepo()
	chain := NewAuditChain(repo)
	companyID := types.NewCompanyID()
	if _, err := chain.Append(ctx, AuditLog{
		CompanyID: companyID, EntityType: "journal_entry", EntityID: "entry-1",
		Action: "POST", ActorID: "actor-1", Reason: "approved reason",
	}); err != nil {
		t.Fatal(err)
	}

	repo.mu.Lock()
	repo.logs[companyID][0].Reason = "tampered reason"
	repo.mu.Unlock()

	if valid, err := chain.VerifyAuditIntegrity(ctx, companyID); valid || !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("verification after changing reason = (%v, %v), want audit chain failure", valid, err)
	}
}

func TestAuditChainRejectsNonAtomicRepository(t *testing.T) {
	repo := nonAtomicAuditRepo{AuditLogRepository: newInMemoryAuditRepo()}
	chain := NewAuditChain(repo)
	_, err := chain.Append(context.Background(), AuditLog{CompanyID: types.NewCompanyID()})
	if !errors.Is(err, ErrAtomicAuditRepositoryRequired) {
		t.Fatalf("append without atomic repository error = %v, want atomic-repository-required", err)
	}
}

func TestAuditChainAppendIsAtomicAcrossInstances(t *testing.T) {
	ctx := context.Background()
	repo := newInMemoryAuditRepo()
	chains := []*AuditChain{NewAuditChain(repo), NewAuditChain(repo)}
	companyID := types.NewCompanyID()
	const entriesPerChain = 20
	errs := make(chan error, len(chains)*entriesPerChain)
	var wait sync.WaitGroup

	for _, chain := range chains {
		wait.Add(1)
		go func(chain *AuditChain) {
			defer wait.Done()
			for i := 0; i < entriesPerChain; i++ {
				_, err := chain.Append(ctx, AuditLog{
					CompanyID: companyID, EntityType: "journal_entry",
					EntityID: types.NewJournalEntryID().String(), Action: "POST",
				})
				if err != nil {
					errs <- err
					return
				}

			}
		}(chain)
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	if valid, err := chains[0].VerifyAuditIntegrity(ctx, companyID); !valid || err != nil {
		t.Fatalf("concurrent append verification = (%v, %v), want valid chain", valid, err)
	}
}
