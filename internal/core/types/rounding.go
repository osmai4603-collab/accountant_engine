package types

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrZeroDenominator is returned when attempting division by zero.
var ErrZeroDenominator = errors.New("cannot divide by zero")

// RoundBanker rounds a decimal number to the given scale using Banker's Rounding (Half To Even).
func RoundBanker(d decimal.Decimal, scale int32) decimal.Decimal {
	return d.RoundBank(scale)
}

// RoundMoney rounds a Money value to its currency's default minor unit scale using Banker's Rounding.
func RoundMoney(m Money) (Money, error) {
	return m.Rounded()
}

