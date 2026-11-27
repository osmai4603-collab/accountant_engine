package sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/shopspring/decimal"
)

func setupPhase2TestFixture(t *testing.T) (
	*memory.Adapter,
	sdk.Engine,
	publicapplication.Service,
	types.CompanyID,
	types.JournalID,
	types.FiscalYearID,
	types.FiscalYearID,
	map[string]types.AccountID,
) {
	t.Helper()
	ctx := context.Background()
	adapter := memory.New()
	store := adapter.MasterDataStore()

	companyID := types.CompanyID(uuid.New())
	if err := store.CreateCompany(ctx, masterdata.Company{
		ID:                   companyID,
		Code:                 "PH2-" + uuid.NewString()[:6],
		LegalName:            "Phase 2 Test Corp",
		FunctionalCurrency:   "USD",
		PresentationCurrency: "USD",
		Active:               true,
	}); err != nil {
		t.Fatalf("create company: %v", err)
	}

	journalID := masterdata.NewJournalID()
	if err := store.CreateJournal(ctx, masterdata.Journal{
		ID:        journalID,
		CompanyID: companyID,
		Code:      "GENERAL",
		Name:      "General Journal",
	}); err != nil {
		t.Fatalf("create journal: %v", err)
	}

	fy2025 := types.FiscalYearID(uuid.New())
	fy2026 := types.FiscalYearID(uuid.New())
	start2025 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	start2026 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end2025 := start2026.Add(-time.Nanosecond)
	end2026 := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)

	if err := store.CreateFiscalYear(ctx, masterdata.FiscalYear{
		ID: fy2025, CompanyID: companyID, Name: "FY2025", StartDate: start2025, EndDate: end2025, State: masterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatalf("create fy2025: %v", err)
	}
	if err := store.CreateFiscalYear(ctx, masterdata.FiscalYear{
		ID: fy2026, CompanyID: companyID, Name: "FY2026", StartDate: start2026, EndDate: end2026, State: masterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatalf("create fy2026: %v", err)
	}

	// Closed monthly periods 1..12 + open period 13 and 14 for FY2025
	for m := 1; m <= 12; m++ {
		mStart := time.Date(2025, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		mEnd := mStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
		if err := store.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
			ID: types.FiscalPeriodID(uuid.New()), CompanyID: companyID, FiscalYearID: fy2025,
			Number: m, Name: mStart.Format("Jan 2006"), StartDate: mStart, EndDate: mEnd,
			State: masterdata.PeriodHardClosed, Type: masterdata.FiscalPeriodTypeNormal,
		}); err != nil {
			t.Fatalf("create period %d: %v", m, err)
		}
	}
	if err := store.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: types.FiscalPeriodID(uuid.New()), CompanyID: companyID, FiscalYearID: fy2025,
		Number: 13, Name: "Year-End Adj 2025", StartDate: end2025, EndDate: end2025,
		State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeYearEndAdjustment,
	}); err != nil {
		t.Fatalf("create period 13: %v", err)
	}
	if err := store.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: types.FiscalPeriodID(uuid.New()), CompanyID: companyID, FiscalYearID: fy2025,
		Number: 14, Name: "Audit Adj 2025", StartDate: end2025, EndDate: end2025,
		State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeAuditAdjustment,
	}); err != nil {
		t.Fatalf("create period 14: %v", err)
	}
	// Open period 1 for FY2026
	if err := store.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: types.FiscalPeriodID(uuid.New()), CompanyID: companyID, FiscalYearID: fy2026,
		Number: 1, Name: "Jan 2026", StartDate: start2026, EndDate: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
	}); err != nil {
		t.Fatalf("create 2026 period 1: %v", err)
	}

	accounts := map[string]types.AccountID{
		"cash":     types.AccountID(uuid.New()),
		"revenue":  types.AccountID(uuid.New()),
		"expense":  types.AccountID(uuid.New()),
		"retained": types.AccountID(uuid.New()),
	}
	defs := []masterdata.Account{
		{ID: accounts["cash"], CompanyID: companyID, Code: "1000", Name: "Cash", Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true},
		{ID: accounts["revenue"], CompanyID: companyID, Code: "4000", Name: "Revenue", Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true},
		{ID: accounts["expense"], CompanyID: companyID, Code: "5000", Name: "Expense", Type: masterdata.AccountTypeExpense, NormalBalance: masterdata.NormalBalanceDebit, Currency: "USD", IsActive: true},
		{ID: accounts["retained"], CompanyID: companyID, Code: "3000", Name: "Retained Earnings", Type: masterdata.AccountTypeEquity, NormalBalance: masterdata.NormalBalanceCredit, Currency: "USD", IsActive: true, SubClassification: "retained_earnings"},
	}
	for _, a := range defs {
		if err := store.CreateAccount(ctx, a); err != nil {
			t.Fatalf("create account %s: %v", a.Code, err)
		}
	}

	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(adapter.Ports())).Build(ctx)
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	appSvc, err := sdk.NewApplicationService(adapter.Ports())
	if err != nil {
		t.Fatalf("new app service: %v", err)
	}

	return adapter, engine, appSvc, companyID, journalID, fy2025, fy2026, accounts
}

func TestCancelDraftAndAdjustmentPeriodsAndYearEndClosing(t *testing.T) {
	ctx := context.Background()
	_, engine, appSvc, companyID, journalID, fy2025, fy2026, accounts := setupPhase2TestFixture(t)

	appCtx := publicapplication.Context{
		Principal: publicapplication.Principal{
			ActorID:    "accountant-1",
			CompanyIDs: []types.CompanyID{companyID},
		},
		CorrelationID: "corr-phase2-1",
	}

	// 1. Create a draft and cancel it via DraftCancellationService
	zeroUSD, _ := types.NewMoneyFromString("0", "USD")
	amount100, _ := types.NewMoneyFromString("100.00", "USD")
	draftRes, err := appSvc.CreateDraft(ctx, publicapplication.CreateDraftCommand{
		Context:             appCtx,
		CompanyID:           companyID,
		JournalID:           journalID,
		EffectiveDate:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		Description:         "Draft to cancel",
		TransactionCurrency: "USD",
		ExchangeRate:        "1",
		Classification:      journal.EntryClassAdjusting,
		TargetPeriodNumber:  13,
		AdjustingEntryType:  journal.AdjustingEntryAccrual,
		AdjustmentReason:    "Accrual adjustment",
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: accounts["cash"], TransactionAmount: amount100, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount100, FunctionalCredit: zeroUSD},
			{ID: types.NewJournalEntryLineID(), AccountID: accounts["revenue"], TransactionAmount: amount100, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amount100},
		},
	})
	if err != nil {
		t.Fatalf("create draft in period 13: %v", err)
	}

	canceller, err := sdk.DraftCancellation(engine)
	if err != nil {
		t.Fatalf("sdk.DraftCancellation: %v", err)
	}
	cancelledRes, err := canceller.CancelDraft(ctx, publicapplication.CancelDraftCommand{
		Context:   appCtx,
		CompanyID: companyID,
		EntryID:   draftRes.Entry.ID,
		Reason:    "entered in error",
	})
	if err != nil {
		t.Fatalf("CancelDraft: %v", err)
	}
	if cancelledRes.Entry.Status != journal.EntryCancelled {
		t.Fatalf("expected status cancelled, got %s", cancelledRes.Entry.Status)
	}

	// 2. Post an adjusting entry into Period 14 (verifies GetPeriodByNumber works end-to-end)
	amount500, _ := types.NewMoneyFromString("500.00", "USD")
	adjDraft, err := appSvc.CreateDraft(ctx, publicapplication.CreateDraftCommand{
		Context:             appCtx,
		CompanyID:           companyID,
		JournalID:           journalID,
		EffectiveDate:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		Description:         "Audit adjustment in period 14",
		TransactionCurrency: "USD",
		ExchangeRate:        "1",
		Classification:      journal.EntryClassAdjusting,
		TargetPeriodNumber:  14,
		AdjustingEntryType:  journal.AdjustingEntryAudit,
		AdjustmentReason:    "Audit accrual",
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: accounts["cash"], TransactionAmount: amount500, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount500, FunctionalCredit: zeroUSD},
			{ID: types.NewJournalEntryLineID(), AccountID: accounts["revenue"], TransactionAmount: amount500, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amount500},
		},
	})
	if err != nil {
		t.Fatalf("create draft in period 14: %v", err)
	}
	postCtx := appCtx
	postCtx.IdempotencyKey = "idem-p14-1"
	if _, err := appSvc.Post(ctx, publicapplication.PostEntryCommand{
		Context:   postCtx,
		CompanyID: companyID,
		EntryID:   adjDraft.Entry.ID,
	}); err != nil {
		t.Fatalf("post period 14 entry: %v", err)
	}

	// 3. Execute Year-End Closing via sdk.YearEndClosing(engine)
	yec, err := sdk.YearEndClosing(engine)
	if err != nil {
		t.Fatalf("sdk.YearEndClosing: %v", err)
	}
	closeRes, err := yec.ExecuteYearEndClosing(ctx, publicapplication.ExecuteYearEndClosingCommand{
		Context:                   appCtx,
		CompanyID:                 companyID,
		FiscalYearID:              fy2025,
		NextFiscalYearID:          fy2026,
		JournalID:                 journalID,
		RetainedEarningsAccountID: accounts["retained"],
		Policy:                    publicapplication.YearEndClosingDirectToRetainedEarnings,
		Currency:                  "USD",
		Reason:                    "Annual close FY2025",
	})
	if err != nil {
		t.Fatalf("ExecuteYearEndClosing: %v", err)
	}
	if len(closeRes.ClosingEntryIDs) == 0 {
		t.Fatalf("expected closing entry IDs, got none")
	}
	if closeRes.OpeningEntryID == (types.JournalEntryID{}) {
		t.Fatalf("expected opening entry ID for FY2026")
	}
	if closeRes.NetIncome != "500" {
		t.Fatalf("expected net income 500, got %s", closeRes.NetIncome)
	}

	// 4. Reopen Year-End Closing
	reopenRes, err := yec.ReopenYearEndClosing(ctx, publicapplication.ReopenYearEndClosingCommand{
		Context:           appCtx,
		CompanyID:         companyID,
		FiscalYearID:      fy2025,
		NextFiscalYearID:  fy2026,
		ApprovalReference: "CFO-APPROVAL-2025-01",
		Reason:            "Post-close audit adjustment required",
	})
	if err != nil {
		t.Fatalf("ReopenYearEndClosing: %v", err)
	}
	if len(reopenRes.ReversalEntryIDs) < 2 {
		t.Fatalf("expected reversal entry IDs for closing + opening entries, got %d", len(reopenRes.ReversalEntryIDs))
	}
}
