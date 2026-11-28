package sdk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	publicsubledger "github.com/osmai4603-collab/accounting_engine/pkg/spi/subledger"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

func TestSDKOptionsServicesAndLedgerLifecycle(t *testing.T) {
	ctx := context.Background()
	adapter := memory.New()
	ports := adapter.Ports()

	// Build with disabled reporting/governance first to cover disabled stubs.
	disabledEngine, err := sdk.NewBuilder(
		sdk.WithStoragePorts(ports),
		sdk.WithPolicyCacheTTL(5*time.Second),
		sdk.WithAllowSyntheticDefaultJournal(),
		sdk.WithEventIdempotencyWaitPolicy(10*time.Millisecond, time.Millisecond),
	).Build(ctx)
	if err != nil {
		t.Fatalf("Build disabled engine: %v", err)
	}
	defer disabledEngine.Close()

	if disabledEngine.Events() == nil || disabledEngine.Scheduler() == nil {
		t.Fatal("expected non-nil Events and Scheduler")
	}
	if disabledEngine.Standards().Registry() == nil || disabledEngine.Standards().Scheduler() == nil {
		t.Fatal("expected non-nil Standards Registry and Scheduler")
	}
	_ = disabledEngine.Standards().HasCapability(standards.CapCurrencyTranslation)

	if _, err := disabledEngine.Reporting().GetTrialBalance(ctx, types.NewCompanyID(), time.Now(), "USD"); err == nil {
		t.Fatal("expected disabled reporting GetTrialBalance error")
	}
	if _, err := disabledEngine.Reporting().GetBalanceSheet(ctx, types.NewCompanyID(), time.Now(), "USD"); err == nil {
		t.Fatal("expected disabled reporting GetBalanceSheet error")
	}
	if _, err := disabledEngine.Reporting().GetIncomeStatement(ctx, types.NewCompanyID(), time.Now(), time.Now(), "USD"); err == nil {
		t.Fatal("expected disabled reporting GetIncomeStatement error")
	}
	if _, err := disabledEngine.Reporting().GetGeneralLedger(ctx, types.NewCompanyID(), types.NewAccountID(), time.Now(), time.Now()); err == nil {
		t.Fatal("expected disabled reporting GetGeneralLedger error")
	}
	if _, err := disabledEngine.Reporting().GetDisclosures(ctx, reporting.DisclosureQuery{}); err == nil {
		t.Fatal("expected disabled reporting GetDisclosures error")
	}
	if _, err := disabledEngine.Governance().GetApprovalFingerprint(ctx, types.NewJournalEntryID()); err == nil {
		t.Fatal("expected disabled governance GetApprovalFingerprint error")
	}
	if _, err := sdk.SubledgerARAP(disabledEngine); !errors.Is(err, types.ErrCapabilityNotImplemented) {
		t.Fatalf("expected ErrCapabilityNotImplemented for SubledgerARAP, got %v", err)
	}
	if _, err := sdk.Consolidation(disabledEngine); err != nil {
		t.Fatalf("expected Consolidation service, got %v", err)
	}
	if _, err := sdk.Reconciliation(disabledEngine); err != nil {
		t.Fatalf("expected Reconciliation service, got %v", err)
	}

	// Build with nil option validation checks
	if _, err := sdk.NewBuilder(sdk.WithReporting(nil)).Build(ctx); err == nil {
		t.Fatal("expected WithReporting(nil) error")
	}
	if _, err := sdk.NewBuilder(sdk.WithGovernance(nil)).Build(ctx); err == nil {
		t.Fatal("expected WithGovernance(nil) error")
	}
	if _, err := sdk.NewBuilder(sdk.WithPolicyCacheTTL(-1)).Build(ctx); err == nil {
		t.Fatal("expected WithPolicyCacheTTL(-1) error")
	}
	if _, err := sdk.NewBuilder(sdk.WithEventIdempotencyWaitPolicy(-1, 0)).Build(ctx); err == nil {
		t.Fatal("expected WithEventIdempotencyWaitPolicy negative timeout error")
	}
	if _, err := sdk.NewBuilder(sdk.WithSubledgerARAP(publicsubledger.ARAPStores{})).Build(ctx); err == nil {
		t.Fatal("expected WithSubledgerARAP empty stores error")
	}

	// Provision company, journal, accounts, fiscal year & period
	companyID := types.NewCompanyID()
	journalID := masterdata.NewJournalID()
	cashID := types.NewAccountID()
	revID := types.NewAccountID()
	fyID := types.FiscalYearID(uuid.New())
	fpID := types.FiscalPeriodID(uuid.New())
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	_ = adapter.MasterDataStore().CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "SDK-1", LegalName: "SDK Co", FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	})
	_ = adapter.MasterDataStore().CreateJournal(ctx, masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"})
	_ = adapter.MasterDataStore().CreateAccount(ctx, masterdata.Account{ID: cashID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true})
	_ = adapter.MasterDataStore().CreateAccount(ctx, masterdata.Account{ID: revID, CompanyID: companyID, Code: "4000", Name: "Revenue", Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true})
	_ = adapter.MasterDataStore().CreateFiscalYear(ctx, masterdata.FiscalYear{ID: fyID, CompanyID: companyID, Name: "FY26", StartDate: start, EndDate: end, State: masterdata.FiscalYearOpen})
	_ = adapter.MasterDataStore().CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{ID: fpID, CompanyID: companyID, FiscalYearID: fyID, Number: 1, Name: "P1", StartDate: start, EndDate: end, State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal})

	amt, _ := types.NewMoneyFromString("250.00", "USD")
	zero, _ := types.NewMoneyFromString("0.00", "USD")
	entry := journal.JournalEntry{
		ID:                  types.NewJournalEntryID(),
		CompanyID:           companyID,
		JournalID:           journalID,
		FiscalPeriodID:      fpID,
		EffectiveDate:       start.Add(24 * time.Hour),
		PostingDate:         start.Add(24 * time.Hour),
		TransactionCurrency: "USD",
		ExchangeRate:        decimal.NewFromInt(1),
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: cashID, TransactionAmount: amt, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: revID, TransactionAmount: amt, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amt},
		},
	}

	postCtx := publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		IdempotencyKey: "sdk-test-1",
		CorrelationID:  "corr-sdk-1",
	})
	posted, err := disabledEngine.Ledger().PostEntry(postCtx, entry)
	if err != nil {
		t.Fatalf("PostEntry: %v", err)
	}
	if _, err := disabledEngine.Ledger().GetEntry(ctx, companyID, posted.ID); err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	revCtx := publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		IdempotencyKey: "sdk-rev-1",
	})
	if _, err := disabledEngine.Ledger().ReverseEntry(revCtx, companyID, posted.ID, "correction"); err != nil {
		t.Fatalf("ReverseEntry: %v", err)
	}

	// Nil typed dependency check via empty storage.Ports
	var nilPorts storage.Ports
	if _, err := sdk.NewBuilder(sdk.WithStoragePorts(nilPorts)).Build(ctx); err == nil {
		t.Fatal("expected error with empty storage ports")
	}
}
