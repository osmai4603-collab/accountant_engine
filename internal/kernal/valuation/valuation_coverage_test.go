package valuation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

type mockRateProviderSPI struct {
	rate decimal.Decimal
	err  error
}

func (m *mockRateProviderSPI) GetRate(ctx context.Context, rateType RateType, fromCurrency, toCurrency string, date time.Time) (decimal.Decimal, error) {
	if m.err != nil {
		return decimal.Zero, m.err
	}
	return m.rate, nil
}

func TestValuationCoverageBranches(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	// 1. ConvertForDate branches:
	// a. nil provider
	amt, _ := types.NewMoney(decimal.NewFromInt(100), "USD")
	if _, _, err := ConvertForDate(ctx, nil, amt, "EUR", date, SpotRate); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("nil provider ConvertForDate error = %v, want ErrRateNotFound", err)
	}

	// b. provider error
	errProv := &mockRateProviderSPI{err: errors.New("provider err")}
	if _, _, err := ConvertForDate(ctx, errProv, amt, "EUR", date, SpotRate); err == nil {
		t.Fatal("expected error from provider")
	}

	// c. non-positive rate
	negProv := &mockRateProviderSPI{rate: decimal.NewFromInt(-1)}
	if _, _, err := ConvertForDate(ctx, negProv, amt, "EUR", date, SpotRate); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("neg rate ConvertForDate error = %v, want ErrInvalidRate", err)
	}

	// d. same currency with rate != 1
	twoProv := &mockRateProviderSPI{rate: decimal.NewFromInt(2)}
	if _, _, err := ConvertForDate(ctx, twoProv, amt, "USD", date, SpotRate); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("same curr non-1 rate error = %v, want ErrInvalidRate", err)
	}

	// 2. ExchangeRate.Validate branches:
	// a. Invalid currency in ToCurrency
	badToRate := ExchangeRate{
		FromCurrency:  "USD",
		ToCurrency:    "INVALID",
		Rate:          decimal.NewFromInt(1),
		EffectiveDate: date,
		Type:          SpotRate,
	}
	if err := badToRate.Validate(); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("invalid ToCurrency ExchangeRate.Validate error = %v, want ErrInvalidCurrency", err)
	}

	// b. invalid RateType
	badTypeRate := ExchangeRate{
		FromCurrency:  "USD",
		ToCurrency:    "EUR",
		Rate:          decimal.NewFromInt(1),
		EffectiveDate: date,
		Type:          RateType("UNKNOWN"),
	}
	if err := badTypeRate.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("invalid rate type ExchangeRate.Validate error = %v, want ErrInvalidRate", err)
	}

	// 3. Convert function branches:
	if _, err := Convert(types.Money{}, "EUR", decimal.NewFromInt(-1)); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("neg rate Convert error = %v, want ErrInvalidRate", err)
	}
	if _, err := Convert(amt, "USD", decimal.NewFromInt(2)); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("same curr non-1 rate Convert error = %v, want ErrInvalidRate", err)
	}
	if _, err := Convert(amt, "INVALID", decimal.NewFromInt(1)); !errors.Is(err, types.ErrInvalidCurrency) {
		t.Fatalf("invalid curr Convert error = %v, want ErrInvalidCurrency", err)
	}

	// 4. RevaluationEngine.Calculate branches:
	registry := NewRateRegistry()
	accID := types.NewAccountID()
	eng := NewRevaluationEngine(registry)

	// a. position.Type != Monetary
	if _, err := eng.Calculate(ctx, MonetaryPosition{Type: PositionType("non_monetary")}, date); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("non monetary error = %v, want ErrInvalidPosition", err)
	}

	// b. zero asOf date
	if _, err := eng.Calculate(ctx, MonetaryPosition{Type: Monetary}, time.Time{}); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("zero asOf error = %v, want ErrInvalidPosition", err)
	}

	// c. invalid position fields (missing account, empty currency, negative balance)
	carryingAmt, _ := types.NewMoney(decimal.NewFromInt(100), "SAR")
	posMissingAcc := MonetaryPosition{
		Type:                   Monetary,
		TransactionCurrency:    "USD",
		FunctionalCurrency:     "SAR",
		ForeignCurrencyBalance: decimal.NewFromInt(100),
		CarryingAmount:         carryingAmt,
	}
	if _, err := eng.Calculate(ctx, posMissingAcc, date); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("missing acc error = %v, want ErrInvalidPosition", err)
	}

	// d. invalid currencies in position
	posInvalidCurr := MonetaryPosition{
		AccountID:              accID,
		Type:                   Monetary,
		TransactionCurrency:    "INVALID",
		FunctionalCurrency:     "SAR",
		ForeignCurrencyBalance: decimal.NewFromInt(100),
		CarryingAmount:         carryingAmt,
	}
	if _, err := eng.Calculate(ctx, posInvalidCurr, date); !errors.Is(err, types.ErrInvalidCurrency) {
		t.Fatalf("invalid curr error = %v, want ErrInvalidCurrency", err)
	}

	// e. nil provider in engine
	engNil := NewRevaluationEngine(nil)
	posValid := MonetaryPosition{
		AccountID:              accID,
		Type:                   Monetary,
		TransactionCurrency:    "USD",
		FunctionalCurrency:     "SAR",
		ForeignCurrencyBalance: decimal.NewFromInt(100),
		CarryingAmount:         carryingAmt,
	}
	if _, err := engNil.Calculate(ctx, posValid, date); !errors.Is(err, ErrRateNotFound) {
		t.Fatalf("nil provider Calculate error = %v, want ErrRateNotFound", err)
	}

	// f. same currency revaluation where rate != 1
	// using a custom mock provider that implements RevaluationRateProvider
	// 5. GenerateRevaluationEntry with zero difference
	zeroDiffMoney, _ := types.Zero("SAR")
	resZero := FXResult{
		AccountID:  accID,
		Currency:   "USD",
		Difference: zeroDiffMoney,
	}
	if _, err := GenerateRevaluationEntry(resZero, FXJournalAccounts{}, date); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("zero diff GenerateRevaluationEntry error = %v, want ErrInvalidPosition", err)
	}

	// 6. GenerateRevaluationEntry with Loss (Asset downward)
	lossDiffMoney, _ := types.NewMoney(decimal.NewFromInt(-50), "SAR")
	resLoss := FXResult{
		AccountID:      accID,
		Currency:       "USD",
		Difference:     lossDiffMoney,
		IsGain:         false,
		ClosingRate:    decimal.NewFromFloat(3.75),
		RevaluedAmount: carryingAmt,
	}
	accounts := FXJournalAccounts{
		CompanyID:        types.NewCompanyID(),
		JournalID:        types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(),
		UnrealizedLossID: types.NewAccountID(),
	}
	entryLoss, err := GenerateRevaluationEntry(resLoss, accounts, date)
	if err != nil {
		t.Fatalf("GenerateRevaluationEntry Loss: %v", err)
	}
	if len(entryLoss.Lines) != 2 {
		t.Fatalf("loss entry lines count = %d, want 2", len(entryLoss.Lines))
	}
}
