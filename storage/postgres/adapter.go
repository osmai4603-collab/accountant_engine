package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// Adapter is the PostgreSQL production storage adapter.
type Adapter struct {
	driver *Driver
}

// New connects to PostgreSQL, runs migrations, seeds currencies, and returns an initialized Adapter.
func New(ctx context.Context, connectionString string) (*Adapter, error) {
	driver, err := NewDriver(ctx, connectionString)
	if err != nil {
		return nil, err
	}
	return &Adapter{driver: driver}, nil
}

// NewWithPool wraps an existing pgxpool.Pool in an Adapter.
func NewWithPool(pool *pgxpool.Pool) *Adapter {
	return &Adapter{driver: NewDriverWithPool(pool)}
}

// SetObserver installs an optional operational Observer on the adapter's
// driver. A nil observer restores the no-op default.
func (a *Adapter) SetObserver(observer publicapplication.Observer) {
	if a != nil && a.driver != nil {
		a.driver.SetObserver(observer)
	}
}

// Ports exports the complete set of storage ports implementing pkg/spi/storage.Ports.
func (a *Adapter) Ports() storage.Ports {
	if a == nil || a.driver == nil {
		return storage.Ports{}
	}
	return storage.Ports{
		Drafts:      a.driver,
		Entries:     a.driver,
		Posting:     a.driver,
		Sequences:   a.driver,
		MasterData:  a.driver,
		Balance:     a.driver,
		Audit:       a.driver,
		Idempotency: a.driver,
		TxRunner:    a.driver,
		UOWRunner:   a.driver,
		Events: publicevents.Stores{
			Idempotency: a.driver,
			Mappings:    a.driver,
		},
		Private: storage.PrivateExtensions{
			storage.PostingLockerExtensionKey:    a.driver,
			storage.AuditConformanceExtensionKey: a.driver,
			storage.PolicyStoreExtensionKey:      a.driver,
		},
	}
}

// Capabilities declares PostgreSQL production capabilities (GuaranteesDurability=true).
func (a *Adapter) Capabilities() storage.AdapterCapabilities {
	if a == nil || a.driver == nil {
		return storage.AdapterCapabilities{}
	}
	return a.driver.Capabilities()
}

// MasterDataStore exposes the master data store.
func (a *Adapter) MasterDataStore() publicmasterdata.Store {
	if a == nil || a.driver == nil {
		return nil
	}
	return a.driver
}

// Ping checks database health.
func (a *Adapter) Ping(ctx context.Context) error {
	if a == nil || a.driver == nil {
		return ErrDriverNotInitialized
	}
	return a.driver.Ping(ctx)
}

// Close releases connection pool resources.
func (a *Adapter) Close() {
	if a != nil && a.driver != nil {
		a.driver.Close()
	}
}

// Driver returns the underlying driver.
func (a *Adapter) Driver() *Driver {
	if a == nil {
		return nil
	}
	return a.driver
}
