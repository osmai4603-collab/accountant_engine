package contract_parity

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/shopspring/decimal"
)

func TestPhase12EnterpriseCapabilitiesParity(t *testing.T) {
	ctx := context.Background()
	f := NewUnifiedFixture()

	adapters := []*StorageAdapterHarness{
		NewMemoryStorageHarness(t, f),
		NewPostgresStorageHarness(t, f),
	}

	for _, h := range adapters {
		if h == nil {
			continue
		}
		h := h
		t.Run(h.Name, func(t *testing.T) {
			defer h.Cleanup()

			engine, err := sdk.NewBuilder(sdk.WithStoragePorts(h.Ports)).Build(ctx)
			if err != nil {
				t.Fatalf("%s: build engine: %v", h.Name, err)
			}
			defer func() { _ = engine.Close() }()

			appSvc, err := sdk.NewApplicationService(h.Ports)
			if err != nil {
				t.Fatalf("%s: new app service: %v", h.Name, err)
			}

			// Provision FY2024 (with closed months 1..12 and open periods 13, 14) + FY2025 + Retained Earnings
			fy2024 := types.FiscalYearID(uuid.New())
			fy2025 := types.FiscalYearID(uuid.New())
			start2024 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			start2025 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			end2024 := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
			end2025 := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

			if err := h.MasterData.CreateFiscalYear(ctx, publicmasterdata.FiscalYear{
				ID: fy2024, CompanyID: f.Company.ID, Name: "FY2024-PARITY", StartDate: start2024, EndDate: end2024, State: publicmasterdata.FiscalYearOpen,
			}); err != nil {
				t.Fatalf("%s: create fy2024: %v", h.Name, err)
			}
			if err := h.MasterData.CreateFiscalYear(ctx, publicmasterdata.FiscalYear{
				ID: fy2025, CompanyID: f.Company.ID, Name: "FY2025-PARITY", StartDate: start2025, EndDate: end2025, State: publicmasterdata.FiscalYearOpen,
			}); err != nil {
				t.Fatalf("%s: create fy2025: %v", h.Name, err)
			}

			for m := 1; m <= 12; m++ {
				mStart := time.Date(2024, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
				mEnd := mStart.AddDate(0, 1, -1)
				if err := h.MasterData.CreateFiscalPeriod(ctx, publicmasterdata.FiscalPeriod{
					ID: types.FiscalPeriodID(uuid.New()), CompanyID: f.Company.ID, FiscalYearID: fy2024,
					Number: m, Name: mStart.Format("Jan 2006"), StartDate: mStart, EndDate: mEnd,
					State: publicmasterdata.PeriodHardClosed, Type: publicmasterdata.FiscalPeriodTypeNormal,
				}); err != nil {
					t.Fatalf("%s: create period %d: %v", h.Name, m, err)
				}
			}
			adjDate := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
			if err := h.MasterData.CreateFiscalPeriod(ctx, publicmasterdata.FiscalPeriod{
				ID: types.FiscalPeriodID(uuid.New()), CompanyID: f.Company.ID, FiscalYearID: fy2024,
				Number: 13, Name: "Adj 13", StartDate: adjDate, EndDate: adjDate,
				State: publicmasterdata.PeriodOpen, Type: publicmasterdata.FiscalPeriodTypeYearEndAdjustment,
			}); err != nil {
				t.Fatalf("%s: create period 13: %v", h.Name, err)
			}
			if err := h.MasterData.CreateFiscalPeriod(ctx, publicmasterdata.FiscalPeriod{
				ID: types.FiscalPeriodID(uuid.New()), CompanyID: f.Company.ID, FiscalYearID: fy2024,
				Number: 14, Name: "Audit 14", StartDate: adjDate, EndDate: adjDate,
				State: publicmasterdata.PeriodOpen, Type: publicmasterdata.FiscalPeriodTypeAuditAdjustment,
			}); err != nil {
				t.Fatalf("%s: create period 14: %v", h.Name, err)
			}
			if err := h.MasterData.CreateFiscalPeriod(ctx, publicmasterdata.FiscalPeriod{
				ID: types.FiscalPeriodID(uuid.New()), CompanyID: f.Company.ID, FiscalYearID: fy2025,
				Number: 1, Name: "Jan 2025", StartDate: start2025, EndDate: time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC),
				State: publicmasterdata.PeriodOpen, Type: publicmasterdata.FiscalPeriodTypeNormal,
			}); err != nil {
				t.Fatalf("%s: create 2025 period 1: %v", h.Name, err)
			}

			retainedID := types.AccountID(uuid.New())
			if err := h.MasterData.CreateAccount(ctx, publicmasterdata.Account{
				ID: retainedID, CompanyID: f.Company.ID, Code: "3100", Name: "Retained Earnings",
				Type: publicmasterdata.AccountTypeEquity, NormalBalance: publicmasterdata.NormalBalanceCredit,
				Currency: "USD", IsActive: true, SubClassification: "retained_earnings",
			}); err != nil {
				t.Fatalf("%s: create retained earnings: %v", h.Name, err)
			}

			appCtx := publicapplication.Context{
				Principal:     publicapplication.Principal{ActorID: "parity-actor", CompanyIDs: []types.CompanyID{f.Company.ID}},
				CorrelationID: "parity-corr-" + h.Name,
			}

			zeroUSD, _ := types.NewMoneyFromString("0", "USD")
			amt200, _ := types.NewMoneyFromString("200.00", "USD")

			// 1. CancelDraft parity
			adjEffectiveDate := adjDate
			draftToCancel, err := appSvc.CreateDraft(ctx, publicapplication.CreateDraftCommand{
				Context:             appCtx,
				CompanyID:           f.Company.ID,
				JournalID:           f.Journal.ID,
				EffectiveDate:       adjEffectiveDate,
				Description:         "Parity draft to cancel",
				TransactionCurrency: "USD",
				ExchangeRate:        "1",
				Classification:      journal.EntryClassAdjusting,
				TargetPeriodNumber:  13,
				AdjustingEntryType:  journal.AdjustingEntryAccrual,
				AdjustmentReason:    "Accrual",
				Lines: []journal.EntryLine{
					{ID: types.NewJournalEntryLineID(), AccountID: f.CashAccount.ID, TransactionAmount: amt200, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt200, FunctionalCredit: zeroUSD},
					{ID: types.NewJournalEntryLineID(), AccountID: f.RevenueAccount.ID, TransactionAmount: amt200, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amt200},
				},
			})
			if err != nil {
				t.Fatalf("%s: create draft: %v", h.Name, err)
			}
			canceller, err := sdk.DraftCancellation(engine)
			if err != nil {
				t.Fatalf("%s: sdk.DraftCancellation: %v", h.Name, err)
			}
			cancelled, err := canceller.CancelDraft(ctx, publicapplication.CancelDraftCommand{
				Context: appCtx, CompanyID: f.Company.ID, EntryID: draftToCancel.Entry.ID, Reason: "parity cancel",
			})
			if err != nil || cancelled.Entry.Status != journal.EntryCancelled {
				t.Fatalf("%s: CancelDraft = %+v, %v", h.Name, cancelled, err)
			}
			// Posting cancelled draft must fail
			postCtx := appCtx
			postCtx.IdempotencyKey = "parity-post-cancelled-" + h.Name
			if _, err := appSvc.Post(ctx, publicapplication.PostEntryCommand{
				Context: postCtx, CompanyID: f.Company.ID, EntryID: cancelled.Entry.ID,
			}); err == nil {
				t.Fatalf("%s: expected error posting cancelled draft", h.Name)
			}

			// 2. Period 14 audit adjustment posting parity
			amt300, _ := types.NewMoneyFromString("300.00", "USD")
			p14Draft, err := appSvc.CreateDraft(ctx, publicapplication.CreateDraftCommand{
				Context:             appCtx,
				CompanyID:           f.Company.ID,
				JournalID:           f.Journal.ID,
				EffectiveDate:       adjEffectiveDate,
				Description:         "Parity period 14",
				TransactionCurrency: "USD",
				ExchangeRate:        "1",
				Classification:      journal.EntryClassAdjusting,
				TargetPeriodNumber:  14,
				AdjustingEntryType:  journal.AdjustingEntryAudit,
				AdjustmentReason:    "Audit adj",
				Lines: []journal.EntryLine{
					{ID: types.NewJournalEntryLineID(), AccountID: f.CashAccount.ID, TransactionAmount: amt300, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amt300, FunctionalCredit: zeroUSD},
					{ID: types.NewJournalEntryLineID(), AccountID: f.RevenueAccount.ID, TransactionAmount: amt300, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zeroUSD, FunctionalCredit: amt300},
				},
			})
			if err != nil {
				t.Fatalf("%s: create p14 draft: %v", h.Name, err)
			}
			postCtx.IdempotencyKey = "parity-post-p14-" + h.Name
			if _, err := appSvc.Post(ctx, publicapplication.PostEntryCommand{
				Context: postCtx, CompanyID: f.Company.ID, EntryID: p14Draft.Entry.ID,
			}); err != nil {
				t.Fatalf("%s: post p14: %v", h.Name, err)
			}

			// 3. YearEndClosing + Reopen parity
			yec, err := sdk.YearEndClosing(engine)
			if err != nil {
				t.Fatalf("%s: sdk.YearEndClosing: %v", h.Name, err)
			}
			closeRes, err := yec.ExecuteYearEndClosing(ctx, publicapplication.ExecuteYearEndClosingCommand{
				Context:                   appCtx,
				CompanyID:                 f.Company.ID,
				FiscalYearID:              fy2024,
				NextFiscalYearID:          fy2025,
				JournalID:                 f.Journal.ID,
				RetainedEarningsAccountID: retainedID,
				Policy:                    publicapplication.YearEndClosingDirectToRetainedEarnings,
				Currency:                  "USD",
				Reason:                    "Parity year-end close",
			})
			if err != nil || len(closeRes.ClosingEntryIDs) == 0 || closeRes.OpeningEntryID == (types.JournalEntryID{}) {
				t.Fatalf("%s: ExecuteYearEndClosing = %+v, %v", h.Name, closeRes, err)
			}

			reopenRes, err := yec.ReopenYearEndClosing(ctx, publicapplication.ReopenYearEndClosingCommand{
				Context:           appCtx,
				CompanyID:         f.Company.ID,
				FiscalYearID:      fy2024,
				NextFiscalYearID:  fy2025,
				ApprovalReference: "PARITY-APPROVAL-01",
				Reason:            "Parity reopen",
			})
			if err != nil || len(reopenRes.ReversalEntryIDs) < 2 {
				t.Fatalf("%s: ReopenYearEndClosing = %+v, %v", h.Name, reopenRes, err)
			}
		})
	}
}
