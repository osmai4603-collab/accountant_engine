package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
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
