package posting

import (
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

func TestRoundEntryRoundsSubMinorDifferenceBeforeSettling(t *testing.T) {
	debit, _ := types.NewMoneyFromString("100.004", "USD")
	credit, _ := types.NewMoneyFromString("100.000", "USD")
	zero, _ := types.Zero("USD")
	entry := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      types.NewCompanyID(),
		JournalID:      types.NewJournalID(),
		Classification: journal.EntryClassRegular,
		Status:         journal.EntryDraft,
		EffectiveDate:  time.Now().UTC(),
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionAmount:   debit,
				TransactionCurrency: "USD",
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     debit,
				FunctionalCredit:    zero,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionAmount:   credit,
				TransactionCurrency: "USD",
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zero,
				FunctionalCredit:    credit,
			},
		},
	}

	if err := RoundEntryToMinorUnits(&entry, types.AccountID{}); err != nil {
		t.Fatalf("rounding a net-zero minor-unit difference: %v", err)
	}
	if len(entry.Lines) != 2 {
		t.Fatalf("rounded net-zero entry has %d lines, want 2", len(entry.Lines))
	}
	if !entry.Lines[0].FunctionalDebit.Amount().Equal(decimal.NewFromInt(100)) ||
		!entry.Lines[1].FunctionalCredit.Amount().Equal(decimal.NewFromInt(100)) {
		t.Fatalf("functional amounts were not rounded to cents: debit=%s credit=%s",
			entry.Lines[0].FunctionalDebit, entry.Lines[1].FunctionalCredit)
	}
}

func TestEntryLineRejectsTransactionCurrencyMismatch(t *testing.T) {
	usd, _ := types.NewMoneyFromString("100.00", "USD")
	zeroUSD, _ := types.Zero("USD")
	line := journal.EntryLine{
		AccountID:           types.NewAccountID(),
		TransactionAmount:   usd,
		TransactionCurrency: "EUR",
		ExchangeRate:        decimal.NewFromInt(1),
		FunctionalDebit:     usd,
		FunctionalCredit:    zeroUSD,
	}
	err := ValidateEntryLine(line, nil)
	if !errors.Is(err, ErrTransactionCurrencyMismatch) {
		t.Fatalf("expected transaction currency mismatch, got %v", err)
	}
}

func TestEntryLineRequiresUnitRateForMatchingCurrencies(t *testing.T) {
	amount := makeMoney(t, "100.00", "USD")
	zero := makeMoney(t, "0.00", "USD")
	line := journal.EntryLine{
		ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
		TransactionAmount: amount, TransactionCurrency: "USD",
		ExchangeRate:    decimal.RequireFromString("1.2"),
		FunctionalDebit: makeMoney(t, "120.00", "USD"), FunctionalCredit: zero,
	}
	if err := ValidateEntryLine(line, nil); !errors.Is(err, ErrInvalidExchangeRate) {
		t.Fatalf("same-currency non-unit rate error = %v, want invalid exchange rate", err)
	}

	line.ExchangeRate = decimal.NewFromInt(1)
	line.FunctionalDebit = amount
	if err := ValidateEntryLine(line, nil); err != nil {
		t.Fatalf("same-currency unit rate: %v", err)
	}
}

func TestRoundingSettlementIDIsStableForSameEntry(t *testing.T) {
	roundingAccountID := types.NewAccountID()
	newUnroundedEntry := func() journal.JournalEntry {
		debitA := makeMoney(t, "10.005", "USD")
		debitB := makeMoney(t, "10.005", "USD")
		credit := makeMoney(t, "20.010", "USD")
		zero := makeMoney(t, "0", "USD")
		return journal.JournalEntry{
			ID:             types.NewJournalEntryID(),
			CompanyID:      types.NewCompanyID(),
			JournalID:      types.NewJournalID(),
			Classification: journal.EntryClassRegular,
			EffectiveDate:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			Lines: []journal.EntryLine{
				{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debitA,
					TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debitA, FunctionalCredit: zero},
				{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debitB,
					TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debitB, FunctionalCredit: zero},
				{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: credit,
					TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: credit},
			},
		}
	}

	first := newUnroundedEntry()
	second := first
	second.Lines = append([]journal.EntryLine(nil), first.Lines...)
	for _, entry := range []*journal.JournalEntry{&first, &second} {
		if err := RoundEntryToMinorUnits(entry, roundingAccountID); err != nil {
			t.Fatalf("RoundEntryToMinorUnits: %v", err)
		}
	}
	if first.Lines[3].ID != second.Lines[3].ID {
		t.Fatalf("settlement IDs differ for same entry: %s vs %s", first.Lines[3].ID, second.Lines[3].ID)
	}
}

func TestRoundingFailureDoesNotPartiallyMutateEntry(t *testing.T) {
	debit := makeMoney(t, "10.005", "USD")
	credit := makeMoney(t, "20.010", "USD")
	zero := makeMoney(t, "0", "USD")
	secondDebit := makeMoney(t, "10.005", "USD")
	entry := journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: debit,
				TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: debit, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: secondDebit,
				TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: secondDebit, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(), TransactionAmount: credit,
				TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: credit},
		},
	}
	before := entry.Lines[0].TransactionAmount
	if err := RoundEntryToMinorUnits(&entry, types.AccountID{}); !errors.Is(err, ErrRoundingAccountRequired) {
		t.Fatalf("RoundEntryToMinorUnits error = %v, want rounding account required", err)
	}
	if !entry.Lines[0].TransactionAmount.Equal(before) || len(entry.Lines) != 3 {
		t.Fatal("failed rounding partially mutated the entry")
	}
}
