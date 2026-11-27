package types

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// TestMonetaryPrecisionContractConstants pins the numeric contract used by every
// monetary path (RULE-KERNEL-005 and RULE-KERNEL-006).
func TestMonetaryPrecisionContractConstants(t *testing.T) {
	if FinancialScale != 4 || InternalScale != 12 {
		t.Fatalf("scales = (%d, %d), want (4, 12)", FinancialScale, InternalScale)
	}
	if MaximumSignificantDigits != 28 || MaximumIntegerDigits != 28 || MaximumRepresentationalScale != 28 {
		t.Fatalf("precision limits = (%d, %d, %d), want (28, 28, 28)",
			MaximumSignificantDigits, MaximumIntegerDigits, MaximumRepresentationalScale)
	}
	if !(FinancialScale <= InternalScale && InternalScale <= MaximumRepresentationalScale) {
		t.Fatal("precision contract must be ordered: display <= internal <= representational")
	}
}

// TestNewMoneyNormalizesCurrencyAndRejectsXXX covers RULE-KERNEL-004 for both
// constructors and proves that normalization is deterministic for case/space.
func TestNewMoneyNormalizesCurrencyAndRejectsXXX(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{input: " usd ", want: "USD"},
		{input: "jpy", want: "JPY"},
		{input: "\tkwd\n", want: "KWD"},
	} {
		money, err := NewMoneyFromString("1", tc.input)
		if err != nil {
			t.Fatalf("NewMoneyFromString(1, %q): %v", tc.input, err)
		}
		if money.Currency() != tc.want {
			t.Errorf("currency for %q = %q, want %q", tc.input, money.Currency(), tc.want)
		}
	}
	for _, invalid := range []string{"XXX", "xxx", " zzz ", ""} {
		if _, err := NewMoneyFromString("1", invalid); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("NewMoneyFromString(1, %q) error = %v, want ErrInvalidCurrency", invalid, err)
		}
	}
}

// TestNewMoneyPrecisionBoundaryMatrix is the executable form of the monetary
// precision matrix: significant digits, integer digits, representational scale
// and textual validity are independent gates.
func TestNewMoneyPrecisionBoundaryMatrix(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr error
	}{
		{name: "28 significant digits", value: strings.Repeat("9", 28)},
		{name: "29 significant digits", value: strings.Repeat("9", 29), wantErr: ErrAmountExceedsPrecision},
		{name: "exponent at integer boundary", value: "1e27"},
		{name: "exponent beyond integer boundary", value: "1e28", wantErr: ErrAmountExceedsPrecision},
		{name: "maximum representational scale", value: "0." + strings.Repeat("0", 27) + "1"},
		{name: "scale beyond limit", value: "1e-29", wantErr: ErrAmountExceedsPrecision},
		{name: "trailing zeros are not significant", value: "1." + strings.Repeat("0", 27)},
		{name: "trailing zeros still consume scale", value: "1." + strings.Repeat("0", 29), wantErr: ErrAmountExceedsPrecision},
		{name: "malformed decimal", value: "1.2.3", wantErr: ErrInvalidAmount},
		{name: "whitespace only", value: "   ", wantErr: ErrInvalidAmount},
		{name: "hexadecimal text", value: "0x10", wantErr: ErrInvalidAmount},
		{name: "unicode digits", value: "١٢٣", wantErr: ErrInvalidAmount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			money, err := NewMoneyFromString(tc.value, "USD")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if _, err := NewMoney(money.Amount(), money.Currency()); err != nil {
				t.Fatalf("accepted amount must revalidate: %v", err)
			}
		})
	}
}

// TestMinorUnitsContractAcrossRegistry exercises every registered currency:
// 0, 2, 3 and 4 minor-unit currencies are covered by the table itself, and
// XXX is excluded because it is not a monetary currency (RULE-KERNEL-004/007).
func TestMinorUnitsContractAcrossRegistry(t *testing.T) {
	for _, currency := range AllCurrencies() {
		code := string(currency.Code)
		if currency.IsSpecial() {
			if _, err := NewMoneyFromString("1", code); !errors.Is(err, ErrInvalidCurrency) {
				t.Fatalf("special currency %s unexpectedly accepted: %v", code, err)
			}
			continue
		}
		minor := currency.MinorUnit
		if minor < 0 || minor > 4 {
			t.Fatalf("currency %s has unsupported minor unit %d", code, minor)
		}

		exactText := "1"
		if minor > 0 {
			exactText += "." + strings.Repeat("0", int(minor))
		}
		exact := mustMoney(t, exactText, code)
		if exact.HasSubMinorUnitFraction() {
			t.Fatalf("%s: exact minor-unit amount %s flagged as fractional", code, exactText)
		}
		wantUnits := int64(1)
		for range minor {
			wantUnits *= 10
		}
		units, err := exact.MinorUnitsExact()
		if err != nil || units != wantUnits {
			t.Fatalf("%s: MinorUnitsExact(%s) = %d, %v; want %d", code, exactText, units, err, wantUnits)
		}
		explicitUnits, err := exact.MinorUnits()
		if err != nil || explicitUnits != wantUnits {
			t.Fatalf("%s: MinorUnits(%s) = %d, %v; want %d", code, exactText, explicitUnits, err, wantUnits)
		}
		rounded, err := exact.Rounded()
		if err != nil || !rounded.Equal(exact) {
			t.Fatalf("%s: Rounded(%s) = %s, %v; want unchanged", code, exactText, rounded.ExactString(), err)
		}
		residual, err := exact.RoundingDifference()
		if err != nil || !residual.Amount().IsZero() {
			t.Fatalf("%s: residual of exact amount = %s, %v; want zero", code, residual.ExactString(), err)
		}

		halfText := "1.5"
		if minor > 0 {
			halfText = "1." + strings.Repeat("0", int(minor)) + "5"
		}
		half := mustMoney(t, halfText, code)
		if !half.HasSubMinorUnitFraction() {
			t.Fatalf("%s: %s is a sub-minor-unit fraction", code, halfText)
		}
		if _, err := half.MinorUnitsExact(); !errors.Is(err, ErrSubMinorUnitAmount) {
			t.Fatalf("%s: MinorUnitsExact(%s) error = %v, want ErrSubMinorUnitAmount", code, halfText, err)
		}
		bankUnits, err := half.MinorUnits()
		if err != nil {
			t.Fatalf("%s: MinorUnits(%s): %v", code, halfText, err)
		}
		wantBankUnits := RoundBanker(half.Amount(), minor).Shift(minor).IntPart()
		if bankUnits != wantBankUnits {
			t.Errorf("%s: MinorUnits(%s) = %d, want banker's result %d", code, halfText, bankUnits, wantBankUnits)
		}
		halfResidual, err := half.RoundingDifference()
		if err != nil {
			t.Fatalf("%s: RoundingDifference(%s): %v", code, halfText, err)
		}
		halfUnit := decimal.New(5, -minor-1)
		if halfResidual.Amount().Abs().GreaterThan(halfUnit) {
			t.Errorf("%s: residual %s exceeds half minor unit %s", code, halfResidual.Amount(), halfUnit)
		}
	}
}

// TestBankersRoundingTieToEvenProperty proves half-to-even for every supported
// minor-unit scale without using floating point. The coefficient k*10+5 at
// exponent -(scale+1) is exactly the midpoint between k and k+1 minor units.
func TestBankersRoundingTieToEvenProperty(t *testing.T) {
	representatives := map[int32]string{0: "JPY", 2: "USD", 3: "KWD", 4: "CLF"}
	for scale := int32(0); scale <= 4; scale++ {
		for k := int64(-20); k <= 20; k++ {
			midpoint := decimal.New(k*10+5, -scale-1)
			got := RoundBanker(midpoint, scale)
			wantUnits := k
			if k%2 != 0 {
				wantUnits = k + 1
			}
			want := decimal.New(wantUnits, -scale)
			if !got.Equal(want) {
				t.Errorf("RoundBanker(%s, %d) = %s, want %s", midpoint, scale, got, want)
			}
			if again := RoundBanker(got, scale); !again.Equal(got) {
				t.Errorf("banker's rounding is not idempotent: %s -> %s", got, again)
			}

			code, ok := representatives[scale]
			if !ok {
				continue
			}
			money, err := NewMoney(midpoint, code)
			if err != nil {
				t.Fatalf("NewMoney(%s, %s): %v", midpoint, code, err)
			}
			rounded, err := money.Rounded()
			if err != nil || !rounded.Amount().Equal(want) {
				t.Errorf("Rounded(%s %s) = %s, %v; want %s", midpoint, code, rounded.Amount(), err, want)
			}
		}
	}
}

// TestSequentialOperationsAreDeterministicAndBounded covers accumulation of
// rounding residuals, exact internal precision and order independence.
func TestSequentialOperationsAreDeterministicAndBounded(t *testing.T) {
	installment, err := mustMoney(t, "1000", "USD").Divide(decimal.NewFromInt(360))
	if err != nil {
		t.Fatal(err)
	}
	total, err := Zero("USD")
	if err != nil {
		t.Fatal(err)
	}
	for range 360 {
		total, err = total.Add(installment)
		if err != nil {
			t.Fatal(err)
		}
	}
	wantTotal, err := NewMoney(installment.Amount().Mul(decimal.NewFromInt(360)), "USD")
	if err != nil {
		t.Fatal(err)
	}
	if !total.Equal(wantTotal) {
		t.Fatalf("sum of 360 installments = %s, want %s", total.ExactString(), wantTotal.ExactString())
	}
	roundedTotal, err := total.Rounded()
	if err != nil {
		t.Fatal(err)
	}
	if residual := total.Amount().Sub(roundedTotal.Amount()).Abs(); residual.GreaterThan(decimal.New(5, -3)) {
		t.Fatalf("residual %s exceeds half a cent at InternalScale=%d", residual, InternalScale)
	}

	postedSum, err := Zero("USD")
	if err != nil {
		t.Fatal(err)
	}
	residualSum := decimal.Zero
	for range 360 {
		posted, err := installment.Rounded()
		if err != nil {
			t.Fatal(err)
		}
		residual, err := installment.RoundingDifference()
		if err != nil {
			t.Fatal(err)
		}
		postedSum, err = postedSum.Add(posted)
		if err != nil {
			t.Fatal(err)
		}
		residualSum = residualSum.Add(residual.Amount())
	}
	if !residualSum.Equal(total.Amount().Sub(postedSum.Amount())) {
		t.Fatalf("sum of residuals %s does not reconcile total %s and posted sum %s",
			residualSum, total.Amount(), postedSum.Amount())
	}

	values := make([]Money, 0, 64)
	for i := 1; i <= 64; i++ {
		values = append(values, mustMoney(t, fmt.Sprintf("%d.%02d", i, (i*37)%100), "USD"))
	}
	permuted := append([]Money(nil), values...)
	sort.Slice(permuted, func(i, j int) bool { return permuted[i].ExactString() < permuted[j].ExactString() })
	sum := func(items []Money) Money {
		result, err := Zero("USD")
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			result, err = result.Add(item)
			if err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	forward := sum(values)
	if !forward.Equal(sum(permuted)) {
		t.Fatal("sum depends on input order")
	}
}

// TestMoneyOperationsPreserveCurrency pins RULE-KERNEL-002 at the value layer:
// arithmetic fails closed, comparisons never order different currencies.
func TestMoneyOperationsPreserveCurrency(t *testing.T) {
	usd := mustMoney(t, "10.005", "USD")
	eur := mustMoney(t, "10.005", "EUR")
	if _, err := usd.Add(eur); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add currency mismatch error = %v", err)
	}
	if _, err := usd.Subtract(eur); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Subtract currency mismatch error = %v", err)
	}
	if usd.Equal(eur) || usd.GreaterThan(eur) || usd.LessThan(eur) {
		t.Error("cross-currency comparison must never report equality or ordering")
	}
	commutative, err := usd.Add(mustMoney(t, "0.995", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := mustMoney(t, "0.995", "USD").Add(usd)
	if err != nil || !commutative.Equal(other) {
		t.Fatalf("addition is not commutative: %s vs %s (%v)", commutative.ExactString(), other.ExactString(), err)
	}
	derived := []struct {
		name  string
		value Money
	}{
		{name: "rounded", value: mustRounded(t, usd)},
		{name: "residual", value: mustResidual(t, usd)},
		{name: "multiplied", value: mustScaled(t, usd, decimal.NewFromInt(2))},
		{name: "divided", value: mustScaled(t, usd, decimal.NewFromInt(4), true)},
	}
	for _, item := range derived {
		if item.value.Currency() != "USD" {
			t.Errorf("%s changed currency to %q", item.name, item.value.Currency())
		}
	}
}

func mustRounded(t *testing.T, m Money) Money {
	t.Helper()
	result, err := m.Rounded()
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustResidual(t *testing.T, m Money) Money {
	t.Helper()
	result, err := m.RoundingDifference()
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustScaled(t *testing.T, m Money, scalar decimal.Decimal, divide ...bool) Money {
	t.Helper()
	if len(divide) > 0 && divide[0] {
		result, err := m.Divide(scalar)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	result, err := m.Multiply(scalar)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestMoneyValueImmutability proves that no mutable internal reference escapes
// through Amount(), and that operands are never modified by arithmetic.
func TestMoneyValueImmutability(t *testing.T) {
	money := mustMoney(t, "12.34", "USD")
	before := money.ExactString()

	coefficient := money.Amount().Coefficient()
	coefficient.SetInt64(999)
	big := money.Amount().BigInt()
	big.SetInt64(1)
	if money.ExactString() != before {
		t.Fatalf("Money internals mutated: %s, want %s", money.ExactString(), before)
	}

	other := mustMoney(t, "0.66", "USD")
	if _, err := money.Add(other); err != nil {
		t.Fatal(err)
	}
	if _, err := money.Subtract(other); err != nil {
		t.Fatal(err)
	}
	if money.ExactString() != before || other.ExactString() != "0.66 USD" {
		t.Fatal("arithmetic mutated its operands")
	}
}
