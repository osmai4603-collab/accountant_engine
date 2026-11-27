package sdk

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// PostingConfig declares the posting controls that are required for an SDK
// engine. Requirements are checked during Build; they are not enabled by
// setting these flags alone.
type PostingConfig struct {
	RequireApproval            bool
	RequireFinancialDimensions bool
	RequireMakerCheckerPolicy  bool
	RequirePolicy              bool
	// AllowSyntheticDefaultJournal explicitly opts in to the legacy fallback
	// that synthesizes a "GENERAL" journal when MasterDataReader does not
	// implement masterdata.JournalRepository. Disabled (fail-closed) by default.
	AllowSyntheticDefaultJournal bool
	// PolicyCacheTTL bounds how long versioned policies remain cached per company
	// before reloading from storage.PolicyStore. Zero caches until invalidated.
	PolicyCacheTTL time.Duration
}

func (c PostingConfig) requiresCapabilities() bool {
	return c.RequireApproval || c.RequireFinancialDimensions ||
		c.RequireMakerCheckerPolicy || c.RequirePolicy
}

func isNilCapability(capability any) bool {
	if capability == nil {
		return true
	}
	value := reflect.ValueOf(capability)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (b *EngineBuilder) validatePostingConfig() error {
	var missing []string
	if b.postingConfig.RequireApproval && isNilCapability(b.postingCaps.ApprovalGate) {
		missing = append(missing, "approval")
	}
	if b.postingConfig.RequireFinancialDimensions && isNilCapability(b.postingCaps.FinancialDimensions) {
		missing = append(missing, "financial dimensions")
	}
	if b.postingConfig.RequireMakerCheckerPolicy && isNilCapability(b.postingCaps.MakerCheckerPolicy) {
		missing = append(missing, "maker-checker policy")
	}
	if b.postingConfig.RequirePolicy {
		// check if policy unit or private posting policy extension is provided
		hasPolicy := false
		if b.portsConfigured && b.ports.Private != nil && b.ports.Private["accounting_engine.posting_policy"] != nil {
			hasPolicy = true
		}
		if !hasPolicy {
			missing = append(missing, "posting policy")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required posting capability is unavailable: %s", strings.Join(missing, ", "))
	}
	return nil
}
