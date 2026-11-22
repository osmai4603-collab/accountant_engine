package posting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

// TestBuildIdempotencyScopeHonorsValidatedRevaluationHint pins the G-024
// carrier: PostOptions.Scope may select the third operation, but the kernel
// still owns the company and key values.
func TestBuildIdempotencyScopeHonorsValidatedRevaluationHint(t *testing.T) {
	base := newPostingTestEntry(t, "creator")
	revaluationKey := "fx-revaluation|company=" + base.CompanyID.String() + "|date=2026-12-31"

	cases := []struct {
		name  string
		entry func() journal.JournalEntry
		opts  func() PostOptions
		want  IdempotencyOperation
	}{
		{
			name:  "valid revaluation hint",
			entry: func() journal.JournalEntry { return base.Clone() },
			opts: func() PostOptions {
				return PostOptions{
					IdempotencyKey: revaluationKey,
					Scope: IdempotencyScope{
						CompanyID: base.CompanyID,
						Operation: IdempotencyRevaluation,
						Key:       "  " + revaluationKey + "  ",
					},
				}
			},
			want: IdempotencyRevaluation,
		},
		{
			name: "valid hint takes precedence over reversal derivation",
			entry: func() journal.JournalEntry {
				entry := base.Clone()
				original := types.NewJournalEntryID()
				entry.ReversalOfID = &original
				return entry
			},
			opts: func() PostOptions {
				return PostOptions{
					IdempotencyKey: revaluationKey,
					Scope: IdempotencyScope{
						CompanyID: base.CompanyID,
						Operation: IdempotencyRevaluation,
						Key:       revaluationKey,
					},
				}
			},
			want: IdempotencyRevaluation,
		},
		{
			name:  "no hint keeps posting derivation",
			entry: func() journal.JournalEntry { return base.Clone() },
			opts: func() PostOptions {
				return PostOptions{IdempotencyKey: "plain-key"}
			},
			want: IdempotencyPosting,
		},
		{
			name: "no hint keeps reversal derivation",
			entry: func() journal.JournalEntry {
				entry := base.Clone()
				original := types.NewJournalEntryID()
				entry.ReversalOfID = &original
				return entry
			},
			opts: func() PostOptions {
				return PostOptions{IdempotencyKey: "reverse-key"}
			},
			want: IdempotencyReversal,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry, opts := tc.entry(), tc.opts()
			got, err := BuildIdempotencyScope(entry, opts)
			if err != nil {
				t.Fatalf("BuildIdempotencyScope: %v", err)
			}
			want := IdempotencyScope{
				CompanyID: entry.CompanyID,
				Operation: tc.want,
				Key:       strings.TrimSpace(opts.IdempotencyKey),
			}
			if got != want {
				t.Fatalf("scope = %s, want %s", got, want)
			}
		})
	}
}

func TestBuildIdempotencyScopeRejectsForgedRevaluationHint(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	key := "fx-revaluation-key"

	cases := []struct {
		name string
		opts PostOptions
	}{
		{
			name: "foreign company in hint",
			opts: PostOptions{
				IdempotencyKey: key,
				Scope: IdempotencyScope{
					CompanyID: types.NewCompanyID(),
					Operation: IdempotencyRevaluation,
					Key:       key,
				},
			},
		},
		{
			name: "different key in hint",
			opts: PostOptions{
				IdempotencyKey: key,
				Scope: IdempotencyScope{
					CompanyID: entry.CompanyID,
					Operation: IdempotencyRevaluation,
					Key:       "another-key",
				},
			},
		},
		{
			name: "hint cannot invent a key",
			opts: PostOptions{
				IdempotencyKey: "  ",
				Scope: IdempotencyScope{
					CompanyID: entry.CompanyID,
					Operation: IdempotencyRevaluation,
					Key:       key,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildIdempotencyScope(entry, tc.opts)
			if !errors.Is(err, ErrIdempotencyScopeRequired) {
				t.Fatalf("error = %v, want ErrIdempotencyScopeRequired", err)
			}
			if got != (IdempotencyScope{}) {
				t.Fatalf("forged hint produced a scope: %+v", got)
			}
		})
	}
}

func TestPostEntryPropagatesRevaluationScopeHintToExecutor(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	opts := PostOptions{
		ActorID:        "poster",
		Reason:         "IAS 21 revaluation",
		IdempotencyKey: "fx-revaluation-pipeline-key",
		Scope: IdempotencyScope{
			CompanyID: entry.CompanyID,
			Operation: IdempotencyRevaluation,
			Key:       "fx-revaluation-pipeline-key",
		},
	}

	var delivered PostOptions
	var deliveredEntry journal.JournalEntry
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(_ context.Context, posted journal.JournalEntry, got PostOptions) (journal.JournalEntry, error) {
			delivered = got
			deliveredEntry = posted
			return posted, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)

	if _, err := pipeline.PostEntry(context.Background(), entry, opts); err != nil {
		t.Fatalf("PostEntry: %v", err)
	}
	wantScope, err := BuildIdempotencyScope(entry, opts)
	if err != nil {
		t.Fatalf("BuildIdempotencyScope: %v", err)
	}
	if wantScope.Operation != IdempotencyRevaluation {
		t.Fatalf("test setup: operation = %q, want revaluation", wantScope.Operation)
	}
	if delivered.Scope != wantScope {
		t.Fatalf("executor scope = %s, want %s", delivered.Scope, wantScope)
	}
	wantFingerprint, err := FingerprintRequest(deliveredEntry, opts, wantScope)
	if err != nil {
		t.Fatalf("FingerprintRequest: %v", err)
	}
	if delivered.RequestFingerprint != wantFingerprint {
		t.Fatalf("executor fingerprint = %q, want %q", delivered.RequestFingerprint, wantFingerprint)
	}
}

func TestPostEntryRejectsForgedRevaluationScopeHint(t *testing.T) {
	entry := newPostingTestEntry(t, "creator")
	opts := PostOptions{
		ActorID:        "poster",
		IdempotencyKey: "fx-revaluation-key",
		Scope: IdempotencyScope{
			CompanyID: types.NewCompanyID(),
			Operation: IdempotencyRevaluation,
			Key:       "fx-revaluation-key",
		},
	}
	called := false
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(context.Context, journal.JournalEntry, PostOptions) (journal.JournalEntry, error) {
			called = true
			return journal.JournalEntry{}, nil
		},
		WithPolicyEngine(testPostingPolicyEngine(t, entry.CompanyID, "USD")),
	)
	if _, err := pipeline.PostEntry(context.Background(), entry, opts); !errors.Is(err, ErrIdempotencyScopeRequired) {
		t.Fatalf("error = %v, want ErrIdempotencyScopeRequired", err)
	}
	if called {
		t.Fatal("executor called for a forged revaluation scope")
	}
}
