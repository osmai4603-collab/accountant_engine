package ifrs9

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

// DerecognitionKind distinguishes full derecognition, modification, and write-off (IFRS 9.3.2 / 5.4.4).
type DerecognitionKind string

const (
	DerecognitionTransfer     DerecognitionKind = "transfer"
	DerecognitionModification DerecognitionKind = "modification"
	DerecognitionWriteOff     DerecognitionKind = "write_off"
)

type DerecognitionInput struct {
	InstrumentID           uuid.UUID
	Kind                   DerecognitionKind
	GrossCarryingAmount    decimal.Decimal
	LossAllowance          decimal.Decimal
	ConsiderationReceived  decimal.Decimal
	ModifiedPresentValue   decimal.Decimal
	SubstantialMod         bool
	Currency               string
	EffectiveDate          time.Time
	HostEvidenceReference  string
}

type DerecognitionResult struct {
	InstrumentID         uuid.UUID
	Kind                 DerecognitionKind
	NetCarryingAmount    decimal.Decimal
	DerecognitionGainLoss decimal.Decimal
	AllowanceReleased    decimal.Decimal
	Derecognized         bool
	Currency             string
}

// EvaluateDerecognition computes derecognition, substantial modification, or write-off
// outcomes from host-evidenced carrying amounts and consideration (IFRS 9.3.2 / 5.4.4).
func EvaluateDerecognition(in DerecognitionInput) (DerecognitionResult, error) {
	if in.InstrumentID == uuid.Nil || strings.TrimSpace(in.Currency) == "" || in.EffectiveDate.IsZero() {
		return DerecognitionResult{}, errors.New("ifrs9: instrument id, currency, and effective date are required")
	}
	if strings.TrimSpace(in.HostEvidenceReference) == "" {
		return DerecognitionResult{}, errors.New("ifrs9: host derecognition evidence reference is required")
	}
	if in.GrossCarryingAmount.IsNegative() || in.LossAllowance.IsNegative() {
		return DerecognitionResult{}, errors.New("ifrs9: carrying amount and loss allowance must be non-negative")
	}
	netCarrying := in.GrossCarryingAmount.Sub(in.LossAllowance)
	switch in.Kind {
	case DerecognitionTransfer:
		gainLoss := in.ConsiderationReceived.Sub(netCarrying)
		return DerecognitionResult{
			InstrumentID:          in.InstrumentID,
			Kind:                  in.Kind,
			NetCarryingAmount:     netCarrying,
			DerecognitionGainLoss: gainLoss,
			AllowanceReleased:     in.LossAllowance,
			Derecognized:          true,
			Currency:              in.Currency,
		}, nil
	case DerecognitionModification:
		gainLoss := in.ModifiedPresentValue.Sub(in.GrossCarryingAmount)
		return DerecognitionResult{
			InstrumentID:          in.InstrumentID,
			Kind:                  in.Kind,
			NetCarryingAmount:     netCarrying,
			DerecognitionGainLoss: gainLoss,
			Derecognized:          in.SubstantialMod,
			Currency:              in.Currency,
		}, nil
	case DerecognitionWriteOff:
		unreservedLoss := in.GrossCarryingAmount.Sub(in.LossAllowance)
		if unreservedLoss.IsNegative() {
			unreservedLoss = decimal.Zero
		}
		return DerecognitionResult{
			InstrumentID:          in.InstrumentID,
			Kind:                  in.Kind,
			NetCarryingAmount:     netCarrying,
			DerecognitionGainLoss: unreservedLoss.Neg(),
			AllowanceReleased:     in.LossAllowance,
			Derecognized:          true,
			Currency:              in.Currency,
		}, nil
	default:
		return DerecognitionResult{}, fmt.Errorf("ifrs9: unsupported derecognition kind %q", in.Kind)
	}
}

type HedgeType string

const (
	HedgeFairValue HedgeType = "fair_value"
	HedgeCashFlow  HedgeType = "cash_flow"
)

type HedgeInput struct {
	RelationshipID        uuid.UUID
	Type                  HedgeType
	HedgingInstrumentGain decimal.Decimal
	HedgedItemGain        decimal.Decimal
	Currency              string
	EffectiveDate         time.Time
	DocumentationRef      string
}

type HedgeResult struct {
	RelationshipID      uuid.UUID
	Type                HedgeType
	EffectivePortionOCI decimal.Decimal
	IneffectivenessPL   decimal.Decimal
	FairValueAdjPL      decimal.Decimal
	Effective           bool
}

// EvaluateHedgeEffectiveness measures IFRS 9 Chapter 6 hedge accounting outcomes
// (fair-value hedge P&L offset and cash-flow hedge OCI/ineffectiveness split).
func EvaluateHedgeEffectiveness(in HedgeInput) (HedgeResult, error) {
	if in.RelationshipID == uuid.Nil || strings.TrimSpace(in.Currency) == "" || in.EffectiveDate.IsZero() {
		return HedgeResult{}, errors.New("ifrs9: hedge relationship id, currency, and effective date are required")
	}
	if strings.TrimSpace(in.DocumentationRef) == "" {
		return HedgeResult{}, errors.New("ifrs9: hedge designation documentation reference is required")
	}
	switch in.Type {
	case HedgeFairValue:
		netPL := in.HedgingInstrumentGain.Add(in.HedgedItemGain)
		return HedgeResult{
			RelationshipID:    in.RelationshipID,
			Type:              in.Type,
			FairValueAdjPL:    in.HedgedItemGain,
			IneffectivenessPL: netPL,
			Effective:         true,
		}, nil
	case HedgeCashFlow:
		// Lower of cumulative gain/loss on hedging instrument and hedged item goes to OCI; excess is ineffectiveness in P&L.
		absInst := in.HedgingInstrumentGain.Abs()
		absItem := in.HedgedItemGain.Abs()
		effective := in.HedgingInstrumentGain
		if absItem.LessThan(absInst) {
			if in.HedgingInstrumentGain.IsNegative() {
				effective = absItem.Neg()
			} else {
				effective = absItem
			}
		}
		ineffective := in.HedgingInstrumentGain.Sub(effective)
		return HedgeResult{
			RelationshipID:      in.RelationshipID,
			Type:                in.Type,
			EffectivePortionOCI: effective,
			IneffectivenessPL:   ineffective,
			Effective:           true,
		}, nil
	default:
		return HedgeResult{}, fmt.Errorf("ifrs9: unsupported hedge type %q", in.Type)
	}
}

type ModelGovernanceInput struct {
	ModelVersion            string
	SPPIAssessmentRef       string
	BusinessModelRef        string
	IndependentReviewerID   string
	ValidationReportRef     string
	ValidatedAt             time.Time
}

// ValidateModelGovernance enforces independent validation and host-responsibility
// boundaries (SPPI & business-model classification) before model activation.
func ValidateModelGovernance(in ModelGovernanceInput) error {
	if strings.TrimSpace(in.ModelVersion) == "" ||
		strings.TrimSpace(in.SPPIAssessmentRef) == "" ||
		strings.TrimSpace(in.BusinessModelRef) == "" ||
		strings.TrimSpace(in.IndependentReviewerID) == "" ||
		strings.TrimSpace(in.ValidationReportRef) == "" ||
		in.ValidatedAt.IsZero() {
		return errors.New("ifrs9: model version, SPPI ref, business model ref, independent reviewer, report ref, and validation date are required")
	}
	return nil
}

// SetProfileConfig configures the host's IFRS 9 edition, effective date, and jurisdiction.
func (m *Module) SetProfileConfig(cfg standards.StandardProfileConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profileConfig = cfg
}

