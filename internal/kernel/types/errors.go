package types

import "errors"

var (
	ErrCurrencyMismatch         = errors.New("currency mismatch")
	ErrInvalidCurrency          = errors.New("invalid currency code")
	ErrInvalidAmount            = errors.New("invalid monetary amount")
	ErrAmountExceedsPrecision   = errors.New("monetary amount exceeds 28 digits")
	ErrSubMinorUnitAmount       = errors.New("amount has fractions beyond the currency minor unit")
	ErrNegativeAmountProhibited = errors.New("negative amount prohibited")
)
