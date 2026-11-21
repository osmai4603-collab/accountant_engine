package masterdata

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrPeriodOutsideFiscalYear        = errors.New("period dates must be within the fiscal year")
	ErrInvalidFiscalYearTransition    = errors.New("invalid fiscal year transition")
	ErrInvalidPeriodTransition        = types.ErrInvalidPeriodTransition
	ErrPeriodTransitionNeedsAudit     = errors.New("fiscal period transition requires an actor and reason")
	ErrPeriodReopenRequiresAudit      = errors.New("reopening a fiscal period requires an actor and reason")
	ErrFiscalPeriodClosed             = types.ErrFiscalPeriodClosed
	ErrFiscalPeriodNotFound           = types.ErrFiscalPeriodNotFound
	ErrClosedFiscalYear               = errors.New("fiscal year is closed")
	ErrAdjustmentPeriodReaderRequired = errors.New("adjustment period reader is required")
	ErrInvalidAdjustmentPeriod        = errors.New("invalid adjustment period")
	ErrInvalidFiscalPeriod            = errors.New("fiscal period is incomplete or inconsistent")
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
	start = start.UTC()
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	fyID := types.NewFiscalYearID()
	year := FiscalYear{
		ID:        fyID,
		CompanyID: companyID,
		Name:      start.Format("2006"),
		StartDate: start,
		EndDate:   addFiscalMonths(start, 12).Add(-time.Nanosecond),
		State:     FiscalYearOpen,
	}

	var periods []FiscalPeriod
	for i := 1; i <= 12; i++ {
		pStart := addFiscalMonths(start, i-1)
		pEnd := addFiscalMonths(start, i).Add(-time.Nanosecond)
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

func addFiscalMonths(start time.Time, months int) time.Time {
	start = start.UTC()
	targetYear, targetMonth := start.Year(), int(start.Month())-1+months
	targetYear += targetMonth / 12
	targetMonth %= 12
	if targetMonth < 0 {
		targetYear--
		targetMonth += 12
	}
	month := time.Month(targetMonth + 1)
	lastDay := time.Date(targetYear, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := start.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(targetYear, month, day, 0, 0, 0, 0, time.UTC)
}

// ValidateFiscalYear validates that a fiscal year and its periods are contiguous, non-overlapping,
// correctly ordered, and fully contained within the fiscal year boundaries.
func ValidateFiscalYear(year FiscalYear, periods []FiscalPeriod) error {
	if year.ID == (types.FiscalYearID{}) || year.CompanyID == (types.CompanyID{}) ||
		year.StartDate.IsZero() || year.EndDate.IsZero() || year.EndDate.Before(year.StartDate) ||
		!validFiscalYearState(year.State) {
		return ErrInvalidFiscalPeriod
	}
	if len(periods) != 12 && len(periods) != 14 {
		return ErrInvalidFiscalPeriod
	}
	seenIDs := make(map[types.FiscalPeriodID]struct{}, len(periods))
	if !periods[0].StartDate.UTC().Equal(year.StartDate.UTC()) {
		return ErrInvalidFiscalPeriod
	}
	if !periods[11].EndDate.UTC().Equal(year.EndDate.UTC()) {
		return ErrInvalidFiscalPeriod
	}
	for i := 0; i < 12; i++ {
		p := periods[i]
		if !validFiscalPeriodIdentity(p, year, seenIDs) {
			return ErrInvalidFiscalPeriod
		}
		if p.Number != i+1 || p.Type != FiscalPeriodTypeNormal {
			return ErrInvalidFiscalPeriod
		}
		if !validPeriodState(p.State) {
			return ErrInvalidFiscalPeriod
		}
		if p.StartDate.IsZero() || p.EndDate.IsZero() || p.EndDate.Before(p.StartDate) {
			return ErrInvalidFiscalPeriod
		}
		if p.StartDate.UTC().Before(year.StartDate.UTC()) || p.EndDate.UTC().After(year.EndDate.UTC()) {
			return ErrInvalidFiscalPeriod
		}
		if i > 0 {
			expectedStart := periods[i-1].EndDate.UTC().Add(time.Nanosecond)
			if !p.StartDate.UTC().Equal(expectedStart) {
				return ErrInvalidFiscalPeriod
			}
		}
	}
	if len(periods) == 14 {
		p13 := periods[12]
		p14 := periods[13]
		if !validFiscalPeriodIdentity(p13, year, seenIDs) ||
			p13.Number != 13 ||
			p13.Type != FiscalPeriodTypeYearEndAdjustment ||
			!validPeriodState(p13.State) ||
			!p13.StartDate.UTC().Equal(year.EndDate.UTC()) || !p13.EndDate.UTC().Equal(year.EndDate.UTC()) {
			return ErrInvalidFiscalPeriod
		}
		if !validFiscalPeriodIdentity(p14, year, seenIDs) ||
			p14.Number != 14 ||
			p14.Type != FiscalPeriodTypeAuditAdjustment ||
			!validPeriodState(p14.State) ||
			!p14.StartDate.UTC().Equal(year.EndDate.UTC()) || !p14.EndDate.UTC().Equal(year.EndDate.UTC()) {
			return ErrInvalidFiscalPeriod
		}
	}
	return nil
}

func validFiscalPeriodIdentity(
	period FiscalPeriod,
	year FiscalYear,
	seenIDs map[types.FiscalPeriodID]struct{},
) bool {
	if period.ID == (types.FiscalPeriodID{}) || period.CompanyID != year.CompanyID || period.FiscalYearID != year.ID {
		return false
	}
	if _, exists := seenIDs[period.ID]; exists {
		return false
	}
	seenIDs[period.ID] = struct{}{}
	return true
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
	return ValidatePeriodForPostingWithSoftClose(ctx, reader, companyID, date, false)
}

func ValidatePeriodForPostingWithSoftClose(
	ctx context.Context,
	reader Reader,
	companyID types.CompanyID,
	date time.Time,
	allowSoftClosed bool,
) (FiscalPeriod, error) {
	if date.IsZero() {
		return FiscalPeriod{}, ErrPeriodOutsideFiscalYear
	}
	period, err := reader.GetPeriodForDate(ctx, companyID, date)
	if err != nil {
		return FiscalPeriod{}, err
	}
	if err := validateNormalPeriodData(period, companyID, date); err != nil {
		return period, err
	}
	year, err := reader.GetFiscalYear(ctx, companyID, period.FiscalYearID)
	if err != nil {
		return period, err
	}
	if err := validateFiscalYearData(year, companyID, period.FiscalYearID); err != nil {
		return period, err
	}
	if !dateWithinYear(date, year) || !rangeWithinYear(period.StartDate, period.EndDate, year) {
		return period, ErrInvalidFiscalPeriod
	}
	if !validFiscalYearState(year.State) {
		return period, ErrInvalidFiscalPeriod
	}
	if year.State != FiscalYearOpen && !(allowSoftClosed && year.State == FiscalYearSoftClosed) {
		return period, ErrClosedFiscalYear
	}
	if !validPeriodState(period.State) {
		return period, ErrInvalidFiscalPeriod
	}
	if period.State != PeriodOpen && !(allowSoftClosed && period.State == PeriodSoftClosed) {
		return period, ErrClosedPeriod
	}
	return period, nil
}

// ValidateAdjustmentPeriodForPosting resolves and validates an explicitly
// selected adjustment period (13 or 14) for the fiscal year containing date.
func ValidateAdjustmentPeriodForPosting(
	ctx context.Context,
	reader Reader,
	companyID types.CompanyID,
	date time.Time,
	periodNumber int,
) (FiscalPeriod, error) {
	return ValidateAdjustmentPeriodForPostingWithSoftClose(ctx, reader, companyID, date, periodNumber, false)
}

func ValidateAdjustmentPeriodForPostingWithSoftClose(
	ctx context.Context,
	reader Reader,
	companyID types.CompanyID,
	date time.Time,
	periodNumber int,
	allowSoftClosed bool,
) (FiscalPeriod, error) {
	if periodNumber != 13 && periodNumber != 14 {
		return FiscalPeriod{}, ErrInvalidAdjustmentPeriod
	}
	if date.IsZero() {
		return FiscalPeriod{}, ErrPeriodOutsideFiscalYear
	}
	regularPeriod, err := reader.GetPeriodForDate(ctx, companyID, date)
	if err != nil {
		return FiscalPeriod{}, err
	}
	if err := validateNormalPeriodData(regularPeriod, companyID, date); err != nil {
		return FiscalPeriod{}, ErrInvalidAdjustmentPeriod
	}
	year, err := reader.GetFiscalYear(ctx, companyID, regularPeriod.FiscalYearID)
	if err != nil {
		return FiscalPeriod{}, err
	}
	if err := validateFiscalYearData(year, companyID, regularPeriod.FiscalYearID); err != nil {
		return FiscalPeriod{}, err
	}
	if !dateWithinYear(date, year) || !rangeWithinYear(regularPeriod.StartDate, regularPeriod.EndDate, year) {
		return FiscalPeriod{}, ErrInvalidFiscalPeriod
	}
	if !validFiscalYearState(year.State) || !validPeriodState(regularPeriod.State) {
		return FiscalPeriod{}, ErrInvalidFiscalPeriod
	}
	if year.State != FiscalYearOpen && !(allowSoftClosed && year.State == FiscalYearSoftClosed) {
		return FiscalPeriod{}, ErrClosedFiscalYear
	}
	adjustmentReader, ok := reader.(AdjustmentPeriodReader)
	if !ok {
		return FiscalPeriod{}, ErrAdjustmentPeriodReaderRequired
	}
	period, err := adjustmentReader.GetPeriodByNumber(ctx, companyID, regularPeriod.FiscalYearID, periodNumber)
	if err != nil {
		return FiscalPeriod{}, err
	}
	expectedType := FiscalPeriodTypeYearEndAdjustment
	if periodNumber == 14 {
		expectedType = FiscalPeriodTypeAuditAdjustment
	}
	if period.ID == (types.FiscalPeriodID{}) || period.CompanyID != companyID ||
		period.FiscalYearID != regularPeriod.FiscalYearID || period.Number != periodNumber ||
		period.Type != expectedType || !validPeriodState(period.State) ||
		period.StartDate.IsZero() || period.EndDate.IsZero() ||
		period.EndDate.Before(period.StartDate) ||
		!period.StartDate.UTC().Equal(year.EndDate.UTC()) ||
		!period.EndDate.UTC().Equal(year.EndDate.UTC()) {
		return FiscalPeriod{}, ErrInvalidAdjustmentPeriod
	}
	if period.State != PeriodOpen && !(allowSoftClosed && period.State == PeriodSoftClosed) {
		return FiscalPeriod{}, ErrClosedPeriod
	}
	return period, nil
}

func validateNormalPeriodData(period FiscalPeriod, companyID types.CompanyID, date time.Time) error {
	if period.ID == (types.FiscalPeriodID{}) || period.CompanyID != companyID ||
		period.FiscalYearID == (types.FiscalYearID{}) || period.Number < 1 || period.Number > 12 ||
		period.Type != FiscalPeriodTypeNormal || period.StartDate.IsZero() || period.EndDate.IsZero() ||
		period.EndDate.Before(period.StartDate) || !validPeriodState(period.State) ||
		!period.Contains(date) {
		return ErrInvalidFiscalPeriod
	}
	return nil
}

func validateFiscalYearData(year FiscalYear, companyID types.CompanyID, yearID types.FiscalYearID) error {
	if year.ID != yearID || year.CompanyID != companyID || year.StartDate.IsZero() ||
		year.EndDate.IsZero() || year.EndDate.Before(year.StartDate) {
		return ErrInvalidFiscalPeriod
	}
	return nil
}

func dateWithinYear(date time.Time, year FiscalYear) bool {
	date = date.UTC()
	return !date.Before(year.StartDate.UTC()) && !date.After(year.EndDate.UTC())
}

func rangeWithinYear(start, end time.Time, year FiscalYear) bool {
	start, end = start.UTC(), end.UTC()
	return !start.Before(year.StartDate.UTC()) && !end.After(year.EndDate.UTC())
}

func validPeriodState(state PeriodState) bool {
	return state == PeriodOpen || state == PeriodSoftClosed || state == PeriodHardClosed
}

func validFiscalYearState(state FiscalYearState) bool {
	return state == FiscalYearOpen || state == FiscalYearSoftClosed || state == FiscalYearHardClosed
}
