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
	cancelledAt := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	reversalOfID := types.NewJournalEntryID()
	reversedByID := types.NewJournalEntryID()
	entry := JournalEntry{
		PostedAt:     &postedAt,
		CancelledAt:  &cancelledAt,
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
	*clone.CancelledAt = cancelledAt.Add(time.Hour)
	*clone.ReversalOfID = types.NewJournalEntryID()
	*clone.ReversedByID = types.NewJournalEntryID()

	if entry.Lines[0].AccountID != (types.AccountID{}) ||
		*entry.Lines[0].PartnerID != partnerID ||
		*entry.Lines[0].CostCenterID != costCenterID ||
		*entry.Lines[0].ProjectID != projectID ||
		*entry.Lines[0].SegmentID != segmentID ||
		*entry.Lines[0].BusinessUnitID != businessUnitID ||
		!entry.PostedAt.Equal(postedAt) ||
		!entry.CancelledAt.Equal(cancelledAt) ||
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

func TestJournalEntryCancelAndBranchCoverage(t *testing.T) {
	now := time.Date(2026, 11, 27, 2, 0, 0, 0, time.FixedZone("UTC+3", 3*3600))
	acc1 := types.NewAccountID()

	// Nil receiver guards across aggregate methods.
	var nilEntry *JournalEntry
	if err := nilEntry.AddLine(EntryLine{AccountID: acc1}); !errors.Is(err, ErrInvalidLineMutation) {
		t.Fatalf("nil AddLine = %v", err)
	}
	if err := nilEntry.RemoveLine(types.NewJournalEntryLineID()); !errors.Is(err, ErrInvalidLineMutation) {
		t.Fatalf("nil RemoveLine = %v", err)
	}
	if err := nilEntry.MarkPosted("a", now, "fp"); !errors.Is(err, ErrInvalidEntryStatus) {
		t.Fatalf("nil MarkPosted = %v", err)
	}
	if err := nilEntry.MarkReversed(types.NewJournalEntryID()); !errors.Is(err, ErrInvalidEntryStatus) {
		t.Fatalf("nil MarkReversed = %v", err)
	}
	if err := nilEntry.Cancel("a", "reason", now); !errors.Is(err, ErrInvalidEntryStatus) {
		t.Fatalf("nil Cancel = %v", err)
	}

	// Empty LinesView and RemoveLine error branches.
	draft := JournalEntry{ID: types.NewJournalEntryID(), Status: EntryDraft}
	if view := draft.LinesView(); view != nil {
		t.Fatalf("expected nil view for empty draft, got %#v", view)
	}
	if err := draft.RemoveLine(types.JournalEntryLineID{}); !errors.Is(err, ErrInvalidLineMutation) {
		t.Fatalf("zero lineID RemoveLine = %v", err)
	}
	if err := draft.RemoveLine(types.NewJournalEntryLineID()); !errors.Is(err, ErrInvalidLineMutation) {
		t.Fatalf("missing lineID RemoveLine = %v", err)
	}
	// Auto-generated line ID when AddLine receives zero ID.
	if err := draft.AddLine(EntryLine{AccountID: acc1}); err != nil || len(draft.Lines) != 1 || draft.Lines[0].ID == (types.JournalEntryLineID{}) {
		t.Fatalf("expected auto-assigned line ID, got lines=%#v err=%v", draft.Lines, err)
	}

	// Cancel validation branches.
	if err := draft.Cancel(" ", "valid reason", now); !errors.Is(err, ErrInvalidCancellation) {
		t.Fatalf("blank actor Cancel = %v, want ErrInvalidCancellation", err)
	}
	if err := draft.Cancel("actor-1", "   ", now); !errors.Is(err, ErrInvalidCancellation) {
		t.Fatalf("blank reason Cancel = %v, want ErrInvalidCancellation", err)
	}
	if err := draft.Cancel("actor-1", "valid reason", time.Time{}); !errors.Is(err, ErrInvalidCancellation) {
		t.Fatalf("zero time Cancel = %v, want ErrInvalidCancellation", err)
	}

	// Successful Draft -> Cancelled transition.
	if err := draft.Cancel("actor-cancel", "entered duplicate draft", now); err != nil {
		t.Fatalf("draft Cancel failed: %v", err)
	}
	if draft.Status != EntryCancelled || draft.CancelledBy != "actor-cancel" ||
		draft.CancellationReason != "entered duplicate draft" ||
		draft.CancelledAt == nil || !draft.CancelledAt.Equal(now.UTC()) {
		t.Fatalf("unexpected cancelled metadata: %+v", draft)
	}
	if draft.IsMutable() {
		t.Fatal("cancelled entry must not be mutable")
	}
	if err := draft.AddLine(EntryLine{AccountID: acc1}); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("cancelled AddLine = %v, want ErrImmutableEntry", err)
	}
	if err := draft.MarkPosted("actor-1", now, "fp"); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("cancelled MarkPosted = %v, want ErrInvalidStatusTransition", err)
	}
	if err := draft.Cancel("actor-cancel", "second attempt", now); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("double Cancel = %v, want ErrInvalidStatusTransition", err)
	}

	// Immutability comparison checks cancelled fields and MatchingNumber / ReconciledAmount.
	tampered := draft.Clone()
	tampered.CancellationReason = "mutated reason"
	if err := ValidateImmutability(draft, tampered); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("mutated cancellation reason must violate immutability: %v", err)
	}
	posted := postedSampleEntry()
	reconciledTamper := posted.Clone()
	reconciledTamper.Lines[0].ReconciledAmount = reconciledTamper.Lines[0].ReconciledAmount.Add(reconciledTamper.Lines[0].ReconciledAmount)
	if err := ValidateImmutability(posted, reconciledTamper); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("mutated ReconciledAmount must violate immutability: %v", err)
	}
	if err := posted.Cancel("actor-1", "cannot cancel posted", now); !errors.Is(err, ErrCannotModifyPostedMove) || !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("posted Cancel = %v, want ErrCannotModifyPostedMove & ErrInvalidStatusTransition", err)
	}
	if err := posted.MarkReversed(types.JournalEntryID{}); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("zero reversedBy MarkReversed = %v", err)
	}
	posted.Status = EntryPosted
	other := types.NewJournalEntryID()
	posted.ReversedByID = &other
	if err := posted.MarkReversed(types.NewJournalEntryID()); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("different reversedBy MarkReversed = %v", err)
	}
	posted.Status = EntryReversed
	if err := posted.Cancel("actor-1", "cannot cancel reversed", now); !errors.Is(err, ErrCannotModifyPostedMove) {
		t.Fatalf("reversed Cancel = %v, want ErrCannotModifyPostedMove", err)
	}

	// moneyFieldEqual empty-currency branch and entriesEqual line length / nil pointer branches.
	aEmpty := postedSampleEntry()
	bEmpty := aEmpty.Clone()
	bEmpty.Lines = append(bEmpty.Lines, bEmpty.Lines[0])
	if err := ValidateImmutability(aEmpty, bEmpty); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("line count mismatch must violate immutability: %v", err)
	}
	bSame := aEmpty.Clone()
	bSame.CancelledAt = nil
	if !moneyFieldEqual(types.Money{}, types.Money{}) {
		t.Fatal("zero Money with empty currency should be equal")
	}
}
