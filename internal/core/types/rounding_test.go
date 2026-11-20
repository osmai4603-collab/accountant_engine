package types

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestBankersRoundingEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		scale    int32
		expected string
		desc     string
	}{
		// Half to even: 2.5 -> 2, 3.5 -> 4
		{"2.5", 0, "2", "Round 2.5 to 0 decimals -> even 2"},
		{"3.5", 0, "4", "Round 3.5 to 0 decimals -> even 4"},
		{"-2.5", 0, "-2", "Round -2.5 to 0 decimals -> even -2"},
		{"-3.5", 0, "-4", "Round -3.5 to 0 decimals -> even -4"},

		// Cents rounding (scale 2)
		{"10.005", 2, "10", "10.005 rounds to 10.00 (0 is even)"},
		{"10.015", 2, "10.02", "10.015 rounds to 10.02 (2 is even)"},
		{"10.025", 2, "10.02", "10.025 rounds to 10.02 (2 is even)"},
		{"10.035", 2, "10.04", "10.035 rounds to 10.04 (4 is even)"},
		{"10.045", 2, "10.04", "10.045 rounds to 10.04 (4 is even)"},
		{"10.055", 2, "10.06", "10.055 rounds to 10.06 (6 is even)"},
		{"10.065", 2, "10.06", "10.065 rounds to 10.06 (6 is even)"},
		{"10.075", 2, "10.08", "10.075 rounds to 10.08 (8 is even)"},
		{"10.085", 2, "10.08", "10.085 rounds to 10.08 (8 is even)"},
		{"10.095", 2, "10.1", "10.095 rounds to 10.10 (10 is even)"},

		// Fils (scale 3 - KWD, BHD, OMR)
		{"1.0005", 3, "1", "1.0005 rounds to 1.000"},
		{"1.0015", 3, "1.002", "1.0015 rounds to 1.002"},
		{"1.0025", 3, "1.002", "1.0025 rounds to 1.002"},
		{"1.0035", 3, "1.004", "1.0035 rounds to 1.004"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			d, err := decimal.NewFromString(tt.input)
			if err != nil {
				t.Fatalf("decimal.NewFromString(%q) error = %v", tt.input, err)
			}
			res := RoundBanker(d, tt.scale)
			exp, err := decimal.NewFromString(tt.expected)
			if err != nil {
				t.Fatalf("decimal.NewFromString(%q) error = %v", tt.expected, err)
			}
			if !res.Equal(exp) {
				t.Errorf("RoundBanker(%s, %d) = %s; want %s", tt.input, tt.scale, res.String(), exp.String())
			}
		})
	}
}

func TestRoundMoney(t *testing.T) {
	usdMoney, err := NewMoneyFromString("10.015", "USD")
	if err != nil {
		t.Fatalf("NewMoneyFromString error = %v", err)
	}

	rounded, err := RoundMoney(usdMoney)
	if err != nil {
		t.Fatalf("RoundMoney error = %v", err)
	}

	expected, _ := NewMoneyFromString("10.02", "USD")
	if !rounded.Equal(expected) {
		t.Errorf("RoundMoney(10.015 USD) = %v; want %v", rounded, expected)
	}

	// Test 3 decimal places currency (KWD)
	kwdMoney, err := NewMoneyFromString("10.0015", "KWD")
	if err != nil {
		t.Fatalf("NewMoneyFromString error = %v", err)
	}
	roundedKWD, err := RoundMoney(kwdMoney)
	if err != nil {
		t.Fatalf("RoundMoney error = %v", err)
	}
	expectedKWD, _ := NewMoneyFromString("10.002", "KWD")
	if !roundedKWD.Equal(expectedKWD) {
		t.Errorf("RoundMoney(10.0015 KWD) = %v; want %v", roundedKWD, expectedKWD)
	}
}

func TestAdditionalCoverage(t *testing.T) {
	all := AllCurrencies()
	if len(all) == 0 {
		t.Fatal("expected currencies list")
	}

	m1, err := NewMoneyFromDecimal(decimal.NewFromInt(100), "USD")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Zero("USD")
	if err != nil {
		t.Fatal(err)
	}

	sub, err := m1.Subtract(m2)
	if err != nil || !sub.Equal(m1) {
		t.Fatalf("Subtract failed: %v", err)
	}

	mul, err := m1.Multiply(decimal.NewFromInt(2))
	if err != nil || mul.Amount().IntPart() != 200 {
		t.Fatalf("Multiply failed: %v", err)
	}

	_, err = m1.Divide(decimal.Zero)
	if err == nil {
		t.Fatal("expected error on division by zero")
	}

	unit := CurrencyMinorUnit("USD")
	if unit != 2 {
		t.Fatalf("expected minor unit 2, got %d", unit)
	}
	unknownUnit := CurrencyMinorUnit("NONEXISTENT")
	if unknownUnit != FinancialScale {
		t.Fatalf("expected fallback scale %d, got %d", FinancialScale, unknownUnit)
	}

	invalidMoney := Money{amount: decimal.NewFromInt(10), currency: "INVALID"}
	_, err = invalidMoney.Rounded()
	if err == nil {
		t.Fatal("expected error rounding invalid currency")
	}

	_, err = invalidMoney.MinorUnits()
	if err == nil {
		t.Fatal("expected error MinorUnits invalid currency")
	}

	_, err = invalidMoney.MinorUnitsExact()
	if err == nil {
		t.Fatal("expected error MinorUnitsExact invalid currency")
	}

	_, err = invalidMoney.RoundingDifference()
	if err == nil {
		t.Fatal("expected error RoundingDifference invalid currency")
	}

	postRounded := invalidMoney.RoundForPosting()
	if postRounded.Currency() != "INVALID" {
		t.Fatalf("expected INVALID currency in RoundForPosting fallback")
	}

	_, ok := LookupCurrencyByNumericCode("999999")
	if ok {
		t.Fatal("expected false on invalid numeric code")
	}
}


func TestCoverageBoostTo95(t *testing.T) {
	// subtract different currency error
	mUSD, _ := NewMoneyFromString("10.00", "USD")
	mEUR, _ := NewMoneyFromString("10.00", "EUR")
	_, err := mUSD.Subtract(mEUR)
	if err == nil {
		t.Fatal("expected currency mismatch")
	}

	// newMoney error via invalid currency in NewMoney
	_, err = NewMoney(decimal.NewFromInt(10), "NOT_A_VALID_CURRENCY")
	if err == nil {
		t.Fatal("expected error for invalid currency in NewMoney")
	}

	// newMoneyAtScale precision exceed
	hugeDecimal, _ := decimal.NewFromString("9999999999999999999999999999999999999999")
	_, err = newMoneyAtScale(hugeDecimal, "USD")
	if err == nil {
		t.Fatal("expected ErrAmountExceedsPrecision")
	}
}
