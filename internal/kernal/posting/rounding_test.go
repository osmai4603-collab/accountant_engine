package posting

import (
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func TestRoundEntryDetectsSubMinorResidual(t *testing.T) {
	debit, _ := types.NewMoneyFromString("100.004", "USD")
	credit, _ := types.NewMoneyFromString("100.000", "USD")
	zero, _ := types.Zero("USD")
	entry := journal.JournalEntry{
		ID:            types.NewJournalEntryID(),
		CompanyID:     types.NewCompanyID(),
		JournalID:     types.NewJournalID(),
		Status:        journal.EntryDraft,
		EffectiveDate: time.Now().UTC(),
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

	err := RoundEntryToMinorUnits(&entry, types.AccountID{})
	if !errors.Is(err, ErrRoundingAccountRequired) {
		t.Fatalf("expected ErrRoundingAccountRequired, got %v", err)
	}

	roundingAccount := types.NewAccountID()
	if err := RoundEntryToMinorUnits(&entry, roundingAccount); err != nil {
		t.Fatalf("expected automatic rounding settlement, got %v", err)
	}
	if len(entry.Lines) != 3 {
		t.Fatalf("expected explicit rounding line, got %d lines", len(entry.Lines))
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
