// Package masterdata defines the public storage contract for accounting master data.
package masterdata

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

var ErrCapabilityNotImplemented = types.ErrCapabilityNotImplemented

var (
	ErrInvalidCompany       = errors.New("invalid company")
	ErrDuplicateCompanyCode = errors.New("duplicate company code")
	ErrInvalidAccount       = errors.New("invalid account")
	ErrDuplicateAccountCode = errors.New("duplicate account code")
	ErrAccountNotFound      = types.ErrAccountNotFound
	ErrInvalidFiscalYear    = errors.New("invalid fiscal year")
	ErrFiscalYearNotFound   = errors.New("fiscal year not found")
	ErrInvalidFiscalPeriod  = errors.New("invalid fiscal period")
	ErrPeriodNotFound       = errors.New("fiscal period not found")
	ErrInvalidJournal       = errors.New("invalid journal")
	ErrCompanyNotFound      = errors.New("company not found")
	ErrJournalNotFound      = errors.New("journal not found")
	ErrDuplicateJournalCode = errors.New("journal code already exists")
	ErrDuplicateJournalID   = errors.New("journal id already exists")
)

type AccountType = types.AccountType

const (
	AccountTypeAsset     = types.AccountTypeAsset
	AccountTypeLiability = types.AccountTypeLiability
	AccountTypeEquity    = types.AccountTypeEquity
	AccountTypeIncome    = types.AccountTypeIncome
	AccountTypeExpense   = types.AccountTypeExpense
)

type NormalBalance = types.NormalBalance

const (
	NormalBalanceDebit  = types.NormalBalanceDebit
	NormalBalanceCredit = types.NormalBalanceCredit
)

type FiscalYearState = types.FiscalYearState
type PeriodState = types.PeriodState
type FiscalPeriodType = types.FiscalPeriodType

const (
	FiscalYearOpen                    = types.FiscalYearOpen
	FiscalYearSoftClosed              = types.FiscalYearSoftClosed
	FiscalYearHardClosed              = types.FiscalYearHardClosed
	PeriodOpen                        = types.PeriodOpen
	PeriodSoftClosed                  = types.PeriodSoftClosed
	PeriodHardClosed                  = types.PeriodHardClosed
	FiscalPeriodTypeNormal            = types.FiscalPeriodTypeNormal
	FiscalPeriodTypeYearEndAdjustment = types.FiscalPeriodTypeYearEndAdjustment
	FiscalPeriodTypeAuditAdjustment   = types.FiscalPeriodTypeAuditAdjustment
)

type Company struct {
	ID                   types.CompanyID
	Code                 string
	LegalName            string
	FunctionalCurrency   string
	PresentationCurrency string
	Active               bool
}

type JournalID = types.JournalID

func NewJournalID() JournalID { return types.NewJournalID() }

// Journal is the public, company-scoped master-data representation of a
// posting journal.
type Journal struct {
	ID                JournalID
	CompanyID         types.CompanyID
	Code              string
	Name              string
	AllowedCurrencies []string
	AllowedAccountIDs []types.AccountID
}

func (j Journal) Validate() error {
	if j.ID == (JournalID{}) || j.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(j.Code) == "" || strings.TrimSpace(j.Name) == "" ||
		utf8.RuneCountInString(j.Code) > 32 || utf8.RuneCountInString(j.Name) > 100 {
		return ErrInvalidJournal
	}
	return nil
}

type Account = types.Account

type FiscalYear = types.FiscalYear
type FiscalPeriod = types.FiscalPeriod

// Store exposes company-scoped reads and typed creation of the minimum master
// data required to prepare an accounting company for use.
type Store interface {
	CreateCompany(context.Context, Company) error
	GetCompany(context.Context, types.CompanyID) (Company, error)
	CreateJournal(context.Context, Journal) error
	GetJournalByCode(context.Context, types.CompanyID, string) (Journal, error)
	CreateAccount(context.Context, Account) error
	GetAccount(context.Context, types.CompanyID, types.AccountID) (Account, error)
	GetAccountByCode(context.Context, types.CompanyID, string) (Account, error)
	CreateFiscalYear(context.Context, FiscalYear) error
	GetFiscalYear(context.Context, types.CompanyID, types.FiscalYearID) (FiscalYear, error)
	CreateFiscalPeriod(context.Context, FiscalPeriod) error
	GetFiscalPeriod(context.Context, types.CompanyID, types.FiscalPeriodID) (FiscalPeriod, error)
}

// JournalRepository exposes journal lookup by unique ID so posting can verify
// effective windows, activity, and account/currency rules against the catalog.
type JournalRepository interface {
	GetJournalByID(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (Journal, error)
}
