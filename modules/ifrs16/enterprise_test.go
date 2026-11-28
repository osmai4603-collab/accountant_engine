package ifrs16

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

func TestIFRS16EnterpriseCapabilities(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	slb, err := EvaluateSaleAndLeaseback(SaleAndLeasebackInput{
		ContractID:          uuid.New(),
		AssetCarryingAmount: decimal.NewFromInt(800),
		AssetFairValue:      decimal.NewFromInt(1000),
		SaleProceeds:        decimal.NewFromInt(1000),
		LeaseLiabilityPV:    decimal.NewFromInt(400),
		TransferSatisfied:   true,
		Currency:            "USD",
		EffectiveDate:       now,
	})
	if err != nil || !slb.ROUAsset.Equal(decimal.RequireFromString("320")) || !slb.GainLossRecognized.Equal(decimal.RequireFromString("120")) {
		t.Fatalf("sale and leaseback = %+v, %v", slb, err)
	}

	sub, err := ClassifyAndMeasureSublease(SubleaseInput{
		SubleaseID:           uuid.New(),
		HeadLeaseROUCarrying: decimal.NewFromInt(300),
		SubleaseReceiptsPV:   decimal.NewFromInt(350),
		Classification:       SubleaseFinance,
		Currency:             "USD",
	})
	if err != nil || !sub.GainLossOnSublease.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("sublease = %+v, %v", sub, err)
	}

	vp, err := RemeasureVariablePayments(VariablePaymentInput{
		LeaseID:             uuid.New(),
		BasePayment:         decimal.NewFromInt(1000),
		LinkedToIndexOrRate: true,
		InitialIndex:        decimal.NewFromInt(100),
		CurrentIndex:        decimal.NewFromInt(110),
		Currency:            "USD",
	})
	if err != nil || !vp.LeaseLiabilityIncluded.Equal(decimal.NewFromInt(1100)) {
		t.Fatalf("variable payment = %+v, %v", vp, err)
	}

	newLiab, newROU, delta := ReassessIndexOrRate(decimal.NewFromInt(1000), decimal.NewFromInt(1150), decimal.NewFromInt(900))
	if !newLiab.Equal(decimal.NewFromInt(1150)) || !newROU.Equal(decimal.NewFromInt(1050)) || !delta.Equal(decimal.NewFromInt(150)) {
		t.Fatalf("reassess = %s, %s, %s", newLiab, newROU, delta)
	}

	impROU, impLoss := AssessROUImpairment(decimal.NewFromInt(1050), decimal.NewFromInt(900))
	if !impROU.Equal(decimal.NewFromInt(900)) || !impLoss.Equal(decimal.NewFromInt(150)) {
		t.Fatalf("impairment = %s, %s", impROU, impLoss)
	}

	termGL := TerminateLease(decimal.NewFromInt(500), decimal.NewFromInt(450), decimal.NewFromInt(20))
	if !termGL.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("termination gain/loss = %s", termGL)
	}

	mod := &Module{}
	mod.SetProfileConfig(standards.StandardProfileConfig{
		Edition:       "IFRS 16 (2024)",
		Amendments:    []string{"Lease Liability in a Sale and Leaseback"},
		EffectiveDate: "2026-01-01",
		Jurisdiction:  "IASB",
	})
	desc := mod.Descriptor()
	if err := desc.Profile.Validate(); err != nil {
		t.Fatalf("profile validate: %v", err)
	}
	discs, err := mod.GetDisclosures(context.Background(), reporting.DisclosureQuery{CompanyID: types.NewCompanyID()})
	if err != nil || len(discs) != 0 {
		t.Fatalf("GetDisclosures = %+v, %v", discs, err)
	}
}
