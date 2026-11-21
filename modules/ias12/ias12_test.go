package ias12

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// Shared test helpers (ported from the reference IAS 12 tests)
// ---------------------------------------------------------------------------

func testMoney(t *testing.T, value, currency string) types.Money {
	t.Helper()
	money, err := types.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func testRate(value string) TaxRate {
	return TaxRate{
		Value: decimal.RequireFromString(value), Enacted: true,
		EnactedOn:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func testPeriod(kind PeriodKind) MeasurementPeriod {
	return MeasurementPeriod{Kind: kind, AsOf: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}
}

// assertBalanced checks the public journal.EntryLine invariants directly; the
// internal posting entry validator is deliberately not part of this module.
func assertBalanced(t *testing.T, entry journal.JournalEntry) {
	t.Helper()
	if len(entry.Lines) < 2 {
		t.Fatalf("generated entry has %d lines, want at least two", len(entry.Lines))
	}
	debit, credit := decimal.Zero, decimal.Zero
	for _, line := range entry.Lines {
		if line.ID == (types.JournalEntryLineID{}) {
			t.Fatal("generated line has no line ID")
		}
		if line.AccountID == (types.AccountID{}) {
			t.Fatal("generated line has no account ID")
		}
		if line.TransactionCurrency == "" || line.TransactionAmount.Currency() != line.TransactionCurrency {
			t.Fatalf("line currency mismatch: %+v", line)
		}
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	if !debit.Equal(credit) || debit.IsZero() {
		t.Fatalf("generated journal entry is unbalanced: debit=%s credit=%s", debit, credit)
	}
}

func assertAccountTotal(t *testing.T, entry journal.JournalEntry, account uuid.UUID, debit, credit string) {
	t.Helper()
	debitAmount := zeroMoney("USD")
	creditAmount := zeroMoney("USD")
	for _, line := range entry.Lines {
		if line.AccountID != types.AccountID(account) {
			continue
		}
		var debitErr, creditErr error
		if line.FunctionalDebit.Currency() != "" {
			debitAmount, debitErr = debitAmount.Add(line.FunctionalDebit)
		}
		if line.FunctionalCredit.Currency() != "" {
			creditAmount, creditErr = creditAmount.Add(line.FunctionalCredit)
		}
		if debitErr != nil || creditErr != nil {
			t.Fatalf("failed to total journal lines: %v %v", debitErr, creditErr)
		}
	}
	debitTotal, creditTotal := debitAmount.Amount().String(), creditAmount.Amount().String()
	if debitTotal != debit || creditTotal != credit {
		t.Errorf("account %s totals debit=%s credit=%s, want debit=%s credit=%s", account, debitTotal, creditTotal, debit, credit)
	}
}

// ---------------------------------------------------------------------------
// Public-port mocks
// ---------------------------------------------------------------------------

type stubEntryReader struct{ standards.EntryReader }
type stubApplication struct{ publicapplication.Service }

type stubMasterDataReader struct{}

func (stubMasterDataReader) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{}, nil
}
func (stubMasterDataReader) GetAccount(context.Context, types.CompanyID, types.AccountID) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (stubMasterDataReader) GetAccountByCode(context.Context, types.CompanyID, string) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (stubMasterDataReader) GetJournalByCode(context.Context, types.CompanyID, string) (masterdata.Journal, error) {
	return masterdata.Journal{}, nil
}

// recordingPoster captures standards.StandardPostingRequest values so tests can
// assert the exact public lines handed to the application posting service.
type recordingPoster struct {
	requests []standards.StandardPostingRequest
	result   publicapplication.EntryResult
	err      error
}

func (p *recordingPoster) PostStandardEntry(
	_ context.Context,
	req standards.StandardPostingRequest,
) (publicapplication.EntryResult, error) {
	p.requests = append(p.requests, req)
	if p.err != nil {
		return publicapplication.EntryResult{}, p.err
	}
	if p.result.Entry.ID == (types.JournalEntryID{}) {
		p.result.Entry.ID = types.NewJournalEntryID()
		p.result.Entry.Status = journal.EntryPosted
	}
	return p.result, nil
}

type mockModulePorts struct {
	entries     standards.EntryReader
	balance     standards.BalanceReader
	masterData  standards.MasterDataReader
	application publicapplication.Service
	posting     standards.StandardPostingCapability
}

func (p *mockModulePorts) Entries() standards.EntryReader               { return p.entries }
func (p *mockModulePorts) Balance() standards.BalanceReader             { return p.balance }
func (p *mockModulePorts) MasterData() standards.MasterDataReader       { return p.masterData }
func (p *mockModulePorts) Application() publicapplication.Service       { return p.application }
func (p *mockModulePorts) Posting() standards.StandardPostingCapability { return p.posting }

func validPorts() standards.ModulePorts {
	return &mockModulePorts{
		entries:     stubEntryReader{},
		masterData:  stubMasterDataReader{},
		application: stubApplication{},
		posting:     &recordingPoster{},
	}
}

func assertPostingRequest(t *testing.T, req standards.StandardPostingRequest, standardRef, entityID, idem string, lineCount int) {
	t.Helper()
	if req.StandardRef != standardRef || req.EntityID != entityID || req.IdempotencyKey != idem {
		t.Fatalf("posting metadata = %+v, want ref=%s entity=%s key=%s", req, standardRef, entityID, idem)
	}
	if len(req.Lines) != lineCount {
		t.Fatalf("posting lines = %d, want %d", len(req.Lines), lineCount)
	}
	debit, credit := decimal.Zero, decimal.Zero
	for _, line := range req.Lines {
		if line.AccountID == (types.AccountID{}) || line.ID == (types.JournalEntryLineID{}) {
			t.Fatalf("incomplete posting line: %+v", line)
		}
		if !line.ExchangeRate.Equal(decimal.NewFromInt(1)) || line.TransactionCurrency == "" {
			t.Fatalf("invalid posting line currency/rate: %+v", line)
		}
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	if !debit.Equal(credit) || debit.IsZero() {
		t.Fatalf("unbalanced posting request: debit=%s credit=%s", debit, credit)
	}
}

// ---------------------------------------------------------------------------
// ias12_test.go (deferred tax measurement / reconciliation)
// ---------------------------------------------------------------------------

func TestMeasureDTAAndDTLSignsAndRates(t *testing.T) {
	probable := true
	measurement, err := Measure([]TaxBalance{
		{ID: "tax-depreciation", Kind: Asset, CarryingAmount: testMoney(t, "100", "USD"), TaxBase: testMoney(t, "70", "USD"), Rate: testRate("0.25")},
		{ID: "ecl-provision", Kind: Asset, CarryingAmount: testMoney(t, "60", "USD"), TaxBase: testMoney(t, "90", "USD"), Rate: testRate("0.20"), FutureTaxableProfitProbable: &probable},
		{ID: "accrual", Kind: Liability, CarryingAmount: testMoney(t, "40", "USD"), TaxBase: testMoney(t, "10", "USD"), Rate: testRate("0.30"), FutureTaxableProfitProbable: &probable},
		{ID: "prepaid-liability", Kind: Liability, CarryingAmount: testMoney(t, "10", "USD"), TaxBase: testMoney(t, "30", "USD"), Rate: testRate("0.10")},
	}, testPeriod(Quarterly))
	if err != nil {
		t.Fatal(err)
	}
	if got := measurement.Balances[0].DTL.Amount().String(); got != "7.5" {
		t.Fatalf("asset DTL = %s, want 7.5", got)
	}
	if got := measurement.Balances[1].DTA.Amount().String(); got != "6" {
		t.Fatalf("asset DTA = %s, want 6", got)
	}
	if got := measurement.Balances[2].DTA.Amount().String(); got != "9" {
		t.Fatalf("liability DTA = %s, want 9", got)
	}
	if got := measurement.Balances[3].DTL.Amount().String(); got != "2" {
		t.Fatalf("liability DTL = %s, want 2", got)
	}
	if got := measurement.DTA.Amount().String(); got != "15" {
		t.Fatalf("total DTA = %s, want 15", got)
	}
	if got := measurement.DTL.Amount().String(); got != "9.5" {
		t.Fatalf("total DTL = %s, want 9.5", got)
	}
	if measurement.Period.Kind != Quarterly {
		t.Fatalf("measurement period = %q, want quarterly", measurement.Period.Kind)
	}
}

func TestMeasureAnnualAndZeroRate(t *testing.T) {
	measurement, err := Measure([]TaxBalance{{
		ID: "zero-rated", Kind: Asset, CarryingAmount: testMoney(t, "12", "EUR"),
		TaxBase: testMoney(t, "10", "EUR"), Rate: testRate("0"),
	}}, testPeriod(Annual))
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.DTL.Amount().IsZero() || measurement.Period.Kind != Annual {
		t.Fatalf("unexpected annual measurement: %+v", measurement)
	}
}

func TestMeasureSuppressesDTAWhenFutureProfitIsNotProbable(t *testing.T) {
	probable := false
	measurement, err := Measure([]TaxBalance{{
		ID: "deductible", Kind: Asset,
		CarryingAmount: testMoney(t, "80", "USD"), TaxBase: testMoney(t, "100", "USD"),
		Rate: testRate("0.25"), FutureTaxableProfitProbable: &probable,
	}}, testPeriod(Annual))
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.Balances[0].DeductibleTemporaryDifference.Amount().Equal(decimal.NewFromInt(20)) ||
		!measurement.DTA.Amount().IsZero() {
		t.Fatalf("unexpected DTA recognition result: %+v", measurement)
	}
}

func TestMeasureRejectsInvalidBalancesRatesAndPeriods(t *testing.T) {
	valid := TaxBalance{
		ID: "asset", Kind: Asset, CarryingAmount: testMoney(t, "10", "USD"),
		TaxBase: testMoney(t, "9", "USD"), Rate: testRate("0.2"),
	}
	tests := []struct {
		name   string
		rows   []TaxBalance
		period MeasurementPeriod
		want   error
	}{
		{"empty balances", nil, testPeriod(Quarterly), ErrNoBalances},
		{"unsupported period", []TaxBalance{valid}, MeasurementPeriod{Kind: "monthly", AsOf: testPeriod(Quarterly).AsOf}, ErrInvalidMeasurementPeriod},
		{"negative carrying amount", []TaxBalance{{ID: "bad", Kind: Asset, CarryingAmount: testMoney(t, "-1", "USD"), TaxBase: testMoney(t, "0", "USD"), Rate: testRate("0.2")}}, testPeriod(Quarterly), types.ErrNegativeAmountProhibited},
		{"mixed currencies", []TaxBalance{valid, {ID: "foreign", Kind: Asset, CarryingAmount: testMoney(t, "1", "EUR"), TaxBase: testMoney(t, "1", "EUR"), Rate: testRate("0.2")}}, testPeriod(Quarterly), ErrCurrencyMismatch},
		{"invalid rate", []TaxBalance{{ID: "rate", Kind: Asset, CarryingAmount: testMoney(t, "1", "USD"), TaxBase: testMoney(t, "0", "USD"), Rate: testRate("1.01")}}, testPeriod(Quarterly), ErrInvalidRate},
		{"unenacted rate", []TaxBalance{{ID: "rate", Kind: Asset, CarryingAmount: testMoney(t, "1", "USD"), TaxBase: testMoney(t, "0", "USD"), Rate: TaxRate{Value: decimal.RequireFromString("0.2")}}}, testPeriod(Quarterly), ErrRateNotEnacted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Measure(tt.rows, tt.period)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Measure() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestReconcilePermanentDifferencesAndEffectiveRate(t *testing.T) {
	report, err := Reconcile(TaxReconciliationInput{
		Period:                    testPeriod(Annual),
		AccountingProfitBeforeTax: testMoney(t, "1000", "USD"),
		StatutoryRate:             testRate("0.20"),
		PermanentDifferences: []PermanentDifference{
			{Description: "nondeductible penalty", Amount: testMoney(t, "50", "USD")},
			{Description: "exempt income", Amount: testMoney(t, "-20", "USD")},
		},
		CurrentTaxExpense:  testMoney(t, "215", "USD"),
		DeferredTaxExpense: testMoney(t, "30", "USD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for label, gotWant := range map[string][2]string{
		"expected":          {report.ExpectedTaxExpense.Amount().String(), "200"},
		"permanent tax":     {report.PermanentDifferencesTax.Amount().String(), "6"},
		"deferred movement": {report.TemporaryDifferencesChange.Amount().String(), "30"},
		"effective expense": {report.EffectiveTaxExpense.Amount().String(), "245"},
		"effective rate":    {report.EffectiveTaxRate.String(), "0.245"},
	} {
		if gotWant[0] != gotWant[1] {
			t.Errorf("%s = %s, want %s", label, gotWant[0], gotWant[1])
		}
	}
	if !report.EffectiveTaxRateAvailable {
		t.Fatal("effective tax rate should be available")
	}
}

func TestReconcileZeroAndNegativeProfitRateCases(t *testing.T) {
	zeroReport, err := Reconcile(TaxReconciliationInput{
		Period: testPeriod(Quarterly), AccountingProfitBeforeTax: testMoney(t, "0", "USD"),
		StatutoryRate: testRate("0.2"), CurrentTaxExpense: testMoney(t, "5", "USD"),
		DeferredTaxExpense: testMoney(t, "-2", "USD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if zeroReport.EffectiveTaxRateAvailable || !zeroReport.EffectiveTaxRate.IsZero() {
		t.Fatalf("zero-profit effective rate should be unavailable, got %s", zeroReport.EffectiveTaxRate)
	}
	lossReport, err := Reconcile(TaxReconciliationInput{
		Period: testPeriod(Quarterly), AccountingProfitBeforeTax: testMoney(t, "-100", "USD"),
		StatutoryRate: testRate("0.2"), CurrentTaxExpense: testMoney(t, "10", "USD"),
		DeferredTaxExpense: testMoney(t, "0", "USD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !lossReport.EffectiveTaxRateAvailable || lossReport.EffectiveTaxRate.String() != "-0.1" {
		t.Fatalf("loss effective rate = %s, available=%v", lossReport.EffectiveTaxRate, lossReport.EffectiveTaxRateAvailable)
	}
}

func TestReconcileRejectsMixedCurrencyAndUnenactedRate(t *testing.T) {
	base := TaxReconciliationInput{
		Period: testPeriod(Quarterly), AccountingProfitBeforeTax: testMoney(t, "10", "USD"),
		StatutoryRate: testRate("0.2"), CurrentTaxExpense: testMoney(t, "1", "USD"),
		DeferredTaxExpense: testMoney(t, "0", "USD"),
	}
	mixed := base
	mixed.PermanentDifferences = []PermanentDifference{{Description: "foreign", Amount: testMoney(t, "1", "EUR")}}
	if _, err := Reconcile(mixed); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("mixed currency error = %v, want %v", err, ErrCurrencyMismatch)
	}
	base.StatutoryRate.Enacted = false
	if _, err := Reconcile(base); !errors.Is(err, ErrRateNotEnacted) {
		t.Fatalf("unenacted rate error = %v, want %v", err, ErrRateNotEnacted)
	}
}

// ---------------------------------------------------------------------------
// journal_test.go (public journal.EntryLine entries)
// ---------------------------------------------------------------------------

func TestGenerateDeferredTaxEntryForIncreaseAndReversal(t *testing.T) {
	expense, dtaAccount, dtlAccount := uuid.New(), uuid.New(), uuid.New()
	base := DeferredTaxEntryInput{
		CompanyID: uuid.New(), JournalID: uuid.New(), EntryDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		CreatedBy: "tax-engine", Accounts: DeferredTaxAccounts{Expense: expense, DTA: dtaAccount, DTL: dtlAccount},
		Previous: DeferredTaxPosition{DTA: testMoney(t, "10", "USD"), DTL: testMoney(t, "20", "USD")},
		Closing:  DeferredTaxPosition{DTA: testMoney(t, "15", "USD"), DTL: testMoney(t, "27", "USD")},
	}
	entry, err := GenerateDeferredTaxEntry(base)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	if len(entry.Lines) != 4 || entry.Classification != journal.EntryClassAdjusting ||
		entry.CompanyID != types.CompanyID(base.CompanyID) || entry.JournalID != types.JournalID(base.JournalID) {
		t.Fatalf("unexpected generated entry: %+v", entry)
	}
	if entry.CreatedBy != base.CreatedBy || entry.Description != "Deferred tax measurement adjustment" ||
		entry.Status != journal.EntryDraft {
		t.Fatalf("unexpected entry header metadata: %+v", entry)
	}
	assertAccountTotal(t, entry, expense, "7", "5")
	assertAccountTotal(t, entry, dtaAccount, "5", "0")
	assertAccountTotal(t, entry, dtlAccount, "0", "7")

	base.Previous = base.Closing
	base.Closing = DeferredTaxPosition{DTA: testMoney(t, "8", "USD"), DTL: testMoney(t, "11", "USD")}
	reversal, err := GenerateDeferredTaxEntry(base)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, reversal)
	assertAccountTotal(t, reversal, expense, "7", "16")
	assertAccountTotal(t, reversal, dtaAccount, "0", "7")
	assertAccountTotal(t, reversal, dtlAccount, "16", "0")
}

func TestGenerateDeferredTaxEntryRejectsInvalidAndZeroMovements(t *testing.T) {
	input := DeferredTaxEntryInput{
		CompanyID: uuid.New(), JournalID: uuid.New(), EntryDate: time.Now().UTC(), CreatedBy: "tax-engine",
		Previous: DeferredTaxPosition{DTA: testMoney(t, "1", "USD"), DTL: testMoney(t, "0", "USD")},
		Closing:  DeferredTaxPosition{DTA: testMoney(t, "1", "USD"), DTL: testMoney(t, "0", "USD")},
		Accounts: DeferredTaxAccounts{Expense: uuid.New(), DTA: uuid.New(), DTL: uuid.New()},
	}

	if _, err := GenerateDeferredTaxEntry(input); !errors.Is(err, ErrNoDeferredTaxMovement) {
		t.Fatalf("zero movement error = %v, want %v", err, ErrNoDeferredTaxMovement)
	}
	input.Closing.DTA = testMoney(t, "-1", "USD")
	if _, err := GenerateDeferredTaxEntry(input); !errors.Is(err, types.ErrNegativeAmountProhibited) {
		t.Fatalf("negative balance error = %v, want %v", err, types.ErrNegativeAmountProhibited)
	}
	input.Closing.DTA = testMoney(t, "2", "EUR")
	if _, err := GenerateDeferredTaxEntry(input); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("mixed currency error = %v, want %v", err, ErrCurrencyMismatch)
	}
}

func TestGenerateDeferredTaxEntryRoutesOCIAndCurrentTaxAccrual(t *testing.T) {
	expense, dta, dtl, oci := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	zero := testMoney(t, "0", "USD")
	entry, err := GenerateDeferredTaxEntry(DeferredTaxEntryInput{
		CompanyID: uuid.New(), JournalID: uuid.New(), EntryDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		CreatedBy: "tax-engine",
		Previous:  DeferredTaxPosition{DTA: zero, DTL: zero},
		Closing:   DeferredTaxPosition{DTA: testMoney(t, "5", "USD"), DTL: zero},
		OCIChange: DeferredTaxPosition{DTA: testMoney(t, "2", "USD"), DTL: zero},
		Accounts:  DeferredTaxAccounts{Expense: expense, DTA: dta, DTL: dtl, OCI: oci},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	assertAccountTotal(t, entry, expense, "0", "3")
	assertAccountTotal(t, entry, oci, "0", "2")
	assertAccountTotal(t, entry, dta, "5", "0")

	currentTax, err := GenerateCurrentTaxEntry(CurrentTaxEntryInput{
		CompanyID: uuid.New(), JournalID: uuid.New(), EntryDate: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		CreatedBy: "tax-engine", Amount: testMoney(t, "25", "USD"),
		ExpenseAccountID: expense, PayableAccountID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, currentTax)
	if len(currentTax.Lines) != 2 || currentTax.Description != "Current income tax accrual" ||
		currentTax.Classification != journal.EntryClassAdjusting {
		t.Fatalf("unexpected current tax entry: %+v", currentTax)
	}
	debitLine := currentTax.Lines[0]
	creditLine := currentTax.Lines[1]
	if debitLine.AccountID != types.AccountID(expense) ||
		!debitLine.FunctionalDebit.Amount().Equal(decimal.NewFromInt(25)) ||
		!debitLine.FunctionalCredit.Amount().IsZero() ||
		creditLine.AccountID != types.AccountID(currentTax.Lines[1].AccountID) ||
		!creditLine.FunctionalCredit.Amount().Equal(decimal.NewFromInt(25)) ||
		!creditLine.FunctionalDebit.Amount().IsZero() {
		t.Fatalf("unexpected current tax lines: %+v", currentTax.Lines)
	}
}

// ---------------------------------------------------------------------------
// profile_test.go (tax profiles and configured postings)
// ---------------------------------------------------------------------------

func testProfile() TaxProfile {
	return TaxProfile{
		ID: "profile-us-v1", CompanyID: uuid.New(), Jurisdiction: "US-TEST",
		Currency: "USD", Version: "v1",
		EffectiveFrom:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		SourceReference: "tax-profile-review-2025",
		Rates: []TaxRate{
			{Value: decimal.RequireFromString("0.25"), Enacted: true,
				EnactedOn:       time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
				EffectiveFrom:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				SourceReference: "law-2025"},
			{Value: decimal.RequireFromString("0.20"), Enacted: true,
				EnactedOn:       time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
				EffectiveFrom:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
				SourceReference: "law-2027"},
		},
	}
}

func TestMeasureWithProfileUsesEffectiveRatesAndBoundedRecognitionEvidence(t *testing.T) {
	profile := testProfile()
	probableEvidence := &DTARecognitionEvidence{
		Reference: "profit-forecast-v4", AssessedOn: testPeriod(Annual).AsOf, Probable: true,
		SupportedDeductibleAmount: testMoney(t, "50", "USD"),
	}
	measurement, err := MeasureWithProfile(profile, []TaxBalance{
		{
			ID: "asset-taxable", Kind: Asset,
			CarryingAmount: testMoney(t, "100", "USD"), TaxBase: testMoney(t, "70", "USD"),
			ExpectedReversalDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			CarryingAmountSource: "fixed-asset-register", TaxBaseSource: "tax-depreciation-schedule",
		},
		{
			ID: "asset-deductible", Kind: Asset,
			CarryingAmount: testMoney(t, "20", "USD"), TaxBase: testMoney(t, "100", "USD"),
			ExpectedReversalDate: time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC),
			CarryingAmountSource: "provision-ledger", TaxBaseSource: "tax-deduction-schedule",
			RecognitionEvidence: probableEvidence,
		},
	}, testPeriod(Annual))
	if err != nil {
		t.Fatal(err)
	}
	if measurement.TaxProfileID != profile.ID || measurement.Currency != "USD" ||
		measurement.CompanyID != profile.CompanyID || measurement.Jurisdiction != profile.Jurisdiction ||
		measurement.ProfileVersion != profile.Version ||
		measurement.ProfileSourceReference != profile.SourceReference {
		t.Fatalf("profile metadata = %+v", measurement)
	}
	if got := measurement.Balances[0].DTL.Amount().String(); got != "7.5" {
		t.Fatalf("2026 DTL = %s, want 7.5", got)
	}
	if got := measurement.Balances[1].DTA.Amount().String(); got != "10" {
		t.Fatalf("2027 DTA = %s, want 10 (50 supported at 20%%)", got)
	}
	if measurement.Balances[1].Rate.SourceReference != "law-2027" ||
		measurement.Balances[1].RecognitionEvidenceReference != probableEvidence.Reference ||
		measurement.Balances[1].TaxBaseSource != "tax-deduction-schedule" {
		t.Fatalf("rate or source provenance missing: %+v", measurement.Balances[1])
	}
}

func TestMeasureWithProfileRequiresDataAndDTAEvidence(t *testing.T) {
	profile := testProfile()
	deductible := TaxBalance{
		ID: "deductible", Kind: Asset,
		CarryingAmount: testMoney(t, "80", "USD"), TaxBase: testMoney(t, "100", "USD"),
		ExpectedReversalDate: testPeriod(Annual).AsOf,
		CarryingAmountSource: "ledger", TaxBaseSource: "tax-schedule",
	}
	if _, err := MeasureWithProfile(profile, []TaxBalance{deductible}, testPeriod(Annual)); !errors.Is(err, ErrMissingDTAEvidence) {
		t.Fatalf("missing evidence error = %v, want %v", err, ErrMissingDTAEvidence)
	}
	deductible.RecognitionEvidence = &DTARecognitionEvidence{
		Reference: "forecast", AssessedOn: testPeriod(Annual).AsOf.AddDate(0, 0, 1),
		Probable: true, SupportedDeductibleAmount: testMoney(t, "20", "USD"),
	}
	if _, err := MeasureWithProfile(profile, []TaxBalance{deductible}, testPeriod(Annual)); !errors.Is(err, ErrMissingDTAEvidence) {
		t.Fatalf("future-dated evidence error = %v, want %v", err, ErrMissingDTAEvidence)
	}
	deductible.RecognitionEvidence = &DTARecognitionEvidence{
		Reference: "forecast", AssessedOn: testPeriod(Annual).AsOf,
		Probable: false, SupportedDeductibleAmount: testMoney(t, "20", "USD"),
	}
	measurement, err := MeasureWithProfile(profile, []TaxBalance{deductible}, testPeriod(Annual))
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.DTA.Amount().IsZero() {
		t.Fatalf("DTA recognized without probable evidence: %s", measurement.DTA)
	}
	deductible.TaxBaseSource = ""
	if _, err := MeasureWithProfile(profile, []TaxBalance{deductible}, testPeriod(Annual)); !errors.Is(err, ErrMissingTaxProvenance) {
		t.Fatalf("missing tax-base source error = %v, want %v", err, ErrMissingTaxProvenance)
	}
}

func TestRateForRequiresRateEffectiveByReversalDate(t *testing.T) {
	profile := testProfile()
	rate, err := profile.RateFor(testPeriod(Annual).AsOf, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rate.Value.String() != "0.25" {
		t.Fatalf("rate effective in 2026 = %s, want 0.25", rate.Value)
	}
	if _, err := profile.RateFor(testPeriod(Annual).AsOf, time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrRateUnavailable) {
		t.Fatalf("unavailable rate error = %v, want %v", err, ErrRateUnavailable)
	}
}

func TestMeasureCurrentTaxUsesPeriodEffectiveRateAndSource(t *testing.T) {
	profile := testProfile()
	input := CurrentTaxInput{
		Period: testPeriod(Annual), TaxableProfit: testMoney(t, "1000", "USD"),
		SourceReference: "taxable-profit-workpaper",
	}
	measurement, err := MeasureCurrentTax(profile, input)
	if err != nil {
		t.Fatal(err)
	}
	if measurement.TaxExpense.Amount().String() != "250" ||
		measurement.Rate.SourceReference != "law-2025" ||
		measurement.SourceReference != input.SourceReference ||
		measurement.ProfileVersion != profile.Version ||
		measurement.ProfileSourceReference != profile.SourceReference {
		t.Fatalf("unexpected current tax measurement: %+v", measurement)
	}
	input.SourceReference = ""
	if _, err := MeasureCurrentTax(profile, input); !errors.Is(err, ErrMissingTaxProvenance) {
		t.Fatalf("missing current-tax source error = %v, want %v", err, ErrMissingTaxProvenance)
	}
}

func TestConfiguredEntriesRequireMappingAndApplication(t *testing.T) {
	profile := testProfile()
	current, err := MeasureCurrentTax(profile, CurrentTaxInput{
		Period: testPeriod(Annual), TaxableProfit: testMoney(t, "100", "USD"),
		SourceReference: "tax-workpaper",
	})
	if err != nil {
		t.Fatal(err)
	}
	config := ConfiguredPosting{CompanyID: profile.CompanyID}
	if _, err := GenerateConfiguredCurrentTaxEntry(current, config, uuid.New(), "tax"); !errors.Is(err, ErrPostingNotConfigured) {
		t.Fatalf("unconfigured current-tax posting error = %v, want %v", err, ErrPostingNotConfigured)
	}
	config.JournalMappingReference = "mapping-2026"
	config.ApplicationConfigurationReference = "app-tax-posting-v1"
	config.CurrentExpenseAccountID, config.CurrentPayableAccountID = uuid.New(), uuid.New()
	wrongCompany := config
	wrongCompany.CompanyID = uuid.New()
	if _, err := GenerateConfiguredCurrentTaxEntry(current, wrongCompany, uuid.New(), "tax"); !errors.Is(err, ErrPostingNotConfigured) {
		t.Fatalf("wrong-company posting error = %v, want %v", err, ErrPostingNotConfigured)
	}
	entry, err := GenerateConfiguredCurrentTaxEntry(current, config, uuid.New(), "tax")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	if entry.Description == "" ||
		!contains(entry.Description, "mapping=mapping-2026") ||
		!contains(entry.Description, "application=app-tax-posting-v1") ||
		!contains(entry.Description, "source=tax-workpaper") {
		t.Fatalf("current-tax entry lost configuration/source trace: %q", entry.Description)
	}

	deferred, err := MeasureWithProfile(profile, []TaxBalance{{
		ID: "fixed-asset", Kind: Asset,
		CarryingAmount: testMoney(t, "100", "USD"), TaxBase: testMoney(t, "70", "USD"),
		ExpectedReversalDate: time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC),
		CarryingAmountSource: "asset-register", TaxBaseSource: "tax-schedule",
	}}, testPeriod(Annual))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateConfiguredDeferredTaxEntry(deferred, DeferredTaxPosition{}, config, uuid.New(), "tax"); !errors.Is(err, ErrPostingNotConfigured) {
		t.Fatalf("unmapped deferred-tax posting error = %v, want %v", err, ErrPostingNotConfigured)
	}
	config.DeferredAccounts = DeferredTaxAccounts{
		Expense: uuid.New(), DTA: uuid.New(), DTL: uuid.New(),
	}
	entry, err = GenerateConfiguredDeferredTaxEntry(deferred, DeferredTaxPosition{}, config, uuid.New(), "tax")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	if entry.Description == "" || !contains(entry.Description, "profile=profile-us-v1") ||
		!contains(entry.Description, "application=app-tax-posting-v1") ||
		!contains(entry.Description, "mapping=mapping-2026") {
		t.Fatalf("deferred-tax entry lost profile/mapping trace: %q", entry.Description)
	}
}

func contains(value, part string) bool {
	return len(part) == 0 || len(value) >= len(part) && indexOf(value, part) >= 0
}

func indexOf(value, part string) int {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

func TestGoldenScenario_GS10_IAS12IncomeTaxes(t *testing.T) {
	companyID := uuid.New()
	profile := TaxProfile{
		ID:              "profile-gs10",
		CompanyID:       companyID,
		Jurisdiction:    "US",
		Currency:        "USD",
		Version:         "v2026",
		EffectiveFrom:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		SourceReference: "tax-code-enacted-2025",
		Rates: []TaxRate{
			{
				Value:           decimal.RequireFromString("0.20"),
				Enacted:         true,
				EnactedOn:       time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
				EffectiveFrom:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				SourceReference: "US-TR-2026",
			},
			{
				Value:           decimal.RequireFromString("0.25"),
				Enacted:         true,
				EnactedOn:       time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
				EffectiveFrom:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
				SourceReference: "US-TR-2027",
			},
		},
	}

	period := testPeriod(Annual)
	evidence := &DTARecognitionEvidence{
		Reference:                 "approved-forecast-2027",
		AssessedOn:                period.AsOf,
		Probable:                  true,
		SupportedDeductibleAmount: testMoney(t, "15000", "USD"),
	}

	balances := []TaxBalance{
		{
			ID:                   "asset-taxable-diff",
			Kind:                 Asset,
			CarryingAmount:       testMoney(t, "100000", "USD"),
			TaxBase:              testMoney(t, "60000", "USD"),
			ExpectedReversalDate: time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC),
			CarryingAmountSource: "fixed-asset-subledger",
			TaxBaseSource:        "macrs-tax-depreciation-schedule",
		},
		{
			ID:                   "liability-deductible-diff",
			Kind:                 Liability,
			CarryingAmount:       testMoney(t, "20000", "USD"),
			TaxBase:              testMoney(t, "0", "USD"),
			ExpectedReversalDate: time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC),
			CarryingAmountSource: "warranty-provision-ledger",
			TaxBaseSource:        "cash-basis-tax-rules",
			RecognitionEvidence:  evidence,
		},
	}

	measurement, err := MeasureWithProfile(profile, balances, period)
	if err != nil {
		t.Fatalf("MeasureWithProfile: %v", err)
	}
	if !measurement.DTL.Amount().Equal(decimal.NewFromInt(10000)) {
		t.Fatalf("gross DTL = %s, want 10000", measurement.DTL.Amount())
	}
	if !measurement.DTA.Amount().Equal(decimal.RequireFromString("3750")) {
		t.Fatalf("recognized DTA = %s, want 3750", measurement.DTA.Amount())
	}

	currentTax, err := MeasureCurrentTax(profile, CurrentTaxInput{
		Period:          period,
		TaxableProfit:   testMoney(t, "50000", "USD"),
		SourceReference: "tax-return-workpaper-2026",
	})
	if err != nil {
		t.Fatalf("MeasureCurrentTax: %v", err)
	}
	if !currentTax.TaxExpense.Amount().Equal(decimal.NewFromInt(10000)) {
		t.Fatalf("current tax = %s, want 10000", currentTax.TaxExpense.Amount())
	}

	config := ConfiguredPosting{
		CompanyID:                         companyID,
		JournalMappingReference:           "mapping-gs10",
		ApplicationConfigurationReference: "tax-service-v1",
		CurrentExpenseAccountID:           uuid.New(),
		CurrentPayableAccountID:           uuid.New(),
		DeferredAccounts: DeferredTaxAccounts{
			Expense: uuid.New(),
			DTA:     uuid.New(),
			DTL:     uuid.New(),
		},
	}

	currentEntry, err := GenerateConfiguredCurrentTaxEntry(currentTax, config, uuid.New(), "tax-accountant")
	if err != nil {
		t.Fatalf("GenerateConfiguredCurrentTaxEntry: %v", err)
	}
	assertBalanced(t, currentEntry)

	deferredEntry, err := GenerateConfiguredDeferredTaxEntry(measurement, DeferredTaxPosition{}, config, uuid.New(), "tax-accountant")
	if err != nil {
		t.Fatalf("GenerateConfiguredDeferredTaxEntry: %v", err)
	}
	assertBalanced(t, deferredEntry)
}

// ---------------------------------------------------------------------------
// uncertain_test.go (IFRIC 23)
// ---------------------------------------------------------------------------

func uncertainTreatment(t *testing.T) UncertainTaxTreatment {
	t.Helper()
	return UncertainTaxTreatment{
		ID:                                "UT-2026-01",
		Description:                       "Deductibility of a research expense",
		UnitOfAccount:                     IndividualTaxUnit,
		UnitOfAccountReference:            "tax-return-line-42",
		UnitOfAccountRationale:            "The item is assessed separately because its facts and resolution are distinct.",
		ExaminationAssumptionAcknowledged: true,
		AssessmentDate:                    testPeriod(Annual).AsOf,
		AssessmentReference:               "tax-position-memo-v1",
		ReportedTaxAmount:                 testMoney(t, "100", "USD"),
		Method:                            MostLikelyAmount,
		MethodRationale:                   "The outcome is binary and one outcome best predicts resolution.",
		FactsAndCircumstancesReference:    "case-facts-v1",
		LimitationDate:                    time.Date(2029, 12, 31, 0, 0, 0, 0, time.UTC),
	}
}

func TestMeasureUncertainTaxTreatmentProbableAcceptanceUsesFiledAmount(t *testing.T) {
	treatment := uncertainTreatment(t)
	treatment.AcceptanceProbable = true
	measurement, err := MeasureUncertainTaxTreatment(treatment, testPeriod(Annual).AsOf)
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.RecognizedTaxAmount.Amount().Equal(decimal.NewFromInt(100)) ||
		!measurement.UncertaintyAdjustment.Amount().IsZero() ||
		!measurement.LiabilityIncrease.Amount().IsZero() ||
		!measurement.TaxAssetIncrease.Amount().IsZero() {
		t.Fatalf("probable acceptance should retain the filed amount without adjustment: %+v", measurement)
	}
	if measurement.Method != "" {
		t.Fatalf("method = %q, want no uncertainty method when acceptance is probable", measurement.Method)
	}
}

func TestMeasureUncertainTaxTreatmentMostLikelyAmountBinaryScenario(t *testing.T) {
	treatment := uncertainTreatment(t)
	treatment.Outcomes = []TaxOutcome{
		{ID: "accepted", TaxAmount: testMoney(t, "100", "USD"), Probability: decimal.RequireFromString("0.45"), SourceReference: "position-analysis"},
		{ID: "rejected", TaxAmount: testMoney(t, "180", "USD"), Probability: decimal.RequireFromString("0.55"), SourceReference: "position-analysis"},
	}
	measurement, err := MeasureUncertainTaxTreatment(treatment, testPeriod(Annual).AsOf)
	if err != nil {
		t.Fatal(err)
	}
	if measurement.AcceptanceProbable || measurement.Method != MostLikelyAmount ||
		measurement.SelectedOutcomeID != "rejected" ||
		measurement.RecognizedTaxAmount.Amount().String() != "180" ||
		measurement.UncertaintyAdjustment.Amount().String() != "80" ||
		measurement.LiabilityIncrease.Amount().String() != "80" ||
		!measurement.TaxAssetIncrease.Amount().IsZero() {
		t.Fatalf("unexpected most-likely-amount measurement: %+v", measurement)
	}
}

func TestMeasureUncertainTaxTreatmentExpectedValueMultipleOutcomes(t *testing.T) {
	treatment := uncertainTreatment(t)
	treatment.UnitOfAccount = GroupTaxUnit
	treatment.UnitOfAccountReference = "tax-return-lines-42-44"
	treatment.UnitOfAccountRationale = "The positions are assessed together because a combined resolution is more predictive."
	treatment.Method = ExpectedValue
	treatment.MethodRationale = "The outcomes span a broad range and expected value best predicts resolution."
	treatment.Outcomes = []TaxOutcome{
		{ID: "low", TaxAmount: testMoney(t, "100", "USD"), Probability: decimal.RequireFromString("0.2"), SourceReference: "scenario-model"},
		{ID: "middle", TaxAmount: testMoney(t, "200", "USD"), Probability: decimal.RequireFromString("0.5"), SourceReference: "scenario-model"},
		{ID: "high", TaxAmount: testMoney(t, "400", "USD"), Probability: decimal.RequireFromString("0.3"), SourceReference: "scenario-model"},
	}
	measurement, err := MeasureUncertainTaxTreatment(treatment, testPeriod(Annual).AsOf)
	if err != nil {
		t.Fatal(err)
	}
	if measurement.Method != ExpectedValue || measurement.SelectedOutcomeID != "" ||
		measurement.RecognizedTaxAmount.Amount().String() != "240" ||
		measurement.UncertaintyAdjustment.Amount().String() != "140" ||
		measurement.LiabilityIncrease.Amount().String() != "140" {
		t.Fatalf("unexpected expected-value measurement: %+v", measurement)
	}
}

func TestMeasureUncertainTaxTreatmentReducesReportedLiabilityWithoutInventingAsset(t *testing.T) {
	treatment := uncertainTreatment(t)
	treatment.Outcomes = []TaxOutcome{
		{ID: "partial-acceptance", TaxAmount: testMoney(t, "60", "USD"), Probability: decimal.RequireFromString("0.7"), SourceReference: "tax-analysis"},
		{ID: "rejection", TaxAmount: testMoney(t, "100", "USD"), Probability: decimal.RequireFromString("0.3"), SourceReference: "tax-analysis"},
	}
	measurement, err := MeasureUncertainTaxTreatment(treatment, testPeriod(Annual).AsOf)
	if err != nil {
		t.Fatal(err)
	}
	if measurement.UncertaintyAdjustment.Amount().String() != "-40" ||
		!measurement.LiabilityIncrease.Amount().IsZero() ||
		measurement.LiabilityDecrease.Amount().String() != "40" ||
		!measurement.TaxAssetIncrease.Amount().IsZero() {
		t.Fatalf("unexpected negative adjustment: %+v", measurement)
	}
}

func TestMeasureUncertainTaxTreatmentCalculatesTaxAssetIncrease(t *testing.T) {
	treatment := uncertainTreatment(t)
	treatment.ReportedTaxAmount = testMoney(t, "-100", "USD")
	treatment.Outcomes = []TaxOutcome{
		{ID: "higher-refund", TaxAmount: testMoney(t, "-180", "USD"), Probability: decimal.RequireFromString("0.6"), SourceReference: "refund-analysis"},
		{ID: "same-refund", TaxAmount: testMoney(t, "-100", "USD"), Probability: decimal.RequireFromString("0.4"), SourceReference: "refund-analysis"},
	}
	measurement, err := MeasureUncertainTaxTreatment(treatment, testPeriod(Annual).AsOf)
	if err != nil {
		t.Fatal(err)
	}
	if measurement.RecognizedTaxAmount.Amount().String() != "-180" ||
		measurement.UncertaintyAdjustment.Amount().String() != "-80" ||
		measurement.TaxAssetIncrease.Amount().String() != "80" ||
		!measurement.TaxAssetDecrease.Amount().IsZero() ||
		!measurement.LiabilityIncrease.Amount().IsZero() {
		t.Fatalf("unexpected tax asset measurement: %+v", measurement)
	}
}

func TestUncertainTaxTreatmentReassessmentOnFactsAndLimitation(t *testing.T) {
	previous := uncertainTreatment(t)
	previous.Outcomes = []TaxOutcome{
		{ID: "accepted", TaxAmount: testMoney(t, "100", "USD"), Probability: decimal.RequireFromString("0.45"), SourceReference: "position-analysis-v1"},
		{ID: "rejected", TaxAmount: testMoney(t, "180", "USD"), Probability: decimal.RequireFromString("0.55"), SourceReference: "position-analysis-v1"},
	}
	asOf := previous.AssessmentDate.AddDate(0, 0, 1)

	updatedForFacts := previous
	updatedForFacts.AssessmentDate = asOf
	updatedForFacts.FactsAndCircumstancesReference = "case-facts-v2"
	required, err := ReassessmentRequired(previous, updatedForFacts, asOf)
	if err != nil || !required {
		t.Fatalf("changed facts should trigger reassessment: required=%v, err=%v", required, err)
	}
	required, err = ReassessmentRequired(previous, previous, previous.LimitationDate)
	if err != nil || !required {
		t.Fatalf("limitation date should trigger reassessment: required=%v, err=%v", required, err)
	}

	updated := previous
	updated.AssessmentDate = asOf
	updated.AssessmentReference = "tax-position-memo-v2"
	updated.FactsAndCircumstancesReference = "case-facts-v2"
	updated.Outcomes = []TaxOutcome{
		{ID: "accepted", TaxAmount: testMoney(t, "100", "USD"), Probability: decimal.RequireFromString("0.6"), SourceReference: "position-analysis-v2"},
		{ID: "rejected", TaxAmount: testMoney(t, "180", "USD"), Probability: decimal.RequireFromString("0.4"), SourceReference: "position-analysis-v2"},
	}
	reassessment, err := ReassessUncertainTaxTreatment(previous, updated, asOf, "New authority guidance changed the acceptance assessment.")
	if err != nil {
		t.Fatal(err)
	}
	if reassessment.PreviousMeasurement.RecognizedTaxAmount.Amount().String() != "180" ||
		reassessment.UpdatedMeasurement.RecognizedTaxAmount.Amount().String() != "100" ||
		reassessment.UpdatedMeasurement.UncertaintyAdjustment.Amount().String() != "0" ||
		reassessment.UpdatedTreatment.ReassessmentReason == "" ||
		reassessment.PreviousTreatment.FactsAndCircumstancesReference == reassessment.UpdatedTreatment.FactsAndCircumstancesReference {
		t.Fatalf("reassessment did not preserve both assessments: %+v", reassessment)
	}

	updated.LimitationDate = updated.LimitationDate.AddDate(0, 0, 1)
	required, err = ReassessmentRequired(previous, updated, asOf)
	if err != nil || !required {
		t.Fatalf("changed limitation date should trigger reassessment: required=%v, err=%v", required, err)
	}
}

func TestUncertainTaxTreatmentRejectsMissingEvidenceAndInvalidOutcomes(t *testing.T) {
	base := uncertainTreatment(t)
	base.Outcomes = []TaxOutcome{
		{ID: "first", TaxAmount: testMoney(t, "100", "USD"), Probability: decimal.RequireFromString("0.5"), SourceReference: "scenarios"},
		{ID: "second", TaxAmount: testMoney(t, "200", "USD"), Probability: decimal.RequireFromString("0.5"), SourceReference: "scenarios"},
	}
	tests := []struct {
		name   string
		mutate func(*UncertainTaxTreatment)
		want   error
	}{
		{
			name: "missing full-knowledge assumption",
			mutate: func(value *UncertainTaxTreatment) {
				value.ExaminationAssumptionAcknowledged = false
			},
			want: ErrInvalidUncertainTaxTreatment,
		},
		{
			name: "probabilities do not sum to one",
			mutate: func(value *UncertainTaxTreatment) {
				value.Outcomes[0].Probability = decimal.RequireFromString("0.4")
			},
			want: ErrInvalidTaxOutcomes,
		},
		{
			name: "ambiguous most likely outcome",
			mutate: func(value *UncertainTaxTreatment) {
				value.Outcomes[0].Probability = decimal.RequireFromString("0.5")
				value.Outcomes[1].Probability = decimal.RequireFromString("0.5")
			},
			want: ErrInvalidTaxOutcomes,
		},
		{
			name: "mixed outcome currencies",
			mutate: func(value *UncertainTaxTreatment) {
				value.Outcomes[1].TaxAmount = testMoney(t, "200", "EUR")
			},
			want: ErrInvalidTaxOutcomes,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			treatment := base
			treatment.Outcomes = append([]TaxOutcome(nil), base.Outcomes...)
			test.mutate(&treatment)
			if _, err := MeasureUncertainTaxTreatment(treatment, testPeriod(Annual).AsOf); !errors.Is(err, test.want) {
				t.Fatalf("MeasureUncertainTaxTreatment error = %v, want %v", err, test.want)
			}
		})
	}

	probable := base
	probable.AcceptanceProbable = true
	probable.Outcomes = nil
	if _, err := MeasureUncertainTaxTreatment(probable, probable.AssessmentDate.AddDate(0, 0, -1)); !errors.Is(err, ErrInvalidUncertainTaxTreatment) {
		t.Fatalf("future assessment error = %v, want %v", err, ErrInvalidUncertainTaxTreatment)
	}
}

// ---------------------------------------------------------------------------
// disclosures_test.go
// ---------------------------------------------------------------------------

func testTaxReconciliationDisclosureInput(t *testing.T) TaxReconciliationDisclosureInput {
	t.Helper()
	period := testPeriod(Annual)
	treatment := uncertainTreatment(t)
	treatment.Outcomes = []TaxOutcome{
		{ID: "full-adjustment", TaxAmount: testMoney(t, "120", "USD"), Probability: decimal.RequireFromString("0.6"), SourceReference: "position-scenarios"},
		{ID: "limited-adjustment", TaxAmount: testMoney(t, "80", "USD"), Probability: decimal.RequireFromString("0.4"), SourceReference: "position-scenarios"},
	}
	uncertainMeasurement, err := MeasureUncertainTaxTreatment(treatment, period.AsOf)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Reconcile(TaxReconciliationInput{
		Period: period, AccountingProfitBeforeTax: testMoney(t, "1000", "USD"),
		StatutoryRate: testRate("0.20"),
		PermanentDifferences: []PermanentDifference{
			{Description: "Non-deductible penalty", Amount: testMoney(t, "50", "USD")},
			{Description: "Exempt income", Amount: testMoney(t, "-20", "USD")},
		},
		CurrentTaxExpense: testMoney(t, "215", "USD"), DeferredTaxExpense: testMoney(t, "30", "USD"),
		UncertainTaxTreatments: []UncertainTaxMeasurement{uncertainMeasurement},
	})
	if err != nil {
		t.Fatal(err)
	}
	return TaxReconciliationDisclosureInput{
		CompanyID:   types.CompanyID(uuid.New()),
		PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   period.AsOf, Report: report,
		SourceReferences: []reporting.DisclosureSource{
			{Kind: "tax-workpaper", Reference: "tax-reconciliation-2026"},
		},
		Narrative: "Prepared from approved tax workpapers.",
	}
}

func TestBuildTaxReconciliationDisclosure(t *testing.T) {
	input := testTaxReconciliationDisclosureInput(t)
	disclosure, err := BuildTaxReconciliationDisclosure(input)
	if err != nil {
		t.Fatal(err)
	}
	if disclosure.CompanyID != input.CompanyID ||
		disclosure.Code != TaxReconciliationDisclosureCode ||
		disclosure.StandardReference != TaxReconciliationStandardReference ||
		disclosure.Currency != "USD" || disclosure.Version != TaxReconciliationDisclosureVersion ||
		len(disclosure.Source) != 2 || disclosure.Narrative != input.Narrative {
		t.Fatalf("unexpected disclosure scope or metadata: %+v", disclosure)
	}
	for key, want := range map[string]string{
		"accounting_profit_before_tax":        "1000",
		"statutory_tax_rate":                  "0.2",
		"expected_tax_expense":                "200",
		"permanent_differences_tax_effect":    "6",
		"current_tax_expense":                 "215",
		"deferred_tax_expense":                "30",
		"uncertain_tax_treatments_adjustment": "20",
		"unallocated_reconciling_difference":  "-11",
		"effective_tax_expense":               "245",
		"effective_tax_rate":                  "0.245",
	} {
		if got, ok := disclosure.Data[key].(string); !ok || got != want {
			t.Errorf("%s = %v, want %s", key, disclosure.Data[key], want)
		}
	}
	if got, ok := disclosure.Data["effective_tax_rate_available"].(bool); !ok || !got {
		t.Fatalf("effective tax rate availability = %v, want true", disclosure.Data["effective_tax_rate_available"])
	}
	if got, ok := disclosure.Data["permanent_differences"].([]map[string]string); !ok || len(got) != 2 ||
		got[0]["description"] != "Non-deductible penalty" || got[1]["tax_effect"] != "-4" {
		t.Fatalf("permanent difference details = %#v", disclosure.Data["permanent_differences"])
	}
	if got, ok := disclosure.Data["uncertain_tax_treatments"].([]map[string]any); !ok ||
		len(got) != 1 || got[0]["treatment_id"] != "UT-2026-01" ||
		got[0]["recognized_tax_position"] != "120" {
		t.Fatalf("uncertain tax treatment details = %#v", disclosure.Data["uncertain_tax_treatments"])
	}
}

func TestTaxReconciliationDisclosureProviderScopesAndRegistry(t *testing.T) {
	input := testTaxReconciliationDisclosureInput(t)
	provider, err := NewTaxReconciliationDisclosureProvider(input)
	if err != nil {
		t.Fatal(err)
	}
	query := reporting.DisclosureQuery{
		CompanyID: input.CompanyID,
		Period:    reporting.DisclosurePeriod{StartDate: input.PeriodStart, EndDate: input.PeriodEnd},
		Currency:  "usd", StandardReference: TaxReconciliationStandardReference,
		Version: TaxReconciliationDisclosureVersion,
	}
	disclosures, err := provider.GetDisclosures(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(disclosures) != 1 || disclosures[0].CompanyID != input.CompanyID {
		t.Fatalf("scoped disclosures = %+v, want one matching record", disclosures)
	}

	otherCompanyQuery := query
	otherCompanyQuery.CompanyID = types.CompanyID(uuid.New())
	otherCompanyDisclosures, err := provider.GetDisclosures(context.Background(), otherCompanyQuery)
	if err != nil || len(otherCompanyDisclosures) != 0 {
		t.Fatalf("cross-company disclosure result = %+v, err=%v", otherCompanyDisclosures, err)
	}
	wrongPeriodQuery := query
	wrongPeriodQuery.Period.EndDate = wrongPeriodQuery.Period.EndDate.AddDate(0, 0, -1)
	wrongPeriodDisclosures, err := provider.GetDisclosures(context.Background(), wrongPeriodQuery)
	if err != nil || len(wrongPeriodDisclosures) != 0 {
		t.Fatalf("cross-period disclosure result = %+v, err=%v", wrongPeriodDisclosures, err)
	}
	if _, err := provider.GetDisclosures(nil, query); !errors.Is(err, ErrInvalidTaxReconciliationDisclosure) {
		t.Fatalf("nil-context error = %v", err)
	}

	// The public standards registry validates the descriptor and records the
	// module even though the unconfigured profile leaves the capability disabled.
	registry := standards.NewRegistry()
	module := NewModule(provider)
	if err := registry.Register(module); err != nil {
		t.Fatalf("register module: %v", err)
	}
	if _, ok := registry.GetDescriptor("ias12"); !ok {
		t.Fatal("standard registry did not record the IAS 12 descriptor")
	}
	if registry.HasCapability(standards.CapDeferredTax) {
		t.Fatal("deferred tax capability must stay disabled until the host configures the profile")
	}
	registered, err := module.GetDisclosures(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 1 || registered[0].Code != TaxReconciliationDisclosureCode {
		t.Fatalf("module disclosures = %+v", registered)
	}
}

func TestBuildTaxReconciliationDisclosureRejectsInvalidScopeAndSources(t *testing.T) {
	input := testTaxReconciliationDisclosureInput(t)
	input.SourceReferences = nil
	if _, err := BuildTaxReconciliationDisclosure(input); !errors.Is(err, ErrInvalidTaxReconciliationDisclosure) {
		t.Fatalf("missing source error = %v", err)
	}
	input = testTaxReconciliationDisclosureInput(t)
	input.PeriodEnd = input.PeriodEnd.AddDate(0, 0, -1)
	if _, err := BuildTaxReconciliationDisclosure(input); !errors.Is(err, ErrInvalidTaxReconciliationDisclosure) {
		t.Fatalf("report-period mismatch error = %v", err)
	}
	input = testTaxReconciliationDisclosureInput(t)
	input.Report.UncertainTaxTreatments = append(
		input.Report.UncertainTaxTreatments,
		input.Report.UncertainTaxTreatments[0],
	)
	if _, err := BuildTaxReconciliationDisclosure(input); !errors.Is(err, ErrInvalidTaxReconciliationDisclosure) {
		t.Fatalf("duplicate uncertain tax treatment error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// module_test.go (public module contract, descriptor and posting adapter)
// ---------------------------------------------------------------------------

func TestModuleDependencies(t *testing.T) {
	module := &Module{}

	if got := module.Dependencies(); len(got) != 0 {
		t.Fatalf("Dependencies() = %v, want no module dependencies", got)
	}
	if module.Name() != "IAS 12" || module.Version() != "1.0.0" {
		t.Fatalf("module identity = %s/%s", module.Name(), module.Version())
	}
	if err := module.ValidateDependencies(nil); err == nil {
		t.Fatal("ValidateDependencies(nil) returned nil, want module ports error")
	}
	if err := module.Initialize(context.Background(), nil); err == nil {
		t.Fatal("Initialize(nil) returned nil, want module ports error")
	}
	if err := module.ValidateDependencies(validPorts()); err != nil {
		t.Fatalf("ValidateDependencies with complete module ports: %v", err)
	}
	if err := module.Initialize(context.Background(), validPorts()); err != nil {
		t.Fatalf("Initialize with complete module ports: %v", err)
	}
	if err := module.RunScheduledJobs(context.Background(), nil); err != nil {
		t.Fatalf("RunScheduledJobs: %v", err)
	}
	if err := module.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRegisterRules(t *testing.T) {
	module := &Module{}
	if err := module.RegisterRules(nil); err != nil {
		t.Fatalf("RegisterRules(nil): %v", err)
	}

	router := events.NewRouter()
	if err := module.RegisterRules(router); err != nil {
		t.Fatalf("RegisterRules(router): %v", err)
	}
	for _, eventType := range []string{"IAS12_DEFERRED_TAX", "IFRS9_ECL_PROVISION"} {
		if got := router.GetRules(eventType); len(got) != 0 {
			t.Errorf("unexpected hard-coded posting rules for %s: %+v", eventType, got)
		}
	}

	descriptor := module.Descriptor()
	if descriptor.ID != "ias12" || descriptor.Version != module.Version() {
		t.Fatalf("descriptor identity = %s/%s", descriptor.ID, descriptor.Version)
	}
	if err := descriptor.Profile.Validate(); err != nil {
		t.Fatalf("invalid standard profile: %v", err)
	}
	if descriptor.Profile.IsConfigured() {
		t.Fatal("profile must remain unconfigured until the host pins edition/amendments/date/jurisdiction")
	}
	for _, workflow := range descriptor.Workflows {
		if err := workflow.Validate(*descriptor.Profile); err != nil {
			t.Errorf("invalid workflow %q: %v", workflow.ID, err)
		}
	}
	if _, ok := interface{}(module).(standards.DisclosureProvider); !ok {
		t.Fatal("IAS 12 module does not implement DisclosureProvider")
	}
	if _, ok := interface{}(module).(standards.DescribedModule); !ok {
		t.Fatal("IAS 12 module does not implement DescribedModule")
	}
	for _, requirementID := range []string{
		"IFRIC23-UNCERTAIN-TAX-TREATMENTS-01",
		"IAS12-TAX-RATE-RECONCILIATION-01",
	} {
		found := false
		for _, requirement := range descriptor.Profile.SupportedRequirements {
			if requirement.ID == requirementID && requirement.Status == standards.RequirementLimited {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing limited supported requirement %q", requirementID)
		}
	}
	for _, requirement := range descriptor.Profile.SupportedRequirements {
		if requirement.Implementation == "" || requirement.TestEvidence == "" {
			t.Errorf("requirement %q is missing implementation or test evidence", requirement.ID)
		}
	}
}

func TestModuleProvidesTaxReconciliationDisclosure(t *testing.T) {
	periodEnd := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	report, err := Reconcile(TaxReconciliationInput{
		Period:                    MeasurementPeriod{Kind: Annual, AsOf: periodEnd},
		AccountingProfitBeforeTax: testMoney(t, "1000", "USD"),
		StatutoryRate:             testRate("0.2"),
		CurrentTaxExpense:         testMoney(t, "215", "USD"),
		DeferredTaxExpense:        testMoney(t, "30", "USD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID := types.CompanyID(uuid.New())
	input := TaxReconciliationDisclosureInput{
		CompanyID:   companyID,
		PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   periodEnd,
		Report:      report,
		SourceReferences: []reporting.DisclosureSource{
			{Kind: "tax-workpaper", Reference: "reconciliation-2026"},
		},
	}
	provider, err := NewTaxReconciliationDisclosureProvider(input)
	if err != nil {
		t.Fatal(err)
	}
	module := NewModule(provider)
	disclosures, err := module.GetDisclosures(context.Background(), reporting.DisclosureQuery{
		CompanyID: companyID,
		Period:    reporting.DisclosurePeriod{StartDate: input.PeriodStart, EndDate: input.PeriodEnd},
		Currency:  "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(disclosures) != 1 || disclosures[0].Code != TaxReconciliationDisclosureCode {
		t.Fatalf("IAS 12 module disclosures = %+v, want tax reconciliation", disclosures)
	}
	if _, err := module.GetDisclosures(nil, reporting.DisclosureQuery{}); err == nil {
		t.Fatal("nil context should be rejected")
	}
	empty := NewModule(nil)
	none, err := empty.GetDisclosures(context.Background(), reporting.DisclosureQuery{})
	if err != nil || len(none) != 0 {
		t.Fatalf("module without provider = %+v, err=%v", none, err)
	}
}

func TestIAS12PostingWorkflow(t *testing.T) {
	poster := &recordingPoster{}
	ports := &mockModulePorts{
		entries:     stubEntryReader{},
		masterData:  stubMasterDataReader{},
		application: stubApplication{},
		posting:     poster,
	}
	module := &Module{}
	if err := module.Initialize(context.Background(), ports); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	companyID := uuid.New()
	journalID := uuid.New()
	entryDate := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	ctx := publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID:    "tax-accountant",
		CompanyIDs: []types.CompanyID{types.CompanyID(companyID)},
	})

	accounts := DeferredTaxAccounts{
		Expense: uuid.New(),
		DTA:     uuid.New(),
		DTL:     uuid.New(),
	}

	t.Run("post deferred tax entry", func(t *testing.T) {
		input := DeferredTaxEntryInput{
			CompanyID: companyID,
			JournalID: journalID,
			EntryDate: entryDate,
			CreatedBy: "tax-accountant",
			Previous: DeferredTaxPosition{
				DTA: testMoney(t, "100", "USD"),
				DTL: testMoney(t, "50", "USD"),
			},
			Closing: DeferredTaxPosition{
				DTA: testMoney(t, "250", "USD"),
				DTL: testMoney(t, "80", "USD"),
			},
			Accounts: accounts,
		}

		result, err := module.PostDeferredTax(ctx, input, "TAX-POS-2026", "idem-tax-def-1")
		if err != nil {
			t.Fatalf("PostDeferredTax: %v", err)
		}
		if result.Entry.Status != journal.EntryPosted {
			t.Errorf("status = %s, want %s", result.Entry.Status, journal.EntryPosted)
		}
		if len(poster.requests) != 1 {
			t.Fatalf("posting requests = %d, want 1", len(poster.requests))
		}
		req := poster.requests[0]
		assertPostingRequest(t, req, "IAS-12-DEFERRED-TAX", "TAX-POS-2026", "idem-tax-def-1", 4)
		if req.CompanyID != types.CompanyID(companyID) || req.JournalID != types.JournalID(journalID) ||
			!req.EffectiveDate.Equal(entryDate) || !req.PostingDate.Equal(entryDate) {
			t.Fatalf("posting scope/date mapping = %+v", req)
		}
		if req.Description != "Deferred tax measurement adjustment" || req.Classification != journal.EntryClassRegular {
			t.Fatalf("posting description/classification = %q/%s", req.Description, req.Classification)
		}
	})

	t.Run("post current tax entry", func(t *testing.T) {
		input := CurrentTaxEntryInput{
			CompanyID:        companyID,
			JournalID:        journalID,
			ExpenseAccountID: uuid.New(),
			PayableAccountID: uuid.New(),
			EntryDate:        entryDate,
			CreatedBy:        "tax-accountant",
			Amount:           testMoney(t, "5000", "USD"),
		}

		result, err := module.PostCurrentTax(ctx, input, "CURR-TAX-2026", "idem-tax-curr-1")
		if err != nil {
			t.Fatalf("PostCurrentTax: %v", err)
		}
		if result.Entry.Status != journal.EntryPosted {
			t.Errorf("status = %s, want %s", result.Entry.Status, journal.EntryPosted)
		}
		if len(poster.requests) != 2 {
			t.Fatalf("posting requests = %d, want 2", len(poster.requests))
		}
		payableAccountID := input.PayableAccountID
		input.PayableAccountID = uuid.New()
		req := poster.requests[1]
		assertPostingRequest(t, req, "IAS-12-CURRENT-TAX", "CURR-TAX-2026", "idem-tax-curr-1", 2)
		if req.CompanyID != types.CompanyID(companyID) || req.JournalID != types.JournalID(journalID) ||
			!req.EffectiveDate.Equal(entryDate) || !req.PostingDate.Equal(entryDate) {
			t.Fatalf("posting scope/date mapping = %+v", req)
		}
		if req.Lines[0].AccountID != types.AccountID(input.ExpenseAccountID) ||
			!req.Lines[0].FunctionalDebit.Amount().Equal(decimal.NewFromInt(5000)) ||
			!req.Lines[0].FunctionalCredit.Amount().IsZero() ||
			req.Lines[1].AccountID != types.AccountID(payableAccountID) ||
			!req.Lines[1].FunctionalCredit.Amount().Equal(decimal.NewFromInt(5000)) ||
			!req.Lines[1].FunctionalDebit.Amount().IsZero() {
			t.Fatalf("current tax posting lines = %+v", req.Lines)
		}
	})

	t.Run("default entity reference and uninitialized module", func(t *testing.T) {
		freshPoster := &recordingPoster{}
		freshModule := NewModule(nil)
		if err := freshModule.Initialize(context.Background(), &mockModulePorts{
			entries:     stubEntryReader{},
			masterData:  stubMasterDataReader{},
			application: stubApplication{},
			posting:     freshPoster,
		}); err != nil {
			t.Fatal(err)
		}
		_, err := freshModule.PostCurrentTax(ctx, CurrentTaxEntryInput{
			CompanyID: companyID, JournalID: journalID, EntryDate: entryDate, CreatedBy: "tax-accountant",
			ExpenseAccountID: uuid.New(), PayableAccountID: uuid.New(), Amount: testMoney(t, "10", "USD"),
		}, "", "idem-default")
		if err != nil {
			t.Fatal(err)
		}
		if len(freshPoster.requests) != 1 || freshPoster.requests[0].EntityID != types.CompanyID(companyID).String()+"-current-tax" {
			t.Fatalf("default entity reference = %+v", freshPoster.requests)
		}
		uninitialized := NewModule(nil)
		if _, err := uninitialized.PostCurrentTax(ctx, CurrentTaxEntryInput{}, "ref", "key"); err == nil {
			t.Fatal("posting without initialized ports should fail")
		}
	})
}

func TestUnitFacade(t *testing.T) {
	unit := NewUnit()
	measurement, err := unit.Measure([]TaxBalance{{
		ID: "asset", Kind: Asset, CarryingAmount: testMoney(t, "100", "USD"),
		TaxBase: testMoney(t, "70", "USD"), Rate: testRate("0.25"),
	}}, testPeriod(Annual))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := unit.GenerateDeferredTaxEntry(DeferredTaxEntryInput{
		CompanyID: uuid.New(), JournalID: uuid.New(), EntryDate: testPeriod(Annual).AsOf,
		CreatedBy: "tax-engine",
		Previous:  DeferredTaxPosition{DTA: zeroMoney("USD"), DTL: zeroMoney("USD")},
		Closing:   DeferredTaxPosition{DTA: measurement.DTA, DTL: measurement.DTL},
		Accounts:  DeferredTaxAccounts{Expense: uuid.New(), DTA: uuid.New(), DTL: uuid.New()},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)

	closed, draft, err := unit.ClosePeriod(
		[]TaxBalance{{
			ID: "asset", Kind: Asset, CarryingAmount: testMoney(t, "100", "USD"),
			TaxBase: testMoney(t, "70", "USD"), Rate: testRate("0.25"),
		}},
		testPeriod(Annual), DeferredTaxPosition{},
		DeferredTaxAccounts{Expense: uuid.New(), DTA: uuid.New(), DTL: uuid.New()},
		uuid.New(), uuid.New(), "tax-engine",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !closed.DTL.Amount().Equal(decimal.RequireFromString("7.5")) {
		t.Fatalf("closed period DTL = %s, want 7.5", closed.DTL.Amount())
	}
	assertBalanced(t, draft)

	var nilUnit *Unit
	if _, err := nilUnit.Measure(nil, testPeriod(Annual)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil Unit error = %v, want %v", err, ErrInvalidInput)
	}
}

// ---------------------------------------------------------------------------
// gapred_test.go (regression cases)
// ---------------------------------------------------------------------------

func TestGAP_STD_01_NilFutureProfitMustNotRecognizeDTA(t *testing.T) {
	measurement, err := Measure([]TaxBalance{{
		ID: "deductible", Kind: Asset,
		CarryingAmount: testMoney(t, "80", "USD"), TaxBase: testMoney(t, "100", "USD"),
		Rate: TaxRate{
			Value: decimal.RequireFromString("0.25"), Enacted: true,
			EnactedOn:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}}, MeasurementPeriod{Kind: Annual, AsOf: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !measurement.DTA.Amount().IsZero() {
		t.Fatalf("GAP-STD-01: DTA = %s recognized without an explicit probable-profit determination", measurement.DTA)
	}
}

func TestGAP_STD_02_EnactedFutureRateIsUsable(t *testing.T) {
	rate := TaxRate{
		Value: decimal.RequireFromString("0.20"), Enacted: true,
		EnactedOn:     time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		EffectiveFrom: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	asOf := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if err := rate.Validate(asOf); err != nil {
		t.Fatalf("GAP-STD-02: enacted future rate rejected: %v", err)
	}

	_, err := Measure([]TaxBalance{{
		ID: "taxable", Kind: Asset,
		CarryingAmount: testMoney(t, "100", "USD"), TaxBase: testMoney(t, "70", "USD"), Rate: rate,
	}}, MeasurementPeriod{Kind: Annual, AsOf: asOf})
	if err != nil {
		t.Fatalf("GAP-STD-02: measurement with enacted future rate failed: %v", err)
	}
}
