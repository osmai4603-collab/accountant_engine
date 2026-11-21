package masterdata

import (
	"errors"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrInvalidJournalType = errors.New("invalid journal type")
	ErrJournalCurrency    = errors.New("journal currency is not allowed")
	ErrJournalAccount     = errors.New("account is not allowed for journal")
)

type JournalType struct {
	ID                  types.JournalID
	CompanyID           types.CompanyID
	Code                string
	Name                string
	AllowedCurrencies   []string
	AllowedAccountIDs   []types.AccountID
	AutomaticallyPosted bool
}

func (j Journal) ValidateWithPolicy(allowedCurrencies []string, allowedAccounts []types.AccountID) error {
	if j.ID == (types.JournalID{}) || j.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(j.Code) == "" || strings.TrimSpace(j.Name) == "" {
		return ErrInvalidJournal
	}
	seenCurrencies := make(map[string]struct{}, len(allowedCurrencies))
	for _, currency := range allowedCurrencies {
		normalized := strings.ToUpper(strings.TrimSpace(currency))
		if currencyInfo, ok := types.LookupCurrency(normalized); !ok || currencyInfo.Code == types.XXX {
			return ErrJournalCurrency
		}
		if _, exists := seenCurrencies[normalized]; exists {
			return ErrJournalCurrency
		}
		seenCurrencies[normalized] = struct{}{}
	}
	seenAccounts := make(map[types.AccountID]struct{}, len(allowedAccounts))
	for _, accountID := range allowedAccounts {
		if accountID == (types.AccountID{}) {
			return ErrJournalAccount
		}
		if _, exists := seenAccounts[accountID]; exists {
			return ErrJournalAccount
		}
		seenAccounts[accountID] = struct{}{}
	}
	return nil
}

func (j Journal) Validate() error {
	if !j.EffectiveFrom.IsZero() && j.EffectiveTo != nil &&
		j.EffectiveTo.UTC().Before(j.EffectiveFrom.UTC()) {
		return ErrInvalidJournal
	}
	return j.ValidateWithPolicy(j.AllowedCurrencies, j.AllowedAccountIDs)
}

func (j Journal) IsCurrent(asOf time.Time) bool {
	if asOf.IsZero() {
		return false
	}
	asOf = asOf.UTC()
	if !j.EffectiveFrom.IsZero() && asOf.Before(j.EffectiveFrom.UTC()) {
		return false
	}
	return j.EffectiveTo == nil || !asOf.After(j.EffectiveTo.UTC())
}

func (j Journal) AllowsCurrency(currency string) bool {
	return j.AllowsCurrencyWithPolicy(currency, j.AllowedCurrencies)
}

func (j Journal) AllowsAccount(accountID types.AccountID) bool {
	return j.AllowsAccountWithPolicy(accountID, j.AllowedAccountIDs)
}

func (j Journal) AllowsCurrencyWithPolicy(currency string, allowedCurrencies []string) bool {
	currencyInfo, ok := types.LookupCurrency(currency)
	if !ok || currencyInfo.Code == types.XXX {
		return false
	}
	if len(allowedCurrencies) == 0 {
		return true
	}
	for _, allowed := range allowedCurrencies {
		allowedInfo, exists := types.LookupCurrency(allowed)
		if exists && allowedInfo.Code != types.XXX && allowedInfo.Code == currencyInfo.Code {
			return true
		}
	}
	return false
}

func (j Journal) AllowsAccountWithPolicy(accountID types.AccountID, allowedAccounts []types.AccountID) bool {
	if len(allowedAccounts) == 0 {
		return true
	}
	for _, allowed := range allowedAccounts {
		if allowed == accountID {
			return true
		}
	}
	return false
}

func (j JournalType) Validate() error {
	if j.ID == (types.JournalID{}) || j.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(j.Code) == "" || strings.TrimSpace(j.Name) == "" {
		return ErrInvalidJournal
	}
	return (Journal{ID: j.ID, CompanyID: j.CompanyID, Code: j.Code, Name: j.Name}).ValidateWithPolicy(j.AllowedCurrencies, j.AllowedAccountIDs)
}

func (j JournalType) AllowsCurrency(currency string) bool {
	return (Journal{AllowedCurrencies: j.AllowedCurrencies}).AllowsCurrency(currency)
}

func (j JournalType) AllowsAccount(accountID types.AccountID) bool {
	if len(j.AllowedAccountIDs) == 0 {
		return true
	}
	for _, allowed := range j.AllowedAccountIDs {
		if allowed == accountID {
			return true
		}
	}
	return false
}
