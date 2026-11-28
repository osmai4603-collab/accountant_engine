package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	"github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	apiTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

var (
	ErrNilJournalEntry           = errors.New("journal entry is nil")
	ErrAccountNotFound           = errors.New("account not found for posting")
	ErrAccountInactive           = errors.New("account is inactive")
	ErrAccountNotEffective       = errors.New("account is not effective on the entry date")
	ErrControlAccountNotPostable = errors.New("control accounts cannot be posted directly")
)

// CheckAccountEligibility verifies whether the accounts are eligible for the given transaction currency
// and whether control accounts are respected.
func CheckAccountEligibility(
	ctx context.Context,
	entry *journal.JournalEntry,
	accounts map[apiTypes.AccountID]masterdata.Account,
) error {
	if entry == nil {
		return ErrNilJournalEntry
	}

	for _, line := range entry.Lines {
		acc, ok := accounts[line.AccountID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrAccountNotFound, line.AccountID)
		}
		if entry.CompanyID != (apiTypes.CompanyID{}) {
			if err := masterdata.ValidateAccountScope(acc, entry.CompanyID, line.AccountID); err != nil {
				return err
			}
		}

		if !acc.IsActive {
			return fmt.Errorf("%w: %s", ErrAccountInactive, acc.Code)
		}
		if !entry.EffectiveDate.IsZero() && !acc.IsCurrent(entry.EffectiveDate) {
			return fmt.Errorf("%w: account %s", ErrAccountNotEffective, acc.Code)
		}
		if acc.IsControlAccount {
			return fmt.Errorf("%w: %s", ErrControlAccountNotPostable, acc.Code)
		}
		if acc.RequiresCostCenter &&
			(line.CostCenterID == nil || *line.CostCenterID == (apiTypes.CostCenterID{})) {
			return fmt.Errorf("%w: account %s requires a cost center", ErrCostCenterRequired, acc.Code)
		}

		// If account specifies a fixed currency, transaction/functional currency must match
		if acc.Currency != "" && !strings.EqualFold(acc.Currency, line.TransactionCurrency) {
			return fmt.Errorf("%w: account %s requires currency %s, got %s",
				apiTypes.ErrCurrencyMismatch, acc.Code, acc.Currency, line.TransactionCurrency)
		}
	}

	return nil
}
