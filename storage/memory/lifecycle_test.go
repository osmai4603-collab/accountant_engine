package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func TestAdapterCloseReleasesDriverState(t *testing.T) {
	adapter := New()
	driver := adapter.Driver()
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}

	ctx := storage.WithLedgerContext(context.Background(), "STATUTORY")
	entryID := types.NewJournalEntryID()

	driver.setEntryLedger(ctx, entryID)
	if !driver.entryVisibleInLedger(ctx, entryID) {
		t.Fatal("expected entry to be visible in STATUTORY ledger before Close")
	}
	if _, ok := multiLedgerEntryOwners.Load(driver); !ok {
		t.Fatal("expected multiLedgerEntryOwners to contain driver before Close")
	}

	// Verify rolled-back UOW does not mutate root driver's entryLedgers or leak snapshot.
	sentinelErr := errors.New("rollback")
	rolledBackEntry := types.NewJournalEntryID()
	var leakedSnapshot *Driver
	err := driver.ExecuteWithOptions(ctx, storage.TxOptions{}, func(txCtx context.Context, _ storage.UnitOfWork) error {
		scope := txCtx.Value(transactionContextKey{}).(memoryTransactionScope)
		leakedSnapshot = scope.driver
		scope.driver.setEntryLedger(txCtx, rolledBackEntry)
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinelErr, got %v", err)
	}
	if driver.entryVisibleInLedger(ctx, rolledBackEntry) {
		t.Fatal("expected rolled-back entry ledger mapping not to leak into root driver")
	}
	if _, ok := multiLedgerEntryOwners.Load(leakedSnapshot); ok {
		t.Fatal("expected rolled-back snapshot to be deleted from multiLedgerEntryOwners")
	}

	if err := adapter.Close(); err != nil {
		t.Fatalf("Adapter.Close failed: %v", err)
	}
	if _, ok := multiLedgerEntryOwners.Load(driver); ok {
		t.Fatal("expected Driver.Close to delete driver from multiLedgerEntryOwners")
	}
	if _, ok := yearEndClosingsByDriver.Load(driver); ok {
		t.Fatal("expected Driver.Close to delete driver from yearEndClosingsByDriver")
	}
	if driver.entries != nil {
		t.Fatal("expected Driver.Close to release internal maps")
	}
	// Idempotent second close
	if err := adapter.Close(); err != nil {
		t.Fatalf("second Adapter.Close failed: %v", err)
	}
}
