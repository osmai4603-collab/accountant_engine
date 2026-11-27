package types

import (
	"errors"
	"testing"
)

// TestCurrencyRegistryIsDeterministicAndIsolated covers RULE-KERNEL-004 and
// the "no mutable internal reference" rule for the currency registry.
func TestCurrencyRegistryIsDeterministicAndIsolated(t *testing.T) {
	all := AllCurrencies()
	if len(all) == 0 {
		t.Fatal("empty currency registry")
	}
	seenCodes := make(map[string]bool, len(all))
	seenNumeric := make(map[string]bool, len(all))
	for i, currency := range all {
		code := string(currency.Code)
		if i > 0 && all[i-1].Code >= currency.Code {
			t.Fatalf("AllCurrencies is not sorted at index %d", i)
		}
		if seenCodes[code] {
			t.Fatalf("duplicate currency code %s", code)
		}
		seenCodes[code] = true
		if currency.NumericCode == "" || (!currency.IsHistoric() && seenNumeric[currency.NumericCode]) {
			t.Fatalf("invalid or duplicate numeric code %q for %s", currency.NumericCode, code)
		}
		if !currency.IsHistoric() {
			seenNumeric[currency.NumericCode] = true
		}
		if currency.MinorUnit < 0 || currency.MinorUnit > 4 {
			t.Fatalf("currency %s has minor unit %d outside 0..4", code, currency.MinorUnit)
		}
	}
	if !seenCodes[string(XXX)] || !seenCodes[string(XCG)] {
		t.Fatal("XXX sentinel and XCG replacement currency must remain registered")
	}
	if got, ok := LookupCurrencyByNumericCode("532"); !ok || got.Code != XCG {
		t.Fatalf("LookupCurrencyByNumericCode(532) = (%+v, %v), want XCG", got, ok)
	}
	for _, special := range []CurrencyCode{XXX, XTS, XDR, XAU, XAG, XPT, XPD} {
		curr, ok := LookupCurrency(string(special))
		if !ok || !curr.IsSpecial() || curr.IsSpendable() {
			t.Fatalf("special code %s = (%+v, %v), want IsSpecial=true IsSpendable=false", special, curr, ok)
		}
	}

	all[0].MinorUnit = 99
	all[0].Code = "ZZZ"
	refetched := AllCurrencies()
	if string(refetched[0].Code) == "ZZZ" || refetched[0].MinorUnit == 99 {
		t.Fatal("AllCurrencies leaked a mutable reference to the registry")
	}

	for _, query := range []struct{ input, want string }{
		{input: " usd ", want: "USD"},
		{input: "jpy", want: "JPY"},
	} {
		currency, ok := LookupCurrency(query.input)
		if !ok || string(currency.Code) != query.want {
			t.Errorf("LookupCurrency(%q) = %v, %v; want %s", query.input, currency, ok, query.want)
		}
	}
	if _, ok := LookupCurrency("NOT_A_CURRENCY"); ok {
		t.Error("unknown currency was resolved")
	}
	for _, invalid := range []string{"XXX", "xxx", "NOT_A_CURRENCY", ""} {
		if _, err := CurrencyMinorUnitChecked(invalid); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("CurrencyMinorUnitChecked(%q) error = %v, want ErrInvalidCurrency", invalid, err)
		}
	}

	for _, numeric := range []string{"840", "392", "414", "990"} {
		first, ok := LookupCurrencyByNumericCode(numeric)
		if !ok {
			t.Fatalf("numeric code %s not found", numeric)
		}
		for range 25 {
			again, ok := LookupCurrencyByNumericCode(numeric)
			if !ok || again.Code != first.Code {
				t.Fatalf("numeric lookup for %s is not deterministic", numeric)
			}
		}
	}
}

// TestCurrencyMinorUnitClasses pins the 0/2/3 decimal classes named in the
// Phase 2 plan and the 4-decimal class actually present in the registry.
func TestCurrencyMinorUnitClasses(t *testing.T) {
	for _, tc := range []struct {
		code string
		want int32
	}{
		{code: "JPY", want: 0},
		{code: "USD", want: 2},
		{code: "SAR", want: 2},
		{code: "KWD", want: 3},
		{code: "BHD", want: 3},
		{code: "CLF", want: 4},
	} {
		checked, err := CurrencyMinorUnitChecked(tc.code)
		if err != nil || checked != tc.want {
			t.Errorf("CurrencyMinorUnitChecked(%s) = %d, %v; want %d", tc.code, checked, err, tc.want)
		}
		legacy := CurrencyMinorUnit(tc.code)
		if legacy != tc.want {
			t.Errorf("CurrencyMinorUnit(%s) = %d, want %d", tc.code, legacy, tc.want)
		}
		if _, err := NewMoneyFromString("1", tc.code); err != nil {
			t.Errorf("NewMoneyFromString(1, %s): %v", tc.code, err)
		}
	}
	if got := CurrencyMinorUnit("NOT_A_CURRENCY"); got != FinancialScale {
		t.Errorf("legacy compatibility fallback = %d, want FinancialScale %d", got, FinancialScale)
	}
}
