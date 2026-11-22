package contract_parity

import (
	"context"
	"errors"
	"testing"
	"time"

	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func TestPhase11_ContractAndAdapterVersionParity(t *testing.T) {
	ForEachStorageAdapter(t, "ContractAndAdapterVersion", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		if h.Capabilities.ContractVersion != storage.StorageContractVersion {
			t.Fatalf("contract version = %q, want %q", h.Capabilities.ContractVersion, storage.StorageContractVersion)
		}
		if h.Capabilities.AdapterName == "" {
			t.Fatal("adapter name is required")
		}
		if h.Capabilities.AdapterVersion == "" {
			t.Fatal("adapter version is required")
		}
	})
}

func TestPhase11_ContextCancellationHasNoPostingEffect(t *testing.T) {
	ForEachStorageAdapter(t, "ContextCancellationNoEffect", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		commit := phase7PostingCommit(f, "p11-cancel-key", "p11-cancel-hash")
		if _, err := h.Ports.Posting.CommitPosting(ctx, commit); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled posting error = %v, want context.Canceled", err)
		}
		if _, err := h.Ports.Entries.GetEntry(context.Background(), f.Company.ID, commit.Entry.ID); err == nil {
			t.Fatal("canceled posting persisted an entry")
		}
		if balance, err := h.Ports.Balance.GetAccountBalance(context.Background(), f.Company.ID, f.CashAccount.ID, time.Date(2026, time.January, 16, 0, 0, 0, 0, time.UTC), f.FunctionalCurrency); err != nil {
			t.Fatal(err)
		} else if !balance.IsZero() {
			t.Fatalf("canceled posting changed cash balance: %s", balance.String())
		}
	})
}

func TestPhase11_AuditAtomicAppendSnapshotAndCheckpoint(t *testing.T) {
	ForEachStorageAdapter(t, "AuditAtomicAppendSnapshotAndCheckpoint", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		raw, ok := h.Ports.Private[storage.AuditConformanceExtensionKey]
		if !ok {
			t.Fatal("audit conformance repository is not exposed")
		}
		repo, ok := raw.(storage.AuditConformanceRepository)
		if !ok {
			t.Fatal("audit conformance extension has the wrong type")
		}

		ctx := context.Background()
		first, err := repo.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{
			CompanyID:     f.Company.ID,
			EntityType:    "journal_entry",
			EntityID:      "JE-P11-1",
			Action:        "POST",
			ActorID:       "actor-p11",
			Reason:        "conformance",
			CorrelationID: "corr-p11-1",
			Payload:       []byte(`{"sequence":1}`),
		})
		if err != nil {
			t.Fatalf("append first audit log: %v", err)
		}
		second, err := repo.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{
			CompanyID:     f.Company.ID,
			EntityType:    "journal_entry",
			EntityID:      "JE-P11-2",
			Action:        "POST",
			ActorID:       "actor-p11",
			Reason:        "conformance",
			CorrelationID: "corr-p11-2",
			Payload:       []byte(`{"sequence":2}`),
		})
		if err != nil {
			t.Fatalf("append second audit log: %v", err)
		}
		if first.SequenceNumber != 1 || second.SequenceNumber != 2 {
			t.Fatalf("audit sequences = (%d,%d), want (1,2)", first.SequenceNumber, second.SequenceNumber)
		}
		if second.PreviousLogHash != first.CurrentLogHash {
			t.Fatal("audit chain links second log to the wrong first log")
		}
		if err := repo.SetExternalAuditCheckpoint(ctx, f.Company.ID, publicgovernance.AuditCheckpoint{
			CompanyID:      f.Company.ID,
			SequenceNumber: second.SequenceNumber,
			Version:        1,
			LogHash:        second.CurrentLogHash,
		}); err != nil {
			t.Fatalf("set external checkpoint: %v", err)
		}
		snapshot, err := repo.GetAuditVerificationSnapshot(ctx, f.Company.ID)
		if err != nil {
			t.Fatalf("get audit snapshot: %v", err)
		}
		if snapshot.SnapshotSequence != 2 || len(snapshot.Logs) != 2 {
			t.Fatalf("snapshot = sequence %d, logs %d; want 2,2", snapshot.SnapshotSequence, len(snapshot.Logs))
		}
		if snapshot.Checkpoint.SequenceNumber != second.SequenceNumber ||
			snapshot.Checkpoint.LogHash != second.CurrentLogHash {
			t.Fatalf("snapshot checkpoint mismatch: %+v", snapshot.Checkpoint)
		}

		last, err := repo.GetLastAuditLogForCompany(ctx, f.Company.ID)
		if err != nil {
			t.Fatal(err)
		}
		if last == nil || last.SequenceNumber != second.SequenceNumber || last.CurrentLogHash != second.CurrentLogHash {
			t.Fatalf("last audit log = %+v, want sequence %d and matching hash", last, second.SequenceNumber)
		}
	})
}
