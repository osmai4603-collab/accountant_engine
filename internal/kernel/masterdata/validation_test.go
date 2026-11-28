package masterdata

import (
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func TestCostCenterAndAnalyticalDimensionValidation(t *testing.T) {
	companyID := types.NewCompanyID()
	centerID := types.NewCostCenterID()
	center := CostCenter{
		ID: centerID, CompanyID: companyID, Code: "CC-1", Name: "Operations", Active: true,
	}
	if err := center.Validate(); err != nil {
		t.Fatalf("valid cost center: %v", err)
	}
	self := centerID
	center.ParentID = &self
	if err := center.Validate(); !errors.Is(err, ErrCostCenterCycle) {
		t.Fatalf("self-referencing cost center error = %v", err)
	}
	center = CostCenter{Code: "", Name: ""}
	if err := center.Validate(); !errors.Is(err, ErrInvalidCostCenter) {
		t.Fatalf("invalid cost center error = %v", err)
	}

	projectID := types.NewProjectID()
	dimension := AnalyticalDimensionValue{
		ID: types.ProjectID(projectID), CompanyID: companyID,
		Kind: DimensionProject, Code: "P-1", Name: "Project One", Active: true,
	}
	if err := dimension.Validate(); err != nil {
		t.Fatalf("valid analytical dimension: %v", err)
	}
	dimension.Kind = AnalyticalDimensionKind("unknown")
	if err := dimension.Validate(); !errors.Is(err, ErrInvalidAnalyticalDimension) {
		t.Fatalf("invalid analytical dimension error = %v", err)
	}
}

func TestAccountValidationMatrix(t *testing.T) {
	account := Account{
		ID: types.NewAccountID(), CompanyID: types.NewCompanyID(),
		Code: "1000", Name: "Cash", Type: AccountTypeAsset,
		SubClassification: SubClassCashAndEquivalents,
		IsReconcilable:    true, IsControlAccount: true,
		IsCashEquivalent: true,
	}
	if err := account.IsValid(); err != nil {
		t.Fatalf("valid account: %v", err)
	}
	if account.NormalBalance != NormalBalanceDebit {
		t.Fatalf("default normal balance = %q, want debit", account.NormalBalance)
	}
	if !account.EffectiveIsCashEquivalent(time.Now().UTC()) {
		t.Fatal("active cash-equivalent account should be recognized")
	}

	invalid := account
	invalid.SubClassification = "not_a_valid_asset_subclass"
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid sub-classification must be rejected")
	}
	invalid = account
	invalid.Type = AccountTypeLiability
	invalid.NormalBalance = NormalBalanceDebit
	if err := invalid.Validate(); err == nil {
		t.Fatal("liability account with debit normal balance must be rejected")
	}
	invalid = account
	invalid.ID = (types.AccountID{})
	if err := invalid.IsValid(); err == nil {
		t.Fatal("account without ID must be rejected")
	}
}

func TestJournalValidationAndPolicyMethods(t *testing.T) {
	companyID := types.NewCompanyID()
	journalID := types.NewJournalID()
	accountID := types.NewAccountID()
	journal := Journal{
		ID: journalID, CompanyID: companyID, Code: "GEN",
		Name: "General", AllowedCurrencies: []string{"USD", "EUR"},
		AllowedAccountIDs: []types.AccountID{accountID},
	}
	if err := journal.Validate(); err != nil {
		t.Fatalf("valid journal: %v", err)
	}
	if !journal.AllowsCurrency("eur") || !journal.AllowsAccount(accountID) {
		t.Fatal("journal policy should allow configured currency and account")
	}
	if journal.AllowsCurrency("SAR") || journal.AllowsAccount(types.NewAccountID()) {
		t.Fatal("journal policy should reject unconfigured currency and account")
	}
	if err := journal.ValidateWithPolicy([]string{"USD", "usd"}, nil); err == nil {
		t.Fatal("duplicate currencies must be rejected")
	}
	if err := journal.ValidateWithPolicy([]string{"BAD"}, nil); err == nil {
		t.Fatal("unknown currency must be rejected")
	}
	if err := journal.ValidateWithPolicy(nil, []types.AccountID{{}}); err == nil {
		t.Fatal("zero account ID must be rejected")
	}

	closedTo := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	journal.EffectiveFrom = closedTo.Add(time.Hour)
	journal.EffectiveTo = &closedTo
	if err := journal.Validate(); err == nil {
		t.Fatal("journal with effective-to before effective-from must be rejected")
	}

	journalType := JournalType{
		ID: journalID, CompanyID: companyID, Code: "GEN", Name: "General",
	}
	if err := journalType.Validate(); err != nil {
		t.Fatalf("valid journal type: %v", err)
	}
	if !journalType.AllowsCurrency("USD") || !journalType.AllowsAccount(accountID) {
		t.Fatal("empty journal type policies must allow values")
	}
}

func TestMasterDataBalanceAndValidityBranches(t *testing.T) {
	for _, test := range []struct {
		accountType AccountType
		want        NormalBalance
	}{
		{AccountTypeAsset, NormalBalanceDebit},
		{AccountTypeExpense, NormalBalanceDebit},
		{AccountTypeLiability, NormalBalanceCredit},
		{AccountTypeEquity, NormalBalanceCredit},
		{AccountTypeIncome, NormalBalanceCredit},
		{AccountType("unknown"), ""},
	} {
		if got := DefaultNormalBalance(test.accountType); got != test.want {
			t.Errorf("DefaultNormalBalance(%s) = %q, want %q", test.accountType, got, test.want)
		}
	}

	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC)
	account := Account{EffectiveFrom: from, EffectiveTo: &to}
	if account.IsCurrent(from) == false {
		t.Fatal("account should be current at effective-from")
	}
	if account.IsCurrent(from.Add(-time.Hour)) {
		t.Fatal("account should not be current before effective-from")
	}
	if account.IsCurrent(to.Add(time.Hour)) {
		t.Fatal("account should not be current after effective-to")
	}
	invalidRange := Account{EffectiveFrom: to, EffectiveTo: &from}
	if invalidRange.IsCurrent(from) {
		t.Fatal("invalid effective range should not be current")
	}

	invalidJournalType := JournalType{}
	if err := invalidJournalType.Validate(); err == nil {
		t.Fatal("journal type without identity must be rejected")
	}
	configuredJournalType := JournalType{
		ID: types.NewJournalID(), CompanyID: types.NewCompanyID(),
		Code: "G", Name: "General",
		AllowedCurrencies: []string{"USD"},
		AllowedAccountIDs: []types.AccountID{types.NewAccountID()},
	}
	accountID := configuredJournalType.AllowedAccountIDs[0]
	if !configuredJournalType.AllowsCurrency("usd") || configuredJournalType.AllowsCurrency("EUR") {
		t.Fatal("configured journal currency policy mismatch")
	}
	if !configuredJournalType.AllowsAccount(accountID) || configuredJournalType.AllowsAccount(types.NewAccountID()) {
		t.Fatal("configured journal account policy mismatch")
	}
}

func TestAllAccountSubClassesAndValidationBranches(t *testing.T) {
	subClasses := []struct {
		accountType AccountType
		sub         string
	}{
		{AccountTypeAsset, SubClassCurrentAssets},
		{AccountTypeAsset, SubClassPropertyPlantEquipment},
		{AccountTypeLiability, SubClassTradePayables},
		{AccountTypeLiability, SubClassLongTermBorrowings},
		{AccountTypeEquity, SubClassRetainedEarnings},
		{AccountTypeEquity, SubClassTreasuryShares},
		{AccountTypeIncome, SubClassOperatingRevenue},
		{AccountTypeIncome, SubClassFinanceIncome},
		{AccountTypeExpense, SubClassCostOfSales},
		{AccountTypeExpense, SubClassFinanceCost},
	}
	for i, test := range subClasses {
		account := Account{
			ID: types.NewAccountID(), Code: "A", Name: "Test",
			Type: test.accountType, SubClassification: test.sub,
		}
		if err := account.Validate(); err != nil {
			t.Errorf("valid sub-class %d: %v", i, err)
		}
	}

	tests := []Account{
		{Code: "", Name: "Name", Type: AccountTypeAsset},
		{Code: "1", Name: "", Type: AccountTypeAsset},
		{Code: "1", Name: "Name", Type: AccountType("unknown")},
		{Code: "1", Name: "Name", Type: AccountTypeAsset, NormalBalance: NormalBalance("unknown")},
		{Code: "1", Name: "Name", Type: AccountTypeAsset, NormalBalance: NormalBalanceCredit},
		{Code: "1", Name: "Name", Type: AccountTypeAsset, IsControlAccount: true},
	}
	for i, account := range tests {
		if err := account.Validate(); err == nil {
			t.Errorf("invalid account %d unexpectedly passed", i)
		}
	}

	cash := Account{
		ID: types.NewAccountID(), Code: "100", Name: "Cash",
		Type: AccountTypeAsset, IsCashEquivalent: true,
	}
	if err := cash.IsValid(); err != nil {
		t.Fatalf("valid account with default balance: %v", err)
	}
	cash.ID = (types.AccountID{})
	if err := cash.IsValid(); err == nil {
		t.Fatal("IsValid must reject zero account ID")
	}
}
