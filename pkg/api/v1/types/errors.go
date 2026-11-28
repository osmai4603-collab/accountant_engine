package types

import (
	"errors"

	kernelJournal "github.com/osmai4603-collab/accounting_engine/internal/kernel/journal"
	kernelMasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernel/masterdata"
	kernelPosting "github.com/osmai4603-collab/accounting_engine/internal/kernel/posting"
	kernelTypes "github.com/osmai4603-collab/accounting_engine/internal/kernel/types"
)

var (
	ErrCurrencyMismatch          = kernelTypes.ErrCurrencyMismatch
	ErrInvalidCurrency           = kernelTypes.ErrInvalidCurrency
	ErrInvalidAmount             = kernelTypes.ErrInvalidAmount
	ErrAmountExceedsPrecision    = kernelTypes.ErrAmountExceedsPrecision
	ErrSubMinorUnitAmount        = kernelTypes.ErrSubMinorUnitAmount
	ErrNegativeAmountProhibited  = kernelTypes.ErrNegativeAmountProhibited
	ErrZeroAmountLine            = kernelPosting.ErrZeroAmountLine
	ErrUnbalancedMove            = kernelPosting.ErrUnbalancedMove
	ErrFiscalPeriodClosed        = kernelMasterdata.ErrFiscalPeriodClosed
	ErrFiscalPeriodNotFound      = kernelMasterdata.ErrFiscalPeriodNotFound
	ErrInvalidPeriodTransition   = kernelMasterdata.ErrInvalidPeriodTransition
	ErrCannotPostToParentAccount = kernelMasterdata.ErrCannotPostToParentAccount
	ErrAccountNotFound           = kernelMasterdata.ErrAccountNotFound
	ErrDuplicateAccountCode      = kernelMasterdata.ErrDuplicateAccountCode
	ErrMoveAlreadyPosted         = kernelJournal.ErrMoveAlreadyPosted
	ErrCannotModifyPostedMove    = kernelJournal.ErrCannotModifyPostedMove
	ErrMakerCheckerViolation     = errors.New("maker-checker violation")
	ErrCapabilityNotImplemented  = errors.New("capability not implemented")
)
