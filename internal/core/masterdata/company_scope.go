package masterdata

import (
	"context"
	"errors"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var (
	ErrCompanyInactive = errors.New("company is inactive")
	ErrCompanyNotFound = masterdata.ErrCompanyNotFound
)

// ValidateCompanyScope checks that the company exists and is active.
func ValidateCompanyScope(ctx context.Context, reader storage.MasterDataReader, companyID types.CompanyID) (masterdata.Company, error) {
	company, err := reader.GetCompany(ctx, companyID)
	if err != nil {
		return masterdata.Company{}, err
	}
	if !company.Active {
		return company, ErrCompanyInactive
	}
	return company, nil
}

