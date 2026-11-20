package apperrors

import (
	"errors"
	"fmt"
)

type CodedError struct {
	Code    string
	Message string
	Cause   error
}

func (e *CodedError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *CodedError) Unwrap() error { return e.Cause }

func New(code, message string, cause error) *CodedError {
	return &CodedError{Code: code, Message: message, Cause: cause}
}

var (
	ErrUnbalancedEntry = New("ERR_UNBALANCED_ENTRY", "journal entry is unbalanced", nil)
	ErrPeriodClosed    = New("ERR_PERIOD_CLOSED", "fiscal period is closed", nil)
)

func IsCode(err error, code string) bool {
	var coded *CodedError
	return errors.As(err, &coded) && coded.Code == code
}
