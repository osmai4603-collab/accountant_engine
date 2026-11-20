package masterdata

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

type Reader interface {
	GetCompany(ctx context.Context, companyID types.CompanyID) (Company, error)
	GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (Account, error)
	GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (Account, error)
	GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (Journal, error)
	HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error)
	GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (FiscalYear, error)
	GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (FiscalPeriod, error)
}
