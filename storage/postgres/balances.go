package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

// GetAccountBalance returns account balance calculated by database query.
func (d *Driver) GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error) {
	var balance decimal.Decimal
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		uow := &postgresUnitOfWork{driver: d, tx: tx}
		var balErr error
		balance, balErr = uow.GetAccountBalance(ctx, companyID, accountID, asOf, currency)
		return balErr
	})
	return balance, err
}

