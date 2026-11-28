package masterdata

import (
	"context"
	"errors"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

var (
	ErrCompanyScopeRequired    = errors.New("company scope is required")
	ErrCompanyInactive         = errors.New("company is inactive")
	ErrCompanyIdentityMismatch = errors.New("company repository returned a different company")
)

// ValidateCompanyScope checks that the company exists and is active. A missing
// scope is rejected before any repository access so an adapter cannot supply a
// company-owned record for an unscoped (zero) request (RULE-KERNEL-016).
func ValidateCompanyScope(ctx context.Context, reader Reader, companyID types.CompanyID) (Company, error) {
	if companyID == (types.CompanyID{}) {
		return Company{}, ErrCompanyScopeRequired
	}
	company, err := reader.GetCompany(ctx, companyID)
	if err != nil {
		return Company{}, err
	}
	if companyID == (types.CompanyID{}) || company.ID != companyID {
		return Company{}, ErrCompanyIdentityMismatch
	}
	if !company.Active {
		return company, ErrCompanyInactive
	}
	return company, nil
}
