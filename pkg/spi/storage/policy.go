package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

// PolicyKind is the adapter-neutral identity of a versioned policy family.
type PolicyKind string

const (
	PolicyKindPosting  PolicyKind = "posting"
	PolicyKindCurrency PolicyKind = "currency"
)

// PolicyVersion is the immutable, durable record of one policy version. The
// payload is a canonical JSON document using only public fields; the kernel
// policy types are private to internal/kernal/policy and never appear in SPI.
type PolicyVersion struct {
	CompanyID     types.CompanyID
	PolicyID      uuid.UUID
	Kind          PolicyKind
	Version       int
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	CreatedBy     string
	ApprovedBy    string
	CreatedAt     time.Time
	Payload       []byte
}

func (v PolicyVersion) Validate() error {
	if v.CompanyID == (types.CompanyID{}) || v.PolicyID == uuid.Nil {
		return errors.New("storage: policy company and policy id are required")
	}
	if v.Kind != PolicyKindPosting && v.Kind != PolicyKindCurrency {
		return fmt.Errorf("storage: unsupported policy kind %q", v.Kind)
	}
	if v.Version <= 0 || v.EffectiveFrom.IsZero() {
		return errors.New("storage: policy version and effective date are required")
	}
	if v.EffectiveTo != nil && !v.EffectiveTo.After(v.EffectiveFrom) {
		return errors.New("storage: policy effective end must follow its start")
	}
	if len(v.Payload) == 0 || !json.Valid(v.Payload) {
		return errors.New("storage: policy payload must be valid JSON")
	}
	return nil
}

// PostingPolicyPayload carries the public, version-stable posting policy
// factors that participate in the kernel policy fingerprint.
type PostingPolicyPayload struct {
	MaxBackdatingDays                          int  `json:"max_backdating_days"`
	AllowBackdatingException                   bool `json:"allow_backdating_exception"`
	AllowSoftClosedPosting                     bool `json:"allow_soft_closed_posting"`
	RequireCostCenterForIncomeExpense          bool `json:"require_cost_center_for_income_expense"`
	RequireAnalyticalDimensionForIncomeExpense bool `json:"require_analytical_dimension_for_income_expense"`
}

// CurrencyPolicyPayload carries the public, version-stable currency policy
// factors that participate in the kernel policy fingerprint.
type CurrencyPolicyPayload struct {
	FunctionalCurrency            string   `json:"functional_currency"`
	PresentationCurrency          string   `json:"presentation_currency"`
	RoundingMethod                string   `json:"rounding_method"`
	AllowedCurrencies             []string `json:"allowed_currencies"`
	RoundingDifferenceAccountID   string   `json:"rounding_difference_account_id,omitempty"`
	AutoSettleRoundingDifferences bool     `json:"auto_settle_rounding_differences"`
}

// PolicyStore is the append-only, company-scoped policy history consumed by
// the SDK. Versions are immutable; a changed rule is a new version with a new
// effective window, so already-posted fingerprints are never reinterpreted.
type PolicyStore interface {
	AppendVersion(ctx context.Context, version PolicyVersion) error
	ListVersions(ctx context.Context, companyID types.CompanyID, kind PolicyKind) ([]PolicyVersion, error)
}

// PolicyStoreExtensionKey exposes an optional policy store through
// Ports.Private without widening the stable storage.Ports contract.
const PolicyStoreExtensionKey = "accounting_engine.policy_store"

var (
	ErrPolicyVersionConflict = errors.New("storage: conflicting policy version")
	ErrPolicyVersionNotFound = errors.New("storage: policy version not found")
)
