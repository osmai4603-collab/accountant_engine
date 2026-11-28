package types

import (
	"errors"
	"fmt"
	"strings"
)

// IdempotencyOperation is the kernel-level kind of an atomic operation. It is
// part of the uniqueness scope: a posting key and a reversal key with the same
// text are different operations (RULE-KERNEL-027).
type IdempotencyOperation string

const (
	IdempotencyPosting     IdempotencyOperation = "post"
	IdempotencyReversal    IdempotencyOperation = "reverse"
	IdempotencyRevaluation IdempotencyOperation = "revaluation"
)

func (operation IdempotencyOperation) Valid() bool {
	switch operation {
	case IdempotencyPosting, IdempotencyReversal, IdempotencyRevaluation:
		return true
	default:
		return false
	}
}

// ErrIdempotencyScopeRequired indicates that an idempotency scope is missing
// required company, operation, or key fields.
var ErrIdempotencyScopeRequired = errors.New("complete idempotency scope is required")

// IdempotencyScope is the complete uniqueness scope of one posting request.
// The host executor must treat the triple, not the bare key, as unique.
type IdempotencyScope struct {
	CompanyID CompanyID
	Operation IdempotencyOperation
	Key       string
}

func (scope IdempotencyScope) Validate() error {
	if scope.CompanyID == (CompanyID{}) ||
		!scope.Operation.Valid() ||
		strings.TrimSpace(scope.Key) == "" {
		return ErrIdempotencyScopeRequired
	}
	return nil
}

// String is the canonical, log-safe representation of the uniqueness scope.
func (scope IdempotencyScope) String() string {
	return fmt.Sprintf("company=%s|operation=%s|key=%s",
		scope.CompanyID.String(), scope.Operation, strings.TrimSpace(scope.Key))
}

