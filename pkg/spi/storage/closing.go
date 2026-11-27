package storage

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
)

// YearEndCloseRecord persists the linkage between a closed fiscal year and its
// generated closing/opening entries so audited reopen can reverse them atomically.
type YearEndCloseRecord struct {
	CompanyID         types.CompanyID
	FiscalYearID      types.FiscalYearID
	NextFiscalYearID  types.FiscalYearID
	ClosingEntryIDs   []types.JournalEntryID
	OpeningEntryID    types.JournalEntryID
	NetIncome         string
	Policy            string
	ClosedBy          string
	ClosedAt          time.Time
	Reason            string
	ReopenedBy        string
	ReopenedAt        *time.Time
	ReopenReason      string
	ApprovalReference string
}

// YearEndClosingStore is an optional storage capability required by
// application-layer YearEndClosingService to execute and reopen annual closes
// atomically within a unit of work.
type YearEndClosingStore interface {
	ListAccounts(ctx context.Context, companyID types.CompanyID) ([]masterdata.Account, error)
	ListFiscalYears(ctx context.Context, companyID types.CompanyID) ([]masterdata.FiscalYear, error)
	ListFiscalPeriods(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) ([]masterdata.FiscalPeriod, error)
	ListEntriesByDateRange(ctx context.Context, companyID types.CompanyID, start, end time.Time) ([]journal.JournalEntry, error)
	UpdateFiscalPeriodState(ctx context.Context, companyID types.CompanyID, periodID types.FiscalPeriodID, state masterdata.PeriodState, actorID, reason string) error
	UpdateFiscalYearState(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID, state masterdata.FiscalYearState, actorID, reason string) error
	SaveYearEndCloseRecord(ctx context.Context, record YearEndCloseRecord) error
	GetYearEndCloseRecord(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (YearEndCloseRecord, bool, error)
}

