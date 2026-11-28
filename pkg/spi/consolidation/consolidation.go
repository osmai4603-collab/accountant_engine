// Package consolidation defines the public SPI contract for group
// consolidation across multiple entities, currency translation, intercompany
// eliminations, and non-controlling interests.
package consolidation

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type AccountType string

const (
	AccountAsset     AccountType = "asset"
	AccountLiability AccountType = "liability"
	AccountEquity    AccountType = "equity"
	AccountRevenue   AccountType = "revenue"
	AccountExpense   AccountType = "expense"
)

type RateClass string

const (
	RateClosing    RateClass = "closing"
	RateAverage    RateClass = "average"
	RateHistorical RateClass = "historical"
)

type Entity struct {
	ID                 string
	FunctionalCurrency string
}

type GroupAccount struct {
	ID   string
	Name string
	Type AccountType
}

type AccountMapping struct {
	EntityID        string
	SourceAccountID string
	GroupAccountID  string
	RateClass       RateClass
	HistoricalDate  time.Time
}

type Ownership struct {
	ParentEntityID           string
	SubsidiaryEntityID       string
	Percentage               decimal.Decimal
	ControlBasis             string
	ControlEvidenceReference string
}

type TrialBalanceLine struct {
	AccountID string
	Balance   decimal.Decimal
}

type TrialBalance struct {
	EntityID  string
	Currency  string
	StartDate time.Time
	EndDate   time.Time
	Lines     []TrialBalanceLine
}

type ExchangeRate struct {
	FromCurrency string
	ToCurrency   string
	RateClass    RateClass
	Rate         decimal.Decimal
}

type ConsolidationRequest struct {
	GroupID         string
	ParentEntityID  string
	GroupCurrency   string
	StartDate       time.Time
	EndDate         time.Time
	Entities        []Entity
	GroupAccounts   []GroupAccount
	AccountMappings []AccountMapping
	Ownerships      []Ownership
	TrialBalances   []TrialBalance
	Rates           []ExchangeRate
}

type FinancialTotals struct {
	Assets      decimal.Decimal
	Liabilities decimal.Decimal
	Equity      decimal.Decimal
	Revenue     decimal.Decimal
	Expenses    decimal.Decimal
	NetIncome   decimal.Decimal
}

type ConsolidationResult struct {
	GroupCurrency   string
	Balances        map[string]decimal.Decimal
	Totals          FinancialTotals
	AdjustmentCount int
}

// Service exposes the public group consolidation capability.
type Service interface {
	Consolidate(ctx context.Context, request ConsolidationRequest) (ConsolidationResult, error)
}
