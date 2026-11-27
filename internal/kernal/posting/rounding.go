package posting

import (
	"errors"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

var ErrRoundingEntryIDRequired = errors.New("journal entry id is required for a rounding settlement")

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
	roundedLines := make([]struct {
		transaction types.Money
		debit       types.Money
		credit      types.Money
	}, len(entry.Lines))
	for i := range entry.Lines {
		line := &entry.Lines[i]
		if err := ValidateEntryLine(*line, round); err != nil {
			return err
		}
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

		if line.FunctionalDebit.Currency() != currency || line.FunctionalCredit.Currency() != currency {
			return types.ErrCurrencyMismatch
		}
		if line.TransactionAmount.Currency() != line.TransactionCurrency {
			return ErrTransactionCurrencyMismatch
		}
		roundedTransaction, err := roundMoney(line.TransactionAmount, round)
		if err != nil {
			return err
		}
		if roundedTransaction.Currency() != line.TransactionCurrency {
			return types.ErrCurrencyMismatch
		}
		if _, err := roundedTransaction.MinorUnitsExact(); err != nil {
			return err
		}
		expectedFunctional, err := types.NewMoney(
			roundedTransaction.Amount().Mul(line.ExchangeRate),
			currency,
		)
		if err != nil {
			return err
		}
		expectedFunctional, err = roundMoney(expectedFunctional, round)
		if err != nil {
			return err
		}
		zero, err := types.Zero(currency)
		if err != nil {
			return err
		}
		roundedDebit, roundedCredit := zero, zero
		if line.FunctionalDebit.Amount().IsPositive() {
			roundedDebit = expectedFunctional
		} else {
			roundedCredit = expectedFunctional
		}
		if roundedDebit.Currency() != currency || roundedCredit.Currency() != currency {
			return types.ErrCurrencyMismatch
		}
		if _, err := roundedDebit.MinorUnitsExact(); err != nil {
			return err
		}
		if _, err := roundedCredit.MinorUnitsExact(); err != nil {
			return err
		}
		roundedLines[i].transaction = roundedTransaction
		roundedLines[i].debit = roundedDebit
		roundedLines[i].credit = roundedCredit
	}

	residual := decimal.Zero
	for _, line := range roundedLines {
		residual = residual.Add(line.debit.Amount()).Sub(line.credit.Amount())
	}

	var settlementLine journal.EntryLine
	if !residual.IsZero() {
		if roundingAccountID == (types.AccountID{}) {
			return ErrRoundingAccountRequired
		}
		if entry.ID == (types.JournalEntryID{}) {
			return ErrRoundingEntryIDRequired
		}
		amount, err := types.NewMoney(residual.Abs(), functionalCurrency)
		if err != nil {
			return err
		}
		zero, err := types.Zero(functionalCurrency)
		if err != nil {
			return err
		}
		settlementLine = journal.EntryLine{
			ID: types.JournalEntryLineID(uuid.NewSHA1(
				uuid.NameSpaceOID,
				[]byte(entry.ID.String()+"\x00rounding:"+roundingAccountID.String()),
			)),
			AccountID:           roundingAccountID,
			TransactionAmount:   amount,
			TransactionCurrency: functionalCurrency,
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zero,
			FunctionalCredit:    amount,
		}
		if residual.IsNegative() {
			settlementLine.FunctionalDebit = amount
			settlementLine.FunctionalCredit = zero
		}
	}
	normalizedLines := append([]journal.EntryLine(nil), entry.Lines...)
	for i := range roundedLines {
		normalizedLines[i].TransactionAmount = roundedLines[i].transaction
		normalizedLines[i].FunctionalDebit = roundedLines[i].debit
		normalizedLines[i].FunctionalCredit = roundedLines[i].credit
	}
	if !residual.IsZero() {
		normalizedLines = append(normalizedLines, settlementLine)
	}
	normalizedEntry := *entry
	normalizedEntry.Lines = normalizedLines
	if err := ValidateEntryDoubleEntry(&normalizedEntry, round); err != nil {
		return err
	}
	*entry = normalizedEntry
	return nil
}

func roundMoney(amount types.Money, round func(types.Money) (types.Money, error)) (types.Money, error) {
	if round != nil {
		return round(amount)
	}
	return amount.Rounded()
}
