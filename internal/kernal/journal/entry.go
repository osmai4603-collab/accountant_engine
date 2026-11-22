package journal

import (
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

type EntryStatus string

const (
	EntryDraft    EntryStatus = "draft"
	EntryPosted   EntryStatus = "posted"
	EntryReversed EntryStatus = "reversed"
)

// IsValid reports whether the status is one of the kernel-defined lifecycle states.
func (s EntryStatus) IsValid() bool {
	switch s {
	case EntryDraft, EntryPosted, EntryReversed:
		return true
	default:
		return false
	}
}

// IsMutable reports whether an entry in this state may be edited by domain logic.
// Posted and reversed entries are immutable accounting facts.
func (s EntryStatus) IsMutable() bool {
	return s == EntryDraft
}

// IsTerminal reports whether no further lifecycle transitions are allowed.
func (s EntryStatus) IsTerminal() bool {
	return s == EntryReversed
}

// CanTransitionTo reports whether a lifecycle transition is allowed.
// The kernel lifecycle is: draft -> posted -> reversed.
func (s EntryStatus) CanTransitionTo(target EntryStatus) bool {
	switch s {
	case EntryDraft:
		return target == EntryPosted
	case EntryPosted:
		return target == EntryReversed
	case EntryReversed:
		return false
	default:
		return false
	}
}

var (
	// ErrInvalidEntryStatus indicates an unknown or empty lifecycle status.
	ErrInvalidEntryStatus = errors.New("invalid journal entry status")
	// ErrInvalidStatusTransition indicates a prohibited lifecycle transition.
	ErrInvalidStatusTransition = errors.New("invalid journal entry status transition")
	// ErrImmutableEntry indicates an attempt to mutate a posted or reversed entry.
	ErrImmutableEntry = errors.New("journal entry is immutable")
	// ErrInvalidReversalLinkage indicates inconsistent reversal/original linkage.
	ErrInvalidReversalLinkage = errors.New("invalid reversal linkage")
)

// AllowedTransitions returns the lifecycle states reachable from s in one step.
func AllowedTransitions(from EntryStatus) []EntryStatus {
	switch from {
	case EntryDraft:
		return []EntryStatus{EntryPosted}
	case EntryPosted:
		return []EntryStatus{EntryReversed}
	case EntryReversed, "":
		return nil
	default:
		return nil
	}
}

// ValidateTransition reports whether moving from one lifecycle status to another
// is allowed by the kernel state machine.
func ValidateTransition(from, to EntryStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("%w: from=%q", ErrInvalidEntryStatus, from)
	}
	if !to.IsValid() {
		return fmt.Errorf("%w: to=%q", ErrInvalidEntryStatus, to)
	}
	if from == to {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStatusTransition, from, to)
	}
	if !from.CanTransitionTo(to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStatusTransition, from, to)
	}
	return nil
}

type EntryClassification string

const (
	EntryClassRegular   EntryClassification = "regular"
	EntryClassAdjusting EntryClassification = "adjusting"
	EntryClassClosing   EntryClassification = "closing"
	EntryClassOpening   EntryClassification = "opening"
	EntryClassReversing EntryClassification = "reversing"
)

type AdjustingEntryType string

const (
	AdjustingEntryAccrual           AdjustingEntryType = "accrual"
	AdjustingEntryDeferral          AdjustingEntryType = "deferral"
	AdjustingEntryProvisionEstimate AdjustingEntryType = "provision_estimate"
	AdjustingEntryReclassification  AdjustingEntryType = "reclassification"
	AdjustingEntryAudit             AdjustingEntryType = "audit_adjustment"
	AdjustingEntryAuditLegacy       AdjustingEntryType = "audit"
)

type EntryLine struct {
	ID                  types.JournalEntryLineID
	AccountID           types.AccountID
	PartnerID           *types.PartnerID
	CostCenterID        *types.CostCenterID
	ProjectID           *types.ProjectID
	SegmentID           *types.SegmentID
	BusinessUnitID      *types.BusinessUnitID
	MatchingNumber      string
	ReconciledAmount    decimal.Decimal
	TransactionAmount   types.Money
	TransactionCurrency string
	ExchangeRate        decimal.Decimal
	FunctionalDebit     types.Money
	FunctionalCredit    types.Money
}

type JournalEntry struct {
	ID                  types.JournalEntryID
	CompanyID           types.CompanyID
	JournalID           types.JournalID
	FiscalPeriodID      types.FiscalPeriodID
	EntryNumber         string
	SequenceNumber      int64
	LegalNumber         string
	Status              EntryStatus
	Classification      EntryClassification
	AdjustingEntryType  AdjustingEntryType
	AdjustmentReason    string
	TargetPeriodNumber  int
	EffectiveDate       time.Time
	PostingDate         time.Time
	Description         string
	SourceModule        string
	ReferenceID         string
	TransactionCurrency string
	ExchangeRate        decimal.Decimal
	// PolicyFingerprint is the deterministic digest of the policy versions
	// resolved at posting time (ADR P5-01). It is audit attribution, not an
	// authorization decision, and is required to stay stable after posting.
	PolicyFingerprint string
	Lines             []EntryLine
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	PostedBy          string
	PostedAt          *time.Time
	ReversalOfID      *types.JournalEntryID
	ReversedByID      *types.JournalEntryID
	ReversalReason    string
}

// Clone returns an independent copy of the entry, including pointer-valued metadata.
func (entry JournalEntry) Clone() JournalEntry {
	clone := entry
	clone.Lines = make([]EntryLine, len(entry.Lines))
	for i, line := range entry.Lines {
		clone.Lines[i] = line
		clone.Lines[i].PartnerID = clonePointer(line.PartnerID)
		clone.Lines[i].CostCenterID = clonePointer(line.CostCenterID)
		clone.Lines[i].ProjectID = clonePointer(line.ProjectID)
		clone.Lines[i].SegmentID = clonePointer(line.SegmentID)
		clone.Lines[i].BusinessUnitID = clonePointer(line.BusinessUnitID)
	}
	clone.PostedAt = clonePointer(entry.PostedAt)
	clone.ReversalOfID = clonePointer(entry.ReversalOfID)
	clone.ReversedByID = clonePointer(entry.ReversedByID)
	return clone
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// IsMutable reports whether the entry can be edited in place by domain logic.
func (entry JournalEntry) IsMutable() bool {
	return entry.Status.IsMutable()
}

// EnsureMutable rejects mutations to posted or reversed entries.
func (entry JournalEntry) EnsureMutable() error {
	if !entry.Status.IsValid() {
		return fmt.Errorf("%w: %q", ErrInvalidEntryStatus, entry.Status)
	}
	if !entry.Status.IsMutable() {
		return fmt.Errorf("%w: status=%s", ErrImmutableEntry, entry.Status)
	}
	return nil
}

// ValidateImmutability rejects any difference between a previously posted/reversed
// entry and its current representation. Posted accounting facts are immutable;
// corrections must be represented by linked reversal/correction entries.
func ValidateImmutability(before, after JournalEntry) error {
	if before.Status.IsMutable() {
		return nil
	}
	if !entriesEqual(before, after) {
		return fmt.Errorf("%w: posted entry %s cannot be modified", ErrImmutableEntry, before.ID)
	}
	return nil
}

func entriesEqual(a, b JournalEntry) bool {
	if a.ID != b.ID ||
		a.CompanyID != b.CompanyID ||
		a.JournalID != b.JournalID ||
		a.FiscalPeriodID != b.FiscalPeriodID ||
		a.EntryNumber != b.EntryNumber ||
		a.SequenceNumber != b.SequenceNumber ||
		a.LegalNumber != b.LegalNumber ||
		a.Status != b.Status ||
		a.Classification != b.Classification ||
		a.AdjustingEntryType != b.AdjustingEntryType ||
		a.AdjustmentReason != b.AdjustmentReason ||
		a.TargetPeriodNumber != b.TargetPeriodNumber ||
		!a.EffectiveDate.Equal(b.EffectiveDate) ||
		!a.PostingDate.Equal(b.PostingDate) ||
		a.Description != b.Description ||
		a.SourceModule != b.SourceModule ||
		a.ReferenceID != b.ReferenceID ||
		a.TransactionCurrency != b.TransactionCurrency ||
		!a.ExchangeRate.Equal(b.ExchangeRate) ||
		a.PolicyFingerprint != b.PolicyFingerprint ||
		a.CreatedBy != b.CreatedBy ||
		!a.CreatedAt.Equal(b.CreatedAt) ||
		!a.UpdatedAt.Equal(b.UpdatedAt) ||
		a.PostedBy != b.PostedBy ||
		a.ReversalReason != b.ReversalReason ||
		!timePtrEqual(a.PostedAt, b.PostedAt) ||
		!journalEntryIDPtrEqual(a.ReversalOfID, b.ReversalOfID) ||
		!journalEntryIDPtrEqual(a.ReversedByID, b.ReversedByID) {
		return false
	}
	if len(a.Lines) != len(b.Lines) {
		return false
	}
	for i := range a.Lines {
		if !entryLinesEqual(a.Lines[i], b.Lines[i]) {
			return false
		}
	}
	return true
}

func entryLinesEqual(a, b EntryLine) bool {
	if a.ID != b.ID ||
		a.AccountID != b.AccountID ||
		a.MatchingNumber != b.MatchingNumber ||
		!a.ReconciledAmount.Equal(b.ReconciledAmount) ||
		!a.TransactionAmount.Equal(b.TransactionAmount) ||
		a.TransactionCurrency != b.TransactionCurrency ||
		!a.ExchangeRate.Equal(b.ExchangeRate) ||
		!a.FunctionalDebit.Equal(b.FunctionalDebit) ||
		!a.FunctionalCredit.Equal(b.FunctionalCredit) ||
		!partnerIDPtrEqual(a.PartnerID, b.PartnerID) ||
		!costCenterIDPtrEqual(a.CostCenterID, b.CostCenterID) ||
		!projectIDPtrEqual(a.ProjectID, b.ProjectID) ||
		!segmentIDPtrEqual(a.SegmentID, b.SegmentID) ||
		!businessUnitIDPtrEqual(a.BusinessUnitID, b.BusinessUnitID) {
		return false
	}
	return true
}

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func journalEntryIDPtrEqual(a, b *types.JournalEntryID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func partnerIDPtrEqual(a, b *types.PartnerID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func costCenterIDPtrEqual(a, b *types.CostCenterID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func projectIDPtrEqual(a, b *types.ProjectID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func segmentIDPtrEqual(a, b *types.SegmentID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func businessUnitIDPtrEqual(a, b *types.BusinessUnitID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ValidateReversalLinkage validates the identity and linkage between an original
// entry and its reversal. It enforces one-reversal identity rules at domain level;
// storage must still enforce atomicity and uniqueness.
func ValidateReversalLinkage(original, reversal JournalEntry) error {
	if original.ID == (types.JournalEntryID{}) || reversal.ID == (types.JournalEntryID{}) {
		return fmt.Errorf("%w: original and reversal ids are required", ErrInvalidReversalLinkage)
	}
	if original.ID == reversal.ID {
		return fmt.Errorf("%w: reversal cannot reference itself", ErrInvalidReversalLinkage)
	}
	if original.CompanyID == (types.CompanyID{}) || reversal.CompanyID == (types.CompanyID{}) {
		return fmt.Errorf("%w: company id is required", ErrInvalidReversalLinkage)
	}
	if original.CompanyID != reversal.CompanyID {
		return fmt.Errorf("%w: company mismatch", ErrInvalidReversalLinkage)
	}
	if !original.Status.IsValid() || !reversal.Status.IsValid() {
		return fmt.Errorf("%w: invalid status", ErrInvalidReversalLinkage)
	}
	if original.Status == EntryDraft {
		return fmt.Errorf("%w: original must be posted before reversal", ErrInvalidReversalLinkage)
	}
	if reversal.ReversalOfID == nil || *reversal.ReversalOfID != original.ID {
		return fmt.Errorf("%w: reversal must reference original entry", ErrInvalidReversalLinkage)
	}
	if reversal.Classification != EntryClassReversing {
		return fmt.Errorf("%w: reversal classification must be reversing", ErrInvalidReversalLinkage)
	}
	if original.Status == EntryReversed {
		if original.ReversedByID == nil || *original.ReversedByID != reversal.ID {
			return fmt.Errorf("%w: reversed original must reference reversal entry", ErrInvalidReversalLinkage)
		}
	}
	if original.ReversedByID != nil && *original.ReversedByID != reversal.ID {
		return fmt.Errorf("%w: original already linked to a different reversal", ErrInvalidReversalLinkage)
	}
	if reversal.ReversedByID != nil {
		return fmt.Errorf("%w: reversal entry cannot itself be reversed by id", ErrInvalidReversalLinkage)
	}
	return nil
}
