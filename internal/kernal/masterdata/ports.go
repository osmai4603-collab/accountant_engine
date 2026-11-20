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

// AdjustmentPeriodReader resolves a period 13 or 14 within a fiscal year.
// Adapters should implement this alongside Reader to enable adjustment posting.
type AdjustmentPeriodReader interface {
	GetPeriodByNumber(
		ctx context.Context,
		companyID types.CompanyID,
		fiscalYearID types.FiscalYearID,
		number int,
	) (FiscalPeriod, error)
}

// JournalRepository resolves a journal by stable ID for posting-time policy checks.
type JournalRepository interface {
	GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (Journal, error)
}
