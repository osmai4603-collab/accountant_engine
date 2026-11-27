package ifrs9

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// Local mocks for the public module ports and application service.
// ---------------------------------------------------------------------------

type fakeApplication struct{ publicapplication.Service }
type fakeEntryReader struct{ standards.EntryReader }
type fakeBalanceReader struct{ standards.BalanceReader }
type fakeMasterDataReader struct{ standards.MasterDataReader }

type testModulePorts struct {
	entries     standards.EntryReader
	balance     standards.BalanceReader
	masterData  standards.MasterDataReader
	application publicapplication.Service
	posting     standards.StandardPostingCapability
}

func (p testModulePorts) Entries() standards.EntryReader               { return p.entries }
func (p testModulePorts) Balance() standards.BalanceReader             { return p.balance }
func (p testModulePorts) MasterData() standards.MasterDataReader       { return p.masterData }
func (p testModulePorts) Application() publicapplication.Service       { return p.application }
func (p testModulePorts) Posting() standards.StandardPostingCapability { return p.posting }

func validPorts() standards.ModulePorts {
	return testModulePorts{
		entries:     fakeEntryReader{},
		balance:     fakeBalanceReader{},
		masterData:  fakeMasterDataReader{},
		application: fakeApplication{},
	}
}

type recordingPoster struct {
	requests []standards.StandardPostingRequest
}

func (p *recordingPoster) PostStandardEntry(
	_ context.Context,
	req standards.StandardPostingRequest,
) (publicapplication.EntryResult, error) {
	p.requests = append(p.requests, req)
	return publicapplication.EntryResult{}, nil
}

type applicationPostingRecorder struct {
	publicapplication.Service
	created publicapplication.CreateDraftCommand
	posted  publicapplication.PostEntryCommand
	entryID types.JournalEntryID
}

func (r *applicationPostingRecorder) CreateDraft(
	_ context.Context,
	command publicapplication.CreateDraftCommand,
) (publicapplication.EntryResult, error) {
	r.created = command
	r.entryID = types.NewJournalEntryID()
	return publicapplication.EntryResult{Entry: journal.JournalEntry{ID: r.entryID}}, nil
}

func (r *applicationPostingRecorder) Post(
	_ context.Context,
	command publicapplication.PostEntryCommand,
) (publicapplication.EntryResult, error) {
	r.posted = command
	return publicapplication.EntryResult{
		Entry: journal.JournalEntry{ID: command.EntryID, Status: journal.EntryPosted},
	}, nil
}

// ---------------------------------------------------------------------------
// Test helpers.
// ---------------------------------------------------------------------------

func mustMoney(t *testing.T, value, currency string) types.Money {
	t.Helper()
	money, err := types.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func measurementMoney(t *testing.T, value string) types.Money {
	t.Helper()
	return mustMoney(t, value, "USD")
}

func defaultMoney(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func testAsOf() time.Time {
	return time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
}

func validHostClassification(category MeasurementCategory) HostClassification {
	return HostClassification{
		MeasurementCategory: category, BusinessModelReference: "host-bm-assessment",
		SPPIAssessmentReference: "host-sppi-assessment",
	}
}

func assertBalancedLines(t *testing.T, lines []journal.EntryLine) {
	t.Helper()
	if len(lines) < 2 {
		t.Fatalf("expected at least two journal lines, got %d", len(lines))
	}
	debit, credit := journalEntryTotals(lines)
	if !debit.Equal(credit) || debit.IsZero() {
		t.Fatalf("entry lines are not balanced: debit=%s credit=%s", debit, credit)
	}
	for i, line := range lines {
		if line.ID == (types.JournalEntryLineID{}) {
			t.Errorf("line %d has no line ID", i)
		}
		if line.AccountID == (types.AccountID{}) {
			t.Errorf("line %d has no account ID", i)
		}
		if line.TransactionCurrency == "" || line.TransactionAmount.Currency() != line.TransactionCurrency {
			t.Errorf("line %d has inconsistent transaction currency: %+v", i, line)
		}
		if line.FunctionalDebit.Currency() != line.TransactionCurrency ||
			line.FunctionalCredit.Currency() != line.TransactionCurrency {
			t.Errorf("line %d has inconsistent functional currencies: %+v", i, line)
		}
	}
}

func assertLineDirection(t *testing.T, line journal.EntryLine, wantAccount types.AccountID, wantDebit bool, wantAmount string) {
	t.Helper()
	if line.AccountID != wantAccount {
		t.Fatalf("line account = %s, want %s", line.AccountID, wantAccount)
	}
	gotDebit, gotCredit := line.FunctionalDebit.Amount(), line.FunctionalCredit.Amount()
	if wantDebit {
		if !gotDebit.Equal(decimal.RequireFromString(wantAmount)) || !gotCredit.IsZero() {
			t.Fatalf("expected debit %s, got debit=%s credit=%s", wantAmount, gotDebit, gotCredit)
		}
		return
	}
	if !gotCredit.Equal(decimal.RequireFromString(wantAmount)) || !gotDebit.IsZero() {
		t.Fatalf("expected credit %s, got debit=%s credit=%s", wantAmount, gotDebit, gotCredit)
	}
}

// ---------------------------------------------------------------------------
// Classification (AC / FVOCI / FVTPL).
// ---------------------------------------------------------------------------

func TestHostClassificationCategories(t *testing.T) {
	for _, category := range []MeasurementCategory{AmortisedCost, FVOCI, FVTPL} {
		classification := validHostClassification(category)
		if err := classification.Validate(); err != nil {
			t.Errorf("category %q rejected: %v", category, err)
		}
		if !validMeasurementCategory(category) {
			t.Errorf("category %q reported invalid", category)
		}
	}
	for _, classification := range []HostClassification{
		{},
		{MeasurementCategory: MeasurementCategory("stage_1"), BusinessModelReference: "bm", SPPIAssessmentReference: "sppi"},
		{MeasurementCategory: AmortisedCost, BusinessModelReference: " ", SPPIAssessmentReference: "sppi"},
		{MeasurementCategory: AmortisedCost, BusinessModelReference: "bm", SPPIAssessmentReference: " "},
	} {
		if err := classification.Validate(); !errors.Is(err, ErrInvalidClassification) {
			t.Fatalf("classification %+v error = %v, want ErrInvalidClassification", classification, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Simplified ECL (provision matrix) tests.
// ---------------------------------------------------------------------------

func testConfig() Config {
	return Config{
		CompanyID:           uuid.New(),
		JournalID:           uuid.New(),
		ImpairmentAccountID: uuid.New(),
		AllowanceAccountID:  uuid.New(),
		Currency:            "USD",
		ProvisionMatrix: map[AgingBucket]decimal.Decimal{
			AgingCurrent: decimal.RequireFromString("0.01"),
			Aging1To30:   decimal.RequireFromString("0.02"),
			Aging31To60:  decimal.RequireFromString("0.05"),
			Aging61To90:  decimal.RequireFromString("0.10"),
			AgingOver90:  decimal.RequireFromString("0.25"),
		},
	}
}

func testExposure(companyID uuid.UUID, dueDate time.Time, amount string) Exposure {
	money, err := types.NewMoneyFromString(amount, "USD")
	if err != nil {
		panic(err)
	}
	return Exposure{Receivable: Receivable{
		CompanyID: companyID, ID: uuid.New(), OriginalAmount: money,
		ResidualAmount: money, DueDate: dueDate,
	}}
}

func TestClassifyStageBackstopsAndQualitativeFlags(t *testing.T) {
	tests := []struct {
		name      string
		days      int
		sicr      bool
		defaulted bool
		want      Stage
	}{
		{name: "current", days: 0, want: Stage1},
		{name: "30 days", days: 30, want: Stage1},
		{name: "31 days", days: 31, want: Stage2},
		{name: "90 days", days: 90, want: Stage2},
		{name: "91 days", days: 91, want: Stage3},
		{name: "qualitative SICR", days: 0, sicr: true, want: Stage2},
		{name: "default overrides SICR", days: 0, sicr: true, defaulted: true, want: Stage3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyStage(tt.days, tt.sicr, tt.defaulted); got != tt.want {
				t.Fatalf("classifyStage() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCalculateProvisionMatrixAndAllowanceAdjustment(t *testing.T) {
	config := testConfig()
	asOf := testAsOf()
	exposure := testExposure(config.CompanyID, asOf.AddDate(0, 0, -40), "1000")
	existing := measurementMoney(t, "30")

	result, err := Calculate(asOf, config, []Exposure{exposure}, existing)
	if err != nil {
		t.Fatal(err)
	}
	// Forty days overdue uses the 31-60 day matrix rate (5%).
	if got := result.RequiredAllowance.Amount().StringFixed(2); got != "50.00" {
		t.Fatalf("required allowance = %s, want 50.00", got)
	}
	if got := result.Adjustment.Amount().StringFixed(2); got != "20.00" {
		t.Fatalf("adjustment = %s, want 20.00", got)
	}
	if result.JournalEntry == nil {
		t.Fatal("expected an adjustment journal entry")
	}
	entry := result.JournalEntry
	assertBalancedLines(t, entry.Lines)
	if entry.Classification != journal.EntryClassAdjusting {
		t.Fatalf("classification = %q, want adjusting", entry.Classification)
	}
	if len(entry.Lines) != 2 {
		t.Fatalf("adjustment lines = %d, want 2", len(entry.Lines))
	}
	assertLineDirection(t, entry.Lines[0], types.AccountID(config.ImpairmentAccountID), true, "20.00")
	assertLineDirection(t, entry.Lines[1], types.AccountID(config.AllowanceAccountID), false, "20.00")
	if entry.CompanyID != types.CompanyID(config.CompanyID) || entry.JournalID != types.JournalID(config.JournalID) {
		t.Fatalf("entry company/journal mismatch: %+v", entry)
	}
	if entry.Lines[0].ID == entry.Lines[1].ID || entry.ID == (types.JournalEntryID{}) {
		t.Fatal("entry and line IDs must be distinct and deterministic")
	}
	repeated, err := Calculate(asOf, config, []Exposure{exposure}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.JournalEntry.ID != entry.ID ||
		repeated.JournalEntry.Lines[0].ID != entry.Lines[0].ID ||
		repeated.JournalEntry.Lines[1].ID != entry.Lines[1].ID {
		t.Fatal("identical calculations did not produce deterministic journal IDs")
	}

	// A larger existing allowance generates a balanced release entry.
	existing = measurementMoney(t, "70")
	result, err = Calculate(asOf, config, []Exposure{exposure}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Adjustment.Amount().StringFixed(2); got != "-20.00" {
		t.Fatalf("release adjustment = %s, want -20.00", got)
	}
	assertBalancedLines(t, result.JournalEntry.Lines)
	assertLineDirection(t, result.JournalEntry.Lines[0], types.AccountID(config.AllowanceAccountID), true, "20.00")
	assertLineDirection(t, result.JournalEntry.Lines[1], types.AccountID(config.ImpairmentAccountID), false, "20.00")
}

func TestCalculateEmptyPortfolioAndNoAdjustment(t *testing.T) {
	config := testConfig()
	existing, err := types.Zero(config.Currency)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Calculate(time.Now(), config, nil, existing)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RequiredAllowance.Amount().IsZero() ||
		!result.Adjustment.Amount().IsZero() || len(result.Receivables) != 0 {
		t.Fatalf("unexpected empty portfolio result: %+v", result)
	}
	if result.JournalEntry != nil {
		t.Fatal("zero adjustment should not produce a journal entry")
	}
}

func TestCalculateRejectsInvalidInputs(t *testing.T) {
	config := testConfig()
	asOf := testAsOf()
	zero, err := types.Zero("USD")
	if err != nil {
		t.Fatal(err)
	}
	valid := testExposure(config.CompanyID, asOf, "100")

	tests := []struct {
		name      string
		date      time.Time
		exposures []Exposure
		allowance types.Money
		wantErr   error
	}{
		{name: "zero measurement date", date: time.Time{}, allowance: zero, wantErr: ErrInvalidDate},
		{name: "nil receivable id", date: asOf, exposures: []Exposure{{Receivable: Receivable{
			CompanyID: config.CompanyID, DueDate: asOf, OriginalAmount: mustMoney(t, "10", "USD"),
			ResidualAmount: mustMoney(t, "10", "USD"),
		}}}, allowance: zero, wantErr: ErrInvalidExposure},
		{name: "nil company id", date: asOf, exposures: []Exposure{func() Exposure {
			e := valid
			e.Receivable.CompanyID = uuid.Nil
			return e
		}()}, allowance: zero, wantErr: ErrInvalidExposure},
		{name: "zero due date", date: asOf, exposures: []Exposure{func() Exposure {
			e := valid
			e.Receivable.DueDate = time.Time{}
			return e
		}()}, allowance: zero, wantErr: ErrInvalidExposure},
		{name: "negative balance", date: asOf, exposures: []Exposure{func() Exposure {
			e := valid
			e.Receivable.ResidualAmount = mustMoney(t, "-1", "USD")
			return e
		}()}, allowance: zero, wantErr: ErrInvalidExposure},
		{name: "mixed currency", date: asOf, exposures: []Exposure{func() Exposure {
			e := valid
			e.Receivable.ResidualAmount = mustMoney(t, "1", "EUR")
			return e
		}()}, allowance: zero, wantErr: ErrInvalidExposure},
		{name: "invalid allowance currency", date: asOf, allowance: mustMoney(t, "1", "EUR"), wantErr: ErrInvalidAllowance},
		{name: "negative allowance", date: asOf, allowance: mustMoney(t, "-1", "USD"), wantErr: ErrInvalidAllowance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Calculate(tt.date, config, tt.exposures, tt.allowance)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Calculate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigRejectsMissingAndOutOfRangeRates(t *testing.T) {
	config := testConfig()
	delete(config.ProvisionMatrix, AgingCurrent)
	if err := config.Validate(); !errors.Is(err, ErrMissingRate) {
		t.Fatalf("Validate() error = %v, want missing rate", err)
	}
	config = testConfig()
	config.ProvisionMatrix[AgingCurrent] = decimal.RequireFromString("1.01")
	if err := config.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("Validate() error = %v, want invalid rate", err)
	}
	config = testConfig()
	config.CompanyID = uuid.Nil
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("Validate() error = %v, want invalid configuration", err)
	}
	config = testConfig()
	config.JournalID = uuid.Nil
	if err := config.Validate(); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("Validate() error = %v, want invalid configuration", err)
	}
}

type testPortfolioSource []Exposure

func (s testPortfolioSource) ListReceivables(ctx context.Context, _ uuid.UUID) ([]Exposure, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

type testAllowanceSource types.Money

func (s testAllowanceSource) ExistingAllowance(ctx context.Context, _, _ uuid.UUID, _ string) (types.Money, error) {
	if err := ctx.Err(); err != nil {
		return types.Money{}, err
	}
	return types.Money(s), nil
}

type testAsOfAllowanceSource types.Money

func (s testAsOfAllowanceSource) ExistingAllowance(ctx context.Context, _, _ uuid.UUID, _ string) (types.Money, error) {
	if err := ctx.Err(); err != nil {
		return types.Money{}, err
	}
	return types.Money(s), nil
}

func (s testAsOfAllowanceSource) ExistingAllowanceAsOf(ctx context.Context, _, _ uuid.UUID, _ string, _ time.Time) (types.Money, error) {
	if err := ctx.Err(); err != nil {
		return types.Money{}, err
	}
	return types.Money(s), nil
}

func TestEngineLoadsUsingContextAndReturnsDeterministicResults(t *testing.T) {
	config := testConfig()
	asOf := time.Date(2026, 11, 10, 12, 0, 0, 0, time.FixedZone("local", 3*60*60))
	first := testExposure(config.CompanyID, asOf.AddDate(0, 0, -31), "100")
	second := testExposure(config.CompanyID, asOf, "100")
	zero, err := types.Zero("USD")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(config, testPortfolioSource{second, first}, testAsOfAllowanceSource(zero))
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Calculate(context.Background(), asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Receivables) != 2 || result.Receivables[0].ReceivableID.String() > result.Receivables[1].ReceivableID.String() {
		t.Fatalf("receivables not deterministically ordered: %+v", result.Receivables)
	}
	if result.Receivables[0].AgingBucket != Aging31To60 && result.Receivables[1].AgingBucket != Aging31To60 {
		t.Fatalf("no receivable mapped to the 31-60 day ageing bucket: %+v", result.Receivables)
	}
	var overdue *ReceivableResult
	for i := range result.Receivables {
		if result.Receivables[i].ReceivableID == first.Receivable.ID {
			overdue = &result.Receivables[i]
		}
	}
	if overdue == nil || overdue.Stage != Stage2 || overdue.DaysPastDue != 31 {
		t.Fatalf("unexpected stage result at 31 days: %+v", overdue)
	}
}

func TestEngineRejectsHistoricalAllowanceWithoutAsOfSource(t *testing.T) {
	config := testConfig()
	allowance, err := types.Zero(config.Currency)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(config, testPortfolioSource(nil), testAllowanceSource(allowance))
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Calculate(context.Background(), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrHistoricalAllowanceUnavailable) {
		t.Fatalf("historical Calculate() error = %v, want %v", err, ErrHistoricalAllowanceUnavailable)
	}
}

func TestCalculateSimplifiedECLDoesNotProducePostingDraft(t *testing.T) {
	config := testConfig()
	exposure := testExposure(config.CompanyID, testAsOf(), "100")
	result, err := CalculateSimplifiedECL(testAsOf(), config.ProvisionMatrix, []Exposure{exposure})
	if err != nil {
		t.Fatal(err)
	}
	if result.JournalEntry != nil {
		t.Fatal("simplified convenience API must not produce a posting draft")
	}
	if result.RequiredAllowance.Amount().StringFixed(2) != "1.00" {
		t.Fatalf("required allowance = %s, want 1.00", result.RequiredAllowance)
	}
}

// ---------------------------------------------------------------------------
// General (PD/LGD/EAD) ECL tests.
// ---------------------------------------------------------------------------

type pdByHorizon struct{}

func (pdByHorizon) ProbabilityOfDefault(_ GeneralExposure, horizon time.Duration) (decimal.Decimal, error) {
	if horizon <= 365*24*time.Hour {
		return decimal.RequireFromString("0.1"), nil
	}
	return decimal.RequireFromString("0.2"), nil
}

type fixedLGD struct{}

func (fixedLGD) LossGivenDefault(GeneralExposure) (decimal.Decimal, error) {
	return decimal.RequireFromString("0.5"), nil
}

func TestCalculateGeneralECLStageHorizonAndEIRDiscount(t *testing.T) {
	ledgerBalance := measurementMoney(t, "900")
	undrawn := measurementMoney(t, "100")
	exposure := GeneralExposure{
		ID: uuid.New(), Stage: Stage1, LedgerBalance: ledgerBalance, UndrawnCommitments: undrawn,
		RemainingLife:         2 * 365 * 24 * time.Hour,
		EffectiveInterestRate: decimal.Zero,
	}
	asOf := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stage1, err := CalculateGeneralECL(asOf, []GeneralExposure{exposure}, pdByHorizon{}, fixedLGD{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := stage1.RequiredECL.Amount().StringFixed(2); got != "50.00" {
		t.Fatalf("Stage 1 ECL = %s, want 50.00", got)
	}
	if got := stage1.Exposures[0].EAD.Amount().StringFixed(2); got != "1000.00" {
		t.Fatalf("calculated EAD = %s, want 1000.00", got)
	}
	if stage1.Exposures[0].Horizon != 365*24*time.Hour {
		t.Fatalf("Stage 1 horizon = %s, want one year", stage1.Exposures[0].Horizon)
	}
	exposure.Stage = Stage2
	stage2, err := CalculateGeneralECL(asOf, []GeneralExposure{exposure}, pdByHorizon{}, fixedLGD{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := stage2.RequiredECL.Amount().StringFixed(2); got != "100.00" {
		t.Fatalf("Stage 2 ECL = %s, want 100.00", got)
	}
	exposure.Stage = Stage1
	exposure.EffectiveInterestRate = decimal.RequireFromString("0.1")
	discounted, err := CalculateGeneralECL(asOf, []GeneralExposure{exposure}, pdByHorizon{}, fixedLGD{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := discounted.RequiredECL.Amount().StringFixed(2); got != "45.45" {
		t.Fatalf("discounted ECL = %s, want 45.45", got)
	}
}

func TestScenarioPDModelWeightsForwardLookingScenarios(t *testing.T) {
	model := ScenarioPDModel{Scenarios: []PDScenario{
		{Name: "base", Weight: decimal.RequireFromString("0.6"), PD12Month: decimal.RequireFromString("0.1"), PDLifetime: decimal.RequireFromString("0.2")},
		{Name: "downside", Weight: decimal.RequireFromString("0.4"), PD12Month: decimal.RequireFromString("0.3"), PDLifetime: decimal.RequireFromString("0.5")},
	}}
	pd, err := model.ProbabilityOfDefault(GeneralExposure{Stage: Stage1}, 365*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !pd.Equal(decimal.RequireFromString("0.18")) {
		t.Fatalf("weighted 12-month PD = %s, want 0.18", pd)
	}
	pd, err = model.ProbabilityOfDefault(GeneralExposure{Stage: Stage2}, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !pd.Equal(decimal.RequireFromString("0.32")) {
		t.Fatalf("weighted lifetime PD = %s, want 0.32", pd)
	}
}

func TestDiscountAtEIRRejectsZeroCompoundingBase(t *testing.T) {
	if _, err := DiscountAtEIR(decimal.NewFromInt(-1), 365*24*time.Hour); err != ErrInvalidRiskEstimate {
		t.Fatalf("discount at -100%% EIR error = %v, want ErrInvalidRiskEstimate", err)
	}
}

// ---------------------------------------------------------------------------
// Measurement and allowance entry tests.
// ---------------------------------------------------------------------------

func TestMeasureSubsequentPeriodByCategory(t *testing.T) {
	tests := []struct {
		name                string
		category            MeasurementCategory
		stage               Stage
		openingGross        string
		openingCarrying     string
		allowance           string
		principal           string
		fairValue           string
		rate                string
		wantInterest        string
		wantClosing         string
		wantFairValueChange string
	}{
		{
			name: "amortised cost effective interest", category: AmortisedCost, stage: Stage1,
			openingGross: "1000", principal: "100", rate: "0.1",
			wantInterest: "100.00", wantClosing: "1000.00", wantFairValueChange: "0.00",
		},
		{
			name: "fvoci interest and other comprehensive income", category: FVOCI, stage: Stage1,
			openingGross: "1000", openingCarrying: "1020", allowance: "0",
			fairValue: "1050", rate: "0.1",
			wantInterest: "100.00", wantClosing: "1050.00", wantFairValueChange: "-70.00",
		},
		{
			name: "fvtpl fair value through profit and loss", category: FVTPL, stage: Stage1,
			openingGross: "1000", openingCarrying: "1000", allowance: "0",
			fairValue: "950", rate: "0",
			wantInterest: "0.00", wantClosing: "950.00", wantFairValueChange: "-50.00",
		},
		{
			name: "stage three interest on net carrying amount", category: AmortisedCost, stage: Stage3,
			openingGross: "1000", allowance: "200", principal: "0", rate: "0.1",
			wantInterest: "80.00", wantClosing: "880.00", wantFairValueChange: "0.00",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := MeasurementInput{
				InstrumentID: uuid.New(), AsOf: testAsOf(), Classification: validHostClassification(test.category),
				Stage: test.stage, OpeningGrossCarrying: measurementMoney(t, test.openingGross),
				ExistingLossAllowance: measurementMoney(t, defaultMoney(test.allowance)),
				PrincipalRepayments:   measurementMoney(t, defaultMoney(test.principal)),
				EffectiveInterestRate: decimal.RequireFromString(test.rate), Period: 365 * 24 * time.Hour,
			}
			if test.openingCarrying != "" {
				input.OpeningCarryingAmount = measurementMoney(t, test.openingCarrying)
				input.FairValue = measurementMoney(t, test.fairValue)
			}
			result, err := MeasureSubsequentPeriod(input)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.InterestIncome.Amount().StringFixed(2); got != test.wantInterest {
				t.Errorf("interest income = %s, want %s", got, test.wantInterest)
			}
			if got := result.ClosingCarryingAmount.Amount().StringFixed(2); got != test.wantClosing {
				t.Errorf("closing carrying amount = %s, want %s", got, test.wantClosing)
			}
			if got := result.FairValueAdjustment.Amount().StringFixed(2); got != test.wantFairValueChange {
				t.Errorf("fair-value adjustment = %s, want %s", got, test.wantFairValueChange)
			}
			if len(result.InputFingerprint) != 64 {
				t.Fatalf("input fingerprint %q is not a SHA-256 digest", result.InputFingerprint)
			}
		})
	}
}

func TestMeasureSubsequentPeriodRequiresHostAssessmentAndRejectsFVTPLAllowance(t *testing.T) {
	base := MeasurementInput{
		InstrumentID: uuid.New(), AsOf: time.Now(), Classification: validHostClassification(AmortisedCost),
		Stage: Stage1, OpeningGrossCarrying: measurementMoney(t, "100"),
		ExistingLossAllowance: measurementMoney(t, "0"), PrincipalRepayments: measurementMoney(t, "0"),
		EffectiveInterestRate: decimal.Zero, Period: 24 * time.Hour,
	}
	base.Classification.SPPIAssessmentReference = " "
	if _, err := MeasureSubsequentPeriod(base); !errors.Is(err, ErrInvalidMeasurement) {
		t.Fatalf("missing host evidence error = %v, want ErrInvalidMeasurement", err)
	}
	base.Classification = validHostClassification(FVTPL)
	base.OpeningCarryingAmount = measurementMoney(t, "100")
	base.FairValue = measurementMoney(t, "100")
	base.ExistingLossAllowance = measurementMoney(t, "1")
	if _, err := MeasureSubsequentPeriod(base); !errors.Is(err, ErrInvalidMeasurement) {
		t.Fatalf("FVTPL loss allowance error = %v, want ErrInvalidMeasurement", err)
	}
}

func TestCalculateEADFromLedgerAndUndrawnCommitments(t *testing.T) {
	ead, err := CalculateEAD(measurementMoney(t, "125"), measurementMoney(t, "75"))
	if err != nil {
		t.Fatal(err)
	}
	if got := ead.Amount().StringFixed(2); got != "200.00" {
		t.Fatalf("EAD = %s, want 200.00", got)
	}
	eur := mustMoney(t, "10", "EUR")
	if _, err := CalculateEAD(measurementMoney(t, "1"), eur); !errors.Is(err, ErrInvalidGeneralExposure) {
		t.Fatalf("currency mismatch error = %v, want ErrInvalidGeneralExposure", err)
	}
}

func TestCalculateEffectiveInterestRate(t *testing.T) {
	initialDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rate, err := CalculateEffectiveInterestRate(measurementMoney(t, "1000"), initialDate, []ContractualCashFlow{
		{Date: initialDate.AddDate(1, 0, 0), Amount: measurementMoney(t, "1100")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rate.Sub(decimal.RequireFromString("0.1")).Abs().LessThan(decimal.RequireFromString("0.000000000001")) {
		t.Fatalf("effective interest rate = %s, want 0.1", rate)
	}
	_, err = CalculateEffectiveInterestRate(measurementMoney(t, "1000"), initialDate, []ContractualCashFlow{
		{Date: initialDate, Amount: measurementMoney(t, "1100")},
	})
	if !errors.Is(err, ErrInvalidContractualCashFlows) {
		t.Fatalf("invalid cash flow error = %v, want ErrInvalidContractualCashFlows", err)
	}
}

func TestBuildMeasurementEntryRoutesFairValueToCategoryAccount(t *testing.T) {
	for _, test := range []struct {
		category MeasurementCategory
		account  string
	}{
		{category: FVOCI, account: "oci"},
		{category: FVTPL, account: "pnl"},
	} {
		t.Run(string(test.category), func(t *testing.T) {
			input := MeasurementInput{
				InstrumentID: uuid.New(), AsOf: testAsOf(),
				Classification: validHostClassification(test.category), Stage: Stage1,
				OpeningGrossCarrying:  measurementMoney(t, "1000"),
				OpeningCarryingAmount: measurementMoney(t, "1000"),
				ExistingLossAllowance: measurementMoney(t, "0"),
				PrincipalRepayments:   measurementMoney(t, "0"),
				FairValue:             measurementMoney(t, "900"),
				EffectiveInterestRate: decimal.Zero, Period: 365 * 24 * time.Hour,
			}
			result, err := MeasureSubsequentPeriod(input)
			if err != nil {
				t.Fatal(err)
			}
			accounts := MeasurementAccounts{
				Asset: uuid.New(), Cash: uuid.New(), InterestIncome: uuid.New(),
				OCIReserve: uuid.New(), FairValueGainLoss: uuid.New(),
			}
			companyID, journalID := uuid.New(), uuid.New()
			first, err := BuildMeasurementEntry(result, accounts, companyID, journalID, "risk_officer")
			if err != nil {
				t.Fatal(err)
			}
			assertBalancedLines(t, first.Lines)
			destination := accounts.FairValueGainLoss
			if test.account == "oci" {
				destination = accounts.OCIReserve
			}
			foundDestination := false
			for _, line := range first.Lines {
				if line.AccountID == types.AccountID(destination) {
					foundDestination = true
				}
			}
			if !foundDestination {
				t.Fatalf("category destination account %s missing from entry lines", destination)
			}
			assertLineDirection(t, first.Lines[0], types.AccountID(destination), true, "100.00")
			assertLineDirection(t, first.Lines[1], types.AccountID(accounts.Asset), false, "100.00")
			if first.CompanyID != types.CompanyID(companyID) || first.JournalID != types.JournalID(journalID) ||
				first.CreatedBy != "risk_officer" || first.SourceModule != "ifrs9" ||
				first.Classification != journal.EntryClassRegular {
				t.Fatalf("measurement entry metadata incorrect: %+v", first)
			}

			retry, err := BuildMeasurementEntry(result, accounts, companyID, journalID, "risk_officer")
			if err != nil {
				t.Fatal(err)
			}
			if first.ID != retry.ID {
				t.Fatal("measurement retry generated a different entry ID")
			}
			for index := range first.Lines {
				if first.Lines[index].ID != retry.Lines[index].ID {
					t.Fatal("measurement retry generated different journal line IDs")
				}
			}
			key := MeasurementIdempotencyKey(companyID, journalID, result)
			if key == "" || key != MeasurementIdempotencyKey(companyID, journalID, result) {
				t.Fatal("measurement idempotency key missing or unstable")
			}
			hash, err := MeasurementIdempotencyPayloadHash(result, accounts, companyID, journalID)
			if err != nil {
				t.Fatal(err)
			}
			retryHash, err := MeasurementIdempotencyPayloadHash(result, accounts, companyID, journalID)
			if err != nil || hash != retryHash {
				t.Fatalf("measurement payload hash is unstable: %q vs %q (%v)", hash, retryHash, err)
			}
		})
	}
}

func TestBuildMeasurementEntryRoutesPrincipalAndInterest(t *testing.T) {
	result, err := MeasureSubsequentPeriod(MeasurementInput{
		InstrumentID: uuid.New(), AsOf: testAsOf(), Classification: validHostClassification(AmortisedCost),
		Stage: Stage1, OpeningGrossCarrying: measurementMoney(t, "1000"),
		ExistingLossAllowance: measurementMoney(t, "0"), PrincipalRepayments: measurementMoney(t, "100"),
		EffectiveInterestRate: decimal.RequireFromString("0.1"), Period: 365 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts := MeasurementAccounts{Asset: uuid.New(), Cash: uuid.New(), InterestIncome: uuid.New()}
	entry, err := BuildMeasurementEntry(result, accounts, uuid.New(), uuid.New(), "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Lines) != 4 {
		t.Fatalf("principal plus interest lines = %d, want 4", len(entry.Lines))
	}
	assertBalancedLines(t, entry.Lines)
	assertLineDirection(t, entry.Lines[0], types.AccountID(accounts.Cash), true, "100.00")
	assertLineDirection(t, entry.Lines[1], types.AccountID(accounts.Asset), false, "100.00")
	assertLineDirection(t, entry.Lines[2], types.AccountID(accounts.Asset), true, "100.00")
	assertLineDirection(t, entry.Lines[3], types.AccountID(accounts.InterestIncome), false, "100.00")
}

func TestBuildMeasurementEntryRequiresMovementAndAccounts(t *testing.T) {
	result, err := MeasureSubsequentPeriod(MeasurementInput{
		InstrumentID: uuid.New(), AsOf: testAsOf(), Classification: validHostClassification(AmortisedCost),
		Stage: Stage1, OpeningGrossCarrying: measurementMoney(t, "1000"),
		ExistingLossAllowance: measurementMoney(t, "0"), PrincipalRepayments: measurementMoney(t, "0"),
		EffectiveInterestRate: decimal.Zero, Period: 365 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildMeasurementEntry(result, MeasurementAccounts{Asset: uuid.New()}, uuid.New(), uuid.New(), "reviewer"); !errors.Is(err, ErrMeasurementNoEntry) {
		t.Fatalf("no movement error = %v, want ErrMeasurementNoEntry", err)
	}
	result.InterestIncome = measurementMoney(t, "10")
	if _, err := BuildMeasurementEntry(result, MeasurementAccounts{Asset: uuid.New()}, uuid.New(), uuid.New(), "reviewer"); !errors.Is(err, ErrMeasurementAccounts) {
		t.Fatalf("missing interest account error = %v, want ErrMeasurementAccounts", err)
	}
}

// ---------------------------------------------------------------------------
// Versioned / scenario-weighted ECL tests.
// ---------------------------------------------------------------------------

func versionedGoldenInput(t *testing.T) VersionedECLInput {
	t.Helper()
	allowance, err := types.NewMoneyFromString("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	firstBalance, err := types.NewMoneyFromString("900", "USD")
	if err != nil {
		t.Fatal(err)
	}
	firstCommitments, err := types.NewMoneyFromString("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	secondBalance, err := types.NewMoneyFromString("400", "USD")
	if err != nil {
		t.Fatal(err)
	}
	secondCommitments, err := types.NewMoneyFromString("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	firstAllowance, err := types.NewMoneyFromString("60", "USD")
	if err != nil {
		t.Fatal(err)
	}
	secondAllowance, err := types.NewMoneyFromString("40", "USD")
	if err != nil {
		t.Fatal(err)
	}
	return VersionedECLInput{
		EvaluationDate: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
		ModelVersion:   "credit-risk-v1.2",
		SourceDataID:   "portfolio-close-2026-11-10",
		SourceDataHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DiscountMethod: DiscountMethodEIRSingleExpectedLossDate,
		Assumptions: []NamedAssumption{
			{Name: "stage1_horizon", Value: "12 months"},
			{Name: "stage2_backstop_days", Value: "30"},
			{Name: "stage3_backstop_days", Value: "90"},
			{Name: "discount_method", Value: DiscountMethodEIRSingleExpectedLossDate},
			{Name: "forward_looking_basis", Value: "two weighted scenarios"},
		},
		Scenarios:         []VersionedScenario{{Name: "base", Weight: decimal.RequireFromString("0.6")}, {Name: "downside", Weight: decimal.RequireFromString("0.4")}},
		ExistingAllowance: allowance,
		Exposures: []VersionedExposure{
			{
				ID:            uuid.MustParse("10000000-0000-0000-0000-000000000001"),
				LedgerBalance: firstBalance, UndrawnCommitments: firstCommitments,
				ExistingAllowance: firstAllowance,
				Classification: HostClassification{
					MeasurementCategory: AmortisedCost, BusinessModelReference: "bm-1", SPPIAssessmentReference: "sppi-1",
				},
				RemainingLife: 2 * 365 * 24 * time.Hour, EffectiveInterestRate: decimal.RequireFromString("0.1"),
				LGD:           decimal.RequireFromString("0.5"),
				StageEvidence: StageEvidence{PreviousStage: Stage1, DaysPastDue: 31, SICR: true},
				ScenarioPDs: []ScenarioPDInput{
					{ScenarioName: "base", PD12Month: decimal.RequireFromString("0.1"), PDLifetime: decimal.RequireFromString("0.2")},
					{ScenarioName: "downside", PD12Month: decimal.RequireFromString("0.3"), PDLifetime: decimal.RequireFromString("0.5")},
				},
			},
			{
				ID:            uuid.MustParse("10000000-0000-0000-0000-000000000002"),
				LedgerBalance: secondBalance, UndrawnCommitments: secondCommitments,
				ExistingAllowance: secondAllowance,
				Classification: HostClassification{
					MeasurementCategory: AmortisedCost, BusinessModelReference: "bm-2", SPPIAssessmentReference: "sppi-2",
				},
				RemainingLife: 2 * 365 * 24 * time.Hour, EffectiveInterestRate: decimal.Zero,
				LGD:           decimal.RequireFromString("0.4"),
				StageEvidence: StageEvidence{PreviousStage: Stage3, CureConfirmed: true},
				ScenarioPDs: []ScenarioPDInput{
					{ScenarioName: "base", PD12Month: decimal.RequireFromString("0.1"), PDLifetime: decimal.RequireFromString("0.2")},
					{ScenarioName: "downside", PD12Month: decimal.RequireFromString("0.3"), PDLifetime: decimal.RequireFromString("0.5")},
				},
			},
		},
	}
}

func TestEvaluateVersionedECLGolden(t *testing.T) {
	input := versionedGoldenInput(t)
	result, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelVersion != input.ModelVersion || result.SourceDataID != input.SourceDataID ||
		result.SourceDataHash != input.SourceDataHash ||
		result.EvaluationDate != input.EvaluationDate {
		t.Fatalf("provenance not preserved in result: %+v", result)
	}
	if got := result.Exposures[0].ECL.Amount().StringFixed(2); got != "132.23" {
		t.Errorf("Stage 2 lifetime ECL = %s, want 132.23", got)
	}
	if got := result.Exposures[1].ECL.Amount().StringFixed(2); got != "36.00" {
		t.Errorf("cured Stage 1 12-month ECL = %s, want 36.00", got)
	}
	if got := result.RequiredAllowance.Amount().StringFixed(2); got != "168.23" {
		t.Errorf("required allowance = %s, want 168.23", got)
	}
	if got := result.AllowanceChange.Amount().StringFixed(2); got != "68.23" {
		t.Errorf("allowance change = %s, want 68.23", got)
	}
	if got := result.Exposures[0].Transition; got != "increased_credit_risk" {
		t.Errorf("Stage 1 to Stage 2 transition = %q", got)
	}
	if got := result.Exposures[1].Transition; got != "cured_credit_impaired" {
		t.Errorf("Stage 3 cure transition = %q", got)
	}
	if got := result.Exposures[1].Horizon; got != stage1MeasurementHorizon {
		t.Errorf("Stage 1 discount horizon = %s, want %s", got, stage1MeasurementHorizon)
	}
	if len(result.InputFingerprint) != 64 {
		t.Fatalf("input fingerprint %q is not a SHA-256 digest", result.InputFingerprint)
	}
	if result.InputFingerprint != "162bafe0f74ad06bdc92cf7c4b9f40c7f69a49229898adb5ed7dff480b73a1d9" {
		t.Errorf("golden input fingerprint = %s", result.InputFingerprint)
	}
}

func TestEvaluateVersionedECLFingerprintAndResultsAreOrderIndependent(t *testing.T) {
	input := versionedGoldenInput(t)
	first, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Exposures[0], input.Exposures[1] = input.Exposures[1], input.Exposures[0]
	input.Scenarios[0], input.Scenarios[1] = input.Scenarios[1], input.Scenarios[0]
	input.Assumptions[0], input.Assumptions[1] = input.Assumptions[1], input.Assumptions[0]
	input.Exposures[0].ScenarioPDs[0], input.Exposures[0].ScenarioPDs[1] =
		input.Exposures[0].ScenarioPDs[1], input.Exposures[0].ScenarioPDs[0]
	reordered, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	if reordered.InputFingerprint != first.InputFingerprint ||
		reordered.RequiredAllowance.Amount().String() != first.RequiredAllowance.Amount().String() ||
		reordered.Exposures[0].ExposureID != first.Exposures[0].ExposureID {
		t.Fatalf("input order changed the deterministic result: first=%+v reordered=%+v", first, reordered)
	}
	input.ModelVersion = "credit-risk-v1.3"
	changed, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	if changed.InputFingerprint == first.InputFingerprint {
		t.Fatal("changing the model version did not change the input fingerprint")
	}
}

func TestEvaluateVersionedECLFingerprintTracksProvenanceAndAssumptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*VersionedECLInput)
	}{
		{name: "source snapshot", mutate: func(input *VersionedECLInput) {
			input.SourceDataHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{name: "evaluation date", mutate: func(input *VersionedECLInput) { input.EvaluationDate = input.EvaluationDate.AddDate(0, 0, 1) }},
		{name: "named assumption", mutate: func(input *VersionedECLInput) { input.Assumptions[4].Value = "revised scenario basis" }},
		{name: "scenario estimate", mutate: func(input *VersionedECLInput) {
			input.Exposures[0].ScenarioPDs[0].PDLifetime = decimal.RequireFromString("0.21")
		}},
		{name: "host classification evidence", mutate: func(input *VersionedECLInput) {
			input.Exposures[0].Classification.BusinessModelReference = "revised-host-assessment"
		}},
		{name: "undrawn commitment", mutate: func(input *VersionedECLInput) {
			input.Exposures[0].UndrawnCommitments = measurementMoney(t, "110")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := versionedGoldenInput(t)
			first, err := EvaluateVersionedECL(input)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&input)
			changed, err := EvaluateVersionedECL(input)
			if err != nil {
				t.Fatal(err)
			}
			if changed.InputFingerprint == first.InputFingerprint {
				t.Fatal("changed input did not change the fingerprint")
			}
		})
	}
}

func TestVersionedECLSeparatesACAndFVOCIAllowances(t *testing.T) {
	input := versionedGoldenInput(t)
	input.Exposures[1].Classification.MeasurementCategory = FVOCI
	result, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CategoryChanges) != 2 {
		t.Fatalf("category changes = %d, want AC and FVOCI", len(result.CategoryChanges))
	}
	if result.CategoryChanges[0].Category != AmortisedCost ||
		result.CategoryChanges[0].AllowanceChange.Amount().StringFixed(2) != "72.23" ||
		result.CategoryChanges[1].Category != FVOCI ||
		result.CategoryChanges[1].AllowanceChange.Amount().StringFixed(2) != "-4.00" {
		t.Fatalf("category allowance movements = %+v", result.CategoryChanges)
	}
	accounts := ECLCategoryAccounts{
		AmortisedCost: ECLAccounts{ImpairmentLoss: uuid.New(), Allowance: uuid.New()},
		FVOCI:         ECLAccounts{ImpairmentLoss: uuid.New(), OCIReserve: uuid.New()},
	}
	companyID, journalID := uuid.New(), uuid.New()
	entries, err := BuildVersionedECLEntries(result, accounts, companyID, journalID, "risk_officer")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("category entries = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		assertBalancedLines(t, entry.Lines)
		if entry.CompanyID != types.CompanyID(companyID) || entry.JournalID != types.JournalID(journalID) ||
			entry.SourceModule != "ifrs9" {
			t.Fatalf("ECL entry metadata incorrect: %+v", entry)
		}
	}
	assertLineDirection(t, entries[0].Lines[0], types.AccountID(accounts.AmortisedCost.ImpairmentLoss), true, "72.23")
	assertLineDirection(t, entries[0].Lines[1], types.AccountID(accounts.AmortisedCost.Allowance), false, "72.23")
	assertLineDirection(t, entries[1].Lines[0], types.AccountID(accounts.FVOCI.OCIReserve), true, "4.00")
	assertLineDirection(t, entries[1].Lines[1], types.AccountID(accounts.FVOCI.ImpairmentLoss), false, "4.00")
	combined, err := BuildVersionedECLApplicationEntry(result, accounts, companyID, journalID, "risk_officer")
	if err != nil {
		t.Fatal(err)
	}
	if len(combined.Lines) != 4 {
		t.Fatalf("combined ECL journal lines = %d, want 4", len(combined.Lines))
	}
	assertBalancedLines(t, combined.Lines)
	if combined.ReferenceID != result.InputFingerprint {
		t.Fatalf("combined reference = %q, want input fingerprint", combined.ReferenceID)
	}
}

func TestEvaluateVersionedECLRequiresCompleteProvenanceAndAssumptions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*VersionedECLInput)
	}{
		{name: "model version", mutate: func(input *VersionedECLInput) { input.ModelVersion = "" }},
		{name: "source identifier", mutate: func(input *VersionedECLInput) { input.SourceDataID = "" }},
		{name: "source hash", mutate: func(input *VersionedECLInput) { input.SourceDataHash = "" }},
		{name: "malformed source hash", mutate: func(input *VersionedECLInput) { input.SourceDataHash = "sha256:not-a-digest" }},
		{name: "assumptions", mutate: func(input *VersionedECLInput) { input.Assumptions = nil }},
		{name: "stage assumptions", mutate: func(input *VersionedECLInput) { input.Assumptions[1].Value = "31" }},
		{name: "evaluation date", mutate: func(input *VersionedECLInput) { input.EvaluationDate = time.Time{} }},
		{name: "missing host classification", mutate: func(input *VersionedECLInput) {
			input.Exposures[0].Classification.SPPIAssessmentReference = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := versionedGoldenInput(t)
			test.mutate(&input)
			if _, err := EvaluateVersionedECL(input); !errors.Is(err, ErrInvalidVersionedInput) {
				t.Fatalf("EvaluateVersionedECL() error = %v, want ErrInvalidVersionedInput", err)
			}
		})
	}
}

func TestVersionedStageTransitionBackstopsAndCure(t *testing.T) {
	tests := []struct {
		name      string
		evidence  StageEvidence
		wantStage Stage
		wantMove  string
		wantErr   bool
	}{
		{name: "initial Stage 1", evidence: StageEvidence{}, wantStage: Stage1, wantMove: "initial"},
		{name: "SICR increases stage", evidence: StageEvidence{PreviousStage: Stage1, SICR: true}, wantStage: Stage2, wantMove: "increased_credit_risk"},
		{name: "30 day backstop boundary", evidence: StageEvidence{PreviousStage: Stage1, DaysPastDue: 30}, wantStage: Stage1, wantMove: "no_change"},
		{name: "31 day backstop", evidence: StageEvidence{PreviousStage: Stage1, DaysPastDue: 31}, wantStage: Stage2, wantMove: "increased_credit_risk"},
		{name: "90 day backstop boundary", evidence: StageEvidence{PreviousStage: Stage2, DaysPastDue: 90}, wantStage: Stage2, wantMove: "no_change"},
		{name: "91 day default backstop", evidence: StageEvidence{PreviousStage: Stage2, DaysPastDue: 91}, wantStage: Stage3, wantMove: "credit_impaired"},
		{name: "improved credit risk", evidence: StageEvidence{PreviousStage: Stage2}, wantStage: Stage1, wantMove: "improved_credit_risk"},
		{name: "default overrides other evidence", evidence: StageEvidence{PreviousStage: Stage1, Defaulted: true}, wantStage: Stage3, wantMove: "credit_impaired"},
		{name: "Stage 3 cannot cure implicitly", evidence: StageEvidence{PreviousStage: Stage3}, wantStage: Stage3, wantMove: "no_change"},
		{name: "explicit cure can improve stage", evidence: StageEvidence{PreviousStage: Stage3, CureConfirmed: true}, wantStage: Stage1, wantMove: "cured_credit_impaired"},
		{name: "cure confirmation only from Stage 3", evidence: StageEvidence{PreviousStage: Stage2, CureConfirmed: true}, wantErr: true},
		{name: "invalid previous stage", evidence: StageEvidence{PreviousStage: "stage_4"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stage, transition, err := assessStage(test.evidence)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidStageTransition) {
					t.Fatalf("assessStage() error = %v, want ErrInvalidStageTransition", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if stage != test.wantStage || transition != test.wantMove {
				t.Fatalf("assessStage() = (%s, %q), want (%s, %q)", stage, transition, test.wantStage, test.wantMove)
			}
		})
	}
}

func TestEvaluateVersionedECLRejectsIncompleteScenarioData(t *testing.T) {
	input := versionedGoldenInput(t)
	input.Scenarios[0].Weight = decimal.RequireFromString("0.5")
	if _, err := EvaluateVersionedECL(input); !errors.Is(err, ErrInvalidVersionedInput) {
		t.Fatalf("invalid scenario weights error = %v, want ErrInvalidVersionedInput", err)
	}
	input = versionedGoldenInput(t)
	input.Exposures[0].ScenarioPDs = input.Exposures[0].ScenarioPDs[:1]
	if _, err := EvaluateVersionedECL(input); !errors.Is(err, ErrInvalidVersionedInput) {
		t.Fatalf("incomplete scenario PDs error = %v, want ErrInvalidVersionedInput", err)
	}
}

func TestEvaluateVersionedECLRejectsFVTPL(t *testing.T) {
	input := versionedGoldenInput(t)
	input.Exposures[0].Classification.MeasurementCategory = FVTPL
	if _, err := EvaluateVersionedECL(input); !errors.Is(err, ErrECLNotApplicable) {
		t.Fatalf("FVTPL ECL error = %v, want ErrECLNotApplicable", err)
	}
}

type mockECLPostingCap struct {
	accounts  ECLAccounts
	available bool
}

func (m mockECLPostingCap) LedgerPostingAvailable() bool { return m.available }

func (m mockECLPostingCap) ResolveECLAccounts(_ context.Context, _ uuid.UUID, mappingID string) (ECLAccounts, error) {
	if mappingID != "ecl_standard" {
		return ECLAccounts{}, ErrECLJournalMappingUnavailable
	}
	return m.accounts, nil
}

type mockECLPostingApp struct {
	posted []journal.JournalEntry
	keys   map[string]string
}

func (m *mockECLPostingApp) PostECLEntry(
	_ context.Context,
	entry journal.JournalEntry,
	key, payloadHash string,
) error {
	if m.keys == nil {
		m.keys = make(map[string]string)
	}
	if existing, ok := m.keys[key]; ok {
		if existing != payloadHash {
			return errors.New("idempotency conflict")
		}
		return nil // idempotent retry
	}
	m.posted = append(m.posted, entry)
	m.keys[key] = payloadHash
	return nil
}

func TestGoldenScenario_GS09_IFRS9VersionedECLPostingAndIdempotency(t *testing.T) {
	result, err := EvaluateVersionedECL(versionedGoldenInput(t))
	if err != nil {
		t.Fatal(err)
	}
	companyID, journalID := uuid.New(), uuid.New()
	accounts := ECLAccounts{ImpairmentLoss: uuid.New(), Allowance: uuid.New()}
	cap := mockECLPostingCap{accounts: accounts, available: true}
	app := &mockECLPostingApp{}
	ctx := context.Background()

	entry1, err := PostVersionedECL(ctx, cap, app, result, companyID, journalID, "ecl_standard", "risk_officer")
	if err != nil {
		t.Fatalf("PostVersionedECL first attempt: %v", err)
	}
	if len(app.posted) != 1 {
		t.Fatalf("expected 1 posted entry, got %d", len(app.posted))
	}
	if entry1.Status != journal.EntryPosted {
		t.Fatalf("entry status = %s, want posted", entry1.Status)
	}
	assertBalancedLines(t, entry1.Lines)
	debits, _ := journalEntryTotals(entry1.Lines)
	if !debits.Equal(result.AllowanceChange.Amount().Abs()) {
		t.Fatalf("ECL entry amount = %s, want %s", debits, result.AllowanceChange.Amount().Abs())
	}

	entry2, err := PostVersionedECL(ctx, cap, app, result, companyID, journalID, "ecl_standard", "risk_officer")
	if err != nil {
		t.Fatalf("PostVersionedECL retry attempt: %v", err)
	}
	wantKey := VersionedECLIdempotencyKey(result, companyID, journalID, AmortisedCost)
	if _, ok := app.keys[wantKey]; !ok {
		t.Fatalf("expected idempotency key %q was not used", wantKey)
	}
	if entry1.ID != entry2.ID {
		t.Fatalf("retry identity mismatch: %s vs %s", entry1.ID, entry2.ID)
	}
	if len(app.posted) != 1 {
		t.Fatalf("duplicate entry posted to application store on retry: count=%d", len(app.posted))
	}
}

// ---------------------------------------------------------------------------
// Application posting adapter tests.
// ---------------------------------------------------------------------------

func measurementEntryForApplication(t *testing.T) (MeasurementResult, MeasurementAccounts, uuid.UUID, uuid.UUID, journal.JournalEntry) {
	t.Helper()
	result, err := MeasureSubsequentPeriod(MeasurementInput{
		InstrumentID: uuid.New(), AsOf: testAsOf(), Classification: validHostClassification(AmortisedCost),
		Stage: Stage1, OpeningGrossCarrying: measurementMoney(t, "100"),
		ExistingLossAllowance: measurementMoney(t, "0"), PrincipalRepayments: measurementMoney(t, "0"),
		EffectiveInterestRate: decimal.RequireFromString("0.1"), Period: 365 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts := MeasurementAccounts{Asset: uuid.New(), Cash: uuid.New(), InterestIncome: uuid.New()}
	companyID, journalID := uuid.New(), uuid.New()
	entry, err := BuildMeasurementEntry(result, accounts, companyID, journalID, "risk_officer")
	if err != nil {
		t.Fatal(err)
	}
	return result, accounts, companyID, journalID, entry
}

func authorizedContext(companyID uuid.UUID) context.Context {
	return publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID: "risk_officer", CompanyIDs: []types.CompanyID{types.CompanyID(companyID)},
	})
}

func TestPostJournalEntryThroughApplication(t *testing.T) {
	result, _, companyID, journalID, entry := measurementEntryForApplication(t)
	app := &applicationPostingRecorder{}
	ctx := authorizedContext(companyID)
	key := MeasurementIdempotencyKey(companyID, journalID, result)
	posted, err := PostJournalEntryThroughApplication(ctx, app, entry, key)
	if err != nil {
		t.Fatal(err)
	}
	if posted.Entry.Status != journal.EntryPosted || app.created.CompanyID != types.CompanyID(companyID) ||
		app.created.JournalID != types.JournalID(journalID) ||
		app.created.IdempotencyKey != key || app.posted.EntryID != app.entryID {
		t.Fatalf("journal was not created and posted through the application service: result=%+v create=%+v post=%+v",
			posted, app.created, app.posted)
	}
	if len(app.created.Lines) != len(entry.Lines) || app.created.SourceModule != "ifrs9" ||
		app.created.EffectiveDate != entry.EffectiveDate || app.created.ReferenceID != entry.ReferenceID {
		t.Fatalf("application draft does not preserve IFRS 9 entry details: %+v", app.created)
	}
	assertBalancedLines(t, app.created.Lines)
}

func TestPostJournalEntryThroughApplicationRejectsMissingInputs(t *testing.T) {
	_, _, companyID, _, entry := measurementEntryForApplication(t)
	app := &applicationPostingRecorder{}
	if _, err := PostJournalEntryThroughApplication(nil, app, entry, "key"); !errors.Is(err, ErrApplicationPrincipalRequired) {
		t.Fatalf("nil context error = %v, want ErrApplicationPrincipalRequired", err)
	}
	if _, err := PostJournalEntryThroughApplication(context.Background(), app, entry, "key"); !errors.Is(err, ErrApplicationPrincipalRequired) {
		t.Fatalf("missing principal error = %v, want ErrApplicationPrincipalRequired", err)
	}
	ctx := authorizedContext(companyID)
	if _, err := PostJournalEntryThroughApplication(ctx, app, entry, ""); !errors.Is(err, ErrApplicationIdempotencyRequired) {
		t.Fatalf("missing key error = %v, want ErrApplicationIdempotencyRequired", err)
	}
	unauthorizedContext := publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID: "risk_officer", CompanyIDs: []types.CompanyID{types.CompanyID(uuid.New())},
	})
	if _, err := PostJournalEntryThroughApplication(unauthorizedContext, app, entry, "key"); !errors.Is(err, ErrApplicationPrincipalRequired) {
		t.Fatalf("unauthorized principal error = %v, want ErrApplicationPrincipalRequired", err)
	}
	if _, err := PostJournalEntryThroughApplication(ctx, nil, entry, "key"); !errors.Is(err, ErrApplicationPostingUnavailable) {
		t.Fatalf("missing application error = %v, want ErrApplicationPostingUnavailable", err)
	}
}

func TestPostCategorizedVersionedECLThroughApplication(t *testing.T) {
	input := versionedGoldenInput(t)
	input.Exposures[1].Classification.MeasurementCategory = FVOCI
	result, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	companyID, journalID := uuid.New(), uuid.New()
	accounts := ECLCategoryAccounts{
		AmortisedCost: ECLAccounts{ImpairmentLoss: uuid.New(), Allowance: uuid.New()},
		FVOCI:         ECLAccounts{ImpairmentLoss: uuid.New(), OCIReserve: uuid.New()},
	}
	app := &applicationPostingRecorder{}
	ctx := authorizedContext(companyID)
	posted, err := PostCategorizedVersionedECLThroughApplication(
		ctx, app, result, accounts, companyID, journalID, "risk_officer",
	)
	if err != nil {
		t.Fatal(err)
	}
	if posted.Entry.Status != journal.EntryPosted || len(app.created.Lines) != 4 ||
		app.created.IdempotencyKey == "" || app.posted.EntryID != app.entryID {
		t.Fatalf("categorized ECL was not posted as one application journal: result=%+v create=%+v", posted, app.created)
	}
	assertBalancedLines(t, app.created.Lines)
	accountsUsed := make(map[types.AccountID]bool, len(app.created.Lines))
	for _, line := range app.created.Lines {
		accountsUsed[line.AccountID] = true
	}
	for _, accountID := range []uuid.UUID{
		accounts.AmortisedCost.Allowance, accounts.FVOCI.OCIReserve,
	} {
		if !accountsUsed[types.AccountID(accountID)] {
			t.Errorf("missing category-specific account %s from application journal", accountID)
		}
	}
}

func TestPostVersionedECLThroughApplication(t *testing.T) {
	result, err := EvaluateVersionedECL(versionedGoldenInput(t))
	if err != nil {
		t.Fatal(err)
	}
	companyID, journalID := uuid.New(), uuid.New()
	accounts := ECLAccounts{ImpairmentLoss: uuid.New(), Allowance: uuid.New()}
	app := &applicationPostingRecorder{}
	posted, err := PostVersionedECLThroughApplication(
		authorizedContext(companyID), app, result, accounts, companyID, journalID, "risk_officer",
	)
	if err != nil {
		t.Fatal(err)
	}
	if posted.Entry.Status != journal.EntryPosted || len(app.created.Lines) != 2 {
		t.Fatalf("single-category ECL was not posted: result=%+v create=%+v", posted, app.created)
	}
	assertBalancedLines(t, app.created.Lines)
}

// ---------------------------------------------------------------------------
// Module / descriptor / posting port tests.
// ---------------------------------------------------------------------------

func TestModuleDependencies(t *testing.T) {
	module := &Module{}
	if got := module.Dependencies(); len(got) != 0 {
		t.Fatalf("Dependencies() = %v, want no module dependencies", got)
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
	if module.Name() != "IFRS 9" || module.Version() != "1.0.0" {
		t.Fatalf("Name/Version = %q/%q", module.Name(), module.Version())
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
	for _, eventType := range []string{"IFRS9_ECL_PROVISION", "IFRS9_LEASE_PAYMENT"} {
		if got := router.GetRules(eventType); len(got) != 0 {
			t.Errorf("unexpected hard-coded posting rules for %s: %+v", eventType, got)
		}
	}
}

func TestDescriptorProfileAndWorkflowsValidate(t *testing.T) {
	descriptor := (&Module{}).Descriptor()
	if descriptor.ID != "ifrs9" || descriptor.Profile == nil {
		t.Fatalf("descriptor identity missing: %+v", descriptor)
	}
	if err := descriptor.Profile.Validate(); err != nil {
		t.Fatalf("invalid standard profile: %v", err)
	}
	if descriptor.Profile.IsConfigured() {
		t.Fatal("profile placeholders must remain unconfigured until the host selects an edition")
	}
	for _, workflow := range descriptor.Workflows {
		if err := workflow.Validate(*descriptor.Profile); err != nil {
			t.Errorf("invalid workflow %q: %v", workflow.ID, err)
		}
	}
}

func TestVersionedGeneralECLWorkflowIsBoundedAndDoesNotPost(t *testing.T) {
	descriptor := (&Module{}).Descriptor()
	var supported bool
	for _, requirement := range descriptor.Profile.SupportedRequirements {
		if requirement.ID == "IFRS9-ECL-GENERAL-VERSIONED-01" {
			supported = requirement.Status == standards.RequirementLimited
		}
	}
	if !supported {
		t.Fatal("versioned general ECL requirement is not declared as limited support")
	}
	for _, workflow := range descriptor.Workflows {
		if workflow.ID != "versioned-general-ecl-measurement" {
			continue
		}
		if workflow.ProducesJournalEntries || workflow.RequiresJournalMapping ||
			workflow.ApplicationContract != "" || len(workflow.RequiredCapabilities) != 0 {
			t.Fatalf("measurement-only workflow unexpectedly declares posting: %+v", workflow)
		}
		return
	}
	t.Fatal("versioned general ECL workflow not declared")
}

func TestDescriptorDeclaresClassificationHedgeExclusionAndApplicationPosting(t *testing.T) {
	descriptor := (&Module{}).Descriptor()
	var classificationSupported, hedgeSupported bool
	for _, requirement := range descriptor.Profile.SupportedRequirements {
		if requirement.ID == "IFRS9-CLASSIFICATION-MEASUREMENT-01" {
			classificationSupported = requirement.Status == standards.RequirementLimited
		}
		if requirement.ID == "IFRS9-HEDGE-ACCOUNTING" {
			hedgeSupported = requirement.Status == standards.RequirementLimited
		}
	}
	if !classificationSupported || !hedgeSupported {
		t.Fatalf("IFRS 9 classification or hedge support is missing: %+v", descriptor.Profile)
	}
	for _, workflowID := range []string{"financial-asset-subsequent-measurement", "versioned-general-ecl-posting"} {
		var found bool
		for _, workflow := range descriptor.Workflows {
			if workflow.ID != workflowID {
				continue
			}
			found = workflow.ProducesJournalEntries && workflow.RequiresJournalMapping &&
				workflow.ApplicationContract == standards.ApplicationLedgerService
		}
		if !found {
			t.Errorf("workflow %q is not wired to the application posting contract", workflowID)
		}
	}
}

func TestModulePostSubsequentMeasurementThroughStandardPosting(t *testing.T) {
	result, err := MeasureSubsequentPeriod(MeasurementInput{
		InstrumentID: uuid.New(), AsOf: testAsOf(), Classification: validHostClassification(AmortisedCost),
		Stage: Stage1, OpeningGrossCarrying: measurementMoney(t, "1000"),
		ExistingLossAllowance: measurementMoney(t, "0"), PrincipalRepayments: measurementMoney(t, "100"),
		EffectiveInterestRate: decimal.RequireFromString("0.1"), Period: 365 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts := MeasurementAccounts{Asset: uuid.New(), Cash: uuid.New(), InterestIncome: uuid.New()}
	companyID, journalID := uuid.New(), uuid.New()
	poster := &recordingPoster{}
	module := &Module{}
	if err := module.Initialize(context.Background(), testModulePorts{
		entries: fakeEntryReader{}, masterData: fakeMasterDataReader{},
		application: fakeApplication{}, posting: poster,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := module.PostSubsequentMeasurement(
		context.Background(), result, accounts, companyID, journalID, "risk_officer", "measurement-key-1",
	); err != nil {
		t.Fatal(err)
	}
	if len(poster.requests) != 1 {
		t.Fatalf("standard posting requests = %d, want 1", len(poster.requests))
	}
	req := poster.requests[0]
	if req.StandardRef != "IFRS-9-SUBSEQUENT-MEASUREMENT" || req.EntityID != result.InstrumentID.String() ||
		req.IdempotencyKey != "measurement-key-1" || req.CompanyID != types.CompanyID(companyID) ||
		req.JournalID != types.JournalID(journalID) || req.Classification != journal.EntryClassRegular {
		t.Fatalf("unexpected standard posting request: %+v", req)
	}
	if len(req.Lines) != 4 || !journalEntryBalanced(req.Lines) {
		t.Fatalf("posted measurement lines unbalanced or missing: %+v", req.Lines)
	}
	if req.EffectiveDate != result.AsOf || req.PostingDate != result.AsOf {
		t.Fatalf("posting dates = %s/%s, want %s", req.EffectiveDate, req.PostingDate, result.AsOf)
	}
}

func TestModulePostVersionedECLThroughStandardPosting(t *testing.T) {
	input := versionedGoldenInput(t)
	input.Exposures[1].Classification.MeasurementCategory = FVOCI
	result, err := EvaluateVersionedECL(input)
	if err != nil {
		t.Fatal(err)
	}
	accounts := ECLCategoryAccounts{
		AmortisedCost: ECLAccounts{ImpairmentLoss: uuid.New(), Allowance: uuid.New()},
		FVOCI:         ECLAccounts{ImpairmentLoss: uuid.New(), OCIReserve: uuid.New()},
	}
	companyID, journalID := uuid.New(), uuid.New()
	poster := &recordingPoster{}
	module := &Module{}
	if err := module.Initialize(context.Background(), testModulePorts{
		entries: fakeEntryReader{}, masterData: fakeMasterDataReader{},
		application: fakeApplication{}, posting: poster,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := module.PostVersionedECL(
		context.Background(), result, accounts, companyID, journalID, "risk_officer", "ecl-key-1",
	); err != nil {
		t.Fatal(err)
	}
	if len(poster.requests) != 1 {
		t.Fatalf("standard posting requests = %d, want 1", len(poster.requests))
	}
	req := poster.requests[0]
	if req.StandardRef != "IFRS-9-ECL" || req.EntityID != result.InputFingerprint ||
		req.IdempotencyKey != "ecl-key-1" || req.CompanyID != types.CompanyID(companyID) ||
		req.JournalID != types.JournalID(journalID) {
		t.Fatalf("unexpected ECL standard posting request: %+v", req)
	}
	if len(req.Lines) != 4 || !journalEntryBalanced(req.Lines) {
		t.Fatalf("posted ECL lines unbalanced or missing: %+v", req.Lines)
	}
	debits, credits := journalEntryTotals(req.Lines)
	// The combined journal posts both category movements: a 72.23 AC increase
	// and a 4.00 FVOCI release (net allowance change is 68.23).
	if !debits.Equal(decimal.RequireFromString("76.23")) || !credits.Equal(debits) {
		t.Fatalf("ECL posting totals = %s/%s, want 76.23", debits, credits)
	}
}

func TestModulePostingRequiresInitializedPorts(t *testing.T) {
	module := &Module{}
	result, err := EvaluateVersionedECL(versionedGoldenInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.PostVersionedECL(
		context.Background(), result, ECLCategoryAccounts{}, uuid.New(), uuid.New(), "risk_officer", "key",
	); err == nil {
		t.Fatal("uninitialized module posted an ECL journal")
	}
	if _, err := module.PostSubsequentMeasurement(
		context.Background(), MeasurementResult{},
		MeasurementAccounts{}, uuid.New(), uuid.New(), "risk_officer", "key",
	); err == nil {
		t.Fatal("uninitialized module posted a measurement journal")
	}
}
