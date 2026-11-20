package journal

import (
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func TestJournalEntryCloneOwnsSlicesAndPointers(t *testing.T) {
	partnerID := types.NewPartnerID()
	costCenterID := types.NewCostCenterID()
	projectID := types.NewProjectID()
	segmentID := types.NewSegmentID()
	businessUnitID := types.NewBusinessUnitID()
	postedAt := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	reversalOfID := types.NewJournalEntryID()
	reversedByID := types.NewJournalEntryID()
	entry := JournalEntry{
		PostedAt:     &postedAt,
		ReversalOfID: &reversalOfID,
		ReversedByID: &reversedByID,
		Lines: []EntryLine{{
			PartnerID:      &partnerID,
			CostCenterID:   &costCenterID,
			ProjectID:      &projectID,
			SegmentID:      &segmentID,
			BusinessUnitID: &businessUnitID,
		}},
	}

	clone := entry.Clone()
	clone.Lines[0].AccountID = types.NewAccountID()
	*clone.Lines[0].PartnerID = types.NewPartnerID()
	*clone.Lines[0].CostCenterID = types.NewCostCenterID()
	*clone.Lines[0].ProjectID = types.NewProjectID()
	*clone.Lines[0].SegmentID = types.NewSegmentID()
	*clone.Lines[0].BusinessUnitID = types.NewBusinessUnitID()
	*clone.PostedAt = postedAt.Add(time.Hour)
	*clone.ReversalOfID = types.NewJournalEntryID()
	*clone.ReversedByID = types.NewJournalEntryID()

	if entry.Lines[0].AccountID != (types.AccountID{}) ||
		*entry.Lines[0].PartnerID != partnerID ||
		*entry.Lines[0].CostCenterID != costCenterID ||
		*entry.Lines[0].ProjectID != projectID ||
		*entry.Lines[0].SegmentID != segmentID ||
		*entry.Lines[0].BusinessUnitID != businessUnitID ||
		!entry.PostedAt.Equal(postedAt) ||
		*entry.ReversalOfID != reversalOfID ||
		*entry.ReversedByID != reversedByID {
		t.Fatal("mutating cloned entry changed original slices or pointers")
	}
}
