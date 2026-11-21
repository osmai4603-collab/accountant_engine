package ifrs16

import (
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/shopspring/decimal"
)

var (
	zero     = decimal.Zero
	one      = decimal.NewFromInt(1)
	scale    = int32(types.FinancialScale)
	internal = int32(types.InternalScale)
)

// BuildAmortizationSchedule returns the complete liability/ROU roll-forward
// without mutating the contract. This is the shared schedule builder used by
// LeaseContract.AmortizationSchedule and Unit.AmortizationSchedule.
func BuildAmortizationSchedule(c LeaseContract) ([]SchedulePeriod, error) {
	if c.InitialLeaseLiability.Currency() == "" || len(c.Payments) == 0 {
		return nil, ErrInvalidContract
	}
	balance := c.InitialLeaseLiability
	rouBalance := c.InitialROUAsset
	accumulated := mustZero(c.Currency)
	lastDate := c.CommencementDate.UTC()
	depreciationPeriods := c.UsefulLifePeriods
	if depreciationPeriods == 0 || depreciationPeriods > len(c.Payments) {
		depreciationPeriods = len(c.Payments)
	}
	baseDepreciation := c.InitialROUAsset.Amount().DivRound(decimal.NewFromInt(int64(depreciationPeriods)), scale)
	result := make([]SchedulePeriod, 0, len(c.Payments))
	for i, payment := range c.Payments {
		growth, err := periodGrowthFactor(c.AnnualDiscountRate, payment.DueDate.Sub(lastDate))
		if err != nil {
			return nil, err
		}
		interestAmount := balance.Amount().Mul(growth.Sub(one)).Round(scale)
		interest, err := types.NewMoney(interestAmount, c.Currency)
		if err != nil {
			return nil, err
		}
		principal, err := payment.Amount.Subtract(interest)
		if err != nil {
			return nil, err
		}
		closing, err := balance.Subtract(principal)
		if err != nil {
			return nil, err
		}
		if closing.IsNegative() {
			return nil, ErrNegativeLiability
		}

		depreciation := mustZero(c.Currency)
		if i < depreciationPeriods {
			amount := baseDepreciation
			if i == depreciationPeriods-1 {
				amount = c.InitialROUAsset.Amount().Sub(accumulated.Amount())
			}
			depreciation, err = types.NewMoney(amount, c.Currency)
			if err != nil {
				return nil, err
			}
			rouBalance, err = rouBalance.Subtract(depreciation)
			if err != nil {
				return nil, err
			}
			accumulated, err = accumulated.Add(depreciation)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, SchedulePeriod{
			LeaseReference:          c.Reference,
			Period:                  i + 1,
			DueDate:                 payment.DueDate,
			Payment:                 payment.Amount,
			OpeningLiability:        balance,
			InterestExpense:         interest,
			PrincipalReduction:      principal,
			ClosingLiability:        closing,
			ROUDepreciation:         depreciation,
			AccumulatedDepreciation: accumulated,
			ClosingROUAsset:         rouBalance,
		})
		balance = closing
		lastDate = payment.DueDate
	}
	return result, nil
}

// PresentValue discounts the supplied payments on an Actual/365 basis from the
// start date. It is exported so lessor measurement and tests share one exact
// discounting algorithm.
func PresentValue(
	payments []LeasePayment, startDate time.Time, annualRate decimal.Decimal, currency string,
) (types.Money, error) {
	factor := one
	total := zero
	lastDate := startDate.UTC()
	for _, payment := range payments {
		growth, err := periodGrowthFactor(annualRate, payment.DueDate.Sub(lastDate))
		if err != nil {
			return types.Money{}, err
		}
		factor = factor.Mul(growth)
		if !factor.IsPositive() {
			return types.Money{}, ErrInvalidContract
		}
		total = total.Add(payment.Amount.Amount().DivRound(factor, internal))
		lastDate = payment.DueDate
	}
	return types.NewMoney(total.Round(scale), currency)
}

func periodGrowthFactor(annualRate decimal.Decimal, elapsed time.Duration) (decimal.Decimal, error) {
	if elapsed <= 0 || annualRate.IsNegative() {
		return decimal.Zero, ErrInvalidContract
	}
	yearNanos := decimal.NewFromInt(int64(365 * 24 * time.Hour))
	yearFraction := decimal.NewFromInt(int64(elapsed)).DivRound(yearNanos, internal)
	growth, err := one.Add(annualRate).PowWithPrecision(yearFraction, internal+8)
	if err != nil || !growth.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: discount factor", ErrInvalidContract)
	}
	return growth, nil
}

// BuildStraightLineSchedule returns the lessor operating lease income and asset
// depreciation roll-forward without mutating the contract.
func BuildStraightLineSchedule(c OperatingLeaseContract) ([]StraightLineLeasePeriod, error) {
	if c.TotalConsideration.Currency() == "" || len(c.Payments) == 0 {
		return nil, ErrInvalidContract
	}
	count := int64(len(c.Payments))
	baseIncome := c.TotalConsideration.Amount().DivRound(decimal.NewFromInt(count), scale)
	baseDepreciation := c.DepreciableAssetBase.Amount().DivRound(
		decimal.NewFromInt(int64(c.DepreciationPeriods)), scale)
	recognizedIncome := mustZero(c.Currency)
	depreciated := mustZero(c.Currency)
	cumulativeContra := mustZero(c.Currency)
	result := make([]StraightLineLeasePeriod, 0, len(c.Payments))
	for i, payment := range c.Payments {
		incomeAmount := baseIncome
		if int64(i) == count-1 {
			incomeAmount = c.TotalConsideration.Amount().Sub(recognizedIncome.Amount())
		}
		incomeMoney, err := types.NewMoney(incomeAmount, c.Currency)
		if err != nil {
			return nil, err
		}
		movement, err := incomeMoney.Subtract(payment.Amount)
		if err != nil {
			return nil, err
		}
		cumulativeContra, err = cumulativeContra.Add(movement)
		if err != nil {
			return nil, err
		}
		recognizedIncome, err = recognizedIncome.Add(incomeMoney)
		if err != nil {
			return nil, err
		}

		depreciation := mustZero(c.Currency)
		if i < c.DepreciationPeriods {
			amount := baseDepreciation
			if i == c.DepreciationPeriods-1 {
				amount = c.DepreciableAssetBase.Amount().Sub(depreciated.Amount())
			}
			depreciation, err = types.NewMoney(amount, c.Currency)
			if err != nil {
				return nil, err
			}
			depreciated, err = depreciated.Add(depreciation)
			if err != nil {
				return nil, err
			}
		}
		closing, err := c.DepreciableAssetBase.Subtract(depreciated)
		if err != nil {
			return nil, err
		}
		result = append(result, StraightLineLeasePeriod{
			LeaseReference:           c.Reference,
			Period:                   i + 1,
			DueDate:                  payment.DueDate,
			CashReceived:             payment.Amount,
			LeaseIncome:              incomeMoney,
			DeferredMovement:         movement,
			CumulativeDeferredIncome: cumulativeContra,
			AssetDepreciation:        depreciation,
			AccumulatedDepreciation:  depreciated,
			ClosingAssetCarrying:     closing,
		})
	}
	return result, nil
}

// BuildFinanceIncomeSchedule allocates finance income at the implicit rate and
// closes the net investment at the unguaranteed residual value.
func BuildFinanceIncomeSchedule(c FinanceLeaseContract) ([]FinanceLeasePeriod, error) {
	if c.NetInvestment.Currency() == "" || len(c.Payments) == 0 {
		return nil, ErrInvalidContract
	}
	balance := c.NetInvestment
	lastDate := c.CommencementDate.UTC()
	result := make([]FinanceLeasePeriod, 0, len(c.Payments))
	for i, payment := range c.Payments {
		growth, err := periodGrowthFactor(c.ImplicitRate, payment.DueDate.Sub(lastDate))
		if err != nil {
			return nil, err
		}
		incomeAmount := balance.Amount().Mul(growth.Sub(one)).Round(scale)
		if i == len(c.Payments)-1 {
			incomeAmount = payment.Amount.Amount().Add(c.UnguaranteedResidual.Amount()).Sub(balance.Amount())
		}
		income, err := types.NewMoney(incomeAmount, c.Currency)
		if err != nil {
			return nil, err
		}
		recovery, err := payment.Amount.Subtract(income)
		if err != nil {
			return nil, err
		}
		closing, err := balance.Subtract(recovery)
		if err != nil {
			return nil, err
		}
		if closing.IsNegative() || income.IsNegative() {
			return nil, ErrNegativeNetInvestment
		}
		result = append(result, FinanceLeasePeriod{
			LeaseReference:       c.Reference,
			Period:               i + 1,
			DueDate:              payment.DueDate,
			OpeningNetInvestment: balance,
			FinanceIncome:        income,
			CashReceived:         payment.Amount,
			PrincipalRecovery:    recovery,
			ClosingNetInvestment: closing,
		})
		balance = closing
		lastDate = payment.DueDate
	}
	return result, nil
}

// BuildExemptionExpenseSchedule returns the straight-line expense roll-forward
// for a short-term or low-value exemption lease.
func BuildExemptionExpenseSchedule(l ExemptibleLease) ([]ExemptionPeriod, error) {
	if l.TotalLeasePayments.Currency() == "" || len(l.Payments) == 0 {
		return nil, ErrInvalidExemption
	}
	count := int64(len(l.Payments))
	baseExpense := l.TotalLeasePayments.Amount().DivRound(decimal.NewFromInt(count), scale)
	recognized := mustZero(l.Currency)
	cumulativeAccrual := mustZero(l.Currency)
	result := make([]ExemptionPeriod, 0, len(l.Payments))
	for i, payment := range l.Payments {
		expenseAmount := baseExpense
		if int64(i) == count-1 {
			expenseAmount = l.TotalLeasePayments.Amount().Sub(recognized.Amount())
		}
		expense, err := types.NewMoney(expenseAmount, l.Currency)
		if err != nil {
			return nil, err
		}
		movement, err := expense.Subtract(payment.Amount)
		if err != nil {
			return nil, err
		}
		cumulativeAccrual, err = cumulativeAccrual.Add(movement)
		if err != nil {
			return nil, err
		}
		recognized, err = recognized.Add(expense)
		if err != nil {
			return nil, err
		}
		result = append(result, ExemptionPeriod{
			LeaseReference:    l.Reference,
			Period:            i + 1,
			DueDate:           payment.DueDate,
			CashPaid:          payment.Amount,
			LeaseExpense:      expense,
			AccrualMovement:   movement,
			CumulativeAccrual: cumulativeAccrual,
		})
	}
	return result, nil
}

func mustZero(currency string) types.Money {
	m, _ := types.Zero(currency)
	return m
}
