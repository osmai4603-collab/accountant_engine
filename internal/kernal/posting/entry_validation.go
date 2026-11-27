package posting

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
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
	ErrMissingEntryID                 = errors.New("journal entry id is required")
	ErrMissingLineID                  = errors.New("journal entry line id is required")
	ErrDuplicateLineID                = errors.New("journal entry line id is duplicated")
	ErrZeroEntryDate                  = errors.New("entry date cannot be zero")
	ErrCompanyMismatch                = errors.New("company id mismatch between header and lines")
	ErrMissingTransactionCurrency     = errors.New("transaction currency is required")
	ErrMissingFunctionalCurrency      = errors.New("functional currency is required")
	ErrInvalidExchangeRate            = errors.New("exchange rate must be strictly positive")
	ErrFunctionalBalanceMismatch      = errors.New("functional debit and credit totals must balance")
	ErrInvalidAdjustingEntryMetadata  = errors.New("invalid adjusting entry metadata")
	ErrAdjustingEntryMetadataRequired = errors.New("adjusting entry metadata is required")
	ErrInvalidAdjustingEntryPeriod    = errors.New("adjusting entry type does not match target period")
	ErrEntryAlreadyReversed           = errors.New("journal entry already reversed")
	ErrReversalExecutorRequired       = errors.New("atomic reversal executor is required")
	ErrEntryNotPosted                 = errors.New("journal entry is not posted")
	ErrEntryNotDraft                  = errors.New("journal entry is not a draft")
	ErrCannotModifyPostedMove         = apiTypes.ErrCannotModifyPostedMove
	ErrReversalDateBeforeOriginal     = errors.New("reversal effective date is before the original entry effective date")
	ErrReversalActorRequired          = errors.New("reversal actor is required")
	ErrReversalReasonRequired         = errors.New("reversal reason is required")
	ErrInvalidReversal                = errors.New("invalid reversal entry")
	ErrInvalidDimensionReference      = errors.New("dimension reference cannot be empty/nil")
	ErrRoundingAccountRequired        = errors.New("rounding difference account is required to settle sub-minor-unit amounts")
	ErrFunctionalCurrencyMismatch     = apiTypes.ErrCurrencyMismatch
	ErrTransactionCurrencyMismatch    = apiTypes.ErrCurrencyMismatch
	ErrPosterIDRequired               = errors.New("poster actor id is required")
	ErrInvalidEntryState              = errors.New("invalid journal entry state")
	ErrSourceTraceabilityRequired     = errors.New("source module or authorized alternative classification is required")
	ErrSourceReferenceRequired        = errors.New("source reference id is required for external source module")
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
	if line.TransactionAmount.Currency() != line.TransactionCurrency {
		return ErrTransactionCurrencyMismatch
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
	if err := apiTypes.ValidateExchangeRatePrecision(line.ExchangeRate); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidExchangeRate, err)
	}
	if line.TransactionCurrency == fDebitCur && !line.ExchangeRate.Equal(decimal.NewFromInt(1)) {
		return ErrInvalidExchangeRate
	}

	// Enforce PST-004 before normalization: caller functional amounts must
	// share the sign of TransactionAmount * ExchangeRate and stay within 0.5
	// functional minor unit of that unrounded product.
	rawExpected := line.TransactionAmount.Amount().Mul(line.ExchangeRate)
	actualMoney := line.FunctionalDebit
	if !creditZero {
		actualMoney = line.FunctionalCredit
	}
	if actualMoney.Amount().Sign() != rawExpected.Sign() {
		return apiTypes.ErrInvalidAmount
	}
	funcCurrency, ok := apiTypes.LookupCurrency(fDebitCur)
	if !ok || funcCurrency.IsSpecial() {
		return apiTypes.ErrInvalidCurrency
	}
	halfMinorUnit := decimal.New(5, -funcCurrency.MinorUnit-1)
	if actualMoney.Amount().Sub(rawExpected).Abs().GreaterThan(halfMinorUnit) {
		return apiTypes.ErrInvalidAmount
	}

	// Compare at the functional currency's posting precision. Transaction
	// amounts can carry greater precision than the amount stored in the ledger.
	expectedMoney, err := apiTypes.NewMoney(rawExpected, fDebitCur)
	if err != nil {
		return err
	}
	expectedMoney, err = roundMoney(expectedMoney, round)
	if err != nil {
		return err
	}
	actualMoney, err = roundMoney(actualMoney, round)
	if err != nil {
		return err
	}
	if !expectedMoney.Amount().Equal(actualMoney.Amount()) {
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
	if err := ValidateEntryIdentity(*entry); err != nil {
		return err
	}
	if entry.JournalID == (apiTypes.JournalID{}) {
		return ErrMissingJournalID
	}
	if entry.EffectiveDate.IsZero() && entry.PostingDate.IsZero() {
		return ErrZeroEntryDate
	}

	if err := ValidateEntryClassificationMetadata(*entry); err != nil {
		return err
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

// ValidateEntryClassificationMetadata enforces the metadata invariants for
// adjusting and reversing entries before any posting-side effects are attempted.
func ValidateEntryClassificationMetadata(entry journal.JournalEntry) error {
	switch entry.Classification {
	case journal.EntryClassRegular, journal.EntryClassAdjusting, journal.EntryClassClosing, journal.EntryClassOpening, journal.EntryClassReversing:
	default:
		return ErrInvalidEntry
	}

	if entry.Classification == journal.EntryClassAdjusting {
		if entry.AdjustingEntryType == "" || strings.TrimSpace(entry.AdjustmentReason) == "" {
			return ErrAdjustingEntryMetadataRequired
		}
		switch entry.AdjustingEntryType {
		case journal.AdjustingEntryAccrual,
			journal.AdjustingEntryDeferral,
			journal.AdjustingEntryProvisionEstimate,
			journal.AdjustingEntryReclassification,
			journal.AdjustingEntryAudit,
			journal.AdjustingEntryAuditLegacy:
		default:
			return ErrInvalidAdjustingEntryMetadata
		}
		isAudit := entry.AdjustingEntryType == journal.AdjustingEntryAudit || entry.AdjustingEntryType == journal.AdjustingEntryAuditLegacy
		if isAudit && entry.TargetPeriodNumber != 14 {
			return ErrInvalidAdjustingEntryPeriod
		}
		if entry.TargetPeriodNumber == 14 && !isAudit {
			return ErrInvalidAdjustingEntryPeriod
		}
	} else if entry.AdjustingEntryType != "" || entry.AdjustmentReason != "" {
		return ErrInvalidAdjustingEntryMetadata
	}

	if err := validateTargetPeriodClassification(entry); err != nil {
		return err
	}
	// A draft cannot already carry a reversal: only a posted entry may be
	// reversed, and posted entries never re-enter the pipeline. Accepting a
	// pre-linked draft would let a caller describe an impossible lifecycle.
	if entry.ReversedByID != nil {
		return ErrInvalidReversal
	}
	if (entry.Classification == journal.EntryClassReversing) != (entry.ReversalOfID != nil) {
		return ErrInvalidReversal
	}
	if entry.ReversalOfID != nil &&
		(*entry.ReversalOfID == (apiTypes.JournalEntryID{}) || *entry.ReversalOfID == entry.ID) {
		return ErrInvalidReversal
	}
	if entry.Classification == journal.EntryClassReversing {
		if strings.TrimSpace(entry.ReversalReason) == "" {
			return ErrReversalReasonRequired
		}
	} else if strings.TrimSpace(entry.ReversalReason) != "" {
		return ErrInvalidReversal
	}
	return nil
}

func validateTargetPeriodClassification(entry journal.JournalEntry) error {
	switch entry.TargetPeriodNumber {
	case 0:
		if entry.Classification == journal.EntryClassOpening {
			return masterdata.ErrInvalidAdjustmentPeriod
		}
	case 1:
		if entry.Classification != journal.EntryClassOpening {
			return masterdata.ErrInvalidAdjustmentPeriod
		}
	case 13, 14:
		if entry.Classification != journal.EntryClassAdjusting &&
			!(entry.TargetPeriodNumber == 13 && (entry.Classification == journal.EntryClassClosing || (entry.Classification == journal.EntryClassReversing && entry.ReversalOfID != nil))) {
			return masterdata.ErrInvalidAdjustmentPeriod
		}
	default:
		return masterdata.ErrInvalidAdjustmentPeriod
	}
	return nil
}

// ValidateSourceTraceability enforces ACC-011: entries originating from an
// external module must carry a non-empty ReferenceID; entries without an
// external source must declare an authorized manual/adjusting/closing/opening
// classification with a non-empty reason or description.
func ValidateSourceTraceability(entry journal.JournalEntry) error {
	source := strings.TrimSpace(entry.SourceModule)
	ref := strings.TrimSpace(entry.ReferenceID)
	if source != "" && !strings.EqualFold(source, "manual") {
		if ref == "" {
			return fmt.Errorf("%w: source_module=%s", ErrSourceReferenceRequired, source)
		}
		return nil
	}
	hasReason := strings.TrimSpace(entry.AdjustmentReason) != "" ||
		strings.TrimSpace(entry.ReversalReason) != "" ||
		strings.TrimSpace(entry.Description) != ""
	switch entry.Classification {
	case journal.EntryClassAdjusting, journal.EntryClassClosing, journal.EntryClassOpening, journal.EntryClassReversing:
		if !hasReason {
			return fmt.Errorf("%w: classification=%s requires reason or description", ErrSourceTraceabilityRequired, entry.Classification)
		}
		return nil
	case journal.EntryClassRegular:
		if strings.EqualFold(source, "manual") && hasReason {
			return nil
		}
		if ref != "" && source != "" {
			return nil
		}
		return fmt.Errorf("%w: regular entry without external source requires SourceModule='manual' and description", ErrSourceTraceabilityRequired)
	default:
		return ErrSourceTraceabilityRequired
	}
}

// ValidateEntryIdentity requires a nonzero entry ID and unique, nonzero line IDs.
func ValidateEntryIdentity(entry journal.JournalEntry) error {
	if entry.ID == (apiTypes.JournalEntryID{}) {
		return ErrMissingEntryID
	}
	seen := make(map[apiTypes.JournalEntryLineID]struct{}, len(entry.Lines))
	for _, line := range entry.Lines {
		if line.ID == (apiTypes.JournalEntryLineID{}) {
			return ErrMissingLineID
		}
		if _, exists := seen[line.ID]; exists {
			return ErrDuplicateLineID
		}
		seen[line.ID] = struct{}{}
	}
	return nil
}

// EnsureImmutable verifies that posted, reversed, or cancelled entries cannot be modified.
func EnsureImmutable(status journal.EntryStatus) error {
	if status == journal.EntryPosted || status == journal.EntryReversed {
		return ErrCannotModifyPostedMove
	}
	if status == journal.EntryCancelled {
		return fmt.Errorf("%w: cancelled entry is immutable", ErrInvalidEntryState)
	}
	return nil
}

// Helper to create a new JournalEntry with defaults using system time
func NewJournalEntry(
	companyID apiTypes.CompanyID,
	journalID apiTypes.JournalID,
	effectiveDate time.Time,
	createdBy string,
	lines []journal.EntryLine,
) (journal.JournalEntry, error) {
	return NewJournalEntryAt(companyID, journalID, effectiveDate, createdBy, lines, systemClock{}.Now())
}

// NewJournalEntryAt creates a new JournalEntry with defaults using an explicit timestamp.
func NewJournalEntryAt(
	companyID apiTypes.CompanyID,
	journalID apiTypes.JournalID,
	effectiveDate time.Time,
	createdBy string,
	lines []journal.EntryLine,
	now time.Time,
) (journal.JournalEntry, error) {
	if journalID == (apiTypes.JournalID{}) {
		return journal.JournalEntry{}, ErrMissingJournalID
	}
	if effectiveDate.IsZero() {
		return journal.JournalEntry{}, ErrZeroEntryDate
	}
	now = now.UTC()
	entry := journal.JournalEntry{
		ID:             apiTypes.NewJournalEntryID(),
		CompanyID:      companyID,
		JournalID:      journalID,
		EffectiveDate:  effectiveDate.UTC(),
		PostingDate:    effectiveDate.UTC(),
		Status:         journal.EntryDraft,
		Classification: journal.EntryClassRegular,
		CreatedBy:      createdBy,
		CreatedAt:      now,
		UpdatedAt:      now,
		Lines:          lines,
	}
	for i := range entry.Lines {
		if entry.Lines[i].ID == (apiTypes.JournalEntryLineID{}) {
			entry.Lines[i].ID = apiTypes.NewJournalEntryLineID()
		}
	}

	if err := ValidateEntryDoubleEntry(&entry, nil); err != nil {
		return journal.JournalEntry{}, err
	}
	return entry, nil
}
