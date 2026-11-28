package ifrs15

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

func TestIFRS15EnterpriseCapabilities(t *testing.T) {
	pa, err := EvaluatePrincipalVsAgent(PrincipalAgentInput{
		ContractID:         uuid.New(),
		PrimaryFulfillment: false,
		GrossConsideration: decimal.NewFromInt(1000),
		SupplierCost:       decimal.NewFromInt(850),
		Currency:           "USD",
		AssessmentRef:      "PA-2026-01",
	})
	if err != nil || pa.Role != RoleAgent || !pa.RecognizedRevenue.Equal(decimal.NewFromInt(150)) {
		t.Fatalf("principal vs agent = %+v, %v", pa, err)
	}

	ip, err := EvaluateIPLicense(IPLicenseInput{
		ContractID:     uuid.New(),
		LicenseType:    IPLicenseRightToAccess,
		FixedFee:       decimal.NewFromInt(5000),
		SalesUsageBase: decimal.NewFromInt(20000),
		RoyaltyRate:    decimal.RequireFromString("0.05"),
		SalesOccurred:  true,
		Currency:       "USD",
	})
	if err != nil || ip.RecognitionPattern != "over_time" || !ip.TotalRevenue.Equal(decimal.NewFromInt(6000)) {
		t.Fatalf("IP license = %+v, %v", ip, err)
	}

	if err := (ControlTransferInput{
		ContractID:         uuid.New(),
		ObligationID:       uuid.New(),
		SatisfactionDate:   time.Now().UTC(),
		ProgressPercentage: decimal.NewFromInt(75),
		HostEvidenceRef:    "MILESTONE-3",
		MeasurementMethod:  "output_milestone",
	}).Validate(); err != nil {
		t.Fatalf("ControlTransferInput validate: %v", err)
	}

	mod := NewModule(WithStandardProfile(standards.StandardProfileConfig{
		Edition:       "IFRS 15 (2024)",
		Amendments:    []string{"Clarifications to IFRS 15"},
		EffectiveDate: "2026-01-01",
		Jurisdiction:  "UK-IFRS",
	}))
	desc := mod.Descriptor()
	if desc.Profile.Jurisdiction != "UK-IFRS" || desc.Profile.Edition != "IFRS 15 (2024)" {
		t.Fatalf("unexpected profile config: %+v", desc.Profile)
	}
	if err := desc.Profile.Validate(); err != nil {
		t.Fatalf("profile validate: %v", err)
	}
}
