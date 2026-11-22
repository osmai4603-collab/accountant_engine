package valuation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
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
	if !result.RateDate.Equal(rateDate) {
		t.Fatalf("rate date = %s, want source effective date %s", result.RateDate, rateDate)
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

type rateTypeCaptureProvider struct {
	got RateType
}

func (p *rateTypeCaptureProvider) GetRate(
	_ context.Context,
	rateType RateType,
	_, _ string,
	_ time.Time,
) (decimal.Decimal, error) {
	p.got = rateType
	return decimal.NewFromInt(2), nil
}

func TestConvertForDatePassesRateType(t *testing.T) {
	provider := &rateTypeCaptureProvider{}
	amount, _ := types.NewMoneyFromString("10.00", "USD")
	if _, _, err := ConvertForDate(context.Background(), provider, amount, "SAR", time.Now(), AverageRate); err != nil {
		t.Fatalf("ConvertForDate: %v", err)
	}
	if provider.got != AverageRate {
		t.Fatalf("provider rate type = %s, want %s", provider.got, AverageRate)
	}
}

func TestSameCurrencyRateValidatesCodeAndDateBeforeReturningOne(t *testing.T) {
	registry := NewRateRegistry()
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	if _, _, _, err := registry.GetRate(context.Background(), SpotRate, "BAD", "BAD", date); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("invalid same-currency code error = %v, want invalid currency", err)
	}
	if err := (ExchangeRate{
		FromCurrency: "XXX", ToCurrency: "USD", Rate: decimal.NewFromInt(1),
		EffectiveDate: date, Type: SpotRate,
	}).Validate(); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("no-currency exchange rate error = %v, want invalid currency", err)
	}
	if _, _, _, err := registry.GetRate(context.Background(), SpotRate, "USD", "USD", time.Time{}); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("zero same-currency date error = %v, want invalid rate", err)
	}
	if rate, _, _, err := registry.GetRate(context.Background(), SpotRate, "USD", "USD", date); err != nil || !rate.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("valid same-currency rate = %s, %v; want 1, nil", rate, err)
	}
}

func TestConvertForDateRejectsInvalidQueryBeforeCallingProvider(t *testing.T) {
	provider := &rateTypeCaptureProvider{}
	amount, _ := types.NewMoneyFromString("10.00", "USD")
	if _, _, err := ConvertForDate(context.Background(), provider, amount, "USD", time.Time{}, SpotRate); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("zero-date conversion error = %v, want invalid rate", err)
	}
	if provider.got != "" {
		t.Fatalf("provider called for invalid query with rate type %s", provider.got)
	}
}

func TestRateRegistryRejectsMissingAndDuplicateEffectiveDates(t *testing.T) {
	registry := NewRateRegistry()
	rate := ExchangeRate{
		FromCurrency: "USD", ToCurrency: "SAR", Rate: decimal.RequireFromString("3.75"),
		Type: ClosingRate,
	}
	if err := registry.RegisterRate(rate); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("missing effective date error = %v, want invalid rate", err)
	}
	rate.EffectiveDate = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	if err := registry.RegisterRate(rate); err != nil {
		t.Fatalf("register initial rate: %v", err)
	}
	if err := registry.RegisterRate(rate); !errors.Is(err, ErrDuplicateRate) {
		t.Fatalf("duplicate effective date error = %v, want duplicate rate", err)
	}
	inverse := rate
	inverse.FromCurrency, inverse.ToCurrency = rate.ToCurrency, rate.FromCurrency
	inverse.Rate = decimal.RequireFromString("0.266666666667")
	if err := registry.RegisterRate(inverse); !errors.Is(err, ErrDuplicateRate) {
		t.Fatalf("inverse pair effective date error = %v, want duplicate rate", err)
	}
}

type fixedRateProvider struct {
	rate     decimal.Decimal
	ref      ExchangeRate
	inverted bool
}

func (p fixedRateProvider) GetRate(context.Context, RateType, string, string, time.Time) (decimal.Decimal, ExchangeRate, bool, error) {
	return p.rate, p.ref, p.inverted, nil
}

func TestRevaluationRejectsNonPositiveProviderRate(t *testing.T) {
	asOf := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	engine := NewRevaluationEngine(fixedRateProvider{
		rate: decimal.Zero,
		ref:  ExchangeRate{EffectiveDate: asOf},
	})
	carrying, _ := types.NewMoneyFromString("100.00", "SAR")
	_, err := engine.Calculate(context.Background(), MonetaryPosition{
		AccountID: types.NewAccountID(), ForeignCurrencyBalance: decimal.NewFromInt(10),
		TransactionCurrency: "USD", FunctionalCurrency: "SAR", CarryingAmount: carrying, Type: Monetary,
	}, asOf)
	if !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("non-positive provider rate error = %v, want invalid rate", err)
	}
}

func TestRevaluationValidatesRateReferenceAgainstRequest(t *testing.T) {
	asOf := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	position := MonetaryPosition{
		AccountID: types.NewAccountID(), ForeignCurrencyBalance: decimal.NewFromInt(10),
		TransactionCurrency: "USD", FunctionalCurrency: "SAR",
		CarryingAmount: mustValuationMoney(t, "37.50", "SAR"), Type: Monetary,
	}
	validReference := ExchangeRate{
		FromCurrency: "USD", ToCurrency: "SAR", Rate: decimal.RequireFromString("3.75"),
		EffectiveDate: asOf, Type: ClosingRate,
	}
	tests := []struct {
		name string
		rate decimal.Decimal
		ref  ExchangeRate
	}{
		{name: "source currency mismatch", rate: validReference.Rate, ref: ExchangeRate{
			FromCurrency: "EUR", ToCurrency: "SAR", Rate: validReference.Rate,
			EffectiveDate: asOf, Type: ClosingRate,
		}},
		{name: "destination currency mismatch", rate: validReference.Rate, ref: ExchangeRate{
			FromCurrency: "USD", ToCurrency: "EUR", Rate: validReference.Rate,
			EffectiveDate: asOf, Type: ClosingRate,
		}},
		{name: "wrong rate type", rate: validReference.Rate, ref: ExchangeRate{
			FromCurrency: "USD", ToCurrency: "SAR", Rate: validReference.Rate,
			EffectiveDate: asOf, Type: SpotRate,
		}},
		{name: "reference rate mismatch", rate: validReference.Rate, ref: ExchangeRate{
			FromCurrency: "USD", ToCurrency: "SAR", Rate: decimal.RequireFromString("3.74"),
			EffectiveDate: asOf, Type: ClosingRate,
		}},
		{name: "provider rate mismatch", rate: decimal.RequireFromString("3.74"), ref: validReference},
		{name: "future reference date", rate: validReference.Rate, ref: ExchangeRate{
			FromCurrency: "USD", ToCurrency: "SAR", Rate: validReference.Rate,
			EffectiveDate: asOf.Add(time.Hour), Type: ClosingRate,
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := NewRevaluationEngine(fixedRateProvider{rate: test.rate, ref: test.ref})
			if _, err := engine.Calculate(context.Background(), position, asOf); !errors.Is(err, ErrInvalidRate) {
				t.Fatalf("Calculate error = %v, want invalid rate", err)
			}
		})
	}

	engine := NewRevaluationEngine(fixedRateProvider{rate: validReference.Rate, ref: validReference})
	if _, err := engine.Calculate(context.Background(), position, asOf); err != nil {
		t.Fatalf("valid rate reference: %v", err)
	}
}

func TestRevaluationValidatesInvertedAndSameCurrencyRates(t *testing.T) {
	asOf := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	invertedReference := ExchangeRate{
		FromCurrency: "SAR", ToCurrency: "USD",
		Rate:          decimal.NewFromInt(1).DivRound(decimal.RequireFromString("3.75"), 12),
		EffectiveDate: asOf, Type: ClosingRate,
	}
	invertedRate := decimal.NewFromInt(1).DivRound(invertedReference.Rate, 12)
	position := MonetaryPosition{
		AccountID: types.NewAccountID(), ForeignCurrencyBalance: decimal.NewFromInt(10),
		TransactionCurrency: "USD", FunctionalCurrency: "SAR",
		CarryingAmount: mustValuationMoney(t, "37.50", "SAR"), Type: Monetary,
	}
	engine := NewRevaluationEngine(fixedRateProvider{
		rate: invertedRate, ref: invertedReference, inverted: true,
	})
	if _, err := engine.Calculate(context.Background(), position, asOf); err != nil {
		t.Fatalf("valid inverted rate reference: %v", err)
	}

	sameCurrencyPosition := MonetaryPosition{
		AccountID: types.NewAccountID(), ForeignCurrencyBalance: decimal.NewFromInt(10),
		TransactionCurrency: "USD", FunctionalCurrency: "USD",
		CarryingAmount: mustValuationMoney(t, "10.00", "USD"), Type: Monetary,
	}
	wrongSameCurrencyReference := ExchangeRate{
		FromCurrency: "USD", ToCurrency: "USD", Rate: decimal.NewFromInt(2),
		EffectiveDate: asOf, Type: ClosingRate,
	}
	engine = NewRevaluationEngine(fixedRateProvider{
		rate: decimal.NewFromInt(2), ref: wrongSameCurrencyReference,
	})
	if _, err := engine.Calculate(context.Background(), sameCurrencyPosition, asOf); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("same-currency non-unit rate error = %v, want invalid rate", err)
	}

	validSameCurrencyReference := wrongSameCurrencyReference
	validSameCurrencyReference.Rate = decimal.NewFromInt(1)
	engine = NewRevaluationEngine(fixedRateProvider{
		rate: decimal.NewFromInt(1), ref: validSameCurrencyReference,
	})
	if _, err := engine.Calculate(context.Background(), sameCurrencyPosition, asOf); err != nil {
		t.Fatalf("same-currency unit rate: %v", err)
	}
}

func TestRevaluationReferenceIsDeterministicAndRegisteredOnce(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 12, 31, 10, 30, 0, 0, time.UTC)
	accountID := types.NewAccountID()
	difference, _ := types.NewMoneyFromString("500.00", "SAR")
	result := FXResult{
		AccountID: accountID, Currency: "USD", Difference: difference, IsGain: true,
	}
	accounts := FXJournalAccounts{
		CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(), UnrealizedLossID: types.NewAccountID(),
	}

	first, err := GenerateRevaluationEntryAt(result, accounts, date, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRevaluationEntryAt(result, accounts, date, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReferenceID != second.ReferenceID {
		t.Fatalf("revaluation reference changed: %q -> %q", first.ReferenceID, second.ReferenceID)
	}
	if !strings.Contains(first.ReferenceID, accountID.String()) {
		t.Fatalf("reference %q does not identify account %s", first.ReferenceID, accountID)
	}

	registry := NewRevaluationRegistry()
	key, err := RevaluationOperationKey(accounts.CompanyID, accountID, result.Currency, date)
	if err != nil {
		t.Fatal(err)
	}
	registered, replay, err := registry.Register(key, first)
	if err != nil || replay {
		t.Fatalf("first registration = replay %v, err %v", replay, err)
	}
	replayed, replay, err := registry.Register(key, second)
	if err != nil || !replay {
		t.Fatalf("second registration = replay %v, err %v", replay, err)
	}
	if replayed.ReferenceID != registered.ReferenceID {
		t.Fatalf("replay returned a different entry: %q -> %q", registered.ReferenceID, replayed.ReferenceID)
	}
}

func TestRevaluationReverseKeepsReversalDateAndPeriod(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 12, 31, 10, 30, 0, 0, time.UTC)
	reversalDate := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	accountID := types.NewAccountID()
	difference, _ := types.NewMoneyFromString("500.00", "SAR")
	result := FXResult{
		AccountID: accountID, Currency: "USD", Difference: difference, IsGain: true,
	}
	accounts := FXJournalAccounts{
		CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(), UnrealizedLossID: types.NewAccountID(),
	}
	entry, err := GenerateRevaluationEntryAt(result, accounts, date, now)
	if err != nil {
		t.Fatal(err)
	}
	entry.Status = journal.EntryPosted

	reversal, err := BuildRevaluationReversalAt(entry, reversalDate, "system-valuation", "correct FX revaluation", now)
	if err != nil {
		t.Fatal(err)
	}
	if reversal.EffectiveDate.UTC().Format("2006-01-02") != "2027-01-31" {
		t.Fatalf("reversal date = %s, want 2027-01-31", reversal.EffectiveDate)
	}
	if reversal.ReversalOfID == nil || *reversal.ReversalOfID != entry.ID {
		t.Fatal("reversal does not link to original revaluation")
	}
	if len(reversal.Lines) != 2 {
		t.Fatalf("reversal line count = %d, want 2", len(reversal.Lines))
	}
}

func mustValuationMoney(t *testing.T, amount, currency string) types.Money {
	t.Helper()
	money, err := types.NewMoneyFromString(amount, currency)
	if err != nil {
		t.Fatal(err)
	}
	return money
}
