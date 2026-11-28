package ifrs9

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

func TestIFRS9EnterpriseCapabilities(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	derec, err := EvaluateDerecognition(DerecognitionInput{
		InstrumentID:          uuid.New(),
		Kind:                  DerecognitionTransfer,
		GrossCarryingAmount:   decimal.NewFromInt(1000),
		LossAllowance:         decimal.NewFromInt(50),
		ConsiderationReceived: decimal.NewFromInt(980),
		Currency:              "USD",
		EffectiveDate:         now,
		HostEvidenceReference: "TRANSFER-2026-01",
	})
	if err != nil || !derec.Derecognized || !derec.DerecognitionGainLoss.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("derecognition = %+v, %v", derec, err)
	}

	hedge, err := EvaluateHedgeEffectiveness(HedgeInput{
		RelationshipID:        uuid.New(),
		Type:                  HedgeCashFlow,
		HedgingInstrumentGain: decimal.NewFromInt(120),
		HedgedItemGain:        decimal.NewFromInt(100),
		Currency:              "USD",
		EffectiveDate:         now,
		DocumentationRef:      "HEDGE-DOC-01",
	})
	if err != nil || !hedge.EffectivePortionOCI.Equal(decimal.NewFromInt(100)) || !hedge.IneffectivenessPL.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("hedge = %+v, %v", hedge, err)
	}

	if err := ValidateModelGovernance(ModelGovernanceInput{
		ModelVersion:          "v2.1",
		SPPIAssessmentRef:     "SPPI-01",
		BusinessModelRef:      "BM-01",
		IndependentReviewerID: "reviewer-1",
		ValidationReportRef:   "VAL-2026-01",
		ValidatedAt:           now,
	}); err != nil {
		t.Fatalf("ValidateModelGovernance: %v", err)
	}

	mod := &Module{}
	mod.SetProfileConfig(standards.StandardProfileConfig{
		Edition:       "IFRS 9 (2024)",
		Amendments:    []string{"Classification and Measurement Amendments 2024"},
		EffectiveDate: "2026-01-01",
		Jurisdiction:  "EU-IFRS",
	})
	desc := mod.Descriptor()
	if desc.Profile.Jurisdiction != "EU-IFRS" || desc.Profile.Edition != "IFRS 9 (2024)" {
		t.Fatalf("unexpected profile config: %+v", desc.Profile)
	}
	if err := desc.Profile.Validate(); err != nil {
		t.Fatalf("profile validate: %v", err)
	}
}
