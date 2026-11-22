package masterdata

import (
	"errors"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrAccountCodeRequired             = errors.New("account code is required")
	ErrAccountNameRequired             = errors.New("account name is required")
	ErrInvalidAccountType              = errors.New("invalid account type")
	ErrInvalidNormalBalance            = errors.New("invalid account normal balance")
	ErrInvalidSubClassificationForType = errors.New("invalid sub-classification for account type")
	ErrInvalidControlAccount           = errors.New("control accounts must be reconcilable asset or liability accounts")
)

const (
	SubClassCashAndEquivalents         = "cash_and_equivalents"
	SubClassTradeReceivables           = "trade_receivables"
	SubClassInventories                = "inventories"
	SubClassPropertyPlantEquipment     = "property_plant_equipment"
	SubClassIntangibleAssets           = "intangible_assets"
	SubClassRightOfUseAssets           = "right_of_use_assets"
	SubClassFinancialInvestments       = "financial_investments"
	SubClassPrepaymentsOtherDebit      = "prepayments_and_other_debit"
	SubClassOtherNonCurrentAssets      = "other_non_current_assets"
	SubClassCurrentAssets              = "current_assets"
	SubClassNonCurrentAssets           = "non_current_assets"
	SubClassTradePayables              = "trade_payables"
	SubClassShortTermBorrowings        = "short_term_borrowings"
	SubClassCurrentLeaseLiabilities    = "current_lease_liabilities"
	SubClassAccrualsOtherCredit        = "accruals_and_other_credit"
	SubClassCurrentTaxLiabilities      = "current_tax_liabilities"
	SubClassLongTermBorrowings         = "long_term_borrowings"
	SubClassNonCurrentLeaseLiabilities = "non_current_lease_liabilities"
	SubClassEndOfServiceProvisions     = "end_of_service_provisions"
	SubClassOtherNonCurrentLiabilities = "other_non_current_liabilities"
	SubClassCurrentLiabilities         = "current_liabilities"
	SubClassNonCurrentLiabilities      = "non_current_liabilities"
	SubClassShareCapital               = "share_capital"
	SubClassSharePremium               = "share_premium"
	SubClassStatutoryReserve           = "statutory_reserve"
	SubClassOCIReserves                = "oci_reserves"
	SubClassRetainedEarnings           = "retained_earnings"
	SubClassTreasuryShares             = "treasury_shares"
	SubClassDistributions              = "dividends_distributions"
	SubClassOperatingRevenue           = "operating_revenue"
	SubClassCostOfSales                = "cost_of_sales"
	SubClassSellingDistributionExpense = "selling_distribution_expense"
	SubClassGeneralAdminExpense        = "general_admin_expense"
	SubClassDepreciationAmortExpense   = "depreciation_amortization_expense"
	SubClassExpectedCreditLossExpense  = "expected_credit_loss_expense"
	SubClassOtherOperatingIncome       = "other_operating_income"
	SubClassOtherOperatingExpense      = "other_operating_expense"
	SubClassFinanceIncome              = "finance_income"
	SubClassFinanceCost                = "finance_cost"
	SubClassTaxExpense                 = "tax_expense"
	SubClassIncomeTaxExpense           = "income_tax_expense"
)

func DefaultNormalBalance(accountType AccountType) NormalBalance {
	switch accountType {
	case AccountTypeAsset, AccountTypeExpense:
		return NormalBalanceDebit
	case AccountTypeLiability, AccountTypeEquity, AccountTypeIncome:
		return NormalBalanceCredit
	default:
		return ""
	}
}

func (a *Account) Validate() error {
	if a == nil {
		return ErrInvalidAccountType
	}
	if strings.TrimSpace(a.Code) == "" {
		return ErrAccountCodeRequired
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrAccountNameRequired
	}
	if DefaultNormalBalance(a.Type) == "" {
		return ErrInvalidAccountType
	}
	if a.NormalBalance == "" {
		a.NormalBalance = DefaultNormalBalance(a.Type)
	}
	if a.NormalBalance != NormalBalanceDebit && a.NormalBalance != NormalBalanceCredit {
		return ErrInvalidNormalBalance
	}
	if !a.IsContraAccount && a.NormalBalance != DefaultNormalBalance(a.Type) {
		return ErrInvalidAccountType
	}
	if a.IsControlAccount && (!a.IsReconcilable || (a.Type != AccountTypeAsset && a.Type != AccountTypeLiability)) {
		return ErrInvalidControlAccount
	}
	if err := validateEffectiveRange(a.EffectiveFrom, a.EffectiveTo); err != nil {
		return err
	}
	if a.SubClassification != "" && !validSubClassification(a.Type, a.SubClassification) {
		return ErrInvalidSubClassificationForType
	}
	return nil
}

// validateEffectiveRange rejects an inverted validity window. Both bounds are
// inclusive; a nil EffectiveTo means "still current" (RULE-KERNEL-017).
func validateEffectiveRange(from time.Time, to *time.Time) error {
	if to != nil && !from.IsZero() && to.UTC().Before(from.UTC()) {
		return ErrInvalidEffectiveRange
	}
	return nil
}

func validSubClassification(accountType AccountType, sub string) bool {
	sub = strings.ToLower(strings.TrimSpace(sub))
	switch accountType {
	case AccountTypeAsset:
		switch sub {
		case SubClassCashAndEquivalents, SubClassTradeReceivables, SubClassInventories,
			SubClassPropertyPlantEquipment, SubClassIntangibleAssets, SubClassRightOfUseAssets,
			SubClassFinancialInvestments, SubClassPrepaymentsOtherDebit,
			SubClassOtherNonCurrentAssets, SubClassCurrentAssets, SubClassNonCurrentAssets:
			return true
		}
	case AccountTypeLiability:
		switch sub {
		case SubClassTradePayables, SubClassShortTermBorrowings, SubClassCurrentLeaseLiabilities,
			SubClassAccrualsOtherCredit, SubClassCurrentTaxLiabilities, SubClassLongTermBorrowings,
			SubClassNonCurrentLeaseLiabilities, SubClassEndOfServiceProvisions,
			SubClassOtherNonCurrentLiabilities, SubClassCurrentLiabilities, SubClassNonCurrentLiabilities:
			return true
		}
	case AccountTypeEquity:
		switch sub {
		case SubClassShareCapital, SubClassSharePremium, SubClassStatutoryReserve,
			SubClassOCIReserves, SubClassRetainedEarnings, SubClassTreasuryShares, SubClassDistributions:
			return true
		}
	case AccountTypeIncome:
		return sub == SubClassOperatingRevenue || sub == SubClassOtherOperatingIncome ||
			sub == SubClassFinanceIncome
	case AccountTypeExpense:
		switch sub {
		case SubClassCostOfSales, SubClassSellingDistributionExpense, SubClassGeneralAdminExpense,
			SubClassDepreciationAmortExpense, SubClassExpectedCreditLossExpense,
			SubClassOtherOperatingExpense, SubClassFinanceCost, SubClassTaxExpense, SubClassIncomeTaxExpense:
			return true
		}
	}
	return false
}

func (a Account) IsCurrent(asOf time.Time) bool {
	asOf = asOf.UTC()
	if !a.EffectiveFrom.IsZero() && asOf.Before(a.EffectiveFrom.UTC()) {
		return false
	}
	if a.EffectiveTo == nil {
		return true
	}
	effectiveTo := a.EffectiveTo.UTC()
	if !a.EffectiveFrom.IsZero() && effectiveTo.Before(a.EffectiveFrom.UTC()) {
		return false
	}
	return !asOf.After(effectiveTo)
}

func (a Account) EffectiveIsCashEquivalent(asOf time.Time) bool {
	return a.IsCurrent(asOf) && a.IsCashEquivalent
}

func (a *Account) IsValid() error {
	if a == nil {
		return ErrInvalidAccountType
	}
	if a.ID == (types.AccountID{}) {
		return ErrInvalidAccountType
	}
	return a.Validate()
}
