package policy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

// Policy windows are half-open: [EffectiveDate, EffectiveTo). A nil EffectiveTo
// means "until the next registered version", which keeps the historical
// latest-effective-wins behavior while making the boundary explicit.
func normalizedPolicyTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC()
}

func clonedPolicyEnd(end *time.Time) *time.Time {
	if end == nil || normalizedPolicyTime(*end).IsZero() {
		return nil
	}
	normalized := normalizedPolicyTime(*end)
	return &normalized
}

func policyWindowEnd(start time.Time, explicitEnd, nextStart *time.Time) (time.Time, bool) {
	if explicitEnd != nil {
		return explicitEnd.UTC(), true
	}
	if nextStart != nil {
		return nextStart.UTC(), true
	}
	return time.Time{}, false
}

func isWindowEffective(start time.Time, explicitEnd, nextStart *time.Time, asOf time.Time) bool {
	if asOf.Before(start) {
		return false
	}
	end, bounded := policyWindowEnd(start, explicitEnd, nextStart)
	return !bounded || asOf.Before(end)
}

type registeredWindow struct {
	start time.Time
	end   *time.Time
}

// checkWindowRegistration rejects duplicate starts and unresolved overlaps.
// Touching windows are legal; an open window is closed by the next version.
func checkWindowRegistration(existing []registeredWindow, candidate registeredWindow) error {
	index := sort.Search(len(existing), func(i int) bool {
		return !existing[i].start.Before(candidate.start)
	})
	if index < len(existing) && existing[index].start.Equal(candidate.start) {
		return fmt.Errorf("%w: duplicate effective date %s", ErrInvalidPolicy, candidate.start.Format(time.RFC3339Nano))
	}

	if index > 0 {
		predecessor := existing[index-1]
		predecessorEnd, bounded := policyWindowEnd(predecessor.start, predecessor.end, &candidate.start)
		if bounded && candidate.start.Before(predecessorEnd) {
			candidateEnd := "-"
			if candidate.end != nil {
				candidateEnd = candidate.end.UTC().Format(time.RFC3339Nano)
			}
			return fmt.Errorf("%w: [start=%s,end=%s) overlaps [start=%s,end=%s)",
				ErrPolicyOverlap,
				predecessor.start.Format(time.RFC3339Nano), predecessorEnd.Format(time.RFC3339Nano),
				candidate.start.Format(time.RFC3339Nano), candidateEnd)
		}
	}
	if index < len(existing) && candidate.end != nil {
		successor := existing[index]
		if successor.start.Before(candidate.end.UTC()) {
			return fmt.Errorf("%w: [start=%s,end=%s) overlaps successor start %s",
				ErrPolicyOverlap,
				candidate.start.Format(time.RFC3339Nano), candidate.end.Format(time.RFC3339Nano),
				successor.start.Format(time.RFC3339Nano))
		}
	}
	return nil
}

type RoundingMethod string

const (
	RoundingBankers RoundingMethod = "bankers"
	RoundingHalfUp  RoundingMethod = "half_up"
)

type CurrencyPolicy struct {
	CompanyID                     apiTypes.CompanyID
	FunctionalCurrency            string
	PresentationCurrency          string
	RoundingMethod                RoundingMethod
	AllowedCurrencies             []string
	RoundingDifferenceAccountID   apiTypes.AccountID
	AutoSettleRoundingDifferences bool
	EffectiveDate                 time.Time
	EffectiveTo                   *time.Time
}

func (p CurrencyPolicy) Validate() error {
	functionalCurrency := strings.ToUpper(strings.TrimSpace(p.FunctionalCurrency))
	if p.CompanyID == (apiTypes.CompanyID{}) || p.EffectiveDate.IsZero() {
		return ErrInvalidPolicy
	}
	if p.EffectiveTo != nil && !normalizedPolicyTime(*p.EffectiveTo).IsZero() &&
		!normalizedPolicyTime(*p.EffectiveTo).After(p.EffectiveDate.UTC()) {
		return ErrInvalidPolicy
	}
	if currency, ok := apiTypes.LookupCurrency(functionalCurrency); !ok || currency.IsSpecial() {
		return ErrInvalidPolicy
	}
	if presentationCurrency := strings.ToUpper(strings.TrimSpace(p.PresentationCurrency)); presentationCurrency != "" {
		currency, ok := apiTypes.LookupCurrency(presentationCurrency)
		if !ok || currency.IsSpecial() {
			return ErrInvalidPolicy
		}
	}
	if p.RoundingMethod != "" && p.RoundingMethod != RoundingBankers && p.RoundingMethod != RoundingHalfUp {
		return ErrInvalidPolicy
	}
	if p.AutoSettleRoundingDifferences && p.RoundingDifferenceAccountID == (apiTypes.AccountID{}) {
		return ErrInvalidPolicy
	}
	seenCurrencies := make(map[apiTypes.CurrencyCode]struct{}, len(p.AllowedCurrencies))
	for _, code := range p.AllowedCurrencies {
		currency, ok := apiTypes.LookupCurrency(code)
		if !ok || currency.IsSpecial() {
			return ErrInvalidPolicy
		}
		if _, exists := seenCurrencies[currency.Code]; exists {
			return ErrInvalidPolicy
		}
		seenCurrencies[currency.Code] = struct{}{}
	}
	return nil
}

func (p CurrencyPolicy) Round(amount apiTypes.Money) (apiTypes.Money, error) {
	currency, ok := apiTypes.LookupCurrency(amount.Currency())
	if !ok || currency.IsSpecial() {
		return apiTypes.Money{}, apiTypes.ErrInvalidCurrency
	}
	rounded := amount.Amount().RoundBank(currency.MinorUnit)
	if p.RoundingMethod == RoundingHalfUp {
		rounded = amount.Amount().Round(currency.MinorUnit)
	}
	return apiTypes.NewMoney(rounded, string(currency.Code))
}

func (p CurrencyPolicy) AllowsCurrency(currency string) bool {
	currencyInfo, ok := apiTypes.LookupCurrency(currency)
	if !ok || currencyInfo.IsSpecial() {
		return false
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == strings.ToUpper(strings.TrimSpace(p.FunctionalCurrency)) {
		return true
	}
	if len(p.AllowedCurrencies) == 0 {
		return false
	}
	for _, allowed := range p.AllowedCurrencies {
		if currency == strings.ToUpper(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

var (
	ErrPolicyEngineRequired     = errors.New("policy engine is required")
	ErrPolicyNotFound           = errors.New("accounting policy not found")
	ErrInvalidPolicy            = errors.New("invalid accounting policy")
	ErrPolicyOverlap            = errors.New("overlapping accounting policy windows")
	ErrAccountTypeNotFound      = errors.New("posting account type not found")
	ErrCurrencyNotAllowed       = errors.New("currency is not allowed by company policy")
	ErrRoundingMismatch         = errors.New("amount does not conform to currency rounding policy")
	ErrBackdatingLimit          = errors.New("journal entry exceeds the permitted backdating threshold")
	ErrCostCenterRequired       = errors.New("cost center is required for income and expense lines")
	ErrAnalyticalDimensionLimit = errors.New("analytical dimension required by company policy")
	ErrPolicyNotEffective       = errors.New("accounting policy is not effective for the posting date")
)

type PostingPolicy struct {
	CompanyID                                  apiTypes.CompanyID
	MaxBackdatingDays                          int
	AllowBackdatingException                   bool
	AllowSoftClosedPosting                     bool
	RequireCostCenterForIncomeExpense          bool
	RequireAnalyticalDimensionForIncomeExpense bool
	EffectiveDate                              time.Time
	EffectiveTo                                *time.Time
}

func (p PostingPolicy) Validate() error {
	if p.CompanyID == (apiTypes.CompanyID{}) || p.EffectiveDate.IsZero() || p.MaxBackdatingDays < 0 {
		return ErrInvalidPolicy
	}
	if p.EffectiveTo != nil && !normalizedPolicyTime(*p.EffectiveTo).IsZero() &&
		!normalizedPolicyTime(*p.EffectiveTo).After(p.EffectiveDate.UTC()) {
		return ErrInvalidPolicy
	}
	return nil
}

type PolicyEngine struct {
	mu               sync.RWMutex
	policies         map[apiTypes.CompanyID][]PostingPolicy
	currencyPolicies map[apiTypes.CompanyID][]CurrencyPolicy
}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies:         make(map[apiTypes.CompanyID][]PostingPolicy),
		currencyPolicies: make(map[apiTypes.CompanyID][]CurrencyPolicy),
	}
}

// ClearCompanyPolicies removes every policy version registered for one
// company. The engine never calls it by itself: it exists so a host can replace
// a previously applied legacy fallback with an authoritative versioned history
// without stale windows surviving in the same process.
func (e *PolicyEngine) ClearCompanyPolicies(companyID apiTypes.CompanyID) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.policies, companyID)
	delete(e.currencyPolicies, companyID)
}

func (e *PolicyEngine) SetPolicy(policy PostingPolicy) error {
	if e == nil {
		return ErrPolicyEngineRequired
	}
	policy.EffectiveDate = normalizedPolicyTime(policy.EffectiveDate)
	if err := policy.Validate(); err != nil {
		return err
	}
	policy.EffectiveTo = clonedPolicyEnd(policy.EffectiveTo)

	e.mu.Lock()
	defer e.mu.Unlock()
	versions := e.policies[policy.CompanyID]
	existing := make([]registeredWindow, len(versions))
	for i, current := range versions {
		existing[i] = registeredWindow{start: current.EffectiveDate, end: current.EffectiveTo}
	}
	if err := checkWindowRegistration(existing, registeredWindow{
		start: policy.EffectiveDate, end: policy.EffectiveTo,
	}); err != nil {
		return err
	}
	versions = append(versions, policy)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].EffectiveDate.Before(versions[j].EffectiveDate)
	})
	e.policies[policy.CompanyID] = versions
	return nil
}

func (e *PolicyEngine) SetCurrencyPolicy(policy CurrencyPolicy) error {
	if e == nil {
		return ErrPolicyEngineRequired
	}
	policy.EffectiveDate = normalizedPolicyTime(policy.EffectiveDate)
	if err := policy.Validate(); err != nil {
		return err
	}
	policy.AllowedCurrencies = append([]string(nil), policy.AllowedCurrencies...)
	policy.FunctionalCurrency = strings.ToUpper(strings.TrimSpace(policy.FunctionalCurrency))
	policy.PresentationCurrency = strings.ToUpper(strings.TrimSpace(policy.PresentationCurrency))
	for i := range policy.AllowedCurrencies {
		policy.AllowedCurrencies[i] = strings.ToUpper(strings.TrimSpace(policy.AllowedCurrencies[i]))
	}
	policy.EffectiveTo = clonedPolicyEnd(policy.EffectiveTo)

	e.mu.Lock()
	defer e.mu.Unlock()
	versions := e.currencyPolicies[policy.CompanyID]
	existing := make([]registeredWindow, len(versions))
	for i, current := range versions {
		existing[i] = registeredWindow{start: current.EffectiveDate, end: current.EffectiveTo}
	}
	if err := checkWindowRegistration(existing, registeredWindow{
		start: policy.EffectiveDate, end: policy.EffectiveTo,
	}); err != nil {
		return err
	}
	versions = append(versions, policy)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].EffectiveDate.Before(versions[j].EffectiveDate)
	})
	e.currencyPolicies[policy.CompanyID] = versions
	return nil
}

func (e *PolicyEngine) PostingPolicyFor(companyID apiTypes.CompanyID, asOf time.Time) (PostingPolicy, error) {
	if e == nil {
		return PostingPolicy{}, ErrPolicyEngineRequired
	}
	asOf = normalizedPolicyTime(asOf)
	e.mu.RLock()
	defer e.mu.RUnlock()
	versions := e.policies[companyID]
	for i, candidate := range versions {
		var nextStart *time.Time
		if i+1 < len(versions) {
			start := versions[i+1].EffectiveDate
			nextStart = &start
		}
		if isWindowEffective(candidate.EffectiveDate, candidate.EffectiveTo, nextStart, asOf) {
			selected := candidate
			selected.EffectiveTo = clonedPolicyEnd(candidate.EffectiveTo)
			return selected, nil
		}
	}
	return PostingPolicy{}, ErrPolicyNotFound
}

func (e *PolicyEngine) CurrencyPolicyFor(companyID apiTypes.CompanyID, asOf time.Time) (CurrencyPolicy, error) {
	if e == nil {
		return CurrencyPolicy{}, ErrPolicyEngineRequired
	}
	asOf = normalizedPolicyTime(asOf)
	e.mu.RLock()
	defer e.mu.RUnlock()
	versions := e.currencyPolicies[companyID]
	for i, candidate := range versions {
		var nextStart *time.Time
		if i+1 < len(versions) {
			start := versions[i+1].EffectiveDate
			nextStart = &start
		}
		if isWindowEffective(candidate.EffectiveDate, candidate.EffectiveTo, nextStart, asOf) {
			selected := candidate
			selected.AllowedCurrencies = append([]string(nil), candidate.AllowedCurrencies...)
			selected.EffectiveTo = clonedPolicyEnd(candidate.EffectiveTo)
			return selected, nil
		}
	}
	return CurrencyPolicy{}, ErrPolicyNotFound
}

// ValidatePostingPolicy executes pre-posting compliance checks against company rules.
func (e *PolicyEngine) ValidatePostingPolicy(
	ctx context.Context,
	policy PostingPolicy,
	entry *journal.JournalEntry,
	accountTypes map[apiTypes.AccountID]masterdata.AccountType,
	now time.Time,
) error {
	if entry == nil {
		return errors.New("journal entry is nil")
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	if entry.CompanyID != policy.CompanyID || entry.EffectiveDate.IsZero() ||
		policy.EffectiveDate.After(entry.EffectiveDate) {
		return ErrPolicyNotEffective
	}

	// 1. Check Backdating Limit
	if policy.MaxBackdatingDays > 0 {
		cutoff := now.UTC().AddDate(0, 0, -policy.MaxBackdatingDays).Truncate(24 * time.Hour)
		entryDate := entry.EffectiveDate.UTC().Truncate(24 * time.Hour)
		if entryDate.Before(cutoff) && !policy.AllowBackdatingException {
			return fmt.Errorf("%w: entry date %s is older than %d days permitted",
				ErrBackdatingLimit, entryDate.Format("2006-01-02"), policy.MaxBackdatingDays)
		}
	}

	// 2. Check Cost Center and Analytical Dimensions requirements
	for _, line := range entry.Lines {
		accType, ok := accountTypes[line.AccountID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrAccountTypeNotFound, line.AccountID)
		}

		isIncomeExpense := accType == masterdata.AccountTypeIncome || accType == masterdata.AccountTypeExpense
		if isIncomeExpense {
			if policy.RequireCostCenterForIncomeExpense {
				if line.CostCenterID == nil || *line.CostCenterID == (apiTypes.CostCenterID{}) {
					return fmt.Errorf("%w: account %s requires a cost center", ErrCostCenterRequired, line.AccountID)
				}
			}
			if policy.RequireAnalyticalDimensionForIncomeExpense {
				hasDim := (line.ProjectID != nil && *line.ProjectID != (apiTypes.ProjectID{})) ||
					(line.SegmentID != nil && *line.SegmentID != (apiTypes.SegmentID{})) ||
					(line.BusinessUnitID != nil && *line.BusinessUnitID != (apiTypes.BusinessUnitID{}))
				if !hasDim {
					return fmt.Errorf("%w: account %s requires at least one analytical dimension", ErrAnalyticalDimensionLimit, line.AccountID)
				}
			}
		}
	}

	return nil
}
