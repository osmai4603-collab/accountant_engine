package masterdata

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
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

// EffectiveAccountReader supplies the account version that was effective at a
// posting date. A reader that does not implement it falls back to GetAccount;
// the kernel then fails closed if the returned record is not current on the
// posting date. Adapters must keep effective-dated versions so a later master
// data edit cannot silently reinterpret an already posted entry.
type EffectiveAccountReader interface {
	GetAccountAsOf(
		ctx context.Context,
		companyID types.CompanyID,
		accountID types.AccountID,
		asOf time.Time,
	) (Account, error)
}

// Clock abstracts wall time for domain helpers that need a "current" date.
// Posting pipelines must pass an injected clock; callers that need an explicit
// date should use the *AsOf variants instead.
type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

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
