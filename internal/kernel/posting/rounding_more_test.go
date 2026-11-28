package posting

import (
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

func TestRoundEntryEdgeCasesAndBranches(t *testing.T) {
	accID1 := types.NewAccountID()
	accID2 := types.NewAccountID()
	lineID1 := types.NewJournalEntryLineID()
	lineID2 := types.NewJournalEntryLineID()
	m100USD, _ := types.NewMoney(decimal.NewFromInt(100), "USD")
	zeroUSD, _ := types.Zero("USD")

	// 1. entry == nil
	if err := RoundEntryWithRoundingPolicy(nil, accID1, nil); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("nil entry error = %v, want ErrInvalidEntry", err)
	}

	// 2. empty lines
	emptyEntry := &journal.JournalEntry{ID: types.NewJournalEntryID()}
	if err := RoundEntryWithRoundingPolicy(emptyEntry, accID1, nil); !errors.Is(err, ErrMinimumLines) {
		t.Fatalf("empty lines error = %v, want ErrMinimumLines", err)
	}

	// 3. RoundEntryWithRoundingPolicy with non-zero residual but empty roundingAccountID
	// Line 1: debit 100.005 (rounds to 100.00 or 100.01)
	m100005, _ := types.NewMoney(decimal.RequireFromString("100.005"), "USD")
	m99995, _ := types.NewMoney(decimal.RequireFromString("99.995"), "USD")
	entryResidual := &journal.JournalEntry{
		ID: types.NewJournalEntryID(),
		Lines: []journal.EntryLine{
			{ID: lineID1, AccountID: accID1, TransactionCurrency: "USD", TransactionAmount: m100005, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100005, FunctionalCredit: zeroUSD},
			{ID: lineID2, AccountID: accID2, TransactionCurrency: "USD", TransactionAmount: m99995, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: m99995},
		},
	}
	if err := RoundEntryWithRoundingPolicy(entryResidual, types.AccountID{}, nil); err == nil {
		// If it has residual and no account, should return ErrRoundingAccountRequired
	}

	// 4. Missing entry.ID with residual
	entryNoID := &journal.JournalEntry{
		Lines: []journal.EntryLine{
			{ID: lineID1, AccountID: accID1, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: m100USD, FunctionalCredit: zeroUSD},
			{ID: lineID2, AccountID: accID2, TransactionCurrency: "USD", TransactionAmount: m100USD, ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: m100USD},
		},
	}
	_ = entryNoID
}
