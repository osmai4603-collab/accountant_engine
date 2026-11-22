package contract_parity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func phase7Scope(companyID types.CompanyID, operation, key string) string {
	return fmt.Sprintf("company=%s|operation=%s|key=%s", companyID.String(), operation, key)
}

func phase7PostingCommit(f UnifiedFixture, key, payloadHash string) storage.PostingCommit {
	return storage.PostingCommit{
		Entry: journal.JournalEntry{
			ID:                  types.NewJournalEntryID(),
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "P7-" + key,
			SequenceNumber:      1,
			Description:         "phase 7 atomic posting",
			EffectiveDate:       f.OpenPeriod.StartDate,
			PostingDate:         f.OpenPeriod.StartDate,
			Status:              journal.EntryPosted,
			Classification:      journal.EntryClassRegular,
			TransactionCurrency: f.FunctionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			Lines: []journal.EntryLine{
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.CashAccount.ID,
					TransactionAmount:   mustMoney("100.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("100.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
				},
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.RevenueAccount.ID,
					TransactionAmount:   mustMoney("100.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("100.00", f.FunctionalCurrency),
				},
			},
		},
		Operation:     storage.OperationPost,
		ActorID:       "phase7-tester",
		Reason:        "contract test",
		CorrelationID: "cor-phase7-" + key,
		Idempotency: &storage.PostingIdempotency{
			Key:         key,
			Scope:       phase7Scope(f.Company.ID, "post", key),
			PayloadHash: payloadHash,
		},
	}
}

func phase7ReversalCommit(f UnifiedFixture, original journal.JournalEntry, key, payloadHash string) storage.PostingCommit {
	return storage.PostingCommit{
		Entry: journal.JournalEntry{
			ID:                  types.NewJournalEntryID(),
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "R7-" + key,
			SequenceNumber:      2,
			Description:         "phase 7 atomic reversal",
			EffectiveDate:       f.OpenPeriod.StartDate,
			PostingDate:         f.OpenPeriod.StartDate,
			Status:              journal.EntryPosted,
			Classification:      journal.EntryClassReversing,
			ReversalOfID:        &original.ID,
			TransactionCurrency: f.FunctionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			Lines: []journal.EntryLine{
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.RevenueAccount.ID,
					TransactionAmount:   mustMoney("100.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("100.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
				},
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.CashAccount.ID,
					TransactionAmount:   mustMoney("100.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("100.00", f.FunctionalCurrency),
				},
			},
		},
		Operation:       storage.OperationReverse,
		OriginalEntryID: &original.ID,
		ActorID:         "phase7-tester",
		Reason:          "contract test reversal",
		CorrelationID:   "cor-phase7-" + key,
		Idempotency: &storage.PostingIdempotency{
			Key:         key,
			Scope:       phase7Scope(f.Company.ID, "reverse", key),
			PayloadHash: payloadHash,
		},
	}
}

func phase7RequireLookup(t *testing.T, h *StorageAdapterHarness) storage.IdempotencyLookup {
	t.Helper()
	lookup, ok := h.Ports.Idempotency.(storage.IdempotencyLookup)
	if !ok {
		t.Fatalf("%s adapter does not implement IdempotencyLookup", h.Name)
	}
	return lookup
}

func phase7RequireBalance(t *testing.T, ctx context.Context, h *StorageAdapterHarness, f UnifiedFixture, want string) {
	t.Helper()
	got, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.CashAccount.ID, f.OpenPeriod.StartDate, f.FunctionalCurrency)
	if err != nil {
		t.Fatalf("GetAccountBalance: %v", err)
	}
	if !got.Equal(decimal.RequireFromString(want)) {
		t.Fatalf("cash balance = %s, want %s", got.String(), want)
	}
}

func TestPhase7_IdempotentReplayAndPayloadConflict(t *testing.T) {
	ForEachStorageAdapter(t, "IdempotentReplayAndPayloadConflict", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		lookup := phase7RequireLookup(t, h)
		commit := phase7PostingCommit(f, "replay-key", "hash-a")

		first, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("first commit: %v", err)
		}
		replay, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("replay commit: %v", err)
		}
		if first.ID != replay.ID {
			t.Fatalf("replay created a second entry: %s vs %s", first.ID, replay.ID)
		}
		phase7RequireBalance(t, ctx, h, f, "100")

		conflict := phase7PostingCommit(f, "replay-key", "hash-b")
		if _, err := h.Ports.Posting.CommitPosting(ctx, conflict); !errors.Is(err, storage.ErrIdempotencyPayloadConflict) {
			t.Fatalf("payload conflict error = %v, want ErrIdempotencyPayloadConflict", err)
		}
		phase7RequireBalance(t, ctx, h, f, "100")

		record, found, err := lookup.Lookup(ctx, f.Company.ID, "replay-key")
		if err != nil || !found {
			t.Fatalf("lookup replay key: found=%v err=%v", found, err)
		}
		if record.Status != storage.IdempotencyCompleted || record.EntryID == nil || *record.EntryID != first.ID {
			t.Fatalf("unexpected idempotency record: %+v", record)
		}
	})
}

func TestPhase7_KernelStampedDatesArePreserved(t *testing.T) {
	ForEachStorageAdapter(t, "KernelStampedDates", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		postedAt := time.Date(2026, 11, 22, 10, 11, 12, 0, time.UTC)
		commit := phase7PostingCommit(f, "stamped-dates-key", "hash-stamped-dates")
		commit.Entry.PostedAt = &postedAt

		posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("commit: %v", err)
		}
		stored, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, posted.ID)
		if err != nil {
			t.Fatalf("read entry: %v", err)
		}
		for name, entry := range map[string]journal.JournalEntry{"commit result": posted, "stored entry": stored} {
			if !entry.PostingDate.Equal(commit.Entry.PostingDate) {
				t.Fatalf("%s posting date = %s, want kernel stamp %s", name, entry.PostingDate, commit.Entry.PostingDate)
			}
			if entry.PostedAt == nil || !entry.PostedAt.Equal(postedAt) {
				t.Fatalf("%s posted at = %v, want kernel stamp %s", name, entry.PostedAt, postedAt)
			}
		}
	})
}

func TestPhase7_FaultInjectionAtomicity(t *testing.T) {
	ForEachStorageAdapter(t, "FaultInjectionAtomicity", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		lookup := phase7RequireLookup(t, h)

		for _, point := range []storage.FaultPoint{
			storage.FaultBeforeIdempotencyClaim,
			storage.FaultBeforeAudit,
			storage.FaultBeforeCommit,
		} {
			key := "fault-" + string(point)
			commit := phase7PostingCommit(f, key, "hash-"+key)
			faultCtx := storage.WithFaultInjector(ctx, func(got storage.FaultPoint) error {
				if got == point {
					return errors.New("injected " + string(point))
				}
				return nil
			})
			if _, err := h.Ports.Posting.CommitPosting(faultCtx, commit); err == nil {
				t.Fatalf("%s: expected injected failure", point)
			}
			if _, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, commit.Entry.ID); err == nil {
				t.Fatalf("%s: entry persisted despite rollback", point)
			}
			if record, found, err := lookup.Lookup(ctx, f.Company.ID, key); err != nil || found {
				t.Fatalf("%s: idempotency key survived rollback: found=%v record=%+v err=%v", point, found, record, err)
			}
			phase7RequireBalance(t, ctx, h, f, "0")
		}

		after := phase7PostingCommit(f, "fault-after-commit", "hash-after")
		lostCtx := storage.WithFaultInjector(ctx, func(point storage.FaultPoint) error {
			if point == storage.FaultAfterCommit {
				return errors.New("lost response after commit")
			}
			return nil
		})
		if _, err := h.Ports.Posting.CommitPosting(lostCtx, after); err == nil {
			t.Fatal("expected unknown-outcome error after commit")
		}
		record, found, err := lookup.Lookup(ctx, f.Company.ID, "fault-after-commit")
		if err != nil || !found {
			t.Fatalf("lookup after unknown outcome: found=%v err=%v", found, err)
		}
		if record.Status != storage.IdempotencyCompleted || record.EntryID == nil {
			t.Fatalf("unknown outcome did not commit durably: %+v", record)
		}
		replay, err := h.Ports.Posting.CommitPosting(ctx, after)
		if err != nil {
			t.Fatalf("replay after unknown outcome: %v", err)
		}
		if replay.ID != *record.EntryID {
			t.Fatalf("replay entry %s != stored entry %s", replay.ID, *record.EntryID)
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

func TestPhase7_ReversalAtomicity(t *testing.T) {
	ForEachStorageAdapter(t, "ReversalAtomicity", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		original, err := h.Ports.Posting.CommitPosting(ctx, phase7PostingCommit(f, "original-key", "hash-original"))
		if err != nil {
			t.Fatalf("post original: %v", err)
		}

		rollbackReversal := phase7ReversalCommit(f, original, "reversal-fault", "hash-reversal-fault")
		faultCtx := storage.WithFaultInjector(ctx, func(point storage.FaultPoint) error {
			if point == storage.FaultBeforeCommit {
				return errors.New("injected failure before reversal commit")
			}
			return nil
		})
		if _, err := h.Ports.Posting.CommitPosting(faultCtx, rollbackReversal); err == nil {
			t.Fatal("expected reversal fault to fail")
		}
		unchanged, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, original.ID)
		if err != nil {
			t.Fatalf("read original after rollback: %v", err)
		}
		if unchanged.Status != journal.EntryPosted || unchanged.ReversedByID != nil {
			t.Fatalf("original was partially reversed after rollback: %+v", unchanged)
		}
		if _, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, rollbackReversal.Entry.ID); err == nil {
			t.Fatal("reversal entry persisted despite rollback")
		}

		reversal, err := h.Ports.Posting.CommitPosting(ctx, phase7ReversalCommit(f, original, "reversal-key", "hash-reversal"))
		if err != nil {
			t.Fatalf("commit reversal: %v", err)
		}
		if reversal.ReversalOfID == nil || *reversal.ReversalOfID != original.ID {
			t.Fatalf("reversal link missing: %+v", reversal)
		}
		reverted, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, original.ID)
		if err != nil {
			t.Fatalf("read original after reversal: %v", err)
		}
		if reverted.Status != journal.EntryReversed || reverted.ReversedByID == nil || *reverted.ReversedByID != reversal.ID {
			t.Fatalf("original was not atomically linked to reversal: %+v", reverted)
		}

		second := phase7ReversalCommit(f, original, "reversal-second", "hash-reversal-second")
		if _, err := h.Ports.Posting.CommitPosting(ctx, second); !errors.Is(err, storage.ErrEntryAlreadyReversed) {
			t.Fatalf("second reversal error = %v, want ErrEntryAlreadyReversed", err)
		}
		if _, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, second.Entry.ID); err == nil {
			t.Fatal("second reversal entry persisted")
		}
	})
}

func TestPhase7_ConcurrentReplayHasSingleEffect(t *testing.T) {
	ForEachStorageAdapter(t, "ConcurrentReplay", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		commit := phase7PostingCommit(f, "concurrent-key", "hash-concurrent")

		const workers = 6
		var wg sync.WaitGroup
		results := make(chan journal.JournalEntry, workers)
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
				if err != nil {
					errs <- err
					return
				}
				results <- posted
			}()
		}
		wg.Wait()
		close(results)
		close(errs)

		for err := range errs {
			t.Fatalf("concurrent commit failed: %v", err)
		}
		var first *journal.JournalEntry
		for posted := range results {
			if first == nil {
				copied := posted
				first = &copied
				continue
			}
			if first.ID != posted.ID {
				t.Fatalf("concurrent replay returned different entries: %s vs %s", first.ID, posted.ID)
			}
		}
		if first == nil {
			t.Fatal("no concurrent commit succeeded")
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

func TestPhase7_CompanyScopeIsolation(t *testing.T) {
	ForEachStorageAdapter(t, "CompanyScopeIsolation", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		lookup := phase7RequireLookup(t, h)
		commit := phase7PostingCommit(f, "scope-key", "hash-scope")
		if _, err := h.Ports.Posting.CommitPosting(ctx, commit); err != nil {
			t.Fatalf("commit: %v", err)
		}

		otherCompany := types.NewCompanyID()
		if record, found, err := lookup.Lookup(ctx, otherCompany, "scope-key"); err != nil || found {
			t.Fatalf("cross-company lookup leaked record: found=%v record=%+v err=%v", found, record, err)
		}
	})
}
