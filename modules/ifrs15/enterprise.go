package ifrs15

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/shopspring/decimal"
)

// ControlTransferInput defines the host-supplied evidence for control transfer
// and progress measurement (IFRS 15.31-45).
type ControlTransferInput struct {
	ContractID         uuid.UUID
	ObligationID       uuid.UUID
	SatisfactionDate   time.Time
	ProgressPercentage decimal.Decimal // 0..100
	HostEvidenceRef    string
	MeasurementMethod  string // e.g., "output_milestone", "cost_to_cost"
}

func (c ControlTransferInput) Validate() error {
	if c.ContractID == uuid.Nil || c.ObligationID == uuid.Nil || c.SatisfactionDate.IsZero() {
		return errors.New("ifrs15: contract id, obligation id, and satisfaction date are required")
	}
	if strings.TrimSpace(c.HostEvidenceRef) == "" || strings.TrimSpace(c.MeasurementMethod) == "" {
		return errors.New("ifrs15: host evidence reference and measurement method are required")
	}
	if c.ProgressPercentage.IsNegative() || c.ProgressPercentage.GreaterThan(decimal.NewFromInt(100)) {
		return errors.New("ifrs15: progress percentage must be between 0 and 100")
	}
	return nil
}

type PrincipalAgentRole string

const (
	RolePrincipal PrincipalAgentRole = "principal"
	RoleAgent     PrincipalAgentRole = "agent"
)

type PrincipalAgentInput struct {
	ContractID          uuid.UUID
	ControlsBeforeTrans bool
	PrimaryFulfillment  bool
	InventoryRisk       bool
	PricingDiscretion   bool
	GrossConsideration  decimal.Decimal
	SupplierCost        decimal.Decimal
	Currency            string
	AssessmentRef       string
}

type PrincipalAgentResult struct {
	Role              PrincipalAgentRole
	RecognizedRevenue decimal.Decimal
	CommissionFee     decimal.Decimal
	Currency          string
}

// EvaluatePrincipalVsAgent applies IFRS 15.B34-B38 indicators to determine gross
// (principal) vs net (agent) revenue recognition.
func EvaluatePrincipalVsAgent(in PrincipalAgentInput) (PrincipalAgentResult, error) {
	if in.ContractID == uuid.Nil || strings.TrimSpace(in.Currency) == "" || strings.TrimSpace(in.AssessmentRef) == "" {
		return PrincipalAgentResult{}, errors.New("ifrs15: contract id, currency, and assessment ref are required")
	}
	if in.GrossConsideration.IsNegative() || in.SupplierCost.IsNegative() {
		return PrincipalAgentResult{}, errors.New("ifrs15: gross consideration and supplier cost must be non-negative")
	}
	netFee := in.GrossConsideration.Sub(in.SupplierCost)
	isPrincipal := in.ControlsBeforeTrans || (in.PrimaryFulfillment && (in.InventoryRisk || in.PricingDiscretion))
	if isPrincipal {
		return PrincipalAgentResult{
			Role:              RolePrincipal,
			RecognizedRevenue: in.GrossConsideration,
			CommissionFee:     netFee,
			Currency:          in.Currency,
		}, nil
	}
	return PrincipalAgentResult{
		Role:              RoleAgent,
		RecognizedRevenue: netFee,
		CommissionFee:     netFee,
		Currency:          in.Currency,
	}, nil
}

type IPLicenseType string

const (
	IPLicenseRightToAccess IPLicenseType = "right_to_access" // over time (IFRS 15.B58)
	IPLicenseRightToUse    IPLicenseType = "right_to_use"    // point in time (IFRS 15.B61)
)

type IPLicenseInput struct {
	ContractID     uuid.UUID
	LicenseType    IPLicenseType
	FixedFee       decimal.Decimal
	SalesUsageBase decimal.Decimal
	RoyaltyRate    decimal.Decimal // e.g. 0.05 for 5%
	SalesOccurred  bool
	Currency       string
	PeriodStart    time.Time
	PeriodEnd      time.Time
}

type IPLicenseResult struct {
	LicenseType        IPLicenseType
	FixedRevenue       decimal.Decimal
	RoyaltyRevenue     decimal.Decimal
	TotalRevenue       decimal.Decimal
	RecognitionPattern string
	Currency           string
}

// EvaluateIPLicense applies IFRS 15.B52-B63 for symbolic/functional IP licenses
// and the sales- or usage-based royalty constraint (IFRS 15.B63).
func EvaluateIPLicense(in IPLicenseInput) (IPLicenseResult, error) {
	if in.ContractID == uuid.Nil || strings.TrimSpace(in.Currency) == "" {
		return IPLicenseResult{}, errors.New("ifrs15: contract id and currency are required")
	}
	if in.FixedFee.IsNegative() || in.SalesUsageBase.IsNegative() || in.RoyaltyRate.IsNegative() {
		return IPLicenseResult{}, errors.New("ifrs15: fee, sales base, and royalty rate must be non-negative")
	}
	pattern := "point_in_time"
	switch in.LicenseType {
	case IPLicenseRightToAccess:
		pattern = "over_time"
	case IPLicenseRightToUse:
		pattern = "point_in_time"
	default:
		return IPLicenseResult{}, fmt.Errorf("ifrs15: unsupported IP license type %q", in.LicenseType)
	}
	royalty := decimal.Zero
	if in.SalesOccurred {
		royalty = in.SalesUsageBase.Mul(in.RoyaltyRate).Round(2)
	}
	return IPLicenseResult{
		LicenseType:        in.LicenseType,
		FixedRevenue:       in.FixedFee,
		RoyaltyRevenue:     royalty,
		TotalRevenue:       in.FixedFee.Add(royalty),
		RecognitionPattern: pattern,
		Currency:           in.Currency,
	}, nil
}

// WithStandardProfile configures the host's IFRS 15 edition, effective date, and jurisdiction.
func WithStandardProfile(cfg standards.StandardProfileConfig) Option {
	return func(m *Module) {
		m.profileConfig = cfg
	}
}
