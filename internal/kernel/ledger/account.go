package ledger

import "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"

type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeEquity    AccountType = "equity"
	AccountTypeIncome    AccountType = "income"
	AccountTypeExpense   AccountType = "expense"
	// AccountTypeRevenue is a documented alias for AccountTypeIncome ("income") retained for compatibility.
	AccountTypeRevenue AccountType = "income"
)

type NormalBalance string

const (
	NormalBalanceDebit  NormalBalance = "debit"
	NormalBalanceCredit NormalBalance = "credit"
)

type Account struct {
	ID                types.AccountID
	CompanyID         types.CompanyID
	Code              string
	Name              string
	Type              AccountType
	SubClassification string
	NormalBalance     NormalBalance
	ParentID          *types.AccountID
	IsReconcilable    bool
	IsActive          bool
	Currency          string
}
