package masterdata

import (
	"context"
	"errors"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
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

type AnalyticalDimensionRepository interface {
	GetAnalyticalDimensionForCompany(
		context.Context,
		types.CompanyID,
		AnalyticalDimensionKind,
		string,
	) (AnalyticalDimensionValue, error)
}

func (v AnalyticalDimensionValue) Validate() error {
	if v.ID == (types.ProjectID{}) || v.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(v.Code) == "" || strings.TrimSpace(v.Name) == "" ||
		(v.Kind != DimensionProject && v.Kind != DimensionSegment && v.Kind != DimensionBusinessUnit) {
		return ErrInvalidAnalyticalDimension
	}
	return nil
}

// DimensionID returns the canonical UUID string of the dimension regardless of Kind.
func (v AnalyticalDimensionValue) DimensionID() string {
	return v.ID.String()
}

// AsProjectID returns the typed ProjectID when Kind == DimensionProject.
func (v AnalyticalDimensionValue) AsProjectID() (types.ProjectID, bool) {
	if v.Kind != DimensionProject {
		return types.ProjectID{}, false
	}
	return v.ID, true
}

// AsSegmentID returns the typed SegmentID when Kind == DimensionSegment.
func (v AnalyticalDimensionValue) AsSegmentID() (types.SegmentID, bool) {
	if v.Kind != DimensionSegment {
		return types.SegmentID{}, false
	}
	return types.SegmentID(v.ID), true
}

// AsBusinessUnitID returns the typed BusinessUnitID when Kind == DimensionBusinessUnit.
func (v AnalyticalDimensionValue) AsBusinessUnitID() (types.BusinessUnitID, bool) {
	if v.Kind != DimensionBusinessUnit {
		return types.BusinessUnitID{}, false
	}
	return types.BusinessUnitID(v.ID), true
}

// NewProjectDimensionValue constructs a typed project dimension value.
func NewProjectDimensionValue(id types.ProjectID, companyID types.CompanyID, code, name string, active bool) AnalyticalDimensionValue {
	return AnalyticalDimensionValue{ID: id, CompanyID: companyID, Kind: DimensionProject, Code: code, Name: name, Active: active}
}

// NewSegmentDimensionValue constructs a typed segment dimension value.
func NewSegmentDimensionValue(id types.SegmentID, companyID types.CompanyID, code, name string, active bool) AnalyticalDimensionValue {
	return AnalyticalDimensionValue{ID: types.ProjectID(id), CompanyID: companyID, Kind: DimensionSegment, Code: code, Name: name, Active: active}
}

// NewBusinessUnitDimensionValue constructs a typed business-unit dimension value.
func NewBusinessUnitDimensionValue(id types.BusinessUnitID, companyID types.CompanyID, code, name string, active bool) AnalyticalDimensionValue {
	return AnalyticalDimensionValue{ID: types.ProjectID(id), CompanyID: companyID, Kind: DimensionBusinessUnit, Code: code, Name: name, Active: active}
}
