package masterdata

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var (
	ErrPeriodOutsideFiscalYear      = errors.New("period dates must be within the fiscal year")
	ErrInvalidFiscalYearTransition  = errors.New("invalid fiscal year transition")
	ErrInvalidPeriodTransition      = types.ErrInvalidPeriodTransition
	ErrPeriodTransitionNeedsAudit   = errors.New("fiscal period transition requires an actor and reason")
	ErrPeriodReopenRequiresAudit    = errors.New("reopening a fiscal period requires an actor and reason")
	ErrPeriodReopenRequiresApproval = errors.New("reopening a fiscal period requires approval metadata")
	ErrFiscalPeriodClosed           = types.ErrFiscalPeriodClosed
	ErrFiscalPeriodNotFound         = types.ErrFiscalPeriodNotFound
)

// GenerateFiscalYear generates a standard 12-month fiscal year plus 2 adjustment periods.
func GenerateFiscalYear(companyID types.CompanyID, start time.Time) (masterdata.FiscalYear, []masterdata.FiscalPeriod) {
	start = start.UTC()
	fyID := types.NewFiscalYearID()
	year := masterdata.FiscalYear{
		ID:        fyID,
		CompanyID: companyID,
		Name:      start.Format("2006"),
		StartDate: start,
		EndDate:   start.AddDate(1, 0, 0).Add(-time.Nanosecond),
		State:     masterdata.FiscalYearOpen,
	}

	var periods []masterdata.FiscalPeriod
	for i := 1; i <= 12; i++ {
		pStart := start.AddDate(0, i-1, 0)
		pEnd := start.AddDate(0, i, 0).Add(-time.Nanosecond)
		periods = append(periods, masterdata.FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    companyID,
			FiscalYearID: fyID,
			Number:       i,
			Name:         pStart.Format("January 2006"),
			StartDate:    pStart,
			EndDate:      pEnd,
			State:        masterdata.PeriodOpen,
			Type:         masterdata.FiscalPeriodTypeNormal,
		})
	}

	// Adjustment periods (13 & 14)
	periods = append(periods,
		masterdata.FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    companyID,
			FiscalYearID: fyID,
			Number:       13,
			Name:         "Period 13",
			StartDate:    year.EndDate,
			EndDate:      year.EndDate,
			State:        masterdata.PeriodOpen,
			Type:         masterdata.FiscalPeriodTypeYearEndAdjustment,
		},
		masterdata.FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    companyID,
			FiscalYearID: fyID,
			Number:       14,
			Name:         "Period 14",
			StartDate:    year.EndDate,
			EndDate:      year.EndDate,
			State:        masterdata.PeriodOpen,
			Type:         masterdata.FiscalPeriodTypeAuditAdjustment,
		},
	)

	return year, periods
}

// ValidatePeriodTransition verifies state transitions for fiscal periods.
func ValidatePeriodTransition(from, to masterdata.PeriodState) error {
	switch {
	case from == masterdata.PeriodOpen && to == masterdata.PeriodSoftClosed:
	case from == masterdata.PeriodSoftClosed && to == masterdata.PeriodHardClosed:
	default:
		return ErrInvalidPeriodTransition
	}
	return nil
}

// ValidatePeriodReopen validates reopening an audited closed period.
func ValidatePeriodReopen(from, to masterdata.PeriodState, actorID string, reason string) error {
	if to != masterdata.PeriodOpen || (from != masterdata.PeriodSoftClosed && from != masterdata.PeriodHardClosed) {
		return ErrInvalidPeriodTransition
	}
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(reason) == "" {
		return ErrPeriodReopenRequiresAudit
	}
	return nil
}

// ValidatePeriodForPosting verifies that a fiscal period is open for posting on the given date.
func ValidatePeriodForPosting(ctx context.Context, reader storage.MasterDataReader, companyID types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error) {
	period, err := reader.GetPeriodForDate(ctx, companyID, date)
	if err != nil {
		return masterdata.FiscalPeriod{}, err
	}
	if period.State != masterdata.PeriodOpen {
		return period, ErrClosedPeriod
	}
	return period, nil
}

