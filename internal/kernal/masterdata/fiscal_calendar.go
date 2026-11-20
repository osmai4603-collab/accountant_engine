package masterdata

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
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
func GenerateFiscalYear(companyID types.CompanyID, start time.Time) (FiscalYear, []FiscalPeriod) {
	start = start.UTC()
	fyID := types.NewFiscalYearID()
	year := FiscalYear{
		ID:        fyID,
		CompanyID: companyID,
		Name:      start.Format("2006"),
		StartDate: start,
		EndDate:   start.AddDate(1, 0, 0).Add(-time.Nanosecond),
		State:     FiscalYearOpen,
	}

	var periods []FiscalPeriod
	for i := 1; i <= 12; i++ {
		pStart := start.AddDate(0, i-1, 0)
		pEnd := start.AddDate(0, i, 0).Add(-time.Nanosecond)
		periods = append(periods, FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    companyID,
			FiscalYearID: fyID,
			Number:       i,
			Name:         pStart.Format("January 2006"),
			StartDate:    pStart,
			EndDate:      pEnd,
			State:        PeriodOpen,
			Type:         FiscalPeriodTypeNormal,
		})
	}

	// Adjustment periods (13 & 14)
	periods = append(periods,
		FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    companyID,
			FiscalYearID: fyID,
			Number:       13,
			Name:         "Period 13",
			StartDate:    year.EndDate,
			EndDate:      year.EndDate,
			State:        PeriodOpen,
			Type:         FiscalPeriodTypeYearEndAdjustment,
		},
		FiscalPeriod{
			ID:           types.NewFiscalPeriodID(),
			CompanyID:    companyID,
			FiscalYearID: fyID,
			Number:       14,
			Name:         "Period 14",
			StartDate:    year.EndDate,
			EndDate:      year.EndDate,
			State:        PeriodOpen,
			Type:         FiscalPeriodTypeAuditAdjustment,
		},
	)

	return year, periods
}

// ValidatePeriodTransition verifies state transitions for fiscal periods.
func ValidatePeriodTransition(from, to PeriodState) error {
	switch {
	case from == PeriodOpen && to == PeriodSoftClosed:
	case from == PeriodSoftClosed && to == PeriodHardClosed:
	default:
		return ErrInvalidPeriodTransition
	}
	return nil
}

// ValidatePeriodReopen validates reopening an audited closed period.
func ValidatePeriodReopen(from, to PeriodState, actorID string, reason string) error {
	if to != PeriodOpen || (from != PeriodSoftClosed && from != PeriodHardClosed) {
		return ErrInvalidPeriodTransition
	}
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(reason) == "" {
		return ErrPeriodReopenRequiresAudit
	}
	return nil
}

// ValidatePeriodForPosting verifies that a fiscal period is open for posting on the given date.
func ValidatePeriodForPosting(ctx context.Context, reader Reader, companyID types.CompanyID, date time.Time) (FiscalPeriod, error) {
	period, err := reader.GetPeriodForDate(ctx, companyID, date)
	if err != nil {
		return FiscalPeriod{}, err
	}
	if period.State != PeriodOpen {
		return period, ErrClosedPeriod
	}
	return period, nil
}
