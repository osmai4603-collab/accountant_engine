package posting

import (
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
)

func TestValidateSourceTraceability(t *testing.T) {
	t.Run("external source with reference passes", func(t *testing.T) {
		entry := journal.JournalEntry{
			Classification: journal.EntryClassRegular,
			SourceModule:   "billing",
			ReferenceID:    "INV-2026-001",
		}
		if err := ValidateSourceTraceability(entry); err != nil {
			t.Fatalf("expected valid external source entry, got %v", err)
		}
	})

	t.Run("external source without reference fails", func(t *testing.T) {
		entry := journal.JournalEntry{
			Classification: journal.EntryClassRegular,
			SourceModule:   "billing",
		}
		if err := ValidateSourceTraceability(entry); !errors.Is(err, ErrSourceReferenceRequired) {
			t.Fatalf("expected ErrSourceReferenceRequired, got %v", err)
		}
	})

	t.Run("manual regular entry with description passes", func(t *testing.T) {
		entry := journal.JournalEntry{
			Classification: journal.EntryClassRegular,
			SourceModule:   "manual",
			Description:    "Manual petty cash replenishment",
		}
		if err := ValidateSourceTraceability(entry); err != nil {
			t.Fatalf("expected valid manual entry, got %v", err)
		}
	})

	t.Run("unclassified empty source regular entry fails", func(t *testing.T) {
		entry := journal.JournalEntry{
			Classification: journal.EntryClassRegular,
		}
		if err := ValidateSourceTraceability(entry); !errors.Is(err, ErrSourceTraceabilityRequired) {
			t.Fatalf("expected ErrSourceTraceabilityRequired, got %v", err)
		}
	})

	t.Run("adjusting entry with reason passes without external reference", func(t *testing.T) {
		entry := journal.JournalEntry{
			Classification:   journal.EntryClassAdjusting,
			AdjustmentReason: "Month-end accrual",
		}
		if err := ValidateSourceTraceability(entry); err != nil {
			t.Fatalf("expected valid adjusting entry, got %v", err)
		}
	})
}
