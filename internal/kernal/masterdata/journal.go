package masterdata

import (
	"errors"
	"strings"

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
	return nil
}

func (j Journal) AllowsCurrency(currency string) bool {
	return currency == "" // Journals without policy do not restrict currency
}

func (j JournalType) Validate() error {
	if j.ID == (types.JournalID{}) || j.CompanyID == (types.CompanyID{}) ||
		strings.TrimSpace(j.Code) == "" || strings.TrimSpace(j.Name) == "" {
		return ErrInvalidJournal
	}
	return nil
}

func (j JournalType) AllowsCurrency(currency string) bool {
	if len(j.AllowedCurrencies) == 0 {
		return true
	}
	for _, allowed := range j.AllowedCurrencies {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(currency)) {
			return true
		}
	}
	return false
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
