package posting

import (
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
)

// EntryValidatorAdapter adapts posting.ValidateEntryDoubleEntry for consumers
// (such as valuation) that depend only on an abstract validation interface.
type EntryValidatorAdapter struct{}

func (EntryValidatorAdapter) ValidateDoubleEntry(entry *journal.JournalEntry) error {
	return ValidateEntryDoubleEntry(entry, nil)
}

// ReversalBuilderAdapter adapts posting.BuildReversalEntryAt for consumers
// (such as valuation) that depend only on an abstract reversal-builder interface.
type ReversalBuilderAdapter struct{}

func (ReversalBuilderAdapter) BuildReversalAt(
	original journal.JournalEntry,
	date time.Time,
	actor, reason string,
	now time.Time,
) (journal.JournalEntry, error) {
	return BuildReversalEntryAt(original, date, actor, reason, now)
}
