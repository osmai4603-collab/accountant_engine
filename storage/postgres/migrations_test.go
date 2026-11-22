package postgres

import (
	"path/filepath"
	"testing"
)

func TestEveryMigrationHasRollback(t *testing.T) {
	directory, err := migrationDirectory()
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := listMigrations(directory)
	if err != nil {
		t.Fatal(err)
	}
	// Phase 10 established 34 migrations; later phases may append more, but the
	// chain must never shrink below the audited Phase 10 baseline.
	if len(migrations) < 34 {
		t.Fatalf("expected at least 34 migrations in dependency chain, got %d", len(migrations))
	}
	for _, migration := range migrations {
		if migration.downPath == "" {
			t.Errorf("migration %s has no rollback script", filepath.Base(migration.upPath))
		}
		if _, err := embeddedMigrations.ReadFile(migration.upPath); err != nil {
			t.Errorf("embedded up migration %s: %v", migration.upPath, err)
		}
		if _, err := embeddedMigrations.ReadFile(migration.downPath); err != nil {
			t.Errorf("embedded down migration %s: %v", migration.downPath, err)
		}
	}
}
