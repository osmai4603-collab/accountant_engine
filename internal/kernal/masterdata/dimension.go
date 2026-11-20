package masterdata

import (
	"errors"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var ErrInvalidAnalyticalDimension = errors.New("invalid or inactive analytical dimension")

type AnalyticalDimensionKind string

const (
	DimensionProject      AnalyticalDimensionKind = "project"
	DimensionSegment      AnalyticalDimensionKind = "segment"
	DimensionBusinessUnit AnalyticalDimensionKind = "business_unit"
)

type AnalyticalDimensionValue struct {
	ID        types.ProjectID
	CompanyID types.CompanyID
	Kind      AnalyticalDimensionKind
	Code      string
	Name      string
	Active    bool
}

func (v AnalyticalDimensionValue) Validate() error {
	if v.ID == (types.ProjectID{}) || v.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(v.Code) == "" || strings.TrimSpace(v.Name) == "" ||
		(v.Kind != DimensionProject && v.Kind != DimensionSegment && v.Kind != DimensionBusinessUnit) {
		return ErrInvalidAnalyticalDimension
	}
	return nil
}
