package ledger

import (
	"testing"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

func TestLedgerPeriodContains(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.January, 31, 23, 59, 59, 0, time.UTC)
	period := FiscalPeriod{
		ID: types.NewFiscalPeriodID(), CompanyID: types.NewCompanyID(),
		FiscalYearID: types.NewFiscalYearID(), Number: 1,
		StartDate: start, EndDate: end, State: PeriodOpen, Type: FiscalPeriodTypeNormal,
	}
	if !period.Contains(start) || !period.Contains(end) {
		t.Fatal("period must contain its boundaries")
	}
	if period.Contains(end.Add(time.Second)) {
		t.Fatal("period must reject dates after its end")
	}
	for _, adjustmentType := range []FiscalPeriodType{
		FiscalPeriodTypeYearEndAdjustment,
		FiscalPeriodTypeAuditAdjustment,
	} {
		period.Type = adjustmentType
		if period.Contains(start) {
			t.Fatalf("adjustment period %s must not participate in date search", adjustmentType)
		}
	}
}
