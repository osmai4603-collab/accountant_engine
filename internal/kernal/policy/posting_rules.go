package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

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
}

func (p CurrencyPolicy) Validate() error {
	functionalCurrency := strings.ToUpper(strings.TrimSpace(p.FunctionalCurrency))
	if p.CompanyID == (apiTypes.CompanyID{}) || p.EffectiveDate.IsZero() {
		return ErrInvalidPolicy
	}
	if currency, ok := apiTypes.LookupCurrency(functionalCurrency); !ok || currency.Code == apiTypes.XXX {
		return ErrInvalidPolicy
	}
	if presentationCurrency := strings.ToUpper(strings.TrimSpace(p.PresentationCurrency)); presentationCurrency != "" {
		currency, ok := apiTypes.LookupCurrency(presentationCurrency)
		if !ok || currency.Code == apiTypes.XXX {
			return ErrInvalidPolicy
		}
	}
	if p.RoundingMethod != "" && p.RoundingMethod != RoundingBankers && p.RoundingMethod != RoundingHalfUp {
		return ErrInvalidPolicy
	}
	if p.AutoSettleRoundingDifferences && p.RoundingDifferenceAccountID == (apiTypes.AccountID{}) {
		return ErrInvalidPolicy
	}
	for _, code := range p.AllowedCurrencies {
		currency, ok := apiTypes.LookupCurrency(strings.ToUpper(strings.TrimSpace(code)))
		if !ok || currency.Code == apiTypes.XXX {
			return ErrInvalidPolicy
		}
	}
	return nil
}

func (p CurrencyPolicy) Round(amount apiTypes.Money) (apiTypes.Money, error) {
	currency, ok := apiTypes.LookupCurrency(amount.Currency())
	if !ok || currency.Code == apiTypes.XXX {
		return apiTypes.Money{}, apiTypes.ErrInvalidCurrency
	}
	rounded := amount.Amount().RoundBank(currency.MinorUnit)
	if p.RoundingMethod == RoundingHalfUp {
		rounded = amount.Amount().Round(currency.MinorUnit)
	}
	return apiTypes.NewMoney(rounded, string(currency.Code))
}

func (p CurrencyPolicy) AllowsCurrency(currency string) bool {
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
	ErrPolicyNotFound           = errors.New("accounting policy not found")
	ErrInvalidPolicy            = errors.New("invalid accounting policy")
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
}

func (p PostingPolicy) Validate() error {
	if p.CompanyID == (apiTypes.CompanyID{}) || p.EffectiveDate.IsZero() || p.MaxBackdatingDays < 0 {
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

func (e *PolicyEngine) SetPolicy(policy PostingPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, current := range e.policies[policy.CompanyID] {
		if current.EffectiveDate.Equal(policy.EffectiveDate) {
			return ErrInvalidPolicy
		}
	}
	e.policies[policy.CompanyID] = append(e.policies[policy.CompanyID], policy)
	return nil
}

func (e *PolicyEngine) SetCurrencyPolicy(policy CurrencyPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	policy.AllowedCurrencies = append([]string(nil), policy.AllowedCurrencies...)
	policy.FunctionalCurrency = strings.ToUpper(strings.TrimSpace(policy.FunctionalCurrency))
	policy.PresentationCurrency = strings.ToUpper(strings.TrimSpace(policy.PresentationCurrency))
	for i := range policy.AllowedCurrencies {
		policy.AllowedCurrencies[i] = strings.ToUpper(strings.TrimSpace(policy.AllowedCurrencies[i]))
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, current := range e.currencyPolicies[policy.CompanyID] {
		if current.EffectiveDate.Equal(policy.EffectiveDate) {
			return ErrInvalidPolicy
		}
	}
	e.currencyPolicies[policy.CompanyID] = append(e.currencyPolicies[policy.CompanyID], policy)
	return nil
}

func (e *PolicyEngine) PostingPolicyFor(companyID apiTypes.CompanyID, asOf time.Time) (PostingPolicy, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var selected PostingPolicy
	found := false
	for _, candidate := range e.policies[companyID] {
		if !candidate.EffectiveDate.After(asOf) &&
			(!found || candidate.EffectiveDate.After(selected.EffectiveDate)) {
			selected, found = candidate, true
		}
	}
	if found {
		return selected, nil
	}
	return PostingPolicy{}, ErrPolicyNotFound
}

func (e *PolicyEngine) CurrencyPolicyFor(companyID apiTypes.CompanyID, asOf time.Time) (CurrencyPolicy, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var selected CurrencyPolicy
	found := false
	for _, candidate := range e.currencyPolicies[companyID] {
		if !candidate.EffectiveDate.After(asOf) &&
			(!found || candidate.EffectiveDate.After(selected.EffectiveDate)) {
			selected, found = candidate, true
		}
	}
	if found {
		selected.AllowedCurrencies = append([]string(nil), selected.AllowedCurrencies...)
		return selected, nil
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
