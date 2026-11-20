package postgres_test

import (
	"context"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/storage/postgres"
)

func TestMigrateUpAndMigrateDownCycle(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	pool := driver.Pool()

	// 1. All 32 migrations are applied by newIsolatedPostgresDriver
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 32 {
		t.Fatalf("expected 32 applied migrations, got %d", count)
	}

	// 2. Migrate down all 32 steps
	if err := postgres.MigrateDown(ctx, pool, 32); err != nil {
		t.Fatalf("MigrateDown(32): %v", err)
	}

	// Verify schema_migrations is now empty
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations after MigrateDown: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 migrations after MigrateDown, got %d", count)
	}

	// 3. Re-apply MigrateUp
	if err := postgres.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("re-applying MigrateUp: %v", err)
	}

	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations after re-apply: %v", err)
	}
	if count != 32 {
		t.Fatalf("expected 32 applied migrations after re-apply, got %d", count)
	}

	// Ensure idempotent repeated MigrateUp
	if err := postgres.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("second MigrateUp should be idempotent: %v", err)
	}
}
