package drdrill

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/postgres"
	parity "github.com/osmai4603-collab/accounting_engine/test/contract_parity"
)

type drillEnvironment struct {
	ctx          context.Context
	baseDSN      string
	sourceDSN    string
	targetDSN    string
	sourceSchema string
	targetDB     string
	adminPool    *pgxpool.Pool
	sourcePool   *pgxpool.Pool
	targetPool   *pgxpool.Pool
	backupDir    string
	backupPath   string
	backupSize   int64
	restoreStart time.Time
	restoreEnd   time.Time
	fixture      parity.UnifiedFixture
	ports        storage.Ports
	engine       sdk.Engine
	app          publicapplication.Service
}

func prepareDrillEnvironment(t *testing.T, ctx context.Context) *drillEnvironment {
	t.Helper()
	base := os.Getenv("POSTGRES_URL")
	if base == "" {
		base = "postgres://osm@localhost:5432/accounting_test?sslmode=disable&host=/var/run/postgresql"
	}
	adminConfig, err := pgxpool.ParseConfig(base)
	if err != nil {
		t.Fatalf("parse base DSN: %v", err)
	}
	adminConfig.ConnConfig.Database = "postgres"
	adminPool, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("ping admin database: %v", err)
	}

	schema := "dr_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	schemaIdent := pgx.Identifier{schema}.Sanitize()
	sourceAdmin, err := pgxpool.New(ctx, base)
	if err != nil {
		adminPool.Close()
		t.Fatalf("connect test database: %v", err)
	}
	if _, err := sourceAdmin.Exec(ctx, "CREATE SCHEMA "+schemaIdent); err != nil {
		sourceAdmin.Close()
		adminPool.Close()
		t.Fatalf("create source schema: %v", err)
	}
	sourceAdmin.Close()

	backupDir, err := os.MkdirTemp("/tmp/opencode", "p7-dr-")
	if err != nil {
		adminPool.Close()
		t.Fatalf("create backup dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(backupDir) })

	env := &drillEnvironment{
		ctx: ctx, baseDSN: base, sourceSchema: schema,
		targetDB:  "dr_restore_" + strings.ReplaceAll(uuid.NewString(), "-", "_"),
		adminPool: adminPool, backupDir: backupDir,
	}

	sourcePool := openSchemaPool(t, ctx, base, schema)
	env.sourcePool = sourcePool
	if err := postgres.EnsureSchema(ctx, sourcePool); err != nil {
		t.Fatalf("initialize source schema: %v", err)
	}
	adapter := postgres.NewWithPool(sourcePool)
	fixture := parity.NewUnifiedFixture()
	fixture.Company.PresentationCurrency = "EUR"
	if err := parity.ProvisionFixture(ctx, adapter.MasterDataStore(), fixture); err != nil {
		t.Fatalf("provision source fixture: %v", err)
	}
	env.fixture = fixture

	ports := adapter.Ports()
	ports.Idempotency = testPayloadHashIdempotency{IdempotencyStore: ports.Idempotency}
	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports)).Build(ctx)
	if err != nil {
		t.Fatalf("build source engine: %v", err)
	}
	app, err := sdk.NewApplicationService(ports)
	if err != nil {
		t.Fatalf("build source application service: %v", err)
	}
	env.ports = ports
	env.engine = engine
	env.app = app

	t.Cleanup(func() {
		_ = engine.Close()
		sourcePool.Close()
		adminCleanup, err := pgxpool.NewWithConfig(context.Background(), adminConfig)
		if err == nil {
			_, _ = adminCleanup.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{env.targetDB}.Sanitize())
			adminCleanup.Close()
		}
		dropAdmin, err := pgxpool.New(context.Background(), base)
		if err == nil {
			_, _ = dropAdmin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schemaIdent+" CASCADE")
			dropAdmin.Close()
		}
		adminPool.Close()
	})
	return env
}

func openSchemaPool(t *testing.T, ctx context.Context, baseDSN, schema string) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("parse schema DSN: %v", err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{schema}.Sanitize() + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	return pool
}

func (e *drillEnvironment) sourceDumpDSN() string {
	return appendOptions(e.baseDSN, "options", "-c search_path="+e.sourceSchema+",public")
}

func withDatabase(dsn, database string) string {
	config, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	config.Path = "/" + database
	return config.String()
}

func appendOptions(dsn, key, value string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%s%s=%s", dsn, separator, key, url.QueryEscape(value))
}

// testPayloadHashIdempotency bridges the SDK's empty payload hash to the
// PostgreSQL idempotency contract. Recorded as a P7-04 integration gap.
type testPayloadHashIdempotency struct {
	storage.IdempotencyStore
}

func (testPayloadHashIdempotency) hash(key string) string { return "p7-dr:" + key }

func (a testPayloadHashIdempotency) Claim(ctx context.Context, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	if record.PayloadHash == "" {
		record.PayloadHash = a.hash(record.Key)
	}
	return a.IdempotencyStore.Claim(ctx, record)
}

func (a testPayloadHashIdempotency) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	if payloadHash == "" {
		payloadHash = a.hash(key)
	}
	return a.IdempotencyStore.Complete(ctx, companyID, key, payloadHash, entryID, response)
}

func (a testPayloadHashIdempotency) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	if payloadHash == "" {
		payloadHash = a.hash(key)
	}
	return a.IdempotencyStore.Fail(ctx, companyID, key, payloadHash, status)
}

func backupFilePath(t *testing.T, env *drillEnvironment) string {
	t.Helper()
	return filepath.Join(env.backupDir, "p7_dr_backup.dump")
}
