// Package postingcompat provides the journal-entry contracts consumed by the
// optional orbital components (subledgers, reporting, reconciliation and
// consolidation).
//
// The P2 kernel intentionally exposes a slimmer journal type (see
// internal/kernal/journal) and a pipeline-oriented posting API. The orbital
// modules were designed against the richer entry shape used by the standalone
// engine. This package bridges that gap without touching the locked kernel:
// it keeps the orbital-facing data shape and delegates mathematical validation
// to the kernel's posting rules.
package posting

import (
	"errors"
	"time"

	"github.com/google/uuid"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidEntry                   = errors.New("invalid journal entry")
	ErrMinimumLines                   = errors.New("journal entry must contain at least two lines")
	ErrLineMustHaveOneSide            = errors.New("journal entry line cannot have debit and credit amounts")
	ErrEntryNotDraft                  = errors.New("journal entry is not a draft")
	ErrEntryNotPosted                 = errors.New("journal entry is not posted")
	ErrEntryNotFound                  = errors.New("journal entry not found")
	ErrEntryAlreadyReversed           = errors.New("journal entry already reversed")
	ErrInvalidReversal                = errors.New("invalid reversal entry")
	ErrReversalActorRequired          = errors.New("reversal actor is required")
	ErrMissingTransactionCurrency     = errors.New("transaction currency is required")
	ErrMissingFunctionalCurrency      = errors.New("functional currency is required")
	ErrInvalidExchangeRate            = errors.New("exchange rate must be strictly positive")
	ErrFunctionalBalanceMismatch      = errors.New("functional debit and credit totals must balance")
	ErrAdjustingEntryMetadataRequired = errors.New("adjusting entry metadata is required")
	ErrInvalidAdjustingEntryMetadata  = errors.New("invalid adjusting entry metadata")
	ErrUnsupportedMutationBackend     = errors.New("unsupported mutation backend")
	ErrMissingJournalID               = errors.New("journal id is required")
	ErrZeroEntryDate                  = errors.New("entry date is required")
	ErrCompanyMismatch                = errors.New("journal entry line company mismatch")
)

type EntryStatus string

type EntryClassification string

type AdjustingEntryType string

const (
	EntryDraft    EntryStatus = "draft"
	EntryPosted   EntryStatus = "posted"
	EntryReversed EntryStatus = "reversed"
)

const (
	EntryClassRegular   EntryClassification = "regular"
	EntryClassAdjusting EntryClassification = "adjusting"
	EntryClassClosing   EntryClassification = "closing"
	EntryClassOpening   EntryClassification = "opening"
	EntryClassReversing EntryClassification = "reversing"
)

const (
	AdjustingEntryAccrual           AdjustingEntryType = "accrual"
	AdjustingEntryDeferral          AdjustingEntryType = "deferral"
	AdjustingEntryProvisionEstimate AdjustingEntryType = "provision_estimate"
	AdjustingEntryReclassification  AdjustingEntryType = "reclassification"
)

const (
	StatusDraft    = EntryDraft
	StatusPosted   = EntryPosted
	StatusReversed = EntryReversed
)

// AccountPeriodBalance is the materialized balance shape consumed by the
// migrated reporting package.
type AccountPeriodBalance struct {
	AccountID       uuid.UUID
	BeginningDebit  decimal.Decimal
	BeginningCredit decimal.Decimal
	PeriodDebit     decimal.Decimal
	PeriodCredit    decimal.Decimal
	EndingDebit     decimal.Decimal
	EndingCredit    decimal.Decimal
}

// JournalEntryLine is the orbital-facing journal line. Debit and Credit are
// kept for source parity; new code should prefer TransactionAmount and the
// functional pair.
type JournalEntryLine struct {
	CompanyID           uuid.UUID
	ID                  uuid.UUID
	AccountID           uuid.UUID
	PartnerID           *uuid.UUID
	CostCenterID        *uuid.UUID
	ProjectID           *uuid.UUID
	SegmentID           *uuid.UUID
	BusinessUnitID      *uuid.UUID
	MatchingNumber      string
	ReconciledAmount    decimal.Decimal
	Debit               financialtypes.Money
	Credit              financialtypes.Money
	TransactionAmount   financialtypes.Money
	TransactionCurrency string
	ExchangeRate        decimal.Decimal
	FunctionalDebit     financialtypes.Money
	FunctionalCredit    financialtypes.Money
	AmountCurrency      financialtypes.Money
	Currency            string
	ForeignAmount       decimal.Decimal
}

// Validate enforces the double-entry invariant on a single line.
func (l JournalEntryLine) Validate() error {
	if l.AccountID == uuid.Nil {
		return ErrInvalidEntry
	}
	if l.Debit.Currency() != "" && l.Credit.Currency() != "" &&
		!l.Debit.Amount().IsZero() && !l.Credit.Amount().IsZero() {
		return ErrLineMustHaveOneSide
	}
	if l.TransactionCurrency == "" && l.AmountCurrency.Currency() == "" &&
		l.Debit.Currency() == "" && l.Credit.Currency() == "" {
		return ErrMissingTransactionCurrency
	}
	if l.ExchangeRate.IsNegative() {
		return ErrInvalidExchangeRate
	}
	return nil
}

// NormalizeLineCurrencies fills the transaction-currency fields from the
// deprecated amount fields when callers have not populated them.
func NormalizeLineCurrencies(line JournalEntryLine) (JournalEntryLine, error) {
	if line.TransactionCurrency == "" {
		if line.AmountCurrency.Currency() != "" {
			line.TransactionCurrency = line.AmountCurrency.Currency()
			line.TransactionAmount = line.AmountCurrency
			if line.ExchangeRate.IsZero() {
				line.ExchangeRate = decimal.NewFromInt(1)
			}
		} else if line.Debit.Currency() != "" {
			line.TransactionCurrency = line.Debit.Currency()
			line.ExchangeRate = decimal.NewFromInt(1)
		} else if line.Credit.Currency() != "" {
			line.TransactionCurrency = line.Credit.Currency()
			line.ExchangeRate = decimal.NewFromInt(1)
		}
	}
	if line.TransactionAmount.Currency() == "" && line.AmountCurrency.Currency() != "" {
		line.TransactionAmount = line.AmountCurrency
	}
	if line.FunctionalDebit.Currency() == "" && line.Debit.Currency() != "" {
		line.FunctionalDebit = line.Debit
	}
	if line.FunctionalCredit.Currency() == "" && line.Credit.Currency() != "" {
		line.FunctionalCredit = line.Credit
	}
	return line, nil
}

// JournalEntry is the orbital-facing journal header.
type JournalEntry struct {
	CompanyID                   uuid.UUID
	ID                          uuid.UUID
	SequenceNumber              int64
	LegalNumber                 string
	JournalID                   uuid.UUID
	PeriodID                    uuid.UUID
	EntryDate                   time.Time
	PostingDate                 time.Time
	Date                        time.Time
	Status                      EntryStatus
	SourceModule                string
	DocumentReference           string
	Reference                   string
	TransactionCurrency         string
	ExchangeRate                decimal.Decimal
	ApprovalFingerprint         string
	CreatedBy                   string
	ApprovedBy                  string
	ApprovedAt                  *time.Time
	PostedBy                    string
	PostedAt                    *time.Time
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
	ReversedEntryID             *uuid.UUID
	OriginalEntryID             *uuid.UUID
	SourceEventID               *uuid.UUID
	TargetPeriodNumber          int
	AdjustmentAuthorized        bool
	AdjustingEntryType          AdjustingEntryType
	AdjustmentReason            string
	AdjustmentApprovalReference string
	Classification              EntryClassification
	ReversalReason              string
	ReversalReference           string
	IdempotencyKey              string
	IdempotencyPayloadHash      string
	Lines                       []JournalEntryLine
}

// Validate checks header identity plus the double-entry balance invariant.
// The kernel remains the authority for posting-time validation; this check
// exists so orbital callers can fail fast on malformed drafts.
func (e JournalEntry) Validate() error {
	if e.ID == uuid.Nil {
		return ErrInvalidEntry
	}
	if len(e.Lines) < 2 {
		return ErrMinimumLines
	}
	if e.Classification == EntryClassAdjusting && e.AdjustingEntryType == "" && e.AdjustmentReason == "" {
		return ErrAdjustingEntryMetadataRequired
	}
	debits, credits, currency := decimal.Zero, decimal.Zero, ""
	for _, line := range e.Lines {
		if err := line.Validate(); err != nil {
			return err
		}
		if line.CompanyID != uuid.Nil && line.CompanyID != e.CompanyID {
			return ErrInvalidEntry
		}
		debit := line.FunctionalDebit.Amount()
		credit := line.FunctionalCredit.Amount()
		if debit.IsZero() && credit.IsZero() {
			debit = line.Debit.Amount()
			credit = line.Credit.Amount()
		}
		if debit.IsNegative() || credit.IsNegative() {
			return ErrInvalidEntry
		}
		if !debit.IsZero() && !credit.IsZero() {
			return ErrLineMustHaveOneSide
		}
		if debit.IsZero() && credit.IsZero() {
			return ErrInvalidEntry
		}
		lineCurrency := line.FunctionalDebit.Currency()
		if lineCurrency == "" {
			lineCurrency = line.FunctionalCredit.Currency()
		}
		if lineCurrency == "" {
			lineCurrency = line.Debit.Currency()
		}
		if lineCurrency == "" {
			lineCurrency = line.Credit.Currency()
		}
		if currency == "" {
			currency = lineCurrency
		} else if currency != lineCurrency {
			return ErrInvalidEntry
		}
		debits = debits.Add(debit)
		credits = credits.Add(credit)
	}
	if currency == "" {
		return ErrMissingFunctionalCurrency
	}
	if !debits.Equal(credits) {
		return ErrFunctionalBalanceMismatch
	}
	return nil
}

// NewJournalEntry creates a draft orbital journal entry from validated lines.
func NewJournalEntry(journalID uuid.UUID, date time.Time, createdBy string, lines []JournalEntryLine) (JournalEntry, error) {
	if journalID == uuid.Nil {
		return JournalEntry{}, ErrMissingJournalID
	}
	if date.IsZero() {
		return JournalEntry{}, ErrZeroEntryDate
	}
	var companyID uuid.UUID
	for _, line := range lines {
		if line.CompanyID == uuid.Nil {
			continue
		}
		if companyID == uuid.Nil {
			companyID = line.CompanyID
		} else if companyID != line.CompanyID {
			return JournalEntry{}, ErrCompanyMismatch
		}
	}
	entry := JournalEntry{
		ID:             uuid.New(),
		CompanyID:      companyID,
		JournalID:      journalID,
		EntryDate:      date.UTC(),
		Date:           date.UTC(),
		Status:         EntryDraft,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().UTC(),
		Classification: EntryClassRegular,
		Lines:          append([]JournalEntryLine(nil), lines...),
	}
	if err := entry.Validate(); err != nil {
		return JournalEntry{}, err
	}
	return entry, nil
}

func equalUUIDPtr(a, b *uuid.UUID) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// Equal reports whether two lines have the same economic content.
func (l JournalEntryLine) Equal(other JournalEntryLine) bool {
	return (l.CompanyID == other.CompanyID || l.CompanyID == uuid.Nil || other.CompanyID == uuid.Nil) &&
		l.ID == other.ID &&
		l.AccountID == other.AccountID &&
		equalUUIDPtr(l.PartnerID, other.PartnerID) &&
		equalUUIDPtr(l.CostCenterID, other.CostCenterID) &&
		equalUUIDPtr(l.ProjectID, other.ProjectID) &&
		equalUUIDPtr(l.SegmentID, other.SegmentID) &&
		equalUUIDPtr(l.BusinessUnitID, other.BusinessUnitID) &&
		l.MatchingNumber == other.MatchingNumber &&
		l.ReconciledAmount.Equal(other.ReconciledAmount) &&
		l.Debit.Equal(other.Debit) &&
		l.Credit.Equal(other.Credit) &&
		l.TransactionAmount.Equal(other.TransactionAmount) &&
		l.TransactionCurrency == other.TransactionCurrency &&
		l.ExchangeRate.Equal(other.ExchangeRate) &&
		l.FunctionalDebit.Equal(other.FunctionalDebit) &&
		l.FunctionalCredit.Equal(other.FunctionalCredit) &&
		l.AmountCurrency.Equal(other.AmountCurrency) &&
		l.Currency == other.Currency &&
		l.ForeignAmount.Equal(other.ForeignAmount)
}

// LineFunctionalAmounts reports the functional debit, credit and currency of
// a line, preferring the canonical fields over their deprecated aliases.
func LineFunctionalAmounts(line JournalEntryLine) (decimal.Decimal, decimal.Decimal, string) {
	debit, credit := line.FunctionalDebit.Amount(), line.FunctionalCredit.Amount()
	currency := line.FunctionalDebit.Currency()
	if currency == "" {
		currency = line.FunctionalCredit.Currency()
	}
	if debit.IsZero() && credit.IsZero() {
		debit, credit = line.Debit.Amount(), line.Credit.Amount()
		currency = line.Debit.Currency()
		if currency == "" {
			currency = line.Credit.Currency()
		}
	}
	return debit, credit, currency
}
