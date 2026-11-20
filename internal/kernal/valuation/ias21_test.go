package valuation

import (
	"context"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

func TestIAS21ExchangeRateAndRevaluation(t *testing.T) {
	ctx := context.Background()
	registry := NewRateRegistry()

	rateDate := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	// Register spot and closing rates: 1 USD = 3.75 SAR
	err := registry.RegisterRate(ExchangeRate{
		FromCurrency:  "USD",
		ToCurrency:    "SAR",
		Rate:          decimal.RequireFromString("3.75"),
		EffectiveDate: rateDate,
		Type:          ClosingRate,
	})
	if err != nil {
		t.Fatalf("RegisterRate failed: %v", err)
	}

	// 1. Direct and Inverted Rate Retrieval
	rate, _, inverted, err := registry.GetRate(ctx, ClosingRate, "USD", "SAR", rateDate)
	if err != nil || !rate.Equal(decimal.RequireFromString("3.75")) || inverted {
		t.Fatalf("expected direct rate 3.75, got %s, inverted=%v, err=%v", rate, inverted, err)
	}

	// Inverted lookup SAR -> USD
	invRate, _, inverted, err := registry.GetRate(ctx, ClosingRate, "SAR", "USD", rateDate)
	if err != nil || !inverted {
		t.Fatalf("expected inverted rate, err=%v", err)
	}
	expectedInv := decimal.NewFromInt(1).DivRound(decimal.RequireFromString("3.75"), 12)
	if !invRate.Equal(expectedInv) {
		t.Fatalf("expected %s, got %s", expectedInv, invRate)
	}

	// 2. Exact Conversion with Banker's Rounding
	usdMoney, _ := types.NewMoneyFromString("100.00", "USD")
	converted, err := Convert(usdMoney, "SAR", decimal.RequireFromString("3.75"))
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	expectedSAR, _ := types.NewMoneyFromString("375.00", "SAR")
	if !converted.Equal(expectedSAR) {
		t.Fatalf("expected %s, got %s", expectedSAR, converted)
	}

	// 3. IAS 21 Monetary Position Revaluation (Asset: Bank Balance)
	engine := NewRevaluationEngine(registry)
	bankAccountID := apiTypes.NewAccountID()

	// Initial carrying amount: 10,000 USD booked at 3.70 SAR = 37,000 SAR
	carrying, _ := types.NewMoneyFromString("37000.00", "SAR")
	pos := MonetaryPosition{
		AccountID:              bankAccountID,
		ForeignCurrencyBalance: decimal.NewFromInt(10000),
		TransactionCurrency:    "USD",
		FunctionalCurrency:     "SAR",
		CarryingAmount:         carrying,
		Type:                   Monetary,
		IsLiability:            false,
	}

	result, err := engine.Calculate(ctx, pos, rateDate)
	if err != nil {
		t.Fatalf("engine.Calculate failed: %v", err)
	}

	// Revalued = 10,000 * 3.75 = 37,500 SAR -> Gain of 500 SAR
	if !result.IsGain {
		t.Fatalf("expected FX Gain, got loss")
	}
	expectedGain, _ := types.NewMoneyFromString("500.00", "SAR")
	if !result.Difference.Equal(expectedGain) {
		t.Fatalf("expected gain %s, got %s", expectedGain, result.Difference)
	}

	// 4. Generate Double-Entry Journal for the Gain
	compID := apiTypes.NewCompanyID()
	journalID := apiTypes.NewJournalID()
	gainAccID := apiTypes.NewAccountID()
	lossAccID := apiTypes.NewAccountID()

	entry, err := GenerateRevaluationEntry(result, FXJournalAccounts{
		CompanyID:        compID,
		JournalID:        journalID,
		UnrealizedGainID: gainAccID,
		UnrealizedLossID: lossAccID,
	}, rateDate)
	if err != nil {
		t.Fatalf("GenerateRevaluationEntry failed: %v", err)
	}

	if len(entry.Lines) != 2 {
		t.Fatalf("expected 2 lines in reval entry, got %d", len(entry.Lines))
	}
	// Debit Bank Account 500 SAR, Credit Unrealized Gain 500 SAR
	if entry.Lines[0].AccountID != bankAccountID || !entry.Lines[0].FunctionalDebit.Amount().Equal(decimal.NewFromInt(500)) {
		t.Fatalf("expected Debit Bank 500, got %v", entry.Lines[0])
	}
	if entry.Lines[1].AccountID != gainAccID || !entry.Lines[1].FunctionalCredit.Amount().Equal(decimal.NewFromInt(500)) {
		t.Fatalf("expected Credit Gain 500, got %v", entry.Lines[1])
	}
}
