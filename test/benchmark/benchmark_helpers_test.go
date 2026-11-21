package benchmark

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/postgres"
	parity "github.com/osmai4603-collab/accounting_engine/test/contract_parity"
	"github.com/shopspring/decimal"
)

// payloadHashIdempotency adapts the public port for PostgreSQL's non-empty
// payload-hash contract. The SDK currently passes an empty hash; this is a
// test-side bridge only, recorded as a P7-04 integration gap.
type payloadHashIdempotency struct {
	storage.IdempotencyStore
}

func (payloadHashIdempotency) hash(key string) string { return "p7-bench:" + key }

func (a payloadHashIdempotency) Claim(ctx context.Context, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	if record.PayloadHash == "" {
		record.PayloadHash = a.hash(record.Key)
	}
	return a.IdempotencyStore.Claim(ctx, record)
}

func (a payloadHashIdempotency) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	if payloadHash == "" {
		payloadHash = a.hash(key)
	}
	return a.IdempotencyStore.Complete(ctx, companyID, key, payloadHash, entryID, response)
}

func (a payloadHashIdempotency) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	if payloadHash == "" {
		payloadHash = a.hash(key)
	}
	return a.IdempotencyStore.Fail(ctx, companyID, key, payloadHash, status)
}

// benchHarness holds an isolated PostgreSQL schema and a public SDK engine.
// storage/memory is deliberately excluded from the certified baseline.
type benchHarness struct {
	ports   storage.Ports
	fixture parity.UnifiedFixture
	engine  sdk.Engine
	app     publicapplication.Service
	cleanup func()
}

func postgresDSN(tb testing.TB) string {
	tb.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://osm@localhost:5432/accounting_test?sslmode=disable&host=/var/run/postgresql"
	}
	return dsn
}

func newPostgresBench(tb testing.TB) *benchHarness {
	tb.Helper()
	ctx := context.Background()
	dsn := postgresDSN(tb)

	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		tb.Fatalf("benchmark: connect to PostgreSQL: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		tb.Fatalf("benchmark: ping PostgreSQL: %v", err)
	}

	schemaName := "bench_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	schema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminPool.Close()
		tb.Fatalf("benchmark: create isolated schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		adminPool.Close()
		tb.Fatalf("benchmark: parse DSN: %v", err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"

	testPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		tb.Fatalf("benchmark: create test pool: %v", err)
	}
	if err := postgres.EnsureSchema(ctx, testPool); err != nil {
		testPool.Close()
		adminPool.Close()
		tb.Fatalf("benchmark: initialize schema: %v", err)
	}

	adapter := postgres.NewWithPool(testPool)
	fixture := parity.NewUnifiedFixture()
	// Distinct presentation currency avoids the auto-provisioned policy's
	// duplicate-currency validation defect (recorded as a P7-04 integration gap).
	fixture.Company.PresentationCurrency = "EUR"
	if err := parity.ProvisionFixture(ctx, adapter.MasterDataStore(), fixture); err != nil {
		testPool.Close()
		adminPool.Close()
		tb.Fatalf("benchmark: provision fixture: %v", err)
	}

	ports := adapter.Ports()
	ports.Idempotency = payloadHashIdempotency{IdempotencyStore: ports.Idempotency}
	engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports)).Build(ctx)
	if err != nil {
		testPool.Close()
		adminPool.Close()
		tb.Fatalf("benchmark: build engine: %v", err)
	}
	app, err := sdk.NewApplicationService(ports)
	if err != nil {
		_ = engine.Close()
		testPool.Close()
		adminPool.Close()
		tb.Fatalf("benchmark: build application service: %v", err)
	}

	h := &benchHarness{
		ports:   ports,
		fixture: fixture,
		engine:  engine,
		app:     app,
		cleanup: func() {
			_ = engine.Close()
			testPool.Close()
			_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			adminPool.Close()
		},
	}
	return h
}

func (h *benchHarness) newEntry(tb testing.TB) journal.JournalEntry {
	tb.Helper()
	amount, err := types.NewMoneyFromString("10.00", "USD")
	if err != nil {
		tb.Fatalf("benchmark amount: %v", err)
	}
	zero, err := types.NewMoneyFromString("0", "USD")
	if err != nil {
		tb.Fatalf("benchmark zero: %v", err)
	}
	f := h.fixture
	date := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	return journal.JournalEntry{
		ID: types.NewJournalEntryID(), CompanyID: f.Company.ID,
		JournalID: types.JournalID(f.Journal.ID), FiscalPeriodID: f.OpenPeriod.ID,
		Status: journal.EntryDraft, Classification: journal.EntryClassRegular,
		EffectiveDate: date, PostingDate: date,
		Description: "benchmark posting", SourceModule: "p7-benchmark",
		TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
		CreatedBy: "benchmark",
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.CashAccount.ID,
				TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: amount, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.RevenueAccount.ID,
				TransactionAmount: amount, TransactionCurrency: "USD",
				ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: zero, FunctionalCredit: amount,
			},
		},
	}
}

func (h *benchHarness) postCtx() context.Context {
	return publicapplication.WithRequestMetadata(context.Background(), publicapplication.RequestMetadata{
		CorrelationID:  "p7-benchmark",
		IdempotencyKey: "bench-" + uuid.NewString(),
		SourceSystem:   "p7-benchmark",
	})
}

func (h *benchHarness) post(tb testing.TB) {
	tb.Helper()
	if _, err := h.engine.Ledger().PostEntry(h.postCtx(), h.newEntry(tb)); err != nil {
		tb.Fatalf("benchmark posting failed: %v", err)
	}
}

// postParallel is safe inside b.RunParallel worker goroutines.
func (h *benchHarness) postParallel(b *testing.B) bool {
	entry := h.newEntry(b)
	if _, err := h.engine.Ledger().PostEntry(h.postCtx(), entry); err != nil {
		b.Errorf("benchmark concurrent posting failed: %v", err)
		return false
	}
	return true
}
