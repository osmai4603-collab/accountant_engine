package journal

import kernalJournal "github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"

type EntryStatus = kernalJournal.EntryStatus
type EntryClassification = kernalJournal.EntryClassification
type AdjustingEntryType = kernalJournal.AdjustingEntryType
type EntryLine = kernalJournal.EntryLine
type JournalEntry = kernalJournal.JournalEntry

const (
	EntryDraft     = kernalJournal.EntryDraft
	EntryPosted    = kernalJournal.EntryPosted
	EntryReversed  = kernalJournal.EntryReversed
	EntryCancelled = kernalJournal.EntryCancelled

	EntryClassRegular   = kernalJournal.EntryClassRegular
	EntryClassAdjusting = kernalJournal.EntryClassAdjusting
	EntryClassClosing   = kernalJournal.EntryClassClosing
	EntryClassOpening   = kernalJournal.EntryClassOpening
	EntryClassReversing = kernalJournal.EntryClassReversing

	AdjustingEntryAccrual           = kernalJournal.AdjustingEntryAccrual
	AdjustingEntryDeferral          = kernalJournal.AdjustingEntryDeferral
	AdjustingEntryProvisionEstimate = kernalJournal.AdjustingEntryProvisionEstimate
	AdjustingEntryReclassification  = kernalJournal.AdjustingEntryReclassification
	AdjustingEntryAudit             = kernalJournal.AdjustingEntryAudit
	AdjustingEntryAuditLegacy       = kernalJournal.AdjustingEntryAuditLegacy
)

var (
	ErrInvalidEntryStatus      = kernalJournal.ErrInvalidEntryStatus
	ErrInvalidStatusTransition = kernalJournal.ErrInvalidStatusTransition
	ErrImmutableEntry          = kernalJournal.ErrImmutableEntry
	ErrInvalidReversalLinkage  = kernalJournal.ErrInvalidReversalLinkage
	ErrInvalidLineMutation     = kernalJournal.ErrInvalidLineMutation
	ErrInvalidCancellation     = kernalJournal.ErrInvalidCancellation
)
