package masterdata

import (
	"errors"
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

func TestJournalPolicyValidationUsesAllowedLists(t *testing.T) {
	journal := Journal{
		ID: types.NewJournalID(), CompanyID: types.NewCompanyID(), Code: "GEN", Name: "General",
	}

	accountID := types.NewAccountID()
	if err := journal.ValidateWithPolicy([]string{"USD", "EUR"}, []types.AccountID{accountID}); err != nil {
		t.Fatalf("valid journal policy: %v", err)
	}
	if err := journal.ValidateWithPolicy([]string{"INVALID"}, []types.AccountID{accountID}); !errors.Is(err, ErrJournalCurrency) {
		t.Fatalf("invalid allowed currency error = %v, want ErrJournalCurrency", err)
	}
	if err := journal.ValidateWithPolicy([]string{"XXX"}, []types.AccountID{accountID}); !errors.Is(err, ErrJournalCurrency) {
		t.Fatalf("no-currency allowed code error = %v, want ErrJournalCurrency", err)
	}
	if err := journal.ValidateWithPolicy([]string{"USD"}, []types.AccountID{{}}); !errors.Is(err, ErrJournalAccount) {
		t.Fatalf("empty allowed account error = %v, want ErrJournalAccount", err)
	}
	if !journal.AllowsCurrency("USD") || !journal.AllowsCurrency("") {
		t.Fatal("journal without a configured currency policy should not restrict currencies")
	}
	if !journal.AllowsCurrencyWithPolicy("usd", []string{"USD"}) ||
		journal.AllowsCurrencyWithPolicy("EUR", []string{"USD"}) {
		t.Fatal("journal currency policy did not enforce its allow-list")
	}
	if !journal.AllowsAccountWithPolicy(accountID, []types.AccountID{accountID}) ||
		journal.AllowsAccountWithPolicy(types.NewAccountID(), []types.AccountID{accountID}) {
		t.Fatal("journal account policy did not enforce its allow-list")
	}
}

func TestJournalValidityUsesEffectiveDate(t *testing.T) {
	effectiveTo := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	journal := Journal{
		ID: types.NewJournalID(), CompanyID: types.NewCompanyID(), Code: "GEN", Name: "General",
		EffectiveTo: &effectiveTo,
	}
	if !journal.IsCurrent(effectiveTo) {
		t.Fatal("journal should remain valid on its inclusive expiration date")
	}
	if journal.IsCurrent(effectiveTo.Add(time.Nanosecond)) {
		t.Fatal("journal should be invalid after its effective-to date")
	}
	if journal.IsCurrent(time.Time{}) {
		t.Fatal("journal should not be considered valid for a zero posting date")
	}
}
