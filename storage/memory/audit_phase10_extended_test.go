package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func phase10Entry(company types.CompanyID) journal.JournalEntry {
	amount, _ := types.NewMoney(decimal.NewFromInt(100), "USD")
	return journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: company, JournalID: types.NewJournalID(),
		Lines: []journal.EntryLine{{
			AccountID: types.NewAccountID(), TransactionCurrency: "USD",
			TransactionAmount: amount, FunctionalDebit: amount,
		}},
	}
}

func TestMemoryPhase10AuditLogPayloadIsDefensivelyCopied(t *testing.T) {
	ctx := context.Background()
	driver := newDriver()
	company := types.NewCompanyID()

	payload := []byte("original")
	appended, err := driver.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1",
		Action: "POST", Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	appended.Payload[0] = 'Y'

	checkPayload := func(phase string) {
		t.Helper()
		snapshot, err := driver.GetAuditVerificationSnapshot(ctx, company)
		if err != nil {
			t.Fatalf("%s snapshot: %v", phase, err)
		}
		if len(snapshot.Logs) != 1 || string(snapshot.Logs[0].Payload) != "original" {
			t.Fatalf("%s stored logs = %+v, want one log with original payload", phase, snapshot.Logs)
		}
		if valid, err := publicgovernance.NewAuditChain(driver).VerifyAuditIntegrity(ctx, company); !valid || err != nil {
			t.Fatalf("%s chain verification = (%v, %v)", phase, valid, err)
		}
		snapshot.Logs[0].Payload[0] = 'Z'
	}
	checkPayload("after caller mutation")
	checkPayload("after snapshot mutation")
}

func TestMemoryPhase10ConcurrentAppendBuildsOneChain(t *testing.T) {
	ctx := context.Background()
	driver := newDriver()
	company := types.NewCompanyID()

	const appenders = 16
	sequenceCh := make(chan uint64, appenders)
	errCh := make(chan error, appenders)
	var wg sync.WaitGroup
	for i := range appenders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			log, err := driver.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{
				CompanyID: company, EntityType: "journal_entry",
				EntityID: fmt.Sprintf("JE-%02d", i), Action: "POST",
			})
			if err != nil {
				errCh <- err
				return
			}
			sequenceCh <- log.SequenceNumber
		}(i)
	}
	wg.Wait()
	close(errCh)
	close(sequenceCh)
	for err := range errCh {
		t.Fatalf("concurrent append: %v", err)
	}
	seen := map[uint64]bool{}
	for sequence := range sequenceCh {
		if seen[sequence] {
			t.Fatalf("duplicate sequence %d", sequence)
		}
		seen[sequence] = true
	}
	for i := 1; i <= appenders; i++ {
		if !seen[uint64(i)] {
			t.Fatalf("missing sequence %d", i)
		}
	}
	if valid, err := publicgovernance.NewAuditChain(driver).VerifyAuditIntegrity(ctx, company); !valid || err != nil {
		t.Fatalf("chain after concurrent appends = (%v, %v)", valid, err)
	}
	snapshot, err := driver.GetAuditVerificationSnapshot(ctx, company)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Logs) != appenders || snapshot.SnapshotSequence != appenders {
		t.Fatalf("snapshot = %d logs / sequence %d, want %d", len(snapshot.Logs), snapshot.SnapshotSequence, appenders)
	}
	if snapshot.Logs[0].SequenceNumber != 1 || snapshot.Logs[0].PreviousLogHash != publicgovernance.GenesisHash {
		t.Fatalf("first snapshot log = %+v", snapshot.Logs[0])
	}
}

func TestMemoryPhase10PostingWritesVerifiableAuditEvidence(t *testing.T) {
	ctx := context.Background()
	driver := newDriver()
	company := types.NewCompanyID()
	entry := phase10Entry(company)

	posted, err := driver.CommitPosting(ctx, storage.PostingCommit{
		Entry: entry, Operation: storage.OperationPost,
		ActorID: "actor-1", Reason: "approved", CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.GetEntry(ctx, company, posted.ID); err != nil {
		t.Fatalf("posted entry not persisted: %v", err)
	}

	chain := publicgovernance.NewAuditChain(driver)
	if valid, err := chain.VerifyAuditIntegrity(ctx, company); !valid || err != nil {
		t.Fatalf("posted audit chain = (%v, %v)", valid, err)
	}
	snapshot, err := driver.GetAuditVerificationSnapshot(ctx, company)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Logs) != 1 || snapshot.Logs[0].EntityID != posted.ID.String() ||
		snapshot.Logs[0].Reason != "approved" || snapshot.Logs[0].CorrelationID != "corr-1" {
		t.Fatalf("posting audit snapshot = %+v", snapshot.Logs)
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: 1, Version: 1, LogHash: snapshot.Logs[0].CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); !valid || err != nil {
		t.Fatalf("posting checkpoint verification = (%v, %v)", valid, err)
	}
}

func TestMemoryPhase10AuditFailureRollsBackPostingEffect(t *testing.T) {
	for _, point := range []storage.FaultPoint{storage.FaultBeforeAudit, storage.FaultBeforeCommit} {
		t.Run(string(point), func(t *testing.T) {
			ctx := context.Background()
			driver := newDriver()
			company := types.NewCompanyID()
			entry := phase10Entry(company)
			injected := errors.New("injected audit persistence failure")
			faultCtx := storage.WithFaultInjector(ctx, func(candidate storage.FaultPoint) error {
				if candidate == point {
					return injected
				}
				return nil
			})

			_, err := driver.CommitPosting(faultCtx, storage.PostingCommit{
				Entry: entry, Operation: storage.OperationPost, ActorID: "actor-1", Reason: "approved",
			})
			if !errors.Is(err, injected) {
				t.Fatalf("CommitPosting error = %v, want injected failure", err)
			}
			if _, err := driver.GetEntry(ctx, company, entry.ID); !errors.Is(err, ErrEntryNotFound) {
				t.Fatalf("entry survived failed audit effect: %v", err)
			}
			if len(driver.auditRecords) != 0 || len(driver.governanceAuditLogs[company]) != 0 {
				t.Fatalf("audit effects survived rollback: legacy=%d v2=%d",
					len(driver.auditRecords), len(driver.governanceAuditLogs[company]))
			}
			valid, err := publicgovernance.NewAuditChain(driver).VerifyAuditIntegrityWithCheckpoint(ctx, company)
			if valid || !errors.Is(err, publicgovernance.ErrAuditCheckpointMismatch) {
				t.Fatalf("verification after rollback = (%v, %v), want fail-closed checkpoint mismatch", valid, err)
			}
		})
	}
}
