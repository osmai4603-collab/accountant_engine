package ledger

import kernalLedger "github.com/osmai4603-collab/accounting_engine/internal/kernal/ledger"

type PostedMovement = kernalLedger.PostedMovement
type AccountBalanceKey = kernalLedger.AccountBalanceKey
type AccountBalance = kernalLedger.AccountBalance
type ForeignCurrencyBalanceKey = kernalLedger.ForeignCurrencyBalanceKey
type ForeignCurrencyBalance = kernalLedger.ForeignCurrencyBalance
type ForeignCurrencyBalanceFilter = kernalLedger.ForeignCurrencyBalanceFilter

var (
	IsPostedAccountingFact         = kernalLedger.IsPostedAccountingFact
	ComparePostedEntries           = kernalLedger.ComparePostedEntries
	CompareEntryLines              = kernalLedger.CompareEntryLines
	ProjectPostedMovements         = kernalLedger.ProjectPostedMovements
	RebuildAccountBalances         = kernalLedger.RebuildAccountBalances
	RebuildForeignCurrencyBalances = kernalLedger.RebuildForeignCurrencyBalances
	SortedForeignCurrencyBalances  = kernalLedger.SortedForeignCurrencyBalances
)
