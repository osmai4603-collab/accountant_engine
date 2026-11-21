package types

import "testing"

func TestKernelIdentifierConstructorsAndConversions(t *testing.T) {
	ids := []struct {
		name string
		id   string
	}{
		{"company", NewCompanyID().String()},
		{"account", NewAccountID().String()},
		{"fiscal-year", NewFiscalYearID().String()},
		{"fiscal-period", NewFiscalPeriodID().String()},
		{"journal", NewJournalID().String()},
		{"entry", NewJournalEntryID().String()},
		{"entry-line", NewJournalEntryLineID().String()},
		{"partner", NewPartnerID().String()},
		{"cost-center", NewCostCenterID().String()},
		{"project", NewProjectID().String()},
		{"segment", NewSegmentID().String()},
		{"business-unit", NewBusinessUnitID().String()},
	}
	for _, item := range ids {
		if item.id == "" {
			t.Fatalf("%s identifier is empty", item.name)
		}
	}

	company := NewCompanyID()
	account := NewAccountID()
	fiscalYear := NewFiscalYearID()
	fiscalPeriod := NewFiscalPeriodID()
	journal := NewJournalID()
	entry := NewJournalEntryID()
	line := NewJournalEntryLineID()
	partner := NewPartnerID()
	costCenter := NewCostCenterID()
	project := NewProjectID()
	segment := NewSegmentID()
	unit := NewBusinessUnitID()

	if company.UUID() != company.UUID() || company.String() == "" ||
		account.UUID().String() == "" || fiscalYear.UUID().String() == "" ||
		fiscalPeriod.UUID().String() == "" || journal.UUID().String() == "" ||
		entry.UUID().String() == "" || line.UUID().String() == "" ||
		partner.UUID().String() == "" || costCenter.UUID().String() == "" ||
		project.UUID().String() == "" || segment.UUID().String() == "" ||
		unit.UUID().String() == "" {
		t.Fatal("identifier UUID conversions failed")
	}
}
