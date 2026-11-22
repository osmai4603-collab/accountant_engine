package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

func TestMemoryAuditTailDeletionIsDetectedByCheckpoint(t *testing.T) {
	ctx := context.Background()
	driver := newDriver()
	company := types.NewCompanyID()
	chain := kernalgovernance.NewAuditChain(driver)

	first, err := chain.AppendAt(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1", Action: "POST",
	}, time.Date(2026, 11, 22, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	second, err := chain.AppendAt(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-2", Action: "POST",
	}, time.Date(2026, 11, 22, 1, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, kernalgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: second.SequenceNumber, Version: 1, LogHash: second.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}

	driver.mu.Lock()
	driver.governanceAuditLogs[company] = driver.governanceAuditLogs[company][:1]
	driver.mu.Unlock()

	valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company)
	if valid || !errors.Is(err, kernalgovernance.ErrAuditCheckpointMismatch) {
		t.Fatalf("tail deletion = (%v, %v), want checkpoint mismatch", valid, err)
	}
	_ = first
}
