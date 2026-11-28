package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func TestPostingPolicyAndEligibility(t *testing.T) {
	ctx := context.Background()
	engine := NewPolicyEngine()

	compID := apiTypes.NewCompanyID()
	effectiveDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pol := PostingPolicy{
		CompanyID:                         compID,
		MaxBackdatingDays:                 30,
		AllowBackdatingException:          false,
		RequireCostCenterForIncomeExpense: true,
		EffectiveDate:                     effectiveDate,
	}

	if err := engine.SetPolicy(pol); err != nil {
		t.Fatalf("SetPolicy failed: %v", err)
	}

	expenseAccID := apiTypes.NewAccountID()
	cashAccID := apiTypes.NewAccountID()

	accountTypes := map[apiTypes.AccountID]masterdata.AccountType{
		expenseAccID: masterdata.AccountTypeExpense,
		cashAccID:    masterdata.AccountTypeAsset,
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// 1. Violation: Expense line without cost center
	entryNoCC := &journal.JournalEntry{
		CompanyID:     compID,
		EffectiveDate: now,
		Lines: []journal.EntryLine{
			{
				AccountID:    expenseAccID,
				CostCenterID: nil, // Missing Cost Center!
			},
			{
				AccountID: cashAccID,
			},
		},
	}

	err := engine.ValidatePostingPolicy(ctx, pol, entryNoCC, accountTypes, now)
	if !errors.Is(err, ErrCostCenterRequired) {
		t.Fatalf("expected ErrCostCenterRequired, got %v", err)
	}

	// 2. Add Cost Center -> Should Pass
	ccID := apiTypes.NewCostCenterID()
	entryWithCC := &journal.JournalEntry{
		CompanyID:     compID,
		EffectiveDate: now,
		Lines: []journal.EntryLine{
			{
				AccountID:    expenseAccID,
				CostCenterID: &ccID,
			},
			{
				AccountID: cashAccID,
			},
		},
	}
	if err := engine.ValidatePostingPolicy(ctx, pol, entryWithCC, accountTypes, now); err != nil {
		t.Fatalf("expected entry with cost center to pass, got %v", err)
	}

	// 3. Violation: Backdating beyond 30 days
	oldDate := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC) // 60 days old
	entryOld := &journal.JournalEntry{
		CompanyID:     compID,
		EffectiveDate: oldDate,
		Lines: []journal.EntryLine{
			{
				AccountID:    expenseAccID,
				CostCenterID: &ccID,
			},
			{
				AccountID: cashAccID,
			},
		},
	}
	err = engine.ValidatePostingPolicy(ctx, pol, entryOld, accountTypes, now)
	if !errors.Is(err, ErrBackdatingLimit) {
		t.Fatalf("expected ErrBackdatingLimit, got %v", err)
	}

	// 4. Currency Eligibility Check
	accounts := map[apiTypes.AccountID]masterdata.Account{
		cashAccID: {
			ID:       cashAccID,
			Code:     "1010",
			Currency: "USD",
			IsActive: true,
		},
	}
	eurLineEntry := &journal.JournalEntry{
		Lines: []journal.EntryLine{
			{
				AccountID:           cashAccID,
				TransactionCurrency: "EUR", // Mismatch with USD account!
			},
		},
	}
	err = CheckAccountEligibility(ctx, eurLineEntry, accounts)
	if !errors.Is(err, apiTypes.ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch for currency-restricted account, got %v", err)
	}
}

func TestPolicyEngineResolvesLatestEffectivePostingAndCurrencyPolicies(t *testing.T) {
	engine := NewPolicyEngine()
	companyID := apiTypes.NewCompanyID()
	january := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, postingPolicy := range []PostingPolicy{
		{CompanyID: companyID, EffectiveDate: january, MaxBackdatingDays: 10},
		{CompanyID: companyID, EffectiveDate: july, MaxBackdatingDays: 30},
	} {
		if err := engine.SetPolicy(postingPolicy); err != nil {
			t.Fatal(err)
		}
	}
	for _, currencyPolicy := range []CurrencyPolicy{
		{CompanyID: companyID, FunctionalCurrency: "USD", EffectiveDate: january, AllowedCurrencies: []string{"USD"}},
		{CompanyID: companyID, FunctionalCurrency: "EUR", EffectiveDate: july, AllowedCurrencies: []string{"EUR"}},
	} {
		if err := engine.SetCurrencyPolicy(currencyPolicy); err != nil {
			t.Fatal(err)
		}
	}

	selectedPosting, err := engine.PostingPolicyFor(companyID, july.AddDate(0, -1, 0))
	if err != nil || selectedPosting.MaxBackdatingDays != 10 {
		t.Fatalf("pre-July policy = %+v, %v; want January version", selectedPosting, err)
	}
	selectedCurrency, err := engine.CurrencyPolicyFor(companyID, july)
	if err != nil || selectedCurrency.FunctionalCurrency != "EUR" {
		t.Fatalf("July currency policy = %+v, %v; want EUR version", selectedCurrency, err)
	}
	selectedCurrency.AllowedCurrencies[0] = "GBP"
	unchangedCurrency, err := engine.CurrencyPolicyFor(companyID, july)
	if err != nil || unchangedCurrency.AllowedCurrencies[0] != "EUR" {
		t.Fatalf("mutating returned currency policy changed stored values: %+v, %v", unchangedCurrency, err)
	}
	if _, err := engine.PostingPolicyFor(companyID, january.Add(-time.Nanosecond)); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("policy lookup before first effective date error = %v, want policy not found", err)
	}
	if err := engine.SetPolicy(PostingPolicy{CompanyID: companyID, EffectiveDate: july}); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("duplicate policy version error = %v, want invalid policy", err)
	}
}

func TestPolicyEngineCopiesCurrencyAllowListsAndRejectsNilReceiver(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	effectiveDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	allowed := []string{"USD", "EUR"}
	policy := CurrencyPolicy{
		CompanyID: companyID, FunctionalCurrency: "USD",
		AllowedCurrencies: allowed, EffectiveDate: effectiveDate,
	}
	engine := NewPolicyEngine()
	if err := engine.SetCurrencyPolicy(policy); err != nil {
		t.Fatal(err)
	}
	allowed[0] = "GBP"

	resolved, err := engine.CurrencyPolicyFor(companyID, effectiveDate)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.AllowedCurrencies[0] != "USD" {
		t.Fatalf("mutating input allow-list changed stored policy: %v", resolved.AllowedCurrencies)
	}
	resolved.AllowedCurrencies[0] = "GBP"
	again, err := engine.CurrencyPolicyFor(companyID, effectiveDate)
	if err != nil || again.AllowedCurrencies[0] != "USD" {
		t.Fatalf("mutating resolved allow-list changed stored policy: %v, %v", again.AllowedCurrencies, err)
	}

	var nilEngine *PolicyEngine
	if err := nilEngine.SetPolicy(PostingPolicy{}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil engine SetPolicy error = %v, want ErrPolicyEngineRequired", err)
	}
	if err := nilEngine.SetCurrencyPolicy(CurrencyPolicy{}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil engine SetCurrencyPolicy error = %v, want ErrPolicyEngineRequired", err)
	}
	if _, err := nilEngine.PostingPolicyFor(companyID, effectiveDate); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil engine PostingPolicyFor error = %v, want ErrPolicyEngineRequired", err)
	}
	if _, err := nilEngine.CurrencyPolicyFor(companyID, effectiveDate); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil engine CurrencyPolicyFor error = %v, want ErrPolicyEngineRequired", err)
	}
}

func TestCurrencyPolicyRoundingAndAllowedCurrency(t *testing.T) {
	pol := CurrencyPolicy{
		FunctionalCurrency: "USD", AllowedCurrencies: []string{"EUR"},
		RoundingMethod: RoundingHalfUp,
	}
	amount, err := apiTypes.NewMoneyFromString("1.005", "USD")
	if err != nil {
		t.Fatal(err)
	}
	rounded, err := pol.Round(amount)
	if err != nil || rounded.Amount().StringFixed(2) != "1.01" {
		t.Fatalf("half-up round = %s, %v; want 1.01 USD", rounded, err)
	}
	if !pol.AllowsCurrency(" usd ") || !pol.AllowsCurrency("eur") || pol.AllowsCurrency("GBP") {
		t.Fatal("currency policy allowed an unconfigured currency or rejected a configured one")
	}
	if pol.AllowsCurrency("XXX") || pol.AllowsCurrency("UNKNOWN") {
		t.Fatal("currency policy must reject XXX and unknown currencies")
	}
	if err := (CurrencyPolicy{
		CompanyID: apiTypes.NewCompanyID(), FunctionalCurrency: "USD",
		AllowedCurrencies: []string{"usd", "USD"}, EffectiveDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("duplicate allowed currencies error = %v, want ErrInvalidPolicy", err)
	}
}

func TestAccountEligibilityRejectsControlAccounts(t *testing.T) {
	accountID := apiTypes.NewAccountID()
	entry := &journal.JournalEntry{Lines: []journal.EntryLine{{AccountID: accountID, TransactionCurrency: "USD"}}}
	accounts := map[apiTypes.AccountID]masterdata.Account{
		accountID: {ID: accountID, Code: "AR", IsActive: true, IsControlAccount: true},
	}
	if err := CheckAccountEligibility(context.Background(), entry, accounts); !errors.Is(err, ErrControlAccountNotPostable) {
		t.Fatalf("control-account eligibility error = %v, want prohibited", err)
	}
}

func TestAccountEligibilityEnforcesAccountCostCenterRequirement(t *testing.T) {
	accountID := apiTypes.NewAccountID()
	entry := &journal.JournalEntry{Lines: []journal.EntryLine{{AccountID: accountID}}}
	accounts := map[apiTypes.AccountID]masterdata.Account{
		accountID: {ID: accountID, Code: "6010", IsActive: true, RequiresCostCenter: true},
	}
	if err := CheckAccountEligibility(context.Background(), entry, accounts); !errors.Is(err, ErrCostCenterRequired) {
		t.Fatalf("missing account-required cost center error = %v, want cost-center-required", err)
	}

	costCenterID := apiTypes.NewCostCenterID()
	entry.Lines[0].CostCenterID = &costCenterID
	if err := CheckAccountEligibility(context.Background(), entry, accounts); err != nil {
		t.Fatalf("account with required cost center: %v", err)
	}
}
