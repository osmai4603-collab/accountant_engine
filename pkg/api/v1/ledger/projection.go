package ledger

import kernelLedger "github.com/osmai4603-collab/accounting_engine/internal/kernel/ledger"

type PostedMovement = kernelLedger.PostedMovement
type AccountBalanceKey = kernelLedger.AccountBalanceKey
type AccountBalance = kernelLedger.AccountBalance
type ForeignCurrencyBalanceKey = kernelLedger.ForeignCurrencyBalanceKey
type ForeignCurrencyBalance = kernelLedger.ForeignCurrencyBalance
type ForeignCurrencyBalanceFilter = kernelLedger.ForeignCurrencyBalanceFilter

var (
	IsPostedAccountingFact         = kernelLedger.IsPostedAccountingFact
	ComparePostedEntries           = kernelLedger.ComparePostedEntries
	CompareEntryLines              = kernelLedger.CompareEntryLines
	ProjectPostedMovements         = kernelLedger.ProjectPostedMovements
	RebuildAccountBalances         = kernelLedger.RebuildAccountBalances
	RebuildForeignCurrencyBalances = kernelLedger.RebuildForeignCurrencyBalances
	SortedForeignCurrencyBalances  = kernelLedger.SortedForeignCurrencyBalances
)
