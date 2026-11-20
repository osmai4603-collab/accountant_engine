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
	ErrClosedFiscalYear             = errors.New("fiscal year is closed")
)

func (p FiscalPeriod) Contains(date time.Time) bool {
	if p.Type == FiscalPeriodTypeYearEndAdjustment || p.Type == FiscalPeriodTypeAuditAdjustment {
		return false
	}
	date = date.UTC()
	return !date.Before(p.StartDate.UTC()) && !date.After(p.EndDate.UTC())
}

// GenerateFiscalYear generates a standard 12-month fiscal year plus 2 adjustment periods.
func GenerateFiscalYear(companyID types.CompanyID, start time.Time) (FiscalYear, []FiscalPeriod) {
	start = time.Date(start.UTC().Year(), start.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
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
		pEnd := pStart.AddDate(0, 1, 0).Add(-time.Nanosecond)
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

func ValidatePeriodReopenWithApproval(from, to PeriodState, actorID string, reason string, approvalID string) error {
	if err := ValidatePeriodReopen(from, to, actorID, reason); err != nil {
		return err
	}
	if strings.TrimSpace(approvalID) == "" {
		return ErrPeriodReopenRequiresApproval
	}
	return nil
}

func ValidateFiscalYearTransition(from, to FiscalYearState) error {
	switch {
	case from == FiscalYearOpen && to == FiscalYearSoftClosed:
	case from == FiscalYearSoftClosed && to == FiscalYearHardClosed:
	default:
		return ErrInvalidFiscalYearTransition
	}
	return nil
}

func ValidateFiscalYearReopen(from, to FiscalYearState, actorID string, reason string) error {
	if (from != FiscalYearHardClosed && from != FiscalYearSoftClosed) || (to != FiscalYearOpen && to != FiscalYearSoftClosed) {
		return ErrInvalidFiscalYearTransition
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
	if period.FiscalYearID != (types.FiscalYearID{}) {
		year, err := reader.GetFiscalYear(ctx, companyID, period.FiscalYearID)
		if err != nil {
			return period, err
		}
		if year.State != FiscalYearOpen {
			return period, ErrClosedFiscalYear
		}
	}
	if period.State != PeriodOpen {
		return period, ErrClosedPeriod
	}
	return period, nil
}
