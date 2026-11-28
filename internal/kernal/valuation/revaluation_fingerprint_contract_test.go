package valuation

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func revaluationFingerprintTestEntry(t *testing.T) (journal.JournalEntry, FXJournalAccounts, types.AccountID) {
	t.Helper()
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	account := types.NewAccountID()
	accounts := FXJournalAccounts{
		CompanyID:        types.NewCompanyID(),
		JournalID:        types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(),
		UnrealizedLossID: types.NewAccountID(),
	}
	entry, err := GenerateRevaluationEntryAt(FXResult{
		AccountID:  account,
		Currency:   "USD",
		Difference: mustValuationMoney(t, "500.00", "SAR"),
		IsGain:     true,
	}, accounts, date, date.Add(time.Hour))
	if err != nil {
		t.Fatalf("GenerateRevaluationEntryAt: %v", err)
	}
	return entry, accounts, account
}

// TestRevaluationPayloadFingerprintExportsStableV2Digest pins the exported host
// hook without changing the kernel-authoritative v2 algorithm.
func TestRevaluationPayloadFingerprintExportsStableV2Digest(t *testing.T) {
	entry, _, _ := revaluationFingerprintTestEntry(t)
	got := RevaluationPayloadFingerprint(entry)
	private := revaluationPayloadFingerprint(entry)
	if got != private {
		t.Fatalf("exported fingerprint = %q, want kernel fingerprint %q", got, private)
	}
	if !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
		t.Fatalf("fingerprint = %q, want a sha256 digest", got)
	}

	if again := RevaluationPayloadFingerprint(entry.Clone()); again != got {
		t.Fatalf("fingerprint is not stable across clones: %q vs %q", got, again)
	}
	reordered := entry.Clone()
	if len(reordered.Lines) < 2 {
		t.Fatal("fixture must have two lines")
	}
	reordered.Lines[0], reordered.Lines[1] = reordered.Lines[1], reordered.Lines[0]
	if reorderedFingerprint := RevaluationPayloadFingerprint(reordered); reorderedFingerprint != got {
		t.Fatalf("line order changed the fingerprint: %q vs %q", got, reorderedFingerprint)
	}

	generated := entry.Clone()
	generated.ID = types.NewJournalEntryID()
	generated.Lines[0].ID = types.NewJournalEntryLineID()
	generated.Status = journal.EntryPosted
	generated.CreatedAt = generated.CreatedAt.Add(time.Hour)
	generated.UpdatedAt = generated.UpdatedAt.Add(time.Hour)
	if generatedFingerprint := RevaluationPayloadFingerprint(generated); generatedFingerprint != got {
		t.Fatalf("generation artefacts changed the fingerprint: %q vs %q", got, generatedFingerprint)
	}

	changed := entry.Clone()
	changed.Lines[0].TransactionAmount = mustValuationMoney(t, "600.00", "SAR")
	if changedFingerprint := RevaluationPayloadFingerprint(changed); changedFingerprint == got {
		t.Fatal("fingerprint did not change with the revaluation amount")
	}
}

// TestRevaluationScopeHintRoundTripsThroughPosting proves the G-024 carrier
// works end to end: the valuation scope can be supplied via PostOptions.Scope
// and BuildIdempotencyScope keeps it instead of downgrading it to post.
func TestRevaluationScopeHintRoundTripsThroughPosting(t *testing.T) {
	entry, _, account := revaluationFingerprintTestEntry(t)
	date := entry.EffectiveDate
	scope, err := RevaluationIdempotencyScope(entry.CompanyID, account, "USD", date)
	if err != nil {
		t.Fatalf("RevaluationIdempotencyScope: %v", err)
	}
	opts := posting.PostOptions{IdempotencyKey: scope.Key, Scope: scope}

	built, err := posting.BuildIdempotencyScope(entry, opts)
	if err != nil {
		t.Fatalf("BuildIdempotencyScope: %v", err)
	}
	if built != scope {
		t.Fatalf("scope = %s, want %s", built, scope)
	}
	if built.Operation != types.IdempotencyRevaluation {
		t.Fatalf("operation = %q, want revaluation", built.Operation)
	}

	forged := scope
	forged.CompanyID = types.NewCompanyID()
	if _, err := posting.BuildIdempotencyScope(entry, posting.PostOptions{
		IdempotencyKey: forged.Key,
		Scope:          forged,
	}); !errors.Is(err, types.ErrIdempotencyScopeRequired) {
		t.Fatalf("forged hint error = %v, want ErrIdempotencyScopeRequired", err)
	}
}
