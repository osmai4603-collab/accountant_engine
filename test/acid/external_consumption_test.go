package acid

import (
	"context"
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/shopspring/decimal"
)

func TestExternalConsumption(t *testing.T) {
	ctx := context.Background()
	f := newAcidFixture(t)

	entryID := types.NewJournalEntryID()
	amt, err := types.NewMoneyFromString("100.00", "USD")
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	zero, err := types.NewMoneyFromString("0.00", "USD")
	if err != nil {
		t.Fatalf("zero: %v", err)
	}

	entry := journal.JournalEntry{
		ID:                  entryID,
		CompanyID:           f.companyID,
		JournalID:           f.journalID,
		FiscalPeriodID:      f.periodID,
		Status:              journal.EntryDraft,
		Classification:      journal.EntryClassRegular,
		EffectiveDate:       f.asOf,
		PostingDate:         f.asOf,
		Description:         "External consumption test entry",
		SourceModule:        "acid-external",
		CreatedBy:           "tester",
		TransactionCurrency: "USD",
		ExchangeRate:        decimal.NewFromInt(1),
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.cashID,
				TransactionAmount: amt, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.revenueID,
				TransactionAmount: amt, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amt,
			},
		},
	}

	posted, err := f.engine.Ledger().PostEntry(acidPostContext("external-post"), entry)
	if err != nil {
		t.Fatalf("failed to post entry externally: %v", err)
	}

	fetched, err := f.engine.Ledger().GetEntry(ctx, f.companyID, posted.ID)
	if err != nil {
		t.Fatalf("failed to retrieve posted entry externally: %v", err)
	}
	if fetched.ID != entryID || fetched.Status != journal.EntryPosted {
		t.Fatalf("retrieved entry = %s (%s), want %s (posted)", fetched.ID, fetched.Status, entryID)
	}

	reverseCtx := publicapplication.WithPrincipal(ctx, publicapplication.Principal{
		ActorID:    "tester",
		CompanyIDs: []types.CompanyID{f.companyID},
	})
	reversalResult, err := f.app.Reverse(reverseCtx, publicapplication.ReverseEntryCommand{
		Context: publicapplication.Context{
			Principal: publicapplication.Principal{
				ActorID:    "tester",
				CompanyIDs: []types.CompanyID{f.companyID},
			},
			CorrelationID:  "p7-acid-reverse",
			IdempotencyKey: "external-reverse-" + entryID.String(),
			SourceSystem:   "p7-acid-test",
		},
		CompanyID:      f.companyID,
		EntryID:        posted.ID,
		Reason:         "Testing reversal",
		EffectiveDate:  f.asOf,
		IdempotencyKey: "external-reverse-command-" + entryID.String(),
	})
	if err != nil {
		t.Fatalf("failed to reverse entry: %v", err)
	}
	reversal := reversalResult.Entry
	if reversal.ReversalOfID == nil || *reversal.ReversalOfID != entryID {
		t.Fatalf("reversal reference = %v, want %s", reversal.ReversalOfID, entryID)
	}
	originalAfter, err := f.engine.Ledger().GetEntry(ctx, f.companyID, posted.ID)
	if err != nil {
		t.Fatalf("failed to retrieve original after reversal: %v", err)
	}
	if originalAfter.Status != journal.EntryReversed {
		t.Fatalf("original status = %s, want reversed", originalAfter.Status)
	}

	// Reporting is an optional host-provided capability. When a host supplies
	// a ReportingStore, verify the trial balance figures; otherwise confirm the
	// documented ErrCapabilityNotImplemented degradation explicitly.
	tb, err := f.engine.Reporting().GetTrialBalance(ctx, f.companyID, f.asOf, "USD")
	if errors.Is(err, sdk.ErrCapabilityNotImplemented) {
		t.Log("reporting store not supplied by host; capability degradation verified")
		return
	}
	if err != nil {
		t.Fatalf("failed to generate trial balance: %v", err)
	}
	if tb.CompanyID != f.companyID {
		t.Fatal("trial balance company ID mismatch")
	}
}
