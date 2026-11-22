package valuation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

// TestLiabilityRevaluationDirectionIsOppositeToAsset pins RULE-KERNEL-023 for
// both position kinds. A liability that grows in functional terms is an
// exchange loss; a liability that shrinks is an exchange gain. The position
// account therefore moves to the opposite side of the asset case.
func TestLiabilityRevaluationDirectionIsOppositeToAsset(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 12, 31, 10, 0, 0, 0, time.UTC)
	positionAccount := types.NewAccountID()
	gainAccount := types.NewAccountID()
	lossAccount := types.NewAccountID()
	company := types.NewCompanyID()
	journalID := types.NewJournalID()
	accounts := FXJournalAccounts{
		CompanyID: company, JournalID: journalID,
		UnrealizedGainID: gainAccount, UnrealizedLossID: lossAccount,
	}

	tests := []struct {
		name            string
		isLiability     bool
		rate            string
		wantGain        bool
		wantPositionDr  bool
		wantPnLAccount  types.AccountID
		wantPnLIsCredit bool
	}{
		{name: "asset gain", rate: "3.75", wantGain: true, wantPositionDr: true, wantPnLAccount: gainAccount, wantPnLIsCredit: true},
		{name: "asset loss", rate: "3.65", wantGain: false, wantPositionDr: false, wantPnLAccount: lossAccount, wantPnLIsCredit: false},
		{name: "liability increase is loss", isLiability: true, rate: "3.75", wantGain: false, wantPositionDr: false, wantPnLAccount: lossAccount, wantPnLIsCredit: false},
		{name: "liability decrease is gain", isLiability: true, rate: "3.65", wantGain: true, wantPositionDr: true, wantPnLAccount: gainAccount, wantPnLIsCredit: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rate := decimal.RequireFromString(test.rate)
			engine := NewRevaluationEngine(fixedRateProvider{
				rate: rate,
				ref: ExchangeRate{
					FromCurrency: "USD", ToCurrency: "SAR", Rate: rate,
					EffectiveDate: date, Type: ClosingRate,
				},
			})
			result, err := engine.Calculate(context.Background(), MonetaryPosition{
				AccountID:              positionAccount,
				ForeignCurrencyBalance: decimal.NewFromInt(10000),
				TransactionCurrency:    "USD",
				FunctionalCurrency:     "SAR",
				CarryingAmount:         mustValuationMoney(t, "37000.00", "SAR"),
				Type:                   Monetary,
				IsLiability:            test.isLiability,
			}, date)
			if err != nil {
				t.Fatalf("Calculate: %v", err)
			}
			if result.IsGain != test.wantGain {
				t.Fatalf("IsGain = %v, want %v", result.IsGain, test.wantGain)
			}
			if result.Difference.Amount().Abs().String() != "500" {
				t.Fatalf("absolute difference = %s, want 500", result.Difference.Amount().Abs())
			}

			entry, err := GenerateRevaluationEntryAt(result, accounts, date, now)
			if err != nil {
				t.Fatalf("GenerateRevaluationEntryAt: %v", err)
			}
			if len(entry.Lines) != 2 {
				t.Fatalf("line count = %d, want 2", len(entry.Lines))
			}
			positionLine, pnlLine := entry.Lines[0], entry.Lines[1]
			if positionLine.AccountID != positionAccount {
				t.Fatalf("position account = %s, want %s", positionLine.AccountID, positionAccount)
			}
			if test.wantPositionDr {
				if !positionLine.FunctionalDebit.Amount().Equal(decimal.NewFromInt(500)) || !positionLine.FunctionalCredit.Amount().IsZero() {
					t.Fatalf("position line must debit 500: %+v", positionLine)
				}
			} else if !positionLine.FunctionalCredit.Amount().Equal(decimal.NewFromInt(500)) || !positionLine.FunctionalDebit.Amount().IsZero() {
				t.Fatalf("position line must credit 500: %+v", positionLine)
			}
			if pnlLine.AccountID != test.wantPnLAccount {
				t.Fatalf("P&L account = %s, want %s", pnlLine.AccountID, test.wantPnLAccount)
			}
			if test.wantPnLIsCredit {
				if !pnlLine.FunctionalCredit.Amount().Equal(decimal.NewFromInt(500)) || !pnlLine.FunctionalDebit.Amount().IsZero() {
					t.Fatalf("gain line must credit 500: %+v", pnlLine)
				}
			} else if !pnlLine.FunctionalDebit.Amount().Equal(decimal.NewFromInt(500)) || !pnlLine.FunctionalCredit.Amount().IsZero() {
				t.Fatalf("loss line must debit 500: %+v", pnlLine)
			}
			debits, credits := decimal.Zero, decimal.Zero
			for _, line := range entry.Lines {
				debits = debits.Add(line.FunctionalDebit.Amount())
				credits = credits.Add(line.FunctionalCredit.Amount())
			}
			if !debits.Equal(credits) || !debits.Equal(decimal.NewFromInt(500)) {
				t.Fatalf("entry not balanced: debits=%s credits=%s", debits, credits)
			}
		})
	}
}

// TestRateRegistrySelectsLatestEffectiveRateAndRejectsFuture covers Phase 9
// task 5: when several historical rates exist for one pair and type, the
// effective-rate lookup must pick the latest one not after the valuation date
// and must never answer from a future rate.
func TestRateRegistrySelectsLatestEffectiveRateAndRejectsFuture(t *testing.T) {
	registry := NewRateRegistry()
	early := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	late := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()

	for _, item := range []struct {
		date time.Time
		rate string
	}{
		{date: early, rate: "3.70"},
		{date: late, rate: "3.75"},
	} {
		if err := registry.RegisterRate(ExchangeRate{
			FromCurrency: "USD", ToCurrency: "SAR",
			Rate:          decimal.RequireFromString(item.rate),
			EffectiveDate: item.date,
			Type:          HistoricalRate,
		}); err != nil {
			t.Fatalf("register %s rate: %v", item.date, err)
		}
	}

	tests := []struct {
		name    string
		asOf    time.Time
		want    string
		wantRef time.Time
		wantErr error
	}{
		{name: "between two rates", asOf: time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), want: "3.70", wantRef: early},
		{name: "on latest rate", asOf: late, want: "3.75", wantRef: late},
		{name: "before first rate", asOf: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), wantErr: ErrRateNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rate, ref, inverted, err := registry.GetRate(ctx, HistoricalRate, "USD", "SAR", test.asOf)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetRate: %v", err)
			}
			if inverted || !rate.Equal(decimal.RequireFromString(test.want)) {
				t.Fatalf("rate = %s, want %s (inverted=%v)", rate, test.want, inverted)
			}
			if !ref.EffectiveDate.Equal(test.wantRef) {
				t.Fatalf("reference date = %s, want %s", ref.EffectiveDate, test.wantRef)
			}
			if ref.EffectiveDate.After(test.asOf) {
				t.Fatalf("selected future rate %s after valuation date %s", ref.EffectiveDate, test.asOf)
			}
		})
	}
}

// TestRevaluationIdempotencyScopeIsDeterministicAndOperationScoped proves the
// operation-level key is wired to the neutral Phase 7 idempotency contract.
func TestRevaluationIdempotencyScopeIsDeterministicAndOperationScoped(t *testing.T) {
	company := types.NewCompanyID()
	account := types.NewAccountID()
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	scope, err := RevaluationIdempotencyScope(company, account, "USD", date)
	if err != nil {
		t.Fatalf("RevaluationIdempotencyScope: %v", err)
	}
	again, err := RevaluationIdempotencyScope(company, account, "usd", date.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("RevaluationIdempotencyScope (normalized): %v", err)
	}
	if scope != again {
		t.Fatalf("scope is not deterministic: %s vs %s", scope, again)
	}
	if scope.CompanyID != company || scope.Operation != posting.IdempotencyRevaluation {
		t.Fatalf("scope = %s, want company %s and operation %s", scope, company, posting.IdempotencyRevaluation)
	}
	if err := scope.Validate(); err != nil {
		t.Fatalf("scope must satisfy the neutral contract: %v", err)
	}
	key, err := RevaluationOperationKey(company, account, "USD", date)
	if err != nil {
		t.Fatal(err)
	}
	if scope.Key != key {
		t.Fatalf("scope key = %q, want operation key %q", scope.Key, key)
	}
	if _, err := RevaluationIdempotencyScope(types.CompanyID{}, account, "USD", date); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("missing company error = %v, want invalid position", err)
	}
	if _, err := RevaluationIdempotencyScope(company, account, "XXX", date); !errors.Is(err, types.ErrInvalidCurrency) {
		t.Fatalf("XXX currency error = %v, want invalid currency", err)
	}
	functionalScope, err := RevaluationIdempotencyScope(company, account, "SAR", date)
	if err != nil {
		t.Fatalf("functional-currency scope: %v", err)
	}
	if functionalScope == scope {
		t.Fatal("transaction currency and functional currency must derive different operation keys")
	}
}

func TestRevaluationRegistryRejectsConflictingPayloadForSameKey(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	now := date.Add(time.Hour)
	account := types.NewAccountID()
	result := FXResult{AccountID: account, Currency: "USD", Difference: mustValuationMoney(t, "500.00", "SAR"), IsGain: true}
	accounts := FXJournalAccounts{
		CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(), UnrealizedLossID: types.NewAccountID(),
	}
	first, err := GenerateRevaluationEntryAt(result, accounts, date, now)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := first.Clone()
	conflicting.ReferenceID = "FX-REVAL-DIFFERENT-RESULT"

	key, err := RevaluationOperationKey(accounts.CompanyID, account, "USD", date)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRevaluationRegistry()
	stored, replay, err := registry.Register(key, first)
	if err != nil || replay {
		t.Fatalf("first registration: replay=%v err=%v", replay, err)
	}
	returned, replay, err := registry.Register(key, conflicting)
	if !replay || !errors.Is(err, ErrRevaluationAlreadyApplied) {
		t.Fatalf("conflicting registration: replay=%v err=%v, want already-applied", replay, err)
	}
	if !errors.Is(err, posting.ErrRevaluationAlreadyApplied) {
		t.Fatalf("conflict must use the phase-7 catalog sentinel: %v", err)
	}
	if posting.IsRetryable(err) {
		t.Fatalf("revaluation payload conflict must be non-retryable: %v", err)
	}
	if returned.ReferenceID != first.ReferenceID {
		t.Fatalf("conflict must return stored result %q, got %q", first.ReferenceID, returned.ReferenceID)
	}
	returned.Lines[0].AccountID = types.NewAccountID()
	stored.Lines[0].AccountID = types.NewAccountID()
	lookedUp, ok := registry.Lookup(key)
	if !ok || lookedUp.ReferenceID != first.ReferenceID || lookedUp.Lines[0].AccountID != account {
		t.Fatalf("registry leaked a mutable result: %+v", lookedUp)
	}

	mutations := []struct {
		name   string
		mutate func(*journal.JournalEntry)
	}{
		{"transaction amount", func(e *journal.JournalEntry) {
			e.Lines[0].TransactionAmount = mustValuationMoney(t, "600.00", "SAR")
		}},
		{"cost center dimension", func(e *journal.JournalEntry) {
			id := types.NewCostCenterID()
			e.Lines[0].CostCenterID = &id
		}},
		{"posting time of day", func(e *journal.JournalEntry) {
			e.PostingDate = e.PostingDate.Add(time.Hour)
		}},
		{"policy fingerprint", func(e *journal.JournalEntry) {
			e.PolicyFingerprint = "policy-v2"
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := first.Clone()
			mutation.mutate(&mutated)
			isolated := NewRevaluationRegistry()
			if _, _, err := isolated.Register(key, mutated); err != nil {
				t.Fatalf("register mutated result: %v", err)
			}
			_, replay, err := isolated.Register(key, first)
			if !replay || !errors.Is(err, ErrRevaluationAlreadyApplied) {
				t.Fatalf("mutated %s replayed silently: replay=%v err=%v", mutation.name, replay, err)
			}
		})
	}
}

func TestRevaluationRegistryConcurrentRegisterHasSingleResult(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	account := types.NewAccountID()
	accounts := FXJournalAccounts{
		CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(), UnrealizedLossID: types.NewAccountID(),
	}
	entry, err := GenerateRevaluationEntryAt(FXResult{
		AccountID: account, Currency: "USD", Difference: mustValuationMoney(t, "500.00", "SAR"), IsGain: true,
	}, accounts, date, date.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	key, err := RevaluationOperationKey(accounts.CompanyID, account, "USD", date)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRevaluationRegistry()

	const workers = 16
	var wg sync.WaitGroup
	results := make(chan journal.JournalEntry, workers)
	errorsCh := make(chan error, workers)
	replays := make(chan bool, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stored, replay, err := registry.Register(key, entry)
			results <- stored
			replays <- replay
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(results)
	close(replays)
	close(errorsCh)

	winners := 0
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent register: %v", err)
		}
	}
	for replay := range replays {
		if !replay {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("registration winners = %d, want 1", winners)
	}
	for stored := range results {
		if stored.ReferenceID != entry.ReferenceID {
			t.Fatalf("concurrent result reference = %q, want %q", stored.ReferenceID, entry.ReferenceID)
		}
	}
}

func TestRevaluationReversalDraftCarriesDateForPeriodResolution(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	reversalDate := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	account := types.NewAccountID()
	accounts := FXJournalAccounts{
		CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(), UnrealizedLossID: types.NewAccountID(),
	}
	entry, err := GenerateRevaluationEntryAt(FXResult{
		AccountID: account, Currency: "USD", Difference: mustValuationMoney(t, "500.00", "SAR"), IsGain: true,
	}, accounts, date, date.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	entry.Status = journal.EntryPosted
	originalPeriod := types.NewFiscalPeriodID()
	entry.FiscalPeriodID = originalPeriod

	reversal, err := BuildRevaluationReversalAt(entry, reversalDate, "system-valuation", "correct FX revaluation", date.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if reversal.FiscalPeriodID != (types.FiscalPeriodID{}) {
		t.Fatal("reversal draft must leave the fiscal period for PostEntry to derive from the reversal date")
	}
	if !reversal.EffectiveDate.Equal(reversalDate) {
		t.Fatalf("reversal effective date = %s, want %s", reversal.EffectiveDate, reversalDate)
	}
	period := masterdata.FiscalPeriod{
		ID:        types.NewFiscalPeriodID(),
		CompanyID: accounts.CompanyID,
		Number:    1,
		State:     masterdata.PeriodOpen,
		Type:      masterdata.FiscalPeriodTypeNormal,
		StartDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2027, 1, 31, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC),
	}
	if !period.Contains(reversal.EffectiveDate) {
		t.Fatalf("reversal date %s is outside the destination period", reversal.EffectiveDate)
	}
	if reversal.ReversalOfID == nil || *reversal.ReversalOfID != entry.ID {
		t.Fatal("reversal must remain linked to the original revaluation")
	}
	if err := journal.ValidateReversalLinkage(entry, reversal); err != nil {
		t.Fatalf("reversal linkage: %v", err)
	}
}

// TestRevaluationOnlyBooksUnrealizedFXDifference pins the implemented part of
// phase-9 task 10: the kernel records the unrealized IAS 21 adjustment on the
// designated unrealized accounts. Realized FX on settlement is a host posting
// decision and is deliberately not produced by this package.
func TestRevaluationOnlyBooksUnrealizedFXDifference(t *testing.T) {
	date := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	rate := decimal.RequireFromString("3.75")
	engine := NewRevaluationEngine(fixedRateProvider{
		rate: rate,
		ref: ExchangeRate{
			FromCurrency: "USD", ToCurrency: "SAR", Rate: rate,
			EffectiveDate: date, Type: ClosingRate,
		},
	})
	result, err := engine.Calculate(context.Background(), MonetaryPosition{
		AccountID:              types.NewAccountID(),
		ForeignCurrencyBalance: decimal.NewFromInt(10000),
		TransactionCurrency:    "USD",
		FunctionalCurrency:     "SAR",
		CarryingAmount:         mustValuationMoney(t, "37000.00", "SAR"),
		Type:                   Monetary,
	}, date)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	accounts := FXJournalAccounts{
		CompanyID: types.NewCompanyID(), JournalID: types.NewJournalID(),
		UnrealizedGainID: types.NewAccountID(), UnrealizedLossID: types.NewAccountID(),
	}
	entry, err := GenerateRevaluationEntryAt(result, accounts, date, date.Add(time.Hour))
	if err != nil {
		t.Fatalf("GenerateRevaluationEntryAt: %v", err)
	}
	if entry.Classification != journal.EntryClassAdjusting || entry.AdjustingEntryType != journal.AdjustingEntryAccrual {
		t.Fatalf("revaluation classification = %s/%s, want an adjusting entry", entry.Classification, entry.AdjustingEntryType)
	}
	if entry.Lines[1].AccountID != accounts.UnrealizedGainID {
		t.Fatalf("P&L account = %s, want unrealized gain account %s", entry.Lines[1].AccountID, accounts.UnrealizedGainID)
	}
	for _, line := range entry.Lines {
		if line.AccountID == accounts.UnrealizedLossID {
			t.Fatal("unrealized gain entry must not post to the loss account")
		}
	}
}

// FuzzRateRegistryEffectiveSelectionNeverPanics checks the phase-9 rate
// selection property: a returned rate is positive, comes from a registered
// effective date not after the query date, and never panics on boundary input.
func FuzzRateRegistryEffectiveSelectionNeverPanics(f *testing.F) {
	f.Add(int64(10), int64(375), int64(3))
	f.Add(int64(-5), int64(1), int64(0))
	f.Add(int64(364), int64(100000), int64(1))
	f.Fuzz(func(t *testing.T, dayOffset, rateUnits, typeIndex int64) {
		if rateUnits <= 0 || rateUnits > 100000 {
			t.Skip()
		}
		rateTypes := []RateType{SpotRate, ClosingRate, AverageRate, HistoricalRate}
		index := int(((typeIndex % int64(len(rateTypes))) + int64(len(rateTypes))) % int64(len(rateTypes)))
		rateType := rateTypes[index]

		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		effective := base.AddDate(0, 0, int(dayOffset%365))
		asOf := effective.AddDate(0, 0, 1)
		rate := decimal.NewFromInt(rateUnits).Div(decimal.NewFromInt(100))

		registry := NewRateRegistry()
		if err := registry.RegisterRate(ExchangeRate{
			FromCurrency: "USD", ToCurrency: "SAR", Rate: rate,
			EffectiveDate: effective, Type: rateType,
		}); err != nil {
			return
		}
		got, ref, inverted, err := registry.GetRate(context.Background(), rateType, "USD", "SAR", asOf)
		if err != nil || inverted {
			t.Fatalf("GetRate(%s, %s) = %s, inverted=%v, err=%v", rateType, asOf, got, inverted, err)
		}
		if !got.IsPositive() || !got.Equal(rate) {
			t.Fatalf("rate = %s, want positive %s", got, rate)
		}
		if !ref.EffectiveDate.Equal(effective) || ref.EffectiveDate.After(asOf) {
			t.Fatalf("reference date = %s, want %s not after %s", ref.EffectiveDate, effective, asOf)
		}
	})
}
