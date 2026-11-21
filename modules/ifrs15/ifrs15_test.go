package ifrs15

import (
	"context"
	"errors"
	"strings"
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
// Local helpers and mocks (public API only).
// ---------------------------------------------------------------------------

func testMoney(t *testing.T, amount, currency string) types.Money {
	t.Helper()
	value, err := types.NewMoneyFromString(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustTestMoney(amount, currency string) types.Money {
	value, err := types.NewMoneyFromString(amount, currency)
	if err != nil {
		panic(err)
	}
	return value
}

func testObligation(start, end string, pattern RecognitionPattern, ssp string) PerformanceObligation {
	parse := func(value string) time.Time {
		date, _ := time.Parse("2006-01-02", value)
		return date
	}
	return PerformanceObligation{
		ID: uuid.New(), Description: "service", StandalonePrice: mustTestMoney(ssp, "USD"),
		RecognitionPattern: pattern, StartDate: parse(start), EndDate: parse(end),
	}
}

func testContract(t *testing.T, price string, obligations ...PerformanceObligation) Contract {
	t.Helper()
	contract, err := NewContract(uuid.New(), uuid.New(), testMoney(t, price, "USD"), obligations)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func assessedTestInput(t *testing.T, obligations ...PerformanceObligation) FiveStepAssessment {
	t.Helper()
	ids := make([]uuid.UUID, 0, len(obligations))
	for _, obligation := range obligations {
		ids = append(ids, obligation.ID)
	}
	return FiveStepAssessment{
		ContractID: uuid.New(), CustomerID: uuid.New(),
		Approved: true, EnforceableRightsIdentified: true, PaymentTermsIdentified: true,
		CommercialSubstance: true, CollectibilityProbable: true, FixedTransactionPrice: true,
		DistinctObligationIDs: ids,
		TransactionPrice:      testMoney(t, "1000.00", "USD"), Obligations: obligations,
	}
}

func testMapping() RevenueJournalMapping {
	return RevenueJournalMapping{
		CompanyID: uuid.New(), JournalID: uuid.New(), RevenueAccountID: uuid.New(),
		ContractAssetAccountID: uuid.New(), ContractLiabilityAccount: uuid.New(),
	}
}

func sumJournalLines(lines []journal.EntryLine) (decimal.Decimal, decimal.Decimal) {
	debit, credit := decimal.Zero, decimal.Zero
	for _, line := range lines {
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	return debit, credit
}

func assertBalanced(t *testing.T, entry journal.JournalEntry) {
	t.Helper()
	debit, credit := sumJournalLines(entry.Lines)
	if !debit.Equal(credit) || debit.IsZero() {
		t.Fatalf("entry is not balanced: debit=%s credit=%s", debit, credit)
	}
}

type recordingPoster struct {
	requests []standards.StandardPostingRequest
	err      error
}

func (p *recordingPoster) PostStandardEntry(_ context.Context, req standards.StandardPostingRequest) (publicapplication.EntryResult, error) {
	p.requests = append(p.requests, req)
	return publicapplication.EntryResult{}, p.err
}

type stubApplication struct{ publicapplication.Service }
type stubEntryReader struct{ standards.EntryReader }

type stubMasterData struct{}

func (stubMasterData) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{}, nil
}
func (stubMasterData) GetAccount(context.Context, types.CompanyID, types.AccountID) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (stubMasterData) GetAccountByCode(context.Context, types.CompanyID, string) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (stubMasterData) GetJournalByCode(context.Context, types.CompanyID, string) (masterdata.Journal, error) {
	return masterdata.Journal{}, nil
}

type stubPorts struct {
	poster standards.StandardPostingCapability
}

func (p stubPorts) Entries() standards.EntryReader               { return stubEntryReader{} }
func (p stubPorts) Balance() standards.BalanceReader             { return nil }
func (p stubPorts) MasterData() standards.MasterDataReader       { return stubMasterData{} }
func (p stubPorts) Application() publicapplication.Service       { return stubApplication{} }
func (p stubPorts) Posting() standards.StandardPostingCapability { return p.poster }

var _ standards.ModulePorts = stubPorts{}

type testRevenueApplication struct {
	err     error
	entries []journal.JournalEntry
	keys    []string
}

func (a *testRevenueApplication) PostRevenueEntry(_ context.Context, entry journal.JournalEntry, key string) error {
	a.entries = append(a.entries, entry)
	a.keys = append(a.keys, key)
	return a.err
}

var _ RevenuePostingApplication = (*testRevenueApplication)(nil)

type stubContractStore struct {
	ContractStore
	contracts []Contract
}

func (s stubContractStore) ListContracts(context.Context, uuid.UUID) ([]Contract, error) {
	return s.contracts, nil
}

// ---------------------------------------------------------------------------
// Five-step validation and allocation.
// ---------------------------------------------------------------------------

func TestAssessedContractValidatesFiveStepInputs(t *testing.T) {
	first := testObligation("2025-01-01", "2025-01-01", PointInTime, "800")
	second := testObligation("2025-01-01", "2025-12-31", OverTime, "400")
	input := assessedTestInput(t, first, second)
	contract, err := NewAssessedContract(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := contract.AllocatedPrices[first.ID].Amount(); !got.Equal(decimal.RequireFromString("666.67")) {
		t.Errorf("step-four allocation = %s, want 666.67", got)
	}
	if got := contract.AllocatedPrices[second.ID].Amount(); !got.Equal(decimal.RequireFromString("333.33")) {
		t.Errorf("step-four allocation = %s, want 333.33", got)
	}

	tests := []struct {
		name   string
		change func(*FiveStepAssessment)
	}{
		{"contract not approved", func(v *FiveStepAssessment) { v.Approved = false }},
		{"rights not identified", func(v *FiveStepAssessment) { v.EnforceableRightsIdentified = false }},
		{"payment terms missing", func(v *FiveStepAssessment) { v.PaymentTermsIdentified = false }},
		{"no commercial substance", func(v *FiveStepAssessment) { v.CommercialSubstance = false }},
		{"collectibility not probable", func(v *FiveStepAssessment) { v.CollectibilityProbable = false }},
		{"price not fixed", func(v *FiveStepAssessment) { v.FixedTransactionPrice = false }},
		{"variable consideration unsupported", func(v *FiveStepAssessment) { v.HasVariableConsideration = true }},
		{"financing unsupported", func(v *FiveStepAssessment) { v.HasSignificantFinancing = true }},
		{"distinct assessment missing", func(v *FiveStepAssessment) { v.DistinctObligationIDs = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := assessedTestInput(t, first)
			test.change(&invalid)
			if _, err := NewAssessedContract(invalid); !errors.Is(err, ErrRevenueAssessmentRequired) {
				t.Fatalf("NewAssessedContract error = %v, want assessment error", err)
			}
		})
	}
}

func TestAssessedContractRejectsObligationSetMismatch(t *testing.T) {
	po := testObligation("2025-01-01", "2025-01-01", PointInTime, "100")
	input := assessedTestInput(t, po)
	input.DistinctObligationIDs = []uuid.UUID{uuid.New()}
	if _, err := NewAssessedContract(input); !errors.Is(err, ErrRevenueAssessmentRequired) {
		t.Fatalf("obligation mismatch error = %v, want assessment error", err)
	}
}

func TestRelativeSSPWeightedAllocation(t *testing.T) {
	first := testObligation("2025-01-01", "2025-01-01", PointInTime, "800")
	second := testObligation("2025-01-01", "2025-12-31", OverTime, "400")
	contract := testContract(t, "1000", first, second)
	if got := contract.AllocatedPrices[first.ID].Amount(); !got.Equal(decimal.NewFromInt(66667).Shift(-2)) {
		t.Errorf("first allocation = %s, want 666.67", got)
	}
	if got := contract.AllocatedPrices[second.ID].Amount(); !got.Equal(decimal.NewFromInt(33333).Shift(-2)) {
		t.Errorf("second allocation = %s, want 333.33", got)
	}

	allocations, err := AllocateTransactionPrice(testMoney(t, "1000", "USD"), []PerformanceObligation{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if !allocations[first.ID].Equal(contract.AllocatedPrices[first.ID]) ||
		!allocations[second.ID].Equal(contract.AllocatedPrices[second.ID]) {
		t.Fatal("public allocation preview differs from contract allocation")
	}
}

func TestAllocationRoundingSumsExactly(t *testing.T) {
	obligations := []PerformanceObligation{
		testObligation("2025-01-01", "2025-01-01", PointInTime, "1"),
		testObligation("2025-01-01", "2025-01-01", PointInTime, "1"),
		testObligation("2025-01-01", "2025-01-01", PointInTime, "1"),
	}
	contract := testContract(t, "0.01", obligations...)
	total := decimal.Zero
	for _, amount := range contract.AllocatedPrices {
		total = total.Add(amount.Amount())
	}
	if !total.Equal(decimal.RequireFromString("0.01")) {
		t.Fatalf("allocation total = %s, want 0.01", total)
	}
}

// ---------------------------------------------------------------------------
// Recognition schedules, balances and journals.
// ---------------------------------------------------------------------------

func TestDailyAndMonthlyRecognitionSchedules(t *testing.T) {
	dailyObligation := testObligation("2024-01-01", "2024-12-31", OverTime, "366")
	dailyContract := testContract(t, "366", dailyObligation)
	daily, err := dailyContract.Schedule(dailyObligation.ID, Daily)
	if err != nil {
		t.Fatal(err)
	}
	exportedDaily, err := BuildRecognitionSchedule(dailyContract, dailyObligation, Daily)
	if err != nil {
		t.Fatal(err)
	}
	if len(exportedDaily) != len(daily) {
		t.Fatalf("exported daily builder returned %d entries, want %d", len(exportedDaily), len(daily))
	}
	if len(daily) != 366 || !daily[59].RecognitionDate.Equal(time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("daily schedule count/date = %d/%s", len(daily), daily[59].RecognitionDate)
	}
	dailyTotal := decimal.Zero
	for _, entry := range daily {
		dailyTotal = dailyTotal.Add(entry.AmountToRecognize.Amount())
	}
	if !dailyTotal.Equal(decimal.NewFromInt(366)) {
		t.Fatalf("daily schedule totals %s, want 366", dailyTotal)
	}

	monthlyObligation := testObligation("2024-01-15", "2024-03-10", OverTime, "100")
	monthlyContract := testContract(t, "100", monthlyObligation)
	monthly, err := monthlyContract.Schedule(monthlyObligation.ID, Monthly)
	if err != nil {
		t.Fatal(err)
	}
	exportedMonthly, err := BuildMonthlyRecognitionSchedule(monthlyContract, monthlyObligation)
	if err != nil {
		t.Fatal(err)
	}
	if len(exportedMonthly) != len(monthly) {
		t.Fatalf("exported monthly builder returned %d entries, want %d", len(exportedMonthly), len(monthly))
	}
	wantDates := []time.Time{
		time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC),
	}
	sum := decimal.Zero
	for i, entry := range monthly {
		if i < len(wantDates) && !entry.RecognitionDate.Equal(wantDates[i]) {
			t.Errorf("monthly date %d = %s, want %s", i, entry.RecognitionDate, wantDates[i])
		}
		sum = sum.Add(entry.AmountToRecognize.Amount())
	}
	if len(monthly) != 3 || !sum.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("monthly schedule = %d periods totaling %s", len(monthly), sum)
	}
	statuses := UpdateScheduleStatus(monthly, time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC))
	if statuses[0].Status != ScheduleRecognized || statuses[1].Status != ScheduleRecognized ||
		statuses[2].Status != ScheduleDeferred || monthly[0].Status != SchedulePending {
		t.Fatalf("unexpected schedule statuses: %+v", statuses)
	}

	point := testObligation("2025-06-01", "", PointInTime, "250")
	pointContract := testContract(t, "250", point)
	pointSchedule, err := BuildRecognitionSchedule(pointContract, point, Monthly)
	if err != nil || len(pointSchedule) != 1 ||
		!pointSchedule[0].AmountToRecognize.Amount().Equal(decimal.NewFromInt(250)) {
		t.Fatalf("point-in-time schedule = %+v, err=%v", pointSchedule, err)
	}
	if _, err := BuildRecognitionSchedule(dailyContract, dailyObligation, "quarterly"); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("invalid frequency error = %v, want ErrInvalidPeriod", err)
	}
}

func TestContractBalancesAndRecognitionJournals(t *testing.T) {
	po := testObligation("2025-01-01", "2025-01-31", OverTime, "100")
	contract := testContract(t, "100", po)
	contract, err := contract.ApplyBilling(testMoney(t, "60", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	asset, liability, err := contract.ContractBalances()
	if err != nil || !liability.Amount().Equal(decimal.NewFromInt(60)) || !asset.Amount().IsZero() {
		t.Fatalf("pre-recognition balances: asset=%s liability=%s err=%v", asset, liability, err)
	}
	entry, err := BuildRecognitionJournal(contract, testMoney(t, "80", "USD"), uuid.New(), time.Now(), uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	contract, err = contract.ApplyRecognition(testMoney(t, "80", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	asset, liability, err = contract.ContractBalances()
	if err != nil || !asset.Amount().Equal(decimal.NewFromInt(20)) || !liability.Amount().IsZero() {
		t.Fatalf("post-recognition balances: asset=%s liability=%s err=%v", asset, liability, err)
	}
	assetJournal, err := BuildRecognitionJournal(contract, testMoney(t, "10", "USD"), uuid.New(), time.Now(), uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, assetJournal)
	if len(assetJournal.Lines) != 2 || !assetJournal.Lines[0].FunctionalDebit.Amount().Equal(decimal.NewFromInt(10)) {
		t.Fatalf("asset recognition journal has wrong lines: %+v", assetJournal.Lines)
	}
}

func TestRejectsInvalidContractInputs(t *testing.T) {
	po := testObligation("2025-02-02", "2025-02-01", OverTime, "10")
	if _, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "10", "USD"), []PerformanceObligation{po}); err == nil {
		t.Fatal("expected invalid obligation period error")
	}

	po = testObligation("2025-01-01", "2025-01-01", PointInTime, "10")
	po.StandalonePrice = testMoney(t, "10", "EUR")
	if _, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "10", "USD"), []PerformanceObligation{po}); err == nil {
		t.Fatal("expected currency mismatch error")
	}
	po.StandalonePrice = testMoney(t, "10", "USD")
	if _, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "0", "USD"), []PerformanceObligation{po}); err == nil {
		t.Fatal("expected invalid transaction price error")
	}
	contract := testContract(t, "10", po)
	if _, err := contract.Schedule(uuid.New(), Daily); err != ErrObligationNotFound {
		t.Fatalf("Schedule error = %v, want obligation not found", err)
	}
	if _, err := contract.ApplyBilling(testMoney(t, "11", "USD")); err == nil {
		t.Fatal("expected billing over transaction price to fail")
	}
	if _, err := contract.ApplyRecognition(testMoney(t, "11", "USD")); err != ErrRecognitionExceeded {
		t.Fatalf("recognition error = %v, want recognition exceeded", err)
	}
	if _, err := contract.Schedule(po.ID, "quarterly"); err != ErrInvalidPeriod {
		t.Fatalf("frequency error = %v, want invalid period", err)
	}
}

func TestUnitCoordinatesAdvanceAndRecognitionJournals(t *testing.T) {
	po := testObligation("2025-01-01", "2025-01-01", PointInTime, "1")
	contract := testContract(t, "100", po)
	unit, err := NewUnit(UnitConfig{
		JournalID: uuid.New(), CashAccountID: uuid.New(), ContractAssetAccountID: uuid.New(),
		ReceivableAccountID: uuid.New(), ContractLiabilityAccount: uuid.New(), RevenueAccountID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := unit.AddContract(contract); err != nil {
		t.Fatal(err)
	}
	_, advance, err := unit.ReceiveAdvance(contract.ID, testMoney(t, "60", "USD"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, advance)
	updated, recognition, err := unit.RecognizeObligation(contract.ID, po.ID, testMoney(t, "80", "USD"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, recognition)
	if len(recognition.Lines) != 3 {
		t.Fatalf("recognition journal lines = %d, want liability, asset, revenue", len(recognition.Lines))
	}
	if !updated.RecognizedAmount.Amount().Equal(decimal.NewFromInt(80)) {
		t.Fatalf("recognized amount = %s, want 80", updated.RecognizedAmount)
	}
	_, invoice, err := unit.BillCustomer(contract.ID, testMoney(t, "40", "USD"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, invoice)
	if _, _, err := unit.RecognizeObligation(contract.ID, po.ID, testMoney(t, "21", "USD"), time.Now()); err != ErrRecognitionExceeded {
		t.Fatalf("recognition beyond obligation allocation error = %v, want %v", err, ErrRecognitionExceeded)
	}
}

// ---------------------------------------------------------------------------
// Variable consideration.
// ---------------------------------------------------------------------------

func TestVariableConsiderationConstraining(t *testing.T) {
	outcomes := []ConsiderationOutcome{
		{Amount: mustTestMoney("10000.00", "USD"), Probability: decimal.RequireFromString("0.60")},
		{Amount: mustTestMoney("5000.00", "USD"), Probability: decimal.RequireFromString("0.30")},
		{Amount: mustTestMoney("0.00", "USD"), Probability: decimal.RequireFromString("0.10")},
	}
	estimate, err := EstimateVariableConsideration(ExpectedValue, outcomes, false, mustTestMoney("0", "USD"), "", time.Now().UTC())
	if err != nil {
		t.Fatalf("EstimateVariableConsideration failed: %v", err)
	}
	if !estimate.EstimatedAmount.Amount().Equal(decimal.RequireFromString("7500.00")) ||
		!estimate.FinalAmount.Amount().Equal(decimal.RequireFromString("7500.00")) {
		t.Fatalf("expected value estimate = %s / %s, want 7500.00", estimate.EstimatedAmount, estimate.FinalAmount)
	}

	limit := mustTestMoney("3000.00", "USD")
	constrained, err := EstimateVariableConsideration(ExpectedValue, outcomes, true, limit, "reversal risk", time.Now().UTC())
	if err != nil {
		t.Fatalf("EstimateVariableConsideration with constraint failed: %v", err)
	}
	if !constrained.IsConstrained || !constrained.EstimatedAmount.Amount().Equal(decimal.RequireFromString("7500.00")) ||
		!constrained.FinalAmount.Amount().Equal(decimal.RequireFromString("3000.00")) {
		t.Fatalf("unexpected constrained estimate: %+v", constrained)
	}
	tooHigh := mustTestMoney("8000.00", "USD")
	if _, err := EstimateVariableConsideration(ExpectedValue, outcomes, true, tooHigh, "", time.Now().UTC()); !errors.Is(err, ErrVariableConstraintExceeded) {
		t.Fatalf("expected ErrVariableConstraintExceeded, got %v", err)
	}
	if _, err := EstimateVariableConsideration(ExpectedValue, nil, false, limit, "", time.Now().UTC()); !errors.Is(err, ErrInvalidVariableConsideration) {
		t.Fatalf("expected ErrInvalidVariableConsideration, got %v", err)
	}
	badProbabilities := []ConsiderationOutcome{
		{Amount: mustTestMoney("100.00", "USD"), Probability: decimal.RequireFromString("0.50")},
		{Amount: mustTestMoney("100.00", "USD"), Probability: decimal.RequireFromString("0.20")},
	}
	if _, err := EstimateVariableConsideration(ExpectedValue, badProbabilities, false, limit, "", time.Now().UTC()); !errors.Is(err, ErrInvalidProbabilitySum) {
		t.Fatalf("expected ErrInvalidProbabilitySum, got %v", err)
	}
}

func TestVariableConsiderationSubsequentReassessment(t *testing.T) {
	obl := PerformanceObligation{
		ID: uuid.New(), Description: "Consulting Services", StandalonePrice: testMoney(t, "1000.00", "USD"),
		RecognitionPattern: OverTime,
		StartDate:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:            time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	basePrice := testMoney(t, "1000.00", "USD")
	contract, err := NewContract(uuid.New(), uuid.New(), basePrice, []PerformanceObligation{obl})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := []ConsiderationOutcome{
		{Amount: mustTestMoney("500.00", "USD"), Probability: decimal.RequireFromString("0.80")},
		{Amount: mustTestMoney("0.00", "USD"), Probability: decimal.RequireFromString("0.20")},
	}
	limit := mustTestMoney("200.00", "USD")
	initial, err := EstimateVariableConsideration(MostLikelyAmount, outcomes, true, limit, "initial", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := ReassessVariableConsideration(contract, basePrice, initial)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.TransactionPrice.Amount().Equal(decimal.RequireFromString("1200.00")) || updated.Status != ContractStatusModified {
		t.Fatalf("reassessed contract = %s / %s", updated.TransactionPrice, updated.Status)
	}
	unconstrained, err := EstimateVariableConsideration(MostLikelyAmount, outcomes, false, limit, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	final, err := ReassessVariableConsideration(updated, basePrice, unconstrained)
	if err != nil {
		t.Fatal(err)
	}
	if !final.TransactionPrice.Amount().Equal(decimal.RequireFromString("1500.00")) {
		t.Fatalf("final price = %s, want 1500.00", final.TransactionPrice)
	}
}

// ---------------------------------------------------------------------------
// Significant financing component.
// ---------------------------------------------------------------------------

func TestFinancingComponentCustomerFinancingAdvancePayment(t *testing.T) {
	contractID := uuid.New()
	nominal := testMoney(t, "100000.00", "USD")
	rate := decimal.RequireFromString("0.06")
	paymentDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	transferDate := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)

	fc, err := CalculateFinancingComponent(contractID, nominal, rate, transferDate, paymentDate, CustomerFinancing)
	if err != nil {
		t.Fatalf("CalculateFinancingComponent failed: %v", err)
	}
	if fc.PracticalExpedient || !fc.TransactionPrice.Amount().Equal(decimal.RequireFromString("112360.00")) ||
		!fc.TotalInterest.Amount().Equal(decimal.RequireFromString("12360.00")) || len(fc.Schedule) != 2 {
		t.Fatalf("unexpected financing component: %+v", fc)
	}
	p1 := fc.Schedule[0]
	if !p1.BeginningAmount.Amount().Equal(decimal.RequireFromString("100000.00")) ||
		!p1.InterestAmount.Amount().Equal(decimal.RequireFromString("6000.00")) ||
		!p1.EndingAmount.Amount().Equal(decimal.RequireFromString("106000.00")) {
		t.Fatalf("unexpected first financing period: %+v", p1)
	}

	journalID := uuid.New()
	interestExpense := uuid.New()
	contractLiability := uuid.New()
	customerID := uuid.New()
	entry, err := BuildFinancingInterestJournal(fc, p1, journalID, p1.EndDate, interestExpense, contractLiability, customerID)
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	if len(entry.Lines) != 2 || entry.Lines[0].AccountID != types.AccountID(interestExpense) ||
		!entry.Lines[0].FunctionalDebit.Amount().Equal(decimal.RequireFromString("6000.00")) ||
		entry.Lines[1].AccountID != types.AccountID(contractLiability) ||
		!entry.Lines[1].FunctionalCredit.Amount().Equal(decimal.RequireFromString("6000.00")) {
		t.Fatalf("unexpected financing interest journal: %+v", entry.Lines)
	}
	if entry.Lines[0].PartnerID == nil || *entry.Lines[0].PartnerID != types.PartnerID(customerID) {
		t.Fatal("financing journal line is missing the customer partner reference")
	}
}

func TestFinancingComponentPracticalExpedientAndArrears(t *testing.T) {
	transfer := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	shortPayment := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	longPayment := time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
	if !IsPracticalExpedientApplicable(transfer, shortPayment) || IsPracticalExpedientApplicable(transfer, longPayment) {
		t.Fatal("practical expedient applicability is incorrect")
	}
	nominal := testMoney(t, "100000.00", "USD")
	short, err := CalculateFinancingComponent(uuid.New(), nominal, decimal.RequireFromString("0.05"), transfer, shortPayment, EntityFinancing)
	if err != nil || !short.PracticalExpedient || !short.TransactionPrice.Equal(nominal) {
		t.Fatalf("short-dated financing = %+v, err=%v", short, err)
	}

	arrearsNominal := testMoney(t, "121000.00", "USD")
	arrears, err := CalculateFinancingComponent(uuid.New(), arrearsNominal, decimal.RequireFromString("0.10"), transfer, longPayment, EntityFinancing)
	if err != nil || !arrears.TransactionPrice.Amount().Equal(decimal.RequireFromString("100000.00")) ||
		!arrears.TotalInterest.Amount().Equal(decimal.RequireFromString("21000.00")) {
		t.Fatalf("entity financing = %+v, err=%v", arrears, err)
	}
	p1 := arrears.Schedule[0]
	entry, err := BuildFinancingInterestJournal(arrears, p1, uuid.New(), p1.EndDate, uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
	if !entry.Lines[0].FunctionalDebit.Amount().Equal(decimal.RequireFromString("10000.00")) {
		t.Fatalf("first arrears interest debit = %s", entry.Lines[0].FunctionalDebit)
	}

	if _, err := CalculateFinancingComponent(uuid.New(), nominal, decimal.RequireFromString("-0.05"), transfer, longPayment, EntityFinancing); !errors.Is(err, ErrInvalidDiscountRate) {
		t.Fatalf("negative rate error = %v", err)
	}
	if _, err := CalculateFinancingComponent(uuid.New(), nominal, decimal.RequireFromString("0.05"), transfer, longPayment, CustomerFinancing); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("inverted customer financing dates error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Contract modifications.
// ---------------------------------------------------------------------------

func TestContractModificationTypeASeparateContract(t *testing.T) {
	obl1 := PerformanceObligation{
		ID: uuid.New(), Description: "Product A - 100 units", StandalonePrice: testMoney(t, "10000.00", "USD"),
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	existing, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "10000.00", "USD"), []PerformanceObligation{obl1})
	if err != nil {
		t.Fatal(err)
	}
	obl2 := PerformanceObligation{
		ID: uuid.New(), Description: "Product B - 50 units", StandalonePrice: testMoney(t, "5000.00", "USD"),
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	}
	mod := ContractModification{
		ID: uuid.New(), ContractID: existing.ID, Type: ModificationSeparateContract,
		Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Description: "Additional units at standalone price",
		AdditionalPrice: testMoney(t, "5000.00", "USD"), AdditionalObligations: []PerformanceObligation{obl2},
	}
	newID := uuid.New()
	original, created, err := ApplySeparateContractModification(existing, mod, newID)
	if err != nil {
		t.Fatal(err)
	}
	if !original.TransactionPrice.Amount().Equal(decimal.RequireFromString("10000.00")) || len(original.Obligations) != 1 {
		t.Fatalf("original contract was modified: %+v", original)
	}
	if created.ID != newID || !created.TransactionPrice.Amount().Equal(decimal.RequireFromString("5000.00")) ||
		len(created.Obligations) != 1 || created.Obligations[0].ID != obl2.ID {
		t.Fatalf("unexpected separate contract: %+v", created)
	}
}

func TestContractModificationTypeBProspective(t *testing.T) {
	obl1 := PerformanceObligation{
		ID: uuid.New(), Description: "Batch 1", StandalonePrice: testMoney(t, "6000.00", "USD"),
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	obl2 := PerformanceObligation{
		ID: uuid.New(), Description: "Batch 2", StandalonePrice: testMoney(t, "6000.00", "USD"),
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
	}
	contract, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "12000.00", "USD"), []PerformanceObligation{obl1, obl2})
	if err != nil {
		t.Fatal(err)
	}
	contract, err = contract.ApplyRecognition(testMoney(t, "6000.00", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	contract.RecognizedPrices[obl1.ID] = testMoney(t, "6000.00", "USD")

	obl3 := PerformanceObligation{
		ID: uuid.New(), Description: "Batch 3", StandalonePrice: testMoney(t, "3000.00", "USD"),
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
	}
	mod := ContractModification{
		ID: uuid.New(), ContractID: contract.ID, Type: ModificationProspective,
		Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), AdditionalPrice: testMoney(t, "2400.00", "USD"),
		AdditionalObligations: []PerformanceObligation{obl3},
	}
	updated, err := ApplyProspectiveModification(contract, mod)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.RecognizedAmount.Amount().Equal(decimal.RequireFromString("6000.00")) ||
		!updated.TransactionPrice.Amount().Equal(decimal.RequireFromString("14400.00")) ||
		updated.Status != ContractStatusModified {
		t.Fatalf("unexpected prospective contract: %+v", updated)
	}
	if !updated.AllocatedPrices[obl2.ID].Amount().Equal(decimal.RequireFromString("5600.00")) ||
		!updated.AllocatedPrices[obl3.ID].Amount().Equal(decimal.RequireFromString("2800.00")) {
		t.Fatalf("unexpected remaining allocation: %+v", updated.AllocatedPrices)
	}
}

func TestContractModificationTypeCCumulativeCatchUp(t *testing.T) {
	obl := PerformanceObligation{
		ID: uuid.New(), Description: "Construction", StandalonePrice: testMoney(t, "1000000.00", "USD"),
		RecognitionPattern: OverTime,
		StartDate:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:            time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	contract, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "1000000.00", "USD"), []PerformanceObligation{obl})
	if err != nil {
		t.Fatal(err)
	}
	contract, err = contract.ApplyRecognition(testMoney(t, "600000.00", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	progress := decimal.RequireFromString("0.60")
	mod := ContractModification{
		ID: uuid.New(), ContractID: contract.ID, Type: ModificationCumulativeCatchUp,
		Date: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), AdditionalPrice: testMoney(t, "200000.00", "USD"),
		UpdatedProgress: &progress,
	}
	revenueAccount := uuid.New()
	assetAccount := uuid.New()
	liabilityAccount := uuid.New()
	updated, entry, err := ApplyCumulativeCatchUpModification(contract, mod, uuid.New(), revenueAccount, assetAccount, liabilityAccount)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.TransactionPrice.Amount().Equal(decimal.RequireFromString("1200000.00")) ||
		!updated.RecognizedAmount.Amount().Equal(decimal.RequireFromString("720000.00")) ||
		updated.Status != ContractStatusModified {
		t.Fatalf("unexpected catch-up contract: %+v", updated)
	}
	assertBalanced(t, entry)
	wantCatchUp := decimal.RequireFromString("120000.00")
	if len(entry.Lines) != 2 || entry.Lines[0].AccountID != types.AccountID(assetAccount) ||
		!entry.Lines[0].FunctionalDebit.Amount().Equal(wantCatchUp) ||
		entry.Lines[1].AccountID != types.AccountID(revenueAccount) ||
		!entry.Lines[1].FunctionalCredit.Amount().Equal(wantCatchUp) {
		t.Fatalf("unexpected catch-up journal: %+v", entry.Lines)
	}
}

// ---------------------------------------------------------------------------
// Posting workflow through the standard posting capability.
// ---------------------------------------------------------------------------

func TestRevenueWorkflowRequiresMappingAndApplication(t *testing.T) {
	if _, err := NewRevenueWorkflow(RevenueJournalMapping{}, &testRevenueApplication{}); !errors.Is(err, ErrRevenueMappingRequired) {
		t.Fatalf("workflow with no mapping error = %v", err)
	}
	if _, err := NewRevenueWorkflow(testMapping(), nil); !errors.Is(err, ErrRevenueApplicationRequired) {
		t.Fatalf("workflow with no posting application error = %v", err)
	}
	po := testObligation("2025-01-01", "2025-01-01", PointInTime, "100")
	workflow, err := NewRevenueWorkflow(testMapping(), &testRevenueApplication{})
	if err != nil {
		t.Fatal(err)
	}
	input := assessedTestInput(t, po)
	input.Approved = false
	if _, err := workflow.AddContract(input); !errors.Is(err, ErrRevenueAssessmentRequired) {
		t.Fatalf("AddContract without five-step assessment error = %v", err)
	}
	contract, err := workflow.AddContract(assessedTestInput(t, po))
	if err != nil {
		t.Fatal(err)
	}
	contract.Obligations[0].Description = "changed after registration"
	stored, ok := workflow.Contract(contract.ID)
	if !ok || stored.Obligations[0].Description == contract.Obligations[0].Description {
		t.Fatal("returned contract mutation changed registered workflow state")
	}
}

func TestRecognitionRequiresExternalEvidenceAndRejectsEventReuse(t *testing.T) {
	po := testObligation("2025-01-01", "2025-01-01", PointInTime, "100")
	app := &testRevenueApplication{}
	workflow, err := NewRevenueWorkflow(testMapping(), app)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := workflow.AddContract(assessedTestInput(t, po))
	if err != nil {
		t.Fatal(err)
	}
	event := SatisfactionEvent{
		ID: uuid.New(), ObligationID: po.ID, Amount: testMoney(t, "10", "USD"),
		Date: time.Now(), Evidence: ServiceDeliveryEvidence{Satisfied: true, MeasureBasis: "completion"},
	}
	if _, _, err := workflow.Recognize(context.Background(), contract.ID, event); !errors.Is(err, ErrRevenueEvidenceRequired) {
		t.Fatalf("missing evidence error = %v", err)
	}
	if len(app.entries) != 0 {
		t.Fatal("missing external evidence reached posting application")
	}
	event.Evidence.EvidenceReference = "completion-record-1"
	if _, _, err := workflow.Recognize(context.Background(), contract.ID, event); err != nil {
		t.Fatal(err)
	}
	event.Amount = mustTestMoney("11", "USD")
	if _, _, err := workflow.Recognize(context.Background(), contract.ID, event); !errors.Is(err, ErrRevenueEventConflict) {
		t.Fatalf("reused event ID error = %v, want conflict", err)
	}
	if len(app.entries) != 1 {
		t.Fatalf("conflicting duplicate reached posting application %d times", len(app.entries))
	}
}

func TestRevenueWorkflowPostingThroughStandardCapability(t *testing.T) {
	ctx := context.Background()
	mapping := testMapping()
	po := testObligation("2026-01-01", "2026-01-01", PointInTime, "1000")
	event := SatisfactionEvent{
		ID: uuid.New(), ObligationID: po.ID, Amount: testMoney(t, "400.00", "USD"),
		Date: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Evidence: ServiceDeliveryEvidence{
			Satisfied: true, MeasureBasis: "delivery-acceptance", EvidenceReference: "DLV-999",
		},
	}

	t.Run("posts a balanced standard request with stable identifiers", func(t *testing.T) {
		poster := &recordingPoster{}
		module := NewModule()
		if err := module.Initialize(ctx, stubPorts{poster: poster}); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		workflow, err := module.NewRevenueWorkflow(mapping)
		if err != nil {
			t.Fatalf("NewRevenueWorkflow: %v", err)
		}
		contract, err := workflow.AddContract(assessedTestInput(t, po))
		if err != nil {
			t.Fatalf("AddContract: %v", err)
		}
		updated, entry, err := workflow.Recognize(ctx, contract.ID, event)
		if err != nil {
			t.Fatalf("Recognize: %v", err)
		}
		if !updated.RecognizedAmount.Amount().Equal(decimal.RequireFromString("400.00")) {
			t.Fatalf("recognized = %s, want 400.00", updated.RecognizedAmount)
		}
		if len(poster.requests) != 1 {
			t.Fatalf("standard posting calls = %d, want 1", len(poster.requests))
		}
		req := poster.requests[0]
		if req.StandardRef != "IFRS-15-REV-REC" {
			t.Errorf("StandardRef = %q", req.StandardRef)
		}
		wantKey := contract.ID.String() + ":" + event.ID.String()
		if req.IdempotencyKey != wantKey {
			t.Errorf("IdempotencyKey = %q, want %q", req.IdempotencyKey, wantKey)
		}
		if req.EntityID != event.ID.String() {
			t.Errorf("EntityID = %q, want %q", req.EntityID, event.ID.String())
		}
		if req.CompanyID != types.CompanyID(mapping.CompanyID) || req.JournalID != types.JournalID(mapping.JournalID) {
			t.Errorf("mapping not applied: company=%s journal=%s", req.CompanyID, req.JournalID)
		}
		if req.EffectiveDate.IsZero() || !req.PostingDate.Equal(event.Date.UTC()) {
			t.Errorf("dates not mapped: effective=%s posting=%s", req.EffectiveDate, req.PostingDate)
		}
		if req.EntryID != entry.ID || len(req.Lines) != len(entry.Lines) || len(req.Lines) < 2 {
			t.Fatalf("journal entry was not passed through: req=%+v entry=%+v", req, entry)
		}
		debit, credit := sumJournalLines(req.Lines)
		if !debit.Equal(credit) || !debit.Equal(decimal.RequireFromString("400.00")) {
			t.Fatalf("unbalanced posting request: debit=%s credit=%s", debit, credit)
		}
		if req.Lines[0].AccountID != types.AccountID(mapping.ContractAssetAccountID) ||
			req.Lines[len(req.Lines)-1].AccountID != types.AccountID(mapping.RevenueAccountID) {
			t.Fatalf("unmapped accounts in posting request: %+v", req.Lines)
		}
		for _, line := range req.Lines {
			if line.PartnerID == nil || *line.PartnerID != types.PartnerID(contract.CustomerID) {
				t.Fatalf("posting line missing customer partner: %+v", line)
			}
		}
		assertBalanced(t, entry)

		stored, err := module.ContractStore().GetContract(ctx, mapping.CompanyID, contract.ID)
		if err != nil || !stored.RecognizedAmount.Amount().Equal(decimal.RequireFromString("400.00")) {
			t.Fatalf("durable contract not committed: %+v err=%v", stored, err)
		}

		// A replay with the same event does not post again.
		again, replayEntry, err := workflow.Recognize(ctx, contract.ID, event)
		if err != nil {
			t.Fatalf("idempotent replay: %v", err)
		}
		if len(poster.requests) != 1 || replayEntry.ID != entry.ID ||
			!again.RecognizedAmount.Equal(updated.RecognizedAmount) {
			t.Fatal("idempotent replay posted twice or changed the contract")
		}
	})

	t.Run("failed posting leaves state uncommitted and reuses the retry key", func(t *testing.T) {
		poster := &recordingPoster{err: errors.New("temporary failure")}
		module := NewModule()
		if err := module.Initialize(ctx, stubPorts{poster: poster}); err != nil {
			t.Fatal(err)
		}
		workflow, err := module.NewRevenueWorkflow(mapping)
		if err != nil {
			t.Fatal(err)
		}
		contract, err := workflow.AddContract(assessedTestInput(t, po))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := workflow.Recognize(ctx, contract.ID, event); err == nil {
			t.Fatal("expected posting failure")
		}
		current, ok := workflow.Contract(contract.ID)
		if !ok || !current.RecognizedAmount.Amount().IsZero() {
			t.Fatalf("failed posting confirmed contract state: %+v found=%t", current, ok)
		}
		stored, err := module.ContractStore().GetContract(ctx, mapping.CompanyID, contract.ID)
		if err != nil || !stored.RecognizedAmount.Amount().IsZero() {
			t.Fatalf("failed posting updated durable state: %+v err=%v", stored, err)
		}

		poster.err = nil
		_, entry, err := workflow.Recognize(ctx, contract.ID, event)
		if err != nil {
			t.Fatalf("retry: %v", err)
		}
		if len(poster.requests) != 2 {
			t.Fatalf("retry posting calls = %d, want 2", len(poster.requests))
		}
		if poster.requests[0].IdempotencyKey != poster.requests[1].IdempotencyKey ||
			poster.requests[0].EntityID != poster.requests[1].EntityID ||
			poster.requests[0].EntryID != poster.requests[1].EntryID ||
			poster.requests[0].EntryID != entry.ID {
			t.Fatal("retry did not reuse entry identity, entity and idempotency key")
		}
		assertBalanced(t, entry)
	})

	t.Run("requires configured ports and posting capability", func(t *testing.T) {
		module := NewModule()
		if _, err := module.NewRevenueWorkflow(mapping); err == nil {
			t.Fatal("NewRevenueWorkflow without initialized ports succeeded")
		}
		if err := module.Initialize(ctx, stubPorts{}); err != nil {
			t.Fatalf("Initialize with nil posting capability: %v", err)
		}
		if _, err := module.NewRevenueWorkflow(mapping); err == nil {
			t.Fatal("NewRevenueWorkflow with nil posting capability succeeded")
		}
	})
}

// ---------------------------------------------------------------------------
// Durable workflow recovery.
// ---------------------------------------------------------------------------

func TestDurableWorkflowRecoveryAcrossRestart(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryContractStore()
	mapping := testMapping()
	app := &testRevenueApplication{}

	workflow1, err := NewRevenueWorkflow(mapping, app, WithWorkflowContractStore(store))
	if err != nil {
		t.Fatal(err)
	}
	obl := testObligation("2026-01-01", "2026-12-31", OverTime, "1000")
	contract, err := workflow1.AddContract(assessedTestInput(t, obl))
	if err != nil {
		t.Fatal(err)
	}
	if contract.Status != ContractStatusActive {
		t.Fatalf("initial status = %s, want Active", contract.Status)
	}
	event1 := SatisfactionEvent{
		ID: uuid.New(), ObligationID: obl.ID, Amount: testMoney(t, "400.00", "USD"),
		Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Evidence: ServiceDeliveryEvidence{
			Satisfied: true, MeasureBasis: "Q1 certification", EvidenceReference: "QC-2026-01",
		},
	}
	updated, _, err := workflow1.Recognize(ctx, contract.ID, event1)
	if err != nil || !updated.RecognizedAmount.Amount().Equal(decimal.RequireFromString("400.00")) {
		t.Fatalf("first recognition = %+v err=%v", updated, err)
	}

	// Simulate engine restart with the same durable store.
	workflow2, err := NewRevenueWorkflow(mapping, app, WithWorkflowContractStore(store))
	if err != nil {
		t.Fatal(err)
	}
	recovered, ok := workflow2.Contract(contract.ID)
	if !ok || !recovered.RecognizedAmount.Amount().Equal(decimal.RequireFromString("400.00")) {
		t.Fatalf("recovered contract = %+v found=%t", recovered, ok)
	}
	tooLarge := SatisfactionEvent{
		ID: uuid.New(), ObligationID: obl.ID, Amount: testMoney(t, "700.00", "USD"),
		Date: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Evidence: ServiceDeliveryEvidence{
			Satisfied: true, MeasureBasis: "exceeded", EvidenceReference: "QC-ERR",
		},
	}
	if _, _, err := workflow2.Recognize(ctx, contract.ID, tooLarge); !errors.Is(err, ErrRecognitionExceeded) {
		t.Fatalf("over-recognition error = %v", err)
	}
	finalEvent := SatisfactionEvent{
		ID: uuid.New(), ObligationID: obl.ID, Amount: testMoney(t, "600.00", "USD"),
		Date: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		Evidence: ServiceDeliveryEvidence{
			Satisfied: true, MeasureBasis: "final acceptance", EvidenceReference: "QC-FINAL",
		},
	}
	final, _, err := workflow2.Recognize(ctx, contract.ID, finalEvent)
	if err != nil || !final.RecognizedAmount.Amount().Equal(decimal.RequireFromString("1000.00")) ||
		final.Status != ContractStatusSatisfied {
		t.Fatalf("final contract = %+v err=%v", final, err)
	}
	persisted, err := store.GetContract(ctx, mapping.CompanyID, contract.ID)
	if err != nil || persisted.Status != ContractStatusSatisfied {
		t.Fatalf("persisted contract = %+v err=%v", persisted, err)
	}
}

// ---------------------------------------------------------------------------
// Disclosures.
// ---------------------------------------------------------------------------

func TestModuleDisclosureProvider(t *testing.T) {
	ctx := context.Background()
	module := NewModule()
	companyID := types.NewCompanyID()

	obl1 := PerformanceObligation{
		ID: uuid.New(), Description: "Hardware delivery", StandalonePrice: testMoney(t, "500.00", "USD"),
		RecognitionPattern: PointInTime, StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	c1, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "500.00", "USD"), []PerformanceObligation{obl1})
	if err != nil {
		t.Fatal(err)
	}
	c1, _ = c1.ApplyBilling(testMoney(t, "500.00", "USD"))
	c1, _ = c1.ApplyRecognition(testMoney(t, "500.00", "USD"))
	c1.RecognizedPrices[obl1.ID] = testMoney(t, "500.00", "USD")
	c1.Status = ContractStatusSatisfied
	if err := module.ContractStore().SaveContract(ctx, uuid.UUID(companyID), c1); err != nil {
		t.Fatal(err)
	}

	obl2 := PerformanceObligation{
		ID: uuid.New(), Description: "SaaS subscription", StandalonePrice: testMoney(t, "1200.00", "USD"),
		RecognitionPattern: OverTime,
		StartDate:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:            time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	}
	c2, err := NewContract(uuid.New(), uuid.New(), testMoney(t, "1200.00", "USD"), []PerformanceObligation{obl2})
	if err != nil {
		t.Fatal(err)
	}
	c2, _ = c2.ApplyBilling(testMoney(t, "600.00", "USD"))
	c2, _ = c2.ApplyRecognition(testMoney(t, "400.00", "USD"))
	c2.RecognizedPrices[obl2.ID] = testMoney(t, "400.00", "USD")
	if err := module.ContractStore().SaveContract(ctx, uuid.UUID(companyID), c2); err != nil {
		t.Fatal(err)
	}

	disclosures, err := module.GetDisclosures(ctx, reporting.DisclosureQuery{
		CompanyID: companyID,
		Period: reporting.DisclosurePeriod{
			StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndDate:   time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		},
		Currency: "USD", StandardReference: "IFRS 15",
	})
	if err != nil {
		t.Fatalf("GetDisclosures: %v", err)
	}
	if len(disclosures) != 3 {
		t.Fatalf("disclosure count = %d, want 3", len(disclosures))
	}
	byCode := make(map[string]reporting.StructuredDisclosure, len(disclosures))
	for _, disclosure := range disclosures {
		byCode[disclosure.Code] = disclosure
		if disclosure.CompanyID != companyID || disclosure.StandardReference != "IFRS 15" || disclosure.Currency != "USD" {
			t.Errorf("disclosure metadata invalid: %+v", disclosure)
		}
	}
	if got := byCode["IFRS15-REV-DISAGG"].Data; got["point_in_time_revenue"] != "500.00" ||
		got["over_time_revenue"] != "400.00" || got["total_revenue"] != "900.00" {
		t.Errorf("revenue disaggregation = %+v", got)
	}
	if got := byCode["IFRS15-CONTRACT-BALANCES"].Data; got["total_billed"] != "1100.00" ||
		got["total_recognized"] != "900.00" || got["total_contract_liabilities"] != "200.00" {
		t.Errorf("contract balances = %+v", got)
	}
	remaining := byCode["IFRS15-REMAINING-OBLIGATIONS"].Data
	if remaining["unsatisfied_amount"] != "800.00" || remaining["obligations_count"] != 1 {
		t.Errorf("remaining obligations = %+v", remaining)
	}
	other, err := module.GetDisclosures(ctx, reporting.DisclosureQuery{
		CompanyID: companyID, StandardReference: "IFRS 16",
	})
	if err != nil || len(other) != 0 {
		t.Fatalf("other-standard disclosure = %+v err=%v", other, err)
	}
}

func TestModuleDisclosureRejectsMixedCurrenciesWithoutFilter(t *testing.T) {
	ctx := context.Background()
	companyID := types.NewCompanyID()
	module := NewModule()
	for _, currency := range []string{"USD", "EUR"} {
		amount, err := types.NewMoneyFromString("100.00", currency)
		if err != nil {
			t.Fatal(err)
		}
		obligation := PerformanceObligation{
			ID: uuid.New(), Description: "Delivered service", StandalonePrice: amount,
			RecognitionPattern: PointInTime, StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		}
		contract, err := NewContract(uuid.New(), uuid.New(), amount, []PerformanceObligation{obligation})
		if err != nil {
			t.Fatal(err)
		}
		contract, err = contract.ApplyRecognition(amount)
		if err != nil {
			t.Fatal(err)
		}
		contract.RecognizedPrices[obligation.ID] = amount
		if err := module.ContractStore().SaveContract(ctx, uuid.UUID(companyID), contract); err != nil {
			t.Fatal(err)
		}
	}
	query := reporting.DisclosureQuery{CompanyID: companyID}
	if _, err := module.GetDisclosures(ctx, query); err == nil {
		t.Fatal("mixed-currency contracts disclosed without a currency filter")
	}
	query.Currency = "usd"
	disclosures, err := module.GetDisclosures(ctx, query)
	if err != nil || len(disclosures) == 0 || disclosures[0].Currency != "USD" ||
		disclosures[0].Data["total_revenue"] != "100.00" {
		t.Fatalf("filtered disclosures = %+v err=%v", disclosures, err)
	}
}

func TestModuleDisclosureReturnsInvalidContractBalanceError(t *testing.T) {
	module := NewModule(WithContractStore(stubContractStore{
		contracts: []Contract{{ID: uuid.New()}},
	}))
	_, err := module.GetDisclosures(context.Background(), reporting.DisclosureQuery{CompanyID: types.NewCompanyID()})
	if err == nil || !strings.Contains(err.Error(), "balances") {
		t.Fatalf("GetDisclosures error = %v, want contract balance validation error", err)
	}
	if _, err := module.GetDisclosures(context.Background(), reporting.DisclosureQuery{}); err == nil {
		t.Fatal("empty company ID was accepted")
	}
}

// ---------------------------------------------------------------------------
// Descriptor and workflow metadata validation.
// ---------------------------------------------------------------------------

func TestDescriptorAndWorkflowValidation(t *testing.T) {
	module := NewModule()
	if module.Name() != "IFRS 15" || module.Version() != "1.1.0" || module.Dependencies() != nil {
		t.Fatalf("unexpected module metadata: %s %s %v", module.Name(), module.Version(), module.Dependencies())
	}
	if err := module.ValidateDependencies(nil); err == nil {
		t.Fatal("ValidateDependencies(nil) succeeded")
	}
	if err := module.Initialize(context.Background(), nil); err == nil {
		t.Fatal("Initialize(nil ports) succeeded")
	}
	if err := module.RegisterRules(nil); err != nil {
		t.Fatalf("RegisterRules(nil): %v", err)
	}
	router := events.NewRouter()
	if err := module.RegisterRules(router); err != nil {
		t.Fatalf("RegisterRules(router): %v", err)
	}
	if got := router.GetRules("IFRS15_REVENUE_RECOGNITION"); len(got) != 0 {
		t.Fatalf("registered hard-coded posting rules: %+v", got)
	}

	descriptor := module.Descriptor()
	if descriptor.ID != "ifrs15" || descriptor.Profile == nil {
		t.Fatalf("unexpected descriptor: %+v", descriptor)
	}
	if err := descriptor.Profile.Validate(); err != nil {
		t.Fatalf("invalid standard profile: %v", err)
	}
	if descriptor.Profile.IsConfigured() {
		t.Fatal("unconfigured profile reported as configured")
	}
	if len(descriptor.Profile.SupportedRequirements) != 9 || len(descriptor.Profile.UnimplementedRequirements) != 2 ||
		len(descriptor.Workflows) != 8 {
		t.Fatalf("profile/workflow counts = %d/%d/%d, want 9/2/8",
			len(descriptor.Profile.SupportedRequirements), len(descriptor.Profile.UnimplementedRequirements), len(descriptor.Workflows))
	}
	for _, requirement := range descriptor.Profile.SupportedRequirements {
		if !strings.HasPrefix(requirement.Implementation, "modules/ifrs15/") ||
			!strings.HasPrefix(requirement.TestEvidence, "modules/ifrs15/ifrs15_test.go:") {
			t.Errorf("requirement %s has stale evidence: %s / %s",
				requirement.ID, requirement.Implementation, requirement.TestEvidence)
		}
	}
	for _, workflow := range descriptor.Workflows {
		if err := workflow.Validate(*descriptor.Profile); err != nil {
			t.Errorf("invalid workflow %q: %v", workflow.ID, err)
		}
	}
}
