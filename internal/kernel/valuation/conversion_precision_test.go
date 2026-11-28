package valuation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
	"github.com/shopspring/decimal"
)

type unitRateProvider struct{ rate decimal.Decimal }

func (p unitRateProvider) GetRate(context.Context, RateType, string, string, time.Time) (decimal.Decimal, error) {
	return p.rate, nil
}

// TestSameCurrencyConversionKeepsUnitRateExact pins RULE-KERNEL-008 at the
// conversion boundary: the unit rate is exact, and any other rate is rejected.
func TestSameCurrencyConversionKeepsUnitRateExact(t *testing.T) {
	date := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	amount := mustMoneySAR(t, "2.777777777778")
	unit := decimal.NewFromInt(1)

	registry := NewRateRegistry()
	rate, _, inverted, err := registry.GetRate(context.Background(), ClosingRate, "SAR", "SAR", date)
	if err != nil || !rate.Equal(unit) || inverted {
		t.Fatalf("same-currency registry rate = %s, inverted=%v, err=%v", rate, inverted, err)
	}
	if _, _, _, err := registry.GetRate(context.Background(), ClosingRate, "XXX", "XXX", date); !errors.Is(err, ErrInvalidCurrency) {
		t.Errorf("XXX same-currency error = %v, want invalid currency", err)
	}
	if _, _, _, err := registry.GetRate(context.Background(), ClosingRate, "SAR", "SAR", time.Time{}); !errors.Is(err, ErrInvalidRate) {
		t.Errorf("zero-date same-currency error = %v, want invalid rate", err)
	}

	converted, err := Convert(amount, "SAR", unit)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := amount.Rounded()
	if err != nil {
		t.Fatal(err)
	}
	if !converted.Equal(expected) {
		t.Fatalf("Convert(SAR->SAR, 1) = %s, want normalized %s", converted.ExactString(), expected.ExactString())
	}
	if _, err := Convert(amount, "SAR", decimal.NewFromInt(2)); !errors.Is(err, ErrInvalidRate) {
		t.Errorf("same-currency rate 2 error = %v, want ErrInvalidRate", err)
	}

	viaProvider, rateUsed, err := ConvertForDate(context.Background(), unitRateProvider{rate: unit}, amount, "sar", date, ClosingRate)
	if err != nil || !rateUsed.Equal(unit) || !viaProvider.Equal(expected) {
		t.Fatalf("ConvertForDate(SAR->SAR, 1) = %s, rate %s, err %v", viaProvider.ExactString(), rateUsed, err)
	}
}

// TestCurrencyConversionIsDeterministic proves repeated conversions with the
// same registered rate and date produce the same minor-unit-exact result.
func TestCurrencyConversionIsDeterministic(t *testing.T) {
	date := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	registry := NewRateRegistry()
	if err := registry.RegisterRate(ExchangeRate{
		FromCurrency:  "SAR",
		ToCurrency:    "USD",
		Rate:          decimal.RequireFromString("0.266666666666"),
		EffectiveDate: date,
		Type:          ClosingRate,
		Source:        "phase2-test",
	}); err != nil {
		t.Fatal(err)
	}
	amount := mustMoneySAR(t, "300")
	var first types.Money
	for i := range 25 {
		rate, _, _, err := registry.GetRate(context.Background(), ClosingRate, "SAR", "USD", date.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		converted, err := Convert(amount, "USD", rate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := converted.MinorUnitsExact(); err != nil {
			t.Fatalf("converted amount must be minor-unit exact: %v", err)
		}
		if i == 0 {
			first = converted
			continue
		}
		if !converted.Equal(first) {
			t.Fatalf("conversion %d = %s, want %s", i, converted.ExactString(), first.ExactString())
		}
	}
}

func mustMoneySAR(t *testing.T, value string) types.Money {
	t.Helper()
	money, err := types.NewMoneyFromString(value, "SAR")
	if err != nil {
		t.Fatalf("NewMoneyFromString(%q, SAR): %v", value, err)
	}
	return money
}

func FuzzFXConversionNeverPanics(f *testing.F) {
	f.Add(int64(100), int64(375), int64(370))
	f.Fuzz(func(t *testing.T, foreignBalance, rateUnits, carryingUnits int64) {
		if foreignBalance <= 0 || rateUnits <= 0 || carryingUnits <= 0 ||
			foreignBalance > 1_000_000 || rateUnits > 1_000_000 || carryingUnits > 1_000_000 {
			t.Skip()
		}
		rate := decimal.NewFromInt(rateUnits).Div(decimal.NewFromInt(100))
		balance, err := types.NewMoney(decimal.NewFromInt(foreignBalance).Div(decimal.NewFromInt(100)), "USD")
		if err != nil {
			t.Skip()
		}
		converted, err := Convert(balance, "SAR", rate)
		if err != nil {
			t.Skip()
		}
		carrying, err := types.NewMoney(decimal.NewFromInt(carryingUnits).Div(decimal.NewFromInt(100)), "SAR")
		if err != nil {
			t.Skip()
		}
		if converted.Currency() != carrying.Currency() {
			t.Fatalf("converted currency %s, carrying currency %s", converted.Currency(), carrying.Currency())
		}
	})
}

func TestExchangeRateValidationEnforcesMoney025Money026AndSpecialCurrencies(t *testing.T) {
	date := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	valid12 := ExchangeRate{
		FromCurrency:  "EUR",
		ToCurrency:    "XCG",
		Rate:          decimal.RequireFromString("1.123456789012"),
		EffectiveDate: date,
		Type:          SpotRate,
	}
	if err := valid12.Validate(); err != nil {
		t.Fatalf("valid 12-scale rate with XCG rejected: %v", err)
	}

	overScale := valid12
	overScale.Rate = decimal.RequireFromString("1.1234567890123")
	if err := overScale.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("13-scale rate error = %v, want ErrInvalidRate", err)
	}

	overDigits := valid12
	overDigits.Rate = decimal.RequireFromString("10000000000000000000000000000")
	if err := overDigits.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("29-digit rate error = %v, want ErrInvalidRate", err)
	}

	for _, special := range []string{"XXX", "XTS", "XAU", "XAG", "XPT", "XPD", "XDR"} {
		badCurr := valid12
		badCurr.FromCurrency = special
		if err := badCurr.Validate(); !errors.Is(err, ErrInvalidCurrency) {
			t.Fatalf("special FromCurrency %s error = %v, want ErrInvalidCurrency", special, err)
		}
	}
}
