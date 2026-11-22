package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
)

func TestMemoryAuditPhase10AtomicAppendAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	adapter := memory.New()
	driver := adapter.Driver()
	company := types.NewCompanyID()
	chain := kernalgovernance.NewAuditChain(driver)

	first, err := chain.AppendAt(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1",
		Action: "POST", ActorID: "actor-1", Reason: "approved",
		CorrelationID: "corr-1", Payload: []byte(`{"amount":"100"}`),
	}, time.Date(2026, 11, 22, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if first.SequenceNumber != 1 || first.PreviousLogHash != kernalgovernance.GenesisHash {
		t.Fatalf("first audit log = %+v", first)
	}

	if err := driver.SetExternalAuditCheckpoint(ctx, company, kernalgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: first.SequenceNumber,
		Version: 1, LogHash: first.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}
	valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company)
	if !valid || err != nil {
		t.Fatalf("checkpoint verification = %v, %v", valid, err)
	}

	second, err := chain.Append(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-2",
		Action: "POST", Payload: []byte(`{"amount":"200"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.SequenceNumber != 2 || second.PreviousLogHash != first.CurrentLogHash {
		t.Fatalf("second audit log = %+v", second)
	}
}

func TestMemoryAuditPhase10WithoutCheckpointHasNoTailAnchor(t *testing.T) {
	ctx := context.Background()
	adapter := memory.New()
	driver := adapter.Driver()
	company := types.NewCompanyID()
	chain := kernalgovernance.NewAuditChain(driver)

	if _, err := chain.Append(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1", Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := chain.Append(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-2", Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company)
	if valid || !errors.Is(err, kernalgovernance.ErrAuditCheckpointMismatch) {
		t.Fatalf("verification without trusted checkpoint = (%v, %v), want mismatch", valid, err)
	}
}

func TestMemoryAuditPhase10TamperingAndRollback(t *testing.T) {
	ctx := context.Background()
	adapter := memory.New()
	driver := adapter.Driver()
	company := types.NewCompanyID()
	chain := kernalgovernance.NewAuditChain(driver)

	log, err := chain.Append(ctx, kernalgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1",
		Action: "POST", Payload: []byte("original"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, kernalgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: 1, Version: 1, LogHash: log.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}

	err = driver.WithinTransaction(ctx, func(txCtx context.Context) error {
		return errors.New("forced rollback")
	})
	if err == nil {
		t.Fatal("forced rollback unexpectedly succeeded")
	}

	valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company)
	if !valid || err != nil {
		t.Fatalf("audit survived unrelated rollback = %v, %v", valid, err)
	}
}
