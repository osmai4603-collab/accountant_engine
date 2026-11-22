package contract_parity

import (
	"context"
	"errors"
	"sync"
	"testing"

	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// phase11RevaluationCommit models the host stamping a kernel-generated
// revaluation scope and its v2 payload digest onto a durable posting commit.
// The payload hashes are opaque at the SPI boundary; valuation owns their
// canonical construction.
func phase11RevaluationCommit(f UnifiedFixture, key, payloadHash string) storage.PostingCommit {
	commit := phase7PostingCommit(f, key, payloadHash)
	commit.Entry.EntryNumber = "RV11-" + key
	commit.Entry.Classification = journal.EntryClassAdjusting
	commit.Entry.AdjustingEntryType = journal.AdjustingEntryAccrual
	commit.Entry.Description = "phase 11 IAS 21 revaluation"
	commit.Entry.ReferenceID = "FX-REVAL-" + key
	commit.Operation = storage.OperationRevaluation
	commit.ActorID = "phase11-revaluation-tester"
	commit.Reason = "IAS 21 revaluation contract test"
	commit.CorrelationID = "cor-phase11-" + key
	commit.Idempotency = &storage.PostingIdempotency{
		Key:         key,
		Scope:       phase7Scope(f.Company.ID, string(storage.OperationRevaluation), key),
		PayloadHash: payloadHash,
	}
	return commit
}

func phase11RequireAuditRepository(t *testing.T, h *StorageAdapterHarness) storage.AuditConformanceRepository {
	t.Helper()
	raw, ok := h.Ports.Private[storage.AuditConformanceExtensionKey]
	if !ok {
		t.Fatalf("%s adapter does not expose the audit conformance repository", h.Name)
	}
	repo, ok := raw.(storage.AuditConformanceRepository)
	if !ok {
		t.Fatalf("%s audit conformance extension has the wrong type", h.Name)
	}
	return repo
}

func TestPhase11_RevaluationReplayWithSamePayload(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationReplaySamePayload", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		lookup := phase7RequireLookup(t, h)
		commit := phase11RevaluationCommit(f, "revaluation-replay-key", "fx-revaluation-payload-v2:base")

		first, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("first revaluation commit: %v", err)
		}
		replay, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("revaluation replay: %v", err)
		}
		if first.ID != replay.ID {
			t.Fatalf("replay created a second revaluation: %s vs %s", first.ID, replay.ID)
		}
		phase7RequireBalance(t, ctx, h, f, "100")

		record, found, err := lookup.Lookup(ctx, f.Company.ID, commit.Idempotency.Key)
		if err != nil || !found {
			t.Fatalf("lookup revaluation key: found=%v err=%v", found, err)
		}
		if record.Status != storage.IdempotencyCompleted || record.EntryID == nil || *record.EntryID != first.ID {
			t.Fatalf("unexpected revaluation idempotency record: %+v", record)
		}
		if record.Operation != string(storage.OperationRevaluation) || record.Scope != commit.Idempotency.Scope ||
			record.PayloadHash != commit.Idempotency.PayloadHash {
			t.Fatalf("durable revaluation scope/hash mismatch: %+v", record)
		}
	})
}

func TestPhase11_RevaluationPayloadConflictIsTerminal(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationPayloadConflict", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		commit := phase11RevaluationCommit(f, "revaluation-conflict-key", "fx-revaluation-payload-v2:base")
		first, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("first revaluation commit: %v", err)
		}

		conflicting := phase11RevaluationCommit(f, commit.Idempotency.Key, "fx-revaluation-payload-v2:different")
		if _, err := h.Ports.Posting.CommitPosting(ctx, conflicting); !errors.Is(err, storage.ErrIdempotencyPayloadConflict) {
			t.Fatalf("payload conflict error = %v, want ErrIdempotencyPayloadConflict", err)
		}
		phase7RequireBalance(t, ctx, h, f, "100")

		replay, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil || replay.ID != first.ID {
			t.Fatalf("original payload could not replay after conflict: entry=%+v err=%v", replay, err)
		}
	})
}

func TestPhase11_RevaluationConcurrentReplayHasSingleWinner(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationConcurrentSingleWinner", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		commit := phase11RevaluationCommit(f, "revaluation-concurrent-key", "fx-revaluation-payload-v2:concurrent")

		const workers = 8
		var wg sync.WaitGroup
		results := make(chan journal.JournalEntry, workers)
		errs := make(chan error, workers)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
				if err != nil {
					errs <- err
					return
				}
				results <- posted
			}()
		}
		wg.Wait()
		close(results)
		close(errs)

		for err := range errs {
			t.Fatalf("concurrent revaluation commit failed: %v", err)
		}
		var winner *journal.JournalEntry
		for posted := range results {
			if winner == nil {
				copied := posted
				winner = &copied
				continue
			}
			if winner.ID != posted.ID {
				t.Fatalf("concurrent replay returned different revaluations: %s vs %s", winner.ID, posted.ID)
			}
		}
		if winner == nil {
			t.Fatal("no concurrent revaluation commit succeeded")
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

func TestPhase11_RevaluationFaultAfterCommitLookupAndReplay(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationFaultAfterCommitReplay", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		lookup := phase7RequireLookup(t, h)
		commit := phase11RevaluationCommit(f, "revaluation-lost-response-key", "fx-revaluation-payload-v2:lost-response")

		lostCtx := storage.WithFaultInjector(ctx, func(point storage.FaultPoint) error {
			if point == storage.FaultAfterCommit {
				return errors.New("lost response after revaluation commit")
			}
			return nil
		})
		if _, err := h.Ports.Posting.CommitPosting(lostCtx, commit); err == nil {
			t.Fatal("expected unknown-outcome error after revaluation commit")
		}

		record, found, err := lookup.Lookup(ctx, f.Company.ID, commit.Idempotency.Key)
		if err != nil || !found {
			t.Fatalf("lookup after unknown outcome: found=%v err=%v", found, err)
		}
		if record.Status != storage.IdempotencyCompleted || record.EntryID == nil {
			t.Fatalf("revaluation did not commit durably: %+v", record)
		}
		if record.Operation != string(storage.OperationRevaluation) || record.Scope != commit.Idempotency.Scope ||
			record.PayloadHash != commit.Idempotency.PayloadHash {
			t.Fatalf("durable unknown-outcome record mismatch: %+v", record)
		}

		replay, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("replay after unknown outcome: %v", err)
		}
		if replay.ID != *record.EntryID {
			t.Fatalf("replay entry %s != stored entry %s", replay.ID, *record.EntryID)
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

func TestPhase11_RevaluationScopeIsSeparateFromPostAndReverse(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationScopeSeparation", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		const sharedKey = "shared-scope-key"

		post := phase7PostingCommit(f, sharedKey, "payload-post")
		posted, err := h.Ports.Posting.CommitPosting(ctx, post)
		if err != nil {
			t.Fatalf("commit post: %v", err)
		}
		reverse := phase7ReversalCommit(f, posted, sharedKey, "payload-reverse")
		reversed, err := h.Ports.Posting.CommitPosting(ctx, reverse)
		if err != nil {
			t.Fatalf("commit reverse: %v", err)
		}
		revaluation := phase11RevaluationCommit(f, sharedKey, "payload-revaluation")
		// The adapters derive the durable move number from SequenceNumber;
		// keep it distinct from the post (1) and reverse (2) in this scenario.
		revaluation.Entry.SequenceNumber = 3
		revalued, err := h.Ports.Posting.CommitPosting(ctx, revaluation)
		if err != nil {
			t.Fatalf("commit revaluation with a post/reverse key: %v", err)
		}

		if posted.ID == reversed.ID || posted.ID == revalued.ID || reversed.ID == revalued.ID {
			t.Fatalf("operations shared an entry: post=%s reverse=%s revaluation=%s", posted.ID, reversed.ID, revalued.ID)
		}
		for _, tc := range []struct {
			name   string
			commit storage.PostingCommit
			want   types.JournalEntryID
		}{
			{name: "post", commit: post, want: posted.ID},
			{name: "reverse", commit: reverse, want: reversed.ID},
			{name: "revaluation", commit: revaluation, want: revalued.ID},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := h.Ports.Posting.CommitPosting(ctx, tc.commit)
				if err != nil {
					t.Fatalf("replay: %v", err)
				}
				if got.ID != tc.want {
					t.Fatalf("replay entry %s != operation entry %s", got.ID, tc.want)
				}
			})
		}
		// The reversal removes the original posting's balance and the
		// revaluation adds its own 100 back.
		phase7RequireBalance(t, ctx, h, f, "0")
	})
}

func TestPhase11_RevaluationCompanyScopeIsolation(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationCompanyScopeIsolation", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		lookup := phase7RequireLookup(t, h)
		commit := phase11RevaluationCommit(f, "revaluation-company-key", "fx-revaluation-payload-v2:isolated")
		if _, err := h.Ports.Posting.CommitPosting(ctx, commit); err != nil {
			t.Fatalf("commit: %v", err)
		}

		otherCompany := types.NewCompanyID()
		if record, found, err := lookup.Lookup(ctx, otherCompany, commit.Idempotency.Key); err != nil || found {
			t.Fatalf("cross-company lookup leaked record: found=%v record=%+v err=%v", found, record, err)
		}

		forged := phase11RevaluationCommit(f, commit.Idempotency.Key, commit.Idempotency.PayloadHash)
		forged.Entry.CompanyID = otherCompany
		if _, err := h.Ports.Posting.CommitPosting(ctx, forged); !errors.Is(err, storage.ErrScopeViolation) {
			t.Fatalf("cross-company commit error = %v, want ErrScopeViolation", err)
		}
		if _, err := h.Ports.Entries.GetEntry(ctx, otherCompany, forged.Entry.ID); err == nil {
			t.Fatal("cross-company revaluation was persisted")
		}
		phase7RequireBalance(t, ctx, h, f, "100")
	})
}

func TestPhase11_RevaluationAuditAction(t *testing.T) {
	ForEachStorageAdapter(t, "RevaluationAuditAction", func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture) {
		ctx := context.Background()
		repo := phase11RequireAuditRepository(t, h)
		commit := phase11RevaluationCommit(f, "revaluation-audit-key", "fx-revaluation-payload-v2:audit")
		posted, err := h.Ports.Posting.CommitPosting(ctx, commit)
		if err != nil {
			t.Fatalf("commit: %v", err)
		}

		found := false
		if err := repo.IterateAuditLogs(ctx, f.Company.ID, func(log publicgovernance.AuditLog) error {
			if log.EntityID != posted.ID.String() {
				return nil
			}
			found = true
			if log.Action != string(storage.OperationRevaluation) {
				t.Errorf("audit action = %q, want %q", log.Action, storage.OperationRevaluation)
			}
			if log.CorrelationID != commit.CorrelationID {
				t.Errorf("audit correlation = %q, want %q", log.CorrelationID, commit.CorrelationID)
			}
			return nil
		}); err != nil {
			t.Fatalf("iterate audit logs: %v", err)
		}
		if !found {
			t.Fatalf("no audit evidence for revaluation entry %s", posted.ID)
		}
	})
}
