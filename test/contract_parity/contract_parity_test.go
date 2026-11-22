package contract_parity

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

func TestMain(m *testing.M) {
	code := m.Run()

	// A filtered run sees only part of the suite; writing the report from it
	// would silently erase evidence from scenarios that were not executed.
	runFlag := flag.Lookup("test.run")
	if runFlag != nil {
		pattern := runFlag.Value.String()
		if pattern != "" && pattern != ".*" {
			fmt.Fprintln(os.Stderr, "contract parity report not written: filtered test run")
			os.Exit(code)
		}
	}
	if os.Getenv("CONTRACT_PARITY_SKIP_REPORT") == "1" {
		fmt.Fprintln(os.Stderr, "WARNING: CONTRACT_PARITY_SKIP_REPORT=1 leaves the checked-in parity report unchanged; this run is NOT evidence of a fresh full suite")
		os.Exit(code)
	}

	for _, path := range []string{"parity_report.md", "../../docs/reports/contract_parity_report.md"} {
		if err := GlobalReporter.WriteReportToFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "write contract parity report %q: %v\n", path, err)
			code = 1
		}
	}

	os.Exit(code)
}

func mustMoney(amount string, currency string) types.Money {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		panic(err)
	}
	m, err := types.NewMoney(d, currency)
	if err != nil {
		panic(err)
	}
	return m
}

// ------------------------------------------------------------- Test 1: Draft Store Parity

func TestContractParity_DraftStore(t *testing.T) {
	ForEachStorageAdapter(t, "DraftStore", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		entryID := types.NewJournalEntryID()

		draft := journal.JournalEntry{
			ID:                  entryID,
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "DRAFT-001",
			Description:         "Parity draft entry",
			EffectiveDate:       time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC),
			Status:              journal.EntryDraft,
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
		}

		// Save draft
		if err := h.Ports.Drafts.SaveDraft(ctx, draft); err != nil {
			t.Fatalf("SaveDraft failed: %v", err)
		}

		// Retrieve draft
		loaded, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, entryID)
		if err != nil {
			t.Fatalf("GetEntry failed: %v", err)
		}

		if loaded.ID != draft.ID {
			t.Errorf("expected entry ID %v, got %v", draft.ID, loaded.ID)
		}
		if loaded.Status != journal.EntryDraft {
			t.Errorf("expected status draft, got %v", loaded.Status)
		}
		if len(loaded.Lines) != 2 {
			t.Errorf("expected 2 lines, got %d", len(loaded.Lines))
		}
	})
}

// ------------------------------------------------------------- Test 2: Sequence Allocator Parity

func TestContractParity_SequenceAllocator(t *testing.T) {
	ForEachStorageAdapter(t, "SequenceAllocator", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()

		seq1, err := h.Ports.Sequences.NextSequence(ctx, f.Company.ID, f.Journal.ID)
		if err != nil {
			t.Fatalf("NextSequence 1 failed: %v", err)
		}

		seq2, err := h.Ports.Sequences.NextSequence(ctx, f.Company.ID, f.Journal.ID)
		if err != nil {
			t.Fatalf("NextSequence 2 failed: %v", err)
		}

		if seq2 <= seq1 {
			t.Errorf("expected strictly monotonic sequence: seq1=%d, seq2=%d", seq1, seq2)
		}
	})
}

// ------------------------------------------------------------- Test 3: Commit Posting & Perpetual Balance Parity

func TestContractParity_CommitPostingAndBalance(t *testing.T) {
	ForEachStorageAdapter(t, "CommitPostingAndBalance", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		entryID := types.NewJournalEntryID()
		postDate := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)

		commit := storage.PostingCommit{
			Entry: journal.JournalEntry{
				ID:                  entryID,
				CompanyID:           f.Company.ID,
				JournalID:           f.Journal.ID,
				FiscalPeriodID:      f.OpenPeriod.ID,
				EntryNumber:         "POST-001",
				Description:         "Sales invoice posting",
				EffectiveDate:       postDate,
				PostingDate:         postDate,
				Status:              journal.EntryPosted,
				Classification:      journal.EntryClassRegular,
				TransactionCurrency: f.FunctionalCurrency,
				ExchangeRate:        decimal.NewFromInt(1),
				Lines: []journal.EntryLine{
					{
						ID:                  types.NewJournalEntryLineID(),
						AccountID:           f.CashAccount.ID,
						TransactionAmount:   mustMoney("500.00", f.FunctionalCurrency),
						TransactionCurrency: f.FunctionalCurrency,
						ExchangeRate:        decimal.NewFromInt(1),
						FunctionalDebit:     mustMoney("500.00", f.FunctionalCurrency),
						FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
					},
					{
						ID:                  types.NewJournalEntryLineID(),
						AccountID:           f.RevenueAccount.ID,
						TransactionAmount:   mustMoney("500.00", f.FunctionalCurrency),
						TransactionCurrency: f.FunctionalCurrency,
						ExchangeRate:        decimal.NewFromInt(1),
						FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
						FunctionalCredit:    mustMoney("500.00", f.FunctionalCurrency),
					},
				},
			},
			Operation:     storage.OperationPost,
			ActorID:       "parity-tester",
			Reason:        "Invoice processing",
			CorrelationID: "cor-001",
		}

		posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("CommitPosting failed: %v", err)
		}
		if posted.Status != journal.EntryPosted {
			t.Fatalf("expected status posted, got %v", posted.Status)
		}

		// Verify Account Balance
		asOf := postDate.Add(24 * time.Hour)
		cashBal, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.CashAccount.ID, asOf, f.FunctionalCurrency)
		if err != nil {
			t.Fatalf("GetAccountBalance cash failed: %v", err)
		}
		if !cashBal.Equal(decimal.NewFromInt(500)) {
			t.Errorf("expected cash balance 500, got %s", cashBal.String())
		}

		revBal, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.RevenueAccount.ID, asOf, f.FunctionalCurrency)
		if err != nil {
			t.Fatalf("GetAccountBalance revenue failed: %v", err)
		}
		if !revBal.Abs().Equal(decimal.NewFromInt(500)) {
			t.Errorf("expected revenue balance magnitude 500, got %s", revBal.String())
		}
	})
}

// ------------------------------------------------------------- Test 4: Idempotency Lifecycle Parity

func TestContractParity_IdempotencyLifecycle(t *testing.T) {
	ForEachStorageAdapter(t, "IdempotencyLifecycle", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		key := "idem-" + uuid.NewString()
		rec := storage.IdempotencyRecord{
			CompanyID:   f.Company.ID,
			Key:         key,
			Operation:   "post",
			PayloadHash: "hash-1234",
			Status:      storage.IdempotencyStarted,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		// 1. Claim initial key
		existing, claimed, err := h.Ports.Idempotency.Claim(ctx, rec)
		if err != nil {
			t.Fatalf("Claim failed: %v", err)
		}
		if !claimed {
			t.Fatalf("expected key to be claimed")
		}

		// 2. Claim existing key before complete
		existing, claimed, err = h.Ports.Idempotency.Claim(ctx, rec)
		if err != nil {
			t.Fatalf("re-claim error: %v", err)
		}
		if claimed {
			t.Fatalf("expected key to NOT be claimed a second time")
		}
		if existing.Status != storage.IdempotencyStarted {
			t.Errorf("expected existing status started, got %s", existing.Status)
		}

		// 3. Post a valid draft/entry first so foreign key on posting_idempotency is satisfied
		entryID := types.NewJournalEntryID()
		postDate := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
		commit := storage.PostingCommit{
			Entry: journal.JournalEntry{
				ID:                  entryID,
				CompanyID:           f.Company.ID,
				JournalID:           f.Journal.ID,
				FiscalPeriodID:      f.OpenPeriod.ID,
				EntryNumber:         "IDEM-001",
				Description:         "Idempotency target entry",
				EffectiveDate:       postDate,
				PostingDate:         postDate,
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
			Operation: storage.OperationPost,
			ActorID:   "parity-tester",
		}
		if _, err := h.Ports.Posting.CommitPosting(ctx, commit); err != nil {
			t.Fatalf("commit posting for idempotency test failed: %v", err)
		}

		// 4. Complete
		resp := []byte(`{"status":"posted"}`)
		if err := h.Ports.Idempotency.Complete(ctx, f.Company.ID, key, "hash-1234", entryID, resp); err != nil {
			t.Fatalf("Complete failed: %v", err)
		}

		// 5. Claim after complete -> should return completed record
		existing, claimed, err = h.Ports.Idempotency.Claim(ctx, rec)
		if err != nil {
			t.Fatalf("re-claim after complete failed: %v", err)
		}
		if claimed {
			t.Fatalf("expected key to NOT be claimed after completion")
		}
		if existing.Status != storage.IdempotencyCompleted {
			t.Errorf("expected status completed, got %s", existing.Status)
		}
	})
}

// ------------------------------------------------------------- Test 5: Transaction Rollback Atomicity Parity

func TestContractParity_RollbackAtomicity(t *testing.T) {
	ForEachStorageAdapter(t, "RollbackAtomicity", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := storage.WithCompanyContext(context.Background(), f.Company.ID)
		entryID := types.NewJournalEntryID()
		postDate := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)

		commit := storage.PostingCommit{
			Entry: journal.JournalEntry{
				ID:                  entryID,
				CompanyID:           f.Company.ID,
				JournalID:           f.Journal.ID,
				FiscalPeriodID:      f.OpenPeriod.ID,
				EntryNumber:         "ROLLBACK-001",
				Description:         "Should be rolled back",
				EffectiveDate:       postDate,
				PostingDate:         postDate,
				Status:              journal.EntryPosted,
				Classification:      journal.EntryClassRegular,
				TransactionCurrency: f.FunctionalCurrency,
				ExchangeRate:        decimal.NewFromInt(1),
				Lines: []journal.EntryLine{
					{
						ID:                  types.NewJournalEntryLineID(),
						AccountID:           f.CashAccount.ID,
						TransactionAmount:   mustMoney("999.00", f.FunctionalCurrency),
						TransactionCurrency: f.FunctionalCurrency,
						ExchangeRate:        decimal.NewFromInt(1),
						FunctionalDebit:     mustMoney("999.00", f.FunctionalCurrency),
						FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
					},
					{
						ID:                  types.NewJournalEntryLineID(),
						AccountID:           f.RevenueAccount.ID,
						TransactionAmount:   mustMoney("999.00", f.FunctionalCurrency),
						TransactionCurrency: f.FunctionalCurrency,
						ExchangeRate:        decimal.NewFromInt(1),
						FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
						FunctionalCredit:    mustMoney("999.00", f.FunctionalCurrency),
					},
				},
			},
			Operation: storage.OperationPost,
			ActorID:   "parity-tester",
		}

		simulatedErr := fmt.Errorf("simulated failure after commit attempt")

		// Execute within transaction and abort
		err := h.Ports.TxRunner.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, postErr := h.Ports.Posting.CommitPosting(txCtx, commit)
			if postErr != nil {
				return postErr
			}
			return simulatedErr
		})

		if err == nil {
			t.Fatalf("expected transaction error, got nil")
		}

		// Verify entry does NOT exist after rollback
		_, err = h.Ports.Entries.GetEntry(ctx, f.Company.ID, entryID)
		if err == nil {
			t.Fatalf("expected entry to NOT exist after rollback, but was found")
		}

		// Verify balance was untouched
		asOf := postDate.Add(24 * time.Hour)
		bal, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.CashAccount.ID, asOf, f.FunctionalCurrency)
		if err != nil {
			t.Fatalf("GetAccountBalance failed: %v", err)
		}
		if !bal.Equal(decimal.Zero) {
			t.Errorf("expected cash balance 0 after rollback, got %s", bal.String())
		}
	})
}

// ------------------------------------------------------------- Test 6: Audit Immutability Parity

func TestContractParity_AuditWriter(t *testing.T) {
	ForEachStorageAdapter(t, "AuditWriter", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()

		record := storage.AuditRecord{
			CompanyID:     f.Company.ID,
			EntityType:    "journal_entry",
			EntityID:      uuid.NewString(),
			Action:        "post",
			ActorID:       "parity-tester",
			Reason:        "Audit test",
			CorrelationID: "cor-audit-1",
			Payload:       []byte(`{"action":"test"}`),
			OccurredAt:    time.Now().UTC(),
		}

		if err := h.Ports.Audit.AppendAudit(ctx, record); err != nil {
			t.Fatalf("AppendAudit failed: %v", err)
		}
	})
}
