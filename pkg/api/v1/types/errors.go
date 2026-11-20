package types

import (
	"errors"

	kernalTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrCurrencyMismatch          = kernalTypes.ErrCurrencyMismatch
	ErrInvalidCurrency           = kernalTypes.ErrInvalidCurrency
	ErrInvalidAmount             = kernalTypes.ErrInvalidAmount
	ErrAmountExceedsPrecision    = kernalTypes.ErrAmountExceedsPrecision
	ErrSubMinorUnitAmount        = kernalTypes.ErrSubMinorUnitAmount
	ErrNegativeAmountProhibited  = kernalTypes.ErrNegativeAmountProhibited
	ErrZeroAmountLine            = kernalTypes.ErrZeroAmountLine
	ErrUnbalancedMove            = kernalTypes.ErrUnbalancedMove
	ErrFiscalPeriodClosed        = kernalTypes.ErrFiscalPeriodClosed
	ErrFiscalPeriodNotFound      = kernalTypes.ErrFiscalPeriodNotFound
	ErrInvalidPeriodTransition   = kernalTypes.ErrInvalidPeriodTransition
	ErrCannotPostToParentAccount = kernalTypes.ErrCannotPostToParentAccount
	ErrAccountNotFound           = kernalTypes.ErrAccountNotFound
	ErrDuplicateAccountCode      = kernalTypes.ErrDuplicateAccountCode
	ErrMoveAlreadyPosted         = kernalTypes.ErrMoveAlreadyPosted
	ErrCannotModifyPostedMove    = kernalTypes.ErrCannotModifyPostedMove
	ErrMakerCheckerViolation     = errors.New("maker-checker violation")
	ErrCapabilityNotImplemented  = errors.New("capability not implemented")
)
