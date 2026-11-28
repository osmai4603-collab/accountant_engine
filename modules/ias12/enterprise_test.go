package ias12

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

func TestIAS12EnterpriseCapabilities(t *testing.T) {
	now := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	ct, err := CalculateCurrentTaxFull(CurrentTaxFullInput{
		CompanyID:               uuid.New(),
		PeriodEnd:               now,
		AccountingProfit:        decimal.NewFromInt(1000),
		PermanentAdditions:      decimal.NewFromInt(100),
		PermanentDeductions:     decimal.NewFromInt(50),
		TemporaryNetAdjustment:  decimal.NewFromInt(50),
		TaxLossCarryforwardUsed: decimal.NewFromInt(100),
		StatutoryRate:           decimal.RequireFromString("0.20"),
		TaxCreditsApplied:       decimal.NewFromInt(20),
		Currency:                "USD",
		Jurisdiction:            "US-FED",
	})
	if err != nil || !ct.TaxableProfit.Equal(decimal.NewFromInt(1000)) || !ct.NetCurrentTax.Equal(decimal.NewFromInt(180)) {
		t.Fatalf("current tax = %+v, %v", ct, err)
	}

	pres, err := BuildTaxPresentationAndDisclosure(TaxPresentationInput{
		CompanyID:               types.NewCompanyID(),
		PeriodStart:             now.AddDate(0, -11, -30),
		PeriodEnd:               now,
		CurrentTaxAssets:        decimal.NewFromInt(50),
		CurrentTaxLiabilities:   decimal.NewFromInt(200),
		DeferredTaxAssets:       decimal.NewFromInt(300),
		DeferredTaxLiabilities:  decimal.NewFromInt(100),
		LegallyEnforceableRight: true,
		CurrentTaxExpense:       decimal.NewFromInt(180),
		DeferredTaxExpense:      decimal.NewFromInt(-20),
		Currency:                "USD",
	})
	if err != nil || !pres.NetCurrentTaxLiability.Equal(decimal.NewFromInt(150)) || !pres.NetDeferredTaxAsset.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("presentation = %+v, %v", pres, err)
	}

	mod := NewModule(nil)
	mod.SetProfileConfig(standards.StandardProfileConfig{
		Edition:       "IAS 12 (2024)",
		Amendments:    []string{"International Tax Reform - Pillar Two Model Rules"},
		EffectiveDate: "2026-01-01",
		Jurisdiction:  "EU-IFRS",
	})
	desc := mod.Descriptor()
	if err := desc.Profile.Validate(); err != nil {
		t.Fatalf("profile validate: %v", err)
	}
}
