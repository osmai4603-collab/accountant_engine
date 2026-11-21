package ifrs9

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

// zeroMoney returns a zero amount in the requested currency. Invalid
// currencies produce the zero value of types.Money so callers can reject it
// through their normal validation path.
func zeroMoney(currency string) types.Money {
	m, err := types.NewMoney(decimal.Zero, currency)
	if err != nil {
		return types.Money{}
	}
	return m
}

// debitLine builds a public journal debit line. The legacy separate
// Debit/Credit fields are represented by FunctionalDebit/FunctionalCredit
// in the public journal API.
func debitLine(accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := journal.EntryLine{
		ID:                  types.NewJournalEntryLineID(),
		AccountID:           types.AccountID(accountID),
		TransactionAmount:   amount,
		TransactionCurrency: amount.Currency(),
		ExchangeRate:        decimal.NewFromInt(1),
		FunctionalDebit:     amount,
		FunctionalCredit:    zeroMoney(amount.Currency()),
	}
	if partnerID != nil {
		id := types.PartnerID(*partnerID)
		line.PartnerID = &id
	}
	return line
}

// creditLine builds a public journal credit line.
func creditLine(accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := journal.EntryLine{
		ID:                  types.NewJournalEntryLineID(),
		AccountID:           types.AccountID(accountID),
		TransactionAmount:   amount,
		TransactionCurrency: amount.Currency(),
		ExchangeRate:        decimal.NewFromInt(1),
		FunctionalDebit:     zeroMoney(amount.Currency()),
		FunctionalCredit:    amount,
	}
	if partnerID != nil {
		id := types.PartnerID(*partnerID)
		line.PartnerID = &id
	}
	return line
}

// debitLineWithID and creditLineWithID keep the deterministic line identities
// used by the reference ECL and measurement drafts.
func debitLineWithID(id, accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := debitLine(accountID, amount, partnerID)
	line.ID = types.JournalEntryLineID(id)
	return line
}

func creditLineWithID(id, accountID uuid.UUID, amount types.Money, partnerID *uuid.UUID) journal.EntryLine {
	line := creditLine(accountID, amount, partnerID)
	line.ID = types.JournalEntryLineID(id)
	return line
}

// newJournalEntry validates the double-entry invariant and constructs a
// public journal.Entry draft. Callers override deterministic IDs, company,
// created-by metadata, and classification when the workflow requires it.
func newJournalEntry(journalID uuid.UUID, date time.Time, description string, lines []journal.EntryLine) (journal.JournalEntry, error) {
	if journalID == uuid.Nil {
		return journal.JournalEntry{}, errors.New("journal ID is required")
	}
	if date.IsZero() {
		return journal.JournalEntry{}, errors.New("entry date is required")
	}
	if len(lines) < 2 {
		return journal.JournalEntry{}, errors.New("at least two lines are required")
	}
	debit, credit := journalEntryTotals(lines)
	if !debit.Equal(credit) || debit.IsZero() {
		return journal.JournalEntry{}, errors.New("entry lines must be balanced")
	}
	currency := lines[0].TransactionCurrency
	for _, line := range lines {
		if line.TransactionCurrency != currency {
			return journal.JournalEntry{}, errors.New("entry lines must use one transaction currency")
		}
	}
	return journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		JournalID:           types.JournalID(journalID),
		EffectiveDate:       date.UTC(),
		PostingDate:         date.UTC(),
		Status:              journal.EntryDraft,
		Classification:      journal.EntryClassRegular,
		Description:         description,
		TransactionCurrency: currency,
		ExchangeRate:        decimal.NewFromInt(1),
		Lines:               append([]journal.EntryLine(nil), lines...),
		CreatedAt:           time.Now().UTC(),
	}, nil
}

func journalEntryTotals(lines []journal.EntryLine) (decimal.Decimal, decimal.Decimal) {
	debit, credit := decimal.Zero, decimal.Zero
	for _, line := range lines {
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	return debit, credit
}

func journalEntryBalanced(lines []journal.EntryLine) bool {
	debit, credit := journalEntryTotals(lines)
	return debit.Equal(credit) && !debit.IsZero()
}
