package memory

import (
	"context"
	"errors"
	"fmt"

	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var ErrReadOnlyTransaction = errors.New("memory: write attempted in read-only transaction")

type transactionContextKey struct{}

type memoryTransactionScope struct {
	driver   *Driver
	readOnly bool
}

// UnitOfWork exposes transactional store ports bound to a single transaction scope.
type memoryUnitOfWork struct {
	driver *Driver
}

func (u *memoryUnitOfWork) Drafts() storage.DraftStore {
	return u.driver
}

func (u *memoryUnitOfWork) Entries() storage.EntryReader {
	return u.driver
}

func (u *memoryUnitOfWork) Posting() storage.PostingWriter {
	return u.driver
}

func (u *memoryUnitOfWork) Sequences() storage.SequenceAllocator {
	return u.driver
}

func (u *memoryUnitOfWork) Audit() storage.AuditWriter {
	return u.driver
}

func (u *memoryUnitOfWork) Idempotency() storage.IdempotencyStore {
	return u.driver
}

func (u *memoryUnitOfWork) Balance() storage.BalanceReader {
	return u.driver
}

// Capabilities reports the guarantees Memory can provide within one process.
func (d *Driver) Capabilities() storage.AdapterCapabilities {
	return storage.AdapterCapabilities{
		AdapterName:            "memory",
		SupportsRepeatableRead: true,
		SupportsSerializable:   true,
		EnforcesForeignKeys:    false,
		GuaranteesDurability:   false,
		SupportsSavepoints:     false,
		DetectsDeadlocks:       false,
	}
}

// WithinTransaction runs fn inside a transaction scope.
func (d *Driver) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return d.Execute(ctx, func(uowCtx context.Context, uow storage.UnitOfWork) error {
		return fn(uowCtx)
	})
}

// WithinTransactionWithOptions runs fn with explicit transaction options.
func (d *Driver) WithinTransactionWithOptions(ctx context.Context, opts storage.TxOptions, fn func(context.Context) error) error {
	return d.ExecuteWithOptions(ctx, opts, func(uowCtx context.Context, uow storage.UnitOfWork) error {
		return fn(uowCtx)
	})
}

// Execute runs fn within a unit of work with default options.
func (d *Driver) Execute(ctx context.Context, fn func(context.Context, storage.UnitOfWork) error) error {
	return d.ExecuteWithOptions(ctx, storage.TxOptions{}, fn)
}

// ExecuteWithOptions executes fn inside an isolated snapshot. If fn returns an error
// or panics, all state mutations are cleanly discarded (rollback).
func (d *Driver) ExecuteWithOptions(ctx context.Context, opts storage.TxOptions, fn func(context.Context, storage.UnitOfWork) error) (err error) {
	if fn == nil {
		return errors.New("memory: nil unit-of-work callback")
	}
	switch opts.Isolation {
	case "", storage.IsolationReadCommitted, storage.IsolationRepeatableRead, storage.IsolationSerializable:
	default:
		return fmt.Errorf("memory: unsupported isolation level %q", opts.Isolation)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", storage.ErrTransactionAborted, err)
	}

	if scope, nested := ctx.Value(transactionContextKey{}).(memoryTransactionScope); nested {
		// Nested unit of work reuses active transaction
		return fn(ctx, &memoryUnitOfWork{driver: scope.driver})
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Snapshot state before executing callback
	snapshot := d.clone()
	if opts.ReadOnly {
		snapshot.readOnly = true
	}

	txCtx := context.WithValue(ctx, transactionContextKey{}, memoryTransactionScope{
		driver:   snapshot,
		readOnly: opts.ReadOnly,
	})

	uow := &memoryUnitOfWork{driver: snapshot}

	defer func() {
		if r := recover(); r != nil {
			// On panic, snapshot is discarded (rollback)
			panic(r)
		}
	}()

	if err := fn(txCtx, uow); err != nil {
		// Rollback: snapshot is discarded, d is untouched
		return err
	}

	if !opts.ReadOnly {
		// Commit: copy back mutated snapshot to root driver
		d.copyFrom(snapshot)
	}
	return nil
}

