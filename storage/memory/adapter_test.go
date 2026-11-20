package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/masterdatacontract"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

func TestMemoryMasterDataContract(t *testing.T) {
	adapter := memory.New()
	masterdatacontract.Run(t, adapter.MasterDataStore())
}

func TestMemoryCapabilitiesDeclaration(t *testing.T) {
	adapter := memory.New()
	caps := adapter.Capabilities()
	if caps.GuaranteesDurability {
		t.Fatal("memory adapter must declare GuaranteesDurability = false")
	}
	if !caps.SupportsRepeatableRead || !caps.SupportsSerializable {
		t.Fatal("memory adapter should support repeatable read and serializable snapshots")
	}
}

func TestMemoryUnitOfWorkRollbackAtomicity(t *testing.T) {
	adapter := memory.New()
	ports := adapter.Ports()
	ctx := context.Background()

	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	accountID := types.NewAccountID()

	// 1. Initial State: sequence is 0, draft doesn't exist
	seq, err := ports.Sequences.NextSequence(ctx, companyID, journalID)
	if err != nil {
		t.Fatalf("initial next sequence: %v", err)
	}
	if seq != 1 {
		t.Fatalf("expected sequence 1, got %d", seq)
	}

	entryID := types.NewJournalEntryID()
	money100, _ := types.NewMoney(decimal.NewFromInt(100), "USD")
	testEntry := journal.JournalEntry{
		ID:        entryID,
		CompanyID: companyID,
		JournalID: journalID,
		Lines: []journal.EntryLine{
			{
				AccountID:           accountID,
				TransactionCurrency: "USD",
				TransactionAmount:   money100,
				FunctionalDebit:     money100,
			},
		},
	}

	// 2. Unit of work that mutates state then fails (Rollback)
	errForced := errors.New("forced rollback error")
	err = ports.UOWRunner.Execute(ctx, func(uowCtx context.Context, uow storage.UnitOfWork) error {
		// Allocate sequence inside UoW
		s, err := uow.Sequences().NextSequence(uowCtx, companyID, journalID)
		if err != nil {
			return err
		}
		if s != 2 {
			t.Errorf("expected sequence 2 inside UoW, got %d", s)
		}

		// Commit posting inside UoW
		_, err = uow.Posting().CommitPosting(uowCtx, storage.PostingCommit{
			Entry:     testEntry,
			Operation: storage.OperationPost,
			ActorID:   "tester",
		})
		if err != nil {
			return err
		}

		// Verify balance changed inside UoW
		bal, err := uow.Balance().GetAccountBalance(uowCtx, companyID, accountID, time.Now(), "USD")
		if err != nil {
			return err
		}
		if !bal.Equal(decimal.NewFromInt(100)) {
			t.Errorf("expected balance 100 inside UoW, got %s", bal)
		}

		// Force rollback
		return errForced
	})

	if !errors.Is(err, errForced) {
		t.Fatalf("expected errForced, got %v", err)
	}

	// 3. Verify Rollback: entry does not exist, balance is zero, sequence reverted
	_, err = ports.Entries.GetEntry(ctx, companyID, entryID)
	if !errors.Is(err, memory.ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound after rollback, got %v", err)
	}

	bal, err := ports.Balance.GetAccountBalance(ctx, companyID, accountID, time.Now(), "USD")
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if !bal.IsZero() {
		t.Fatalf("expected 0 balance after rollback, got %s", bal)
	}

	nextSeq, err := ports.Sequences.NextSequence(ctx, companyID, journalID)
	if err != nil {
		t.Fatalf("next sequence: %v", err)
	}
	if nextSeq != 2 {
		t.Fatalf("expected sequence count to roll back to 1, next sequence should be 2, got %d", nextSeq)
	}
}

func TestMemoryIdempotencyLifecycle(t *testing.T) {
	adapter := memory.New()
	ports := adapter.Ports()
	ctx := context.Background()

	companyID := types.NewCompanyID()
	key := "test-idemp-" + uuid.NewString()

	rec := storage.IdempotencyRecord{
		CompanyID:   companyID,
		Key:         key,
		Operation:   "post_entry",
		PayloadHash: "hash123",
	}

	claimedRec, claimed, err := ports.Idempotency.Claim(ctx, rec)
	if err != nil || !claimed {
		t.Fatalf("claim failed: claimed=%v, err=%v", claimed, err)
	}
	if claimedRec.Status != storage.IdempotencyStarted {
		t.Fatalf("expected started status, got %s", claimedRec.Status)
	}

	// Duplicate claim returns existing
	dupRec, claimed2, err := ports.Idempotency.Claim(ctx, rec)
	if err != nil || claimed2 {
		t.Fatalf("duplicate claim should not succeed: claimed=%v, err=%v", claimed2, err)
	}
	if dupRec.Key != key {
		t.Fatalf("expected same key %s, got %s", key, dupRec.Key)
	}

	// Complete
	entryID := types.NewJournalEntryID()
	if err := ports.Idempotency.Complete(ctx, companyID, key, "hash123", entryID, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// Final check
	recAfter, claimed3, err := ports.Idempotency.Claim(ctx, rec)
	if err != nil || claimed3 {
		t.Fatalf("claim after complete: claimed=%v, err=%v", claimed3, err)
	}
	if recAfter.Status != storage.IdempotencyCompleted || *recAfter.EntryID != entryID {
		t.Fatalf("expected completed with entryID, got %+v", recAfter)
	}
}
