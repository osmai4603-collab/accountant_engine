package externalconsumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/consolidation"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/reconciliation"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

func TestEnterpriseOrbitalAndLifecycleContracts(t *testing.T) {
	ctx := context.Background()
	drv := memory.New()
	md := drv.MasterDataStore()

	companyID := types.NewCompanyID()
	if err := md.CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: "EXT-ENT", LegalName: "External Enterprise Corp",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}
	journalID := masterdata.NewJournalID()
	if err := md.CreateJournal(ctx, masterdata.Journal{ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General"}); err != nil {
		t.Fatalf("create journal: %v", err)
	}
	cashID := types.NewAccountID()
	revID := types.NewAccountID()
	retID := types.NewAccountID()
	for _, acc := range []masterdata.Account{
		{ID: cashID, CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true},
		{ID: revID, CompanyID: companyID, Code: "4000", Name: "Revenue", Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true},
		{ID: retID, CompanyID: companyID, Code: "3000", Name: "Retained Earnings", Type: masterdata.AccountTypeEquity, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true, SubClassification: "retained_earnings"},
	} {
		if err := md.CreateAccount(ctx, acc); err != nil {
			t.Fatalf("create account: %v", err)
		}
	}

	fy2025 := types.NewFiscalYearID()
	start2025 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end2025 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	if err := md.CreateFiscalYear(ctx, masterdata.FiscalYear{ID: fy2025, CompanyID: companyID, Name: "FY2025", StartDate: start2025, EndDate: end2025, State: masterdata.FiscalYearOpen}); err != nil {
		t.Fatalf("create fy2025: %v", err)
	}
	for m := 1; m <= 12; m++ {
		mStart := time.Date(2025, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		mEnd := mStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
		_ = md.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
			ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: fy2025,
			Number: m, Name: mStart.Format("Jan"), StartDate: mStart, EndDate: mEnd,
			State: masterdata.PeriodHardClosed, Type: masterdata.FiscalPeriodTypeNormal,
		})
	}
	_ = md.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: fy2025,
		Number: 13, Name: "Adj 13", StartDate: end2025, EndDate: end2025,
		State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeYearEndAdjustment,
	})

	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(drv.Ports())).Build(ctx)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

	// 1. DraftCancellation from external consumer
	appSvc, err := sdk.NewApplicationService(drv.Ports())
	if err != nil {
		t.Fatalf("NewApplicationService: %v", err)
	}
	appCtx := application.Context{Principal: application.Principal{ActorID: "ext-user", CompanyIDs: []types.CompanyID{companyID}}}
	amt100, _ := types.NewMoneyFromString("100.00", "USD")
	zeroUSD, _ := types.NewMoneyFromString("0", "USD")
	draft, err := appSvc.CreateDraft(ctx, application.CreateDraftCommand{
		Context: appCtx, CompanyID: companyID, JournalID: journalID, EffectiveDate: end2025,
		Description: "Ext draft", TransactionCurrency: "USD", ExchangeRate: "1",
		Classification: journal.EntryClassAdjusting, TargetPeriodNumber: 13,
		AdjustingEntryType: journal.AdjustingEntryAccrual, AdjustmentReason: "Ext accrual",
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: cashID, TransactionAmount: amt100, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt100, FunctionalCredit: zeroUSD},
			{ID: types.NewJournalEntryLineID(), AccountID: revID, TransactionAmount: amt100, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amt100},
		},
	})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	canceller, err := sdk.DraftCancellation(engine)
	if err != nil {
		t.Fatalf("sdk.DraftCancellation: %v", err)
	}
	cancelled, err := canceller.CancelDraft(ctx, application.CancelDraftCommand{
		Context: appCtx, CompanyID: companyID, EntryID: draft.Entry.ID, Reason: "external cancel",
	})
	if err != nil || cancelled.Entry.Status != journal.EntryCancelled {
		t.Fatalf("CancelDraft = %+v, %v", cancelled, err)
	}

	// 2. YearEndClosing from external consumer
	yec, err := sdk.YearEndClosing(engine)
	if err != nil {
		t.Fatalf("sdk.YearEndClosing: %v", err)
	}
	if _, err := yec.ExecuteYearEndClosing(ctx, application.ExecuteYearEndClosingCommand{
		Context: appCtx, CompanyID: companyID, FiscalYearID: fy2025, JournalID: journalID,
		RetainedEarningsAccountID: retID, Policy: application.YearEndClosingDirectToRetainedEarnings,
		Currency: "USD", Reason: "Ext year-end close",
	}); err != nil {
		t.Fatalf("ExecuteYearEndClosing: %v", err)
	}

	// 3. Consolidation from external consumer
	consSvc, err := sdk.Consolidation(engine)
	if err != nil {
		t.Fatalf("sdk.Consolidation: %v", err)
	}
	entityID := types.NewCompanyID().String()
	consRes, err := consSvc.Consolidate(ctx, consolidation.ConsolidationRequest{
		GroupID: "GRP-1", ParentEntityID: entityID, GroupCurrency: "USD",
		StartDate: start2025, EndDate: end2025,
		Entities: []consolidation.Entity{{ID: entityID, FunctionalCurrency: "USD"}},
		GroupAccounts: []consolidation.GroupAccount{
			{ID: "G-CASH", Name: "Cash", Type: consolidation.AccountAsset},
			{ID: "G-EQ", Name: "Equity", Type: consolidation.AccountEquity},
		},
		AccountMappings: []consolidation.AccountMapping{
			{EntityID: entityID, SourceAccountID: "1000", GroupAccountID: "G-CASH", RateClass: consolidation.RateClosing},
			{EntityID: entityID, SourceAccountID: "3000", GroupAccountID: "G-EQ", RateClass: consolidation.RateHistorical, HistoricalDate: start2025},
		},
		TrialBalances: []consolidation.TrialBalance{{
			EntityID: entityID, Currency: "USD", StartDate: start2025, EndDate: end2025,
			Lines: []consolidation.TrialBalanceLine{
				{AccountID: "1000", Balance: decimal.NewFromInt(500)},
				{AccountID: "3000", Balance: decimal.NewFromInt(-500)},
			},
		}},
	})
	if err != nil || !consRes.Totals.Assets.Equal(decimal.NewFromInt(500)) {
		t.Fatalf("Consolidate = %+v, %v", consRes, err)
	}

	// 4. Reconciliation from external consumer
	recSvc, err := sdk.Reconciliation(engine)
	if err != nil {
		t.Fatalf("sdk.Reconciliation: %v", err)
	}
	recRes, err := recSvc.ReconcileRecords(ctx, reconciliation.ReconciliationRequest{
		LeftSourceName: "Bank", RightSourceName: "GL", Currency: "USD",
		StartDate: start2025, EndDate: end2025, Tolerance: decimal.Zero,
		LeftRecords:  []reconciliation.Record{{Key: "TX-1", Amount: decimal.NewFromInt(100), Currency: "USD"}},
		RightRecords: []reconciliation.Record{{Key: "TX-1", Amount: decimal.NewFromInt(100), Currency: "USD"}},
	})
	if err != nil || !recRes.Balanced || len(recRes.Matched) != 1 {
		t.Fatalf("ReconcileRecords = %+v, %v", recRes, err)
	}
}
