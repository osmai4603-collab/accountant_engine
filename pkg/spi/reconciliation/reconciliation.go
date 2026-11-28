// Package reconciliation defines the public SPI contract for source-to-source
// and bank-to-ledger reconciliation.
package reconciliation

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type Record struct {
	Key         string
	Reference   string
	Amount      decimal.Decimal
	Currency    string
	Date        time.Time
	Description string
}

type ReconciliationRequest struct {
	LeftSourceName  string
	RightSourceName string
	Currency        string
	StartDate       time.Time
	EndDate         time.Time
	Tolerance       decimal.Decimal
	LeftRecords     []Record
	RightRecords    []Record
}

type Match struct {
	Key         string
	LeftAmount  decimal.Decimal
	RightAmount decimal.Decimal
	Difference  decimal.Decimal
}

type Exception struct {
	Side   string // "left_only", "right_only", "amount_mismatch"
	Key    string
	Amount decimal.Decimal
	Reason string
}

type ReconciliationResult struct {
	Matched    []Match
	Exceptions []Exception
	Balanced   bool
}

// Service exposes the public reconciliation capability.
type Service interface {
	ReconcileRecords(ctx context.Context, request ReconciliationRequest) (ReconciliationResult, error)
}
