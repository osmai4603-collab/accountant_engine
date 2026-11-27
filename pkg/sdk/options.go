package sdk

import (
	"errors"
	"reflect"
	"time"

	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	publicsubledger "github.com/osmai4603-collab/accounting_engine/pkg/spi/subledger"
)

type Option func(*EngineBuilder) error

// WithStoragePorts configures the engine from the stable public storage ports.
// All required ports are validated during Build before any module is initialized.
func WithStoragePorts(ports storage.Ports) Option {
	return func(b *EngineBuilder) error {
		b.ports = ports
		b.portsConfigured = true
		return nil
	}
}

// WithReporting enables reporting capabilities. Reporting is optional; when it
// is not configured, reporting calls fail explicitly with ErrCapabilityNotImplemented.
func WithReporting(store storage.ReportingStore) Option {
	return func(b *EngineBuilder) error {
		if store == nil {
			return errors.New("reporting store is nil")
		}
		b.reportingStore = store
		return nil
	}
}

// WithGovernance enables governance capabilities. Governance is optional; when
// it is not configured, governance calls fail explicitly with ErrCapabilityNotImplemented.
func WithGovernance(store storage.GovernanceStore) Option {
	return func(b *EngineBuilder) error {
		if store == nil {
			return errors.New("governance store is nil")
		}
		b.governanceStore = store
		return nil
	}
}

// WithAuthorizer configures a host-supplied authorizer for access control.
func WithAuthorizer(authorizer publicapplication.Authorizer) Option {
	return func(b *EngineBuilder) error {
		if authorizer == nil {
			return errors.New("authorizer is nil")
		}
		b.authorizer = authorizer
		return nil
	}
}

// WithPostingConfig declares posting controls that must be available before
// the engine can be built.
func WithPostingConfig(config PostingConfig) Option {
	return func(b *EngineBuilder) error {
		b.postingConfig = config
		return nil
	}
}

// WithAllowSyntheticDefaultJournal enables the legacy "GENERAL" fallback when
// storage.MasterDataReader does not implement masterdata.JournalRepository.
func WithAllowSyntheticDefaultJournal() Option {
	return func(b *EngineBuilder) error {
		b.postingConfig.AllowSyntheticDefaultJournal = true
		return nil
	}
}

// WithPolicyCacheTTL configures how long loaded policy versions stay cached
// per company before reloading from storage.PolicyStore. Zero caches until
// explicit invalidation via PolicyCacheManager.
func WithPolicyCacheTTL(ttl time.Duration) Option {
	return func(b *EngineBuilder) error {
		if ttl < 0 {
			return errors.New("policy cache TTL must not be negative")
		}
		b.postingConfig.PolicyCacheTTL = ttl
		return nil
	}
}

// WithPostingCapabilities supplies optional controls used by PostingConfig.
func WithPostingCapabilities(capabilities storage.PostingCapabilities) Option {
	return func(b *EngineBuilder) error {
		b.postingCaps = capabilities
		return nil
	}
}

// WithEventIdempotencyWaitPolicy bounds how long ProcessEvent waits for a
// concurrent owner of the same idempotency key to finish before recovering
// the committed result or reporting ErrIdempotencyInProgress.
func WithEventIdempotencyWaitPolicy(timeout, interval time.Duration) Option {
	return func(b *EngineBuilder) error {
		if timeout < 0 {
			return errors.New("event idempotency wait timeout must not be negative")
		}
		if interval < 0 {
			return errors.New("event idempotency wait interval must not be negative")
		}
		b.eventWaitTimeout = timeout
		b.eventWaitInterval = interval
		return nil
	}
}

// WithStandard registers an international standard module (e.g. IFRS 15, IFRS 16).
func WithStandard(module standards.StandardModule) Option {
	return func(b *EngineBuilder) error {
		if module == nil {
			return errors.New("standard module is nil")
		}
		b.standards = append(b.standards, module)
		return nil
	}
}

// WithSubledgerARAP enables the optional receivables and payables capability.
func WithSubledgerARAP(stores publicsubledger.ARAPStores) Option {
	return func(b *EngineBuilder) error {
		if isNilDependency(stores.Receivables) || isNilDependency(stores.Payables) || isNilDependency(stores.Balance) {
			return errors.New("AR/AP requires receivable, payable and balance stores")
		}
		b.arapStores = stores
		b.arapStoresConfigured = true
		return nil
	}
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
