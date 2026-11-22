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

	// 1. All migrations are applied by newIsolatedPostgresDriver. The count is
	// read from the schema so later phases may append migrations without
	// invalidating the Phase 10 up/down-cycle evidence.
	var count, total int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	total = count
	if total < 34 {
		t.Fatalf("expected at least 34 applied migrations, got %d", total)
	}

	// 2. Migrate down every applied step
	if err := postgres.MigrateDown(ctx, pool, total); err != nil {
		t.Fatalf("MigrateDown(%d): %v", total, err)
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
	if count != total {
		t.Fatalf("expected %d applied migrations after re-apply, got %d", total, count)
	}

	// Ensure idempotent repeated MigrateUp
	if err := postgres.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("second MigrateUp should be idempotent: %v", err)
	}
}
