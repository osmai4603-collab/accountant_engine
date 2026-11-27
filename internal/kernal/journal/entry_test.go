package journal

import (
	"errors"
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

func TestJournalEntryAggregateRootGuardsMutations(t *testing.T) {
	entry := JournalEntry{
		ID:     types.NewJournalEntryID(),
		Status: EntryDraft,
	}
	acc1 := types.NewAccountID()
	acc2 := types.NewAccountID()
	line1 := EntryLine{ID: types.NewJournalEntryLineID(), AccountID: acc1}
	line2 := EntryLine{ID: types.NewJournalEntryLineID(), AccountID: acc2}

	if err := entry.AddLine(EntryLine{ID: types.NewJournalEntryLineID()}); !errors.Is(err, ErrInvalidLineMutation) {
		t.Fatalf("expected ErrInvalidLineMutation for missing account id, got %v", err)
	}
	if err := entry.AddLine(line1); err != nil {
		t.Fatalf("AddLine line1 failed: %v", err)
	}
	if err := entry.AddLine(line1); !errors.Is(err, ErrInvalidLineMutation) {
		t.Fatalf("expected duplicate line id error, got %v", err)
	}
	if err := entry.AddLine(line2); err != nil {
		t.Fatalf("AddLine line2 failed: %v", err)
	}

	view := entry.LinesView()
	view[0].AccountID = types.NewAccountID()
	if entry.Lines[0].AccountID != acc1 {
		t.Fatal("LinesView leaked mutable slice backing array")
	}

	if err := entry.RemoveLine(line1.ID); err != nil {
		t.Fatalf("RemoveLine failed: %v", err)
	}
	if len(entry.Lines) != 1 || entry.Lines[0].ID != line2.ID {
		t.Fatalf("unexpected remaining lines after RemoveLine: %#v", entry.Lines)
	}

	now := time.Date(2026, 11, 27, 1, 0, 0, 0, time.UTC)
	if err := entry.MarkPosted("actor-1", now, "fp-v1"); err != nil {
		t.Fatalf("MarkPosted failed: %v", err)
	}
	if entry.Status != EntryPosted || entry.PostedBy != "actor-1" || entry.PolicyFingerprint != "fp-v1" {
		t.Fatalf("unexpected posted metadata: %#v", entry)
	}

	if err := entry.AddLine(EntryLine{ID: types.NewJournalEntryLineID(), AccountID: acc1}); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("expected ErrImmutableEntry on posted AddLine, got %v", err)
	}
	if err := entry.RemoveLine(line2.ID); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("expected ErrImmutableEntry on posted RemoveLine, got %v", err)
	}
	if err := entry.MarkPosted("actor-2", now, "fp-v2"); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("expected ErrInvalidStatusTransition on double MarkPosted, got %v", err)
	}

	revID := types.NewJournalEntryID()
	if err := entry.MarkReversed(revID); err != nil {
		t.Fatalf("MarkReversed failed: %v", err)
	}
	if err := entry.MarkReversed(types.NewJournalEntryID()); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("expected ErrInvalidStatusTransition on double MarkReversed, got %v", err)
	}
}
