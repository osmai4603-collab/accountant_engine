package ifrs16

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

type p2DisclosureProvider struct {
	disclosures []reporting.StructuredDisclosure
	err         error
}

func (p *p2DisclosureProvider) Name() string { return "ifrs16-p2" }

func (p *p2DisclosureProvider) GetDisclosures(
	context.Context, reporting.DisclosureQuery,
) ([]reporting.StructuredDisclosure, error) {
	return p.disclosures, p.err
}

func TestP2IFRS16UnitAndEntryHelpers(t *testing.T) {
	ctx := context.Background()
	contract := testContract(t)
	var nilUnit *Unit

	if _, err := nilUnit.AmortizationSchedule(contract); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit schedule = %v", err)
	}
	if _, err := nilUnit.ModifyLeaseContext(contract, date(2026, 2, 1), nil, decimal.Zero,
		money(t, "0", "USD"), money(t, "0", "USD")); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit modification = %v", err)
	}
	if _, err := nilUnit.ClosePeriod(ctx, contract, date(2026, 3, 1), nil, "", uuid.New(), uuid.New(), ""); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("nil unit close period = %v", err)
	}

	builder := testEntryBuilder(t, testAccounts())
	if _, err := NewUnit().ClosePeriod(ctx, LeaseContract{}, date(2026, 3, 1), builder,
		"leases", uuid.New(), uuid.New(), ""); !errors.Is(err, ErrLeaseReferenceRequired) {
		t.Fatalf("close period without reference = %v", err)
	}
	broken := contract
	broken.Payments = nil
	broken.InitialLeaseLiability = types.Money{}
	if _, err := NewUnit().ClosePeriod(ctx, broken, date(2026, 3, 1), builder,
		"leases", uuid.New(), uuid.New(), ""); err == nil {
		t.Fatal("expected close period schedule error")
	}

	if got := zeroMoney(""); got.Currency() != "" {
		t.Fatalf("zero money with invalid currency = %+v", got)
	}
	partner := uuid.New()
	debit := debitLine(uuid.New(), money(t, "10", "USD"), &partner)
	if debit.PartnerID == nil || *debit.PartnerID != types.PartnerID(partner) {
		t.Fatalf("debit partner = %v", debit.PartnerID)
	}
	credit := creditLine(uuid.New(), money(t, "10", "USD"), &partner)
	if credit.PartnerID == nil || *credit.PartnerID != types.PartnerID(partner) {
		t.Fatalf("credit partner = %v", credit.PartnerID)
	}

	if _, err := newJournalEntry(uuid.Nil, date(2026, 2, 1), "", nil); err == nil {
		t.Fatal("expected missing journal id error")
	}
	if _, err := newJournalEntry(uuid.New(), time.Time{}, "", nil); err == nil {
		t.Fatal("expected missing entry date error")
	}
	if _, err := newJournalEntry(uuid.New(), date(2026, 2, 1), "", nil); err == nil {
		t.Fatal("expected minimum lines error")
	}
	unbalanced := []journal.EntryLine{debit, debit}
	if _, err := newJournalEntry(uuid.New(), date(2026, 2, 1), "", unbalanced); err == nil {
		t.Fatal("expected unbalanced entry error")
	}
	if _, err := newLeaseEntry(time.Time{}, uuid.New(), uuid.New(), "", unbalanced); err == nil {
		t.Fatal("expected invalid lease entry error")
	}
}

func TestP2IFRS16EnterpriseBranches(t *testing.T) {
	effective := date(2026, 12, 31)
	if _, err := EvaluateSaleAndLeaseback(SaleAndLeasebackInput{}); err == nil {
		t.Fatal("expected sale-and-leaseback validation error")
	}
	financing, err := EvaluateSaleAndLeaseback(SaleAndLeasebackInput{
		ContractID: uuid.New(), Currency: "USD", EffectiveDate: effective,
		SaleProceeds: decimal.NewFromInt(100), TransferSatisfied: false,
	})
	if err != nil || financing.TransferSatisfied || !financing.FinancingLiability.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("financing sale-and-leaseback = %+v %v", financing, err)
	}
	if _, err := EvaluateSaleAndLeaseback(SaleAndLeasebackInput{
		ContractID: uuid.New(), Currency: "USD", EffectiveDate: effective, TransferSatisfied: true,
	}); err == nil {
		t.Fatal("expected positive fair value error")
	}
	cappedLiability, err := EvaluateSaleAndLeaseback(SaleAndLeasebackInput{
		ContractID: uuid.New(), Currency: "USD", EffectiveDate: effective, TransferSatisfied: true,
		AssetCarryingAmount: decimal.NewFromInt(80), AssetFairValue: decimal.NewFromInt(100),
		SaleProceeds: decimal.NewFromInt(300),
	})
	if err != nil || !cappedLiability.LeaseLiability.IsZero() || !cappedLiability.ROUAsset.IsZero() {
		t.Fatalf("capped liability = %+v %v", cappedLiability, err)
	}
	cappedRatio, err := EvaluateSaleAndLeaseback(SaleAndLeasebackInput{
		ContractID: uuid.New(), Currency: "USD", EffectiveDate: effective, TransferSatisfied: true,
		AssetCarryingAmount: decimal.NewFromInt(80), AssetFairValue: decimal.NewFromInt(100),
		LeaseLiabilityPV: decimal.NewFromInt(500),
	})
	if err != nil || !cappedRatio.ROUAsset.Equal(decimal.NewFromInt(80)) ||
		!cappedRatio.GainLossRecognized.IsZero() {
		t.Fatalf("capped retained ratio = %+v %v", cappedRatio, err)
	}

	if _, err := ClassifyAndMeasureSublease(SubleaseInput{}); err == nil {
		t.Fatal("expected sublease validation error")
	}
	operating, err := ClassifyAndMeasureSublease(SubleaseInput{
		SubleaseID: uuid.New(), Currency: "USD", Classification: SubleaseOperating,
	})
	if err != nil || operating.Classification != SubleaseOperating {
		t.Fatalf("operating sublease = %+v %v", operating, err)
	}
	finance, err := ClassifyAndMeasureSublease(SubleaseInput{
		SubleaseID: uuid.New(), Currency: "USD", Classification: SubleaseFinance,
		HeadLeaseROUCarrying: decimal.NewFromInt(100), SubleaseReceiptsPV: decimal.NewFromInt(120),
	})
	if err != nil || !finance.GainLossOnSublease.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("finance sublease = %+v %v", finance, err)
	}
	if _, err := ClassifyAndMeasureSublease(SubleaseInput{
		SubleaseID: uuid.New(), Currency: "USD", Classification: "unsupported",
	}); err == nil {
		t.Fatal("expected unsupported sublease classification error")
	}

	if _, err := RemeasureVariablePayments(VariablePaymentInput{}); err == nil {
		t.Fatal("expected variable payment validation error")
	}
	if _, err := RemeasureVariablePayments(VariablePaymentInput{
		LeaseID: uuid.New(), Currency: "USD", LinkedToIndexOrRate: true,
	}); err == nil {
		t.Fatal("expected invalid index error")
	}
	indexed, err := RemeasureVariablePayments(VariablePaymentInput{
		LeaseID: uuid.New(), Currency: "USD", LinkedToIndexOrRate: true,
		BasePayment: decimal.NewFromInt(100), InitialIndex: decimal.NewFromInt(100),
		CurrentIndex: decimal.NewFromInt(110),
	})
	if err != nil || !indexed.LeaseLiabilityIncluded.Equal(decimal.NewFromInt(110)) {
		t.Fatalf("indexed payment = %+v %v", indexed, err)
	}
	usage, err := RemeasureVariablePayments(VariablePaymentInput{
		LeaseID: uuid.New(), Currency: "USD", UsageVolume: decimal.NewFromInt(10),
		UsageRate: decimal.NewFromInt(3),
	})
	if err != nil || !usage.PeriodExpensePL.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("usage payment = %+v %v", usage, err)
	}

	newLiability, newROU, delta := ReassessIndexOrRate(decimal.NewFromInt(100), decimal.NewFromInt(20), decimal.NewFromInt(10))
	if !newLiability.Equal(decimal.NewFromInt(20)) || !newROU.IsZero() || !delta.Equal(decimal.NewFromInt(-80)) {
		t.Fatalf("index reassessment = %s/%s/%s", newLiability, newROU, delta)
	}
	impairedROU, impairment := AssessROUImpairment(decimal.NewFromInt(100), decimal.NewFromInt(80))
	if !impairedROU.Equal(decimal.NewFromInt(80)) || !impairment.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("impairment = %s/%s", impairedROU, impairment)
	}
	unimpairedROU, impairment := AssessROUImpairment(decimal.NewFromInt(80), decimal.NewFromInt(100))
	if !unimpairedROU.Equal(decimal.NewFromInt(80)) || !impairment.IsZero() {
		t.Fatalf("no impairment = %s/%s", unimpairedROU, impairment)
	}
	if gainLoss := TerminateLease(decimal.NewFromInt(100), decimal.NewFromInt(80), decimal.NewFromInt(10)); !gainLoss.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("termination gain = %s", gainLoss)
	}
}

func TestP2IFRS16ModuleDisclosureAndPostingErrors(t *testing.T) {
	ctx := context.Background()
	mod := &Module{}
	var nilMod *Module

	if _, err := nilMod.GetDisclosures(ctx, reporting.DisclosureQuery{}); err == nil {
		t.Fatal("expected nil module error")
	}
	if _, err := mod.GetDisclosures(nil, reporting.DisclosureQuery{}); err == nil {
		t.Fatal("expected nil context error")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := mod.GetDisclosures(canceled, reporting.DisclosureQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled disclosure lookup = %v", err)
	}
	mod.SetProfileConfig(standards.StandardProfileConfig{})
	disclosures, err := mod.GetDisclosures(ctx, reporting.DisclosureQuery{})
	if err != nil || len(disclosures) != 0 {
		t.Fatalf("unconfigured disclosure lookup = %+v %v", disclosures, err)
	}
	provider := &p2DisclosureProvider{disclosures: []reporting.StructuredDisclosure{{Code: "P2-IFRS16"}}}
	mod.SetDisclosures(provider)
	disclosures, err = mod.GetDisclosures(ctx, reporting.DisclosureQuery{})
	if err != nil || len(disclosures) != 1 || disclosures[0].Code != "P2-IFRS16" {
		t.Fatalf("configured disclosure lookup = %+v %v", disclosures, err)
	}

	companyID, journalID := uuid.New(), uuid.New()
	if _, err := mod.PostCommencement(ctx, LeaseContract{}, LeaseAccounts{}, companyID, journalID, "", ""); err == nil {
		t.Fatal("expected invalid commencement entry error")
	}
	if _, err := mod.PostPayment(ctx, "", SchedulePeriod{}, LeaseAccounts{}, companyID, journalID, "", ""); err == nil {
		t.Fatal("expected invalid payment entry error")
	}
	if _, err := mod.PostDepreciation(ctx, "", SchedulePeriod{}, LeaseAccounts{}, companyID, journalID, "", ""); err == nil {
		t.Fatal("expected invalid depreciation entry error")
	}
	if _, err := mod.PostLessorOperatingIncome(ctx, "", StraightLineLeasePeriod{}, LessorAccounts{}, companyID, journalID, "", ""); err == nil {
		t.Fatal("expected invalid lessor income entry error")
	}
	if _, err := mod.PostLessorFinanceCommencement(ctx, FinanceLeaseContract{}, LessorAccounts{}, companyID, journalID, "", ""); err == nil {
		t.Fatal("expected invalid finance commencement entry error")
	}
	if _, err := mod.PostExemptionExpense(ctx, "", ExemptionPeriod{}, ExemptionAccounts{}, companyID, journalID, "", ""); err == nil {
		t.Fatal("expected invalid exemption entry error")
	}
}

func TestP2IFRS16LesseeEntryBuilderBranches(t *testing.T) {
	ctx := context.Background()
	companyID, journalID := uuid.New(), uuid.New()
	if _, err := NewEntryBuilder(nil); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil posting capability = %v", err)
	}
	var nilBuilder *EntryBuilder
	if _, err := nilBuilder.BuildCommencementEntry(ctx, testContract(t), companyID, journalID, "leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil builder commencement = %v", err)
	}
	if _, err := nilBuilder.BuildPaymentEntry(ctx, SchedulePeriod{}, companyID, journalID, "leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil builder payment = %v", err)
	}
	if _, err := nilBuilder.BuildDepreciationEntry(ctx, SchedulePeriod{}, companyID, journalID, "leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil builder depreciation = %v", err)
	}
	if _, err := nilBuilder.BuildModificationEntry(ctx, ModificationResult{}, companyID, journalID, "leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil builder modification = %v", err)
	}
	if _, err := nilBuilder.BuildTerminationEntry(ctx, TerminationResult{}, companyID, journalID, "leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil builder termination = %v", err)
	}

	builder := testEntryBuilder(t, testAccounts())
	if _, err := builder.BuildPaymentEntry(ctx, SchedulePeriod{}, companyID, journalID, "bad", ""); !errors.Is(err, ErrJournalMappingUnavailable) {
		t.Fatalf("unresolvable mapping = %v", err)
	}

	contract := testContract(t)
	modification, err := contract.RecalculateModification(
		date(2026, 2, 1),
		[]LeasePayment{{DueDate: date(2026, 3, 1), Amount: money(t, "100", "USD")}},
		decimal.Zero, money(t, "60", "USD"), money(t, "20", "USD"),
	)
	if err != nil {
		t.Fatalf("positive modification: %v", err)
	}
	entry, err := BuildModificationEntry(modification, testAccounts(), companyID, journalID, "tester")
	if err != nil {
		t.Fatalf("BuildModificationEntry: %v", err)
	}
	assertBalanced(t, entry)
	if _, err := BuildModificationEntry(ModificationResult{}, testAccounts(), companyID, journalID, "tester"); !errors.Is(err, ErrLeaseReferenceRequired) {
		t.Fatalf("modification without reference = %v", err)
	}
	zeroAdjustment := modification
	zeroAdjustment.LiabilityAdjustment = zeroMoney("USD")
	if _, err := BuildModificationEntry(zeroAdjustment, testAccounts(), companyID, journalID, "tester"); !errors.Is(err, ErrInvalidCarrying) {
		t.Fatalf("zero modification adjustment = %v", err)
	}

	termination, err := contract.EarlyTerminate(date(2026, 2, 1),
		money(t, "100", "USD"), money(t, "80", "USD"), money(t, "10", "USD"))
	if err != nil {
		t.Fatalf("termination: %v", err)
	}
	entry, err = BuildTerminationEntry(termination, testAccounts(), companyID, journalID, "tester")
	if err != nil {
		t.Fatalf("BuildTerminationEntry: %v", err)
	}
	assertBalanced(t, entry)
	if _, err := BuildTerminationEntry(TerminationResult{}, testAccounts(), companyID, journalID, "tester"); !errors.Is(err, ErrLeaseReferenceRequired) {
		t.Fatalf("termination without reference = %v", err)
	}
}

func TestP2IFRS16LessorEntryAndContractBranches(t *testing.T) {
	ctx := context.Background()
	companyID, journalID := uuid.New(), uuid.New()
	if _, err := NewLessorEntryBuilder(nil); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil lessor posting capability = %v", err)
	}
	var nilBuilder *LessorEntryBuilder
	if _, err := nilBuilder.BuildOperatingInitialDirectCostsEntry(ctx, testOperatingContract(t), companyID, journalID, "lessor-leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil lessor builder initial costs = %v", err)
	}
	if _, err := nilBuilder.BuildOperatingIncomeEntry(ctx, StraightLineLeasePeriod{}, companyID, journalID, "lessor-leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil lessor builder income = %v", err)
	}
	if _, err := nilBuilder.BuildOperatingDepreciationEntry(ctx, StraightLineLeasePeriod{}, companyID, journalID, "lessor-leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil lessor builder depreciation = %v", err)
	}
	if _, err := nilBuilder.BuildFinanceCommencementEntry(ctx, FinanceLeaseContract{}, companyID, journalID, "lessor-leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil lessor builder commencement = %v", err)
	}
	if _, err := nilBuilder.BuildFinanceReceiptEntry(ctx, FinanceLeasePeriod{}, companyID, journalID, "lessor-leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil lessor builder receipt = %v", err)
	}

	builder := testLessorEntryBuilder(t, testLessorAccounts())
	if _, err := builder.BuildFinanceReceiptEntry(ctx, FinanceLeasePeriod{}, companyID, journalID, "bad", ""); !errors.Is(err, ErrJournalMappingUnavailable) {
		t.Fatalf("unresolvable lessor mapping = %v", err)
	}
	receipt := FinanceLeasePeriod{
		LeaseReference:    "p2-finance-receipt",
		Period:            1,
		DueDate:           date(2026, 2, 1),
		CashReceived:      money(t, "100", "USD"),
		FinanceIncome:     money(t, "10", "USD"),
		PrincipalRecovery: money(t, "90", "USD"),
	}
	entry, err := BuildLessorFinanceReceiptEntry(receipt, testLessorAccounts(), companyID, journalID, "tester")
	if err != nil {
		t.Fatalf("BuildLessorFinanceReceiptEntry: %v", err)
	}
	assertBalanced(t, entry)
	negativeRecovery, err := types.NewMoney(decimal.NewFromInt(-10), "USD")
	if err != nil {
		t.Fatalf("negative recovery fixture: %v", err)
	}
	receipt.FinanceIncome = money(t, "110", "USD")
	receipt.PrincipalRecovery = negativeRecovery
	entry, err = BuildLessorFinanceReceiptEntry(receipt, testLessorAccounts(), companyID, journalID, "tester")
	if err != nil {
		t.Fatalf("negative principal recovery receipt: %v", err)
	}
	assertBalanced(t, entry)
	if _, err := BuildLessorFinanceReceiptEntry(FinanceLeasePeriod{}, testLessorAccounts(), companyID, journalID, "tester"); !errors.Is(err, ErrLeaseReferenceRequired) {
		t.Fatalf("finance receipt without reference = %v", err)
	}

	payments := []LeasePayment{{DueDate: date(2026, 12, 31), Amount: money(t, "1000", "USD")}}
	if _, err := NewFinanceLeaseContract(date(2026, 1, 1), decimal.Zero, payments,
		types.Money{}, money(t, "50", "USD"), money(t, "800", "USD")); err != nil {
		t.Fatalf("finance lease residual currency fallback = %v", err)
	}
	if _, err := NewFinanceLeaseContract(date(2026, 1, 1), decimal.Zero, payments,
		types.Money{}, types.Money{}, money(t, "800", "USD")); err != nil {
		t.Fatalf("finance lease carrying currency fallback = %v", err)
	}
	if _, err := NewFinanceLeaseContract(date(2026, 1, 1), decimal.Zero, payments,
		types.Money{}, types.Money{}, types.Money{}); err != nil {
		t.Fatalf("finance lease payment currency fallback = %v", err)
	}
	if _, err := NewFinanceLeaseContract(date(2026, 1, 1), decimal.Zero,
		[]LeasePayment{{DueDate: date(2026, 12, 31), Amount: types.Money{}}},
		types.Money{}, types.Money{}, types.Money{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("finance lease without currency = %v", err)
	}
	if _, err := NewOperatingLeaseContract(date(2026, 1, 1), payments,
		types.Money{}, money(t, "100", "USD"), 1); err == nil {
		t.Fatal("expected operating lease asset cost error after currency fallback")
	}
	if _, err := NewOperatingLeaseContract(date(2026, 1, 1), payments,
		types.Money{}, types.Money{}, 1); err == nil {
		t.Fatal("expected operating lease asset cost error after payment currency fallback")
	}
}

func TestP2IFRS16ExemptionEntryBranches(t *testing.T) {
	ctx := context.Background()
	companyID, journalID := uuid.New(), uuid.New()
	if _, err := NewExemptionEntryBuilder(nil); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil exemption posting capability = %v", err)
	}
	var nilBuilder *ExemptionEntryBuilder
	if _, err := nilBuilder.BuildExpenseEntry(ctx, ExemptionPeriod{}, companyID, journalID, "exempt-leases", ""); !errors.Is(err, ErrPostingCapabilityUnavailable) {
		t.Fatalf("nil exemption builder = %v", err)
	}
	builder := testExemptionEntryBuilder(t, testExemptionAccounts())
	if _, err := builder.BuildExpenseEntry(ctx, ExemptionPeriod{}, companyID, journalID, "bad", ""); !errors.Is(err, ErrJournalMappingUnavailable) {
		t.Fatalf("unresolvable exemption mapping = %v", err)
	}

	lease, err := NewExemptibleLease(date(2026, 1, 1),
		[]LeasePayment{{DueDate: date(2026, 4, 1), Amount: money(t, "200", "USD")}},
		ShortTermExemption, 3, false, money(t, "0", "USD"), money(t, "0", "USD"))
	if err != nil {
		t.Fatalf("exemption fixture: %v", err)
	}
	schedule, err := lease.ExpenseSchedule()
	if err != nil {
		t.Fatalf("exemption schedule: %v", err)
	}
	period := schedule[0]
	period.LeaseReference = "p2-exemption"
	entry, err := BuildExemptionExpenseEntry(period, testExemptionAccounts(), companyID, journalID, "tester")
	if err != nil {
		t.Fatalf("BuildExemptionExpenseEntry: %v", err)
	}
	assertBalanced(t, entry)
	if _, err := BuildExemptionExpenseEntry(ExemptionPeriod{}, testExemptionAccounts(), companyID, journalID, "tester"); !errors.Is(err, ErrLeaseReferenceRequired) {
		t.Fatalf("exemption without reference = %v", err)
	}
	invalid := period
	invalid.LeaseExpense = zeroMoney("USD")
	if _, err := BuildExemptionExpenseEntry(invalid, testExemptionAccounts(), companyID, journalID, "tester"); !errors.Is(err, ErrInvalidExemptionAccnt) {
		t.Fatalf("exemption without expense = %v", err)
	}
	missingAccrual := period
	missingAccrual.AccrualMovement = money(t, "10", "USD")
	accounts := testExemptionAccounts()
	accounts.AccruedLeaseExpense = uuid.Nil
	if _, err := BuildExemptionExpenseEntry(missingAccrual, accounts, companyID, journalID, "tester"); !errors.Is(err, ErrInvalidExemptionAccnt) {
		t.Fatalf("exemption without accrued account = %v", err)
	}
	missingPrepaid := period
	missingPrepaid.AccrualMovement, err = types.NewMoney(decimal.NewFromInt(-10), "USD")
	if err != nil {
		t.Fatalf("negative accrual fixture: %v", err)
	}
	accounts = testExemptionAccounts()
	accounts.PrepaidLease = uuid.Nil
	if _, err := BuildExemptionExpenseEntry(missingPrepaid, accounts, companyID, journalID, "tester"); !errors.Is(err, ErrInvalidExemptionAccnt) {
		t.Fatalf("exemption without prepaid account = %v", err)
	}

	invalidPayment := []LeasePayment{{DueDate: date(2026, 6, 1), Amount: types.Money{}}}
	if _, err := NewExemptibleLease(date(2026, 1, 1), invalidPayment, ShortTermExemption, 6, false,
		money(t, "100", "USD"), money(t, "100", "USD")); err == nil {
		t.Fatal("expected exemption fair-value currency fallback error")
	}
	if _, err := NewExemptibleLease(date(2026, 1, 1), invalidPayment, ShortTermExemption, 6, false,
		types.Money{}, money(t, "100", "USD")); err == nil {
		t.Fatal("expected exemption threshold currency fallback error")
	}
	if _, err := NewExemptibleLease(date(2026, 1, 1), invalidPayment, ShortTermExemption, 6, false,
		types.Money{}, types.Money{}); !errors.Is(err, ErrInvalidExemption) {
		t.Fatalf("exemption without currency = %v", err)
	}
}

func TestP2IFRS16DisclosureValidationBranches(t *testing.T) {
	if _, err := FutureLeasePaymentBuckets(LeaseContract{}, date(2026, 1, 1)); !errors.Is(err, ErrInvalidDisclosureInput) {
		t.Fatalf("invalid maturity input = %v", err)
	}
	if _, err := LeaseCarryingLiability(LeaseContract{}, date(2026, 1, 1)); !errors.Is(err, ErrInvalidDisclosureInput) {
		t.Fatalf("invalid liability input = %v", err)
	}
	brokenSchedule := LeaseContract{Currency: "USD"}
	if _, err := LeaseCarryingLiability(brokenSchedule, date(2026, 1, 1)); err == nil {
		t.Fatal("expected liability schedule error")
	}

	expense := LeaseExpenseDisclosureInput{
		CompanyID:       types.CompanyID(uuid.New()),
		PeriodStart:     date(2026, 1, 1),
		PeriodEnd:       date(2026, 12, 31),
		Currency:        "USD",
		ROUDepreciation: money(t, "100", "USD"),
		SourceReferences: []reporting.DisclosureSource{
			{Kind: "", Reference: "missing-kind"},
		},
	}
	if _, err := BuildLeaseExpenseDisclosure(expense); !errors.Is(err, ErrInvalidDisclosureInput) {
		t.Fatalf("expense disclosure with invalid source = %v", err)
	}

	maturity := LeaseMaturityDisclosureInput{
		CompanyID:              types.CompanyID(uuid.New()),
		PeriodStart:            date(2026, 1, 1),
		PeriodEnd:              date(2026, 12, 31),
		Currency:               "USD",
		LeaseLiabilityCarrying: money(t, "10", "USD"),
		Buckets: []MaturityBucket{
			{Label: "within_one_year", Months: 12, Amount: "10.0000"},
		},
	}
	mutations := []struct {
		name   string
		mutate func(*LeaseMaturityDisclosureInput)
	}{
		{"missing company", func(in *LeaseMaturityDisclosureInput) { in.CompanyID = types.CompanyID{} }},
		{"wrong carrying currency", func(in *LeaseMaturityDisclosureInput) { in.LeaseLiabilityCarrying = money(t, "10", "EUR") }},
		{"missing buckets", func(in *LeaseMaturityDisclosureInput) { in.Buckets = nil }},
		{"empty bucket label", func(in *LeaseMaturityDisclosureInput) { in.Buckets[0].Label = "  " }},
		{"negative bucket boundary", func(in *LeaseMaturityDisclosureInput) { in.Buckets[0].Months = -1 }},
		{"negative bucket amount", func(in *LeaseMaturityDisclosureInput) { in.Buckets[0].Amount = "-1.0000" }},
		{"invalid source", func(in *LeaseMaturityDisclosureInput) {
			in.SourceReferences = []reporting.DisclosureSource{{Kind: "contract", Reference: " "}}
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			input := maturity
			input.Buckets = append([]MaturityBucket(nil), maturity.Buckets...)
			test.mutate(&input)
			if _, err := BuildLeaseMaturityDisclosure(input); !errors.Is(err, ErrInvalidDisclosureInput) {
				t.Fatalf("maturity disclosure error = %v", err)
			}
		})
	}
	if months := wholeMonths(date(2026, 6, 1), date(2026, 1, 1)); months != 0 {
		t.Fatalf("whole months for reversed dates = %d", months)
	}
}

func TestP2IFRS16ScheduleAndConstructorValidationBranches(t *testing.T) {
	if _, err := BuildAmortizationSchedule(LeaseContract{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("empty amortization schedule = %v", err)
	}
	backwards := LeaseContract{
		Reference:             "p2-backwards",
		Currency:              "USD",
		CommencementDate:      date(2026, 2, 1),
		InitialLeaseLiability: money(t, "100", "USD"),
		Payments: []LeasePayment{
			{DueDate: date(2026, 1, 1), Amount: money(t, "10", "USD")},
		},
	}
	if _, err := BuildAmortizationSchedule(backwards); err == nil {
		t.Fatal("expected amortization growth factor error")
	}
	if _, err := PresentValue([]LeasePayment{{DueDate: date(2026, 1, 1), Amount: money(t, "10", "USD")}},
		date(2026, 2, 1), decimal.Zero, "USD"); err == nil {
		t.Fatal("expected present value growth factor error")
	}
	if _, err := periodGrowthFactor(decimal.NewFromInt(-1), 24*time.Hour); err == nil {
		t.Fatal("expected negative discount rate error")
	}
	if _, err := BuildStraightLineSchedule(OperatingLeaseContract{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("empty straight-line schedule = %v", err)
	}
	if _, err := BuildFinanceIncomeSchedule(FinanceLeaseContract{}); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("empty finance income schedule = %v", err)
	}
	negativeNetInvestment := FinanceLeaseContract{
		Reference:        "p2-negative-net",
		Currency:         "USD",
		CommencementDate: date(2026, 1, 1),
		NetInvestment:    money(t, "10", "USD"),
		Payments: []LeasePayment{
			{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")},
			{DueDate: date(2026, 3, 1), Amount: money(t, "1", "USD")},
		},
	}
	if _, err := BuildFinanceIncomeSchedule(negativeNetInvestment); !errors.Is(err, ErrNegativeNetInvestment) {
		t.Fatalf("negative net investment schedule = %v", err)
	}
	if _, err := BuildExemptionExpenseSchedule(ExemptibleLease{}); !errors.Is(err, ErrInvalidExemption) {
		t.Fatalf("empty exemption schedule = %v", err)
	}

	payment := []LeasePayment{{DueDate: date(2026, 2, 1), Amount: money(t, "100", "USD")}}
	if _, err := NewLeaseContract(date(2026, 1, 1), decimal.Zero, payment,
		types.Money{}, money(t, "10", "USD"), money(t, "0", "USD"), 1); err == nil {
		t.Fatal("expected prepayment currency fallback validation error")
	}
	if _, err := NewLeaseContract(date(2026, 1, 1), decimal.Zero, payment,
		types.Money{}, types.Money{}, money(t, "10", "USD"), 1); err == nil {
		t.Fatal("expected incentive currency fallback validation error")
	}
	if _, err := NewLeaseContract(date(2026, 1, 1), decimal.Zero, payment,
		types.Money{}, types.Money{}, types.Money{}, 1); err == nil {
		t.Fatal("expected payment currency fallback validation error")
	}
	if _, err := NewLeaseContract(date(2026, 1, 1), decimal.Zero,
		[]LeasePayment{{DueDate: date(2026, 2, 1), Amount: types.Money{}}},
		types.Money{}, types.Money{}, types.Money{}, 1); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("lease without currency = %v", err)
	}
}
