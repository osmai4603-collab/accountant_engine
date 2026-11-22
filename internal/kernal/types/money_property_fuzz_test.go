package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// FuzzMinorUnitsExactNeverDropsFraction proves that the strict path fails
// closed while the explicit rounding path is deterministic and bounded.
func FuzzMinorUnitsExactNeverDropsFraction(f *testing.F) {
	for _, seed := range []struct{ amount, currency string }{
		{amount: "1.005", currency: "USD"},
		{amount: "2.505", currency: "KWD"},
		{amount: "1.5", currency: "JPY"},
		{amount: "1.00005", currency: "CLF"},
		{amount: "92233720368547758.07", currency: "USD"},
		{amount: "92233720368547758.08", currency: "USD"},
	} {
		f.Add(seed.amount, seed.currency)
	}
	f.Fuzz(func(t *testing.T, amount, currency string) {
		money, err := NewMoneyFromString(amount, currency)
		if err != nil {
			return
		}
		info, ok := LookupCurrency(money.Currency())
		if !ok || info.Code == XXX {
			return
		}

		exact, exactErr := money.MinorUnitsExact()
		explicit, explicitErr := money.MinorUnits()
		rounded, roundedErr := money.Rounded()
		if roundedErr != nil {
			t.Fatalf("Rounded(%s): %v", money.ExactString(), roundedErr)
		}

		switch {
		case exactErr == nil:
			if explicitErr != nil || exact != explicit {
				t.Fatalf("exact %d and explicit %d (%v) disagree for %s", exact, explicit, explicitErr, money.ExactString())
			}
			if money.HasSubMinorUnitFraction() {
				t.Fatalf("%s has a sub-minor fraction but MinorUnitsExact succeeded", money.ExactString())
			}
			if !rounded.Equal(money) {
				t.Fatalf("Rounded(%s) = %s; exact amount must be unchanged", money.ExactString(), rounded.ExactString())
			}
		case errors.Is(exactErr, ErrSubMinorUnitAmount):
			if !money.HasSubMinorUnitFraction() {
				t.Fatalf("MinorUnitsExact reported a dropped fraction for %s", money.ExactString())
			}
			if explicitErr != nil {
				t.Fatalf("explicit MinorUnits(%s): %v", money.ExactString(), explicitErr)
			}
			want := RoundBanker(money.Amount(), info.MinorUnit).Shift(info.MinorUnit).IntPart()
			if explicit != want {
				t.Fatalf("MinorUnits(%s) = %d, want %d", money.ExactString(), explicit, want)
			}
			residual, err := money.RoundingDifference()
			if err != nil {
				t.Fatalf("RoundingDifference(%s): %v", money.ExactString(), err)
			}
			if !money.Amount().Sub(residual.Amount()).Equal(rounded.Amount()) {
				t.Fatalf("residual does not reconcile %s with %s", money.ExactString(), rounded.ExactString())
			}
		case errors.Is(exactErr, ErrAmountExceedsPrecision):
			// Overflow is reported by both paths; no further equality is claimed.
		default:
			t.Fatalf("unexpected MinorUnitsExact error for %s: %v", money.ExactString(), exactErr)
		}
	})
}

// FuzzMoneyOperationsPreserveCurrencyAndDeterminism covers RULE-KERNEL-002:
// no mixed-currency arithmetic, no cross-currency ordering, no order effects.
func FuzzMoneyOperationsPreserveCurrencyAndDeterminism(f *testing.F) {
	for _, seed := range []struct {
		left, leftCurrency, right, rightCurrency string
	}{
		{left: "10.005", leftCurrency: "USD", right: "0.995", rightCurrency: "USD"},
		{left: "10.005", leftCurrency: "USD", right: "10.005", rightCurrency: "EUR"},
		{left: "-0.004", leftCurrency: "KWD", right: "2.505", rightCurrency: "KWD"},
		{left: "1e27", leftCurrency: "JPY", right: "1", rightCurrency: "JPY"},
	} {
		f.Add(seed.left, seed.leftCurrency, seed.right, seed.rightCurrency)
	}
	f.Fuzz(func(t *testing.T, leftText, leftCurrency, rightText, rightCurrency string) {
		left, leftErr := NewMoneyFromString(leftText, leftCurrency)
		right, rightErr := NewMoneyFromString(rightText, rightCurrency)
		if leftErr != nil || rightErr != nil {
			return
		}
		if left.Currency() != right.Currency() {
			if _, err := left.Add(right); !errors.Is(err, ErrCurrencyMismatch) {
				t.Fatalf("mixed-currency Add error = %v", err)
			}
			if _, err := left.Subtract(right); !errors.Is(err, ErrCurrencyMismatch) {
				t.Fatalf("mixed-currency Subtract error = %v", err)
			}
			if left.Equal(right) || left.GreaterThan(right) || left.LessThan(right) {
				t.Fatal("mixed currencies must not be ordered")
			}
			return
		}
		if left.Equal(right) != right.Equal(left) {
			t.Fatal("equality is not symmetric")
		}
		forward, err := left.Add(right)
		if err != nil {
			return // combined value is outside the precision contract
		}
		backward, err := right.Add(left)
		if err != nil || !forward.Equal(backward) {
			t.Fatalf("addition is not commutative: %s vs %s (%v)", forward.ExactString(), backward.ExactString(), err)
		}
		if forward.Currency() != left.Currency() {
			t.Fatalf("addition changed currency to %q", forward.Currency())
		}
	})
}

// FuzzMoneyConstructorPrecisionAndRoundTrip proves that every accepted value
// is reproducible through decimal, textual and JSON representations.
func FuzzMoneyConstructorPrecisionAndRoundTrip(f *testing.F) {
	for _, seed := range []struct{ amount, currency string }{
		{amount: "0", currency: "USD"},
		{amount: "1e27", currency: "USD"},
		{amount: "0.0000000000000000000000000001", currency: "SAR"},
		{amount: "1e-29", currency: "USD"},
		{amount: "-1.005", currency: "KWD"},
		{amount: " 42 ", currency: "jpy"},
	} {
		f.Add(seed.amount, seed.currency)
	}
	f.Fuzz(func(t *testing.T, amount, currency string) {
		money, err := NewMoneyFromString(amount, currency)
		if err != nil {
			return
		}
		fromDecimal, err := NewMoney(money.Amount(), money.Currency())
		if err != nil || !fromDecimal.Equal(money) {
			t.Fatalf("decimal round trip failed: %s, %v", fromDecimal.ExactString(), err)
		}
		fromText, err := NewMoneyFromString(money.Amount().String(), money.Currency())
		if err != nil || !fromText.Equal(money) {
			t.Fatalf("textual round trip failed: %s, %v", fromText.ExactString(), err)
		}
		encoded, err := json.Marshal(money)
		if err != nil {
			t.Fatalf("MarshalJSON(%s): %v", money.ExactString(), err)
		}
		var decoded Money
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", encoded, err)
		}
		if !decoded.Equal(money) {
			t.Fatalf("JSON round trip changed %s to %s", money.ExactString(), decoded.ExactString())
		}
	})
}

// FuzzBankersRoundingIsDeterministicAndBounded checks the residual bound and
// idempotence of the single supported monetary rounding mode.
func FuzzBankersRoundingIsDeterministicAndBounded(f *testing.F) {
	for _, seed := range []struct{ value, scale string }{
		{value: "2.5", scale: ""},
		{value: "10.005", scale: "xx"},
		{value: "-3.5", scale: "xxx"},
		{value: "1.00005", scale: "xxxx"},
		{value: "0", scale: "xxxxx"},
	} {
		f.Add(seed.value, seed.scale)
	}
	f.Fuzz(func(t *testing.T, value, scaleSeed string) {
		if len(value) > 64 {
			return
		}
		amount, err := decimal.NewFromString(strings.TrimSpace(value))
		if err != nil {
			return
		}
		scale := int32(len(scaleSeed) % 6)
		rounded := RoundBanker(amount, scale)
		halfUnit := decimal.New(5, -scale-1)
		if residual := amount.Sub(rounded).Abs(); residual.GreaterThan(halfUnit) {
			t.Fatalf("residual %s for %s at scale %d exceeds %s", residual, amount, scale, halfUnit)
		}
		if again := RoundBanker(rounded, scale); !again.Equal(rounded) {
			t.Fatalf("rounding is not idempotent: %s -> %s", rounded, again)
		}
	})
}
