package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func policyTime(day int) time.Time {
	return time.Date(2026, time.January, day, 0, 0, 0, 0, time.UTC)
}

func mustSetPostingPolicy(t *testing.T, engine *PolicyEngine, policy PostingPolicy) {
	t.Helper()
	if err := engine.SetPolicy(policy); err != nil {
		t.Fatalf("SetPolicy(%+v): %v", policy, err)
	}
}

func mustSetCurrencyPolicy(t *testing.T, engine *PolicyEngine, policy CurrencyPolicy) {
	t.Helper()
	if err := engine.SetCurrencyPolicy(policy); err != nil {
		t.Fatalf("SetCurrencyPolicy(%+v): %v", policy, err)
	}
}

// TestPolicyEffectiveDateBoundaryMatrix fixes the inclusive-start/exclusive-end
// contract for both policy families. Insertion order must not change the result.
func TestPolicyEffectiveDateBoundaryMatrix(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	firstEnd := policyTime(1).AddDate(0, 6, 0)

	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse_insertion=%t", reverse), func(t *testing.T) {
			engine := NewPolicyEngine()
			firstEffectiveTo := firstEnd
			postingVersions := []PostingPolicy{
				{CompanyID: companyID, EffectiveDate: policyTime(1), EffectiveTo: &firstEffectiveTo, MaxBackdatingDays: 10},
				{CompanyID: companyID, EffectiveDate: firstEnd, MaxBackdatingDays: 30},
			}
			currencyVersions := []CurrencyPolicy{
				{CompanyID: companyID, EffectiveDate: policyTime(1), EffectiveTo: &firstEffectiveTo, FunctionalCurrency: "USD"},
				{CompanyID: companyID, EffectiveDate: firstEnd, FunctionalCurrency: "EUR"},
			}
			order := []int{0, 1}
			if reverse {
				order = []int{1, 0}
			}
			for _, i := range order {
				mustSetPostingPolicy(t, engine, postingVersions[i])
				mustSetCurrencyPolicy(t, engine, currencyVersions[i])
			}

			cases := []struct {
				name       string
				asOf       time.Time
				found      bool
				backdating int
				functional string
			}{
				{"before_first_start", policyTime(1).Add(-time.Nanosecond), false, 0, ""},
				{"at_first_start", policyTime(1), true, 10, "USD"},
				{"inside_first_window", policyTime(90), true, 10, "USD"},
				{"one_nanosecond_before_first_end", firstEnd.Add(-time.Nanosecond), true, 10, "USD"},
				{"at_first_end_is_second_start", firstEnd, true, 30, "EUR"},
				{"after_second_start", firstEnd.AddDate(0, 1, 0), true, 30, "EUR"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					posting, postingErr := engine.PostingPolicyFor(companyID, tc.asOf)
					currency, currencyErr := engine.CurrencyPolicyFor(companyID, tc.asOf)
					if !tc.found {
						if !errors.Is(postingErr, ErrPolicyNotFound) || !errors.Is(currencyErr, ErrPolicyNotFound) {
							t.Fatalf("outside all windows: posting=%v currency=%v, want ErrPolicyNotFound", postingErr, currencyErr)
						}
						return
					}
					if postingErr != nil || currencyErr != nil {
						t.Fatalf("inside a window: posting=%v currency=%v", postingErr, currencyErr)
					}
					if posting.MaxBackdatingDays != tc.backdating || currency.FunctionalCurrency != tc.functional {
						t.Fatalf("resolved posting=%+v currency=%+v, want days=%d currency=%s",
							posting, currency, tc.backdating, tc.functional)
					}
				})
			}
		})
	}
}

// TestPolicyEngineRejectsUnresolvedOverlaps proves that ambiguity is rejected
// at registration time instead of being resolved by map/slice iteration order.
func TestPolicyEngineRejectsUnresolvedOverlaps(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	january := policyTime(1)
	march := policyTime(90)
	june := policyTime(180)
	july := policyTime(181)
	december := policyTime(365)

	boundedFor := func(owner apiTypes.CompanyID, start, end time.Time, days int) PostingPolicy {
		endCopy := end
		return PostingPolicy{CompanyID: owner, EffectiveDate: start, EffectiveTo: &endCopy, MaxBackdatingDays: days}
	}
	bounded := func(start, end time.Time, days int) PostingPolicy {
		return boundedFor(companyID, start, end, days)
	}

	cases := []struct {
		name      string
		seed      []PostingPolicy
		candidate PostingPolicy
		wantErr   error
	}{
		{
			name:      "duplicate_effective_start",
			seed:      []PostingPolicy{{CompanyID: companyID, EffectiveDate: january}},
			candidate: PostingPolicy{CompanyID: companyID, EffectiveDate: january},
			wantErr:   ErrInvalidPolicy,
		},
		{
			name: "inverted_window",
			candidate: PostingPolicy{
				CompanyID: companyID, EffectiveDate: july, EffectiveTo: &march,
			},
			wantErr: ErrInvalidPolicy,
		},
		{
			name:      "explicit_bounded_overlap",
			seed:      []PostingPolicy{bounded(january, july, 10)},
			candidate: bounded(june, december, 20),
			wantErr:   ErrPolicyOverlap,
		},
		{
			name:      "open_candidate_overlaps_bounded_predecessor",
			seed:      []PostingPolicy{bounded(january, july, 10)},
			candidate: PostingPolicy{CompanyID: companyID, EffectiveDate: march},
			wantErr:   ErrPolicyOverlap,
		},
		{
			name:      "implicit_version_chain_is_resolved",
			seed:      []PostingPolicy{{CompanyID: companyID, EffectiveDate: january}},
			candidate: PostingPolicy{CompanyID: companyID, EffectiveDate: july},
		},
		{
			name:      "touching_windows_are_not_overlap",
			seed:      []PostingPolicy{bounded(january, july, 10)},
			candidate: bounded(july, december, 20),
		},
		{
			name:      "explicit_gap_is_allowed_and_fails_closed",
			seed:      []PostingPolicy{bounded(january, march, 10)},
			candidate: bounded(june, december, 20),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := NewPolicyEngine()
			for _, seeded := range tc.seed {
				mustSetPostingPolicy(t, engine, seeded)
			}
			err := engine.SetPolicy(tc.candidate)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("candidate rejected: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("candidate error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("currency_policies_use_the_same_overlap_rule", func(t *testing.T) {
		engine := NewPolicyEngine()
		januaryEnd := july
		mustSetCurrencyPolicy(t, engine, CurrencyPolicy{
			CompanyID: companyID, EffectiveDate: january, EffectiveTo: &januaryEnd, FunctionalCurrency: "USD",
		})
		candidateEnd := december
		err := engine.SetCurrencyPolicy(CurrencyPolicy{
			CompanyID: companyID, EffectiveDate: june, EffectiveTo: &candidateEnd, FunctionalCurrency: "EUR",
		})
		if !errors.Is(err, ErrPolicyOverlap) {
			t.Fatalf("overlapping currency policy error = %v, want ErrPolicyOverlap", err)
		}
	})

	t.Run("same_window_in_another_company_is_allowed", func(t *testing.T) {
		engine := NewPolicyEngine()
		otherCompany := apiTypes.NewCompanyID()
		mustSetPostingPolicy(t, engine, bounded(january, july, 10))
		if err := engine.SetPolicy(boundedFor(otherCompany, january, july, 20)); err != nil {
			t.Fatalf("second company policy rejected: %v", err)
		}
		if err := engine.SetPolicy(bounded(july, december, 30)); err != nil {
			t.Fatalf("bounded successor for same company rejected: %v", err)
		}
	})
}

func TestPolicySelectionIsScopedAndFailsClosed(t *testing.T) {
	companyA, companyB, companyC := apiTypes.NewCompanyID(), apiTypes.NewCompanyID(), apiTypes.NewCompanyID()
	date := policyTime(90)

	engine := NewPolicyEngine()
	if _, err := engine.PostingPolicyFor(companyA, date); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("empty engine posting error = %v, want ErrPolicyNotFound", err)
	}
	if _, err := engine.CurrencyPolicyFor(companyA, date); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("empty engine currency error = %v, want ErrPolicyNotFound", err)
	}

	mustSetPostingPolicy(t, engine, PostingPolicy{CompanyID: companyA, EffectiveDate: policyTime(1), MaxBackdatingDays: 11})
	mustSetPostingPolicy(t, engine, PostingPolicy{CompanyID: companyB, EffectiveDate: policyTime(1), MaxBackdatingDays: 22})
	mustSetCurrencyPolicy(t, engine, CurrencyPolicy{CompanyID: companyA, EffectiveDate: policyTime(1), FunctionalCurrency: "USD"})
	mustSetCurrencyPolicy(t, engine, CurrencyPolicy{CompanyID: companyB, EffectiveDate: policyTime(1), FunctionalCurrency: "EUR"})

	for _, tc := range []struct {
		company    apiTypes.CompanyID
		days       int
		functional string
	}{
		{companyA, 11, "USD"},
		{companyB, 22, "EUR"},
	} {
		posting, err := engine.PostingPolicyFor(tc.company, date)
		if err != nil || posting.MaxBackdatingDays != tc.days {
			t.Fatalf("company %s posting = %+v, %v", tc.company, posting, err)
		}
		currency, err := engine.CurrencyPolicyFor(tc.company, date)
		if err != nil || currency.FunctionalCurrency != tc.functional {
			t.Fatalf("company %s currency = %+v, %v", tc.company, currency, err)
		}
	}
	if _, err := engine.PostingPolicyFor(companyC, date); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("unknown company posting error = %v, want ErrPolicyNotFound", err)
	}
	if _, err := engine.CurrencyPolicyFor(companyC, date); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("unknown company currency error = %v, want ErrPolicyNotFound", err)
	}
}

func TestStoredPoliciesAreIsolatedFromSharedSlicesAndPointers(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	start := policyTime(1)
	endTime := policyTime(180)
	end := endTime
	allowed := []string{"USD", "EUR"}

	posting := PostingPolicy{CompanyID: companyID, EffectiveDate: start, EffectiveTo: &end, MaxBackdatingDays: 7}
	currency := CurrencyPolicy{
		CompanyID: companyID, EffectiveDate: start, EffectiveTo: &end,
		FunctionalCurrency: "USD", AllowedCurrencies: allowed,
	}
	engine := NewPolicyEngine()
	mustSetPostingPolicy(t, engine, posting)
	mustSetCurrencyPolicy(t, engine, currency)

	posting.MaxBackdatingDays = 99
	posting.EffectiveTo = nil
	endTime = policyTime(300)
	allowed[0] = "GBP"

	storedPosting, err := engine.PostingPolicyFor(companyID, policyTime(2))
	if err != nil || storedPosting.MaxBackdatingDays != 7 || storedPosting.EffectiveTo == nil ||
		!storedPosting.EffectiveTo.Equal(end) {
		t.Fatalf("input posting mutation leaked into store: %+v, %v", storedPosting, err)
	}
	storedCurrency, err := engine.CurrencyPolicyFor(companyID, policyTime(2))
	if err != nil || storedCurrency.AllowedCurrencies[0] != "USD" || !storedCurrency.EffectiveTo.Equal(end) {
		t.Fatalf("input currency mutation leaked into store: %+v, %v", storedCurrency, err)
	}

	storedPosting.MaxBackdatingDays = 100
	*storedPosting.EffectiveTo = policyTime(400)
	storedCurrency.AllowedCurrencies[0] = "JPY"
	*storedCurrency.EffectiveTo = policyTime(400)

	againPosting, err := engine.PostingPolicyFor(companyID, policyTime(2))
	if err != nil || againPosting.MaxBackdatingDays != 7 || !againPosting.EffectiveTo.Equal(end) {
		t.Fatalf("returned posting mutation leaked into store: %+v, %v", againPosting, err)
	}
	againCurrency, err := engine.CurrencyPolicyFor(companyID, policyTime(2))
	if err != nil || againCurrency.AllowedCurrencies[0] != "USD" || !againCurrency.EffectiveTo.Equal(end) {
		t.Fatalf("returned currency mutation leaked into store: %+v, %v", againCurrency, err)
	}
}

func TestCurrencyPolicyValidationAndRoundingAudit(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	date := policyTime(1)
	base := CurrencyPolicy{CompanyID: companyID, EffectiveDate: date, FunctionalCurrency: "USD"}

	invalid := []struct {
		name   string
		change func(*CurrencyPolicy)
	}{
		{"zero_company", func(p *CurrencyPolicy) { p.CompanyID = apiTypes.CompanyID{} }},
		{"zero_effective_date", func(p *CurrencyPolicy) { p.EffectiveDate = time.Time{} }},
		{"unknown_functional_currency", func(p *CurrencyPolicy) { p.FunctionalCurrency = "ZZZ" }},
		{"xxx_functional_currency", func(p *CurrencyPolicy) { p.FunctionalCurrency = "XXX" }},
		{"unknown_presentation_currency", func(p *CurrencyPolicy) { p.PresentationCurrency = "ZZZ" }},
		{"xxx_presentation_currency", func(p *CurrencyPolicy) { p.PresentationCurrency = "XXX" }},
		{"unknown_rounding_method", func(p *CurrencyPolicy) { p.RoundingMethod = RoundingMethod("nearest") }},
		{"auto_settle_without_account", func(p *CurrencyPolicy) { p.AutoSettleRoundingDifferences = true }},
		{"unknown_allowed_currency", func(p *CurrencyPolicy) { p.AllowedCurrencies = []string{"ZZZ"} }},
		{"xxx_allowed_currency", func(p *CurrencyPolicy) { p.AllowedCurrencies = []string{"XXX"} }},
		{"duplicate_allowed_currency", func(p *CurrencyPolicy) { p.AllowedCurrencies = []string{"usd", "USD"} }},
		{"inverted_window", func(p *CurrencyPolicy) { end := date.AddDate(0, -1, 0); p.EffectiveTo = &end }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.change(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("Validate() = %v, want ErrInvalidPolicy", err)
			}
		})
	}

	valid := base
	if err := valid.Validate(); err != nil {
		t.Fatalf("default banker's rounding policy rejected: %v", err)
	}
	valid.RoundingMethod = RoundingHalfUp
	if err := valid.Validate(); err != nil {
		t.Fatalf("half-up rounding policy rejected: %v", err)
	}

	for _, tc := range []struct {
		method RoundingMethod
		want   string
	}{
		{"", "1.00"},
		{RoundingBankers, "1.00"},
		{RoundingHalfUp, "1.01"},
	} {
		amount, err := apiTypes.NewMoneyFromString("1.005", "USD")
		if err != nil {
			t.Fatal(err)
		}
		rounded, err := (CurrencyPolicy{RoundingMethod: tc.method}).Round(amount)
		if err != nil || rounded.Amount().StringFixed(2) != tc.want {
			t.Fatalf("round(%q) = %s, %v; want %s", tc.method, rounded, err, tc.want)
		}
	}

	if _, err := valid.Round(apiTypes.Money{}); !errors.Is(err, apiTypes.ErrInvalidCurrency) {
		t.Fatalf("rounding an invalid money error = %v, want ErrInvalidCurrency", err)
	}
	if valid.AllowsCurrency("usd") != true || valid.AllowsCurrency("eur") != false {
		t.Fatal("allowed-currency decision must be normalized and fail closed")
	}
}

// policyCloseReader is the minimal master-data surface required to prove that
// the *selected* posting policy controls soft-close acceptance.
type policyCloseReader struct {
	masterdata.Reader
	companyID   apiTypes.CompanyID
	yearID      apiTypes.FiscalYearID
	periodState masterdata.PeriodState
	yearState   masterdata.FiscalYearState
	start       time.Time
	end         time.Time
}

func (r policyCloseReader) GetPeriodForDate(
	_ context.Context, companyID apiTypes.CompanyID, date time.Time,
) (masterdata.FiscalPeriod, error) {
	if companyID != r.companyID || date.Before(r.start) || date.After(r.end) {
		return masterdata.FiscalPeriod{}, masterdata.ErrInvalidFiscalPeriod
	}
	return masterdata.FiscalPeriod{
		ID: apiTypes.NewFiscalPeriodID(), CompanyID: companyID, FiscalYearID: r.yearID,
		Number: 6, StartDate: r.start, EndDate: r.end, State: r.periodState,
		Type: masterdata.FiscalPeriodTypeNormal,
	}, nil
}

func (r policyCloseReader) GetFiscalYear(
	_ context.Context, companyID apiTypes.CompanyID, yearID apiTypes.FiscalYearID,
) (masterdata.FiscalYear, error) {
	if companyID != r.companyID || yearID != r.yearID {
		return masterdata.FiscalYear{}, masterdata.ErrInvalidFiscalPeriod
	}
	return masterdata.FiscalYear{
		ID: r.yearID, CompanyID: companyID, StartDate: r.start, EndDate: r.end, State: r.yearState,
	}, nil
}

func TestSelectedPostingPolicyControlsSoftCloseOnly(t *testing.T) {
	ctx := context.Background()
	companyID := apiTypes.NewCompanyID()
	entryDate := policyTime(180)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)

	engine := NewPolicyEngine()
	mustSetPostingPolicy(t, engine, PostingPolicy{
		CompanyID: companyID, EffectiveDate: start, AllowSoftClosedPosting: false,
	})
	mustSetCurrencyPolicy(t, engine, CurrencyPolicy{
		CompanyID: companyID, EffectiveDate: start, FunctionalCurrency: "USD",
	})

	reader := policyCloseReader{
		companyID: companyID, yearID: apiTypes.NewFiscalYearID(),
		periodState: masterdata.PeriodSoftClosed, yearState: masterdata.FiscalYearOpen,
		start: start, end: end,
	}

	selected, err := engine.PostingPolicyFor(companyID, entryDate)
	if err != nil {
		t.Fatal(err)
	}
	if selected.AllowSoftClosedPosting {
		t.Fatal("first policy version unexpectedly allows soft close")
	}
	if _, err := masterdata.ValidatePeriodForPostingWithSoftClose(
		ctx, reader, companyID, entryDate, selected.AllowSoftClosedPosting,
	); !errors.Is(err, masterdata.ErrClosedPeriod) {
		t.Fatalf("soft-close posting without explicit permission error = %v, want ErrClosedPeriod", err)
	}

	// A new policy version changes the decision only from its effective date on.
	changeDate := policyTime(181)
	mustSetPostingPolicy(t, engine, PostingPolicy{
		CompanyID: companyID, EffectiveDate: changeDate, AllowSoftClosedPosting: true,
	})
	oldSelected, err := engine.PostingPolicyFor(companyID, entryDate)
	if err != nil || oldSelected.AllowSoftClosedPosting {
		t.Fatalf("historical policy version was rewritten: %+v, %v", oldSelected, err)
	}
	newSelected, err := engine.PostingPolicyFor(companyID, changeDate)
	if err != nil || !newSelected.AllowSoftClosedPosting {
		t.Fatalf("new policy version not selected at its boundary: %+v, %v", newSelected, err)
	}
	if _, err := masterdata.ValidatePeriodForPostingWithSoftClose(
		ctx, reader, companyID, changeDate, newSelected.AllowSoftClosedPosting,
	); err != nil {
		t.Fatalf("soft-close posting with explicit permission rejected: %v", err)
	}

	reader.periodState = masterdata.PeriodHardClosed
	reader.yearState = masterdata.FiscalYearOpen
	if _, err := masterdata.ValidatePeriodForPostingWithSoftClose(
		ctx, reader, companyID, changeDate, newSelected.AllowSoftClosedPosting,
	); !errors.Is(err, masterdata.ErrClosedPeriod) {
		t.Fatalf("hard-close posting with permissive policy error = %v, want ErrClosedPeriod", err)
	}
}

func TestAccountEligibilityDecisionMatrix(t *testing.T) {
	ctx := context.Background()
	companyID := apiTypes.NewCompanyID()
	accountID := apiTypes.NewAccountID()
	expired := policyTime(1)
	validEntry := &journal.JournalEntry{
		CompanyID: companyID, EffectiveDate: policyTime(180),
		Lines: []journal.EntryLine{{AccountID: accountID, TransactionCurrency: "USD"}},
	}

	cases := []struct {
		name    string
		entry   *journal.JournalEntry
		account masterdata.Account
		wantErr error
	}{
		{
			name:    "nil_entry",
			entry:   nil,
			wantErr: ErrNilJournalEntry,
		},
		{
			name:    "missing_account",
			entry:   validEntry,
			wantErr: ErrAccountNotFound,
		},
		{
			name:    "inactive_account",
			entry:   validEntry,
			account: masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1000"},
			wantErr: ErrAccountInactive,
		},
		{
			name:    "foreign_company_account",
			entry:   validEntry,
			account: masterdata.Account{ID: accountID, CompanyID: apiTypes.NewCompanyID(), Code: "1000", IsActive: true},
			wantErr: masterdata.ErrAccountIdentityMismatch,
		},
		{
			name:    "expired_account",
			entry:   validEntry,
			account: masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1000", IsActive: true, EffectiveTo: &expired},
			wantErr: ErrAccountNotEffective,
		},
		{
			name:    "control_account",
			entry:   validEntry,
			account: masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1000", IsActive: true, IsControlAccount: true},
			wantErr: ErrControlAccountNotPostable,
		},
		{
			name: "missing_required_cost_center",
			entry: &journal.JournalEntry{CompanyID: companyID, EffectiveDate: policyTime(180),
				Lines: []journal.EntryLine{{AccountID: accountID, TransactionCurrency: "USD"}}},
			account: masterdata.Account{ID: accountID, CompanyID: companyID, Code: "6010", IsActive: true, RequiresCostCenter: true},
			wantErr: ErrCostCenterRequired,
		},
		{
			name: "currency_mismatch",
			entry: &journal.JournalEntry{CompanyID: companyID, EffectiveDate: policyTime(180),
				Lines: []journal.EntryLine{{AccountID: accountID, TransactionCurrency: "EUR"}}},
			account: masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1010", IsActive: true, Currency: "USD"},
			wantErr: apiTypes.ErrCurrencyMismatch,
		},
		{
			name:    "eligible_account",
			entry:   validEntry,
			account: masterdata.Account{ID: accountID, CompanyID: companyID, Code: "1010", IsActive: true, Currency: "USD"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accounts := map[apiTypes.AccountID]masterdata.Account{}
			if tc.account.ID != (apiTypes.AccountID{}) {
				accounts[accountID] = tc.account
			}
			err := CheckAccountEligibility(ctx, tc.entry, accounts)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("eligible account rejected: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("eligibility error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	costCenterID := apiTypes.NewCostCenterID()
	withCostCenter := &journal.JournalEntry{CompanyID: companyID, EffectiveDate: policyTime(180),
		Lines: []journal.EntryLine{{AccountID: accountID, TransactionCurrency: "USD", CostCenterID: &costCenterID}}}
	eligible := map[apiTypes.AccountID]masterdata.Account{accountID: {
		ID: accountID, CompanyID: companyID, Code: "6010", IsActive: true, RequiresCostCenter: true,
	}}
	if err := CheckAccountEligibility(ctx, withCostCenter, eligible); err != nil {
		t.Fatalf("account with required cost center rejected: %v", err)
	}
}

func TestPolicyFingerprintIsCanonicalAndContentBound(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	posting := PostingPolicy{
		CompanyID: companyID, EffectiveDate: policyTime(1), MaxBackdatingDays: 30,
		AllowSoftClosedPosting: true,
	}
	currency := CurrencyPolicy{
		CompanyID: companyID, EffectiveDate: policyTime(1), FunctionalCurrency: "USD",
		PresentationCurrency: "EUR", RoundingMethod: RoundingBankers,
		AllowedCurrencies: []string{"USD", "EUR"},
	}

	first := Fingerprint(posting, currency)
	if !strings.HasPrefix(first, "sha256:") || len(first) != len("sha256:")+64 {
		t.Fatalf("fingerprint format = %q, want sha256:<64 hex chars>", first)
	}
	for i := 0; i < 50; i++ {
		if next := Fingerprint(posting, currency); next != first {
			t.Fatalf("fingerprint changed between calls: %q then %q", first, next)
		}
	}

	reordered := currency
	reordered.AllowedCurrencies = []string{"EUR", "USD"}
	if got := Fingerprint(posting, reordered); got != first {
		t.Fatal("allowed-currency slice order must not change the fingerprint")
	}
	if len(currency.AllowedCurrencies) != 2 || currency.AllowedCurrencies[0] != "USD" {
		t.Fatal("Fingerprint mutated the input allow-list")
	}

	mutations := []struct {
		name     string
		posting  PostingPolicy
		currency CurrencyPolicy
	}{
		{"company", posting, currency},
		{"posting_window", posting, currency},
		{"posting_backdating", posting, currency},
		{"soft_close", posting, currency},
		{"currency_window", posting, currency},
		{"functional_currency", posting, currency},
		{"presentation_currency", posting, currency},
		{"rounding_method", posting, currency},
		{"allowed_currencies", posting, currency},
		{"rounding_account", posting, currency},
	}
	mutations[0].posting.CompanyID = apiTypes.NewCompanyID()
	mutations[1].posting.EffectiveTo = &[]time.Time{policyTime(180)}[0]
	mutations[2].posting.MaxBackdatingDays = 31
	mutations[3].posting.AllowSoftClosedPosting = false
	mutations[4].currency.EffectiveTo = &[]time.Time{policyTime(180)}[0]
	mutations[5].currency.FunctionalCurrency = "EUR"
	mutations[6].currency.PresentationCurrency = "USD"
	mutations[7].currency.RoundingMethod = RoundingHalfUp
	mutations[8].currency.AllowedCurrencies = []string{"USD"}
	mutations[9].currency.AutoSettleRoundingDifferences = true
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			if got := Fingerprint(tc.posting, tc.currency); got == first {
				t.Fatalf("fingerprint did not change after %s mutation", tc.name)
			}
		})
	}
}

func TestPolicyEngineConcurrentResolveAndVersionUpdate(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	stableDate := policyTime(90)
	engine := NewPolicyEngine()
	mustSetPostingPolicy(t, engine, PostingPolicy{CompanyID: companyID, EffectiveDate: policyTime(1), MaxBackdatingDays: 7})
	mustSetCurrencyPolicy(t, engine, CurrencyPolicy{CompanyID: companyID, EffectiveDate: policyTime(1), FunctionalCurrency: "USD"})

	var wg sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				posting, err := engine.PostingPolicyFor(companyID, stableDate)
				if err != nil || posting.MaxBackdatingDays != 7 {
					t.Errorf("concurrent posting resolve = %+v, %v", posting, err)
					return
				}
				currency, err := engine.CurrencyPolicyFor(companyID, stableDate)
				if err != nil || currency.FunctionalCurrency != "USD" {
					t.Errorf("concurrent currency resolve = %+v, %v", currency, err)
					return
				}
			}
		}()
	}
	for writer := 0; writer < 2; writer++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				start := policyTime(181).AddDate(0, (offset*10+i)+1, 0)
				if err := engine.SetPolicy(PostingPolicy{
					CompanyID: companyID, EffectiveDate: start, MaxBackdatingDays: i + 1,
				}); err != nil {
					t.Errorf("concurrent SetPolicy(%s): %v", start, err)
					return
				}
				if err := engine.SetCurrencyPolicy(CurrencyPolicy{
					CompanyID: companyID, EffectiveDate: start, FunctionalCurrency: "USD",
				}); err != nil {
					t.Errorf("concurrent SetCurrencyPolicy(%s): %v", start, err)
					return
				}
			}
		}(writer)
	}
	wg.Wait()
}

func TestConcurrentRegistrationOfSameEffectiveStartHasOneWinner(t *testing.T) {
	companyID := apiTypes.NewCompanyID()
	start := policyTime(1)
	engine := NewPolicyEngine()

	const contenders = 12
	results := make(chan error, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(days int) {
			defer wg.Done()
			results <- engine.SetPolicy(PostingPolicy{
				CompanyID: companyID, EffectiveDate: start, MaxBackdatingDays: days,
			})
		}(i + 1)
	}
	wg.Wait()
	close(results)

	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("losing registration error = %v, want ErrInvalidPolicy", err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d registrations for the same effective start, want 1", accepted)
	}
}
