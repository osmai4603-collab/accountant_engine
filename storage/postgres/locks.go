package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

// WithinPostingLock acquires a transaction-scoped advisory lock on (company, journal)
// before executing fn within a unit of work.
func (d *Driver) WithinPostingLock(
	ctx context.Context,
	companyID uuid.UUID,
	journalID uuid.UUID,
	fn func(context.Context) error,
) error {
	if fn == nil {
		return ErrInvalidEntry
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, active := ctx.Value(transactionContextKey{}).(transactionScope); active {
		if err := d.acquirePostingAdvisoryLock(ctx, companyID, journalID); err != nil {
			return err
		}
		return fn(ctx)
	}
	lockCtx := storage.WithCompanyContext(ctx, types.CompanyID(companyID))
	return d.ExecuteWithOptions(lockCtx, storage.TxOptions{}, func(txCtx context.Context, _ storage.UnitOfWork) error {
		if err := d.acquirePostingAdvisoryLock(txCtx, companyID, journalID); err != nil {
			return err
		}
		return fn(txCtx)
	})
}

func (d *Driver) acquirePostingAdvisoryLock(ctx context.Context, companyID, journalID uuid.UUID) error {
	scope, ok := ctx.Value(transactionContextKey{}).(transactionScope)
	if !ok {
		return fmt.Errorf("postgres: posting lock requires an active transaction")
	}
	if scope.companyID != companyID {
		return fmt.Errorf("postgres: transaction company does not match requested company")
	}
	key := companyID.String() + ":" + journalID.String()
	if _, err := scope.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return translateTransactionError(err)
	}
	return nil
}

