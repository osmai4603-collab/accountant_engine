package masterdata

import (
	"context"
	"errors"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrInvalidCostCenter = errors.New("invalid cost center")
	ErrCostCenterCycle   = errors.New("cost center hierarchy contains a cycle")
)

type CostCenter struct {
	ID        types.CostCenterID
	CompanyID types.CompanyID
	ParentID  *types.CostCenterID
	Code      string
	Name      string
	Active    bool
}

func (c CostCenter) Validate() error {
	if c.ID == (types.CostCenterID{}) || c.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(c.Code) == "" || strings.TrimSpace(c.Name) == "" {
		return ErrInvalidCostCenter
	}
	if c.ParentID != nil && *c.ParentID == c.ID {
		return ErrCostCenterCycle
	}
	return nil
}

type CostCenterRepository interface {
	GetCostCenterForCompany(context.Context, types.CompanyID, types.CostCenterID) (CostCenter, error)
}
