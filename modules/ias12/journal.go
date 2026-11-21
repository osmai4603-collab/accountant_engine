package ias12

import (
	"strings"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

// GenerateDeferredTaxEntry posts each gross DTA/DTL change, including reversals.
// Every line is a public journal.EntryLine built by the local debitLine/
// creditLine helpers; no internal posting types are involved.
func GenerateDeferredTaxEntry(input DeferredTaxEntryInput) (journal.JournalEntry, error) {
	if input.CompanyID == uuid.Nil || input.JournalID == uuid.Nil || input.EntryDate.IsZero() ||
		strings.TrimSpace(input.CreatedBy) == "" ||
		input.Accounts.Expense == uuid.Nil || input.Accounts.DTA == uuid.Nil || input.Accounts.DTL == uuid.Nil ||
		input.Accounts.Expense == input.Accounts.DTA || input.Accounts.Expense == input.Accounts.DTL ||
		input.Accounts.DTA == input.Accounts.DTL ||
		(input.Accounts.OCI != uuid.Nil && (input.Accounts.OCI == input.Accounts.Expense ||
			input.Accounts.OCI == input.Accounts.DTA || input.Accounts.OCI == input.Accounts.DTL)) {
		return journal.JournalEntry{}, ErrInvalidInput
	}
	for _, amount := range []types.Money{
		input.Previous.DTA, input.Previous.DTL, input.Closing.DTA, input.Closing.DTL,
	} {
		if err := validateNonnegativeMoney(amount); err != nil {
			return journal.JournalEntry{}, err
		}
	}
	currency := input.Previous.DTA.Currency()
	for _, amount := range []types.Money{
		input.Previous.DTL, input.Closing.DTA, input.Closing.DTL,
	} {
		if amount.Currency() != currency {
			return journal.JournalEntry{}, ErrCurrencyMismatch
		}
	}
	if input.OCIChange.DTA.Currency() == "" {
		input.OCIChange.DTA, _ = types.Zero(currency)
	}
	if input.OCIChange.DTL.Currency() == "" {
		input.OCIChange.DTL, _ = types.Zero(currency)
	}
	for _, amount := range []types.Money{input.OCIChange.DTA, input.OCIChange.DTL} {
		if err := validateSignedMoney(amount); err != nil {
			return journal.JournalEntry{}, err
		}
		if amount.Currency() != currency {
			return journal.JournalEntry{}, ErrCurrencyMismatch
		}
		if !amount.Amount().IsZero() && input.Accounts.OCI == uuid.Nil {
			return journal.JournalEntry{}, ErrInvalidInput
		}
	}

	dtaChange := input.Closing.DTA.Amount().Sub(input.Previous.DTA.Amount())
	dtlChange := input.Closing.DTL.Amount().Sub(input.Previous.DTL.Amount())
	lines := make([]journal.EntryLine, 0, 4)
	appendMovement := func(account uuid.UUID, change decimal.Decimal, asset bool, offsetAccount uuid.UUID) error {
		amount, err := types.NewMoney(change.Abs(), currency)
		if err != nil {
			return err
		}
		amount = amount.RoundForPosting()
		if amount.Amount().IsZero() {
			return nil
		}
		debitAccount, creditAccount := offsetAccount, account
		if asset && change.IsPositive() {
			debitAccount, creditAccount = account, offsetAccount
		} else if !asset && change.IsNegative() {
			debitAccount, creditAccount = account, offsetAccount
		}
		lines = append(lines,
			debitLine(debitAccount, amount, nil),
			creditLine(creditAccount, amount, nil),
		)
		return nil
	}
	if err := appendMovement(input.Accounts.DTA, dtaChange.Sub(input.OCIChange.DTA.Amount()), true, input.Accounts.Expense); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := appendMovement(input.Accounts.DTL, dtlChange.Sub(input.OCIChange.DTL.Amount()), false, input.Accounts.Expense); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := appendMovement(input.Accounts.DTA, input.OCIChange.DTA.Amount(), true, input.Accounts.OCI); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := appendMovement(input.Accounts.DTL, input.OCIChange.DTL.Amount(), false, input.Accounts.OCI); err != nil {
		return journal.JournalEntry{}, err
	}
	if len(lines) == 0 {
		return journal.JournalEntry{}, ErrNoDeferredTaxMovement
	}

	const reference = "Deferred tax measurement adjustment"
	entry, err := newJournalEntry(input.JournalID, input.EntryDate, reference, lines)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	entry.CompanyID = types.CompanyID(input.CompanyID)
	entry.CreatedBy = input.CreatedBy
	entry.Classification = journal.EntryClassAdjusting
	entry.AdjustingEntryType = journal.AdjustingEntryProvisionEstimate
	entry.AdjustmentReason = reference
	return entry, nil
}

// GenerateCurrentTaxEntry accrues the period's current income tax expense.
func GenerateCurrentTaxEntry(input CurrentTaxEntryInput) (journal.JournalEntry, error) {
	if input.CompanyID == uuid.Nil || input.JournalID == uuid.Nil || input.EntryDate.IsZero() ||
		strings.TrimSpace(input.CreatedBy) == "" || input.ExpenseAccountID == uuid.Nil ||
		input.PayableAccountID == uuid.Nil || input.ExpenseAccountID == input.PayableAccountID ||
		input.Amount.Currency() == "" || !input.Amount.Amount().IsPositive() {
		return journal.JournalEntry{}, ErrInvalidInput
	}
	if err := validateNonnegativeMoney(input.Amount); err != nil {
		return journal.JournalEntry{}, err
	}
	amount := input.Amount.RoundForPosting()
	const reference = "Current income tax accrual"
	entry, err := newJournalEntry(input.JournalID, input.EntryDate, reference, []journal.EntryLine{
		debitLine(input.ExpenseAccountID, amount, nil),
		creditLine(input.PayableAccountID, amount, nil),
	})
	if err != nil {
		return journal.JournalEntry{}, err
	}
	entry.CompanyID = types.CompanyID(input.CompanyID)
	entry.CreatedBy = input.CreatedBy
	entry.Classification = journal.EntryClassAdjusting
	entry.AdjustingEntryType = journal.AdjustingEntryAccrual
	entry.AdjustmentReason = reference
	return entry, nil
}
