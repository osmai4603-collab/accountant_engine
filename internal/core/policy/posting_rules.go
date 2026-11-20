package policy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	apiTypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

var (
	ErrPolicyNotFound           = errors.New("accounting policy not found")
	ErrInvalidPolicy            = errors.New("invalid accounting policy")
	ErrCurrencyNotAllowed       = errors.New("currency is not allowed by company policy")
	ErrRoundingMismatch         = errors.New("amount does not conform to currency rounding policy")
	ErrBackdatingLimit          = errors.New("journal entry exceeds the permitted backdating threshold")
	ErrCostCenterRequired       = errors.New("cost center is required for income and expense lines")
	ErrAnalyticalDimensionLimit = errors.New("analytical dimension required by company policy")
)

type PostingPolicy struct {
	CompanyID                                  apiTypes.CompanyID
	MaxBackdatingDays                          int
	AllowBackdatingException                   bool
	AllowSoftClosedPosting                     bool
	RequireManagerForSoftClosed                bool
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
	policies map[apiTypes.CompanyID]PostingPolicy
}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies: make(map[apiTypes.CompanyID]PostingPolicy),
	}
}

func (e *PolicyEngine) SetPolicy(policy PostingPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	e.policies[policy.CompanyID] = policy
	return nil
}

// ValidatePostingPolicy executes pre-posting compliance checks against company rules.
func (e *PolicyEngine) ValidatePostingPolicy(
	ctx context.Context,
	policy PostingPolicy,
	entry *ledger.JournalEntry,
	accountTypes map[apiTypes.AccountID]masterdata.AccountType,
	now time.Time,
) error {
	if entry == nil {
		return errors.New("journal entry is nil")
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
			continue
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
