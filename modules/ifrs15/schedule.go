package ifrs15

import (
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

// BuildRecognitionSchedule is the exported recognition-schedule builder. It
// accepts a validated contract and one of its performance obligations, and
// produces the same dated amounts as Contract.Schedule without duplicating the
// daily/monthly allocation algorithm.
func BuildRecognitionSchedule(contract Contract, obligation PerformanceObligation, frequency ScheduleFrequency) ([]RevenueScheduleEntry, error) {
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	if frequency != Daily && frequency != Monthly {
		return nil, ErrInvalidPeriod
	}
	found := false
	for _, candidate := range contract.Obligations {
		if candidate.ID == obligation.ID {
			obligation = candidate
			found = true
			break
		}
	}
	if !found {
		return nil, ErrObligationNotFound
	}
	amount := contract.AllocatedPrices[obligation.ID]
	if obligation.RecognitionPattern == PointInTime {
		return []RevenueScheduleEntry{{
			ContractID:              contract.ID,
			PerformanceObligationID: obligation.ID,
			RecognitionDate:         dateOnly(obligation.StartDate),
			AmountToRecognize:       amount,
			Status:                  SchedulePending,
		}}, nil
	}
	if frequency == Daily {
		return contract.dailySchedule(obligation, amount)
	}
	return contract.monthlySchedule(obligation, amount)
}

// BuildDailyRecognitionSchedule builds a daily over-time recognition schedule.
func BuildDailyRecognitionSchedule(contract Contract, obligation PerformanceObligation) ([]RevenueScheduleEntry, error) {
	return BuildRecognitionSchedule(contract, obligation, Daily)
}

// BuildMonthlyRecognitionSchedule builds a monthly over-time recognition schedule.
func BuildMonthlyRecognitionSchedule(contract Contract, obligation PerformanceObligation) ([]RevenueScheduleEntry, error) {
	return BuildRecognitionSchedule(contract, obligation, Monthly)
}

func (c Contract) dailySchedule(obligation PerformanceObligation, total types.Money) ([]RevenueScheduleEntry, error) {
	start, end := dateOnly(obligation.StartDate), dateOnly(obligation.EndDate)
	days := int(end.Sub(start).Hours()/24) + 1
	if days <= 0 {
		return nil, ErrInvalidPeriod
	}
	result := make([]RevenueScheduleEntry, 0, days)
	rounded, err := total.Divide(decimal.NewFromInt(int64(days)))
	if err != nil {
		return nil, err
	}
	rounded, err = rounded.Rounded()
	if err != nil {
		return nil, err
	}
	remaining := total
	for i := 0; i < days; i++ {
		amount := rounded
		if i == days-1 {
			amount = remaining
		} else {
			remaining, err = remaining.Subtract(amount)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, RevenueScheduleEntry{
			ContractID:              c.ID,
			PerformanceObligationID: obligation.ID,
			RecognitionDate:         start.AddDate(0, 0, i),
			AmountToRecognize:       amount,
			Status:                  SchedulePending,
		})
	}
	return result, nil
}

func (c Contract) monthlySchedule(obligation PerformanceObligation, total types.Money) ([]RevenueScheduleEntry, error) {
	start, end := dateOnly(obligation.StartDate), dateOnly(obligation.EndDate)
	totalDays := int(end.Sub(start).Hours()/24) + 1
	if totalDays <= 0 {
		return nil, ErrInvalidPeriod
	}
	entries := make([]RevenueScheduleEntry, 0)
	remaining := total
	for monthStart := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC); !monthStart.After(end); monthStart = monthStart.AddDate(0, 1, 0) {
		monthEnd := monthStart.AddDate(0, 1, -1)
		periodStart, periodEnd := monthStart, monthEnd
		if periodStart.Before(start) {
			periodStart = start
		}
		if periodEnd.After(end) {
			periodEnd = end
		}
		periodDays := int(periodEnd.Sub(periodStart).Hours()/24) + 1
		amount := remaining
		if periodEnd.Before(end) {
			var err error
			amount, err = total.Multiply(decimal.NewFromInt(int64(periodDays)))
			if err != nil {
				return nil, err
			}
			amount, err = amount.Divide(decimal.NewFromInt(int64(totalDays)))
			if err != nil {
				return nil, err
			}
			amount, err = amount.Rounded()
			if err != nil {
				return nil, err
			}
			remaining, err = remaining.Subtract(amount)
			if err != nil {
				return nil, err
			}
		}
		entries = append(entries, RevenueScheduleEntry{
			ContractID:              c.ID,
			PerformanceObligationID: obligation.ID,
			RecognitionDate:         periodEnd,
			AmountToRecognize:       amount,
			Status:                  SchedulePending,
		})
	}
	return entries, nil
}
