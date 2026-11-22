package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

type phase8CompanyFixture struct {
	companyID  types.CompanyID
	journalID  types.JournalID
	periodID   types.FiscalPeriodID
	debitID    types.AccountID
	creditID   types.AccountID
	functional string
}

func phase8ProvisionCompany(t *testing.T, ctx context.Context, driver interface {
	CreateCompany(context.Context, masterdata.Company) error
	CreateJournal(context.Context, masterdata.Journal) error
	CreateAccount(context.Context, masterdata.Account) error
	CreateFiscalYear(context.Context, masterdata.FiscalYear) error
	CreateFiscalPeriod(context.Context, masterdata.FiscalPeriod) error
}, code string) phase8CompanyFixture {
	t.Helper()
	companyID := types.NewCompanyID()
	journalID := types.JournalID(masterdata.NewJournalID())
	yearID := types.NewFiscalYearID()
	periodID := types.NewFiscalPeriodID()
	debitID := types.NewAccountID()
	creditID := types.NewAccountID()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := driver.CreateCompany(ctx, masterdata.Company{
		ID: companyID, Code: code, LegalName: "Phase 8 " + code,
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatalf("create company %s: %v", code, err)
	}
	if err := driver.CreateJournal(ctx, masterdata.Journal{
		ID: masterdata.JournalID(journalID), CompanyID: companyID, Code: "GEN", Name: "General",
	}); err != nil {
		t.Fatalf("create journal: %v", err)
	}
	if err := driver.CreateFiscalYear(ctx, masterdata.FiscalYear{
		ID: yearID, CompanyID: companyID, Name: "FY2026",
		StartDate: start, EndDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		State: masterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatalf("create fiscal year: %v", err)
	}
	if err := driver.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: periodID, CompanyID: companyID, FiscalYearID: yearID, Number: 1,
		Name: "January", StartDate: start, EndDate: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		State: masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
	}); err != nil {
		t.Fatalf("create fiscal period: %v", err)
	}
	for _, account := range []masterdata.Account{
		{ID: debitID, CompanyID: companyID, Code: "1000", Name: "Cash",
			Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit,
			Currency: "USD", IsActive: true},
		{ID: creditID, CompanyID: companyID, Code: "4000", Name: "Revenue",
			Type: masterdata.AccountTypeIncome, NormalBalance: masterdata.NormalBalanceCredit,
			Currency: "USD", IsActive: true},
	} {
		if err := driver.CreateAccount(ctx, account); err != nil {
			t.Fatalf("create account %s: %v", account.Code, err)
		}
	}
	return phase8CompanyFixture{
		companyID: companyID, journalID: journalID, periodID: periodID,
		debitID: debitID, creditID: creditID, functional: "USD",
	}
}

func phase8PostgresCommit(t *testing.T, ctx context.Context, driver interface {
	Execute(context.Context, func(context.Context, storage.UnitOfWork) error) error
}, f phase8CompanyFixture, key string) journal.JournalEntry {
	t.Helper()
	amount, err := types.NewMoney(decimal.NewFromInt(100), f.functional)
	if err != nil {
		t.Fatalf("money: %v", err)
	}
	zero, err := types.NewMoney(decimal.Zero, f.functional)
	if err != nil {
		t.Fatalf("zero money: %v", err)
	}
	entry := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      f.companyID,
		JournalID:      f.journalID,
		FiscalPeriodID: f.periodID,
		EffectiveDate:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		PostingDate:    time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Status:         journal.EntryDraft,
		Lines: []journal.EntryLine{
			{ID: types.NewJournalEntryLineID(), AccountID: f.debitID,
				TransactionAmount: amount, TransactionCurrency: f.functional, ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: amount, FunctionalCredit: zero},
			{ID: types.NewJournalEntryLineID(), AccountID: f.creditID,
				TransactionAmount: amount, TransactionCurrency: f.functional, ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: zero, FunctionalCredit: amount},
		},
	}
	companyCtx := storage.WithCompanyContext(ctx, f.companyID)
	if err := driver.Execute(companyCtx, func(uowCtx context.Context, uow storage.UnitOfWork) error {
		_, postErr := uow.Posting().CommitPosting(uowCtx, storage.PostingCommit{
			Entry:     entry,
			Operation: storage.OperationPost,
			ActorID:   "phase8-rls-test",
			Idempotency: &storage.PostingIdempotency{
				Key:         key,
				Scope:       "company=" + f.companyID.String() + "|operation=post|key=" + key,
				PayloadHash: "hash-" + key,
			},
		})
		return postErr
	}); err != nil {
		t.Fatalf("commit posting: %v", err)
	}
	return entry
}

// TestPhase8PostgresRLSRuntimeIsolation proves that forced row-level security
// in the PostgreSQL adapter hides another company's committed rows even when
// the SQL is written directly against the isolated schema. Domain scope checks
// remain the first line of defence; RLS is defence in depth, not a substitute.
func TestPhase8PostgresRLSRuntimeIsolation(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	companyA := phase8ProvisionCompany(t, ctx, driver, "PHASE8-A")
	companyB := phase8ProvisionCompany(t, ctx, driver, "PHASE8-B")
	entryA := phase8PostgresCommit(t, ctx, driver, companyA, "rls-key-a")
	entryB := phase8PostgresCommit(t, ctx, driver, companyB, "rls-key-b")

	// Adapter scope: B cannot read A's entry identifier through its own scope.
	if _, err := driver.GetEntry(ctx, companyB.companyID, entryA.ID); err == nil {
		t.Fatal("adapter read leaked company A entry to company B scope")
	}

	// The harness user is a superuser and would bypass RLS even with FORCE
	// enabled. Query through a plain role so the runtime policy is actually
	// evaluated; domain checks in the adapter remain the primary boundary.
	role := "phase8_rls_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	roleID := pgx.Identifier{role}.Sanitize()
	if _, err := driver.Pool().Exec(ctx, "CREATE ROLE "+roleID+" NOLOGIN"); err != nil {
		t.Skipf("cannot create non-superuser role for RLS test: %v", err)
	}
	defer func() {
		cleanupCtx := context.WithoutCancel(ctx)
		if _, err := driver.Pool().Exec(cleanupCtx, "DROP OWNED BY "+roleID); err != nil {
			t.Errorf("revoke RLS test role privileges: %v", err)
		}
		if _, err := driver.Pool().Exec(cleanupCtx, "DROP ROLE IF EXISTS "+roleID); err != nil {
			t.Errorf("drop RLS test role: %v", err)
		}
	}()
	var schema string
	if err := driver.Pool().QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("current schema: %v", err)
	}
	schemaID := pgx.Identifier{schema}.Sanitize()
	if _, err := driver.Pool().Exec(ctx, "GRANT USAGE ON SCHEMA "+schemaID+" TO "+roleID); err != nil {
		t.Fatalf("grant schema usage: %v", err)
	}
	if _, err := driver.Pool().Exec(ctx, "GRANT SELECT ON ALL TABLES IN SCHEMA "+schemaID+" TO "+roleID); err != nil {
		t.Fatalf("grant table select: %v", err)
	}

	countScoped := func(viewer, target types.CompanyID) int {
		t.Helper()
		tx, err := driver.Pool().Begin(ctx)
		if err != nil {
			t.Fatalf("begin raw transaction: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+roleID); err != nil {
			t.Fatalf("set test role: %v", err)
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_company_id', $1, true)", uuid.UUID(viewer).String()); err != nil {
			t.Fatalf("set company context: %v", err)
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM account_moves WHERE company_id=$1`, uuid.UUID(target)).Scan(&count); err != nil {
			t.Fatalf("count account_moves: %v", err)
		}
		return count
	}

	if got := countScoped(companyA.companyID, companyA.companyID); got != 1 {
		t.Fatalf("company A sees %d own rows, want 1", got)
	}
	if got := countScoped(companyA.companyID, companyB.companyID); got != 0 {
		t.Fatalf("company A context sees %d company B rows, want 0", got)
	}
	if got := countScoped(companyB.companyID, companyB.companyID); got != 1 {
		t.Fatalf("company B sees %d own rows, want 1", got)
	}
	if got := countScoped(companyB.companyID, companyA.companyID); got != 0 {
		t.Fatalf("company B context sees %d company A rows, want 0", got)
	}

	// Both entries exist physically; RLS, not absence, produced the zero counts.
	var physical int
	if err := driver.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM account_moves WHERE id IN ($1, $2)`,
		uuid.UUID(entryA.ID), uuid.UUID(entryB.ID)).Scan(&physical); err != nil {
		t.Fatalf("count physical rows: %v", err)
	}
	if physical != 2 {
		t.Fatalf("physical entry rows = %d, want 2", physical)
	}
}
