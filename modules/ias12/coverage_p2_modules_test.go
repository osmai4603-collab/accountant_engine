package ias12

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	publictypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

func TestP2IAS12UnitFacadeBranches(t *testing.T) {
	var nilUnit *Unit
	asOf := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	if _, err := nilUnit.Measure(nil, MeasurementPeriod{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit Measure = %v", err)
	}
	if _, err := NewUnit().Measure(nil, MeasurementPeriod{}); err == nil {
		t.Fatal("expected invalid period error")
	}

	if _, err := nilUnit.MeasureWithProfile(TaxProfile{}, nil, MeasurementPeriod{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit MeasureWithProfile = %v", err)
	}
	if _, err := NewUnit().MeasureWithProfile(TaxProfile{}, nil, MeasurementPeriod{}); err == nil {
		t.Fatal("expected invalid profile period error")
	}

	if _, err := nilUnit.MeasureCurrentTax(TaxProfile{}, CurrentTaxInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit MeasureCurrentTax = %v", err)
	}
	if _, err := NewUnit().MeasureCurrentTax(TaxProfile{}, CurrentTaxInput{}); err == nil {
		t.Fatal("expected invalid current tax period error")
	}

	if _, err := nilUnit.Reconcile(TaxReconciliationInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit Reconcile = %v", err)
	}
	if _, err := NewUnit().Reconcile(TaxReconciliationInput{}); err == nil {
		t.Fatal("expected invalid reconciliation period error")
	}

	if _, err := nilUnit.MeasureUncertainTaxTreatment(UncertainTaxTreatment{}, asOf); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit MeasureUncertainTaxTreatment = %v", err)
	}
	if _, err := NewUnit().MeasureUncertainTaxTreatment(UncertainTaxTreatment{}, asOf); err == nil {
		t.Fatal("expected invalid treatment error")
	}

	if _, err := nilUnit.ReassessUncertainTaxTreatment(UncertainTaxTreatment{}, UncertainTaxTreatment{}, asOf, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit ReassessUncertainTaxTreatment = %v", err)
	}
	if _, err := NewUnit().ReassessUncertainTaxTreatment(UncertainTaxTreatment{}, UncertainTaxTreatment{}, asOf, "reason"); err == nil {
		t.Fatal("expected invalid reassessment error")
	}

	if _, err := nilUnit.GenerateConfiguredDeferredTaxEntry(Measurement{}, DeferredTaxPosition{}, ConfiguredPosting{}, uuid.New(), "actor"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit GenerateConfiguredDeferredTaxEntry = %v", err)
	}
	if _, err := NewUnit().GenerateConfiguredDeferredTaxEntry(Measurement{}, DeferredTaxPosition{}, ConfiguredPosting{}, uuid.New(), "actor"); err == nil {
		t.Fatal("expected unconfigured deferred posting error")
	}

	if _, err := nilUnit.GenerateConfiguredCurrentTaxEntry(CurrentTaxMeasurement{}, ConfiguredPosting{}, uuid.New(), "actor"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit GenerateConfiguredCurrentTaxEntry = %v", err)
	}
	if _, err := NewUnit().GenerateConfiguredCurrentTaxEntry(CurrentTaxMeasurement{}, ConfiguredPosting{}, uuid.New(), "actor"); err == nil {
		t.Fatal("expected unconfigured current posting error")
	}

	if _, err := nilUnit.GenerateDeferredTaxEntry(DeferredTaxEntryInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit GenerateDeferredTaxEntry = %v", err)
	}
	if _, err := NewUnit().GenerateDeferredTaxEntry(DeferredTaxEntryInput{}); err == nil {
		t.Fatal("expected invalid deferred entry error")
	}

	if _, err := nilUnit.GenerateCurrentTaxEntry(CurrentTaxEntryInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil unit GenerateCurrentTaxEntry = %v", err)
	}
	if _, err := NewUnit().GenerateCurrentTaxEntry(CurrentTaxEntryInput{}); err == nil {
		t.Fatal("expected invalid current entry error")
	}
}

func TestP2IAS12MoneyAndLineHelpers(t *testing.T) {
	money, err := publictypes.NewMoney(decimal.NewFromInt(25), "USD")
	if err != nil {
		t.Fatalf("money fixture: %v", err)
	}
	partner := uuid.New()
	if line := debitLine(uuid.New(), money, &partner); line.PartnerID == nil || *line.PartnerID != publictypes.PartnerID(partner) {
		t.Fatalf("debit partner = %v", line.PartnerID)
	}
	if line := creditLine(uuid.New(), money, &partner); line.PartnerID == nil || *line.PartnerID != publictypes.PartnerID(partner) {
		t.Fatalf("credit partner = %v", line.PartnerID)
	}
	negative, err := publictypes.NewMoney(decimal.NewFromInt(-1), "USD")
	if err != nil {
		t.Fatalf("negative money fixture: %v", err)
	}
	if err := validateNonnegativeMoney(negative); !errors.Is(err, publictypes.ErrNegativeAmountProhibited) {
		t.Fatalf("negative money validation = %v", err)
	}
	if sameDate(time.Time{}, time.Now()) {
		t.Fatal("only one zero date must not be the same date")
	}
}

func TestP2IAS12EnterpriseBranchCoverage(t *testing.T) {
	periodEnd := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	base := CurrentTaxFullInput{
		CompanyID: uuid.New(), PeriodEnd: periodEnd, AccountingProfit: decimal.NewFromInt(100),
		StatutoryRate: decimal.RequireFromString("0.20"), Currency: "USD",
	}
	invalid := base
	invalid.CompanyID = uuid.Nil
	if _, err := CalculateCurrentTaxFull(invalid); err == nil {
		t.Fatal("expected missing company error")
	}
	invalid = base
	invalid.StatutoryRate = decimal.NewFromInt(2)
	if _, err := CalculateCurrentTaxFull(invalid); err == nil {
		t.Fatal("expected invalid rate error")
	}
	invalid = base
	invalid.TaxCreditsApplied = decimal.NewFromInt(-1)
	if _, err := CalculateCurrentTaxFull(invalid); err == nil {
		t.Fatal("expected negative credits error")
	}

	loss := base
	loss.TaxLossCarryforwardUsed = decimal.NewFromInt(200)
	res, err := CalculateCurrentTaxFull(loss)
	if err != nil || !res.TaxableProfit.IsZero() {
		t.Fatalf("capped loss carryforward = %+v %v", res, err)
	}
	negativeProfit := base
	negativeProfit.AccountingProfit = decimal.NewFromInt(-100)
	res, err = CalculateCurrentTaxFull(negativeProfit)
	if err != nil || !res.TaxableProfit.IsZero() {
		t.Fatalf("negative profit = %+v %v", res, err)
	}
	credits := base
	credits.TaxCreditsApplied = decimal.NewFromInt(100)
	res, err = CalculateCurrentTaxFull(credits)
	if err != nil || !res.NetCurrentTax.IsZero() {
		t.Fatalf("excess credits = %+v %v", res, err)
	}

	if _, err := BuildTaxPresentationAndDisclosure(TaxPresentationInput{}); err == nil {
		t.Fatal("expected presentation validation error")
	}
	presentation, err := BuildTaxPresentationAndDisclosure(TaxPresentationInput{
		CompanyID:               publictypes.NewCompanyID(),
		PeriodStart:             periodEnd.AddDate(0, -1, 0),
		PeriodEnd:               periodEnd,
		CurrentTaxAssets:        decimal.NewFromInt(10),
		CurrentTaxLiabilities:   decimal.NewFromInt(50),
		DeferredTaxAssets:       decimal.NewFromInt(10),
		DeferredTaxLiabilities:  decimal.NewFromInt(50),
		LegallyEnforceableRight: true,
		Currency:                "USD",
	})
	if err != nil || !presentation.NetCurrentTaxLiability.Equal(decimal.NewFromInt(40)) ||
		!presentation.NetDeferredTaxLiability.Equal(decimal.NewFromInt(40)) {
		t.Fatalf("netting liabilities = %+v %v", presentation, err)
	}
}

func TestP2IAS12ModuleBranchCoverage(t *testing.T) {
	ctx := context.Background()
	mod := NewModule(nil)
	var nilMod *Module
	if _, err := nilMod.GetDisclosures(ctx, reporting.DisclosureQuery{}); err == nil {
		t.Fatal("expected nil module error")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := mod.GetDisclosures(canceled, reporting.DisclosureQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled disclosures = %v", err)
	}
	if _, err := mod.PostDeferredTax(ctx, DeferredTaxEntryInput{}, "", ""); err == nil {
		t.Fatal("expected missing ports for deferred posting")
	}
	if _, err := mod.PostCurrentTax(ctx, CurrentTaxEntryInput{}, "", ""); err == nil {
		t.Fatal("expected missing ports for current posting")
	}
}

func TestP2IAS12SmallHelperBranches(t *testing.T) {
	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if _, err := newJournalEntry(uuid.Nil, asOf, "draft", nil); err == nil {
		t.Fatal("expected missing journal id error")
	}
	if _, err := newJournalEntry(uuid.New(), time.Time{}, "draft", nil); err == nil {
		t.Fatal("expected missing entry date error")
	}
	if _, err := newJournalEntry(uuid.New(), asOf, "draft", nil); err == nil {
		t.Fatal("expected minimum lines error")
	}
	unbalanced := make([]journal.EntryLine, 2)
	if _, err := newJournalEntry(uuid.New(), asOf, "draft", unbalanced); err == nil {
		t.Fatal("expected unbalanced lines error")
	}
	if err := validateMoney(publictypes.Money{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("money without currency = %v", err)
	}
	usd, err := publictypes.NewMoney(decimal.NewFromInt(1), "USD")
	if err != nil {
		t.Fatalf("USD fixture: %v", err)
	}
	eur, err := publictypes.NewMoney(decimal.NewFromInt(1), "EUR")
	if err != nil {
		t.Fatalf("EUR fixture: %v", err)
	}
	if err := sameCurrency(usd, eur); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("currency mismatch = %v", err)
	}
	if got := zeroMoney(""); got.Currency() != "" {
		t.Fatalf("zero money with invalid currency = %+v", got)
	}
	provider, err := NewTaxReconciliationDisclosureProvider()
	if err != nil || provider == nil || provider.Name() != "IAS 12 tax reconciliation" {
		t.Fatalf("empty disclosure provider = %+v %v", provider, err)
	}
	if _, err := NewTaxReconciliationDisclosureProvider(TaxReconciliationDisclosureInput{}); err == nil {
		t.Fatal("expected invalid disclosure input error")
	}
}

func TestP2IAS12ProfileBranchCoverage(t *testing.T) {
	asOf := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if err := (TaxProfile{}).Validate(asOf); !errors.Is(err, ErrInvalidTaxProfile) {
		t.Fatalf("empty profile = %v", err)
	}
	if err := (TaxRate{Value: decimal.NewFromInt(-1)}).Validate(asOf); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("negative rate = %v", err)
	}
	if err := (TaxRate{Value: decimal.RequireFromString("0.2")}).Validate(asOf); !errors.Is(err, ErrRateNotEnacted) {
		t.Fatalf("unenacted rate = %v", err)
	}
	if _, err := (TaxProfile{}).RateFor(asOf, asOf); err == nil {
		t.Fatal("expected invalid profile rate error")
	}
}
