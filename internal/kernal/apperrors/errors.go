package apperrors

import (
	"errors"
	"fmt"
)

// ErrorCategory classifies kernel errors into the standard operational taxonomy
// so callers can distinguish terminal domain/validation failures from transient
// concurrency or infrastructure failures (Quality Standard §5.13.e).
type ErrorCategory string

const (
	CategoryDomain         ErrorCategory = "domain"
	CategoryValidation     ErrorCategory = "validation"
	CategoryConcurrency    ErrorCategory = "concurrency"
	CategoryPersistence    ErrorCategory = "persistence"
	CategoryInfrastructure ErrorCategory = "infrastructure"
)

type CodedError struct {
	Code      string
	Category  ErrorCategory
	Retryable bool
	Message   string
	Cause     error
}

func (e *CodedError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *CodedError) Unwrap() error { return e.Cause }

func New(code, message string, cause error) *CodedError {
	return &CodedError{
		Code:      code,
		Category:  CategoryDomain,
		Retryable: false,
		Message:   message,
		Cause:     cause,
	}
}

// NewCategorized creates a CodedError with an explicit ErrorCategory and retryability contract.
// Domain and validation errors are forced non-retryable.
func NewCategorized(code string, category ErrorCategory, retryable bool, message string, cause error) *CodedError {
	if category == CategoryDomain || category == CategoryValidation {
		retryable = false
	}
	return &CodedError{
		Code:      code,
		Category:  category,
		Retryable: retryable,
		Message:   message,
		Cause:     cause,
	}
}

var (
	ErrUnbalancedEntry = NewCategorized("ERR_UNBALANCED_ENTRY", CategoryDomain, false, "journal entry is unbalanced", nil)
	ErrPeriodClosed    = NewCategorized("ERR_PERIOD_CLOSED", CategoryDomain, false, "fiscal period is closed", nil)
)

func IsCode(err error, code string) bool {
	var coded *CodedError
	return errors.As(err, &coded) && coded.Code == code
}

// CategoryOf returns the ErrorCategory of err when wrapped as a CodedError,
// defaulting to CategoryDomain for unclassified kernel errors.
func CategoryOf(err error) ErrorCategory {
	var coded *CodedError
	if errors.As(err, &coded) && coded.Category != "" {
		return coded.Category
	}
	return CategoryDomain
}

// IsRetryable reports whether err is explicitly marked as safe to retry.
func IsRetryable(err error) bool {
	var coded *CodedError
	if errors.As(err, &coded) {
		if coded.Category == CategoryDomain || coded.Category == CategoryValidation {
			return false
		}
		return coded.Retryable
	}
	return false
}
