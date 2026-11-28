package journal

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

func TestEntryStatusStateMachine(t *testing.T) {
	valid := []EntryStatus{EntryDraft, EntryPosted, EntryReversed, EntryCancelled}
	for _, s := range valid {
		if !s.IsValid() {
			t.Fatalf("expected %q to be valid", s)
		}
	}
	if EntryStatus("").IsValid() || EntryStatus("bogus").IsValid() {
		t.Fatal("invalid statuses must not be valid")
	}
	if !EntryDraft.IsMutable() || EntryPosted.IsMutable() || EntryReversed.IsMutable() || EntryCancelled.IsMutable() {
		t.Fatal("mutability mismatch")
	}
	if EntryDraft.IsTerminal() || EntryPosted.IsTerminal() || !EntryReversed.IsTerminal() || !EntryCancelled.IsTerminal() {
		t.Fatal("terminal mismatch")
	}
}

func TestEntryStatusTransitions(t *testing.T) {
	cases := []struct {
		from    EntryStatus
		to      EntryStatus
		allowed bool
	}{
		{EntryDraft, EntryPosted, true},
		{EntryDraft, EntryCancelled, true},
		{EntryPosted, EntryReversed, true},
		{EntryDraft, EntryDraft, false},
		{EntryDraft, EntryReversed, false},
		{EntryPosted, EntryDraft, false},
		{EntryPosted, EntryPosted, false},
		{EntryPosted, EntryCancelled, false},
		{EntryReversed, EntryDraft, false},
		{EntryReversed, EntryPosted, false},
		{EntryReversed, EntryReversed, false},
		{EntryReversed, EntryCancelled, false},
		{EntryCancelled, EntryDraft, false},
		{EntryCancelled, EntryPosted, false},
		{EntryCancelled, EntryReversed, false},
		{EntryCancelled, EntryCancelled, false},
		{EntryStatus("bogus"), EntryPosted, false},
		{EntryDraft, EntryStatus("bogus"), false},
	}
	for _, tc := range cases {
		if got := tc.from.CanTransitionTo(tc.to); got != tc.allowed {
			t.Fatalf("CanTransitionTo(%q,%q)=%v want %v", tc.from, tc.to, got, tc.allowed)
		}
		err := ValidateTransition(tc.from, tc.to)
		if tc.allowed && err != nil {
			t.Fatalf("ValidateTransition(%q,%q) unexpected error: %v", tc.from, tc.to, err)
		}
		if !tc.allowed && err == nil {
			t.Fatalf("ValidateTransition(%q,%q) expected error", tc.from, tc.to)
		}
		if !tc.allowed && err != nil {
			if !errors.Is(err, ErrInvalidStatusTransition) && !errors.Is(err, ErrInvalidEntryStatus) {
				t.Fatalf("ValidateTransition(%q,%q) unexpected error type: %v", tc.from, tc.to, err)
			}
		}
	}
}

func TestAllowedTransitions(t *testing.T) {
	if got := AllowedTransitions(EntryDraft); !reflect.DeepEqual(got, []EntryStatus{EntryPosted, EntryCancelled}) {
		t.Fatalf("draft transitions: %#v", got)
	}
	if got := AllowedTransitions(EntryPosted); !reflect.DeepEqual(got, []EntryStatus{EntryReversed}) {
		t.Fatalf("posted transitions: %#v", got)
	}
	if got := AllowedTransitions(EntryReversed); got != nil {
		t.Fatalf("reversed transitions: %#v", got)
	}
	if got := AllowedTransitions(EntryCancelled); got != nil {
		t.Fatalf("cancelled transitions: %#v", got)
	}
	if got := AllowedTransitions(EntryStatus("bogus")); got != nil {
		t.Fatalf("invalid transitions: %#v", got)
	}
}

func TestJournalEntryEnsureMutable(t *testing.T) {
	entry := JournalEntry{Status: EntryDraft}
	if err := entry.EnsureMutable(); err != nil {
		t.Fatalf("draft should be mutable: %v", err)
	}
	if !entry.IsMutable() {
		t.Fatal("draft entry.IsMutable() should be true")
	}
	for _, status := range []EntryStatus{EntryPosted, EntryReversed, EntryCancelled} {
		entry.Status = status
		if entry.IsMutable() {
			t.Fatalf("status %s entry.IsMutable() should be false", status)
		}
		err := entry.EnsureMutable()
		if !errors.Is(err, ErrImmutableEntry) {
			t.Fatalf("status %s should be immutable: %v", status, err)
		}
	}
	if err := (JournalEntry{Status: EntryStatus("bogus")}).EnsureMutable(); !errors.Is(err, ErrInvalidEntryStatus) {
		t.Fatalf("invalid status should fail: %v", err)
	}
}

func TestValidateImmutabilityRejectsPostedChanges(t *testing.T) {
	before := postedSampleEntry()
	after := before.Clone()
	after.Description = "tampered"
	if err := ValidateImmutability(before, after); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("expected immutable error, got %v", err)
	}

	draftBefore := before
	draftBefore.Status = EntryDraft
	draftAfter := draftBefore.Clone()
	draftAfter.Description = "ok"
	if err := ValidateImmutability(draftBefore, draftAfter); err != nil {
		t.Fatalf("draft should be mutable: %v", err)
	}
}

func TestValidateImmutabilityDetectsLineChanges(t *testing.T) {
	before := postedSampleEntry()
	after := before.Clone()
	after.Lines[0].AccountID = types.NewAccountID()
	if err := ValidateImmutability(before, after); !errors.Is(err, ErrImmutableEntry) {
		t.Fatalf("expected immutable error for line change, got %v", err)
	}
}

func TestCloneIsDeepAndCoversPointerFields(t *testing.T) {
	entry := postedSampleEntry()
	original := entry.Clone()
	clone := entry.Clone()

	if &entry.Lines[0] == &clone.Lines[0] {
		t.Fatal("line slice elements should not share backing memory")
	}
	clone.Description = "changed"
	clone.Lines[0].MatchingNumber = "changed"
	*clone.Lines[0].PartnerID = types.NewPartnerID()
	*clone.Lines[0].CostCenterID = types.NewCostCenterID()
	*clone.Lines[0].ProjectID = types.NewProjectID()
	*clone.Lines[0].SegmentID = types.NewSegmentID()
	*clone.Lines[0].BusinessUnitID = types.NewBusinessUnitID()
	*clone.PostedAt = clone.PostedAt.Add(time.Hour)
	*clone.ReversalOfID = types.NewJournalEntryID()
	*clone.ReversedByID = types.NewJournalEntryID()
	clone.Lines = append(clone.Lines, clone.Lines[0])

	if err := ValidateImmutability(original, entry); err != nil {
		t.Fatalf("original changed after clone mutation: %v", err)
	}
	if len(original.Lines) != len(entry.Lines) {
		t.Fatal("original line count changed")
	}

	// Reflection guard: any pointer-typed or slice-typed field in JournalEntry/EntryLine
	// must not alias after Clone. This catches future field additions.
	assertDeepClone(t, reflect.ValueOf(entry), reflect.ValueOf(entry.Clone()), "JournalEntry")
}

func TestValidateReversalLinkage(t *testing.T) {
	original := postedSampleEntry()
	reversalID := types.NewJournalEntryID()
	original.Status = EntryReversed
	original.ReversedByID = &reversalID
	reversal := postedSampleEntry()
	reversal.ID = reversalID
	reversal.CompanyID = original.CompanyID
	reversal.Status = EntryPosted
	reversal.Classification = EntryClassReversing
	reversal.ReversalOfID = &original.ID
	reversal.ReversedByID = nil

	if err := ValidateReversalLinkage(original, reversal); err != nil {
		t.Fatalf("valid linkage rejected: %v", err)
	}

	bad := reversal
	bad.ReversalOfID = nil
	if err := ValidateReversalLinkage(original, bad); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("missing reversal link should fail: %v", err)
	}

	bad2 := original
	other := types.NewJournalEntryID()
	bad2.ReversedByID = &other
	if err := ValidateReversalLinkage(bad2, reversal); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("mismatched reversed_by should fail: %v", err)
	}

	draftOriginal := original
	draftOriginal.Status = EntryDraft
	if err := ValidateReversalLinkage(draftOriginal, reversal); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("draft original should fail: %v", err)
	}

	cancelledOriginal := original
	cancelledOriginal.Status = EntryCancelled
	if err := ValidateReversalLinkage(cancelledOriginal, reversal); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("cancelled original should fail: %v", err)
	}
	cancelledReversal := reversal
	cancelledReversal.Status = EntryCancelled
	if err := ValidateReversalLinkage(original, cancelledReversal); !errors.Is(err, ErrInvalidReversalLinkage) {
		t.Fatalf("cancelled reversal should fail: %v", err)
	}
}

func postedSampleEntry() JournalEntry {
	postedAt := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	reversalOf := types.NewJournalEntryID()
	reversedBy := types.NewJournalEntryID()
	partner := types.NewPartnerID()
	cc := types.NewCostCenterID()
	project := types.NewProjectID()
	segment := types.NewSegmentID()
	bu := types.NewBusinessUnitID()
	money := mustMoney("100.00", "USD")
	zero := mustMoney("0", "USD")
	return JournalEntry{
		ID:                  types.NewJournalEntryID(),
		CompanyID:           types.NewCompanyID(),
		JournalID:           types.NewJournalID(),
		FiscalPeriodID:      types.NewFiscalPeriodID(),
		EntryNumber:         "JE-1",
		SequenceNumber:      1,
		LegalNumber:         "L-1",
		Status:              EntryPosted,
		Classification:      EntryClassRegular,
		EffectiveDate:       time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		PostingDate:         postedAt,
		Description:         "desc",
		SourceModule:        "test",
		ReferenceID:         "ref",
		TransactionCurrency: "USD",
		ExchangeRate:        decimal.NewFromInt(1),
		Lines: []EntryLine{{
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           types.NewAccountID(),
			PartnerID:           &partner,
			CostCenterID:        &cc,
			ProjectID:           &project,
			SegmentID:           &segment,
			BusinessUnitID:      &bu,
			MatchingNumber:      "M-1",
			ReconciledAmount:    decimal.RequireFromString("10.00"),
			TransactionAmount:   money,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     money,
			FunctionalCredit:    zero,
		}, {
			ID:                  types.NewJournalEntryLineID(),
			AccountID:           types.NewAccountID(),
			TransactionAmount:   money,
			TransactionCurrency: "USD",
			ExchangeRate:        decimal.NewFromInt(1),
			FunctionalDebit:     zero,
			FunctionalCredit:    money,
		}},
		CreatedBy:      "creator",
		CreatedAt:      postedAt.Add(-time.Hour),
		UpdatedAt:      postedAt,
		PostedBy:       "poster",
		PostedAt:       &postedAt,
		ReversalOfID:   &reversalOf,
		ReversedByID:   &reversedBy,
		ReversalReason: "",
	}
}

func assertDeepClone(t *testing.T, a, b reflect.Value, path string) {
	t.Helper()
	if !a.IsValid() || !b.IsValid() {
		return
	}
	if a.Type() != b.Type() {
		t.Fatalf("%s type mismatch", path)
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				t.Fatalf("%s nil mismatch", path)
			}
			return
		}
		if a.UnsafePointer() == b.UnsafePointer() {
			t.Fatalf("%s pointer aliases", path)
		}
		assertDeepClone(t, a.Elem(), b.Elem(), path+".*")
	case reflect.Slice:
		if a.IsNil() || b.IsNil() {
			if a.IsNil() != b.IsNil() {
				t.Fatalf("%s nil mismatch", path)
			}
			return
		}
		if a.Len() > 0 && a.Index(0).UnsafeAddr() == b.Index(0).UnsafeAddr() {
			t.Fatalf("%s slice backing array aliases", path)
		}
		for i := 0; i < a.Len() && i < b.Len(); i++ {
			assertDeepClone(t, a.Index(i), b.Index(i), path+"["+string(rune('0'+i))+"]")
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			field := a.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			assertDeepClone(t, a.Field(i), b.Field(i), path+"."+field.Name)
		}
	}
}

func mustMoney(amount, currency string) types.Money {
	m, err := types.NewMoney(decimal.RequireFromString(amount), currency)
	if err != nil {
		panic(err)
	}
	return m
}
