package masterdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func TestValidateCanPostToAccountUsesEffectiveDate(t *testing.T) {
	ctx := context.Background()
	reader := newMockReader()
	companyID, accountID := types.NewCompanyID(), types.NewAccountID()
	effectiveFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	effectiveTo := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	reader.accounts[accountID] = Account{
		ID: accountID, CompanyID: companyID, Code: "1000", IsActive: true,
		EffectiveFrom: effectiveFrom, EffectiveTo: &effectiveTo,
	}

	if err := ValidateCanPostToAccountAsOf(ctx, reader, companyID, accountID, effectiveFrom); err != nil {
		t.Fatalf("posting on effective-from date: %v", err)
	}
	if err := ValidateCanPostToAccountAsOf(ctx, reader, companyID, accountID, effectiveTo); err != nil {
		t.Fatalf("posting on effective-to date: %v", err)
	}
	if err := ValidateCanPostToAccountAsOf(ctx, reader, companyID, accountID, effectiveFrom.Add(-time.Nanosecond)); !errors.Is(err, ErrAccountNotCurrent) {
		t.Fatalf("posting before effective date error = %v, want account-not-current", err)
	}
	if err := ValidateCanPostToAccountAsOf(ctx, reader, companyID, accountID, effectiveTo.Add(time.Nanosecond)); !errors.Is(err, ErrAccountNotCurrent) {
		t.Fatalf("posting after expiration error = %v, want account-not-current", err)
	}
}

func TestAccountExpirationAppliesWithoutEffectiveFrom(t *testing.T) {
	expires := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	account := Account{EffectiveTo: &expires}
	if account.IsCurrent(expires.Add(time.Nanosecond)) {
		t.Fatal("account with no effective-from date remained current after expiration")
	}
	if !account.IsCurrent(expires) {
		t.Fatal("effective-to date should remain inclusive")
	}

	start := expires.Add(time.Hour)
	account.EffectiveFrom, account.EffectiveTo = start, &expires
	if account.IsCurrent(expires) || account.IsCurrent(start) {
		t.Fatal("account with reversed effective date range should never be current")
	}
}

func TestAdjustmentPostingRequiresPeriodLookupCapability(t *testing.T) {
	reader := newMockReader()
	companyID := types.NewCompanyID()
	year, periods := GenerateFiscalYear(companyID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	reader.fiscalYears[year.ID] = year
	reader.periods[companyID] = periods

	_, err := ValidateAdjustmentPeriodForPosting(
		context.Background(),
		reader,
		companyID,
		time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC),
		13,
	)
	if !errors.Is(err, ErrAdjustmentPeriodReaderRequired) {
		t.Fatalf("adjustment period without lookup capability error = %v, want reader-required", err)
	}

}

func TestAccountValidationStoresDefaultNormalBalanceAndAcceptsFinanceIncome(t *testing.T) {
	account := Account{
		Code: "4100", Name: "Finance income", Type: AccountTypeIncome,
		SubClassification: SubClassFinanceIncome,
	}
	if err := account.Validate(); err != nil {
		t.Fatalf("Validate finance-income account: %v", err)
	}
	if account.NormalBalance != NormalBalanceCredit {
		t.Fatalf("normal balance = %q, want %q", account.NormalBalance, NormalBalanceCredit)
	}
}

func TestContraAccountRequiresValidNormalBalance(t *testing.T) {
	account := Account{
		Code: "1099", Name: "Contra asset", Type: AccountTypeAsset,
		IsContraAccount: true, NormalBalance: "neutral",
	}
	if err := account.Validate(); !errors.Is(err, ErrInvalidNormalBalance) {
		t.Fatalf("invalid contra normal balance error = %v, want invalid normal balance", err)
	}

	account.NormalBalance = NormalBalanceCredit
	if err := account.Validate(); err != nil {
		t.Fatalf("contra account with reversed normal balance: %v", err)
	}
}
