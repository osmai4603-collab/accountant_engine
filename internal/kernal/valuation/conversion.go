package valuation

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

type RateProviderSPI interface {
	GetRate(ctx context.Context, rateType RateType, fromCurrency, toCurrency string, date time.Time) (decimal.Decimal, error)
}

func ConvertForDate(
	ctx context.Context,
	provider RateProviderSPI,
	amount types.Money,
	toCurrency string,
	date time.Time,
	rateType RateType,
) (types.Money, decimal.Decimal, error) {
	if provider == nil {
		return types.Money{}, decimal.Zero, ErrRateNotFound
	}
	from, to, err := validateRateQuery(amount.Currency(), toCurrency, rateType, date)
	if err != nil {
		return types.Money{}, decimal.Zero, err
	}
	rate, err := provider.GetRate(ctx, rateType, from, to, date)
	if err != nil {
		return types.Money{}, decimal.Zero, err
	}
	if !rate.IsPositive() {
		return types.Money{}, decimal.Zero, ErrInvalidRate
	}
	if from == to && !rate.Equal(decimal.NewFromInt(1)) {
		return types.Money{}, decimal.Zero, ErrInvalidRate
	}
	converted, err := types.NewMoney(amount.Amount().Mul(rate), to)
	if err != nil {
		return types.Money{}, decimal.Zero, err
	}
	converted, err = converted.Rounded()
	return converted, rate, err
}
