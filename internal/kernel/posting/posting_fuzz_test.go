package posting

import (
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

func FuzzValidateEntryDoubleEntryNeverPanics(f *testing.F) {
	for _, seed := range [][2]int64{
		{100, 100},
		{10001, 10000},
		{-100, -100},
		{0, 0},
		{1, -1},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, debitMinor, creditMinor int64) {
		if debitMinor < -1_000_000_000 || debitMinor > 1_000_000_000 ||
			creditMinor < -1_000_000_000 || creditMinor > 1_000_000_000 {
			return
		}
		debit, err := types.NewMoney(decimal.NewFromInt(debitMinor).Shift(-2), "USD")
		if err != nil {
			return
		}
		credit, err := types.NewMoney(decimal.NewFromInt(creditMinor).Shift(-2), "USD")
		if err != nil {
			return
		}
		zero, err := types.Zero("USD")
		if err != nil {
			t.Fatal(err)
		}
		entry := journal.JournalEntry{
			ID: types.NewJournalEntryID(), JournalID: types.NewJournalID(),
			CompanyID: types.NewCompanyID(), Status: journal.EntryDraft,
			Classification: journal.EntryClassRegular,
			EffectiveDate:  time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
			Lines: []journal.EntryLine{
				{
					ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
					TransactionAmount: debit, TransactionCurrency: "USD",
					ExchangeRate:    decimal.NewFromInt(1),
					FunctionalDebit: debit, FunctionalCredit: zero,
				},
				{
					ID: types.NewJournalEntryLineID(), AccountID: types.NewAccountID(),
					TransactionAmount: credit, TransactionCurrency: "USD",
					ExchangeRate:    decimal.NewFromInt(1),
					FunctionalDebit: zero, FunctionalCredit: credit,
				},
			},
		}
		err = ValidateEntryDoubleEntry(&entry, nil)
		switch {
		case debitMinor <= 0 || creditMinor <= 0:
			if err == nil {
				t.Fatal("entry with a non-positive line amount was accepted")
			}
		case debitMinor == creditMinor:
			if err != nil {
				t.Fatalf("balanced entry rejected: %v", err)
			}
		case !errors.Is(err, ErrUnbalancedEntry):
			t.Fatalf("unbalanced entry error = %v, want ErrUnbalancedEntry", err)
		}
	})
}

func FuzzEntryLineRejectsCurrencyAndExchangeRateMismatch(f *testing.F) {
	f.Add(int64(2), false)
	f.Add(int64(0), false)
	f.Add(int64(1), true)
	f.Fuzz(func(t *testing.T, rate int64, currencyMismatch bool) {
		if rate < -1_000_000 || rate > 1_000_000 {
			return
		}
		entry := newPostingTestEntry(t, "creator")
		line := entry.Lines[0]
		want := ErrInvalidExchangeRate
		if currencyMismatch {
			line.TransactionCurrency = "EUR"
			want = ErrTransactionCurrencyMismatch
		} else {
			if rate == 1 {
				rate = 2
			}
			line.ExchangeRate = decimal.NewFromInt(rate)
		}
		if err := ValidateEntryLine(line, nil); !errors.Is(err, want) {
			t.Fatalf("line validation error = %v, want %v", err, want)
		}
	})
}
