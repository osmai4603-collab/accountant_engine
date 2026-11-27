package posting

import (
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func BenchmarkValidateEntryDoubleEntry(b *testing.B) {
	amt, _ := types.NewMoneyFromString("1250.50", "USD")
	zero, _ := types.Zero("USD")
	entry := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      types.NewCompanyID(),
		JournalID:      types.NewJournalID(),
		Status:         journal.EntryDraft,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionAmount:   amt,
				TransactionCurrency: "USD",
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     amt,
				FunctionalCredit:    zero,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionAmount:   amt,
				TransactionCurrency: "USD",
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zero,
				FunctionalCredit:    amt,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoundEntryToMinorUnits(b *testing.B) {
	amt, _ := types.NewMoneyFromString("100.005", "USD")
	zero, _ := types.Zero("USD")
	roundAcc := types.NewAccountID()
	base := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      types.NewCompanyID(),
		JournalID:      types.NewJournalID(),
		Status:         journal.EntryDraft,
		Classification: journal.EntryClassRegular,
		EffectiveDate:  time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		Lines: []journal.EntryLine{
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionAmount:   amt,
				TransactionCurrency: "USD",
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     amt,
				FunctionalCredit:    zero,
			},
			{
				ID:                  types.NewJournalEntryLineID(),
				AccountID:           types.NewAccountID(),
				TransactionAmount:   amt,
				TransactionCurrency: "USD",
				ExchangeRate:        decimal.NewFromInt(1),
				FunctionalDebit:     zero,
				FunctionalCredit:    amt,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := base.Clone()
		if err := RoundEntryToMinorUnits(&entry, roundAcc); err != nil {
			b.Fatal(err)
		}
	}
}
