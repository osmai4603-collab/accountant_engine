package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/policy"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func policyStoreFromPorts(ports storage.Ports) storage.PolicyStore {
	raw, ok := ports.Private[storage.PolicyStoreExtensionKey]
	if !ok || raw == nil {
		return nil
	}
	store, ok := raw.(storage.PolicyStore)
	if !ok {
		return nil
	}
	return store
}

func clonePolicyEnd(end *time.Time) *time.Time {
	if end == nil {
		return nil
	}
	value := end.UTC()
	return &value
}

func postingPolicyFromVersion(version storage.PolicyVersion) (policy.PostingPolicy, error) {
	var payload storage.PostingPolicyPayload
	if err := json.Unmarshal(version.Payload, &payload); err != nil {
		return policy.PostingPolicy{}, fmt.Errorf("decode posting policy version %d: %w", version.Version, err)
	}
	result := policy.PostingPolicy{
		CompanyID:                                  version.CompanyID,
		MaxBackdatingDays:                          payload.MaxBackdatingDays,
		AllowBackdatingException:                   payload.AllowBackdatingException,
		AllowSoftClosedPosting:                     payload.AllowSoftClosedPosting,
		RequireCostCenterForIncomeExpense:          payload.RequireCostCenterForIncomeExpense,
		RequireAnalyticalDimensionForIncomeExpense: payload.RequireAnalyticalDimensionForIncomeExpense,
		EffectiveDate:                              version.EffectiveFrom.UTC(),
		EffectiveTo:                                clonePolicyEnd(version.EffectiveTo),
	}
	if err := result.Validate(); err != nil {
		return policy.PostingPolicy{}, fmt.Errorf("invalid stored posting policy version %d: %w", version.Version, err)
	}
	return result, nil
}

func currencyPolicyFromVersion(version storage.PolicyVersion) (policy.CurrencyPolicy, error) {
	var payload storage.CurrencyPolicyPayload
	if err := json.Unmarshal(version.Payload, &payload); err != nil {
		return policy.CurrencyPolicy{}, fmt.Errorf("decode currency policy version %d: %w", version.Version, err)
	}
	result := policy.CurrencyPolicy{
		CompanyID:                     version.CompanyID,
		FunctionalCurrency:            strings.ToUpper(strings.TrimSpace(payload.FunctionalCurrency)),
		PresentationCurrency:          strings.ToUpper(strings.TrimSpace(payload.PresentationCurrency)),
		RoundingMethod:                policy.RoundingMethod(payload.RoundingMethod),
		AllowedCurrencies:             append([]string(nil), payload.AllowedCurrencies...),
		AutoSettleRoundingDifferences: payload.AutoSettleRoundingDifferences,
		EffectiveDate:                 version.EffectiveFrom.UTC(),
		EffectiveTo:                   clonePolicyEnd(version.EffectiveTo),
	}
	if trimmed := strings.TrimSpace(payload.RoundingDifferenceAccountID); trimmed != "" {
		parsed, err := uuid.Parse(trimmed)
		if err != nil {
			return policy.CurrencyPolicy{}, fmt.Errorf("invalid rounding difference account in policy version %d: %w", version.Version, err)
		}
		result.RoundingDifferenceAccountID = types.AccountID(parsed)
	}
	if err := result.Validate(); err != nil {
		return policy.CurrencyPolicy{}, fmt.Errorf("invalid stored currency policy version %d: %w", version.Version, err)
	}
	return result, nil
}

// errNoVersionedPolicies signals that the configured store holds no policy
// versions at all for a company. That tenant has not opted into versioned
// policies, so the legacy derivation remains available. As soon as one version
// exists the store is authoritative and uncovered dates fail closed.
var errNoVersionedPolicies = errors.New("sdk: no versioned policies configured for company")

// loadVersionedPolicies registers every stored version exactly once for a
// company. Registering only the version effective today would make historical
// postings pick a different policy and change their fingerprint.
func (s *applicationService) loadVersionedPolicies(ctx context.Context, companyID types.CompanyID) error {
	if s.policyStore == nil {
		return errors.New("sdk: policy store is not configured")
	}
	if s.policiesLoaded == nil {
		s.policiesLoaded = make(map[types.CompanyID]bool)
	}
	if s.policiesLoaded[companyID] {
		return nil
	}

	postingVersions, err := s.policyStore.ListVersions(ctx, companyID, storage.PolicyKindPosting)
	if err != nil {
		return fmt.Errorf("list posting policy versions: %w", err)
	}
	currencyVersions, err := s.policyStore.ListVersions(ctx, companyID, storage.PolicyKindCurrency)
	if err != nil {
		return fmt.Errorf("list currency policy versions: %w", err)
	}
	if len(postingVersions) == 0 && len(currencyVersions) == 0 {
		return fmt.Errorf("company %s: %w", companyID.String(), errNoVersionedPolicies)
	}

	// The store has just become authoritative. Any legacy fallback registered
	// earlier for this company must be removed first: otherwise its synthetic
	// 2000-01-01 window would keep answering dates the store does not cover,
	// and a first stored version starting on that date would collide with it.
	if s.syntheticPolicies[companyID] {
		s.policyEngine.ClearCompanyPolicies(companyID)
		delete(s.syntheticPolicies, companyID)
	}

	// Registration is all-or-nothing from the engine's point of view. A
	// conversion or registration failure must not leave earlier versions
	// behind: with policiesLoaded still false, the next call would try to
	// register them again and fail forever on duplicate effective dates.
	rollback := func(err error) error {
		s.policyEngine.ClearCompanyPolicies(companyID)
		return err
	}

	for _, version := range postingVersions {
		converted, err := postingPolicyFromVersion(version)
		if err != nil {
			return rollback(err)
		}
		if err := s.policyEngine.SetPolicy(converted); err != nil {
			return rollback(fmt.Errorf("register stored posting policy version %d: %w", version.Version, err))
		}
	}
	for _, version := range currencyVersions {
		converted, err := currencyPolicyFromVersion(version)
		if err != nil {
			return rollback(err)
		}
		if err := s.policyEngine.SetCurrencyPolicy(converted); err != nil {
			return rollback(fmt.Errorf("register stored currency policy version %d: %w", version.Version, err))
		}
	}

	s.policiesLoaded[companyID] = true
	return nil
}

// ensureVersionedPoliciesIsEffective fails closed when the authoritative
// store has no version covering the posting date. No synthetic policy is
// created in this mode.
func (s *applicationService) ensureVersionedPoliciesIsEffective(companyID types.CompanyID, asOf time.Time) error {
	if _, err := s.policyEngine.PostingPolicyFor(companyID, asOf); err != nil {
		return fmt.Errorf("stored posting policy: %w", err)
	}
	if _, err := s.policyEngine.CurrencyPolicyFor(companyID, asOf); err != nil {
		return fmt.Errorf("stored currency policy: %w", err)
	}
	return nil
}
