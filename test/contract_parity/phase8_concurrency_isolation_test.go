package contract_parity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// phase8LockRecorder is the shared, concurrency-safe observer used by the
// Phase 8 contract tests. Adapters must call it through
// storage.WithLockObserver for every actual lock acquisition in CommitPosting.
type phase8LockRecorder struct {
	mu       sync.Mutex
	acquired []storage.LockTarget
	released []storage.LockTarget
}

func (r *phase8LockRecorder) Acquired(target storage.LockTarget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acquired = append(r.acquired, target)
}

func (r *phase8LockRecorder) Released(target storage.LockTarget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.released = append(r.released, target)
}

func (r *phase8LockRecorder) snapshot() (acquired, released []storage.LockTarget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]storage.LockTarget(nil), r.acquired...), append([]storage.LockTarget(nil), r.released...)
}

func phase8TestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func phase8CanonicalKeys(targets []storage.LockTarget) []string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, target.Canonical())
	}
	return keys
}

// phase8AssertOrdered proves that the adapter acquired the real locks in
// canonical order and released them in reverse order. Sorting the target slice
// alone cannot satisfy this assertion: the observer is invoked by the acquire
// and release primitives inside the adapter.
func phase8AssertOrdered(t *testing.T, recorder *phase8LockRecorder, wantCompany types.CompanyID) {
	t.Helper()
	acquired, released := recorder.snapshot()
	if len(acquired) == 0 {
		t.Fatal("adapter did not report actual lock acquisitions to the observer")
	}
	for i, target := range acquired {
		if err := target.Validate(); err != nil {
			t.Fatalf("acquired invalid lock target: %v", err)
		}
		if target.CompanyID != wantCompany {
			t.Fatalf("lock target %d has company %s, want %s", i, target.CompanyID, wantCompany)
		}
		if i > 0 && acquired[i-1].Canonical() >= target.Canonical() {
			t.Fatalf("actual lock acquisitions are not strictly ascending: %v", phase8CanonicalKeys(acquired))
		}
	}
	if len(released) != len(acquired) {
		t.Fatalf("released %d locks, acquired %d", len(released), len(acquired))
	}
	for i := range acquired {
		want := acquired[len(acquired)-1-i].Canonical()
		if released[i].Canonical() != want {
			t.Fatalf("release %d = %s, want reverse order %s", i, released[i].Canonical(), want)
		}
	}
	kinds := map[storage.LockKind]bool{}
	for _, target := range acquired {
		kinds[target.Kind] = true
	}
	for _, required := range []storage.LockKind{storage.LockKindAccount, storage.LockKindEntry, storage.LockKindIdempotency} {
		if !kinds[required] {
			t.Fatalf("missing required lock kind %s in %v", required, phase8CanonicalKeys(acquired))
		}
	}
}

func phase8Commit(t *testing.T, ctx context.Context, h *StorageAdapterHarness, commit storage.PostingCommit) journal.JournalEntry {
	t.Helper()
	posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
	if err != nil {
		t.Fatalf("CommitPosting: %v", err)
	}
	return posted
}

func phase8SecondCompany(t *testing.T, ctx context.Context, h *StorageAdapterHarness) UnifiedFixture {
	t.Helper()
	other := NewUnifiedFixture()
	if err := ProvisionFixture(ctx, h.MasterData, other); err != nil {
		t.Fatalf("provision second company: %v", err)
	}
	return other
}

func phase8Lookup(t *testing.T, ctx context.Context, h *StorageAdapterHarness, companyID types.CompanyID, key string) (storage.IdempotencyRecord, bool) {
	t.Helper()
	lookup, ok := h.Ports.Idempotency.(storage.IdempotencyLookup)
	if !ok {
		t.Fatal("adapter does not implement IdempotencyLookup")
	}
	record, found, err := lookup.Lookup(ctx, companyID, key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	return record, found
}

// TestPhase8_LockOrderEnforced proves that the adapter, not the kernel,
// performs the actual ordered lock acquisition and release.
func TestPhase8_LockOrderEnforced(t *testing.T) {
	ForEachStorageAdapter(t, "LockOrderEnforced", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		recorder := &phase8LockRecorder{}
		observedCtx := storage.WithLockObserver(ctx, recorder)

		commit := phase7PostingCommit(f, "p8-order-key", "p8-order-hash")
		posted := phase8Commit(t, observedCtx, h, commit)
		if posted.ID != commit.Entry.ID {
			t.Fatalf("posted entry %s != submitted %s", posted.ID, commit.Entry.ID)
		}
		phase8AssertOrdered(t, recorder, f.Company.ID)
	})
}

// TestPhase8_SameAccountConcurrentPosting posts two different entries that
// touch the same accounts at the same time. Both must succeed: shared-account
// serialization must not be mistaken for idempotency suppression.
func TestPhase8_SameAccountConcurrentPosting(t *testing.T) {
	ForEachStorageAdapter(t, "SameAccountConcurrentPosting", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		first := phase7PostingCommit(f, "p8-same-key-1", "p8-same-hash-1")
		second := phase7PostingCommit(f, "p8-same-key-2", "p8-same-hash-2")
		second.Entry.SequenceNumber = 2

		var wg sync.WaitGroup
		results := make(chan journal.JournalEntry, 2)
		errs := make(chan error, 2)
		start := make(chan struct{})
		wg.Add(2)
		for _, commit := range []storage.PostingCommit{first, second} {
			go func(commit storage.PostingCommit) {
				defer wg.Done()
				<-start
				posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
				if err != nil {
					errs <- err
					return
				}
				results <- posted
			}(commit)
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent shared-account posting failed: %v", err)
		}
		var seen []types.JournalEntryID
		for posted := range results {
			seen = append(seen, posted.ID)
		}
		if len(seen) != 2 || seen[0] == seen[1] {
			t.Fatalf("distinct postings collapsed into one entry: %v", seen)
		}
		phase7RequireBalance(t, ctx, h, f, "200")
	})
}

// TestPhase8_CrossAccountReverseOrderNoDeadlock submits two postings whose
// line order is reversed. Without canonical locking this interleaving can
// deadlock; with it both commits succeed before the bounded context expires.
func TestPhase8_CrossAccountReverseOrderNoDeadlock(t *testing.T) {
	ForEachStorageAdapter(t, "CrossAccountReverseOrderNoDeadlock", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		first := phase7PostingCommit(f, "p8-reverse-key-1", "p8-reverse-hash-1")
		second := phase7PostingCommit(f, "p8-reverse-key-2", "p8-reverse-hash-2")
		second.Entry.SequenceNumber = 2
		first.Entry.Lines[0], first.Entry.Lines[1] = first.Entry.Lines[1], first.Entry.Lines[0]
		recorderA, recorderB := &phase8LockRecorder{}, &phase8LockRecorder{}
		ctxA := storage.WithLockObserver(ctx, recorderA)
		ctxB := storage.WithLockObserver(ctx, recorderB)

		var wg sync.WaitGroup
		errs := make(chan error, 2)
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.Ports.Posting.CommitPosting(ctxA, first)
			if err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := h.Ports.Posting.CommitPosting(ctxB, second)
			if err != nil {
				errs <- err
			}
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("reverse-order posting failed (possible deadlock): %v", err)
		}
		phase8AssertOrdered(t, recorderA, f.Company.ID)
		phase8AssertOrdered(t, recorderB, f.Company.ID)
		phase7RequireBalance(t, ctx, h, f, "200")
	})
}

// TestPhase8_ConcurrentReplayAtScale runs a replay storm on one idempotency
// key. Every caller must observe exactly the same committed entry.
func TestPhase8_ConcurrentReplayAtScale(t *testing.T) {
	ForEachStorageAdapter(t, "ConcurrentReplayAtScale", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		commit := phase7PostingCommit(f, "p8-replay-key", "p8-replay-hash")
		const contenders = 16
		recorders := make([]*phase8LockRecorder, contenders)
		var wg sync.WaitGroup
		results := make(chan journal.JournalEntry, contenders)
		errs := make(chan error, contenders)
		start := make(chan struct{})
		wg.Add(contenders)
		for i := 0; i < contenders; i++ {
			recorder := &phase8LockRecorder{}
			recorders[i] = recorder
			observedCtx := storage.WithLockObserver(ctx, recorder)
			go func() {
				defer wg.Done()
				<-start
				posted, err := h.Ports.Posting.CommitPosting(observedCtx, commit)
				if err != nil {
					errs <- err
					return
				}
				results <- posted
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent replay failed: %v", err)
		}
		var want types.JournalEntryID
		for i, posted := range readChannel(results) {
			if i == 0 {
				want = posted.ID
				continue
			}
			if posted.ID != want {
				t.Fatalf("replay returned different entries: %s vs %s", want, posted.ID)
			}
		}
		if want == (types.JournalEntryID{}) {
			t.Fatal("no replay commit succeeded")
		}
		for _, recorder := range recorders {
			phase8AssertOrdered(t, recorder, f.Company.ID)
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

// TestPhase8_ConcurrentReversalSingleWinner races reversals of the same
// original entry using the same idempotency key. All callers must observe one
// reversal and the original must be linked exactly once.
func TestPhase8_ConcurrentReversalSingleWinner(t *testing.T) {
	ForEachStorageAdapter(t, "ConcurrentReversalSingleWinner", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		original := phase8Commit(t, ctx, h, phase7PostingCommit(f, "p8-original-key", "p8-original-hash"))
		reversal := phase7ReversalCommit(f, original, "p8-reversal-key", "p8-reversal-hash")

		const contenders = 12
		recorders := make([]*phase8LockRecorder, contenders)
		var wg sync.WaitGroup
		results := make(chan journal.JournalEntry, contenders)
		errs := make(chan error, contenders)
		start := make(chan struct{})
		wg.Add(contenders)
		for i := 0; i < contenders; i++ {
			recorder := &phase8LockRecorder{}
			recorders[i] = recorder
			observedCtx := storage.WithLockObserver(ctx, recorder)
			go func() {
				defer wg.Done()
				<-start
				posted, err := h.Ports.Posting.CommitPosting(observedCtx, reversal)
				if err != nil {
					errs <- err
					return
				}
				results <- posted
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent reversal failed: %v", err)
		}
		var want types.JournalEntryID
		for i, posted := range readChannel(results) {
			if i == 0 {
				want = posted.ID
				continue
			}
			if posted.ID != want {
				t.Fatalf("reversal returned different entries: %s vs %s", want, posted.ID)
			}
		}
		stored, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, original.ID)
		if err != nil {
			t.Fatalf("read original: %v", err)
		}
		if stored.Status != journal.EntryReversed || stored.ReversedByID == nil || *stored.ReversedByID != want {
			t.Fatalf("original not atomically linked to the single reversal: %+v", stored)
		}
		for _, recorder := range recorders {
			phase8AssertOrdered(t, recorder, f.Company.ID)
		}
	})
}

// TestPhase8_InterleavedCompanyIsolation interleaves commits for two companies
// and verifies every read and every lock remains inside its own scope.
func TestPhase8_InterleavedCompanyIsolation(t *testing.T) {
	ForEachStorageAdapter(t, "InterleavedCompanyIsolation", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		other := phase8SecondCompany(t, ctx, h)
		firstCommit := phase7PostingCommit(f, "p8-iso-key-1", "p8-iso-hash-1")
		secondCommit := phase7PostingCommit(other, "p8-iso-key-2", "p8-iso-hash-2")
		recorderA, recorderB := &phase8LockRecorder{}, &phase8LockRecorder{}
		ctxA := storage.WithLockObserver(ctx, recorderA)
		ctxB := storage.WithLockObserver(ctx, recorderB)

		var wg sync.WaitGroup
		errs := make(chan error, 2)
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.Ports.Posting.CommitPosting(ctxA, firstCommit)
			if err != nil {
				errs <- fmt.Errorf("company A: %w", err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			_, err := h.Ports.Posting.CommitPosting(ctxB, secondCommit)
			if err != nil {
				errs <- fmt.Errorf("company B: %w", err)
			}
		}()
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("interleaved company posting failed: %v", err)
		}
		phase8AssertOrdered(t, recorderA, f.Company.ID)
		phase8AssertOrdered(t, recorderB, other.Company.ID)

		if _, found := phase8Lookup(t, ctx, h, f.Company.ID, "p8-iso-key-1"); !found {
			t.Fatal("company A idempotency record missing")
		}
		if _, found := phase8Lookup(t, ctx, h, other.Company.ID, "p8-iso-key-2"); !found {
			t.Fatal("company B idempotency record missing")
		}
		if _, found := phase8Lookup(t, ctx, h, f.Company.ID, "p8-iso-key-2"); found {
			t.Fatal("company B idempotency key leaked into company A")
		}
		if _, found := phase8Lookup(t, ctx, h, other.Company.ID, "p8-iso-key-1"); found {
			t.Fatal("company A idempotency key leaked into company B")
		}
		phase7RequireBalance(t, ctx, h, f, "100")
		phase7RequireBalance(t, ctx, h, other, "100")
	})
}

// TestPhase8_CompanyScopeOnEveryPort verifies that an entry committed for one
// company is unreachable through the other company's scope on every read port.
func TestPhase8_CompanyScopeOnEveryPort(t *testing.T) {
	ForEachStorageAdapter(t, "CompanyScopeOnEveryPort", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		other := phase8SecondCompany(t, ctx, h)
		posted := phase8Commit(t, ctx, h, phase7PostingCommit(f, "p8-scope-key", "p8-scope-hash"))

		if _, err := h.Ports.Entries.GetEntry(ctx, other.Company.ID, posted.ID); err == nil {
			t.Fatal("EntryReader leaked entry across companies")
		}
		if _, found := phase8Lookup(t, ctx, h, other.Company.ID, "p8-scope-key"); found {
			t.Fatal("idempotency lookup leaked record across companies")
		}
		balance, err := h.Ports.Balance.GetAccountBalance(ctx, other.Company.ID, other.CashAccount.ID, other.OpenPeriod.StartDate, other.FunctionalCurrency)
		if err != nil {
			t.Fatalf("other company balance: %v", err)
		}
		if !balance.IsZero() {
			t.Fatalf("other company balance = %s, want 0", balance)
		}
		if _, err := h.Ports.MasterData.GetAccount(ctx, other.Company.ID, f.CashAccount.ID); err == nil {
			t.Fatal("master data reader leaked account across companies")
		}
	})
}

// TestPhase8_FrozenSnapshotRemainsStableUnderConcurrentStateChange freezes a
// commit, mutates master data used to derive it, and verifies that the stored
// entry uses only the frozen values. No adapter re-reads master data.
func TestPhase8_FrozenSnapshotRemainsStableUnderConcurrentStateChange(t *testing.T) {
	ForEachStorageAdapter(t, "FrozenSnapshotRemainsStable", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		commit := phase7PostingCommit(f, "p8-snapshot-key", "p8-snapshot-hash")
		postedAt := time.Date(2026, 1, 5, 10, 11, 12, 0, time.UTC)
		commit.Entry.PostedAt = &postedAt

		// Add another open period while the frozen commit is in flight. A
		// correct adapter must not select a period by date at write time.
		extraPeriod := masterdata.FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    f.Company.ID,
			FiscalYearID: f.FiscalYear.ID,
			Number:       3,
			Name:         "changed period",
			StartDate:    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			EndDate:      time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
			State:        masterdata.PeriodOpen,
			Type:         masterdata.FiscalPeriodTypeNormal,
		}
		if err := h.MasterData.CreateFiscalPeriod(ctx, extraPeriod); err != nil {
			t.Fatalf("create changed period: %v", err)
		}

		posted := phase8Commit(t, ctx, h, commit)
		stored, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, posted.ID)
		if err != nil {
			t.Fatalf("read stored entry: %v", err)
		}
		if stored.FiscalPeriodID != commit.Entry.FiscalPeriodID {
			t.Fatalf("stored period %s != frozen period %s", stored.FiscalPeriodID, commit.Entry.FiscalPeriodID)
		}
		if !stored.PostingDate.Equal(commit.Entry.PostingDate) {
			t.Fatalf("stored posting date %s != frozen %s", stored.PostingDate, commit.Entry.PostingDate)
		}
		if stored.PostedAt == nil || !stored.PostedAt.Equal(postedAt) {
			t.Fatalf("stored posted at %v != frozen %s", stored.PostedAt, postedAt)
		}
	})
}

// TestPhase8_ScopeViolationRejected verifies that an idempotency scope naming
// a different company is rejected with the non-retryable scope error.
func TestPhase8_ScopeViolationRejected(t *testing.T) {
	ForEachStorageAdapter(t, "ScopeViolationRejected", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := phase8TestContext(t)
		other := phase8SecondCompany(t, ctx, h)
		commit := phase7PostingCommit(f, "p8-scope-violation-key", "p8-scope-violation-hash")
		commit.Idempotency.Scope = phase7Scope(other.Company.ID, "post", "p8-scope-violation-key")

		if _, err := h.Ports.Posting.CommitPosting(ctx, commit); !errors.Is(err, storage.ErrScopeViolation) {
			t.Fatalf("cross-company scope error = %v, want ErrScopeViolation", err)
		}
		if _, err := h.Ports.Entries.GetEntry(ctx, f.Company.ID, commit.Entry.ID); err == nil {
			t.Fatal("entry persisted despite scope violation")
		}
		if _, found := phase8Lookup(t, ctx, h, f.Company.ID, commit.Idempotency.Key); found {
			t.Fatal("idempotency record persisted despite scope violation")
		}
	})
}

// TestPhase8_CanonicalCompanyLockOrder is a pure SPI check that company identity
// participates in the canonical lock order and every target is deduplicated.
func TestPhase8_CanonicalCompanyLockOrder(t *testing.T) {
	companyA, companyB := types.NewCompanyID(), types.NewCompanyID()
	targets := []storage.LockTarget{
		{Kind: storage.LockKindAccount, CompanyID: companyB, ID: "1000"},
		{Kind: storage.LockKindAccount, CompanyID: companyA, ID: "9000"},
		{Kind: storage.LockKindAccount, CompanyID: companyA, ID: "9000"},
	}
	ordered := storage.OrderLockTargets(targets)
	if len(ordered) != 2 {
		t.Fatalf("expected deduplicated targets, got %d", len(ordered))
	}
	if ordered[0].Canonical() >= ordered[1].Canonical() {
		t.Fatalf("targets not ordered: %v", phase8CanonicalKeys(ordered))
	}
	if len(targets) != 3 || targets[0].ID != "1000" {
		t.Fatal("OrderLockTargets mutated its input")
	}
	for _, target := range ordered {
		if err := target.Validate(); err != nil {
			t.Fatalf("ordered target invalid: %v", err)
		}
	}
}

func readChannel[T any](ch <-chan T) []T {
	var values []T
	for value := range ch {
		values = append(values, value)
	}
	return values
}
