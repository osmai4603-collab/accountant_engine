package posting

import (
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func TestRoundEntryToMinorUnitsRoundsBeforeSettlingResidual(t *testing.T) {
	debit := makeMoney(t, "100.004", "USD")
	credit := makeMoney(t, "100.000", "USD")
	zero := makeMoney(t, "0", "USD")
	entry := journal.JournalEntry{
		ID:            types.NewJournalEntryID(),
		CompanyID:     types.NewCompanyID(),
		JournalID:     types.NewJournalID(),
		Status:        journal.EntryDraft,
		EffectiveDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
				TransactionAmount: debit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: debit, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
				TransactionAmount: credit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: zero, FunctionalCredit: credit,
			},
		},
	}

	if err := RoundEntryToMinorUnits(&entry, types.NewAccountID()); err != nil {
		t.Fatalf("RoundEntryToMinorUnits: %v", err)
	}
	if len(entry.Lines) != 2 {
		t.Fatalf("rounded net-zero entry has %d lines, want 2", len(entry.Lines))
	}
	if got := entry.Lines[0].FunctionalDebit.Amount().StringFixed(2); got != "100.00" {
		t.Fatalf("rounded debit = %s, want 100.00", got)
	}
	if got := entry.Lines[1].FunctionalCredit.Amount().StringFixed(2); got != "100.00" {
		t.Fatalf("rounded credit = %s, want 100.00", got)
	}
	for i, line := range entry.Lines {
		if _, err := line.FunctionalDebit.MinorUnitsExact(); err != nil {
			t.Errorf("line %d debit is not storable in minor units: %v", i, err)
		}
		if _, err := line.FunctionalCredit.MinorUnitsExact(); err != nil {
			t.Errorf("line %d credit is not storable in minor units: %v", i, err)
		}
	}
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		t.Fatalf("rounded entry validation: %v", err)
	}
}

func TestRoundBalancedEntrySettlesRoundedFunctionalDifference(t *testing.T) {
	debitA := makeMoney(t, "10.005", "USD")
	debitB := makeMoney(t, "10.005", "USD")
	credit := makeMoney(t, "20.010", "USD")
	zero := makeMoney(t, "0", "USD")
	entry := journal.JournalEntry{
		ID:            types.NewJournalEntryID(),
		CompanyID:     types.NewCompanyID(),
		JournalID:     types.NewJournalID(),
		Status:        journal.EntryDraft,
		EffectiveDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
				TransactionAmount: debitA, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: debitA, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
				TransactionAmount: debitB, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: debitB, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
				TransactionAmount: credit, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: zero, FunctionalCredit: credit,
			},
		},
	}
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		t.Fatalf("entry should balance before rounding: %v", err)
	}

	roundingAccountID := types.NewAccountID()
	if err := RoundEntryToMinorUnits(&entry, roundingAccountID); err != nil {
		t.Fatalf("RoundEntryToMinorUnits: %v", err)
	}
	settlement := entry.Lines[len(entry.Lines)-1]
	if settlement.AccountID != roundingAccountID ||
		settlement.FunctionalDebit.Amount().StringFixed(2) != "0.01" ||
		!settlement.FunctionalCredit.Amount().IsZero() {
		t.Fatalf("settlement = %+v, want 0.01 debit to rounding account", settlement)
	}
	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		t.Fatalf("rounded entry validation: %v", err)
	}
	for i, line := range entry.Lines {
		if _, err := line.FunctionalDebit.MinorUnitsExact(); err != nil {
			t.Errorf("line %d debit is not storable in minor units: %v", i, err)
		}
		if _, err := line.FunctionalCredit.MinorUnitsExact(); err != nil {
			t.Errorf("line %d credit is not storable in minor units: %v", i, err)
		}
	}
}
