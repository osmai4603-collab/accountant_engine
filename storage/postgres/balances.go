package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

var _ storage.TransactionBalanceReader = (*Driver)(nil)

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

// GetTransactionBalances implements storage.TransactionBalanceReader (LDG-003)
// alongside BalanceReader without modifying BalanceReader semantics.
func (d *Driver) GetTransactionBalances(ctx context.Context, filter ledger.ForeignCurrencyBalanceFilter) ([]ledger.ForeignCurrencyBalance, error) {
	if filter.CompanyID == (types.CompanyID{}) {
		return nil, errors.New("postgres: company_id is required")
	}
	var out []ledger.ForeignCurrencyBalance
	err := d.WithCompanyTx(ctx, uuid.UUID(filter.CompanyID), func(tx pgx.Tx) error {
		uow := &postgresUnitOfWork{driver: d, tx: tx}
		var err error
		out, err = uow.GetTransactionBalances(ctx, filter)
		return err
	})
	return out, err
}
