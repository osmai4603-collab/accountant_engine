package ifrs16

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

// Unit coordinates lease schedule, modification, and period-entry operations.
type Unit struct{}

func NewUnit() *Unit { return &Unit{} }

func (u *Unit) AmortizationSchedule(contract LeaseContract) ([]SchedulePeriod, error) {
	if u == nil {
		return nil, ErrInvalidContract
	}
	return BuildAmortizationSchedule(contract)
}

func (u *Unit) ModifyLeaseContext(
	contract LeaseContract,
	effectiveDate time.Time,
	remainingPayments []LeasePayment,
	newAnnualDiscountRate decimal.Decimal,
	liabilityCarryingAmount, rouCarryingAmount types.Money,
) (ModificationResult, error) {
	if u == nil {
		return ModificationResult{}, ErrInvalidContract
	}
	return contract.RecalculateModification(effectiveDate, remainingPayments, newAnnualDiscountRate,
		liabilityCarryingAmount, rouCarryingAmount)
}

// ClosePeriod returns balanced payment/interest and depreciation journal drafts
// for all schedule periods due by the requested date.
func (u *Unit) ClosePeriod(
	ctx context.Context,
	contract LeaseContract,
	through time.Time,
	builder *EntryBuilder,
	mappingID string,
	companyID, journalID uuid.UUID,
	createdBy string,
) ([]journal.JournalEntry, error) {
	if u == nil || through.IsZero() || builder == nil {
		return nil, ErrInvalidContract
	}
	if contract.Reference == "" {
		return nil, ErrLeaseReferenceRequired
	}
	schedule, err := BuildAmortizationSchedule(contract)
	if err != nil {
		return nil, err
	}
	entries := make([]journal.JournalEntry, 0, len(schedule)*2)
	for _, period := range schedule {
		if period.DueDate.After(through) {
			continue
		}
		payment, err := builder.BuildPaymentEntry(ctx, period, companyID, journalID, mappingID, createdBy)
		if err != nil {
			return nil, err
		}
		depreciation, err := builder.BuildDepreciationEntry(ctx, period, companyID, journalID, mappingID, createdBy)
		if err != nil {
			return nil, err
		}
		entries = append(entries, payment, depreciation)
	}
	return entries, nil
}

// zeroMoney returns a zero amount in the given currency. Invalid currencies
// produce a zero-value Money, which the entry builders reject before posting.
func zeroMoney(currency string) types.Money {
	m, err := types.NewMoney(decimal.Zero, currency)
	if err != nil {
		return types.Money{}
	}
	return m
}

// debitLine builds a functional debit line directly from a domain account UUID.
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

// creditLine builds a functional credit line directly from a domain account UUID.
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

// newJournalEntry creates a deterministic, balanced journal draft from the
// supplied lines. It replaces the reference project's posting.NewJournalEntry.
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
	debit, credit := decimal.Zero, decimal.Zero
	for _, line := range lines {
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	if !debit.Equal(credit) || debit.IsZero() {
		return journal.JournalEntry{}, errors.New("entry lines must be balanced")
	}
	return journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		JournalID:           types.JournalID(journalID),
		EffectiveDate:       date.UTC(),
		PostingDate:         date.UTC(),
		Status:              journal.EntryDraft,
		Classification:      journal.EntryClassRegular,
		Description:         description,
		TransactionCurrency: lines[0].TransactionCurrency,
		ExchangeRate:        decimal.NewFromInt(1),
		Lines:               append([]journal.EntryLine(nil), lines...),
		CreatedAt:           time.Now().UTC(),
	}, nil
}

// newLeaseEntry stamps the company and creator on a balanced journal draft.
func newLeaseEntry(
	date time.Time, companyID, journalID uuid.UUID, createdBy string, lines []journal.EntryLine,
) (journal.JournalEntry, error) {
	entry, err := newJournalEntry(journalID, date, "", lines)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	entry.CompanyID = types.CompanyID(companyID)
	entry.CreatedBy = createdBy
	entry.UpdatedAt = entry.CreatedAt
	return entry, nil
}
