package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/osmai4603-collab/accounting_engine/storage/postgres"
	"github.com/shopspring/decimal"
)

func TestPostgresAuditPhase10AtomicSnapshotAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	company := types.NewCompanyID()
	if err := driver.CreateCompany(ctx, masterdata.Company{
		ID: company, Code: "P10-" + company.String()[:8], LegalName: "P10 Audit",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	chain := publicgovernance.NewAuditChain(driver)

	first, err := chain.AppendAt(ctx, publicgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1",
		Action: "POST", ActorID: "actor-1", Reason: "approved",
		CorrelationID: "corr-1", Payload: []byte(`{"amount":"100"}`),
	}, time.Date(2026, 11, 22, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	second, err := chain.AppendAt(ctx, publicgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-2",
		Action: "POST", ActorID: "actor-1", Reason: "approved",
		CorrelationID: "corr-2", Payload: []byte(`{"amount":"200"}`),
	}, time.Date(2026, 11, 22, 1, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if second.SequenceNumber != 2 || second.PreviousLogHash != first.CurrentLogHash {
		t.Fatalf("second audit log = %+v", second)
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: second.SequenceNumber,
		Version: 1, LogHash: second.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}
	valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company)
	if !valid || err != nil {
		t.Fatalf("PostgreSQL checkpoint snapshot = (%v, %v)", valid, err)
	}
}

func TestPostgresLegacyAppendUsesKernelV2Hash(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	company := types.NewCompanyID()
	if err := driver.CreateCompany(ctx, masterdata.Company{
		ID: company, Code: "P10-LEGACY", LegalName: "P10 Legacy",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	timestamp := time.Unix(0, 1795309018470346000).UTC()
	if err := driver.AppendAudit(ctx, storage.AuditRecord{
		CompanyID: company, EntityType: "journal_entry", EntityID: types.NewJournalEntryID().String(),
		Action: "post", ActorID: types.NewAccountID().String(), Reason: "approved",
		CorrelationID: "corr-1", Payload: []byte(`{"entry_id":"x","sequence":1}`),
		OccurredAt: timestamp,
	}); err != nil {
		t.Fatal(err)
	}
	chain := publicgovernance.NewAuditChain(driver)
	valid, err := chain.VerifyAuditIntegrity(ctx, company)
	if !valid || err != nil {
		t.Fatalf("legacy append kernel verification = (%v, %v)", valid, err)
	}
}

func newPhase10Company(t *testing.T, ctx context.Context, driver *postgres.Driver) types.CompanyID {
	t.Helper()
	company := types.NewCompanyID()
	if err := driver.CreateCompany(ctx, masterdata.Company{
		ID: company, Code: "P10-" + company.String()[:8], LegalName: "P10 Audit",
		FunctionalCurrency: "USD", PresentationCurrency: "USD", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	return company
}

func assertPhase10SnapshotChain(t *testing.T, snapshot publicgovernance.AuditVerificationSnapshot, company types.CompanyID) {
	t.Helper()
	if uint64(len(snapshot.Logs)) != snapshot.SnapshotSequence {
		t.Fatalf("snapshot boundary = %d logs, want %d", len(snapshot.Logs), snapshot.SnapshotSequence)
	}
	previous := publicgovernance.GenesisHash
	for index, log := range snapshot.Logs {
		wantSequence := uint64(index + 1)
		if log.SequenceNumber != wantSequence {
			t.Fatalf("snapshot log %d sequence = %d, want %d", index, log.SequenceNumber, wantSequence)
		}
		if log.CompanyID != company {
			t.Fatalf("snapshot log %d company = %s, want %s", index, log.CompanyID, company)
		}
		if log.PreviousLogHash != previous {
			t.Fatalf("snapshot log %d previous hash = %s, want %s", index, log.PreviousLogHash, previous)
		}
		if computed := publicgovernance.CalculateHash(log); computed != log.CurrentLogHash {
			t.Fatalf("snapshot log %d stored hash = %s, computed %s", index, log.CurrentLogHash, computed)
		}
		previous = log.CurrentLogHash
	}
}

func TestPostgresAuditPhase10ConcurrentAppendSequenceAndChain(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	company := newPhase10Company(t, ctx, driver)

	const appenders = 12
	sequenceCh := make(chan uint64, appenders)
	errCh := make(chan error, appenders)
	var wg sync.WaitGroup
	for i := range appenders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			log, err := driver.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{
				CompanyID: company, EntityType: "journal_entry",
				EntityID: fmt.Sprintf("JE-%02d", i), Action: "POST",
				ActorID: "actor-1", Reason: "concurrent", CorrelationID: fmt.Sprintf("corr-%02d", i),
				Payload: []byte(fmt.Sprintf(`{"n":%d}`, i)),
			})
			if err != nil {
				errCh <- err
				return
			}
			sequenceCh <- log.SequenceNumber
		}(i)
	}
	wg.Wait()
	close(errCh)
	close(sequenceCh)
	for err := range errCh {
		t.Fatalf("concurrent append: %v", err)
	}
	seen := make(map[uint64]bool, appenders)
	for sequence := range sequenceCh {
		if seen[sequence] {
			t.Fatalf("duplicate sequence %d under concurrent append", sequence)
		}
		seen[sequence] = true
	}
	for i := 1; i <= appenders; i++ {
		if !seen[uint64(i)] {
			t.Fatalf("missing sequence %d under concurrent append", i)
		}
	}

	chain := publicgovernance.NewAuditChain(driver)
	if valid, err := chain.VerifyAuditIntegrity(ctx, company); !valid || err != nil {
		t.Fatalf("chain after concurrent appends = (%v, %v)", valid, err)
	}
	snapshot, err := driver.GetAuditVerificationSnapshot(ctx, company)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Logs) != appenders {
		t.Fatalf("snapshot contains %d logs, want %d", len(snapshot.Logs), appenders)
	}
	first := snapshot.Logs[0]
	if first.SequenceNumber != 1 || first.PreviousLogHash != publicgovernance.GenesisHash {
		t.Fatalf("first log = %+v, want sequence 1 and genesis previous hash", first)
	}
	assertPhase10SnapshotChain(t, snapshot, company)
}

func TestPostgresAuditPhase10SnapshotIsStableDuringConcurrentAppends(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	company := newPhase10Company(t, ctx, driver)
	chain := publicgovernance.NewAuditChain(driver)

	seed, err := chain.Append(ctx, publicgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-seed", Action: "POST",
	})
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	readerErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readerErr <- nil
				return
			default:
			}
			snapshot, err := driver.GetAuditVerificationSnapshot(ctx, company)
			if err != nil {
				readerErr <- err
				return
			}
			if snapshot.CheckpointPresent {
				readerErr <- errors.New("unexpected checkpoint before one was saved")
				return
			}
			if uint64(len(snapshot.Logs)) == 0 {
				readerErr <- errors.New("snapshot lost the seeded log during appends")
				return
			}
			// Check the snapshot's own boundary and links while writes continue.
			previous := publicgovernance.GenesisHash
			for index, log := range snapshot.Logs {
				if log.SequenceNumber != uint64(index+1) || log.PreviousLogHash != previous ||
					publicgovernance.CalculateHash(log) != log.CurrentLogHash {
					readerErr <- fmt.Errorf("inconsistent snapshot during append at log %d", index)
					return
				}
				previous = log.CurrentLogHash
			}
			if uint64(len(snapshot.Logs)) != snapshot.SnapshotSequence {
				readerErr <- fmt.Errorf("snapshot boundary %d != %d", len(snapshot.Logs), snapshot.SnapshotSequence)
				return
			}
		}
	}()

	const appenders = 10
	appendErr := make(chan error, appenders)
	var wg sync.WaitGroup
	for i := range appenders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := driver.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{
				CompanyID: company, EntityType: "journal_entry",
				EntityID: fmt.Sprintf("JE-%02d", i), Action: "POST",
			})
			appendErr <- err
		}(i)
	}
	wg.Wait()
	close(stop)
	if err := <-readerErr; err != nil {
		t.Fatalf("snapshot reader: %v", err)
	}
	close(appendErr)
	for err := range appendErr {
		if err != nil {
			t.Fatalf("append during snapshot reads: %v", err)
		}
	}

	last, err := driver.GetLastAuditLogForCompany(ctx, company)
	if err != nil || last == nil {
		t.Fatalf("last audit log = %v, %v", last, err)
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: last.SequenceNumber, Version: 1, LogHash: last.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); !valid || err != nil {
		t.Fatalf("final checkpoint verification = (%v, %v), want valid", valid, err)
	}
	_ = seed
}

// bypassAuditWorm disables WORM triggers for the rest of the transaction so a
// test can simulate an attacker with database superuser privileges. RLS is kept
// in force explicitly by setting the tenant context.
func bypassAuditWorm(t *testing.T, ctx context.Context, tx pgx.Tx, company types.CompanyID) {
	t.Helper()
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
		t.Fatalf("disable WORM triggers: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_company_id',$1,true)", company.String()); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}
}

func TestPostgresAuditPhase10TamperingIsDetected(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	company := newPhase10Company(t, ctx, driver)
	chain := publicgovernance.NewAuditChain(driver)

	var last *publicgovernance.AuditLog
	for i := range 3 {
		log, err := chain.Append(ctx, publicgovernance.AuditLog{
			CompanyID: company, EntityType: "journal_entry",
			EntityID: fmt.Sprintf("JE-%d", i), Action: "POST", Payload: []byte(`{"amount":"10"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		last = &log
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: last.SequenceNumber, Version: 1, LogHash: last.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}

	pool := driver.Pool()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	bypassAuditWorm(t, ctx, tx, company)
	if _, err := tx.Exec(ctx, `UPDATE financial_audit_logs SET payload_after=$2::bytea
		WHERE company_id=$1 AND sequence_number=2`,
		uuid.UUID(company), []byte(`{"amount":"999999"}`)); err != nil {
		t.Fatalf("tamper with audit payload: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tampering: %v", err)
	}

	if valid, err := chain.VerifyAuditIntegrity(ctx, company); valid || !errors.Is(err, publicgovernance.ErrAuditChainBroken) {
		t.Fatalf("verification after middle-record tampering = (%v, %v), want chain broken", valid, err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); valid || err == nil {
		t.Fatalf("checkpoint verification after tampering = (%v, %v), want failure", valid, err)
	}
}

func TestPostgresAuditPhase10TailDeletionFailsClosedWithCheckpoint(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	company := newPhase10Company(t, ctx, driver)
	chain := publicgovernance.NewAuditChain(driver)

	var last *publicgovernance.AuditLog
	for i := range 3 {
		log, err := chain.Append(ctx, publicgovernance.AuditLog{
			CompanyID: company, EntityType: "journal_entry", EntityID: fmt.Sprintf("JE-%d", i), Action: "POST",
		})
		if err != nil {
			t.Fatal(err)
		}
		last = &log
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: last.SequenceNumber, Version: 1, LogHash: last.CurrentLogHash,
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := driver.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	bypassAuditWorm(t, ctx, tx, company)
	if _, err := tx.Exec(ctx, `DELETE FROM financial_audit_logs
		WHERE company_id=$1 AND sequence_number=3`, uuid.UUID(company)); err != nil {
		t.Fatalf("delete audit tail: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tail deletion: %v", err)
	}

	// Hash-chain verification alone cannot prove that a trailing record existed.
	if valid, err := chain.VerifyAuditIntegrity(ctx, company); !valid || err != nil {
		t.Fatalf("remaining prefix verification = (%v, %v), want valid", valid, err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); valid ||
		!errors.Is(err, publicgovernance.ErrAuditCheckpointMismatch) {
		t.Fatalf("tail deletion = (%v, %v), want checkpoint mismatch", valid, err)
	}
}

func TestPostgresAuditPhase10MissingCheckpointFailsClosed(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	company := newPhase10Company(t, ctx, driver)
	chain := publicgovernance.NewAuditChain(driver)

	check := func(phase string) {
		t.Helper()
		if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); valid ||
			!errors.Is(err, publicgovernance.ErrAuditCheckpointMismatch) {
			t.Fatalf("%s without checkpoint = (%v, %v), want checkpoint mismatch", phase, valid, err)
		}
	}
	check("empty chain")
	if _, err := chain.Append(ctx, publicgovernance.AuditLog{
		CompanyID: company, EntityType: "journal_entry", EntityID: "JE-1", Action: "POST",
	}); err != nil {
		t.Fatal(err)
	}
	check("non-empty chain")
	if valid, err := chain.VerifyAuditIntegrity(ctx, company); !valid || err != nil {
		t.Fatalf("chain without checkpoint = (%v, %v), want valid", valid, err)
	}
}

func TestPostgresAuditPhase10CheckpointCannotRegress(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	company := newPhase10Company(t, ctx, driver)
	chain := publicgovernance.NewAuditChain(driver)

	var logs []publicgovernance.AuditLog
	for i := range 2 {
		log, err := chain.Append(ctx, publicgovernance.AuditLog{
			CompanyID: company, EntityType: "journal_entry", EntityID: fmt.Sprintf("JE-%d", i), Action: "POST",
		})
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
	}
	anchor := publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: 1, Version: 1, LogHash: logs[0].CurrentLogHash,
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, anchor); err != nil {
		t.Fatal(err)
	}

	// The checkpoint hash is trusted external custody: the adapter checks its
	// shape and monotonic position, not whether the hash matches internal logs.
	rejected := []publicgovernance.AuditCheckpoint{
		{CompanyID: company, SequenceNumber: 2, Version: 1, LogHash: logs[1].CurrentLogHash},
		{CompanyID: company, SequenceNumber: 0, Version: 3, LogHash: publicgovernance.GenesisHash},
		{CompanyID: company, SequenceNumber: 2, Version: 0, LogHash: logs[1].CurrentLogHash},
	}
	for _, checkpoint := range rejected {
		if err := driver.SetExternalAuditCheckpoint(ctx, company, checkpoint); !errors.Is(err, publicgovernance.ErrInvalidAuditCheckpoint) {
			t.Fatalf("regressive checkpoint %+v = %v, want ErrInvalidAuditCheckpoint", checkpoint, err)
		}
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, company, publicgovernance.AuditCheckpoint{
		CompanyID: company, SequenceNumber: 2, Version: 2, LogHash: logs[1].CurrentLogHash,
	}); err != nil {
		t.Fatalf("valid advancing checkpoint: %v", err)
	}
	if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); !valid || err != nil {
		t.Fatalf("verification after checkpoint advance = (%v, %v)", valid, err)
	}

	// A database-side regression must be rejected by the migration trigger too.
	tx, err := driver.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_company_id',$1,true)", company.String()); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE external_audit_checkpoints
		SET sequence_number=1, version=1, log_hash=$2 WHERE company_id=$1`,
		uuid.UUID(company), logs[0].CurrentLogHash)
	if err == nil || !strings.Contains(err.Error(), "cannot move backwards") {
		t.Fatalf("database checkpoint regression = %v, want monotonic trigger error", err)
	}
}

func TestPostgresPhase10BackfillRechainsLegacyRows(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()
	pool := driver.Pool()
	company := newPhase10Company(t, ctx, driver)

	// Other phases may have added migrations after the Phase 10 audit evidence
	// migration. Roll back every migration from that version onward so the
	// legacy row is inserted against the pre-Phase-10 schema, then re-apply all
	// of them through MigrateUp below.
	const phase10AuditMigrationVersion = 202611220002
	var rollbackSteps int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM schema_migrations WHERE version >= $1`,
		phase10AuditMigrationVersion).Scan(&rollbackSteps); err != nil {
		t.Fatal(err)
	}
	if rollbackSteps < 1 {
		t.Fatal("Phase 10 audit evidence migration is not applied")
	}
	if err := postgres.MigrateDown(ctx, pool, rollbackSteps); err != nil {
		t.Fatalf("MigrateDown(%d) before inserting legacy audit rows: %v", rollbackSteps, err)
	}

	legacyHashes := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	for i, hash := range legacyHashes {
		timestamp := time.Date(2026, 11, 22, 2, 0, i, 0, time.UTC)
		_, err := pool.Exec(ctx, `INSERT INTO financial_audit_logs(
			company_id, entity_type, entity_id, action, actor_id,
			payload_after, timestamp, previous_log_hash, current_log_hash
		) VALUES($1,'journal_entry',$2,'POST',$3,$4,$5,$6,$7)`,
			uuid.UUID(company), uuid.NewString(), uuid.NewString(),
			[]byte(fmt.Sprintf(`{"legacy":%d}`, i)), timestamp, legacyHashes[i], hash)
		if err != nil {
			t.Fatalf("insert legacy audit row %d: %v", i, err)
		}
	}

	if err := postgres.MigrateUp(ctx, pool); err != nil {
		t.Fatalf("MigrateUp after inserting legacy rows: %v", err)
	}

	var firstSequence int64
	var firstPrevious, firstCurrent string
	if err := pool.QueryRow(ctx, `SELECT sequence_number, previous_log_hash, current_log_hash
		FROM financial_audit_logs WHERE company_id=$1 ORDER BY id LIMIT 1`, uuid.UUID(company)).
		Scan(&firstSequence, &firstPrevious, &firstCurrent); err != nil {
		t.Fatalf("read backfilled first row: %v", err)
	}
	if firstSequence != 1 || firstPrevious != publicgovernance.GenesisHash || firstCurrent == legacyHashes[0] {
		t.Fatalf("backfilled first row = (%d, %s, %s)", firstSequence, firstPrevious, firstCurrent)
	}
	chain := publicgovernance.NewAuditChain(driver)
	if valid, err := chain.VerifyAuditIntegrity(ctx, company); !valid || err != nil {
		t.Fatalf("backfilled v2 chain verification = (%v, %v)", valid, err)
	}
	snapshot, err := driver.GetAuditVerificationSnapshot(ctx, company)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CheckpointPresent {
		t.Fatal("backfill must not invent a trusted external checkpoint")
	}
	assertPhase10SnapshotChain(t, snapshot, company)
}

// TestPostgresPhase10PostingAuditFailureRollsBack proves on PostgreSQL that a
// posting cannot leave a committed financial effect without verifiable audit
// evidence when audit persistence fails inside the posting transaction.
func TestPostgresPhase10PostingAuditFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	driver, err := newIsolatedPostgresDriver(t, ctx)
	if err != nil {
		t.Skipf("isolated PostgreSQL unavailable: %v", err)
	}
	defer driver.Close()

	company := newPhase10Company(t, ctx, driver)

	// Provision the same master data the immutability drill uses so the posting
	// kernel sees a real journal, open period and balanced account pair.
	fyID := types.NewFiscalYearID()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := driver.CreateFiscalYear(ctx, masterdata.FiscalYear{
		ID: fyID, CompanyID: company, Name: "FY2026",
		StartDate: start, EndDate: time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
		State: masterdata.FiscalYearOpen,
	}); err != nil {
		t.Fatal(err)
	}
	periodID := types.NewFiscalPeriodID()
	if err := driver.CreateFiscalPeriod(ctx, masterdata.FiscalPeriod{
		ID: periodID, CompanyID: company, FiscalYearID: fyID,
		Number: 1, Name: "P1-2026", StartDate: start,
		EndDate: time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
		State:   masterdata.PeriodOpen, Type: masterdata.FiscalPeriodTypeNormal,
	}); err != nil {
		t.Fatal(err)
	}
	journalID := masterdata.NewJournalID()
	if err := driver.CreateJournal(ctx, masterdata.Journal{
		ID: journalID, CompanyID: company, Code: "GEN", Name: "General",
	}); err != nil {
		t.Fatal(err)
	}
	debitID, creditID := types.NewAccountID(), types.NewAccountID()
	for _, account := range []masterdata.Account{
		{ID: debitID, CompanyID: company, Code: "1000", Name: "Cash",
			Type: masterdata.AccountTypeAsset, NormalBalance: masterdata.NormalBalanceDebit,
			Currency: "USD", IsActive: true},
		{ID: creditID, CompanyID: company, Code: "2000", Name: "Payable",
			Type: masterdata.AccountTypeLiability, NormalBalance: masterdata.NormalBalanceCredit,
			Currency: "USD", IsActive: true},
	} {
		if err := driver.CreateAccount(ctx, account); err != nil {
			t.Fatal(err)
		}
	}
	money, err := types.NewMoney(decimal.NewFromInt(100), "USD")
	if err != nil {
		t.Fatal(err)
	}

	for _, point := range []storage.FaultPoint{storage.FaultBeforeAudit, storage.FaultBeforeCommit} {
		t.Run(string(point), func(t *testing.T) {
			entryID := types.NewJournalEntryID()
			key := "p10-fault-" + string(point)
			entry := journal.JournalEntry{
				ID: entryID, CompanyID: company,
				JournalID: types.JournalID(journalID), FiscalPeriodID: periodID,
				EntryNumber: key, SequenceNumber: 1,
				EffectiveDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				Status:        journal.EntryDraft, Classification: journal.EntryClassRegular,
				TransactionCurrency: "USD", ExchangeRate: decimal.NewFromInt(1),
				Lines: []journal.EntryLine{
					{ID: types.NewJournalEntryLineID(), AccountID: debitID,
						TransactionCurrency: "USD", TransactionAmount: money,
						ExchangeRate: decimal.NewFromInt(1), FunctionalDebit: money},
					{ID: types.NewJournalEntryLineID(), AccountID: creditID,
						TransactionCurrency: "USD", TransactionAmount: money,
						ExchangeRate: decimal.NewFromInt(1), FunctionalCredit: money},
				},
			}
			commit := storage.PostingCommit{
				Entry: entry, Operation: storage.OperationPost, ActorID: "auditor-1",
				Idempotency: &storage.PostingIdempotency{
					Key:         key,
					Scope:       fmt.Sprintf("company=%s|operation=post|key=%s", company.String(), key),
					PayloadHash: "hash-" + key,
				},
			}
			faultCtx := storage.WithFaultInjector(ctx, func(got storage.FaultPoint) error {
				if got == point {
					return errors.New("injected " + string(point))
				}
				return nil
			})
			postCtx := storage.WithCompanyContext(faultCtx, company)
			if err := driver.Execute(postCtx, func(uowCtx context.Context, uow storage.UnitOfWork) error {
				_, postErr := uow.Posting().CommitPosting(uowCtx, commit)
				return postErr
			}); err == nil {
				t.Fatalf("%s: expected injected failure", point)
			}

			// No account_move may survive the rollback.
			var moveCount int
			if err := driver.Pool().QueryRow(ctx,
				`SELECT count(*) FROM account_moves WHERE company_id=$1 AND id=$2`,
				uuid.UUID(company), uuid.UUID(entryID)).Scan(&moveCount); err != nil {
				t.Fatal(err)
			}
			if moveCount != 0 {
				t.Fatalf("%s: %d account moves survived rollback", point, moveCount)
			}

			// No audit evidence may be stranded without its posting.
			var auditCount int
			if err := driver.Pool().QueryRow(ctx,
				`SELECT count(*) FROM financial_audit_logs WHERE company_id=$1 AND entity_id=$2`,
				uuid.UUID(company), entryID.String()).Scan(&auditCount); err != nil {
				t.Fatal(err)
			}
			if auditCount != 0 {
				t.Fatalf("%s: %d audit logs survived rollback", point, auditCount)
			}

			// No trusted checkpoint was set, so checkpoint verification fails
			// closed rather than silently accepting the empty chain.
			chain := publicgovernance.NewAuditChain(driver)
			if valid, err := chain.VerifyAuditIntegrityWithCheckpoint(ctx, company); valid ||
				!errors.Is(err, publicgovernance.ErrAuditCheckpointMismatch) {
				t.Fatalf("%s: checkpoint verification = (%v, %v), want fail closed", point, valid, err)
			}
		})
	}
}
