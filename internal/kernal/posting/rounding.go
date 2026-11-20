package posting

import (
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func RoundEntryToMinorUnits(entry *journal.JournalEntry, roundingAccountID types.AccountID) error {
	return RoundEntryWithRoundingPolicy(entry, roundingAccountID, nil)
}

func RoundEntryWithRoundingPolicy(
	entry *journal.JournalEntry,
	roundingAccountID types.AccountID,
	round func(types.Money) (types.Money, error),
) error {
	if entry == nil {
		return ErrInvalidEntry
	}
	if len(entry.Lines) == 0 {
		return ErrMinimumLines
	}

	functionalCurrency := ""
	rawResidual := decimal.Zero
	roundedResidual := decimal.Zero
	roundedLines := make([]struct {
		debit  types.Money
		credit types.Money
	}, len(entry.Lines))
	for i := range entry.Lines {
		line := &entry.Lines[i]
		currency := line.FunctionalDebit.Currency()
		if currency == "" {
			currency = line.FunctionalCredit.Currency()
		}
		if currency == "" {
			return ErrMissingFunctionalCurrency
		}
		if functionalCurrency == "" {
			functionalCurrency = currency
		} else if currency != functionalCurrency {
			return types.ErrCurrencyMismatch
		}

		rawResidual = rawResidual.Add(line.FunctionalDebit.Amount()).Sub(line.FunctionalCredit.Amount())
		roundedDebit, err := roundMoney(line.FunctionalDebit, round)
		if err != nil {
			return err
		}
		roundedCredit, err := roundMoney(line.FunctionalCredit, round)
		if err != nil {
			return err
		}
		roundedResidual = roundedResidual.Add(roundedDebit.Amount()).Sub(roundedCredit.Amount())
		roundedLines[i].debit = roundedDebit
		roundedLines[i].credit = roundedCredit
	}

	residual := rawResidual
	applyRoundedValues := residual.IsZero()
	if applyRoundedValues {
		residual = roundedResidual
	}

	if !residual.IsZero() {
		if roundingAccountID == (types.AccountID{}) {
			return ErrRoundingAccountRequired
		}
		amount, err := types.NewMoney(residual.Abs(), functionalCurrency)
		if err != nil {
			return err
		}
		zero, err := types.Zero(functionalCurrency)
		if err != nil {
			return err
		}
		line := journal.EntryLine{
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           roundingAccountID,
			TransactionAmount:   amount,
			TransactionCurrency: functionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zero,
			FunctionalCredit:    amount,
		}
		if residual.IsNegative() {
			line.FunctionalDebit = amount
			line.FunctionalCredit = zero
		}
		entry.Lines = append(entry.Lines, line)
	}
	if applyRoundedValues {
		for i := range entry.Lines[:len(roundedLines)] {
			entry.Lines[i].FunctionalDebit = roundedLines[i].debit
			entry.Lines[i].FunctionalCredit = roundedLines[i].credit
		}
	}
	return ValidateEntryDoubleEntry(entry, nil)
}

func roundMoney(amount types.Money, round func(types.Money) (types.Money, error)) (types.Money, error) {
	if round != nil {
		return round(amount)
	}
	return amount.Rounded()
}
