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

// InvalidateCompanyPolicies clears cached policy versions for companyID so the
// next posting reloads fresh versions from storage.PolicyStore.
func (s *applicationService) InvalidateCompanyPolicies(companyID types.CompanyID) {
	lock := s.companyPolicyLock(companyID)
	lock.Lock()
	defer lock.Unlock()
	delete(s.policiesLoaded, companyID)
	delete(s.policyLoadedAt, companyID)
	delete(s.policyVersionDigests, companyID)
	delete(s.syntheticPolicies, companyID)
	if s.policyEngine != nil {
		s.policyEngine.ClearCompanyPolicies(companyID)
	}
}

// ReloadCompanyPolicies immediately fetches and validates stored policies for
// companyID, clearing the cached engine state only if loading fails or versions changed.
func (s *applicationService) ReloadCompanyPolicies(ctx context.Context, companyID types.CompanyID) error {
	lock := s.companyPolicyLock(companyID)
	lock.Lock()
	defer lock.Unlock()
	return s.loadVersionedPoliciesLocked(ctx, companyID, true)
}

// PolicyCacheManager allows hosts that mutate PolicyStore out-of-band to flush
// or reload cached per-company policy versions explicitly.
type PolicyCacheManager interface {
	InvalidateCompanyPolicies(companyID types.CompanyID)
	ReloadCompanyPolicies(ctx context.Context, companyID types.CompanyID) error
}

// loadVersionedPolicies registers every stored version for a company. When
// PolicyCacheTTL > 0 and elapsed, versions are re-checked and reloaded only if
// the stored set changed; when PolicyCacheTTL == 0 versions stay cached until
// InvalidateCompanyPolicies or ReloadCompanyPolicies is called.
func (s *applicationService) loadVersionedPolicies(ctx context.Context, companyID types.CompanyID) error {
	forceRefresh := false
	if s.config.PolicyCacheTTL > 0 && s.policiesLoaded[companyID] {
		if loadedAt, ok := s.policyLoadedAt[companyID]; !ok || time.Since(loadedAt) >= s.config.PolicyCacheTTL {
			forceRefresh = true
		}
	}
	return s.loadVersionedPoliciesLocked(ctx, companyID, forceRefresh)
}

func (s *applicationService) loadVersionedPoliciesLocked(ctx context.Context, companyID types.CompanyID, forceRefresh bool) error {
	if s.policyStore == nil {
		return errors.New("sdk: policy store is not configured")
	}
	if s.policiesLoaded == nil {
		s.policiesLoaded = make(map[types.CompanyID]bool)
	}
	if s.policyLoadedAt == nil {
		s.policyLoadedAt = make(map[types.CompanyID]time.Time)
	}
	if s.policyVersionDigests == nil {
		s.policyVersionDigests = make(map[types.CompanyID]string)
	}
	if s.policiesLoaded[companyID] && !forceRefresh {
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
		delete(s.policiesLoaded, companyID)
		delete(s.policyLoadedAt, companyID)
		delete(s.policyVersionDigests, companyID)
		if forceRefresh {
			s.policyEngine.ClearCompanyPolicies(companyID)
		}
		return fmt.Errorf("company %s: %w", companyID.String(), errNoVersionedPolicies)
	}

	digest := policyVersionsDigest(postingVersions, currencyVersions)
	if s.policiesLoaded[companyID] && digest == s.policyVersionDigests[companyID] {
		s.policyLoadedAt[companyID] = time.Now().UTC()
		return nil
	}

	convertedPosting := make([]policy.PostingPolicy, 0, len(postingVersions))
	for _, version := range postingVersions {
		converted, err := postingPolicyFromVersion(version)
		if err != nil {
			return err
		}
		convertedPosting = append(convertedPosting, converted)
	}
	convertedCurrency := make([]policy.CurrencyPolicy, 0, len(currencyVersions))
	for _, version := range currencyVersions {
		converted, err := currencyPolicyFromVersion(version)
		if err != nil {
			return err
		}
		convertedCurrency = append(convertedCurrency, converted)
	}

	// Clear any prior synthetic or stale policy versions before applying the
	// newly fetched snapshot so overlapping or removed windows cannot persist.
	s.policyEngine.ClearCompanyPolicies(companyID)
	delete(s.syntheticPolicies, companyID)

	// Registration is all-or-nothing from the engine's point of view. A
	// conversion or registration failure must not leave earlier versions
	// behind: with policiesLoaded still false, the next call retries cleanly.
	rollback := func(err error) error {
		s.policyEngine.ClearCompanyPolicies(companyID)
		delete(s.policiesLoaded, companyID)
		delete(s.policyLoadedAt, companyID)
		delete(s.policyVersionDigests, companyID)
		return err
	}

	for i, converted := range convertedPosting {
		if err := s.policyEngine.SetPolicy(converted); err != nil {
			return rollback(fmt.Errorf("register stored posting policy version %d: %w", postingVersions[i].Version, err))
		}
	}
	for i, converted := range convertedCurrency {
		if err := s.policyEngine.SetCurrencyPolicy(converted); err != nil {
			return rollback(fmt.Errorf("register stored currency policy version %d: %w", currencyVersions[i].Version, err))
		}
	}

	s.policiesLoaded[companyID] = true
	s.policyLoadedAt[companyID] = time.Now().UTC()
	s.policyVersionDigests[companyID] = digest
	return nil
}

func policyVersionsDigest(postingVersions, currencyVersions []storage.PolicyVersion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "p=%d:", len(postingVersions))
	for _, v := range postingVersions {
		fmt.Fprintf(&b, "%s/%d/%d;", v.PolicyID.String(), v.Version, v.EffectiveFrom.UTC().UnixNano())
	}
	fmt.Fprintf(&b, "|c=%d:", len(currencyVersions))
	for _, v := range currencyVersions {
		fmt.Fprintf(&b, "%s/%d/%d;", v.PolicyID.String(), v.Version, v.EffectiveFrom.UTC().UnixNano())
	}
	return b.String()
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
