package contract_parity

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	publicmasterdata "github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/memory"
	"github.com/osmai4603-collab/accounting_engine/storage/postgres"
)

// StorageAdapterHarness wraps an adapter under test, exposing its storage.Ports,
// MasterData store, and declared Capabilities.
type StorageAdapterHarness struct {
	Name         string
	Ports        storage.Ports
	MasterData   publicmasterdata.Store
	Capabilities storage.AdapterCapabilities
	Cleanup      func()
}

// NewMemoryStorageHarness provisions and wires an in-memory adapter for testing.
func NewMemoryStorageHarness(t *testing.T, f UnifiedFixture) *StorageAdapterHarness {
	t.Helper()
	ctx := context.Background()

	drv := memory.New()
	mdStore := drv.MasterDataStore()
	if err := ProvisionFixture(ctx, mdStore, f); err != nil {
		t.Fatalf("memory: provision fixture: %v", err)
	}

	return &StorageAdapterHarness{
		Name:         "memory",
		Ports:        drv.Ports(),
		MasterData:   mdStore,
		Capabilities: drv.Capabilities(),
		Cleanup:      func() {},
	}
}

// NewPostgresStorageHarness provisions and wires a PostgreSQL adapter in an isolated schema.
func NewPostgresStorageHarness(t *testing.T, f UnifiedFixture) *StorageAdapterHarness {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://osm@localhost:5432/accounting_test?sslmode=disable&host=/var/run/postgresql"
	}

	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		if os.Getenv("POSTGRES_URL") != "" {
			t.Fatalf("postgres: parse configured test database URL: %v", err)
		}
		t.Skipf("postgres: connect to test database skipped: %v", err)
		return nil
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		if os.Getenv("POSTGRES_URL") != "" {
			t.Fatalf("postgres: ping configured test database: %v", err)
		}
		t.Skipf("postgres: ping test database skipped: %v", err)
		return nil
	}

	schemaName := "parity_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	schema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminPool.Close()
		t.Fatalf("postgres: create isolated schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		adminPool.Close()
		t.Fatalf("postgres: parse dsn: %v", err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"

	testPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		t.Fatalf("postgres: create isolated test pool: %v", err)
	}

	if err := postgres.EnsureSchema(ctx, testPool); err != nil {
		testPool.Close()
		adminPool.Close()
		t.Fatalf("postgres: initialize isolated schema: %v", err)
	}

	adapter := postgres.NewWithPool(testPool)

	mdStore := adapter.MasterDataStore()
	if err := ProvisionFixture(ctx, mdStore, f); err != nil {
		testPool.Close()
		adminPool.Close()
		t.Fatalf("postgres: provision fixture: %v", err)
	}

	return &StorageAdapterHarness{
		Name:         "postgres",
		Ports:        adapter.Ports(),
		MasterData:   mdStore,
		Capabilities: adapter.Capabilities(),
		Cleanup: func() {
			adapter.Close()
			testPool.Close()
			_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			adminPool.Close()
		},
	}
}

// ForEachStorageAdapter runs a test body across Memory and Postgres adapters with freshly isolated state.
func ForEachStorageAdapter(t *testing.T, scenarioName string, testFn func(t *testing.T, h *StorageAdapterHarness, f UnifiedFixture)) {
	t.Helper()

	adapters := []struct {
		name string
		ctor func(t *testing.T, f UnifiedFixture) *StorageAdapterHarness
	}{
		{name: "memory", ctor: NewMemoryStorageHarness},
		{name: "postgres", ctor: NewPostgresStorageHarness},
	}

	for _, tc := range adapters {
		t.Run(tc.name+"/"+scenarioName, func(t *testing.T) {
			start := time.Now()
			var capabilities storage.AdapterCapabilities
			defer func() {
				differences := []string(nil)
				semanticDifferences := []string{"none; storage contract assertions passed"}
				unexpected := []string(nil)
				if capabilities.AdapterName != "" {
					differences = []string{
						"durability=" + boolStr(capabilities.GuaranteesDurability),
						"foreign_keys=" + boolStr(capabilities.EnforcesForeignKeys),
						"serializable=" + boolStr(capabilities.SupportsSerializable),
						"savepoints=" + boolStr(capabilities.SupportsSavepoints),
					}
				}
				if t.Failed() {
					unexpected = []string{"contract assertions failed; see test logs"}
					semanticDifferences = []string{"contract assertions failed"}
				}
				GlobalReporter.Record(ScenarioResult{
					ScenarioName:          scenarioName,
					AdapterName:           tc.name,
					EngineVersion:         EngineVersion(),
					Passed:                !t.Failed() && !t.Skipped(),
					Skipped:               t.Skipped(),
					CapabilityDifferences: differences,
					SemanticDifferences:   semanticDifferences,
					UnexpectedDifferences: unexpected,
					ExecutionDuration:     time.Since(start),
				})
			}()

			fixture := NewUnifiedFixture()
			h := tc.ctor(t, fixture)
			if h == nil {
				return
			}
			capabilities = h.Capabilities
			defer h.Cleanup()

			testFn(t, h, fixture)
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
