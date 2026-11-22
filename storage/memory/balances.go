package memory

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

// GetAccountBalance derives the balance from committed, posted entries so the
// as-of boundary and functional-currency resolution follow the same rules as
// the PostgreSQL adapter. A reversal replaces its original because the
// original entry is moved to the reversed state at commit time.
func (d *Driver) GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error) {
	if err := ctx.Err(); err != nil {
		return decimal.Zero, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()

	if currency == "" {
		if comp, ok := d.companies[companyID]; ok {
			currency = comp.FunctionalCurrency
		}
	}

	total := decimal.Zero
	for _, entry := range d.entries[companyID] {
		if entry.Status != journal.EntryPosted {
			continue
		}
		if !asOf.IsZero() && entry.PostingDate.After(asOf) {
			continue
		}
		for _, line := range entry.Lines {
			if line.AccountID != accountID {
				continue
			}
			lineCurrency := line.TransactionCurrency
			if lineCurrency == "" {
				lineCurrency = line.FunctionalDebit.Currency()
			}
			if lineCurrency == "" {
				lineCurrency = line.FunctionalCredit.Currency()
			}
			if currency != "" && lineCurrency != currency {
				continue
			}
			total = total.Add(line.FunctionalDebit.Amount().Sub(line.FunctionalCredit.Amount()))
		}
	}
	return total, nil
}
