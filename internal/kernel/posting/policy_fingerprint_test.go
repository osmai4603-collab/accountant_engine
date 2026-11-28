package posting

import (
	"context"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

func phase5TestEntry(t *testing.T, companyID types.CompanyID, journalID types.JournalID, date time.Time) journal.JournalEntry {
	t.Helper()
	zero := makeMoney(t, "0", "USD")
	amount := makeMoney(t, "100.00", "USD")
	entry, err := NewJournalEntry(companyID, journalID, date, "creator", []journal.EntryLine{
		{
			AccountID: types.NewAccountID(), TransactionCurrency: "USD", TransactionAmount: amount,
			ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero,
		},
		{
			AccountID: types.NewAccountID(), TransactionCurrency: "USD", TransactionAmount: amount,
			ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

// TestPostEntryStampsPolicyFingerprintAndKeepsHistoryStable proves that a later
// policy version changes future postings without rewriting the audit evidence
// recorded on an entry that was already posted under the earlier version.
func TestPostEntryStampsPolicyFingerprintAndKeepsHistoryStable(t *testing.T) {
	ctx := context.Background()
	companyID, journalID := types.NewCompanyID(), types.NewJournalID()
	engine := testPostingPolicyEngine(t, companyID, "USD")
	pipeline := NewPostingPipeline(
		&mockMasterReaderForPosting{compActive: true, periodOpen: true},
		func(_ context.Context, posted journal.JournalEntry, _ PostOptions) (journal.JournalEntry, error) {
			if posted.PolicyFingerprint == "" {
				t.Error("executor received an entry without a policy fingerprint")
			}
			return posted, nil
		},
		WithPolicyEngine(engine),
	)

	historicalDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	post := func(date time.Time, key string) journal.JournalEntry {
		t.Helper()
		posted, err := pipeline.PostEntry(ctx, phase5TestEntry(t, companyID, journalID, date), PostOptions{
			ActorID: "poster", IdempotencyKey: key,
		})
		if err != nil {
			t.Fatalf("PostEntry(%s): %v", date, err)
		}
		return posted
	}

	first := post(historicalDate, "policy-phase5-first")
	if first.PolicyFingerprint == "" {
		t.Fatal("posted entry has no policy fingerprint")
	}
	firstFingerprint := first.PolicyFingerprint

	changeDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := engine.SetPolicy(policy.PostingPolicy{
		CompanyID: companyID, EffectiveDate: changeDate, MaxBackdatingDays: 365,
	}); err != nil {
		t.Fatal(err)
	}
	if err := engine.SetCurrencyPolicy(policy.CurrencyPolicy{
		CompanyID: companyID, EffectiveDate: changeDate, FunctionalCurrency: "USD",
		PresentationCurrency: "EUR", RoundingMethod: policy.RoundingHalfUp,
	}); err != nil {
		t.Fatal(err)
	}

	stillHistorical := post(historicalDate, "policy-phase5-historical")
	if stillHistorical.PolicyFingerprint != firstFingerprint {
		t.Fatalf("historical posting fingerprint changed: %q -> %q", firstFingerprint, stillHistorical.PolicyFingerprint)
	}
	if first.PolicyFingerprint != firstFingerprint || first.Status != journal.EntryPosted {
		t.Fatal("previously posted entry was mutated")
	}

	future := post(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "policy-phase5-future")
	if future.PolicyFingerprint == firstFingerprint {
		t.Fatal("new policy version produced the same fingerprint as the historical version")
	}
}
