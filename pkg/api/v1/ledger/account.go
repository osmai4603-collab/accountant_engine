package ledger

import kernelLedger "github.com/osmai4603-collab/accounting_engine/internal/kernel/ledger"

type AccountType = kernelLedger.AccountType
type NormalBalance = kernelLedger.NormalBalance
type Account = kernelLedger.Account

const (
	AccountTypeAsset     = kernelLedger.AccountTypeAsset
	AccountTypeLiability = kernelLedger.AccountTypeLiability
	AccountTypeEquity    = kernelLedger.AccountTypeEquity
	AccountTypeIncome    = kernelLedger.AccountTypeIncome
	AccountTypeExpense   = kernelLedger.AccountTypeExpense
	AccountTypeRevenue   = kernelLedger.AccountTypeRevenue
	NormalBalanceDebit   = kernelLedger.NormalBalanceDebit
	NormalBalanceCredit  = kernelLedger.NormalBalanceCredit
)
