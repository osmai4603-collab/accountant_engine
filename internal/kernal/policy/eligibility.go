package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

// CheckAccountEligibility verifies whether the accounts are eligible for the given transaction currency
// and whether control accounts are respected.
func CheckAccountEligibility(
	ctx context.Context,
	entry *journal.JournalEntry,
	accounts map[apiTypes.AccountID]masterdata.Account,
) error {
	if entry == nil {
		return errors.New("journal entry is nil")
	}

	for _, line := range entry.Lines {
		acc, ok := accounts[line.AccountID]
		if !ok {
			return fmt.Errorf("account %s not found", line.AccountID)
		}

		if !acc.IsActive {
			return fmt.Errorf("account %s is inactive", acc.Code)
		}

		// If account specifies a fixed currency, transaction/functional currency must match
		if acc.Currency != "" && !strings.EqualFold(acc.Currency, line.TransactionCurrency) {
			return fmt.Errorf("%w: account %s requires currency %s, got %s",
				apiTypes.ErrCurrencyMismatch, acc.Code, acc.Currency, line.TransactionCurrency)
		}
	}

	return nil
}
