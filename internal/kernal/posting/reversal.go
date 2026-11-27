package posting

import (
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

// BuildReversalEntry constructs an exact reversing journal entry for an already posted entry.
// Invariants enforced:
// 1. Debits and Credits are cleanly swapped on each line.
// 2. Analytical dimensions (Partner, CostCenter, Project, Segment, BusinessUnit) are preserved 1:1.
// 3. Reversal date cannot precede original entry effective date.
// 4. Combined net impact of original + reversal across all accounts is identically zero.
func BuildReversalEntry(original journal.JournalEntry, reversalDate time.Time, actorID string, reason string) (journal.JournalEntry, error) {
	// Wall time enters domain code only through the Clock boundary. Callers
	// that need deterministic metadata must use BuildReversalEntryAt.
	return BuildReversalEntryAt(original, reversalDate, actorID, reason, systemClock{}.Now())
}

// BuildReversalEntryAt constructs a reversing journal entry using an explicit timestamp.
func BuildReversalEntryAt(original journal.JournalEntry, reversalDate time.Time, actorID string, reason string, now time.Time) (journal.JournalEntry, error) {
	if original.Status != journal.EntryPosted {
		return journal.JournalEntry{}, ErrEntryNotPosted
	}
	if original.ReversedByID != nil {
		return journal.JournalEntry{}, ErrEntryAlreadyReversed
	}
	if original.Classification == journal.EntryClassReversing || original.ReversalOfID != nil {
		return journal.JournalEntry{}, ErrInvalidReversal
	}

	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return journal.JournalEntry{}, ErrReversalActorRequired
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return journal.JournalEntry{}, ErrReversalReasonRequired
	}
	original = original.Clone()

	if reversalDate.IsZero() {
		reversalDate = now.UTC()
	} else {
		reversalDate = reversalDate.UTC()
	}

	// Compare calendar dates (day-level granularity)
	origCal := original.EffectiveDate.UTC().Truncate(24 * time.Hour)
	revCal := reversalDate.Truncate(24 * time.Hour)
	if revCal.Before(origCal) {
		return journal.JournalEntry{}, ErrReversalDateBeforeOriginal
	}

	now = now.UTC()
	originalID := original.ID

	reversal := journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		CompanyID:           original.CompanyID,
		JournalID:           original.JournalID,
		EffectiveDate:       reversalDate,
		Status:              journal.EntryDraft,
		Classification:      journal.EntryClassReversing,
		Description:         "Reversal of " + original.EntryNumber,
		SourceModule:        original.SourceModule,
		TransactionCurrency: original.TransactionCurrency,
		ExchangeRate:        original.ExchangeRate,
		CreatedBy:           actorID,
		CreatedAt:           now,
		UpdatedAt:           now,
		ReversalOfID:        &originalID,
		ReversalReason:      reason,
		Lines:               make([]journal.EntryLine, len(original.Lines)),
	}

	for i, line := range original.Lines {
		reversal.Lines[i] = journal.EntryLine{
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           line.AccountID,
			PartnerID:           line.PartnerID,
			CostCenterID:        line.CostCenterID,
			ProjectID:           line.ProjectID,
			SegmentID:           line.SegmentID,
			BusinessUnitID:      line.BusinessUnitID,
			MatchingNumber:      line.MatchingNumber,
			ReconciledAmount:    decimal.Zero,
			TransactionAmount:   line.TransactionAmount,
			TransactionCurrency: line.TransactionCurrency,
			ExchangeRate:        line.ExchangeRate,
			// Swap functional debit and credit
			FunctionalDebit:  line.FunctionalCredit,
			FunctionalCredit: line.FunctionalDebit,
		}
	}

	if err := ValidateEntryDoubleEntry(&reversal, nil); err != nil {
		return journal.JournalEntry{}, err
	}

	return reversal, nil
}
