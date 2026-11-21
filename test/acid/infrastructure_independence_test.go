package acid

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	"github.com/shopspring/decimal"
)

func TestInfrastructureIndependence(t *testing.T) {
	ctx := context.Background()
	f := newAcidFixture(t)

	var wg sync.WaitGroup
	const numEntries = 200
	failures := make(chan error, numEntries)
	for i := 0; i < numEntries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			amt, err := types.NewMoneyFromString("50.00", "USD")
			if err != nil {
				failures <- err
				return
			}
			zero, err := types.NewMoneyFromString("0.00", "USD")
			if err != nil {
				failures <- err
				return
			}
			entry := journal.JournalEntry{
				ID: types.NewJournalEntryID(), CompanyID: f.companyID,
				JournalID: f.journalID, FiscalPeriodID: f.periodID,
				Status: journal.EntryDraft, Classification: journal.EntryClassRegular,
				EffectiveDate: f.asOf, PostingDate: f.asOf,
				Description: "Concurrent memory store test", SourceModule: "acid-infra",
				CreatedBy: "tester", TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1),
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
			if _, err := f.engine.Ledger().PostEntry(acidPostContext("infra"), entry); err != nil {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent posting failed: %v", err)
	}
	if t.Failed() {
		t.Fatal("expected all concurrent posts through public ports to succeed")
	}

	tb, err := f.engine.Reporting().GetTrialBalance(ctx, f.companyID, f.asOf, "USD")
	if errors.Is(err, sdk.ErrCapabilityNotImplemented) {
		t.Log("reporting store not supplied by host; capability degradation verified")
		return
	}
	if err != nil {
		t.Fatalf("failed to retrieve trial balance after concurrent load: %v", err)
	}
	if tb.CompanyID != f.companyID {
		t.Fatal("trial balance company ID mismatch")
	}
}
