package masterdata

import "errors"

var (
	ErrFiscalPeriodClosed        = errors.New("fiscal period closed")
	ErrFiscalPeriodNotFound      = errors.New("fiscal period not found")
	ErrInvalidPeriodTransition   = errors.New("invalid fiscal period transition")
	ErrCannotPostToParentAccount = errors.New("cannot post to parent account")
	ErrAccountNotFound           = errors.New("account not found")
	ErrDuplicateAccountCode      = errors.New("duplicate account code")
)
