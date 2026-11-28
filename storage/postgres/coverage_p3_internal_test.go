package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// Exercise unexported helpers whose error branches are otherwise only reached
// on database faults. All cases here are side-effect free.
func TestCoverageP3PostgresPureHelpers(t *testing.T) {
	if got := nullableString("", "USD"); got != "USD" {
		t.Fatalf("nullableString fallback = %q", got)
	}
	if got := nullableString("EUR", "USD"); got != "EUR" {
		t.Fatalf("nullableString value = %q", got)
	}
	if defaultTime(time.Time{}).IsZero() {
		t.Fatal("defaultTime left a zero timestamp")
	}
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := defaultTime(fixed); !got.Equal(fixed) {
		t.Fatalf("defaultTime changed a non-zero timestamp to %v", got)
	}

	if _, err := listMigrations("does-not-exist"); err == nil {
		t.Fatal("expected missing embedded migration directory to fail")
	}
	if sum := migrationChecksum([]byte("p3")); len(sum) != 64 {
		t.Fatalf("migration checksum length = %d", len(sum))
	}
	migrations := []migrationFile{{version: 1, upPath: "a"}, {version: 2, upPath: "b"}}
	if got, ok := findMigration(migrations, 2); !ok || got.version != 2 {
		t.Fatalf("findMigration existing = %+v, %v", got, ok)
	}
	if _, ok := findMigration(migrations, 99); ok {
		t.Fatal("findMigration returned a missing version")
	}

	if options, err := pgxTxOptions(storage.TxOptions{}); err != nil || options.AccessMode != pgx.ReadWrite {
		t.Fatalf("pgxTxOptions defaults = %+v, %v", options, err)
	}
	for _, isolation := range []storage.IsolationLevel{storage.IsolationReadCommitted, storage.IsolationRepeatableRead, storage.IsolationSerializable} {
		if _, err := pgxTxOptions(storage.TxOptions{Isolation: isolation, ReadOnly: true}); err != nil {
			t.Fatalf("pgxTxOptions(%q): %v", isolation, err)
		}
	}
	if _, err := pgxTxOptions(storage.TxOptions{Isolation: "bogus"}); err == nil {
		t.Fatal("expected invalid isolation level to fail")
	}

	for code, expected := range map[string]error{
		"40001": storage.ErrSerializationFailure,
		"40P01": storage.ErrDeadlockDetected,
		"23505": storage.ErrUniqueConstraint,
		"57014": storage.ErrTransactionAborted,
	} {
		pgErr := &pgconn.PgError{Code: code}
		if err := translateTransactionError(pgErr); !errors.Is(err, expected) {
			t.Errorf("translate %s = %v, want %v", code, err, expected)
		}
	}
	if err := translateTransactionError(errors.New("plain failure")); err == nil {
		t.Fatal("expected an unclassified error to be returned")
	}
	if err := translateTransactionError(nil); err != nil {
		t.Fatalf("translate nil = %v", err)
	}
	if err := translateTransactionError(context.Canceled); !errors.Is(err, storage.ErrTransactionAborted) {
		t.Fatalf("translate cancellation = %v", err)
	}
}

func TestCoverageP3PostgresEntryAliasCodec(t *testing.T) {
	originalID := types.NewJournalEntryID()
	reversalID := types.NewJournalEntryID()
	entry := journal.JournalEntry{
		ID:             types.NewJournalEntryID(),
		CompanyID:      types.NewCompanyID(),
		JournalID:      types.NewJournalID(),
		Status:         journal.EntryReversed,
		ReversalOfID:   &originalID,
		ReversedByID:   &reversalID,
		SequenceNumber: 7,
	}
	data, err := marshalEntryForStorage(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	decoded, err := unmarshalEntryFromStorage(data)
	if err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if decoded.ReversalOfID == nil || *decoded.ReversalOfID != originalID {
		t.Fatalf("OriginalEntryID alias = %v", decoded.ReversalOfID)
	}
	if decoded.ReversedByID == nil || *decoded.ReversedByID != reversalID {
		t.Fatalf("ReversedEntryID alias = %v", decoded.ReversedByID)
	}
	for _, payload := range []string{`not-json`, `{"ReversedEntryID":"not-a-uuid"}`, `{"OriginalEntryID":"not-a-uuid"}`} {
		if _, err := unmarshalEntryFromStorage([]byte(payload)); err == nil {
			t.Errorf("expected malformed payload %q to fail", payload)
		}
	}
}

func TestCoverageP3PostgresNilGuards(t *testing.T) {
	ctx := context.Background()
	companyID := types.NewCompanyID()

	var driver *Driver
	if _, err := driver.AppendAuditLogAtomic(ctx, publicgovernance.AuditLog{CompanyID: companyID}); !errors.Is(err, ErrDriverNotInitialized) {
		t.Fatalf("nil driver AppendAuditLogAtomic = %v", err)
	}
	if err := driver.AppendAuditLog(ctx, publicgovernance.AuditLog{CompanyID: companyID}); !errors.Is(err, ErrDriverNotInitialized) {
		t.Fatalf("nil driver AppendAuditLog = %v", err)
	}
	if _, err := driver.GetLastAuditLogForCompany(ctx, companyID); !errors.Is(err, ErrDriverNotInitialized) {
		t.Fatalf("nil driver GetLastAuditLogForCompany = %v", err)
	}
	if err := driver.IterateAuditLogs(ctx, companyID, func(publicgovernance.AuditLog) error { return nil }); !errors.Is(err, ErrDriverNotInitialized) {
		t.Fatalf("nil driver IterateAuditLogs = %v", err)
	}
	if _, err := driver.GetAuditVerificationSnapshot(ctx, companyID); !errors.Is(err, ErrDriverNotInitialized) {
		t.Fatalf("nil driver GetAuditVerificationSnapshot = %v", err)
	}
	if err := driver.SetExternalAuditCheckpoint(ctx, companyID, publicgovernance.AuditCheckpoint{}); !errors.Is(err, ErrDriverNotInitialized) {
		t.Fatalf("nil driver SetExternalAuditCheckpoint = %v", err)
	}

	if _, _, err := claimIdempotencyTx(ctx, nil, storage.IdempotencyRecord{}); err == nil {
		t.Fatal("expected empty idempotency claim to fail")
	}
	uow := &postgresUnitOfWork{}
	if _, err := uow.acquireOrderedAdvisoryLock(ctx, storage.LockTarget{}); err == nil {
		t.Fatal("expected invalid lock target to fail")
	}
	validTarget := storage.LockTarget{Kind: storage.LockKindEntry, CompanyID: companyID, ID: "entry"}
	if _, err := uow.acquireOrderedAdvisoryLock(ctx, validTarget); err == nil {
		t.Fatal("expected ordered lock without a transaction to fail")
	}
	otherCompany := uuid.UUID(types.NewCompanyID())
	scopedCtx := context.WithValue(ctx, transactionContextKey{}, transactionScope{companyID: otherCompany})
	if _, err := uow.acquireOrderedAdvisoryLock(scopedCtx, validTarget); !errors.Is(err, storage.ErrScopeViolation) {
		t.Fatalf("ordered lock company mismatch = %v", err)
	}
	if err := (&Driver{}).acquirePostingAdvisoryLock(ctx, uuid.UUID(companyID), uuid.New()); err == nil {
		t.Fatal("expected posting lock without a transaction to fail")
	}

	if err := ValidateSchemaHealth(ctx, nil); err == nil {
		t.Fatal("expected nil pool schema health check to fail")
	}
	if err := MigrateDown(ctx, nil, 0); err == nil {
		t.Fatal("expected non-positive migration rollback to fail")
	}
}
