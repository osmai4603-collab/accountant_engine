package governance

import (
	"context"
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func TestVerifyAuditIntegrityHelpers(t *testing.T) {
	repo := newInMemoryAuditRepo()
	companyID := types.NewCompanyID()
	chain := NewAuditChain(repo)
	if _, err := chain.Append(context.Background(), AuditLog{
		CompanyID: companyID, EntityType: "journal_entry",
		EntityID: "entry-1", Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyAuditIntegrity(context.Background(), repo, companyID)
	if err != nil || !valid {
		t.Fatalf("VerifyAuditIntegrity helper = (%v, %v)", valid, err)
	}
	if valid, err := VerifyAuditIntegrity(context.Background(), nil, companyID); valid || !errors.Is(err, ErrAuditChainBroken) {
		t.Fatalf("nil repo helper = (%v, %v), want broken chain", valid, err)
	}
}
