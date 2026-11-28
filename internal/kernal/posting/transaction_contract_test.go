package posting

import (
	"context"
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func TestIdempotencyScopeValidatesAllUniquenessComponents(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	valid := IdempotencyScope{
		CompanyID: entry.CompanyID,
		Operation: IdempotencyPosting,
		Key:       "scope-1",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid scope rejected: %v", err)
	}

	cases := []struct {
		name  string
		scope IdempotencyScope
		want  error
	}{
		{
			name:  "missing company",
			scope: IdempotencyScope{Operation: IdempotencyPosting, Key: "scope-1"},
			want:  ErrIdempotencyScopeRequired,
		},
		{
			name:  "unknown operation",
			scope: IdempotencyScope{CompanyID: entry.CompanyID, Key: "scope-1"},
			want:  ErrIdempotencyScopeRequired,
		},
		{
			name:  "blank key",
			scope: IdempotencyScope{CompanyID: entry.CompanyID, Operation: IdempotencyPosting, Key: "   "},
			want:  ErrIdempotencyScopeRequired,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.scope.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRequestFingerprintIsCanonicalAndPayloadSensitive(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	opts := PostOptions{ActorID: "poster", Reason: "sale", CorrelationID: "trace-1", IdempotencyKey: "fingerprint-key"}
	scope, err := BuildIdempotencyScope(entry, opts)
	if err != nil {
		t.Fatalf("BuildIdempotencyScope: %v", err)
	}

	first, err := FingerprintRequest(entry, opts, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest: %v", err)
	}
	again, err := FingerprintRequest(entry.Clone(), opts, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest(clone): %v", err)
	}
	if first == "" || first != again {
		t.Fatalf("fingerprint is not stable: %q vs %q", first, again)
	}

	// Correlation is transport metadata, not an accounting payload field.
	withCorrelation := opts
	withCorrelation.CorrelationID = "trace-2"
	got, err := FingerprintRequest(entry, withCorrelation, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest(correlation): %v", err)
	}
	if got != first {
		t.Fatal("correlation ID must not change the payload fingerprint")
	}

	// A different key is a different operation scope but the same payload.
	otherScope := scope
	otherScope.Key = "other-key"
	got, err = FingerprintRequest(entry, opts, otherScope)
	if err != nil {
		t.Fatalf("FingerprintRequest(scope): %v", err)
	}
	if got != first {
		t.Fatal("idempotency key must not be hashed into the payload fingerprint")
	}

	mutations := []struct {
		name   string
		mutate func(*journal.JournalEntry)
	}{
		{name: "amount", mutate: func(e *journal.JournalEntry) { e.Lines[0].FunctionalDebit = makeMoney(t, "101.00", "USD") }},
		{name: "account", mutate: func(e *journal.JournalEntry) { e.Lines[0].AccountID = types.NewAccountID() }},
		{name: "company", mutate: func(e *journal.JournalEntry) { e.CompanyID = types.NewCompanyID() }},
		{name: "currency", mutate: func(e *journal.JournalEntry) { e.Lines[0].TransactionCurrency = "EUR" }},
		{name: "date", mutate: func(e *journal.JournalEntry) { e.EffectiveDate = e.EffectiveDate.AddDate(0, 0, 1) }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			changed := entry.Clone()
			tc.mutate(&changed)
			got, err := FingerprintRequest(changed, opts, scope)
			if err != nil {
				t.Fatalf("FingerprintRequest: %v", err)
			}
			if got == first {
				t.Fatalf("fingerprint did not change for %s", tc.name)
			}
		})
	}

	changedOpts := opts
	changedOpts.ActorID = "different-poster"
	got, err = FingerprintRequest(entry, changedOpts, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest(actor): %v", err)
	}
	if got == first {
		t.Fatal("fingerprint did not change for actor")
	}
	changedOpts = opts
	changedOpts.Reason = "different-reason"
	got, err = FingerprintRequest(entry, changedOpts, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest(reason): %v", err)
	}
	if got == first {
		t.Fatal("fingerprint did not change for reason")
	}
}

func TestRequestFingerprintIgnoresLineOrder(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	if len(entry.Lines) < 2 {
		t.Fatal("fixture must have at least two lines")
	}
	opts := PostOptions{ActorID: "poster", Reason: "sale", IdempotencyKey: "line-order-key"}
	scope, err := BuildIdempotencyScope(entry, opts)
	if err != nil {
		t.Fatalf("BuildIdempotencyScope: %v", err)
	}
	first, err := FingerprintRequest(entry, opts, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest: %v", err)
	}
	reversed := entry.Clone()
	reversed.Lines[0], reversed.Lines[len(reversed.Lines)-1] = reversed.Lines[len(reversed.Lines)-1], reversed.Lines[0]
	got, err := FingerprintRequest(reversed, opts, scope)
	if err != nil {
		t.Fatalf("FingerprintRequest(reversed): %v", err)
	}
	if got != first {
		t.Fatal("line order must not affect the canonical payload fingerprint")
	}
}

func TestPostEntryStampsIdempotencyContractOnExecutorOptions(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	reader := &mockMasterReaderForPosting{compActive: true, periodOpen: true}
	var delivered PostOptions
	var deliveredEntry journal.JournalEntry
	pipeline := NewPostingPipeline(reader,
		func(_ context.Context, posted journal.JournalEntry, opts PostOptions) (journal.JournalEntry, error) {
			delivered = opts
			deliveredEntry = posted
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)

	opts := PostOptions{
		ActorID:            "poster",
		Reason:             "sale",
		IdempotencyKey:     "phase7-contract-key",
		Scope:              IdempotencyScope{CompanyID: types.NewCompanyID(), Operation: IdempotencyReversal, Key: "spoofed"},
		RequestFingerprint: "spoofed",
	}
	if _, err := pipeline.PostEntry(context.Background(), entry, opts); err != nil {
		t.Fatalf("PostEntry: %v", err)
	}

	wantScope, err := BuildIdempotencyScope(entry, opts)
	if err != nil {
		t.Fatalf("BuildIdempotencyScope: %v", err)
	}
	if delivered.Scope != wantScope {
		t.Fatalf("executor scope = %+v, want %+v", delivered.Scope, wantScope)
	}
	if delivered.Scope.Operation != IdempotencyPosting {
		t.Fatalf("operation = %q, want posting", delivered.Scope.Operation)
	}
	// The executor receives the frozen, stamped copy, whose policy and period
	// fields were filled during validation. The fingerprint must describe that
	// exact snapshot, not the caller's unvalidated draft.
	wantFingerprint, err := FingerprintRequest(deliveredEntry, opts, wantScope)
	if err != nil {
		t.Fatalf("FingerprintRequest: %v", err)
	}
	if delivered.RequestFingerprint == "" || delivered.RequestFingerprint == "spoofed" {
		t.Fatalf("executor fingerprint = %q, want a kernel-computed digest", delivered.RequestFingerprint)
	}
	if delivered.RequestFingerprint != wantFingerprint {
		t.Fatalf("executor fingerprint = %q, want %q", delivered.RequestFingerprint, wantFingerprint)
	}
	wantCallerFingerprint, err := FingerprintCallerPayload(deliveredEntry, opts, wantScope)
	if err != nil {
		t.Fatalf("FingerprintCallerPayload: %v", err)
	}
	if delivered.CallerFingerprint == "" || delivered.CallerFingerprint == "spoofed" {
		t.Fatalf("executor caller fingerprint = %q, want a kernel-computed digest", delivered.CallerFingerprint)
	}
	if delivered.CallerFingerprint != wantCallerFingerprint {
		t.Fatalf("executor caller fingerprint = %q, want %q", delivered.CallerFingerprint, wantCallerFingerprint)
	}
}

func TestTransactionErrorClassification(t *testing.T) {
	nonRetryable := []error{
		ErrIdempotencyConflict,
		ErrReversalAlreadyApplied,
		ErrAtomicityViolation,
	}
	for _, err := range nonRetryable {
		if IsRetryable(err) {
			t.Fatalf("%v must not be retryable", err)
		}
	}
	retryable := []error{
		ErrIdempotencyInProgress,
		ErrIdempotencyStorageUnavailable,
		ErrConcurrencyConflict,
		context.DeadlineExceeded,
		&RetryableError{Err: ErrIdempotencyStorageUnavailable, Reason: "connection lost"},
	}
	for _, err := range retryable {
		if !IsRetryable(err) {
			t.Fatalf("%v must be retryable", err)
		}
	}
	if !errors.Is(&RetryableError{Err: ErrIdempotencyInProgress}, ErrIdempotencyInProgress) {
		t.Fatal("RetryableError must unwrap to its cause")
	}
}

func TestFingerprintRejectsDelimiterInjectionInFreeTextFields(t *testing.T) {
	entry := newPostingTestEntry(t, "creator-1")
	scope := IdempotencyScope{
		CompanyID: entry.CompanyID,
		Operation: IdempotencyPosting,
		Key:       "fx-revaluation|allowed-in-key",
	}
	opts := PostOptions{ActorID: "actor-1", Reason: "valid"}

	if _, err := FingerprintCallerPayload(entry, opts, scope); err != nil {
		t.Fatalf("baseline FingerprintCallerPayload failed with pipe in IdempotencyKey: %v", err)
	}

	malicious := []func(*journal.JournalEntry, *PostOptions){
		func(e *journal.JournalEntry, _ *PostOptions) { e.Description = "ok|entry.posted_by=attacker" },
		func(e *journal.JournalEntry, _ *PostOptions) { e.SourceModule = "billing|bad" },
		func(e *journal.JournalEntry, _ *PostOptions) { e.ReferenceID = "ref\ninjected" },
		func(e *journal.JournalEntry, _ *PostOptions) { e.ReversalReason = "bad\rreason" },
		func(e *journal.JournalEntry, _ *PostOptions) { e.Lines[0].MatchingNumber = "M|1" },
		func(_ *journal.JournalEntry, o *PostOptions) { o.ActorID = "actor|spoof" },
		func(_ *journal.JournalEntry, o *PostOptions) { o.Reason = "reason|spoof" },
	}
	for i, mutate := range malicious {
		e := entry.Clone()
		o := opts
		mutate(&e, &o)
		if _, err := FingerprintCallerPayload(e, o, scope); !errors.Is(err, ErrInvalidEntry) {
			t.Fatalf("case %d: FingerprintCallerPayload error = %v, want ErrInvalidEntry", i, err)
		}
		if _, err := FingerprintRequest(e, o, scope); !errors.Is(err, ErrInvalidEntry) {
			t.Fatalf("case %d: FingerprintRequest error = %v, want ErrInvalidEntry", i, err)
		}
	}
}
