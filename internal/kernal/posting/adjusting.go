package posting

import (
	"fmt"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
)

// AdjustingEntryPreCloseSummary is intended for use by a fiscal-year
// pre-close checklist after it has loaded entries for the year.
type AdjustingEntryPreCloseSummary struct {
	TotalAdjustingEntries int
	PendingDrafts         int
	MissingAuditMetadata  int
	Evidence              []string
}

func (s AdjustingEntryPreCloseSummary) Ready() bool {
	return s.PendingDrafts == 0 && s.MissingAuditMetadata == 0
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

		if !validAdjustingEntryType(entry.AdjustingEntryType) ||
			strings.TrimSpace(entry.AdjustmentReason) == "" ||
			strings.TrimSpace(entry.AdjustmentApprovalReference) == "" ||
			!entry.AdjustmentAuthorized ||
			strings.TrimSpace(entry.CreatedBy) == "" || entry.CreatedAt.IsZero() ||
			strings.TrimSpace(entry.ApprovedBy) == "" || entry.ApprovedAt == nil || entry.ApprovedAt.IsZero() {
			summary.MissingAuditMetadata++
			summary.Evidence = append(summary.Evidence, fmt.Sprintf("adjusting entry %s lacks approval or audit metadata", reference))
		}
	}
	return summary
}

func validAdjustingEntryType(t journal.AdjustingEntryType) bool {
	switch t {
	case journal.AdjustingEntryAccrual,
		journal.AdjustingEntryDeferral,
		journal.AdjustingEntryProvisionEstimate,
		journal.AdjustingEntryReclassification:
		return true
	default:
		return false
	}
}
