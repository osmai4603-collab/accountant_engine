package taxonomy

import (
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
)

// The kernel masterdata package models IFRS categories as free-form strings
// and consolidates the OCI subclasses. These constants preserve the source
// taxonomy vocabulary without modifying the locked kernel.
const (
	subClassOCINonRecyclable = "oci_non_recyclable"
	subClassOCIRecyclable    = "oci_recyclable"

	ifrsCategoryOperating              = "operating"
	ifrsCategoryInvesting              = "investing"
	ifrsCategoryFinancing              = "financing"
	ifrsCategoryTax                    = "tax"
	ifrsCategoryIncomeTaxes            = "income_taxes"
	ifrsCategoryDiscontinued           = "discontinued"
	ifrsCategoryDiscontinuedOperations = "discontinued_operations"
	ifrsCategoryEquityMovements        = "equity_movements"
)

func taxonomyIsCurrentAsset(account masterdata.Account) bool {
	if account.Type != masterdata.AccountTypeAsset {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.SubClassification)) {
	case masterdata.SubClassCashAndEquivalents, masterdata.SubClassTradeReceivables,
		masterdata.SubClassInventories, masterdata.SubClassPrepaymentsOtherDebit,
		masterdata.SubClassCurrentAssets, "bank", "cash", "accounts_receivable":
		return true
	default:
		return false
	}
}

func taxonomyIsCurrentLiability(account masterdata.Account) bool {
	if account.Type != masterdata.AccountTypeLiability {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.SubClassification)) {
	case masterdata.SubClassShortTermBorrowings, masterdata.SubClassCurrentLeaseLiabilities,
		masterdata.SubClassTradePayables, masterdata.SubClassAccrualsOtherCredit,
		masterdata.SubClassCurrentTaxLiabilities, masterdata.SubClassCurrentLiabilities,
		"accounts_payable", "payable":
		return true
	default:
		return false
	}
}

func taxonomyIsCurrent(account masterdata.Account) bool {
	switch account.Type {
	case masterdata.AccountTypeAsset:
		return taxonomyIsCurrentAsset(account)
	case masterdata.AccountTypeLiability:
		return taxonomyIsCurrentLiability(account)
	default:
		asOf := time.Now().UTC()
		return account.IsCurrent(asOf)
	}
}
