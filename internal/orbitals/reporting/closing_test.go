package reporting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/reporting"
)

type closingTestRepo struct {
	*testRepository
	year    masterdata.FiscalYear
	periods []masterdata.FiscalPeriod
	drafts  []posting.JournalEntry
}

func (r *closingTestRepo) GetFiscalYearForCompanyContext(_ context.Context, companyID, yearID uuid.UUID) (masterdata.FiscalYear, error) {
	if r.year.ID.UUID() == yearID && r.year.CompanyID.UUID() == companyID {
		return r.year, nil
	}
	return masterdata.FiscalYear{}, errors.New("fiscal year not found")
}

func (r *closingTestRepo) ListFiscalPeriodsForCompanyContext(_ context.Context, _, yearID uuid.UUID) ([]masterdata.FiscalPeriod, error) {
	if r.year.ID.UUID() == yearID {
		return r.periods, nil
	}
	return nil, nil
}

func (r *closingTestRepo) UpdatePeriodStateForCompanyContext(_ context.Context, _, periodID uuid.UUID, state masterdata.PeriodState) error {
	for i := range r.periods {
		if r.periods[i].ID.UUID() == periodID {
			r.periods[i].State = state
		}
	}
	return nil
}

func (r *closingTestRepo) UpdateFiscalYearStateForCompanyContext(_ context.Context, _, yearID uuid.UUID, state masterdata.FiscalYearState) error {
	if r.year.ID.UUID() == yearID {
		r.year.State = state
	}
	return nil
}

func (r *closingTestRepo) ListDraftEntriesForCompanyContext(context.Context, uuid.UUID) ([]posting.JournalEntry, error) {
	return r.drafts, nil
}

func TestClosingEngineLifecycle(t *testing.T) {
	ctx := context.Background()
	baseRepo, companyID, ids, start, end := richReportingFixture(t)
	yearID := uuid.New()
	periodID := uuid.New()
	journalID := uuid.New()

	periods := make([]masterdata.FiscalPeriod, 0, 13)
	for m := 1; m <= 12; m++ {
		periods = append(periods, masterdata.FiscalPeriod{
			ID:           financialtypes.FiscalPeriodID(uuid.New()),
			CompanyID:    financialtypes.CompanyID(companyID),
			FiscalYearID: financialtypes.FiscalYearID(yearID),
			Number:       m,
			Name:         "P",
			StartDate:    start,
			EndDate:      end,
			State:        masterdata.PeriodHardClosed,
			Type:         masterdata.FiscalPeriodTypeNormal,
		})
	}
	periods = append(periods, masterdata.FiscalPeriod{
		ID:           financialtypes.FiscalPeriodID(periodID),
		CompanyID:    financialtypes.CompanyID(companyID),
		FiscalYearID: financialtypes.FiscalYearID(yearID),
		Number:       13,
		Name:         "P13",
		StartDate:    end,
		EndDate:      end,
		State:        masterdata.PeriodOpen,
		Type:         masterdata.FiscalPeriodTypeYearEndAdjustment,
	})

	for i := range baseRepo.entries {
		if baseRepo.entries[i].PostingDate.IsZero() {
			baseRepo.entries[i].PostingDate = baseRepo.entries[i].EntryDate
		}
		if baseRepo.entries[i].PeriodID == uuid.Nil && !baseRepo.entries[i].EntryDate.Before(start) {
			baseRepo.entries[i].PeriodID = periods[0].ID.UUID()
		}
		for j := range baseRepo.entries[i].Lines {
			l := &baseRepo.entries[i].Lines[j]
			if l.TransactionAmount.Amount().IsZero() {
				if !l.Debit.Amount().IsZero() {
					l.TransactionAmount = l.Debit
				} else {
					l.TransactionAmount = l.Credit
				}
			}
		}
	}

	repo := &closingTestRepo{
		testRepository: baseRepo,
		year: masterdata.FiscalYear{
			ID:        financialtypes.FiscalYearID(yearID),
			CompanyID: financialtypes.CompanyID(companyID),
			Name:      "FY2026",
			StartDate: start,
			EndDate:   end,
			State:     masterdata.FiscalYearOpen,
		},
		periods: periods,
	}

	engine := reporting.NewEngine(repo)

	// 1. EvaluateClosingChecklist
	checklist, err := engine.Closing.EvaluateClosingChecklist(ctx, repo.year)
	if err != nil {
		t.Fatalf("EvaluateClosingChecklist: %v", err)
	}
	if checklist.Summary() == "" {
		t.Fatal("expected non-empty checklist summary")
	}

	// 2. GenerateClosingMoveContext
	closingEntry, netIncome, err := engine.Closing.GenerateClosingMoveContext(ctx, repo.year, journalID, ids["retained"], "USD")
	if err != nil {
		t.Fatalf("GenerateClosingMoveContext: %v", err)
	}
	_ = netIncome
	if closingEntry.ID == uuid.Nil {
		t.Fatal("expected non-nil closing entry ID")
	}

	// Income summary policy
	summaryAccountID := uuid.New()
	repo.accounts = append(repo.accounts, testAccount(companyID, summaryAccountID, "3999", "Income Summary", masterdata.AccountTypeEquity, masterdata.SubClassStatutoryReserve))
	summaryEntries, _, err := engine.Closing.GenerateClosingMovesContext(ctx, repo.year, journalID, ids["retained"], "USD", reporting.ClosingOptions{
		Policy:                 reporting.ClosingPolicyIncomeSummary,
		IncomeSummaryAccountID: summaryAccountID,
	})
	if err != nil || len(summaryEntries) != 2 {
		t.Fatalf("GenerateClosingMovesContext IncomeSummary: len=%d err=%v", len(summaryEntries), err)
	}

	// 3. ConfirmFiscalYearCloseWithoutEntriesContext on an empty year
	emptyBase := &testRepository{
		companyID: companyID,
		accounts:  baseRepo.accounts,
	}
	emptyRepo := &closingTestRepo{
		testRepository: emptyBase,
		year:           repo.year,
		periods:        append([]masterdata.FiscalPeriod(nil), periods...),
	}
	emptyEngine := reporting.NewEngine(emptyRepo)
	if err := emptyEngine.Closing.ConfirmFiscalYearCloseWithoutEntriesContext(ctx, emptyRepo.year, journalID, ids["retained"], "USD", reporting.ClosingOptions{}); err != nil {
		t.Fatalf("ConfirmFiscalYearCloseWithoutEntriesContext: %v", err)
	}
	if emptyRepo.year.State == masterdata.FiscalYearOpen {
		t.Fatalf("expected fiscal year to be closed, got %s", emptyRepo.year.State)
	}

	// 4. CarryForwardBalances & GenerateOpeningMoveContext
	if err := emptyEngine.Closing.CarryForwardBalances(ctx, emptyRepo.year, journalID); err == nil {
		t.Fatal("expected CarryForwardBalances without materialized balance repo to fail")
	}
	openingEntry, err := emptyEngine.Closing.GenerateOpeningMoveContext(ctx, emptyRepo.year, journalID, "USD")
	if err != nil {
		t.Fatalf("GenerateOpeningMoveContext: %v", err)
	}
	_ = openingEntry

	// 5. Validation error paths
	if _, _, err := emptyEngine.Closing.GenerateClosingMoveContext(ctx, emptyRepo.year, journalID, ids["retained"], "USD"); !errors.Is(err, reporting.ErrFiscalYearNotOpen) {
		t.Fatalf("expected ErrFiscalYearNotOpen on closed year, got %v", err)
	}
	openYear := repo.year
	openYear.State = masterdata.FiscalYearOpen
	if _, _, err := engine.Closing.GenerateClosingMoveContext(ctx, openYear, journalID, uuid.Nil, "USD"); !errors.Is(err, reporting.ErrRetainedEarningsNotSet) {
		t.Fatalf("expected ErrRetainedEarningsNotSet, got %v", err)
	}
}
