package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func TestPolicyValidationAndEligibilityEdgeCases(t *testing.T) {
	companyID := types.NewCompanyID()
	effDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 1. CheckAccountEligibility: nil entry, missing account, inactive account
	if err := CheckAccountEligibility(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error on nil entry")
	}

	accID1 := types.NewAccountID()
	accID2 := types.NewAccountID()
	entry := &journal.JournalEntry{
		CompanyID:     companyID,
		EffectiveDate: effDate,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: accID1, TransactionCurrency: "USD"},
		},
	}

	// Missing account
	if err := CheckAccountEligibility(context.Background(), entry, map[apiTypes.AccountID]masterdata.Account{}); err == nil {
		t.Fatal("expected error on missing account")
	}

	// Inactive account
	if err := CheckAccountEligibility(context.Background(), entry, map[apiTypes.AccountID]masterdata.Account{
		accID1: {ID: accID1, Code: "1001", IsActive: false},
	}); err == nil {
		t.Fatal("expected error on inactive account")
	}

	// 2. CurrencyPolicy.Validate edge cases:
	// a. empty company or zero effective date
	if err := (CurrencyPolicy{FunctionalCurrency: "USD"}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("empty company error = %v, want ErrInvalidPolicy", err)
	}

	// b. invalid functional currency
	if err := (CurrencyPolicy{CompanyID: companyID, EffectiveDate: effDate, FunctionalCurrency: "INVALID"}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("invalid func curr error = %v, want ErrInvalidPolicy", err)
	}

	// c. invalid presentation currency
	if err := (CurrencyPolicy{CompanyID: companyID, EffectiveDate: effDate, FunctionalCurrency: "USD", PresentationCurrency: "INVALID"}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("invalid pres curr error = %v, want ErrInvalidPolicy", err)
	}

	// d. invalid rounding method
	if err := (CurrencyPolicy{CompanyID: companyID, EffectiveDate: effDate, FunctionalCurrency: "USD", RoundingMethod: "UNKNOWN"}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("invalid rounding method error = %v, want ErrInvalidPolicy", err)
	}

	// e. auto settle without rounding account
	if err := (CurrencyPolicy{CompanyID: companyID, EffectiveDate: effDate, FunctionalCurrency: "USD", AutoSettleRoundingDifferences: true}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("auto settle without acc error = %v, want ErrInvalidPolicy", err)
	}

	// f. invalid allowed currency
	if err := (CurrencyPolicy{CompanyID: companyID, EffectiveDate: effDate, FunctionalCurrency: "USD", AllowedCurrencies: []string{"INVALID"}}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("invalid allowed currency error = %v, want ErrInvalidPolicy", err)
	}

	// 3. CurrencyPolicy.Round with unsupported method
	currPol := CurrencyPolicy{RoundingMethod: "UNKNOWN"}
	if _, err := currPol.Round(types.Money{}); err == nil {
		t.Fatal("expected error for Round with unknown method")
	}

	// 4. CurrencyPolicy.AllowsCurrency with XXX / empty
	currPolValid := CurrencyPolicy{FunctionalCurrency: "USD", AllowedCurrencies: []string{"EUR"}}
	if currPolValid.AllowsCurrency("XXX") {
		t.Fatal("AllowsCurrency(XXX) should be false")
	}

	// 5. PostingPolicy.Validate: negative MaxBackdatingDays
	if err := (PostingPolicy{CompanyID: companyID, EffectiveDate: effDate, MaxBackdatingDays: -1}).Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("negative max backdating error = %v, want ErrInvalidPolicy", err)
	}

	// 6. PolicyEngine nil receiver or invalid input
	if err := (*PolicyEngine)(nil).SetPolicy(PostingPolicy{}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil engine SetPolicy error = %v, want ErrPolicyEngineRequired", err)
	}
	if err := (*PolicyEngine)(nil).SetCurrencyPolicy(CurrencyPolicy{}); !errors.Is(err, ErrPolicyEngineRequired) {
		t.Fatalf("nil engine SetCurrencyPolicy error = %v, want ErrPolicyEngineRequired", err)
	}

	engine := NewPolicyEngine()
	// duplicate date for same company in SetCurrencyPolicy
	cp := CurrencyPolicy{CompanyID: companyID, EffectiveDate: effDate, FunctionalCurrency: "USD"}
	if err := engine.SetCurrencyPolicy(cp); err != nil {
		t.Fatalf("SetCurrencyPolicy: %v", err)
	}
	if err := engine.SetCurrencyPolicy(cp); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("duplicate currency policy date error = %v, want ErrInvalidPolicy", err)
	}

	// 7. ValidatePostingPolicy branches:
	// a. entry == nil
	if err := engine.ValidatePostingPolicy(context.Background(), PostingPolicy{}, nil, nil, time.Now()); err == nil {
		t.Fatal("expected error on nil entry")
	}
	// b. invalid policy
	if err := engine.ValidatePostingPolicy(context.Background(), PostingPolicy{}, entry, nil, time.Now()); err == nil {
		t.Fatal("expected error on invalid policy")
	}
	// c. company mismatch or date not effective
	validPP := PostingPolicy{CompanyID: companyID, EffectiveDate: effDate.Add(24 * time.Hour)}
	if err := engine.ValidatePostingPolicy(context.Background(), validPP, entry, nil, time.Now()); !errors.Is(err, ErrPolicyNotEffective) {
		t.Fatalf("not effective policy error = %v, want ErrPolicyNotEffective", err)
	}

	validPPCurrent := PostingPolicy{CompanyID: companyID, EffectiveDate: effDate}
	// d. account type not found
	if err := engine.ValidatePostingPolicy(context.Background(), validPPCurrent, entry, map[apiTypes.AccountID]masterdata.AccountType{}, time.Now()); !errors.Is(err, ErrAccountTypeNotFound) {
		t.Fatalf("account type not found error = %v, want ErrAccountTypeNotFound", err)
	}

	// e. analytical dimension requirement missing
	validPPReqDim := PostingPolicy{
		CompanyID:     companyID,
		EffectiveDate: effDate,
		RequireAnalyticalDimensionForIncomeExpense: true,
	}
	accTypes := map[apiTypes.AccountID]masterdata.AccountType{
		accID1: masterdata.AccountTypeIncome,
	}
	if err := engine.ValidatePostingPolicy(context.Background(), validPPReqDim, entry, accTypes, time.Now()); !errors.Is(err, ErrAnalyticalDimensionLimit) {
		t.Fatalf("missing dimension error = %v, want ErrAnalyticalDimensionLimit", err)
	}

	// with analytical dimension provided
	projectID := types.NewProjectID()
	entryWithDim := &journal.JournalEntry{
		CompanyID:     companyID,
		EffectiveDate: effDate,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: accID1, TransactionCurrency: "USD", ProjectID: &projectID},
		},
	}
	if err := engine.ValidatePostingPolicy(context.Background(), validPPReqDim, entryWithDim, accTypes, time.Now()); err != nil {
		t.Fatalf("valid with projectID: %v", err)
	}

	// check other branches of hasDim: SegmentID and BusinessUnitID
	segID := types.NewSegmentID()
	entryWithSeg := &journal.JournalEntry{
		CompanyID:     companyID,
		EffectiveDate: effDate,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: accID1, TransactionCurrency: "USD", SegmentID: &segID},
		},
	}
	if err := engine.ValidatePostingPolicy(context.Background(), validPPReqDim, entryWithSeg, accTypes, time.Now()); err != nil {
		t.Fatalf("valid with segID: %v", err)
	}

	buID := types.NewBusinessUnitID()
	entryWithBU := &journal.JournalEntry{
		CompanyID:     companyID,
		EffectiveDate: effDate,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: accID1, TransactionCurrency: "USD", BusinessUnitID: &buID},
		},
	}
	if err := engine.ValidatePostingPolicy(context.Background(), validPPReqDim, entryWithBU, accTypes, time.Now()); err != nil {
		t.Fatalf("valid with buID: %v", err)
	}
	_ = accID2
}
