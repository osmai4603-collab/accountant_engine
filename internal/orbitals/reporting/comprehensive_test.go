package reporting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/policy"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/valuation"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/reporting"
	"github.com/shopspring/decimal"
)

type fixedRateRepo struct {
	*testRepository
	rates map[string]decimal.Decimal
}

func (r *fixedRateRepo) GetRateForCompany(_ context.Context, _ uuid.UUID, from, to string, _ time.Time) (decimal.Decimal, error) {
	if from == to {
		return decimal.NewFromInt(1), nil
	}
	if rate, ok := r.rates[from+"->"+to]; ok {
		return rate, nil
	}
	return decimal.Zero, valuation.ErrRateNotFound
}

func (r *fixedRateRepo) GetRateOfTypeForCompany(ctx context.Context, companyID uuid.UUID, _ valuation.RateType, from, to string, asOf time.Time) (decimal.Decimal, error) {
	return r.GetRateForCompany(ctx, companyID, from, to, asOf)
}

func (r *fixedRateRepo) GetRate(ctx context.Context, _ valuation.RateType, from, to string, asOf time.Time) (decimal.Decimal, error) {
	return r.GetRateForCompany(ctx, uuid.Nil, from, to, asOf)
}

type failingPolicyUnit struct {
	err error
}

func (u failingPolicyUnit) GetCurrencyPolicyAsOf(context.Context, uuid.UUID, time.Time) (policy.CurrencyPolicy, error) {
	return policy.CurrencyPolicy{}, u.err
}

func richReportingFixture(t *testing.T) (*testRepository, uuid.UUID, map[string]uuid.UUID, time.Time, time.Time) {
	t.Helper()
	companyID := uuid.New()
	ids := map[string]uuid.UUID{
		"cash":      uuid.New(),
		"ar":        uuid.New(),
		"ppe":       uuid.New(),
		"ap":        uuid.New(),
		"debt":      uuid.New(),
		"capital":   uuid.New(),
		"retained":  uuid.New(),
		"revenue":   uuid.New(),
		"cogs":      uuid.New(),
		"opex":      uuid.New(),
		"interest":  uuid.New(),
		"tax":       uuid.New(),
		"dividends": uuid.New(),
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	arAcc := testAccount(companyID, ids["ar"], "1100", "Accounts Receivable", masterdata.AccountTypeAsset, masterdata.SubClassTradeReceivables)
	ppeAcc := testAccount(companyID, ids["ppe"], "1500", "Equipment", masterdata.AccountTypeAsset, masterdata.SubClassPropertyPlantEquipment)
	ppeAcc.CashFlowActivity = "investing"
	apAcc := testAccount(companyID, ids["ap"], "2000", "Accounts Payable", masterdata.AccountTypeLiability, masterdata.SubClassTradePayables)
	apAcc.CashFlowActivity = "operating"
	debtAcc := testAccount(companyID, ids["debt"], "2500", "Long-Term Debt", masterdata.AccountTypeLiability, masterdata.SubClassLongTermBorrowings)
	debtAcc.CashFlowActivity = "financing"
	retainedAcc := testAccount(companyID, ids["retained"], "3100", "Retained Earnings", masterdata.AccountTypeEquity, masterdata.SubClassRetainedEarnings)
	divAcc := testAccount(companyID, ids["dividends"], "3200", "Dividends", masterdata.AccountTypeEquity, masterdata.SubClassDistributions)
	cogsAcc := testAccount(companyID, ids["cogs"], "5000", "COGS", masterdata.AccountTypeExpense, masterdata.SubClassCostOfSales)
	opexAcc := testAccount(companyID, ids["opex"], "6000", "Operating Expenses", masterdata.AccountTypeExpense, masterdata.SubClassGeneralAdminExpense)
	intAcc := testAccount(companyID, ids["interest"], "7000", "Interest Expense", masterdata.AccountTypeExpense, masterdata.SubClassFinanceCost)
	taxAcc := testAccount(companyID, ids["tax"], "8000", "Tax Expense", masterdata.AccountTypeExpense, masterdata.SubClassIncomeTaxExpense)

	repo := &testRepository{
		companyID: companyID,
		accounts: []masterdata.Account{
			testAccount(companyID, ids["cash"], "1000", "Cash", masterdata.AccountTypeAsset, masterdata.SubClassCashAndEquivalents),
			arAcc,
			ppeAcc,
			apAcc,
			debtAcc,
			testAccount(companyID, ids["capital"], "3000", "Share Capital", masterdata.AccountTypeEquity, masterdata.SubClassShareCapital),
			retainedAcc,
			divAcc,
			testAccount(companyID, ids["revenue"], "4000", "Revenue", masterdata.AccountTypeIncome, masterdata.SubClassOperatingRevenue),
			cogsAcc,
			opexAcc,
			intAcc,
			taxAcc,
		},
		entries: []posting.JournalEntry{
			// Opening balances before start
			testEntry(t, companyID, start.Add(-48*time.Hour),
				testLine(ids["cash"], "500", "0"),
				testLine(ids["capital"], "0", "500"),
			),
			// Revenue on credit + cash collection
			testEntry(t, companyID, start.Add(10*24*time.Hour),
				testLine(ids["ar"], "300", "0"),
				testLine(ids["revenue"], "0", "300"),
			),
			testEntry(t, companyID, start.Add(20*24*time.Hour),
				testLine(ids["cash"], "200", "0"),
				testLine(ids["ar"], "0", "200"),
			),
			// Expenses
			testEntry(t, companyID, start.Add(30*24*time.Hour),
				testLine(ids["cogs"], "100", "0"),
				testLine(ids["ap"], "0", "100"),
			),
			testEntry(t, companyID, start.Add(40*24*time.Hour),
				testLine(ids["opex"], "50", "0"),
				testLine(ids["cash"], "0", "50"),
			),
			testEntry(t, companyID, start.Add(50*24*time.Hour),
				testLine(ids["interest"], "10", "0"),
				testLine(ids["cash"], "0", "10"),
			),
			testEntry(t, companyID, start.Add(60*24*time.Hour),
				testLine(ids["tax"], "20", "0"),
				testLine(ids["cash"], "0", "20"),
			),
			// Investing: PPE purchase
			testEntry(t, companyID, start.Add(70*24*time.Hour),
				testLine(ids["ppe"], "150", "0"),
				testLine(ids["cash"], "0", "150"),
			),
			// Financing: Debt issuance
			testEntry(t, companyID, start.Add(80*24*time.Hour),
				testLine(ids["cash"], "100", "0"),
				testLine(ids["debt"], "0", "100"),
			),
		},
	}
	return repo, companyID, ids, start, end
}

func TestComprehensiveReportingEngines(t *testing.T) {
	ctx := context.Background()
	repo, companyID, ids, start, end := richReportingFixture(t)
	engine := reporting.NewEngine(repo)

	// 1. Income Statement
	is, err := engine.Income.GenerateForCompanyContext(ctx, companyID, start, end, "USD")
	if err != nil {
		t.Fatalf("Income.GenerateForCompanyContext: %v", err)
	}
	if !is.Revenue.Equal(decimal.NewFromInt(300)) || !is.NetProfit.Equal(decimal.NewFromInt(120)) {
		t.Fatalf("unexpected income statement: revenue=%s netProfit=%s", is.Revenue, is.NetProfit)
	}
	if _, err := engine.Income.Generate(start, end, "USD"); err != nil {
		t.Fatalf("Income.Generate: %v", err)
	}

	// 2. Equity Statement
	eq, err := engine.Equity.GenerateForCompanyContext(ctx, companyID, start, end, "USD")
	if err != nil {
		t.Fatalf("Equity.GenerateForCompanyContext: %v", err)
	}
	if eq.Currency != "USD" {
		t.Fatalf("expected USD equity currency, got %q", eq.Currency)
	}
	if _, err := engine.Equity.Generate(start, end, "USD"); err != nil {
		t.Fatalf("Equity.Generate: %v", err)
	}

	// 3. General Ledger
	gl, err := engine.GeneralLedger.GenerateForCompanyContext(ctx, companyID, ids["cash"], start, end, "USD")
	if err != nil {
		t.Fatalf("GeneralLedger.GenerateForCompanyContext: %v", err)
	}
	if len(gl.Lines) == 0 {
		t.Fatal("expected non-empty general ledger lines for cash")
	}
	if _, err := engine.GeneralLedger.Generate(ids["cash"], start, end, "USD"); err != nil {
		t.Fatalf("GeneralLedger.Generate: %v", err)
	}

	// 4. Ratios
	ratios, err := engine.Ratios.GenerateForCompanyContext(ctx, companyID, start, end, "USD")
	if err != nil {
		t.Fatalf("Ratios.GenerateForCompanyContext: %v", err)
	}
	if ratios.Profitability.NetMargin.IsZero() {
		t.Fatalf("expected non-zero NetMargin, got %+v", ratios.Profitability)
	}
	if _, err := engine.Ratios.Generate(start, end, "USD"); err != nil {
		t.Fatalf("Ratios.Generate: %v", err)
	}

	// 5. CashFlow Direct & Indirect
	classifications := make(map[uuid.UUID]reporting.CashFlowTransactionClassification)
	for _, e := range repo.entries {
		for _, l := range e.Lines {
			classifications[l.ID] = reporting.CashFlowTransactionClassification{
				Activity: reporting.CashFlowActivityOperating,
				Code:     "OP",
				Label:    "Operating",
			}
		}
	}
	cfDirect, err := engine.CashFlow.GenerateDirectWithClassifications(start, end, "USD", classifications)
	if err != nil {
		t.Fatalf("CashFlow.GenerateDirectWithClassifications: %v", err)
	}
	if cfDirect.Currency != "USD" {
		t.Fatalf("unexpected direct cash flow currency: %q", cfDirect.Currency)
	}
	if _, err := engine.CashFlow.GenerateDirectForCompanyContext(ctx, companyID, start, end, "USD"); err == nil {
		t.Fatal("expected GenerateDirectForCompanyContext without classifications to return error")
	}
	if _, err := engine.CashFlow.GenerateWithRequest(reporting.CashFlowRequest{
		StartDate: start,
		EndDate:   end,
		Currency:  "USD",
	}); err != nil {
		t.Fatalf("CashFlow.GenerateWithRequest: %v", err)
	}

	// 6. Bundle + PolicyUnit
	engine.SetPolicyUnit(testPolicyUnit{policy: policy.CurrencyPolicy{
		FunctionalCurrency:   "USD",
		PresentationCurrency: "USD",
		AllowedCurrencies:    []string{"USD", "EUR"},
		EffectiveDate:        start,
	}})
	bundle, err := engine.GenerateBundleForCompanyContext(ctx, companyID, start, end, "")
	if err != nil {
		t.Fatalf("GenerateBundleForCompanyContext: %v", err)
	}
	if !bundle.BalanceSheet.Balanced {
		t.Fatal("expected balanced balance sheet in bundle")
	}

	// PolicyUnit failure
	engine.SetPolicyUnit(failingPolicyUnit{err: errors.New("policy missing")})
	if _, err := engine.GenerateBundleForCompanyContext(ctx, companyID, start, end, ""); err == nil {
		t.Fatal("expected error when PolicyUnit fails")
	}
	engine.SetPolicyUnit(nil)
	if _, err := engine.GenerateBundleForCompanyContext(ctx, companyID, start, end, "usd"); err != nil {
		t.Fatalf("expected bundle generation without policyUnit to succeed: %v", err)
	}

	// 7. Comparative Bundle
	priorStart := start.AddDate(-1, 0, 0)
	priorEnd := end.AddDate(-1, 0, 0)
	comp, err := engine.Bundle.GenerateComparativeForCompanyContext(ctx, companyID, start, end, priorStart, priorEnd, "USD", []reporting.NoteDisclosure{
		{Code: "N1", Title: "Basis of Preparation", Description: "IFRS"},
	})
	if err != nil {
		t.Fatalf("GenerateComparativeForCompanyContext: %v", err)
	}
	if comp.BalanceSheet.Currency != "USD" {
		t.Fatalf("expected USD comparative bundle, got %q", comp.BalanceSheet.Currency)
	}

	// 8. Statement Reconciliation & Balance Reconciliation
	reconReport, err := engine.StatementRecon.Reconcile(ctx, companyID, start, end, "USD")
	if err != nil {
		t.Fatalf("StatementRecon.Reconcile: %v", err)
	}
	if !reconReport.TrialBalanceBalanced {
		t.Fatalf("expected StatementRecon.Reconcile TrialBalanceBalanced=true, got %+v", reconReport)
	}
	compRecon, err := engine.StatementRecon.ReconcileComparativePeriods(ctx, companyID, priorStart, priorEnd, start, end, "USD")
	if err != nil {
		t.Fatalf("StatementRecon.ReconcileComparativePeriods: %v", err)
	}
	if !compRecon.OpeningBalancesTied {
		t.Fatalf("expected ReconcileComparativePeriods OpeningBalancesTied=true, got %+v", compRecon)
	}

	if _, err := engine.Reconciliation.Reconcile(ctx, companyID, uuid.New(), "USD"); err == nil {
		t.Fatal("expected Reconciliation.Reconcile to report unsupported repository")
	}

	// 9. Multi-currency Trial Balance & CurrencyTranslator
	rateRepo := &fixedRateRepo{
		testRepository: repo,
		rates:          map[string]decimal.Decimal{"USD->EUR": decimal.NewFromFloat(0.90)},
	}
	mcEngine := reporting.NewTrialBalanceEngine(rateRepo)
	mcTB, err := mcEngine.GenerateMultiCurrency(start, end, "EUR")
	if err != nil {
		t.Fatalf("GenerateMultiCurrency: %v", err)
	}
	if mcTB.FunctionalCurrency != "EUR" {
		t.Fatalf("expected EUR functional currency, got %q", mcTB.FunctionalCurrency)
	}

	// Dimension P&L
	dimPL, err := engine.GenerateDimensionProfitAndLossForCompanyContext(ctx, companyID, start, end, "USD")
	if err != nil {
		t.Fatalf("GenerateDimensionProfitAndLossForCompanyContext: %v", err)
	}
	_ = dimPL

	translator := reporting.NewCurrencyTranslator(rateRepo)
	amtUSD, _ := financialtypes.NewMoneyFromString("100.00", "USD")
	amtEUR, err := translator.TranslateAmount(ctx, amtUSD, "EUR", end, valuation.SpotRate)
	if err != nil || amtEUR.Currency() != "EUR" {
		t.Fatalf("TranslateAmount failed: got=%+v err=%v", amtEUR, err)
	}
	if _, _, err := translator.TranslateBalanceSheet([]reporting.BalanceSheetItem{
		{Amount: decimal.NewFromInt(100), Classification: reporting.TranslationAsset},
	}, "USD", "EUR", end); err != nil {
		t.Fatalf("TranslateBalanceSheet failed: %v", err)
	}
	if _, err := translator.TranslateIncomeStatement([]reporting.IncomeStatementItem{
		{Amount: decimal.NewFromInt(300)},
	}, "USD", "EUR", start, end); err != nil {
		t.Fatalf("TranslateIncomeStatement failed: %v", err)
	}
	if _, err := translator.TranslateEquity([]reporting.EquityItem{
		{Amount: decimal.NewFromInt(500), HistoricalDate: start},
	}, "USD", "EUR"); err != nil {
		t.Fatalf("TranslateEquity failed: %v", err)
	}

	// 10. Capability checks & BalanceIntegrityUnit
	if err := engine.ValidateCapability(reporting.ReportTrialBalance, "USD", "USD"); err != nil {
		t.Fatalf("ValidateCapability same currency failed: %v", err)
	}
	if err := engine.ValidateCapability(reporting.ReportTrialBalance, "USD", "EUR"); err != nil {
		t.Fatalf("ValidateCapability cross-currency with RateProvider failed: %v", err)
	}
	if err := reporting.NewEngine(nil).ValidateCapability(reporting.ReportTrialBalance, "USD", "EUR"); err == nil {
		t.Fatal("expected ValidateCapability cross-currency on nil engine to fail")
	}
	biu := reporting.NewBalanceIntegrityUnit(repo)
	if _, err := biu.Reconcile(ctx, companyID, uuid.New(), "USD"); err == nil {
		t.Fatal("expected BalanceIntegrityUnit.Reconcile to report unsupported repository")
	}
}

type errReportingRepo struct {
	*testRepository
	err error
}

func (r *errReportingRepo) ListAccounts(context.Context) ([]masterdata.Account, error) {
	return nil, r.err
}

func (r *errReportingRepo) ListAccountsForCompanyContext(context.Context, uuid.UUID) ([]masterdata.Account, error) {
	return nil, r.err
}

func (r *errReportingRepo) ListAccountsForCompanyAsOfContext(context.Context, uuid.UUID, time.Time) ([]masterdata.Account, error) {
	return nil, r.err
}

func TestReportingErrorPaths(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	errEngine := reporting.NewEngine(&errReportingRepo{testRepository: &testRepository{}, err: errors.New("db down")})
	if _, err := errEngine.TrialBalance.GenerateForCompanyContext(ctx, companyID, start, end, "USD"); err == nil {
		t.Fatal("expected error when repository fails")
	}
	if _, err := errEngine.BalanceSheet.GenerateForCompanyContext(ctx, companyID, end, "USD"); err == nil {
		t.Fatal("expected error when repository fails")
	}
	if _, err := errEngine.Income.GenerateForCompanyContext(ctx, companyID, start, end, "USD"); err == nil {
		t.Fatal("expected error when repository fails")
	}
	if _, err := errEngine.CashFlow.GenerateForCompanyContext(ctx, companyID, start, end, "USD"); err == nil {
		t.Fatal("expected error when repository fails")
	}
	if _, err := errEngine.Equity.GenerateForCompanyContext(ctx, companyID, start, end, "USD"); err == nil {
		t.Fatal("expected error when repository fails")
	}
	if _, err := errEngine.Ratios.GenerateForCompanyContext(ctx, companyID, start, end, "USD"); err == nil {
		t.Fatal("expected error when repository fails")
	}
}
