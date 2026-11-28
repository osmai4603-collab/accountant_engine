package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var (
	_ storage.TransactionRunner         = (*Driver)(nil)
	_ storage.AdvancedTransactionRunner = (*Driver)(nil)
	_ storage.UnitOfWorkRunner          = (*Driver)(nil)
)

type transactionContextKey struct{}

type transactionScope struct {
	tx        pgx.Tx
	companyID uuid.UUID
	state     *joinedTransactionState
}

type joinedTransactionState struct {
	mu           sync.Mutex
	rollbackOnly bool
}

type joinedTx struct {
	pgx.Tx
	state *joinedTransactionState
	mu    sync.Mutex
	done  bool
}

func (tx *joinedTx) Commit(context.Context) error {
	tx.mu.Lock()
	tx.done = true
	tx.mu.Unlock()
	return nil
}

func (tx *joinedTx) Rollback(context.Context) error {
	tx.mu.Lock()
	done := tx.done
	tx.done = true
	tx.mu.Unlock()
	if !done {
		tx.state.mu.Lock()
		tx.state.rollbackOnly = true
		tx.state.mu.Unlock()
	}
	return nil
}

func (d *Driver) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return d.WithinTransactionWithOptions(ctx, storage.TxOptions{}, fn)
}

func (d *Driver) WithinTransactionWithOptions(ctx context.Context, opts storage.TxOptions, fn func(context.Context) error) error {
	return d.ExecuteWithOptions(ctx, opts, func(txCtx context.Context, _ storage.UnitOfWork) error {
		return fn(txCtx)
	})
}

func (d *Driver) Execute(ctx context.Context, fn func(context.Context, storage.UnitOfWork) error) error {
	return d.ExecuteWithOptions(ctx, storage.TxOptions{}, fn)
}

func (d *Driver) ExecuteWithOptions(ctx context.Context, opts storage.TxOptions, fn func(context.Context, storage.UnitOfWork) error) (result error) {
	if ctx == nil {
		return errors.New("postgres: transaction context is nil")
	}
	started := time.Now()
	defer func() {
		d.Observer().ObserveStorageTx(ctx, "postgres.unit_of_work", time.Since(started), result)
	}()
	if fn == nil {
		return errors.New("postgres: transaction callback is nil")
	}
	if _, nested := ctx.Value(transactionContextKey{}).(transactionScope); nested {
		return storage.ErrNestedUnitOfWork
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", storage.ErrTransactionAborted, err)
	}

	companyID, ok := storage.CompanyFromContext(ctx)
	if !ok || uuid.UUID(companyID) == uuid.Nil {
		return errors.New("postgres: transaction requires company context")
	}

	pgxOptions, err := pgxTxOptions(opts)
	if err != nil {
		return err
	}

	tx, err := d.pool.BeginTx(ctx, pgxOptions)
	if err != nil {
		return translateTransactionError(err)
	}
	rollbackState := &joinedTransactionState{}
	rollback := func() {
		if rollbackErr := tx.Rollback(context.WithoutCancel(ctx)); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			result = errors.Join(result, fmt.Errorf("rollback postgres unit of work: %w", rollbackErr))
		}
	}
	defer rollback()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_company_id', $1, true)", uuid.UUID(companyID).String()); err != nil {
		return translateTransactionError(err)
	}
	if actorID, ok := storage.ActorFromContext(ctx); ok {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_actor_id', $1, true)", actorID); err != nil {
			return translateTransactionError(err)
		}
	}

	scope := transactionScope{tx: tx, companyID: uuid.UUID(companyID), state: rollbackState}
	txCtx := context.WithValue(ctx, transactionContextKey{}, scope)
	uow := &postgresUnitOfWork{driver: d, tx: tx}

	if err := fn(txCtx, uow); err != nil {
		return translateTransactionError(err)
	}
	rollbackState.mu.Lock()
	rollbackOnly := rollbackState.rollbackOnly
	rollbackState.mu.Unlock()
	if rollbackOnly {
		return storage.ErrTransactionAborted
	}
	if err := tx.Commit(ctx); err != nil {
		return translateTransactionError(err)
	}
	return nil
}

func pgxTxOptions(opts storage.TxOptions) (pgx.TxOptions, error) {
	options := pgx.TxOptions{AccessMode: pgx.ReadWrite}
	if opts.ReadOnly {
		options.AccessMode = pgx.ReadOnly
	}
	switch opts.Isolation {
	case "", storage.IsolationReadCommitted:
		options.IsoLevel = pgx.ReadCommitted
	case storage.IsolationRepeatableRead:
		options.IsoLevel = pgx.RepeatableRead
	case storage.IsolationSerializable:
		options.IsoLevel = pgx.Serializable
	default:
		return pgx.TxOptions{}, fmt.Errorf("postgres: unsupported isolation level %q", opts.Isolation)
	}
	return options, nil
}

func translateTransactionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", storage.ErrTransactionAborted, err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "40001":
		return fmt.Errorf("%w: %v", storage.ErrSerializationFailure, err)
	case "40P01":
		return fmt.Errorf("%w: %v", storage.ErrDeadlockDetected, err)
	case "23505":
		return fmt.Errorf("%w: %v", storage.ErrUniqueConstraint, err)
	case "57014":
		return fmt.Errorf("%w: %v", storage.ErrTransactionAborted, err)
	default:
		return err
	}
}

func (d *Driver) beginCompanyTx(ctx context.Context, companyID uuid.UUID) (pgx.Tx, error) {
	if scope, ok := ctx.Value(transactionContextKey{}).(transactionScope); ok {
		if scope.companyID != companyID {
			return nil, errors.New("postgres: transaction company does not match requested company")
		}
		return &joinedTx{Tx: scope.tx, state: scope.state}, nil
	}
	return d.beginCompanyTxUnscoped(ctx, companyID)
}

func (d *Driver) beginCompanyTxUnscoped(ctx context.Context, companyID uuid.UUID) (pgx.Tx, error) {
	if companyID == uuid.Nil {
		return nil, errors.New("company transaction requires company id")
	}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_company_id', $1, true)", companyID.String()); err != nil {
		if rollbackErr := tx.Rollback(context.WithoutCancel(ctx)); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return nil, errors.Join(err, fmt.Errorf("rollback tenant context transaction: %w", rollbackErr))
		}
		return nil, err
	}
	if actorID, ok := storage.ActorFromContext(ctx); ok {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_actor_id', $1, true)", actorID); err != nil {
			if rollbackErr := tx.Rollback(context.WithoutCancel(ctx)); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
				return nil, errors.Join(err, fmt.Errorf("rollback actor context transaction: %w", rollbackErr))
			}
			return nil, err
		}
	}
	return tx, nil
}

func (d *Driver) WithCompanyTx(ctx context.Context, companyID uuid.UUID, fn func(pgx.Tx) error) (result error) {
	tx, err := d.beginCompanyTx(ctx, companyID)
	if err != nil {
		return err
	}
	finished := false
	defer func() {
		if finished {
			return
		}
		if rollbackErr := tx.Rollback(context.WithoutCancel(ctx)); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			result = errors.Join(result, fmt.Errorf("rollback company transaction: %w", rollbackErr))
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	finished = true
	return nil
}
