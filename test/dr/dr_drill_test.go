package drdrill

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/sdk"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/storage/postgres"
	parity "github.com/osmai4603-collab/accounting_engine/test/contract_parity"
	"github.com/shopspring/decimal"
)

// drillActorID is stable across the source posting and its post-restore replay
// so the replay is recognised as the same semantic request, not a payload
// conflict under a different principal.
var drillActorID = uuid.NewString()

func drillEntry(f parity.UnifiedFixture, id types.JournalEntryID) journal.JournalEntry {
	amount, _ := types.NewMoneyFromString("25.00", "USD")
	zero, _ := types.NewMoneyFromString("0", "USD")
	date := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	return journal.JournalEntry{
		ID: id, CompanyID: f.Company.ID, JournalID: types.JournalID(f.Journal.ID),
		FiscalPeriodID: f.OpenPeriod.ID, Status: journal.EntryDraft,
		Classification: journal.EntryClassRegular, EffectiveDate: date, PostingDate: date,
		Description: "DR drill posting", SourceModule: "p7-dr", CreatedBy: "dr-drill",
		TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
		Lines: []journal.EntryLine{
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.CashAccount.ID,
				TransactionAmount: amount, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: amount, FunctionalCredit: zero,
			},
			{
				ID: types.NewJournalEntryLineID(), AccountID: f.RevenueAccount.ID,
				TransactionAmount: amount, TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				FunctionalDebit: zero, FunctionalCredit: amount,
			},
		},
	}
}

func postDrillEntry(t *testing.T, engine sdk.Engine, f parity.UnifiedFixture, id types.JournalEntryID, key string) journal.JournalEntry {
	t.Helper()
	// PostgreSQL stores actor_id as uuid but hashes the original actor string.
	// Use a UUID actor so the stored v1 audit row can be hash-verified after
	// restore; non-UUID actors are an adapter limitation recorded as a gap.
	ctx := publicapplication.WithPrincipal(context.Background(), publicapplication.Principal{
		ActorID:    drillActorID,
		CompanyIDs: []types.CompanyID{f.Company.ID},
	})
	ctx = publicapplication.WithRequestMetadata(ctx, publicapplication.RequestMetadata{
		CorrelationID: "p7-dr", IdempotencyKey: key, SourceSystem: "p7-dr-drill",
	})
	posted, err := engine.Ledger().PostEntry(ctx, drillEntry(f, id))
	if err != nil {
		t.Fatalf("post drill entry: %v", err)
	}
	return posted
}

func TestDRDrill_FullRecovery(t *testing.T) {
	ctx := context.Background()
	env := prepareDrillEnvironment(t, ctx)

	// Source writes: three ordinary postings plus one entry whose idempotency
	// key is replayed after the restore.
	replayID := types.NewJournalEntryID()
	const replayKey = "p7-dr-replay-key"
	postDrillEntry(t, env.engine, env.fixture, types.NewJournalEntryID(), "p7-dr-source-1")
	postDrillEntry(t, env.engine, env.fixture, types.NewJournalEntryID(), "p7-dr-source-2")
	postDrillEntry(t, env.engine, env.fixture, replayID, replayKey)

	// Anchor the v2 audit chain before pg_dump. The checkpoint lives in its own
	// table and is restored with the database, so the post-restore verifier has
	// a real trusted tail to compare instead of silently skipping the branch.
	sourceDriver := postgres.NewDriverWithPool(env.sourcePool)
	lastAudit, err := sourceDriver.GetLastAuditLogForCompany(ctx, env.fixture.Company.ID)
	if err != nil || lastAudit == nil {
		t.Fatalf("read last source audit log: %v", err)
	}
	if err := sourceDriver.SetExternalAuditCheckpoint(ctx, env.fixture.Company.ID, kernalgovernance.AuditCheckpoint{
		CompanyID: env.fixture.Company.ID, SequenceNumber: lastAudit.SequenceNumber,
		Version: 1, LogHash: lastAudit.CurrentLogHash,
	}); err != nil {
		t.Fatalf("set source audit checkpoint: %v", err)
	}

	sourceSnapshot := captureSnapshot(t, ctx, env.sourcePool, env.fixture.Company.ID)
	t.Logf("source snapshot: %+v", sourceSnapshot)

	var backupDuration time.Duration
	var backupSize int64
	var targetDSN string

	t.Run("01_backup_pg_dump", func(t *testing.T) {
		env.backupPath = backupFilePath(t, env)
		start := time.Now()
		cmd := exec.CommandContext(ctx, "pg_dump", "--format=custom",
			"--schema="+env.sourceSchema, "--file="+env.backupPath,
			"--dbname="+env.baseDSN)
		output, err := cmd.CombinedOutput()
		backupDuration = time.Since(start)
		if err != nil {
			t.Fatalf("pg_dump failed: %v\n%s", err, output)
		}
		info, err := os.Stat(env.backupPath)
		if err != nil {
			t.Fatalf("stat backup: %v", err)
		}
		if info.Size() == 0 {
			t.Fatal("pg_dump produced an empty backup")
		}
		backupSize = info.Size()
		t.Logf("pg_dump: %d bytes in %s", backupSize, backupDuration.Round(time.Millisecond))
	})

	t.Run("02_restore_pg_restore", func(t *testing.T) {
		adminPool, err := pgxpool.New(ctx, withDatabase(env.baseDSN, "postgres"))
		if err != nil {
			t.Fatalf("connect target admin: %v", err)
		}
		defer adminPool.Close()
		if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{env.targetDB}.Sanitize()); err != nil {
			t.Fatalf("create target database: %v", err)
		}
		// The source schema uses a gist exclusion constraint on uuid columns;
		// install its prerequisite in the pristine target before restoring.
		bootstrap, err := pgxpool.New(ctx, withDatabase(env.baseDSN, env.targetDB))
		if err != nil {
			t.Fatalf("connect pristine target: %v", err)
		}
		if _, err := bootstrap.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS btree_gist"); err != nil {
			bootstrap.Close()
			t.Fatalf("install btree_gist on target: %v", err)
		}
		bootstrap.Close()
		targetDSN = withDatabase(env.baseDSN, env.targetDB)
		env.targetDSN = targetDSN
		env.restoreStart = time.Now()
		cmd := exec.CommandContext(ctx, "pg_restore", "--exit-on-error",
			"--dbname="+targetDSN, env.backupPath)
		output, err := cmd.CombinedOutput()
		env.restoreEnd = time.Now()
		if err != nil {
			t.Fatalf("pg_restore failed: %v\n%s", err, output)
		}
		if len(output) > 0 {
			t.Logf("pg_restore output: %s", output)
		}
		env.targetPool = openSchemaPool(t, ctx, targetDSN, env.sourceSchema)
	})

	if env.targetPool == nil {
		t.Fatal("restore phase did not complete")
	}
	defer env.targetPool.Close()

	t.Run("03_schema_health", func(t *testing.T) {
		if err := postgres.EnsureSchema(ctx, env.targetPool); err != nil {
			t.Fatalf("restored schema is not healthy: %v", err)
		}
		targetSnapshot := captureSnapshot(t, ctx, env.targetPool, env.fixture.Company.ID)
		if targetSnapshot != sourceSnapshot {
			t.Fatalf("restored snapshot = %+v, want %+v", targetSnapshot, sourceSnapshot)
		}
		t.Logf("restored snapshot matches source: %+v", targetSnapshot)
	})

	t.Run("04_double_entry_balance", func(t *testing.T) {
		rows, err := env.targetPool.Query(ctx, `SELECT move_id, SUM(debit)-SUM(credit) AS delta
			FROM account_move_lines WHERE company_id=$1 GROUP BY move_id`, env.fixture.Company.ID)
		if err != nil {
			t.Fatalf("query balance: %v", err)
		}
		defer rows.Close()
		checked := 0
		for rows.Next() {
			var moveID string
			var delta float64
			if err := rows.Scan(&moveID, &delta); err != nil {
				t.Fatalf("scan balance: %v", err)
			}
			if abs(delta) > 0.000001 {
				t.Fatalf("unbalanced move %s: delta=%f", moveID, delta)
			}
			checked++
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate balance: %v", err)
		}
		if checked == 0 {
			t.Fatal("no moves found in restored database")
		}
		t.Logf("verified %d balanced moves", checked)
	})

	t.Run("05_audit_chain", func(t *testing.T) {
		verifyPostgresAuditChain(t, ctx, env.targetPool, env.fixture.Company.ID, true)
	})

	t.Run("06_idempotent_replay", func(t *testing.T) {
		adapter := postgres.NewWithPool(env.targetPool)
		ports := adapter.Ports()
		ports.Idempotency = testPayloadHashIdempotency{IdempotencyStore: ports.Idempotency}
		engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports)).Build(ctx)
		if err != nil {
			t.Fatalf("build restored engine: %v", err)
		}
		defer engine.Close()
		app, err := sdk.NewApplicationService(ports)
		if err != nil {
			t.Fatalf("build restored application service: %v", err)
		}

		principal := publicapplication.Principal{
			ActorID:    drillActorID,
			CompanyIDs: []types.CompanyID{env.fixture.Company.ID},
		}
		replayCtx := publicapplication.WithPrincipal(ctx, principal)
		replayCtx = publicapplication.WithRequestMetadata(replayCtx, publicapplication.RequestMetadata{
			CorrelationID:  "p7-dr-replay",
			IdempotencyKey: replayKey,
			SourceSystem:   "p7-dr-drill",
		})

		before := captureSnapshot(t, ctx, env.targetPool, env.fixture.Company.ID)
		replay, err := app.Post(replayCtx, publicapplication.PostEntryCommand{
			Context: publicapplication.Context{
				Principal:      principal,
				CorrelationID:  "p7-dr-replay",
				IdempotencyKey: replayKey,
				SourceSystem:   "p7-dr-drill",
			},
			CompanyID: env.fixture.Company.ID,
			EntryID:   replayID,
		})
		if err != nil {
			t.Fatalf("idempotent replay after restore: %v", err)
		}
		if !replay.IdempotentReplay {
			t.Fatal("idempotent replay was not recognised")
		}
		if replay.Entry.ID != replayID {
			t.Fatalf("replay entry ID = %s, want %s", replay.Entry.ID, replayID)
		}
		after := captureSnapshot(t, ctx, env.targetPool, env.fixture.Company.ID)
		if before != after {
			t.Fatalf("idempotency replay changed state: before=%+v after=%+v", before, after)
		}
		t.Log("idempotency replay produced no duplicate posting")

		t.Run("resume_posting", func(t *testing.T) {
			postDrillEntry(t, engine, env.fixture, types.NewJournalEntryID(), "p7-dr-after-restore")
			resumed := captureSnapshot(t, ctx, env.targetPool, env.fixture.Company.ID)
			if resumed.PostedMoves != after.PostedMoves+1 {
				t.Fatalf("resumed posting count = %d, want %d", resumed.PostedMoves, after.PostedMoves+1)
			}
			t.Log("engine resumed posting successfully on restored database")
		})
	})

	t.Run("07_sequence_continuity", func(t *testing.T) {
		rows, err := env.targetPool.Query(ctx, `SELECT entry_number FROM account_moves
			WHERE company_id=$1 AND status='posted' AND entry_number > 0 ORDER BY entry_number`, env.fixture.Company.ID)
		if err != nil {
			t.Fatalf("query sequences: %v", err)
		}
		defer rows.Close()
		var previous int64
		count := 0
		for rows.Next() {
			var number int64
			if err := rows.Scan(&number); err != nil {
				t.Fatalf("scan sequence: %v", err)
			}
			if previous > 0 && number != previous+1 {
				t.Fatalf("sequence gap: %d followed %d", number, previous)
			}
			previous = number
			count++
		}
		if count == 0 {
			t.Fatal("no posted sequences found")
		}
		t.Logf("verified %d contiguous sequences", count)
	})

	rto := time.Since(env.restoreStart)
	t.Logf("DR metrics: backup_duration=%s backup_bytes=%d restore_duration=%s rto_restore_to_verified=%s rpo=0",
		backupDuration.Round(time.Millisecond), backupSize,
		env.restoreEnd.Sub(env.restoreStart).Round(time.Millisecond), rto.Round(time.Millisecond))
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

var _ = fmt.Sprintf
var _ = pgx.ErrNoRows
var _ = uuid.NewString
