package masterdata

import (
	"errors"
	"testing"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
)

func TestCoverageJournalHelpers(t *testing.T) {
	id := NewJournalID()
	if id == (JournalID{}) {
		t.Fatal("NewJournalID returned a zero ID")
	}
	valid := Journal{
		ID:        id,
		CompanyID: types.CompanyID(id),
		Code:      "GEN",
		Name:      "General Journal",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid journal: %v", err)
	}
	invalid := []Journal{
		{},
		{ID: id, CompanyID: types.CompanyID(id), Code: "   ", Name: "General"},
		{ID: id, CompanyID: types.CompanyID(id), Code: "GEN", Name: " "},
		{ID: id, CompanyID: types.CompanyID(id), Code: string(make([]rune, 33)), Name: "General"},
		{ID: id, CompanyID: types.CompanyID(id), Code: "GEN", Name: string(make([]rune, 101))},
	}
	for _, journal := range invalid {
		if err := journal.Validate(); !errors.Is(err, ErrInvalidJournal) {
			t.Fatalf("journal %+v error = %v", journal, err)
		}
	}
}

func TestCoverageMasterDataErrorAliases(t *testing.T) {
	if !errors.Is(ErrCapabilityNotImplemented, ErrCapabilityNotImplemented) {
		t.Fatal("capability error alias mismatch")
	}
	if !errors.Is(ErrAccountNotFound, ErrAccountNotFound) {
		t.Fatal("account error alias mismatch")
	}
	for _, err := range []error{
		ErrInvalidCompany, ErrDuplicateCompanyCode, ErrInvalidAccount, ErrDuplicateAccountCode,
		ErrInvalidFiscalYear, ErrFiscalYearNotFound, ErrInvalidFiscalPeriod, ErrPeriodNotFound,
		ErrInvalidJournal, ErrCompanyNotFound, ErrJournalNotFound, ErrDuplicateJournalCode,
		ErrDuplicateJournalID, ErrInvalidCostCenter, ErrDuplicateCostCenter,
		ErrInvalidAnalyticalDimension, ErrDuplicateAnalyticalDimension,
		ErrInvalidEffectiveRange, ErrAccountVersionOverlap,
	} {
		if err == nil || err.Error() == "" {
			t.Fatal("master data sentinel errors must be non-empty")
		}
	}
}
