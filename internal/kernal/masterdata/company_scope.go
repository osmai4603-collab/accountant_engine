package masterdata

import (
	"context"
	"errors"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrCompanyInactive = errors.New("company is inactive")
)

// ValidateCompanyScope checks that the company exists and is active.
func ValidateCompanyScope(ctx context.Context, reader Reader, companyID types.CompanyID) (Company, error) {
	company, err := reader.GetCompany(ctx, companyID)
	if err != nil {
		return Company{}, err
	}
	if !company.Active {
		return company, ErrCompanyInactive
	}
	return company, nil
}
