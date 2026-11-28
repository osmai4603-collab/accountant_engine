package journal

import kernelJournal "github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"

type EntryStatus = kernelJournal.EntryStatus
type EntryClassification = kernelJournal.EntryClassification
type AdjustingEntryType = kernelJournal.AdjustingEntryType
type EntryLine = kernelJournal.EntryLine
type JournalEntry = kernelJournal.JournalEntry

const (
	EntryDraft     = kernelJournal.EntryDraft
	EntryPosted    = kernelJournal.EntryPosted
	EntryReversed  = kernelJournal.EntryReversed
	EntryCancelled = kernelJournal.EntryCancelled

	EntryClassRegular   = kernelJournal.EntryClassRegular
	EntryClassAdjusting = kernelJournal.EntryClassAdjusting
	EntryClassClosing   = kernelJournal.EntryClassClosing
	EntryClassOpening   = kernelJournal.EntryClassOpening
	EntryClassReversing = kernelJournal.EntryClassReversing

	AdjustingEntryAccrual           = kernelJournal.AdjustingEntryAccrual
	AdjustingEntryDeferral          = kernelJournal.AdjustingEntryDeferral
	AdjustingEntryProvisionEstimate = kernelJournal.AdjustingEntryProvisionEstimate
	AdjustingEntryReclassification  = kernelJournal.AdjustingEntryReclassification
	AdjustingEntryAudit             = kernelJournal.AdjustingEntryAudit
	AdjustingEntryAuditLegacy       = kernelJournal.AdjustingEntryAuditLegacy
)

var (
	ErrInvalidEntryStatus      = kernelJournal.ErrInvalidEntryStatus
	ErrInvalidStatusTransition = kernelJournal.ErrInvalidStatusTransition
	ErrImmutableEntry          = kernelJournal.ErrImmutableEntry
	ErrInvalidReversalLinkage  = kernelJournal.ErrInvalidReversalLinkage
	ErrInvalidLineMutation     = kernelJournal.ErrInvalidLineMutation
	ErrInvalidCancellation     = kernelJournal.ErrInvalidCancellation
)
