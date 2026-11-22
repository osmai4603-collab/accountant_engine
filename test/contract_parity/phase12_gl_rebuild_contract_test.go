package contract_parity

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

// TestContractParity_GLRebuildDeterminismAndParity verifies that general ledger balances
// rebuilt from canonical posted journal entries match the storage adapter balances exactly,
// that draft entries are excluded, and that rebuild is deterministic regardless of entry ordering.
func TestContractParity_GLRebuildDeterminismAndParity(t *testing.T) {
	ForEachStorageAdapter(t, "GLRebuildDeterminismAndParity", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		postDate := time.Date(2026, time.January, 20, 10, 0, 0, 0, time.UTC)
		asOf := postDate.Add(24 * time.Hour)

		// 1. Post Entry 1: Sale of 1000 (Cash DR 1000, Revenue CR 1000)
		entry1 := journal.JournalEntry{
			ID:                  types.NewJournalEntryID(),
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "P12-GL-001",
			SequenceNumber:      1,
			Description:         "Customer Invoice",
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
					TransactionAmount:   mustMoney("1000.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("1000.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
				},
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.RevenueAccount.ID,
					TransactionAmount:   mustMoney("1000.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("1000.00", f.FunctionalCurrency),
				},
			},
		}

		posted1, err := h.Ports.Posting.CommitPosting(ctx, storage.PostingCommit{
			Entry:         entry1,
			Operation:     storage.OperationPost,
			ActorID:       "gl-rebuild-tester",
			Reason:        "Invoice 1",
			CorrelationID: "gl-cor-001",
		})
		if err != nil {
			t.Fatalf("CommitPosting entry1 failed: %v", err)
		}

		// 2. Post Entry 2: Office Expense of 300 (Expense DR 300, Cash CR 300)
		entry2 := journal.JournalEntry{
			ID:                  types.NewJournalEntryID(),
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "P12-GL-002",
			SequenceNumber:      2,
			Description:         "Office Expense",
			EffectiveDate:       postDate,
			PostingDate:         postDate,
			Status:              journal.EntryPosted,
			Classification:      journal.EntryClassRegular,
			TransactionCurrency: f.FunctionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			Lines: []journal.EntryLine{
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.ExpenseAccount.ID,
					TransactionAmount:   mustMoney("300.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("300.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
				},
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.CashAccount.ID,
					TransactionAmount:   mustMoney("300.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("300.00", f.FunctionalCurrency),
				},
			},
		}

		posted2, err := h.Ports.Posting.CommitPosting(ctx, storage.PostingCommit{
			Entry:         entry2,
			Operation:     storage.OperationPost,
			ActorID:       "gl-rebuild-tester",
			Reason:        "Office Expense",
			CorrelationID: "gl-cor-002",
		})
		if err != nil {
			t.Fatalf("CommitPosting entry2 failed: %v", err)
		}

		// 3. Post Entry 3: Additional Expense of 150 (Expense DR 150, Cash CR 150)
		entry3 := journal.JournalEntry{
			ID:                  types.NewJournalEntryID(),
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "P12-GL-003",
			SequenceNumber:      3,
			Description:         "Additional Operating Expense",
			EffectiveDate:       postDate,
			PostingDate:         postDate,
			Status:              journal.EntryPosted,
			Classification:      journal.EntryClassRegular,
			TransactionCurrency: f.FunctionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			Lines: []journal.EntryLine{
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.ExpenseAccount.ID,
					TransactionAmount:   mustMoney("150.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("150.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
				},
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.CashAccount.ID,
					TransactionAmount:   mustMoney("150.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("0.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("150.00", f.FunctionalCurrency),
				},
			},
		}

		posted3, err := h.Ports.Posting.CommitPosting(ctx, storage.PostingCommit{
			Entry:         entry3,
			Operation:     storage.OperationPost,
			ActorID:       "gl-rebuild-tester",
			Reason:        "Additional Expense",
			CorrelationID: "gl-cor-003",
		})
		if err != nil {
			t.Fatalf("CommitPosting entry3 failed: %v", err)
		}

		// 4. Add an unposted Draft entry to canonical entries slice (must be skipped by rebuild)
		draftEntry := journal.JournalEntry{
			ID:                  types.NewJournalEntryID(),
			CompanyID:           f.Company.ID,
			JournalID:           f.Journal.ID,
			FiscalPeriodID:      f.OpenPeriod.ID,
			EntryNumber:         "DRAFT-999",
			SequenceNumber:      999,
			Description:         "Unposted Draft",
			EffectiveDate:       postDate,
			PostingDate:         postDate,
			Status:              journal.EntryDraft,
			Classification:      journal.EntryClassRegular,
			TransactionCurrency: f.FunctionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			Lines: []journal.EntryLine{
				{
					ID:                  types.NewJournalEntryLineID(),
					AccountID:           f.CashAccount.ID,
					TransactionAmount:   mustMoney("9999.00", f.FunctionalCurrency),
					TransactionCurrency: f.FunctionalCurrency,
					ExchangeRate:        decimal.NewFromInt(1),
					FunctionalDebit:     mustMoney("9999.00", f.FunctionalCurrency),
					FunctionalCredit:    mustMoney("0.00", f.FunctionalCurrency),
				},
			},
		}

		canonicalEntries := []journal.JournalEntry{
			posted1,
			posted2,
			posted3,
			draftEntry,
		}

		// Rebuild Account Balances from canonical journal entries
		rebuiltBalances, err := ledger.RebuildAccountBalances(canonicalEntries)
		if err != nil {
			t.Fatalf("RebuildAccountBalances failed: %v", err)
		}

		// Expected Net Positions:
		// Cash Account:
		//   Entry 1: +1000 DR
		//   Entry 2: -300 CR
		//   Entry 3: -150 CR
		//   Net Cash: +550 DR (1000 Debit, 450 Credit, Net = 550)
		// Revenue Account:
		//   Entry 1: +1000 CR
		//   Net Revenue: -1000 (0 Debit, 1000 Credit, Net = -1000)
		// Expense Account:
		//   Entry 2: +300 DR
		//   Entry 3: +150 DR
		//   Net Expense: +450 (450 Debit, 0 Credit, Net = 450)

		cashKey := ledger.AccountBalanceKey{
			CompanyID:          f.Company.ID,
			AccountID:          f.CashAccount.ID,
			FunctionalCurrency: f.FunctionalCurrency,
		}
		cashRebuilt, ok := rebuiltBalances[cashKey]
		if !ok {
			t.Fatalf("cash balance missing from rebuilt balances")
		}
		if !cashRebuilt.Net.Amount().Equal(decimal.NewFromInt(550)) {
			t.Errorf("rebuilt cash net = %s, want 550", cashRebuilt.Net.Amount().String())
		}
		if cashRebuilt.MovementCount != 3 {
			t.Errorf("rebuilt cash movements = %d, want 3", cashRebuilt.MovementCount)
		}

		revKey := ledger.AccountBalanceKey{
			CompanyID:          f.Company.ID,
			AccountID:          f.RevenueAccount.ID,
			FunctionalCurrency: f.FunctionalCurrency,
		}
		revRebuilt, ok := rebuiltBalances[revKey]
		if !ok {
			t.Fatalf("revenue balance missing from rebuilt balances")
		}
		if !revRebuilt.Credit.Amount().Equal(decimal.NewFromInt(1000)) {
			t.Errorf("rebuilt revenue credit = %s, want 1000", revRebuilt.Credit.Amount().String())
		}

		expKey := ledger.AccountBalanceKey{
			CompanyID:          f.Company.ID,
			AccountID:          f.ExpenseAccount.ID,
			FunctionalCurrency: f.FunctionalCurrency,
		}
		expRebuilt, ok := rebuiltBalances[expKey]
		if !ok {
			t.Fatalf("expense balance missing from rebuilt balances")
		}
		if !expRebuilt.Net.Amount().Equal(decimal.NewFromInt(450)) {
			t.Errorf("rebuilt expense net = %s, want 450", expRebuilt.Net.Amount().String())
		}

		// Verify Parity with Storage Adapter Perpetual Balances
		adapterCashBal, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.CashAccount.ID, asOf, f.FunctionalCurrency)
		if err != nil {
			t.Fatalf("adapter GetAccountBalance cash failed: %v", err)
		}
		if !adapterCashBal.Equal(cashRebuilt.Net.Amount()) {
			t.Errorf("adapter cash balance %s != rebuilt cash net %s", adapterCashBal.String(), cashRebuilt.Net.Amount().String())
		}

		adapterRevBal, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.RevenueAccount.ID, asOf, f.FunctionalCurrency)
		if err != nil {
			t.Fatalf("adapter GetAccountBalance revenue failed: %v", err)
		}
		// Storage adapter reports Credit-normal balance or absolute net
		if !adapterRevBal.Abs().Equal(revRebuilt.Credit.Amount()) {
			t.Errorf("adapter revenue balance %s != rebuilt revenue credit %s", adapterRevBal.String(), revRebuilt.Credit.Amount().String())
		}

		adapterExpBal, err := h.Ports.Balance.GetAccountBalance(ctx, f.Company.ID, f.ExpenseAccount.ID, asOf, f.FunctionalCurrency)
		if err != nil {
			t.Fatalf("adapter GetAccountBalance expense failed: %v", err)
		}
		if !adapterExpBal.Equal(expRebuilt.Net.Amount()) {
			t.Errorf("adapter expense balance %s != rebuilt expense net %s", adapterExpBal.String(), expRebuilt.Net.Amount().String())
		}

		// Rebuild Determinism Check: shuffle entries and ensure reconstructed balances are identical
		rng := rand.New(rand.NewSource(42))
		for round := 0; round < 5; round++ {
			shuffled := append([]journal.JournalEntry(nil), canonicalEntries...)
			rng.Shuffle(len(shuffled), func(i, j int) {
				shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
			})

			shuffledBalances, err := ledger.RebuildAccountBalances(shuffled)
			if err != nil {
				t.Fatalf("shuffled RebuildAccountBalances round %d failed: %v", round, err)
			}

			if !shuffledBalances[cashKey].Net.Amount().Equal(cashRebuilt.Net.Amount()) ||
				!shuffledBalances[revKey].Credit.Amount().Equal(revRebuilt.Credit.Amount()) ||
				!shuffledBalances[expKey].Net.Amount().Equal(expRebuilt.Net.Amount()) {
				t.Fatalf("rebuild determinism violated on shuffle round %d", round)
			}
		}
	})
}
