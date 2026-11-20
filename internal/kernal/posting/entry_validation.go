package posting

import (
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidEntry                   = errors.New("invalid journal entry")
	ErrUnbalancedEntry                = errors.New("unbalanced journal entry: debits do not equal credits")
	ErrMinimumLines                   = errors.New("journal entry must contain at least two lines")
	ErrLineMustHaveOneSide            = errors.New("journal entry line cannot have debit and credit amounts")
	ErrZeroAmountLine                 = apiTypes.ErrZeroAmountLine
	ErrNegativeAmountProhibited       = apiTypes.ErrNegativeAmountProhibited
	ErrMissingJournalID               = errors.New("journal id is required")
	ErrZeroEntryDate                  = errors.New("entry date cannot be zero")
	ErrCompanyMismatch                = errors.New("company id mismatch between header and lines")
	ErrMissingTransactionCurrency     = errors.New("transaction currency is required")
	ErrMissingFunctionalCurrency      = errors.New("functional currency is required")
	ErrInvalidExchangeRate            = errors.New("exchange rate must be strictly positive")
	ErrFunctionalBalanceMismatch      = errors.New("functional debit and credit totals must balance")
	ErrInvalidAdjustingEntryMetadata  = errors.New("invalid adjusting entry metadata")
	ErrAdjustingEntryMetadataRequired = errors.New("adjusting entry metadata is required")
	ErrEntryAlreadyReversed           = errors.New("journal entry already reversed")
	ErrEntryNotPosted                 = errors.New("journal entry is not posted")
	ErrEntryNotDraft                  = errors.New("journal entry is not a draft")
	ErrCannotModifyPostedMove         = apiTypes.ErrCannotModifyPostedMove
	ErrReversalDateBeforeOriginal     = errors.New("reversal effective date is before the original entry effective date")
	ErrReversalActorRequired          = errors.New("reversal actor is required")
	ErrInvalidReversal                = errors.New("invalid reversal entry")
	ErrInvalidDimensionReference      = errors.New("dimension reference cannot be empty/nil")
	ErrRoundingAccountRequired        = errors.New("rounding difference account is required to settle sub-minor-unit amounts")
)

// ValidateEntryLine validates a single entry line.
func ValidateEntryLine(line journal.EntryLine, round func(apiTypes.Money) (apiTypes.Money, error)) error {
	if line.AccountID == (apiTypes.AccountID{}) {
		return ErrInvalidEntry
	}

	if line.TransactionAmount.IsNegative() || line.FunctionalDebit.IsNegative() || line.FunctionalCredit.IsNegative() {
		return ErrNegativeAmountProhibited
	}

	if line.TransactionCurrency == "" {
		return ErrMissingTransactionCurrency
	}
	if line.TransactionAmount.Amount().IsZero() {
		return ErrZeroAmountLine
	}

	fDebitCur := line.FunctionalDebit.Currency()
	fCreditCur := line.FunctionalCredit.Currency()
	if fDebitCur == "" || fCreditCur == "" {
		return ErrMissingFunctionalCurrency
	}
	if fDebitCur != fCreditCur {
		return apiTypes.ErrCurrencyMismatch
	}

	debitZero := line.FunctionalDebit.Amount().IsZero()
	creditZero := line.FunctionalCredit.Amount().IsZero()
	if debitZero == creditZero {
		if debitZero {
			return ErrZeroAmountLine
		}
		return ErrLineMustHaveOneSide
	}

	if !line.ExchangeRate.IsPositive() {
		return ErrInvalidExchangeRate
	}

	// Mathematical consistency: TransactionAmount * ExchangeRate == FunctionalAmount
	expectedAmt := line.TransactionAmount.Amount().Mul(line.ExchangeRate)
	expectedMoney, err := apiTypes.NewMoney(expectedAmt, fDebitCur)
	if err != nil {
		return err
	}
	if round != nil {
		expectedMoney, err = round(expectedMoney)
	} else {
		expectedMoney, err = expectedMoney.Rounded()
	}
	if err != nil {
		return err
	}

	actualMoney := line.FunctionalDebit
	if !creditZero {
		actualMoney = line.FunctionalCredit
	}
	if round != nil {
		actualMoney, err = round(actualMoney)
	} else {
		actualMoney, err = actualMoney.Rounded()
	}
	if err != nil {
		return err
	}

	if !expectedMoney.Equal(actualMoney) {
		return apiTypes.ErrInvalidAmount
	}

	return nil
}

// ValidateEntryDoubleEntry performs strict double-entry mathematical and invariant validation.
func ValidateEntryDoubleEntry(entry *journal.JournalEntry, round func(apiTypes.Money) (apiTypes.Money, error)) error {
	if entry == nil {
		return ErrInvalidEntry
	}
	if len(entry.Lines) < 2 {
		return ErrMinimumLines
	}
	if entry.JournalID == (apiTypes.JournalID{}) {
		return ErrMissingJournalID
	}
	if entry.EffectiveDate.IsZero() && entry.PostingDate.IsZero() {
		return ErrZeroEntryDate
	}

	if entry.Classification != "" {
		switch entry.Classification {
		case journal.EntryClassRegular, journal.EntryClassAdjusting, journal.EntryClassClosing, journal.EntryClassOpening, journal.EntryClassReversing:
		default:
			return ErrInvalidEntry
		}
	}

	if entry.AdjustingEntryType != "" {
		switch entry.AdjustingEntryType {
		case journal.AdjustingEntryAccrual, journal.AdjustingEntryDeferral, journal.AdjustingEntryProvisionEstimate, journal.AdjustingEntryReclassification:
			if entry.Classification != journal.EntryClassAdjusting {
				return ErrInvalidAdjustingEntryMetadata
			}
		default:
			return ErrInvalidAdjustingEntryMetadata
		}
	}
	if entry.Classification != journal.EntryClassAdjusting &&
		(entry.AdjustmentReason != "" || entry.AdjustmentApprovalReference != "") {
		return ErrInvalidAdjustingEntryMetadata
	}

	totals := make(map[string]decimal.Decimal)
	hasDebit, hasCredit := false, false

	for _, line := range entry.Lines {
		if err := ValidateEntryLine(line, round); err != nil {
			return err
		}

		if line.FunctionalDebit.Amount().IsPositive() {
			hasDebit = true
		}
		if line.FunctionalCredit.Amount().IsPositive() {
			hasCredit = true
		}

		currency := line.FunctionalDebit.Currency()
		totals[currency] = totals[currency].Add(
			line.FunctionalDebit.Amount().Sub(line.FunctionalCredit.Amount()),
		)
	}

	if !hasDebit || !hasCredit {
		return ErrMinimumLines
	}
	if len(totals) != 1 {
		return apiTypes.ErrCurrencyMismatch
	}

	for _, net := range totals {
		if !net.IsZero() {
			// Mathematical pre-check failure
			return fmt.Errorf("%w: net functional difference is %s (expected 0)", ErrUnbalancedEntry, net.String())
		}
	}

	return nil
}

// EnsureImmutable verifies that posted or reversed entries cannot be modified.
func EnsureImmutable(status journal.EntryStatus) error {
	if status == journal.EntryPosted || status == journal.EntryReversed {
		return ErrCannotModifyPostedMove
	}
	return nil
}

// Helper to create a new JournalEntry with defaults
func NewJournalEntry(
	companyID apiTypes.CompanyID,
	journalID apiTypes.JournalID,
	effectiveDate time.Time,
	createdBy string,
	lines []journal.EntryLine,
) (journal.JournalEntry, error) {
	if journalID == (apiTypes.JournalID{}) {
		return journal.JournalEntry{}, ErrMissingJournalID
	}
	if effectiveDate.IsZero() {
		return journal.JournalEntry{}, ErrZeroEntryDate
	}
	entry := journal.JournalEntry{
		ID:             apiTypes.NewJournalEntryID(),
		CompanyID:      companyID,
		JournalID:      journalID,
		EffectiveDate:  effectiveDate.UTC(),
		PostingDate:    effectiveDate.UTC(),
		Status:         journal.EntryDraft,
		Classification: journal.EntryClassRegular,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		Lines:          lines,
	}

	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return journal.JournalEntry{}, err
	}
	return entry, nil
}
