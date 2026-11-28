package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// multiLedgerEntryOwners tracks entry-to-ledger ownership per Driver without
// altering the exported Driver struct layout (ADR P12-01). Entries are removed
// on Driver.Close() and on transaction snapshot discard/commit.
var multiLedgerEntryOwners sync.Map // map[*Driver]*sync.Map

func activeLedgerID(ctx context.Context) string {
	id, ok := storage.LedgerFromContext(ctx)
	if !ok || strings.TrimSpace(id) == "" {
		return storage.DefaultLedgerID
	}
	return strings.TrimSpace(id)
}

func ledgerScopedIdempotencyKey(ctx context.Context, key string) string {
	ledgerID := activeLedgerID(ctx)
	if ledgerID == storage.DefaultLedgerID {
		return key
	}
	return "ledger:" + ledgerID + ":" + key
}

func (d *Driver) setEntryLedger(ctx context.Context, entryID types.JournalEntryID) {
	ledgerID := activeLedgerID(ctx)
	val, _ := multiLedgerEntryOwners.LoadOrStore(d, &sync.Map{})
	val.(*sync.Map).Store(entryID, ledgerID)
}

func (d *Driver) entryVisibleInLedger(ctx context.Context, entryID types.JournalEntryID) bool {
	val, ok := multiLedgerEntryOwners.Load(d)
	if !ok {
		return activeLedgerID(ctx) == storage.DefaultLedgerID
	}
	owner, exists := val.(*sync.Map).Load(entryID)
	if !exists {
		return activeLedgerID(ctx) == storage.DefaultLedgerID
	}
	return owner.(string) == activeLedgerID(ctx)
}

func cloneMultiLedgerOwners(src, dst *Driver) {
	val, ok := multiLedgerEntryOwners.Load(src)
	if !ok {
		return
	}
	dstMap := &sync.Map{}
	val.(*sync.Map).Range(func(k, v any) bool {
		dstMap.Store(k, v)
		return true
	})
	multiLedgerEntryOwners.Store(dst, dstMap)
}

func copyMultiLedgerOwners(src, dst *Driver) {
	val, ok := multiLedgerEntryOwners.Load(src)
	if !ok {
		multiLedgerEntryOwners.Delete(dst)
		return
	}
	multiLedgerEntryOwners.Store(dst, val)
	multiLedgerEntryOwners.Delete(src)
}
