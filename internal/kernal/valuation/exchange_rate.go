package valuation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	"github.com/shopspring/decimal"
)

var (
	ErrRateNotFound    = errors.New("exchange rate not found")
	ErrInvalidRate     = errors.New("invalid exchange rate")
	ErrInvalidCurrency = errors.New("invalid exchange-rate currency")
	ErrDuplicateRate   = errors.New("exchange rate already registered for date and type")
)

type RateType string

const (
	SpotRate       RateType = "SPOT"
	ClosingRate    RateType = "CLOSING"
	AverageRate    RateType = "AVERAGE"
	HistoricalRate RateType = "HISTORICAL"
)

type ExchangeRate struct {
	FromCurrency  string
	ToCurrency    string
	Rate          decimal.Decimal
	EffectiveDate time.Time
	Type          RateType
	Source        string
}

func (r ExchangeRate) Validate() error {
	from := strings.ToUpper(strings.TrimSpace(r.FromCurrency))
	to := strings.ToUpper(strings.TrimSpace(r.ToCurrency))
	fromCurrency, fromOK := types.LookupCurrency(from)
	if !fromOK || fromCurrency.IsSpecial() {
		return ErrInvalidCurrency
	}
	toCurrency, toOK := types.LookupCurrency(to)
	if !toOK || toCurrency.IsSpecial() {
		return ErrInvalidCurrency
	}
	if !r.Rate.IsPositive() || r.EffectiveDate.IsZero() {
		return ErrInvalidRate
	}
	if err := types.ValidateExchangeRatePrecision(r.Rate); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRate, err)
	}
	if !validRateType(r.Type) {
		return ErrInvalidRate
	}
	return nil
}

func validRateType(rateType RateType) bool {
	switch rateType {
	case SpotRate, ClosingRate, AverageRate, HistoricalRate:
		return true
	default:
		return false
	}
}

func validateRateQuery(from, to string, rateType RateType, asOf time.Time) (string, string, error) {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	fromCurrency, fromOK := types.LookupCurrency(from)
	toCurrency, toOK := types.LookupCurrency(to)
	if !fromOK || !toOK || fromCurrency.IsSpecial() || toCurrency.IsSpecial() {
		return "", "", ErrInvalidCurrency
	}
	if asOf.IsZero() || !validRateType(rateType) {
		return "", "", ErrInvalidRate
	}
	return from, to, nil
}

type RateRegistry struct {
	mu    sync.RWMutex
	rates map[string][]ExchangeRate
}

func NewRateRegistry() *RateRegistry {
	return &RateRegistry{
		rates: make(map[string][]ExchangeRate),
	}
}

func (r *RateRegistry) RegisterRate(rate ExchangeRate) error {
	if err := rate.Validate(); err != nil {
		return err
	}
	rate.FromCurrency = strings.ToUpper(strings.TrimSpace(rate.FromCurrency))
	rate.ToCurrency = strings.ToUpper(strings.TrimSpace(rate.ToCurrency))
	rate.EffectiveDate = rate.EffectiveDate.UTC().Truncate(24 * time.Hour)

	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.rateKey(rate.Type, rate.FromCurrency, rate.ToCurrency)
	list := r.rates[key]
	for _, current := range list {
		if current.EffectiveDate.Equal(rate.EffectiveDate) {
			return ErrDuplicateRate
		}
	}
	inverseKey := r.rateKey(rate.Type, rate.ToCurrency, rate.FromCurrency)
	if inverseList := r.rates[inverseKey]; len(inverseList) > 0 {
		for _, current := range inverseList {
			if current.EffectiveDate.Equal(rate.EffectiveDate) {
				return ErrDuplicateRate
			}
		}
	}
	list = append(list, rate)
	sort.Slice(list, func(i, j int) bool {
		return list[i].EffectiveDate.Before(list[j].EffectiveDate)
	})
	r.rates[key] = list
	return nil
}

func (r *RateRegistry) GetRate(ctx context.Context, rateType RateType, from, to string, asOf time.Time) (decimal.Decimal, ExchangeRate, bool, error) {
	from, to, err := validateRateQuery(from, to, rateType, asOf)
	if err != nil {
		return decimal.Zero, ExchangeRate{}, false, err
	}
	if from == to {
		return decimal.NewFromInt(1), ExchangeRate{
			FromCurrency:  from,
			ToCurrency:    to,
			Rate:          decimal.NewFromInt(1),
			EffectiveDate: asOf.UTC(),
			Type:          rateType,
		}, false, nil
	}

	asOf = asOf.UTC().Truncate(24 * time.Hour)

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Direct lookup
	key := r.rateKey(rateType, from, to)
	if list, ok := r.rates[key]; ok && len(list) > 0 {
		if rate, found := findEffectiveRate(list, asOf); found {
			return rate.Rate, rate, false, nil
		}
	}

	// Inverted lookup
	invKey := r.rateKey(rateType, to, from)
	if list, ok := r.rates[invKey]; ok && len(list) > 0 {
		if rate, found := findEffectiveRate(list, asOf); found {
			invRate := decimal.NewFromInt(1).DivRound(rate.Rate, types.InternalScale)
			return invRate, rate, true, nil
		}
	}

	return decimal.Zero, ExchangeRate{}, false, ErrRateNotFound
}

func (r *RateRegistry) rateKey(t RateType, from, to string) string {
	return string(t) + ":" + from + ":" + to
}

func findEffectiveRate(list []ExchangeRate, asOf time.Time) (ExchangeRate, bool) {
	var best ExchangeRate
	found := false
	for _, item := range list {
		if !item.EffectiveDate.After(asOf) {
			best = item
			found = true
		} else {
			break
		}
	}
	return best, found
}

// Convert converts an amount to a target currency using the given rate and banker's rounding.
func Convert(amount types.Money, toCurrency string, rate decimal.Decimal) (types.Money, error) {
	if !rate.IsPositive() {
		return types.Money{}, ErrInvalidRate
	}
	toCurrency = strings.ToUpper(strings.TrimSpace(toCurrency))
	if strings.ToUpper(strings.TrimSpace(amount.Currency())) == toCurrency {
		if !rate.Equal(decimal.NewFromInt(1)) {
			return types.Money{}, ErrInvalidRate
		}
	}
	convertedAmount := amount.Amount().Mul(rate)
	result, err := types.NewMoney(convertedAmount, toCurrency)
	if err != nil {
		return types.Money{}, err
	}
	return result.Rounded()
}
