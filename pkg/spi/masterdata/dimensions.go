package masterdata

import (
	"context"
	"errors"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

var (
	ErrInvalidCostCenter            = errors.New("invalid cost center")
	ErrDuplicateCostCenter          = errors.New("duplicate cost center code")
	ErrInvalidAnalyticalDimension   = errors.New("invalid or inactive analytical dimension")
	ErrDuplicateAnalyticalDimension = errors.New("duplicate analytical dimension code")
)

// CostCenter and AnalyticalDimensionValue are public DTOs re-exported through
// pkg/api/v1/types. Their kind constants are likewise public so that hosts and
// adapters can exchange analytical data without touching internal/kernel.
type CostCenter = types.CostCenter
type AnalyticalDimensionValue = types.AnalyticalDimensionValue
type AnalyticalDimensionKind = types.AnalyticalDimensionKind

const (
	DimensionProject      = types.DimensionProject
	DimensionSegment      = types.DimensionSegment
	DimensionBusinessUnit = types.DimensionBusinessUnit
)

// CostCenterReader resolves a cost center strictly inside the requested
// company. Implementations must reject foreign IDs and cross-company scopes.
type CostCenterReader interface {
	GetCostCenterForCompany(ctx context.Context, companyID types.CompanyID, costCenterID types.CostCenterID) (CostCenter, error)
}

// AnalyticalDimensionReader resolves a dimensional value strictly inside the
// requested company. The id is the stable string form of the public DTO ID.
type AnalyticalDimensionReader interface {
	GetAnalyticalDimensionForCompany(
		ctx context.Context,
		companyID types.CompanyID,
		kind AnalyticalDimensionKind,
		id string,
	) (AnalyticalDimensionValue, error)
}

// CostCenterWriter and AnalyticalDimensionWriter are setup-time capabilities
// used to provision master data through the same public DTOs as the readers.
type CostCenterWriter interface {
	CreateCostCenter(ctx context.Context, costCenter CostCenter) error
}

type AnalyticalDimensionWriter interface {
	CreateAnalyticalDimensionValue(ctx context.Context, value AnalyticalDimensionValue) error
}
