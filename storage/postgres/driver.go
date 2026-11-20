package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var (
	ErrDriverNotInitialized = errors.New("postgres: driver is not initialized")
	ErrNotImplemented       = errors.New("postgres: operation not implemented")
)

// Driver manages the PostgreSQL connection pool and transaction lifecycle.
type Driver struct {
	pool *pgxpool.Pool
}

// NewDriver connects to PostgreSQL, verifies connectivity, applies pending migrations,
// seeds the currency registry, and returns a live Driver.
func NewDriver(ctx context.Context, connectionString string) (*Driver, error) {
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := EnsureSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	if err := seedCurrencyRegistry(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Driver{pool: pool}, nil
}

// NewDriverWithPool wraps an existing pgxpool.Pool.
func NewDriverWithPool(pool *pgxpool.Pool) *Driver {
	return &Driver{pool: pool}
}

// EnsureSchema executes all pending migrations up to the latest version.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	return MigrateUp(ctx, pool)
}

// Pool returns the underlying connection pool.
func (d *Driver) Pool() *pgxpool.Pool {
	if d == nil {
		return nil
	}
	return d.pool
}

// Ping verifies database connectivity.
func (d *Driver) Ping(ctx context.Context) error {
	if d == nil || d.pool == nil {
		return ErrDriverNotInitialized
	}
	return d.pool.Ping(ctx)
}

// Close releases the connection pool resources.
func (d *Driver) Close() {
	if d != nil && d.pool != nil {
		d.pool.Close()
	}
}

// seedCurrencyRegistry initializes ISO 4217 currencies from the engine's types package.
func seedCurrencyRegistry(ctx context.Context, pool *pgxpool.Pool) error {
	currencies := types.AllCurrencies()
	if len(currencies) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, c := range currencies {
		batch.Queue(`INSERT INTO currencies(code, numeric_code, name, minor_units)
			VALUES($1, $2, $3, $4)
			ON CONFLICT(code) DO UPDATE SET
				numeric_code=EXCLUDED.numeric_code,
				name=EXCLUDED.name,
				minor_units=EXCLUDED.minor_units`,
			c.Code, c.NumericCode, c.Name, c.MinorUnit)
	}
	results := pool.SendBatch(ctx, batch)
	defer results.Close()
	for range currencies {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return results.Close()
}

// Capabilities declares the production capabilities guaranteed by PostgreSQL.
func (d *Driver) Capabilities() storage.AdapterCapabilities {
	return storage.AdapterCapabilities{
		AdapterName:            "postgres",
		SupportsRepeatableRead: true,
		SupportsSerializable:   true,
		EnforcesForeignKeys:    true,
		GuaranteesDurability:   true,
		SupportsSavepoints:     false,
		DetectsDeadlocks:       true,
	}
}

func nullableString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value
}
