package ledger

import kernalLedger "github.com/osmai4603-collab/accounting_engine/internal/kernal/ledger"

type AccountType = kernalLedger.AccountType
type NormalBalance = kernalLedger.NormalBalance
type Account = kernalLedger.Account

const (
	AccountTypeAsset     = kernalLedger.AccountTypeAsset
	AccountTypeLiability = kernalLedger.AccountTypeLiability
	AccountTypeEquity    = kernalLedger.AccountTypeEquity
	AccountTypeIncome    = kernalLedger.AccountTypeIncome
	AccountTypeExpense   = kernalLedger.AccountTypeExpense
	AccountTypeRevenue   = kernalLedger.AccountTypeRevenue
	NormalBalanceDebit   = kernalLedger.NormalBalanceDebit
	NormalBalanceCredit  = kernalLedger.NormalBalanceCredit
)
