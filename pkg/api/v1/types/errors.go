package types

import (
	"errors"

	kernalJournal "github.com/osmai4603-collab/accounting_engine/internal/kernal/journal"
	kernalMasterdata "github.com/osmai4603-collab/accounting_engine/internal/kernal/masterdata"
	kernalPosting "github.com/osmai4603-collab/accounting_engine/internal/kernal/posting"
	kernalTypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

var (
	ErrCurrencyMismatch          = kernalTypes.ErrCurrencyMismatch
	ErrInvalidCurrency           = kernalTypes.ErrInvalidCurrency
	ErrInvalidAmount             = kernalTypes.ErrInvalidAmount
	ErrAmountExceedsPrecision    = kernalTypes.ErrAmountExceedsPrecision
	ErrSubMinorUnitAmount        = kernalTypes.ErrSubMinorUnitAmount
	ErrNegativeAmountProhibited  = kernalTypes.ErrNegativeAmountProhibited
	ErrZeroAmountLine            = kernalPosting.ErrZeroAmountLine
	ErrUnbalancedMove            = kernalPosting.ErrUnbalancedMove
	ErrFiscalPeriodClosed        = kernalMasterdata.ErrFiscalPeriodClosed
	ErrFiscalPeriodNotFound      = kernalMasterdata.ErrFiscalPeriodNotFound
	ErrInvalidPeriodTransition   = kernalMasterdata.ErrInvalidPeriodTransition
	ErrCannotPostToParentAccount = kernalMasterdata.ErrCannotPostToParentAccount
	ErrAccountNotFound           = kernalMasterdata.ErrAccountNotFound
	ErrDuplicateAccountCode      = kernalMasterdata.ErrDuplicateAccountCode
	ErrMoveAlreadyPosted         = kernalJournal.ErrMoveAlreadyPosted
	ErrCannotModifyPostedMove    = kernalJournal.ErrCannotModifyPostedMove
	ErrMakerCheckerViolation     = errors.New("maker-checker violation")
	ErrCapabilityNotImplemented  = errors.New("capability not implemented")
)
