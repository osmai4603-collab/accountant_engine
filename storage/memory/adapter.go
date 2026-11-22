package memory

import (
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// Adapter is the top-level memory storage adapter.
type Adapter struct {
	driver *Driver
}

// New creates a clean, isolated in-memory storage adapter.
func New() *Adapter {
	return &Adapter{driver: newDriver()}
}

// Ports exports the complete storage ports implementing pkg/spi/storage.Ports.
func (a *Adapter) Ports() storage.Ports {
	if a == nil || a.driver == nil {
		return storage.Ports{}
	}
	t := transactionAwareDriver{driver: a.driver}
	return storage.Ports{
		Drafts:      t,
		Entries:     t,
		Posting:     t,
		Sequences:   t,
		MasterData:  t,
		Balance:     t,
		Audit:       t,
		Idempotency: t,
		TxRunner:    a.driver,
		UOWRunner:   a.driver,
		Events: publicevents.Stores{
			Idempotency: a.driver,
			Mappings:    a.driver,
		},
		Private: storage.PrivateExtensions{
			storage.PostingLockerExtensionKey:    a.driver,
			storage.AuditConformanceExtensionKey: a.driver,
			// Transaction-aware so policy appends join and roll back with the
			// active unit of work.
			storage.PolicyStoreExtensionKey: t,
		},
	}
}

// Capabilities declares adapter capabilities with GuaranteesDurability=false.
func (a *Adapter) Capabilities() storage.AdapterCapabilities {
	if a == nil || a.driver == nil {
		return storage.AdapterCapabilities{}
	}
	return a.driver.Capabilities()
}

// MasterDataStore exposes the master data provisioning store.
func (a *Adapter) MasterDataStore() publicmasterdata.Store {
	if a == nil || a.driver == nil {
		return nil
	}
	return a.driver
}

// Driver returns the underlying memory driver for test inspection.
func (a *Adapter) Driver() *Driver {
	if a == nil {
		return nil
	}
	return a.driver
}

// Close is a no-op for the memory adapter.
func (a *Adapter) Close() error {
	return nil
}
