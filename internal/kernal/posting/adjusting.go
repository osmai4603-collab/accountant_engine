package posting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
)

// AdjustingEntryPreCloseSummary is intended for use by a fiscal-year
// pre-close checklist after it has loaded entries for the year.
type AdjustingEntryPreCloseSummary struct {
	TotalAdjustingEntries       int
	PendingDrafts               int
	MissingAuditMetadata        int
	InvalidClassificationPeriod int
	Evidence                    []string
}

func (s AdjustingEntryPreCloseSummary) Ready() bool {
	return s.PendingDrafts == 0 && s.MissingAuditMetadata == 0 && s.InvalidClassificationPeriod == 0
}

// SummarizeAdjustingEntriesForPreClose identifies pending or unauditable
// adjusting entries in an already company/year-scoped entry set.
func SummarizeAdjustingEntriesForPreClose(entries []journal.JournalEntry) AdjustingEntryPreCloseSummary {
	summary := AdjustingEntryPreCloseSummary{Evidence: make([]string, 0)}
	for _, entry := range entries {
		if entry.Classification != journal.EntryClassAdjusting {
			continue
		}
		summary.TotalAdjustingEntries++
		reference := strings.TrimSpace(entry.LegalNumber)
		if reference == "" {
			reference = entry.EntryNumber
		}
		if reference == "" {
			reference = entry.ID.String()
		}
		if entry.Status == journal.EntryDraft {
			summary.PendingDrafts++
			summary.Evidence = append(summary.Evidence, fmt.Sprintf("adjusting entry %s is still a draft", reference))
		}

		classificationErr := ValidateEntryClassificationMetadata(entry)
		classificationMetadataInvalid := classificationErr != nil &&
			!errors.Is(classificationErr, ErrInvalidAdjustingEntryPeriod)
		if !validAdjustingEntryType(entry.AdjustingEntryType) ||
			classificationMetadataInvalid ||
			strings.TrimSpace(entry.AdjustmentReason) == "" ||
			strings.TrimSpace(entry.CreatedBy) == "" || entry.CreatedAt.IsZero() {
			summary.MissingAuditMetadata++
			summary.Evidence = append(summary.Evidence, fmt.Sprintf("adjusting entry %s lacks required accounting or audit metadata", reference))
		}
		if errors.Is(classificationErr, ErrInvalidAdjustingEntryPeriod) {
			summary.InvalidClassificationPeriod++
			summary.Evidence = append(summary.Evidence, fmt.Sprintf(
				"adjusting entry %s has type %q incompatible with target period %d",
				reference, entry.AdjustingEntryType, entry.TargetPeriodNumber,
			))
		}
	}
	return summary
}

func validAdjustingEntryType(t journal.AdjustingEntryType) bool {
	switch t {
	case journal.AdjustingEntryAccrual,
		journal.AdjustingEntryDeferral,
		journal.AdjustingEntryProvisionEstimate,
		journal.AdjustingEntryReclassification,
		journal.AdjustingEntryAudit:
		return true
	default:
		return false
	}
}
