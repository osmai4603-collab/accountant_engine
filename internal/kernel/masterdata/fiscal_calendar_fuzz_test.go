package masterdata

import (
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func FuzzGenerateFiscalYearMaintainsContinuityAndContainment(f *testing.F) {
	for _, seed := range [][3]int{
		{2026, 1, 1},
		{2026, 1, 31},
		{2026, 2, 29},
		{2024, 2, 29},
		{2028, 12, 31},
		{2000, 3, 15},
	} {
		f.Add(seed[0], seed[1], seed[2])
	}
	f.Fuzz(func(t *testing.T, year, month, day int) {
		if year < 1 || year > 9999 {
			return
		}
		if month < 1 || month > 12 {
			return
		}
		if day < 1 || day > 31 {
			return
		}
		start := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		companyID := types.NewCompanyID()
		fiscalYear, periods := GenerateFiscalYear(companyID, start)
		if len(periods) != 14 {
			t.Fatalf("period count = %d, want 14", len(periods))
		}
		for i, period := range periods {
			if period.CompanyID != companyID || period.FiscalYearID != fiscalYear.ID {
				t.Fatalf("period %d does not belong to its generated company/fiscal year", period.Number)
			}
			if period.Number != i+1 {
				t.Fatalf("period index %d contains number %d", i, period.Number)
			}
			if period.StartDate.IsZero() || period.EndDate.IsZero() || period.EndDate.Before(period.StartDate) {
				t.Fatalf("period %d has invalid boundaries", period.Number)
			}
			if i < 12 {
				if period.Type != FiscalPeriodTypeNormal {
					t.Fatalf("normal period %d has type %q", period.Number, period.Type)
				}
				if period.StartDate.Before(fiscalYear.StartDate) || period.EndDate.After(fiscalYear.EndDate) {
					t.Fatalf("normal period %d falls outside the fiscal year", period.Number)
				}
			}
		}
		if !periods[0].StartDate.Equal(fiscalYear.StartDate) ||
			!periods[11].EndDate.Equal(fiscalYear.EndDate) {
			t.Fatal("normal periods do not span the fiscal year boundaries")
		}
		for i := 0; i < 12; i++ {
			period := periods[i]
			if i > 0 && !periods[i-1].EndDate.Add(time.Nanosecond).Equal(period.StartDate) {
				t.Fatalf("gap or overlap between periods %d and %d", i, i+1)
			}
		}
		if periods[12].Type != FiscalPeriodTypeYearEndAdjustment ||
			periods[13].Type != FiscalPeriodTypeAuditAdjustment ||
			!periods[12].StartDate.Equal(fiscalYear.EndDate) ||
			!periods[12].EndDate.Equal(fiscalYear.EndDate) ||
			!periods[13].StartDate.Equal(fiscalYear.EndDate) ||
			!periods[13].EndDate.Equal(fiscalYear.EndDate) {
			t.Fatal("adjustment periods do not have the expected types and fiscal-year-end boundaries")
		}

		// Also verify ValidateFiscalYear passes
		if err := ValidateFiscalYear(fiscalYear, periods); err != nil {
			t.Fatalf("ValidateFiscalYear failed on generated fiscal year: %v", err)
		}
	})
}
