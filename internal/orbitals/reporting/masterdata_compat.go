package reporting

import (
	"errors"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
)

// The kernel masterdata package intentionally exposes a smaller vocabulary
// than the standalone engine this module was ported from. These aliases and
// helpers restore the source semantics locally without changing the kernel.

const (
	subClassAccountsReceivable = "accounts_receivable"
	subClassAccountsPayable    = "accounts_payable"
	subClassBank               = "bank"

	ifrsCategoryOperating              = "operating"
	ifrsCategoryInvesting              = "investing"
	ifrsCategoryFinancing              = "financing"
	ifrsCategoryTax                    = "tax"
	ifrsCategoryIncomeTaxes            = "income_taxes"
	ifrsCategoryDiscontinued           = "discontinued"
	ifrsCategoryDiscontinuedOperations = "discontinued_operations"
	ifrsCategoryEquityMovements        = "equity_movements"
	ifrsCategoryBalanceSheetPerpetual  = "balance_sheet_perpetual"

	cashFlowOperating         = "operating"
	cashFlowInvesting         = "investing"
	cashFlowFinancing         = "financing"
	cashFlowCashEquivalent    = "cash_equivalents"
	cashFlowNonCashAdjustment = "non_cash"
)

var errFiscalYearNotFound = errors.New("fiscal year not found")

func isCurrentAsset(account masterdata.Account) bool {
	if account.Type != masterdata.AccountTypeAsset {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.SubClassification)) {
	case masterdata.SubClassCashAndEquivalents, masterdata.SubClassTradeReceivables,
		masterdata.SubClassInventories, masterdata.SubClassPrepaymentsOtherDebit,
		masterdata.SubClassCurrentAssets, subClassAccountsReceivable, subClassBank, "cash":
		return true
	default:
		return false
	}
}

func isCurrentLiability(account masterdata.Account) bool {
	if account.Type != masterdata.AccountTypeLiability {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.SubClassification)) {
	case masterdata.SubClassShortTermBorrowings, masterdata.SubClassCurrentLeaseLiabilities,
		masterdata.SubClassTradePayables, masterdata.SubClassAccrualsOtherCredit,
		masterdata.SubClassCurrentTaxLiabilities, masterdata.SubClassCurrentLiabilities,
		subClassAccountsPayable, "payable":
		return true
	default:
		return false
	}
}

func isCurrentClassification(account masterdata.Account) bool {
	switch account.Type {
	case masterdata.AccountTypeAsset:
		return isCurrentAsset(account)
	case masterdata.AccountTypeLiability:
		return isCurrentLiability(account)
	default:
		return false
	}
}

func effectiveIsCashEquivalent(account masterdata.Account) bool {
	classification := strings.ToLower(strings.TrimSpace(account.SubClassification))
	if account.IsCashEquivalent {
		return true
	}
	return account.Type == masterdata.AccountTypeAsset &&
		(classification == masterdata.SubClassCashAndEquivalents || classification == subClassBank || classification == "cash")
}

func isPPE(account masterdata.Account) bool {
	if account.Type != masterdata.AccountTypeAsset {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.SubClassification)) {
	case masterdata.SubClassPropertyPlantEquipment, "fixed_asset", "fixed_assets", "ppe":
		return true
	default:
		return false
	}
}
