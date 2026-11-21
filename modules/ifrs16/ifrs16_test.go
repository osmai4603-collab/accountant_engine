package ifrs16

import (
	"context"
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
// Lessee measurement and amortization
// ---------------------------------------------------------------------------

func TestLeaseContractPVAndSchedule(t *testing.T) {
	contract := testContract(t)
	assertAmount(t, contract.InitialLeaseLiability, "197.2269")
	assertAmount(t, contract.InitialROUAsset, "222.2269")

	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule) != 2 {
		t.Fatalf("schedule length = %d, want 2", len(schedule))
	}
	assertAmount(t, schedule[0].InterestExpense, "1.9075")
	assertAmount(t, schedule[0].PrincipalReduction, "98.0925")
	assertAmount(t, schedule[0].ClosingLiability, "99.1344")
	assertAmount(t, schedule[1].InterestExpense, "0.8656")
	assertAmount(t, schedule[1].ClosingLiability, "0")
	assertAmount(t, schedule[0].ROUDepreciation, "111.1135")
	assertAmount(t, schedule[1].AccumulatedDepreciation, "222.2269")
	assertAmount(t, schedule[1].ClosingROUAsset, "0")

	unitSchedule, err := NewUnit().AmortizationSchedule(contract)
	if err != nil {
		t.Fatal(err)
	}
	if len(unitSchedule) != len(schedule) {
		t.Fatalf("Unit schedule length = %d, want %d", len(unitSchedule), len(schedule))
	}
	assertAmount(t, unitSchedule[0].InterestExpense, "1.9075")
}

func TestIrregularPeriodLeaseDiscounting(t *testing.T) {
	contract, err := NewLeaseContract(
		date(2026, 1, 1), decimal.RequireFromString("0.12"),
		[]LeasePayment{
			{DueDate: date(2026, 2, 15), Amount: money(t, "100", "USD")},
			{DueDate: date(2026, 3, 31), Amount: money(t, "100", "USD")},
		},
		money(t, "0", "USD"), money(t, "0", "USD"), money(t, "0", "USD"), 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, contract.InitialLeaseLiability, "195.8870")

	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, schedule[0].InterestExpense, "2.7561")
	assertAmount(t, schedule[1].InterestExpense, "1.3569")
	assertAmount(t, schedule[1].ClosingLiability, "0")
}

func TestScheduleRejectsPaymentThatOverpaysLiability(t *testing.T) {
	contract, err := NewLeaseContract(
		date(2026, 1, 1), decimal.Zero,
		[]LeasePayment{
			{DueDate: date(2026, 2, 1), Amount: money(t, "10", "USD")},
			{DueDate: date(2026, 3, 1), Amount: money(t, "11", "USD")},
		},
		money(t, "0", "USD"), money(t, "0", "USD"), money(t, "0", "USD"), 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract.InitialLeaseLiability = money(t, "1", "USD")
	if _, err := contract.AmortizationSchedule(); err != ErrNegativeLiability {
		t.Fatalf("AmortizationSchedule error = %v, want %v", err, ErrNegativeLiability)
	}
}

func TestGoldenScenario_GS08_IFRS16Leases(t *testing.T) {
	commencement := date(2026, 1, 1)
	rate := decimal.RequireFromString("0.10")
	payments := []LeasePayment{
		{DueDate: date(2026, 7, 1), Amount: money(t, "1000", "USD")},
		{DueDate: date(2027, 1, 1), Amount: money(t, "1000", "USD")},
	}
	contract, err := NewLeaseContract(commencement, rate, payments,
		money(t, "50", "USD"), money(t, "100", "USD"), money(t, "30", "USD"), 2)
	if err != nil {
		t.Fatalf("NewLeaseContract: %v", err)
	}
	contract = contract.WithReference("LEASE-GS08-2026")

	if !contract.InitialLeaseLiability.Amount().IsPositive() {
		t.Fatalf("expected positive initial liability, got %s", contract.InitialLeaseLiability.Amount())
	}
	expectedROU := contract.InitialLeaseLiability.Amount().Add(decimal.NewFromInt(120))
	if !contract.InitialROUAsset.Amount().Equal(expectedROU) {
		t.Fatalf("initial ROU = %s, want %s", contract.InitialROUAsset.Amount(), expectedROU)
	}

	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatalf("AmortizationSchedule: %v", err)
	}
	if len(schedule) != 2 {
		t.Fatalf("schedule length = %d, want 2", len(schedule))
	}
	if !schedule[1].ClosingLiability.Amount().IsZero() {
		t.Fatalf("final closing liability = %s, want 0.00", schedule[1].ClosingLiability.Amount())
	}
	if !schedule[1].AccumulatedDepreciation.Amount().Equal(contract.InitialROUAsset.Amount()) {
		t.Fatalf("accumulated depreciation = %s, want initial ROU %s",
			schedule[1].AccumulatedDepreciation.Amount(), contract.InitialROUAsset.Amount())
	}
	if !schedule[1].ClosingROUAsset.Amount().IsZero() {
		t.Fatalf("final closing ROU = %s, want 0.00", schedule[1].ClosingROUAsset.Amount())
	}
}

func TestLeaseValidation(t *testing.T) {
	zero := money(t, "0", "USD")
	validPayment := LeasePayment{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")}
	cases := []struct {
		name  string
		rate  decimal.Decimal
		terms []LeasePayment
	}{
		{"negative discount rate", decimal.NewFromInt(-1), []LeasePayment{validPayment}},
		{"zero payment", decimal.Zero, []LeasePayment{{DueDate: validPayment.DueDate, Amount: zero}}},
		{"wrong currency", decimal.Zero, []LeasePayment{{DueDate: validPayment.DueDate, Amount: money(t, "100", "EUR")}}},
		{"invalid date", decimal.Zero, []LeasePayment{{DueDate: date(2026, 1, 1), Amount: money(t, "100", "USD")}}},
		{"unordered dates", decimal.Zero, []LeasePayment{validPayment, validPayment}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewLeaseContract(
				date(2026, 1, 1), test.rate, test.terms, zero, zero, zero, 0,
			); err == nil {
				t.Fatal("expected invalid lease terms error")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Lessee journal entries and modifications
// ---------------------------------------------------------------------------

func TestLeaseJournalEntriesBalance(t *testing.T) {
	contract := testContract(t)
	accounts := testAccounts()
	builder := testEntryBuilder(t, accounts)
	companyID, journalID := uuid.New(), uuid.New()
	ctx := context.Background()

	commencement, err := builder.BuildCommencementEntry(ctx, contract, companyID, journalID, "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, commencement)
	if commencement.SourceModule != "ifrs16" || commencement.ReferenceID != contract.Reference {
		t.Fatalf("commencement traceability incomplete: %+v", commencement)
	}

	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatal(err)
	}
	payment, err := builder.BuildPaymentEntry(ctx, schedule[0], companyID, journalID, "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, payment)
	depreciation, err := builder.BuildDepreciationEntry(ctx, schedule[0], companyID, journalID, "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, depreciation)
	if len(payment.Lines) != 3 {
		t.Fatalf("payment line count = %d, want interest, principal, and cash", len(payment.Lines))
	}
	if len(depreciation.Lines) != 2 {
		t.Fatalf("depreciation line count = %d, want 2", len(depreciation.Lines))
	}
}

func TestEntryBuilderRequiresCapabilityAndMappingAndIsRetryStable(t *testing.T) {
	accounts := testAccounts()
	if _, err := NewEntryBuilder(testLeasePostingCapability{accounts: accounts}); err != ErrPostingCapabilityUnavailable {
		t.Fatalf("NewEntryBuilder without posting capability error = %v", err)
	}
	builder := testEntryBuilder(t, accounts)
	contract := testContract(t)
	companyID, journalID := uuid.New(), uuid.New()
	if _, err := builder.BuildCommencementEntry(context.Background(), contract, companyID, journalID, "", "tester"); err != ErrJournalMappingUnavailable {
		t.Fatalf("BuildCommencementEntry without mapping error = %v", err)
	}

	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatal(err)
	}
	first, err := builder.BuildPaymentEntry(context.Background(), schedule[0], companyID, journalID, "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := builder.BuildPaymentEntry(context.Background(), schedule[0], companyID, journalID, "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != retry.ID || EntryIdempotencyKey(first) != EntryIdempotencyKey(retry) ||
		EntryIdempotencyPayloadHash(first) != EntryIdempotencyPayloadHash(retry) {
		t.Fatal("retry did not reproduce the same entry identity and payload")
	}
	if first.SourceModule != "ifrs16" || first.ReferenceID != contract.Reference || first.Description == "" {
		t.Fatalf("entry traceability fields are incomplete: %+v", first)
	}
	remapped, err := testEntryBuilder(t, testAccounts()).BuildPaymentEntry(
		context.Background(), schedule[0], companyID, journalID, "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if EntryIdempotencyKey(remapped) != EntryIdempotencyKey(first) ||
		EntryIdempotencyPayloadHash(remapped) == EntryIdempotencyPayloadHash(first) {
		t.Fatal("remapping must retain operation identity and change the payload hash")
	}
}

func TestPaymentEntryHandlesNegativePrincipal(t *testing.T) {
	contract, err := NewLeaseContract(
		date(2026, 1, 1), decimal.RequireFromString("0.12"),
		[]LeasePayment{
			{DueDate: date(2026, 2, 1), Amount: money(t, "0.01", "USD")},
			{DueDate: date(2026, 3, 1), Amount: money(t, "100", "USD")},
		},
		money(t, "0", "USD"), money(t, "0", "USD"), money(t, "0", "USD"), 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("negative-principal")
	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatal(err)
	}
	if !schedule[0].PrincipalReduction.Amount().IsNegative() {
		t.Fatal("expected first payment to increase the liability")
	}
	entry, err := testEntryBuilder(t, testAccounts()).BuildPaymentEntry(
		context.Background(), schedule[0], uuid.New(), uuid.New(), "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
}

func TestModificationRecalculationAndEntry(t *testing.T) {
	contract := testContract(t)
	result, err := contract.RecalculateModification(
		date(2026, 2, 1),
		[]LeasePayment{{DueDate: date(2026, 3, 1), Amount: money(t, "60", "USD")}},
		decimal.Zero,
		money(t, "100", "USD"),
		money(t, "20", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, result.RemeasuredLiability, "60")
	assertAmount(t, result.NewROUAsset, "0")
	assertAmount(t, result.ModificationGain, "20")
	entry, err := testEntryBuilder(t, testAccounts()).BuildModificationEntry(
		context.Background(), result, uuid.New(), uuid.New(), "leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
}

func TestEarlyTerminationGainAndLossEntries(t *testing.T) {
	contract := testContract(t)
	accounts := testAccounts()
	for _, test := range []struct {
		name, liability, rou, payment, gainLoss string
	}{
		{"gain", "100", "80", "10", "10"},
		{"loss", "50", "80", "10", "-40"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := contract.EarlyTerminate(date(2026, 2, 1),
				money(t, test.liability, "USD"), money(t, test.rou, "USD"), money(t, test.payment, "USD"))
			if err != nil {
				t.Fatal(err)
			}
			assertAmount(t, result.TerminationGainLoss, test.gainLoss)
			entry, err := testEntryBuilder(t, accounts).BuildTerminationEntry(
				context.Background(), result, uuid.New(), uuid.New(), "leases", "tester")
			if err != nil {
				t.Fatal(err)
			}
			assertBalanced(t, entry)
		})
	}
}

func TestUnitClosePeriodBuildsPaymentAndDepreciationEntries(t *testing.T) {
	unit := NewUnit()
	builder := testEntryBuilder(t, testAccounts())
	entries, err := unit.ClosePeriod(
		context.Background(), testContract(t), date(2026, 3, 1), builder, "leases", uuid.New(), uuid.New(), "lease-close",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("ClosePeriod() returned %d entries, want payment and depreciation for two periods", len(entries))
	}
	for _, entry := range entries {
		assertBalanced(t, entry)
	}
}

// ---------------------------------------------------------------------------
// Lessor: operating and finance leases
// ---------------------------------------------------------------------------

func TestOperatingLeaseStraightLineSchedule(t *testing.T) {
	contract, err := NewOperatingLeaseContract(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")},
			{DueDate: date(2026, 10, 1), Amount: money(t, "400", "USD")},
		},
		money(t, "1000", "USD"), money(t, "100", "USD"), 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-operating-1")

	assertAmount(t, contract.TotalConsideration, "600")
	assertAmount(t, contract.DepreciableAssetBase, "1100")

	schedule, err := contract.StraightLineSchedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule) != 2 {
		t.Fatalf("schedule length = %d, want 2", len(schedule))
	}
	assertAmount(t, schedule[0].LeaseIncome, "300")
	assertAmount(t, schedule[1].LeaseIncome, "300")
	assertAmount(t, schedule[0].DeferredMovement, "100")
	assertAmount(t, schedule[0].CumulativeDeferredIncome, "100")
	assertAmount(t, schedule[1].DeferredMovement, "-100")
	assertAmount(t, schedule[1].CumulativeDeferredIncome, "0")
	assertAmount(t, schedule[0].AssetDepreciation, "550")
	assertAmount(t, schedule[1].AssetDepreciation, "550")
	assertAmount(t, schedule[1].AccumulatedDepreciation, "1100")
	assertAmount(t, schedule[1].ClosingAssetCarrying, "0")
}

func TestOperatingLeaseRetainedCarryingWhenUsefulLifeExceedsTerm(t *testing.T) {
	contract, err := NewOperatingLeaseContract(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2027, 1, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2028, 1, 1), Amount: money(t, "100", "USD")},
		},
		money(t, "900", "USD"), money(t, "0", "USD"), 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := contract.StraightLineSchedule()
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, schedule[1].AssetDepreciation, "225")
	assertAmount(t, schedule[1].AccumulatedDepreciation, "450")
	assertAmount(t, schedule[1].ClosingAssetCarrying, "450")
}

func TestOperatingLeaseEntriesBalance(t *testing.T) {
	contract, err := NewOperatingLeaseContract(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")},
			{DueDate: date(2026, 10, 1), Amount: money(t, "400", "USD")},
		},
		money(t, "1000", "USD"), money(t, "100", "USD"), 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-operating-entry")
	builder := testLessorEntryBuilder(t, testLessorAccounts())
	companyID, journalID := uuid.New(), uuid.New()
	ctx := context.Background()

	idc, err := builder.BuildOperatingInitialDirectCostsEntry(ctx, contract, companyID, journalID, "lessor-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, idc)

	schedule, err := contract.StraightLineSchedule()
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range schedule {
		income, err := builder.BuildOperatingIncomeEntry(ctx, period, companyID, journalID, "lessor-leases", "tester")
		if err != nil {
			t.Fatal(err)
		}
		assertBalanced(t, income)
		depreciation, err := builder.BuildOperatingDepreciationEntry(ctx, period, companyID, journalID, "lessor-leases", "tester")
		if err != nil {
			t.Fatal(err)
		}
		assertBalanced(t, depreciation)
	}
}

func TestOperatingLeaseValidation(t *testing.T) {
	cases := []struct {
		name       string
		payments   []LeasePayment
		assetCost  string
		directCost string
		periods    int
	}{
		{"zero asset cost", []LeasePayment{{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")}}, "0", "0", 0},
		{"negative direct costs", []LeasePayment{{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")}}, "100", "-5", 0},
		{"unordered payments", []LeasePayment{
			{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")},
		}, "100", "0", 0},
		{"negative depreciation periods", []LeasePayment{{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")}}, "100", "0", -1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewOperatingLeaseContract(
				date(2026, 1, 1), test.payments,
				money(t, test.assetCost, "USD"), money(t, test.directCost, "USD"), test.periods,
			); err == nil {
				t.Fatal("expected invalid operating lease error")
			}
		})
	}
}

func TestOperatingDepreciationRejectsZeroPeriod(t *testing.T) {
	contract, err := NewOperatingLeaseContract(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2026, 7, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2027, 1, 1), Amount: money(t, "100", "USD")},
		},
		money(t, "1000", "USD"), money(t, "0", "USD"), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-operating-zero")
	schedule, err := contract.StraightLineSchedule()
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, schedule[1].AssetDepreciation, "0")
	if _, err := testLessorEntryBuilder(t, testLessorAccounts()).BuildOperatingDepreciationEntry(
		context.Background(), schedule[1], uuid.New(), uuid.New(), "lessor-leases", "tester",
	); err != ErrAssetDepreciationUnav {
		t.Fatalf("BuildOperatingDepreciationEntry error = %v, want %v", err, ErrAssetDepreciationUnav)
	}
}

func TestOperatingIncomeEntryRequiresContraAccounts(t *testing.T) {
	contract, err := NewOperatingLeaseContract(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2026, 7, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2027, 1, 1), Amount: money(t, "300", "USD")},
		},
		money(t, "1000", "USD"), money(t, "0", "USD"), 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-operating-contra")
	schedule, err := contract.StraightLineSchedule()
	if err != nil {
		t.Fatal(err)
	}
	companyID, journalID := uuid.New(), uuid.New()
	cases := []struct {
		name    string
		period  StraightLineLeasePeriod
		missing func(*LessorAccounts)
	}{
		{"missing accrued rent receivable", schedule[0], func(a *LessorAccounts) { a.AccruedRentReceivable = uuid.Nil }},
		{"missing deferred lease income", schedule[1], func(a *LessorAccounts) { a.DeferredLeaseIncome = uuid.Nil }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			broken := testLessorAccounts()
			test.missing(&broken)
			builder := testLessorEntryBuilder(t, broken)
			if _, err := builder.BuildOperatingIncomeEntry(
				context.Background(), test.period, companyID, journalID, "lessor-leases", "tester",
			); err == nil {
				t.Fatal("expected invalid lessor accounts error")
			}
		})
	}
}

func TestFinanceLeaseZeroRateExactBalances(t *testing.T) {
	contract, err := NewFinanceLeaseContract(
		date(2026, 1, 1), decimal.Zero,
		[]LeasePayment{
			{DueDate: date(2026, 12, 31), Amount: money(t, "5000", "USD")},
			{DueDate: date(2027, 12, 31), Amount: money(t, "5000", "USD")},
		},
		money(t, "1000", "USD"), money(t, "0", "USD"), money(t, "8000", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-finance-0")

	assertAmount(t, contract.NetInvestment, "11000")
	assertAmount(t, contract.FairValueEquivalent, "11000")
	assertAmount(t, contract.DerecognitionGainLoss, "3000")

	schedule, err := contract.FinanceIncomeSchedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule) != 2 {
		t.Fatalf("schedule length = %d, want 2", len(schedule))
	}
	assertAmount(t, schedule[0].FinanceIncome, "0")
	assertAmount(t, schedule[0].PrincipalRecovery, "5000")
	assertAmount(t, schedule[0].ClosingNetInvestment, "6000")
	assertAmount(t, schedule[1].ClosingNetInvestment, "1000")
	assertAmount(t, schedule[1].ClosingNetInvestment, contract.UnguaranteedResidual.Amount().String())
}

func TestFinanceLeaseNetInvestmentAndFinanceIncome(t *testing.T) {
	contract, err := NewFinanceLeaseContract(
		date(2026, 1, 1), decimal.RequireFromString("0.10"),
		[]LeasePayment{
			{DueDate: date(2026, 12, 31), Amount: money(t, "5000", "USD")},
			{DueDate: date(2027, 12, 31), Amount: money(t, "5000", "USD")},
		},
		money(t, "1000", "USD"), money(t, "200", "USD"), money(t, "8000", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-finance-1")

	receiptsPV, err := PresentValue(contract.Payments, contract.CommencementDate, contract.ImplicitRate, contract.Currency)
	if err != nil {
		t.Fatal(err)
	}
	residualPV, err := PresentValue(
		[]LeasePayment{{DueDate: date(2027, 12, 31), Amount: money(t, "1000", "USD")}},
		contract.CommencementDate, contract.ImplicitRate, contract.Currency)
	if err != nil {
		t.Fatal(err)
	}
	expectedNI := receiptsPV.Amount().Add(residualPV.Amount()).Add(decimal.RequireFromString("200"))
	if !contract.NetInvestment.Amount().Equal(expectedNI) {
		t.Fatalf("net investment = %s, want %s", contract.NetInvestment.Amount(), expectedNI)
	}

	schedule, err := contract.FinanceIncomeSchedule()
	if err != nil {
		t.Fatal(err)
	}
	totalIncome := decimal.Zero
	for _, period := range schedule {
		if period.FinanceIncome.Amount().IsNegative() || period.ClosingNetInvestment.Amount().IsNegative() {
			t.Fatalf("negative finance income or net investment in period %d", period.Period)
		}
		totalIncome = totalIncome.Add(period.FinanceIncome.Amount())
	}
	expectedIncome := decimal.RequireFromString("10000").Add(decimal.RequireFromString("1000")).Sub(contract.NetInvestment.Amount())
	if !totalIncome.Round(scale).Equal(expectedIncome.Round(scale)) {
		t.Fatalf("total finance income = %s, want %s", totalIncome, expectedIncome)
	}
	assertAmount(t, schedule[len(schedule)-1].ClosingNetInvestment, "1000")
}

func TestFinanceLeaseEntriesBalanceAndIdempotency(t *testing.T) {
	contract, err := NewFinanceLeaseContract(
		date(2026, 1, 1), decimal.RequireFromString("0.10"),
		[]LeasePayment{
			{DueDate: date(2026, 12, 31), Amount: money(t, "5000", "USD")},
			{DueDate: date(2027, 12, 31), Amount: money(t, "5000", "USD")},
		},
		money(t, "1000", "USD"), money(t, "200", "USD"), money(t, "8000", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-finance-entry")
	accounts := testLessorAccounts()
	builder := testLessorEntryBuilder(t, accounts)
	companyID, journalID := uuid.New(), uuid.New()
	ctx := context.Background()

	commencement, err := builder.BuildFinanceCommencementEntry(ctx, contract, companyID, journalID, "lessor-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, commencement)
	retry, err := builder.BuildFinanceCommencementEntry(ctx, contract, companyID, journalID, "lessor-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if commencement.ID != retry.ID || EntryIdempotencyKey(commencement) != EntryIdempotencyKey(retry) {
		t.Fatal("retry did not reproduce the same finance commencement identity")
	}
	if commencement.SourceModule != "ifrs16" || commencement.ReferenceID != contract.Reference {
		t.Fatalf("commencement traceability fields are incomplete: %+v", commencement)
	}

	schedule, err := contract.FinanceIncomeSchedule()
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range schedule {
		receipt, err := builder.BuildFinanceReceiptEntry(ctx, period, companyID, journalID, "lessor-leases", "tester")
		if err != nil {
			t.Fatal(err)
		}
		assertBalanced(t, receipt)
	}
}

func TestFinanceLeaseValidation(t *testing.T) {
	valid := []LeasePayment{{DueDate: date(2026, 12, 31), Amount: money(t, "5000", "USD")}}
	cases := []struct {
		name  string
		rate  decimal.Decimal
		carry string
	}{
		{"negative implicit rate", decimal.NewFromInt(-1), "8000"},
		{"negative asset carrying", decimal.Zero, "-10"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewFinanceLeaseContract(
				date(2026, 1, 1), test.rate, valid,
				money(t, "1000", "USD"), money(t, "200", "USD"), money(t, test.carry, "USD"),
			); err == nil {
				t.Fatal("expected invalid finance lease error")
			}
		})
	}
}

func TestLessorEntryBuilderRequiresCapabilityAndMapping(t *testing.T) {
	if _, err := NewLessorEntryBuilder(testLessorPostingCapability{accounts: testLessorAccounts()}); err != ErrPostingCapabilityUnavailable {
		t.Fatalf("NewLessorEntryBuilder without posting capability error = %v", err)
	}
	builder := testLessorEntryBuilder(t, testLessorAccounts())
	contract := testOperatingContract(t)
	if _, err := builder.BuildOperatingIncomeEntry(
		context.Background(), StraightLineLeasePeriod{LeaseReference: contract.Reference}, uuid.New(), uuid.New(), "", "tester",
	); err != ErrJournalMappingUnavailable {
		t.Fatalf("BuildOperatingIncomeEntry without mapping error = %v", err)
	}
}

func TestFinanceLeaseDerecognitionLossEntry(t *testing.T) {
	contract, err := NewFinanceLeaseContract(
		date(2026, 1, 1), decimal.Zero,
		[]LeasePayment{{DueDate: date(2026, 12, 31), Amount: money(t, "11000", "USD")}},
		money(t, "0", "USD"), money(t, "0", "USD"), money(t, "12000", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-finance-loss")

	assertAmount(t, contract.DerecognitionGainLoss, "-1000")
	entry, err := testLessorEntryBuilder(t, testLessorAccounts()).BuildFinanceCommencementEntry(
		context.Background(), contract, uuid.New(), uuid.New(), "lessor-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
}

func TestFinanceLeaseCommencementRequiresAccounts(t *testing.T) {
	contract, err := NewFinanceLeaseContract(
		date(2026, 1, 1), decimal.Zero,
		[]LeasePayment{{DueDate: date(2026, 12, 31), Amount: money(t, "1000", "USD")}},
		money(t, "0", "USD"), money(t, "50", "USD"), money(t, "800", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("lessor-finance-accounts")
	accounts := testLessorAccounts()
	companyID, journalID := uuid.New(), uuid.New()

	for name, mutate := range map[string]func(*LessorAccounts){
		"missing net investment account": func(a *LessorAccounts) { a.NetInvestmentInLease = uuid.Nil },
		"missing derecognition gain":     func(a *LessorAccounts) { a.DerecognitionGain = uuid.Nil },
		"missing cash for direct costs":  func(a *LessorAccounts) { a.Cash = uuid.Nil },
	} {
		t.Run(name, func(t *testing.T) {
			broken := accounts
			mutate(&broken)
			brokenBuilder := testLessorEntryBuilder(t, broken)
			if _, err := brokenBuilder.BuildFinanceCommencementEntry(
				context.Background(), contract, companyID, journalID, "lessor-leases", "tester",
			); err == nil {
				t.Fatal("expected invalid lessor accounts error")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Exemptions
// ---------------------------------------------------------------------------

func TestShortTermExemptionEligibilityAndExpense(t *testing.T) {
	lease, err := NewExemptibleLease(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2026, 2, 1), Amount: money(t, "200", "USD")},
			{DueDate: date(2026, 4, 1), Amount: money(t, "400", "USD")},
		},
		ShortTermExemption, 3, false,
		money(t, "0", "USD"), money(t, "0", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	lease = lease.WithReference("short-term-1")

	assertAmount(t, lease.TotalLeasePayments, "600")
	schedule, err := lease.ExpenseSchedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule) != 2 {
		t.Fatalf("schedule length = %d, want 2", len(schedule))
	}
	assertAmount(t, schedule[0].LeaseExpense, "300")
	assertAmount(t, schedule[1].LeaseExpense, "300")
	assertAmount(t, schedule[0].AccrualMovement, "100")
	assertAmount(t, schedule[0].CumulativeAccrual, "100")
	assertAmount(t, schedule[1].AccrualMovement, "-100")
	assertAmount(t, schedule[1].CumulativeAccrual, "0")

	builder := testExemptionEntryBuilder(t, testExemptionAccounts())
	companyID, journalID := uuid.New(), uuid.New()
	for _, period := range schedule {
		entry, err := builder.BuildExpenseEntry(context.Background(), period, companyID, journalID, "exempt-leases", "tester")
		if err != nil {
			t.Fatal(err)
		}
		assertBalanced(t, entry)
	}
}

func TestShortTermExemptionEvenPayments(t *testing.T) {
	payments := make([]LeasePayment, 0, 12)
	for month := 2; month <= 13; month++ {
		year, calMonth := 2026, month
		if month > 12 {
			year, calMonth = 2027, month-12
		}
		payments = append(payments, LeasePayment{
			DueDate: date(year, time.Month(calMonth), 1), Amount: money(t, "100", "USD"),
		})
	}
	lease, err := NewExemptibleLease(date(2026, 1, 1), payments, ShortTermExemption, 12, false,
		money(t, "0", "USD"), money(t, "0", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := lease.ExpenseSchedule()
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule) != 12 {
		t.Fatalf("schedule length = %d, want 12", len(schedule))
	}
	assertAmount(t, schedule[11].LeaseExpense, "100")
	assertAmount(t, schedule[11].CumulativeAccrual, "0")
}

func TestShortTermExemptionRejections(t *testing.T) {
	sixMonths := []LeasePayment{
		{DueDate: date(2026, 7, 1), Amount: money(t, "100", "USD")},
	}
	cases := []struct {
		name     string
		term     int
		purchase bool
		payments []LeasePayment
	}{
		{"term exceeds twelve months", 13, false, sixMonths},
		{"purchase option", 6, true, sixMonths},
		{"payment beyond the term", 6, false, []LeasePayment{
			{DueDate: date(2026, 12, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2027, 6, 1), Amount: money(t, "100", "USD")},
		}},
		{"unknown exemption type", 6, false, []LeasePayment{{DueDate: date(2026, 7, 1), Amount: money(t, "100", "USD")}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			exemption := ShortTermExemption
			if test.name == "unknown exemption type" {
				exemption = LeaseExemption("bogus")
			}
			if _, err := NewExemptibleLease(
				date(2026, 1, 1), test.payments, exemption, test.term, test.purchase,
				money(t, "0", "USD"), money(t, "0", "USD"),
			); err == nil {
				t.Fatal("expected exemption eligibility error")
			}
		})
	}
}

func TestLowValueExemptionThresholdAndExpense(t *testing.T) {
	lease, err := NewExemptibleLease(
		date(2026, 1, 1),
		[]LeasePayment{
			{DueDate: date(2026, 6, 1), Amount: money(t, "300", "USD")},
			{DueDate: date(2026, 9, 1), Amount: money(t, "500", "USD")},
		},
		LowValueExemption, 8, false,
		money(t, "3000", "USD"), money(t, "5000", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	lease = lease.WithReference("low-value-1")

	assertAmount(t, lease.AssetFairValue, "3000")
	assertAmount(t, lease.LowValueThreshold, "5000")
	schedule, err := lease.ExpenseSchedule()
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, schedule[0].LeaseExpense, "400")
	assertAmount(t, schedule[1].LeaseExpense, "400")
	assertAmount(t, schedule[1].CumulativeAccrual, "0")

	builder := testExemptionEntryBuilder(t, testExemptionAccounts())
	entry, err := builder.BuildExpenseEntry(
		context.Background(), schedule[0], uuid.New(), uuid.New(), "exempt-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	assertBalanced(t, entry)
}

func TestLowValueExemptionRejections(t *testing.T) {
	payments := []LeasePayment{{DueDate: date(2026, 6, 1), Amount: money(t, "100", "USD")}}
	cases := []struct {
		name      string
		fairValue string
		threshold string
	}{
		{"asset above the threshold", "6000", "5000"},
		{"zero asset fair value", "0", "5000"},
		{"zero threshold", "1000", "0"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewExemptibleLease(
				date(2026, 1, 1), payments, LowValueExemption, 5, false,
				money(t, test.fairValue, "USD"), money(t, test.threshold, "USD"),
			); err == nil {
				t.Fatal("expected exemption eligibility error")
			}
		})
	}
}

func TestExemptionEntryRetryStability(t *testing.T) {
	lease, err := NewExemptibleLease(
		date(2026, 1, 1),
		[]LeasePayment{{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")}},
		ShortTermExemption, 3, false,
		money(t, "0", "USD"), money(t, "0", "USD"),
	)
	if err != nil {
		t.Fatal(err)
	}
	lease = lease.WithReference("short-term-retry")
	schedule, err := lease.ExpenseSchedule()
	if err != nil {
		t.Fatal(err)
	}
	builder := testExemptionEntryBuilder(t, testExemptionAccounts())
	companyID, journalID := uuid.New(), uuid.New()
	ctx := context.Background()

	first, err := builder.BuildExpenseEntry(ctx, schedule[0], companyID, journalID, "exempt-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := builder.BuildExpenseEntry(ctx, schedule[0], companyID, journalID, "exempt-leases", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != retry.ID || EntryIdempotencyKey(first) != EntryIdempotencyKey(retry) ||
		EntryIdempotencyPayloadHash(first) != EntryIdempotencyPayloadHash(retry) {
		t.Fatal("retry did not reproduce the same exemption entry identity and payload")
	}
	if first.SourceModule != "ifrs16" || first.ReferenceID != lease.Reference || first.Description == "" {
		t.Fatalf("exemption traceability fields are incomplete: %+v", first)
	}
}

// ---------------------------------------------------------------------------
// Disclosures
// ---------------------------------------------------------------------------

func TestFutureLeasePaymentBuckets(t *testing.T) {
	contract, err := NewLeaseContract(
		date(2026, 1, 1), decimal.RequireFromString("0.10"),
		[]LeasePayment{
			{DueDate: date(2026, 7, 1), Amount: money(t, "1000", "USD")},
			{DueDate: date(2027, 7, 1), Amount: money(t, "1000", "USD")},
			{DueDate: date(2028, 1, 1), Amount: money(t, "1000", "USD")},
			{DueDate: date(2032, 1, 1), Amount: money(t, "500", "USD")},
		},
		money(t, "0", "USD"), money(t, "0", "USD"), money(t, "0", "USD"), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	contract = contract.WithReference("disclosure-lease")

	buckets, err := FutureLeasePaymentBuckets(contract, date(2026, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 6 {
		t.Fatalf("bucket count = %d, want 6", len(buckets))
	}
	expectations := map[string]string{
		"within_one_year":     "1000.0000",
		"one_to_two_years":    "2000.0000",
		"two_to_three_years":  "0.0000",
		"three_to_four_years": "0.0000",
		"four_to_five_years":  "0.0000",
		"beyond_five_years":   "500.0000",
	}
	for _, bucket := range buckets {
		if buckets[bucketIndex(t, buckets, bucket.Label)].Amount != expectations[bucket.Label] {
			t.Fatalf("bucket %s = %s, want %s", bucket.Label, bucket.Amount, expectations[bucket.Label])
		}
	}

	afterFirst, err := FutureLeasePaymentBuckets(contract, date(2026, 7, 1))
	if err != nil {
		t.Fatal(err)
	}
	if afterFirst[bucketIndex(t, afterFirst, "within_one_year")].Amount != "2000.0000" {
		t.Fatalf("post-payment analysis = %s, want 2000",
			afterFirst[bucketIndex(t, afterFirst, "within_one_year")].Amount)
	}
}

func TestLeaseCarryingLiability(t *testing.T) {
	contract := testContract(t)
	carrying, err := LeaseCarryingLiability(contract, date(2026, 1, 15))
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, carrying, contract.InitialLeaseLiability.Amount().String())

	carrying, err = LeaseCarryingLiability(contract, date(2026, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := contract.AmortizationSchedule()
	if err != nil {
		t.Fatal(err)
	}
	assertAmount(t, carrying, schedule[len(schedule)-1].ClosingLiability.Amount().String())
}

func TestBuildLeaseExpenseDisclosure(t *testing.T) {
	payload, err := BuildLeaseExpenseDisclosure(LeaseExpenseDisclosureInput{
		CompanyID:                  types.CompanyID(uuid.New()),
		PeriodStart:                date(2026, 1, 1),
		PeriodEnd:                  date(2026, 12, 31),
		Currency:                   "usd",
		ROUDepreciation:            money(t, "111.1135", "USD"),
		InterestOnLeaseLiabilities: money(t, "2.7731", "USD"),
		ShortTermLeaseExpense:      money(t, "500", "USD"),
		OperatingLeaseIncome:       money(t, "1200", "USD"),
		Narrative:                  " Lessee and lessor lease balances for 2026. ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Code != DisclosureCodeLeaseExpense || payload.StandardReference != DisclosureStandardReference ||
		payload.Version != DisclosureSchemaVersion {
		t.Fatalf("disclosure identity fields are incomplete: %+v", payload)
	}
	if payload.Currency != "USD" {
		t.Fatalf("currency = %s, want USD", payload.Currency)
	}
	if payload.Data["depreciation_of_right_of_use_assets"] != "111.1135" ||
		payload.Data["interest_on_lease_liabilities"] != "2.7731" ||
		payload.Data["short_term_lease_expense"] != "500.0000" ||
		payload.Data["operating_lease_income"] != "1200.0000" {
		t.Fatalf("unexpected expense payload data: %+v", payload.Data)
	}
	if payload.Data["total_expense"] != "613.8866" || payload.Data["total_income"] != "1200.0000" {
		t.Fatalf("unexpected expense totals: %+v", payload.Data)
	}
	if payload.Narrative != "Lessee and lessor lease balances for 2026." {
		t.Fatalf("narrative not trimmed: %q", payload.Narrative)
	}
}

func TestBuildLeaseExpenseDisclosureValidation(t *testing.T) {
	valid := LeaseExpenseDisclosureInput{
		CompanyID:       types.CompanyID(uuid.New()),
		PeriodStart:     date(2026, 1, 1),
		PeriodEnd:       date(2026, 12, 31),
		Currency:        "USD",
		ROUDepreciation: money(t, "100", "USD"),
	}
	cases := []struct {
		name   string
		mutate func(LeaseExpenseDisclosureInput) LeaseExpenseDisclosureInput
	}{
		{"no amounts supplied", func(in LeaseExpenseDisclosureInput) LeaseExpenseDisclosureInput {
			in.ROUDepreciation = types.Money{}
			return in
		}},
		{"currency mismatch", func(in LeaseExpenseDisclosureInput) LeaseExpenseDisclosureInput {
			in.ROUDepreciation = money(t, "100", "EUR")
			return in
		}},
		{"missing company", func(in LeaseExpenseDisclosureInput) LeaseExpenseDisclosureInput {
			in.CompanyID = types.CompanyID{}
			return in
		}},
		{"reversed period", func(in LeaseExpenseDisclosureInput) LeaseExpenseDisclosureInput {
			in.PeriodStart, in.PeriodEnd = in.PeriodEnd, in.PeriodStart
			return in
		}},
		{"missing currency", func(in LeaseExpenseDisclosureInput) LeaseExpenseDisclosureInput {
			in.Currency = "  "
			return in
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildLeaseExpenseDisclosure(test.mutate(valid)); err == nil {
				t.Fatal("expected disclosure validation error")
			}
		})
	}
}

func TestBuildLeaseMaturityDisclosure(t *testing.T) {
	contract := testContract(t)
	carrying, err := LeaseCarryingLiability(contract, date(2026, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	buckets, err := FutureLeasePaymentBuckets(contract, date(2026, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := BuildLeaseMaturityDisclosure(LeaseMaturityDisclosureInput{
		CompanyID:              types.CompanyID(uuid.New()),
		PeriodStart:            date(2026, 1, 1),
		PeriodEnd:              date(2026, 12, 31),
		Currency:               "USD",
		LeaseLiabilityCarrying: carrying,
		Buckets:                buckets,
		SourceReferences: []reporting.DisclosureSource{
			{Kind: "lease-contract", Reference: "test-lease"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Code != DisclosureCodeMaturity {
		t.Fatalf("code = %s, want %s", payload.Code, DisclosureCodeMaturity)
	}
	if payload.Data["total_undiscounted_payments"] != "200.0000" {
		t.Fatalf("total = %v, want 200.0000", payload.Data["total_undiscounted_payments"])
	}
	if len(payload.Source) != 2 {
		t.Fatalf("source count = %d, want module plus host reference", len(payload.Source))
	}

	if _, err := BuildLeaseMaturityDisclosure(LeaseMaturityDisclosureInput{
		CompanyID:              types.CompanyID(uuid.New()),
		PeriodStart:            date(2026, 1, 1),
		PeriodEnd:              date(2026, 12, 31),
		Currency:               "USD",
		LeaseLiabilityCarrying: carrying,
		Buckets: []MaturityBucket{
			{Label: "within_one_year", Months: 12, Amount: "not-a-number"},
		},
	}); err == nil {
		t.Fatal("expected bucket amount validation error")
	}
}

// ---------------------------------------------------------------------------
// Module descriptor, dependencies, and posting workflow
// ---------------------------------------------------------------------------

type fakeApplication struct{ publicapplication.Service }
type fakeEntryReader struct{ standards.EntryReader }
type fakeMasterDataReader struct{}

func (fakeMasterDataReader) GetCompany(context.Context, types.CompanyID) (masterdata.Company, error) {
	return masterdata.Company{}, nil
}
func (fakeMasterDataReader) GetAccount(context.Context, types.CompanyID, types.AccountID) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (fakeMasterDataReader) GetAccountByCode(context.Context, types.CompanyID, string) (masterdata.Account, error) {
	return masterdata.Account{}, nil
}
func (fakeMasterDataReader) GetJournalByCode(context.Context, types.CompanyID, string) (masterdata.Journal, error) {
	return masterdata.Journal{}, nil
}

// recordingPostingCapability captures the public posting requests emitted by
// the module adapter so the tests can assert balance and audit metadata.
type recordingPostingCapability struct {
	requests []standards.StandardPostingRequest
}

func (c *recordingPostingCapability) PostStandardEntry(
	_ context.Context, req standards.StandardPostingRequest,
) (publicapplication.EntryResult, error) {
	c.requests = append(c.requests, req)
	entryID := req.EntryID
	if uuid.UUID(entryID) == uuid.Nil {
		entryID = types.NewJournalEntryID()
	}
	return publicapplication.EntryResult{
		Entry: journal.JournalEntry{ID: entryID, Status: journal.EntryPosted},
	}, nil
}

// testModulePorts implements all five ModulePorts methods with local mocks.
type testModulePorts struct {
	app     publicapplication.Service
	posting standards.StandardPostingCapability
}

func (p testModulePorts) Entries() standards.EntryReader               { return fakeEntryReader{} }
func (p testModulePorts) Balance() standards.BalanceReader             { return nil }
func (p testModulePorts) MasterData() standards.MasterDataReader       { return fakeMasterDataReader{} }
func (p testModulePorts) Application() publicapplication.Service       { return p.app }
func (p testModulePorts) Posting() standards.StandardPostingCapability { return p.posting }

func validPorts() standards.ModulePorts {
	return standards.NewModulePorts(fakeEntryReader{}, nil, fakeMasterDataReader{}, fakeApplication{})
}

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
	if module.Name() != "IFRS 16" || module.Version() != "1.1.0" {
		t.Fatalf("module identity = %s/%s", module.Name(), module.Version())
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
	for _, eventType := range []string{"IFRS16_LEASE_PAYMENT", "IFRS9_ECL_PROVISION"} {
		if got := router.GetRules(eventType); len(got) != 0 {
			t.Errorf("unexpected hard-coded posting rules for %s: %+v", eventType, got)
		}
	}

	descriptor := module.Descriptor()
	if descriptor.ID != "ifrs16" || descriptor.Name != "IFRS 16" {
		t.Fatalf("descriptor identity = %s/%s", descriptor.ID, descriptor.Name)
	}
	if descriptor.Profile == nil || descriptor.Profile.IsConfigured() {
		t.Fatal("profile placeholders must leave the module capability unconfigured")
	}
	if err := descriptor.Profile.Validate(); err != nil {
		t.Fatalf("invalid standard profile: %v", err)
	}
	for _, workflow := range descriptor.Workflows {
		if err := workflow.Validate(*descriptor.Profile); err != nil {
			t.Errorf("invalid workflow %q: %v", workflow.ID, err)
		}
		if workflow.ProducesJournalEntries {
			t.Errorf("module unexpectedly claims application posting in workflow %q", workflow.ID)
		}
	}
	postingRequirementSupported := false
	for _, requirement := range descriptor.Profile.SupportedRequirements {
		if requirement.ID == "IFRS16-POSTING-WORKFLOW" &&
			requirement.Status == standards.RequirementLimited {
			postingRequirementSupported = true
		}
	}
	if !postingRequirementSupported {
		t.Fatal("module must claim application posting workflow in SupportedRequirements")
	}
}

func TestIFRS16PostingWorkflow(t *testing.T) {
	posting := &recordingPostingCapability{}
	ports := testModulePorts{app: fakeApplication{}, posting: posting}
	module := &Module{}
	if err := module.Initialize(context.Background(), ports); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	companyID := uuid.New()
	journalID := uuid.New()
	company := types.CompanyID(companyID)
	journalPublic := types.JournalID(journalID)
	ctx := publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID:    "lease-officer",
		CompanyIDs: []types.CompanyID{company},
	})

	accounts := testAccounts()

	t.Run("post commencement draft and commit through application", func(t *testing.T) {
		contract, err := NewLeaseContract(
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			decimal.RequireFromString("0.05"),
			[]LeasePayment{{DueDate: date(2026, 12, 31), Amount: money(t, "1000", "USD")}},
			money(t, "0", "USD"), money(t, "0", "USD"), money(t, "0", "USD"), 12,
		)
		if err != nil {
			t.Fatalf("NewLeaseContract: %v", err)
		}
		contract = contract.WithReference("LEASE-2026-001")

		result, err := module.PostCommencement(ctx, contract, accounts, companyID, journalID, "lease-officer", "idem-commence-1")
		if err != nil {
			t.Fatalf("PostCommencement: %v", err)
		}
		if result.Entry.Status != journal.EntryPosted {
			t.Errorf("status = %s, want %s", result.Entry.Status, journal.EntryPosted)
		}
		req := posting.requests[len(posting.requests)-1]
		assertPostingRequest(t, req, "IFRS-16-LEASE-COMMENCEMENT", "LEASE-2026-001", "idem-commence-1", company, journalPublic)
		if len(req.Lines) != 2 {
			t.Fatalf("commencement line count = %d, want 2", len(req.Lines))
		}
		if !req.EffectiveDate.Equal(date(2026, 1, 1)) || !req.PostingDate.Equal(date(2026, 1, 1)) {
			t.Fatalf("dates = %s/%s, want commencement date", req.EffectiveDate, req.PostingDate)
		}
	})

	t.Run("post payment, depreciation, lessor, and exemption entries", func(t *testing.T) {
		contract := testContract(t)
		schedule, err := contract.AmortizationSchedule()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.PostPayment(ctx, contract.Reference, schedule[0], accounts,
			companyID, journalID, "lease-officer", "idem-payment-1"); err != nil {
			t.Fatalf("PostPayment: %v", err)
		}
		assertPostingRequest(t, posting.requests[len(posting.requests)-1],
			"IFRS-16-LEASE-PAYMENT", contract.Reference, "idem-payment-1", company, journalPublic)

		if _, err := module.PostDepreciation(ctx, contract.Reference, schedule[0], accounts,
			companyID, journalID, "lease-officer", "idem-depreciation-1"); err != nil {
			t.Fatalf("PostDepreciation: %v", err)
		}
		assertPostingRequest(t, posting.requests[len(posting.requests)-1],
			"IFRS-16-LEASE-DEPRECIATION", contract.Reference, "idem-depreciation-1", company, journalPublic)

		operating, err := NewOperatingLeaseContract(date(2026, 1, 1),
			[]LeasePayment{{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")}},
			money(t, "1000", "USD"), money(t, "100", "USD"), 1)
		if err != nil {
			t.Fatal(err)
		}
		operating = operating.WithReference("LESSOR-OP-1")
		operatingSchedule, err := operating.StraightLineSchedule()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.PostLessorOperatingIncome(ctx, operating.Reference, operatingSchedule[0], testLessorAccounts(),
			companyID, journalID, "lease-officer", "idem-operating-1"); err != nil {
			t.Fatalf("PostLessorOperatingIncome: %v", err)
		}
		assertPostingRequest(t, posting.requests[len(posting.requests)-1],
			"IFRS-16-LESSOR-OPERATING", "LESSOR-OP-1", "idem-operating-1", company, journalPublic)

		finance, err := NewFinanceLeaseContract(date(2026, 1, 1), decimal.Zero,
			[]LeasePayment{{DueDate: date(2026, 12, 31), Amount: money(t, "1000", "USD")}},
			money(t, "0", "USD"), money(t, "0", "USD"), money(t, "800", "USD"))
		if err != nil {
			t.Fatal(err)
		}
		finance = finance.WithReference("LESSOR-FIN-1")
		if _, err := module.PostLessorFinanceCommencement(ctx, finance, testLessorAccounts(),
			companyID, journalID, "lease-officer", "idem-finance-1"); err != nil {
			t.Fatalf("PostLessorFinanceCommencement: %v", err)
		}
		assertPostingRequest(t, posting.requests[len(posting.requests)-1],
			"IFRS-16-LESSOR-FINANCE", "LESSOR-FIN-1", "idem-finance-1", company, journalPublic)

		lease, err := NewExemptibleLease(date(2026, 1, 1),
			[]LeasePayment{{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")}},
			ShortTermExemption, 3, false, money(t, "0", "USD"), money(t, "0", "USD"))
		if err != nil {
			t.Fatal(err)
		}
		lease = lease.WithReference("EXEMPT-1")
		exemptionSchedule, err := lease.ExpenseSchedule()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.PostExemptionExpense(ctx, "EXEMPT-1", exemptionSchedule[0], testExemptionAccounts(),
			companyID, journalID, "lease-officer", "idem-exemption-1"); err != nil {
			t.Fatalf("PostExemptionExpense: %v", err)
		}
		assertPostingRequest(t, posting.requests[len(posting.requests)-1],
			"IFRS-16-EXEMPTION", "EXEMPT-1", "idem-exemption-1", company, journalPublic)
	})

	t.Run("posting requires configured ports", func(t *testing.T) {
		uninitialized := &Module{}
		contract := testContract(t)
		if _, err := uninitialized.PostCommencement(ctx, contract, accounts, companyID, journalID, "lease-officer", "idem-uninit"); err == nil {
			t.Fatal("expected error when module has no configured posting capability")
		}
	})
}

func TestIFRS16PostingIdempotencyAndRetryPreventsDuplicatePosting(t *testing.T) {
	contract := testContract(t)
	accounts := testAccounts()
	builder := testEntryBuilder(t, accounts)
	companyID, journalID := uuid.New(), uuid.New()
	ctx := context.Background()

	first, err := builder.BuildCommencementEntry(ctx, contract, companyID, journalID, "leases", "auditor")
	if err != nil {
		t.Fatalf("BuildCommencementEntry: %v", err)
	}
	assertBalanced(t, first)
	retry, err := builder.BuildCommencementEntry(ctx, contract, companyID, journalID, "leases", "auditor")
	if err != nil {
		t.Fatalf("BuildCommencementEntry retry: %v", err)
	}
	if first.ID != retry.ID || EntryIdempotencyKey(first) != EntryIdempotencyKey(retry) ||
		EntryIdempotencyPayloadHash(first) != EntryIdempotencyPayloadHash(retry) {
		t.Fatalf("retry entry identity mismatch: %s vs %s", first.ID, retry.ID)
	}

	// Strict idempotency: a changed payload must conflict with the first key.
	postedStore := map[string]string{EntryIdempotencyKey(first): EntryIdempotencyPayloadHash(first)}
	if stored := postedStore[EntryIdempotencyKey(retry)]; stored != EntryIdempotencyPayloadHash(retry) {
		t.Fatal("idempotency conflict: changed payload")
	}
	remapped, err := testEntryBuilder(t, testAccounts()).BuildCommencementEntry(
		ctx, contract, companyID, journalID, "leases", "auditor")
	if err != nil {
		t.Fatal(err)
	}
	if EntryIdempotencyPayloadHash(remapped) == postedStore[EntryIdempotencyKey(remapped)] {
		t.Fatal("remapped accounts must change the payload hash")
	}
}

// ---------------------------------------------------------------------------
// Local helpers
// ---------------------------------------------------------------------------

func assertPostingRequest(
	t *testing.T,
	req standards.StandardPostingRequest,
	standardRef, entityID, idempotencyKey string,
	company types.CompanyID, journal types.JournalID,
) {
	t.Helper()
	if req.StandardRef != standardRef || req.EntityID != entityID || req.IdempotencyKey != idempotencyKey {
		t.Fatalf("posting metadata = %q/%q/%q, want %q/%q/%q",
			req.StandardRef, req.EntityID, req.IdempotencyKey, standardRef, entityID, idempotencyKey)
	}
	if req.CompanyID != company || req.JournalID != journal {
		t.Fatalf("posting destination = %s/%s, want %s/%s", req.CompanyID, req.JournalID, company, journal)
	}
	if req.Classification != journalEntryClassRegular() {
		t.Fatalf("classification = %s, want %s", req.Classification, journalEntryClassRegular())
	}
	if req.EntryID == (types.JournalEntryID{}) {
		t.Fatal("posting request must carry the deterministic entry ID")
	}
	assertLinesBalanced(t, req.Lines)
}

func journalEntryClassRegular() journal.EntryClassification {
	return journal.EntryClassRegular
}

func assertBalanced(t *testing.T, entry journal.JournalEntry) {
	t.Helper()
	if entry.ID == (types.JournalEntryID{}) {
		t.Fatal("built entry has no ID")
	}
	if entry.Status != journal.EntryDraft {
		t.Fatalf("built entry status = %s, want %s", entry.Status, journal.EntryDraft)
	}
	assertLinesBalanced(t, entry.Lines)
}

func assertLinesBalanced(t *testing.T, lines []journal.EntryLine) {
	t.Helper()
	if len(lines) < 2 {
		t.Fatalf("line count = %d, want at least two", len(lines))
	}
	debit, credit := decimal.Zero, decimal.Zero
	for i, line := range lines {
		if line.AccountID == (types.AccountID{}) {
			t.Fatalf("line %d has no account ID", i)
		}
		debit = debit.Add(line.FunctionalDebit.Amount())
		credit = credit.Add(line.FunctionalCredit.Amount())
	}
	if !debit.Equal(credit) || debit.IsZero() {
		t.Fatalf("entry lines not balanced: debit %s, credit %s", debit, credit)
	}
}

func testContract(t *testing.T) LeaseContract {
	t.Helper()
	contract, err := NewLeaseContract(
		date(2026, 1, 1), decimal.RequireFromString("0.12"),
		[]LeasePayment{
			{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2026, 3, 1), Amount: money(t, "100", "USD")},
		},
		money(t, "10", "USD"), money(t, "20", "USD"), money(t, "5", "USD"), 0,
	)
	if err != nil {
		t.Fatal(err)
	}
	return contract.WithReference("test-lease")
}

type testLeasePostingCapability struct {
	accounts  LeaseAccounts
	available bool
}

func (c testLeasePostingCapability) LedgerPostingAvailable() bool { return c.available }

func (c testLeasePostingCapability) ResolveLeaseAccounts(
	_ context.Context, _ uuid.UUID, mappingID string,
) (LeaseAccounts, error) {
	if mappingID != "leases" {
		return LeaseAccounts{}, ErrJournalMappingUnavailable
	}
	return c.accounts, nil
}

func testEntryBuilder(t *testing.T, accounts LeaseAccounts) *EntryBuilder {
	t.Helper()
	builder, err := NewEntryBuilder(testLeasePostingCapability{accounts: accounts, available: true})
	if err != nil {
		t.Fatal(err)
	}
	return builder
}

func testAccounts() LeaseAccounts {
	return LeaseAccounts{
		ROUAsset: uuid.New(), LeaseLiability: uuid.New(), Cash: uuid.New(), PrepaidLease: uuid.New(),
		InterestExpense: uuid.New(), DepreciationExpense: uuid.New(), AccumulatedDepreciation: uuid.New(),
		ModificationGain: uuid.New(), TerminationGain: uuid.New(), TerminationLoss: uuid.New(),
	}
}

type testLessorPostingCapability struct {
	accounts  LessorAccounts
	available bool
}

func (c testLessorPostingCapability) LedgerPostingAvailable() bool { return c.available }

func (c testLessorPostingCapability) ResolveLessorAccounts(
	_ context.Context, _ uuid.UUID, mappingID string,
) (LessorAccounts, error) {
	if mappingID != "lessor-leases" {
		return LessorAccounts{}, ErrJournalMappingUnavailable
	}
	return c.accounts, nil
}

func testLessorEntryBuilder(t *testing.T, accounts LessorAccounts) *LessorEntryBuilder {
	t.Helper()
	builder, err := NewLessorEntryBuilder(testLessorPostingCapability{accounts: accounts, available: true})
	if err != nil {
		t.Fatal(err)
	}
	return builder
}

func testLessorAccounts() LessorAccounts {
	return LessorAccounts{
		LeasedAsset: uuid.New(), AccumulatedDepreciation: uuid.New(), NetInvestmentInLease: uuid.New(),
		Cash: uuid.New(), OperatingLeaseIncome: uuid.New(), FinanceIncome: uuid.New(),
		DepreciationExpense: uuid.New(), DeferredLeaseIncome: uuid.New(), AccruedRentReceivable: uuid.New(),
		DerecognitionGain: uuid.New(), DerecognitionLoss: uuid.New(),
	}
}

func testOperatingContract(t *testing.T) OperatingLeaseContract {
	t.Helper()
	contract, err := NewOperatingLeaseContract(
		date(2026, 1, 1),
		[]LeasePayment{{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")}},
		money(t, "1000", "USD"), money(t, "0", "USD"), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	return contract.WithReference("lessor-operating-helper")
}

type testExemptionPostingCapability struct {
	accounts  ExemptionAccounts
	available bool
}

func (c testExemptionPostingCapability) LedgerPostingAvailable() bool { return c.available }

func (c testExemptionPostingCapability) ResolveExemptionAccounts(
	_ context.Context, _ uuid.UUID, mappingID string,
) (ExemptionAccounts, error) {
	if mappingID != "exempt-leases" {
		return ExemptionAccounts{}, ErrJournalMappingUnavailable
	}
	return c.accounts, nil
}

func testExemptionEntryBuilder(t *testing.T, accounts ExemptionAccounts) *ExemptionEntryBuilder {
	t.Helper()
	builder, err := NewExemptionEntryBuilder(testExemptionPostingCapability{accounts: accounts, available: true})
	if err != nil {
		t.Fatal(err)
	}
	return builder
}

func testExemptionAccounts() ExemptionAccounts {
	return ExemptionAccounts{
		Cash: uuid.New(), LeaseExpense: uuid.New(),
		AccruedLeaseExpense: uuid.New(), PrepaidLease: uuid.New(),
	}
}

func money(t *testing.T, amount, currency string) types.Money {
	t.Helper()
	value, err := types.NewMoneyFromString(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func assertAmount(t *testing.T, value types.Money, expected string) {
	t.Helper()
	want := decimal.RequireFromString(expected)
	if !value.Amount().Equal(want) {
		t.Fatalf("amount = %s, want %s", value.Amount(), want)
	}
}

func bucketIndex(t *testing.T, buckets []MaturityBucket, label string) int {
	t.Helper()
	for i, bucket := range buckets {
		if bucket.Label == label {
			return i
		}
	}
	t.Fatalf("missing bucket %q", label)
	return -1
}
