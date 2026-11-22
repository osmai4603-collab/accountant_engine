package storage_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	publictypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

type recordingLockObserver struct {
	acquired []storage.LockTarget
	released []storage.LockTarget
}

func (o *recordingLockObserver) Acquired(target storage.LockTarget) {
	o.acquired = append(o.acquired, target)
}
func (o *recordingLockObserver) Released(target storage.LockTarget) {
	o.released = append(o.released, target)
}

func lockTargetsForTest(company publictypes.CompanyID, ids ...string) []storage.LockTarget {
	targets := make([]storage.LockTarget, 0, len(ids))
	for _, id := range ids {
		targets = append(targets, storage.LockTarget{Kind: storage.LockKindAccount, CompanyID: company, ID: id})
	}
	return targets
}

func TestLockTargetsAreCanonicalAndSorted(t *testing.T) {
	company := publictypes.NewCompanyID()
	other := publictypes.NewCompanyID()

	unsorted := append(lockTargetsForTest(company, "c", "a", "b", "a"),
		storage.LockTarget{Kind: storage.LockKindEntry, CompanyID: company, ID: "entry"},
		storage.LockTarget{Kind: storage.LockKindAccount, CompanyID: other, ID: "a"},
	)
	original := append([]storage.LockTarget(nil), unsorted...)
	got := storage.OrderLockTargets(unsorted)

	if !reflect.DeepEqual(unsorted, original) {
		t.Fatal("OrderLockTargets mutated the caller's slice")
	}
	if len(got) != 5 {
		t.Fatalf("ordered targets = %d, want 5 unique targets: %v", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Canonical() >= got[i].Canonical() {
			t.Fatalf("targets not strictly ascending at %d:\n got %q\nwant < %q", i, got[i-1].Canonical(), got[i].Canonical())
		}
	}
	if got[0].CompanyID != company && got[0].CompanyID != other {
		t.Fatal("company scope disappeared from canonical lock targets")
	}
}

func TestLockTargetRejectsMissingScope(t *testing.T) {
	for _, target := range []storage.LockTarget{
		{},
		{Kind: storage.LockKindAccount, ID: "account"},
		{Kind: storage.LockKindAccount, CompanyID: publictypes.NewCompanyID()},
	} {
		if err := target.Validate(); err == nil {
			t.Fatalf("expected validation error for %+v", target)
		}
	}
}

func TestPostingLockTargetsCoverAllConcurrencyKeys(t *testing.T) {
	company := publictypes.NewCompanyID()
	originalID := publictypes.NewJournalEntryID()
	entryID := publictypes.NewJournalEntryID()
	commit := storage.PostingCommit{
		Entry: journal.JournalEntry{
			ID:        entryID,
			CompanyID: company,
			Lines: []journal.EntryLine{
				{AccountID: publictypes.NewAccountID()},
				{AccountID: publictypes.NewAccountID()},
			},
		},
		Operation:       storage.OperationReverse,
		OriginalEntryID: &originalID,
		Idempotency: &storage.PostingIdempotency{
			Key:         "phase8-key",
			Scope:       fmt.Sprintf("company=%s|operation=reverse|key=phase8-key", company.String()),
			PayloadHash: "hash",
		},
	}

	targets := storage.PostingLockTargets(commit)
	wantKinds := map[storage.LockKind]int{
		storage.LockKindAccount:     2,
		storage.LockKindEntry:       1,
		storage.LockKindOriginal:    1,
		storage.LockKindIdempotency: 1,
	}
	counts := make(map[storage.LockKind]int)
	for _, target := range targets {
		counts[target.Kind]++
		if target.CompanyID != company {
			t.Fatalf("lock target from another company: %+v", target)
		}
	}
	if !reflect.DeepEqual(counts, wantKinds) {
		t.Fatalf("lock target kinds = %v, want %v", counts, wantKinds)
	}
	again := storage.PostingLockTargets(commit)
	if !reflect.DeepEqual(targets, again) {
		t.Fatalf("PostingLockTargets is not deterministic:\n%v\n%v", targets, again)
	}
}

func TestWithOrderedLocksAcquiresAndReleasesInOrder(t *testing.T) {
	company := publictypes.NewCompanyID()
	targets := lockTargetsForTest(company, "c", "a", "b")
	observer := &recordingLockObserver{}
	ctx := storage.WithLockObserver(context.Background(), observer)

	var acquired []string
	acquire := func(_ context.Context, target storage.LockTarget) (func(), error) {
		acquired = append(acquired, target.ID)
		return func() {}, nil
	}
	if err := storage.WithOrderedLocks(ctx, targets, acquire, func() error { return nil }); err != nil {
		t.Fatalf("WithOrderedLocks: %v", err)
	}
	if !reflect.DeepEqual(acquired, []string{"a", "b", "c"}) {
		t.Fatalf("acquisition order = %v, want sorted [a b c]", acquired)
	}
	if len(observer.acquired) != 3 || len(observer.released) != 3 {
		t.Fatalf("observer saw %d acquisitions and %d releases, want 3 each", len(observer.acquired), len(observer.released))
	}
	for i := range observer.acquired {
		if observer.acquired[i] != observer.released[len(observer.released)-1-i] {
			t.Fatalf("release order is not LIFO: %v vs %v", observer.acquired, observer.released)
		}
	}
}

func TestWithOrderedLocksReleasesAcquiredTargetsOnFailure(t *testing.T) {
	company := publictypes.NewCompanyID()
	targets := lockTargetsForTest(company, "a", "b", "c")
	observer := &recordingLockObserver{}
	ctx := storage.WithLockObserver(context.Background(), observer)

	wantErr := errors.New("lock b unavailable")
	acquire := func(_ context.Context, target storage.LockTarget) (func(), error) {
		if target.ID == "b" {
			return nil, wantErr
		}
		return func() {}, nil
	}
	err := storage.WithOrderedLocks(ctx, targets, acquire, func() error {
		t.Fatal("callback must not run when acquisition fails")
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if len(observer.released) != 1 || observer.released[0].ID != "a" {
		t.Fatalf("failed acquisition did not release earlier locks: %v", observer.released)
	}
}

func TestValidateIdempotencyScopeRejectsForeignCompany(t *testing.T) {
	company := publictypes.NewCompanyID()
	foreign := publictypes.NewCompanyID()
	good := &storage.PostingIdempotency{
		Key:   "key",
		Scope: fmt.Sprintf("company=%s|operation=post|key=key", company.String()),
	}
	if err := storage.ValidateIdempotencyScope(company, storage.OperationPost, good); err != nil {
		t.Fatalf("canonical scope rejected: %v", err)
	}
	for name, idem := range map[string]*storage.PostingIdempotency{
		"foreign company":   {Key: "key", Scope: fmt.Sprintf("company=%s|operation=post|key=key", foreign.String())},
		"foreign operation": {Key: "key", Scope: fmt.Sprintf("company=%s|operation=reverse|key=key", company.String())},
		"foreign key":       {Key: "key", Scope: fmt.Sprintf("company=%s|operation=post|key=other", company.String())},
	} {
		if err := storage.ValidateIdempotencyScope(company, storage.OperationPost, idem); !errors.Is(err, storage.ErrScopeViolation) {
			t.Fatalf("%s: error = %v, want ErrScopeViolation", name, err)
		}
	}
	// Legacy callers without a scope remain source compatible; the company
	// column of the idempotency record still isolates them.
	if err := storage.ValidateIdempotencyScope(company, storage.OperationPost, nil); err != nil {
		t.Fatalf("nil idempotency rejected: %v", err)
	}
}

func TestRetryPolicyIsBounded(t *testing.T) {
	if err := storage.DefaultRetryPolicy().Validate(); err != nil {
		t.Fatalf("default retry policy invalid: %v", err)
	}
	for _, policy := range []storage.RetryPolicy{
		{MaxRetries: -1},
		{MaxRetries: 100},
		{MaxRetries: 1, InitialBackoff: -1},
		{MaxRetries: 1, InitialBackoff: 10, MaxBackoff: 1},
	} {
		if err := policy.Validate(); err == nil {
			t.Fatalf("unbounded/invalid retry policy accepted: %+v", policy)
		}
	}
}
