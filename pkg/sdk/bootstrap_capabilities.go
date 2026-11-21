package sdk

import (
	"fmt"
)

// BootstrapCapability names an optional engine capability that a host can
// require at build time. Requiring a capability enforces:
//
//	Enabled(capability) => Available(capability)
//
// Build fails fast with an actionable error when the dependency is missing, so
// the engine never starts in a partially configured state.
type BootstrapCapability string

const (
	BootstrapReporting      BootstrapCapability = "reporting"
	BootstrapGovernance     BootstrapCapability = "governance"
	BootstrapStandards      BootstrapCapability = "standards"
	BootstrapEventIngestion BootstrapCapability = "event_ingestion"
	BootstrapSubledgers     BootstrapCapability = "subledgers"
	BootstrapPostingPolicy  BootstrapCapability = "posting_policy"
)

func (c BootstrapCapability) valid() bool {
	switch c {
	case BootstrapReporting, BootstrapGovernance, BootstrapStandards,
		BootstrapEventIngestion, BootstrapSubledgers, BootstrapPostingPolicy:
		return true
	default:
		return false
	}
}

// WithRequiredCapabilities declares capabilities that must be fully available
// before the engine is built. It is a fail-fast contract, not an activation
// switch: dependencies must still be supplied through their own options.
func WithRequiredCapabilities(capabilities ...BootstrapCapability) Option {
	return func(b *EngineBuilder) error {
		for _, capability := range capabilities {
			if !capability.valid() {
				return fmt.Errorf("bootstrap: unknown required capability %q", capability)
			}
			if b.hasRequiredCapability(capability) {
				continue
			}
			b.requiredCapabilities = append(b.requiredCapabilities, capability)
		}
		return nil
	}
}

func (b *EngineBuilder) hasRequiredCapability(capability BootstrapCapability) bool {
	for _, existing := range b.requiredCapabilities {
		if existing == capability {
			return true
		}
	}
	return false
}

// validateRequiredCapabilities checks every explicitly required capability
// against the configured dependency sources. It reports all missing
// dependencies at once with the option or port needed to fix each one.
func (b *EngineBuilder) validateRequiredCapabilities() []error {
	var errs []error
	for _, capability := range b.requiredCapabilities {
		var err error
		switch capability {
		case BootstrapReporting:
			err = b.validateReportingAvailability()
		case BootstrapGovernance:
			err = b.validateGovernanceAvailability()
		case BootstrapStandards:
			err = b.validateStandardsAvailability()
		case BootstrapEventIngestion:
			err = b.validateEventIngestionAvailability()
		case BootstrapSubledgers:
			err = b.validateSubledgersAvailability()
		case BootstrapPostingPolicy:
			err = b.validatePostingPolicyAvailability()
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (b *EngineBuilder) validateReportingAvailability() error {
	if b.reportingStore != nil {
		return nil
	}
	if b.portsConfigured && b.ports.Balance != nil && b.ports.MasterData != nil && b.ports.Entries != nil {
		return nil
	}
	return fmt.Errorf("bootstrap: capability %q required but reporting store or reporting ports are missing; configure WithReporting or provide Balance, MasterData and Entry ports", BootstrapReporting)
}

func (b *EngineBuilder) validateGovernanceAvailability() error {
	if b.governanceStore != nil {
		return nil
	}
	if b.portsConfigured && b.ports.Audit != nil {
		return nil
	}
	return fmt.Errorf("bootstrap: capability %q required but governance store or audit port is missing; configure WithGovernance or provide Audit port", BootstrapGovernance)
}

func (b *EngineBuilder) validateStandardsAvailability() error {
	if len(b.standards) > 0 {
		return nil
	}
	return fmt.Errorf("bootstrap: capability %q required but no standard modules configured; use WithStandard", BootstrapStandards)
}

func (b *EngineBuilder) validateEventIngestionAvailability() error {
	if b.portsConfigured && b.ports.Events.Idempotency != nil && b.ports.Events.Mappings != nil {
		return nil
	}
	return fmt.Errorf("bootstrap: capability %q required but event idempotency and mapping stores are missing; supply Ports.Events", BootstrapEventIngestion)
}

func (b *EngineBuilder) validateSubledgersAvailability() error {
	if b.arapStoresConfigured {
		return nil
	}
	return fmt.Errorf("bootstrap: capability %q required but AR/AP stores not configured; use WithSubledgerARAP", BootstrapSubledgers)
}

func (b *EngineBuilder) validatePostingPolicyAvailability() error {
	if b.portsConfigured && b.ports.Private != nil && b.ports.Private["accounting_engine.posting_policy"] != nil {
		return nil
	}
	if b.postingCaps.MakerCheckerPolicy != nil || b.postingCaps.ApprovalGate != nil || b.postingCaps.FinancialDimensions != nil {
		return nil
	}
	return fmt.Errorf("bootstrap: capability %q required but posting policy controls are missing", BootstrapPostingPolicy)
}

