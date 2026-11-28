package types

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestCoverageCurrencyClassification(t *testing.T) {
	for _, code := range []CurrencyCode{XXX, XTS, XDR, XAU, XAG, XPT, XPD} {
		if !(Currency{Code: code}).IsSpecial() {
			t.Fatalf("%s should be special", code)
		}
	}
	if (Currency{Code: USD}).IsSpecial() {
		t.Fatal("USD must not be special")
	}
	if !(Currency{Code: ANG}).IsHistoric() || (Currency{Code: XCG}).IsHistoric() {
		t.Fatal("historic currency classification is wrong")
	}
	if !(Currency{Code: USD}).IsSpendable() || (Currency{Code: XXX}).IsSpendable() ||
		(Currency{}).IsSpendable() {
		t.Fatal("spendable classification is wrong")
	}
}

func TestCoverageCurrencyLookups(t *testing.T) {
	currency, ok := LookupCurrency("  usd ")
	if !ok || currency.Code != USD || currency.NumericCode != "840" || currency.MinorUnit != 2 {
		t.Fatalf("LookupCurrency = %+v, %v", currency, ok)
	}
	if _, ok := LookupCurrency("ZZZ"); ok {
		t.Fatal("unknown currency was found")
	}

	byNumeric, ok := LookupCurrencyByNumericCode(" 784 ")
	if !ok || byNumeric.Code != AED {
		t.Fatalf("LookupCurrencyByNumericCode = %+v, %v", byNumeric, ok)
	}
	sharedNumeric, ok := LookupCurrencyByNumericCode("532")
	if !ok || sharedNumeric.Code != XCG {
		t.Fatalf("historic currency should not win: %+v, %v", sharedNumeric, ok)
	}
	if _, ok := LookupCurrencyByNumericCode("999-invalid"); ok {
		t.Fatal("unknown numeric code was found")
	}

	all := AllCurrencies()
	if len(all) == 0 {
		t.Fatal("currency table is empty")
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Code >= all[i].Code {
			t.Fatalf("currency table is not sorted at %d", i)
		}
	}
}

func TestCoverageCurrencyNormalization(t *testing.T) {
	normalized, err := normalizeCurrency(" eur ")
	if err != nil || normalized != "EUR" {
		t.Fatalf("normalizeCurrency = %q, %v", normalized, err)
	}
	if _, err := normalizeCurrency("not-a-currency"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("normalizeCurrency error = %v", err)
	}
}

func TestCoveragePublicBridgeConstructors(t *testing.T) {
	companyID := NewCompanyID()
	accountID := NewAccountID()
	yearID := NewFiscalYearID()
	periodID := NewFiscalPeriodID()
	journalID := NewJournalID()
	entryID := NewJournalEntryID()
	lineID := NewJournalEntryLineID()
	partnerID := NewPartnerID()
	costCenterID := NewCostCenterID()
	projectID := NewProjectID()
	segmentID := NewSegmentID()
	businessUnitID := NewBusinessUnitID()
	if companyID == (CompanyID{}) || accountID == (AccountID{}) || yearID == (FiscalYearID{}) ||
		periodID == (FiscalPeriodID{}) || journalID == (JournalID{}) || entryID == (JournalEntryID{}) ||
		lineID == (JournalEntryLineID{}) || partnerID == (PartnerID{}) ||
		costCenterID == (CostCenterID{}) || projectID == (ProjectID{}) ||
		segmentID == (SegmentID{}) || businessUnitID == (BusinessUnitID{}) {
		t.Fatal("public ID constructors produced zero values")
	}

	money, err := NewMoney(decimal.RequireFromString("12.34"), "USD")
	if err != nil || money.Amount().IsZero() {
		t.Fatalf("NewMoney = %+v, %v", money, err)
	}
	fromDecimal, err := NewMoneyFromDecimal(decimal.RequireFromString("1.00"), "EUR")
	if err != nil || fromDecimal.Currency() != "EUR" {
		t.Fatalf("NewMoneyFromDecimal = %+v, %v", fromDecimal, err)
	}
	fromString, err := NewMoneyFromString(" 5.00 ", "USD")
	if err != nil || !fromString.Amount().Equal(decimal.RequireFromString("5")) {
		t.Fatalf("NewMoneyFromString = %+v, %v", fromString, err)
	}
	zero, err := Zero("USD")
	if err != nil || !zero.Amount().IsZero() {
		t.Fatalf("Zero = %+v, %v", zero, err)
	}
	if minor := CurrencyMinorUnit(string(USD)); minor != 2 {
		t.Fatalf("CurrencyMinorUnit(USD) = %d", minor)
	}
	if FinancialScale <= 0 || InternalScale < FinancialScale {
		t.Fatalf("unexpected scales: %d %d", FinancialScale, InternalScale)
	}
}
