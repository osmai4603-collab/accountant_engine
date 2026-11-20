package masterdata

import (
	"errors"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrCompanyNotFound       = errors.New("company not found")
	ErrJournalNotFound       = errors.New("journal not found")
	ErrInvalidJournal        = errors.New("invalid journal")
	ErrJournalNotCurrent     = errors.New("journal is not valid on the posting date")
	ErrJournalReaderRequired = errors.New("journal repository is required for posting")
)

type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeEquity    AccountType = "equity"
	AccountTypeIncome    AccountType = "income"
	AccountTypeExpense   AccountType = "expense"
)

type NormalBalance string

const (
	NormalBalanceDebit  NormalBalance = "debit"
	NormalBalanceCredit NormalBalance = "credit"
)

type PeriodState string

const (
	PeriodOpen       PeriodState = "open"
	PeriodSoftClosed PeriodState = "soft_closed"
	PeriodHardClosed PeriodState = "hard_closed"
)

type FiscalYearState string

const (
	FiscalYearOpen       FiscalYearState = "open"
	FiscalYearSoftClosed FiscalYearState = "soft_closed"
	FiscalYearHardClosed FiscalYearState = "hard_closed"
)

type FiscalPeriodType string

const (
	FiscalPeriodTypeNormal            FiscalPeriodType = "normal"
	FiscalPeriodTypeYearEndAdjustment FiscalPeriodType = "year_end_adjustment"
	FiscalPeriodTypeAuditAdjustment   FiscalPeriodType = "audit_adjustment"
)

type Company struct {
	ID                   types.CompanyID
	Code                 string
	LegalName            string
	FunctionalCurrency   string
	PresentationCurrency string
	Active               bool
}

type Journal struct {
	ID                types.JournalID
	CompanyID         types.CompanyID
	Code              string
	Name              string
	AllowedCurrencies []string
	AllowedAccountIDs []types.AccountID
	EffectiveFrom     time.Time
	EffectiveTo       *time.Time
}

type Account struct {
	ID                  types.AccountID
	CompanyID           types.CompanyID
	Code                string
	Name                string
	Type                AccountType
	NormalBalance       NormalBalance
	Currency            string
	ParentID            *types.AccountID
	IsReconcilable      bool
	IsControlAccount    bool
	IsContraAccount     bool
	IsActive            bool
	RequiresCostCenter  bool
	IsCashEquivalent    bool
	IsNonCashAdjustment bool
	CashFlowActivity    string
	SubClassification   string
	IFRSCategory        string
	DisplayOrder        int
	EffectiveFrom       time.Time
	EffectiveTo         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type FiscalYear struct {
	ID        types.FiscalYearID
	CompanyID types.CompanyID
	Name      string
	StartDate time.Time
	EndDate   time.Time
	State     FiscalYearState
}

type FiscalPeriod struct {
	ID           types.FiscalPeriodID
	CompanyID    types.CompanyID
	FiscalYearID types.FiscalYearID
	Number       int
	Name         string
	StartDate    time.Time
	EndDate      time.Time
	State        PeriodState
	Type         FiscalPeriodType
}
