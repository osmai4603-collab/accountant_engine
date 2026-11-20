package memory

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

func (d *Driver) GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error) {
	if err := ctx.Err(); err != nil {
		return decimal.Zero, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()

	// If currency is empty, attempt to resolve from company functional currency
	if currency == "" {
		if comp, ok := d.companies[companyID]; ok {
			currency = comp.FunctionalCurrency
		}
	}

	key := accountBalanceKey{
		companyID: companyID,
		accountID: accountID,
		currency:  currency,
	}

	bal, ok := d.balances[key]
	if !ok {
		return decimal.Zero, nil
	}
	return bal, nil
}

