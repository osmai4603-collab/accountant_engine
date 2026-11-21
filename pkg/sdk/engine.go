package sdk

import (
	"context"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/reporting"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicapplication "github.com/osmai4603-collab/accounting_engine/pkg/spi/application"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/standards"
	publicsubledger "github.com/osmai4603-collab/accounting_engine/pkg/spi/subledger"
)

// Engine is the primary entry point to the embeddable accounting engine.
// All capabilities are exposed through dedicated services without network protocols.
type Engine interface {
	Ledger() LedgerService
	Reporting() ReportingService
	Governance() GovernanceService
	Events() EventIngestionService
	Standards() StandardsService
	Scheduler() standards.SchedulerPort
	Close() error
	CloseWithContext(ctx context.Context) error
}

type StandardsService interface {
	Registry() *standards.Registry
	Diagnostics() standards.EngineDiagnostics
	HasCapability(cap standards.Capability) bool
	Scheduler() standards.SchedulerPort
}

type LedgerService interface {
	PostEntry(ctx context.Context, entry journal.JournalEntry) (journal.JournalEntry, error)
	GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error)
	ReverseEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID, reason string) (journal.JournalEntry, error)
}

type ReportingService interface {
	GetTrialBalance(ctx context.Context, companyID types.CompanyID, asOf time.Time, currency string) (reporting.TrialBalance, error)
	GetBalanceSheet(ctx context.Context, companyID types.CompanyID, asOf time.Time, currency string) (reporting.BalanceSheetReport, error)
	GetIncomeStatement(ctx context.Context, companyID types.CompanyID, start, end time.Time, currency string) (reporting.IncomeStatementReport, error)
	GetGeneralLedger(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, start, end time.Time) (reporting.GeneralLedgerReport, error)
	GetCashFlow(ctx context.Context, request reporting.CashFlowRequest) (reporting.CashFlowStatementReport, error)
	GetDisclosures(ctx context.Context, query reporting.DisclosureQuery) ([]reporting.StructuredDisclosure, error)
}

type GovernanceService interface {
	RecordAuditLog(ctx context.Context, eventID string, details map[string]any) error
	GetApprovalFingerprint(ctx context.Context, entryID types.JournalEntryID) (string, error)
}

type EventIngestionService interface {
	ProcessEvent(ctx context.Context, event events.AccountingEvent) (journal.JournalEntry, error)
}

// YearEndClosing retrieves the optional atomic year-end closing capability.
func YearEndClosing(engine Engine) (publicapplication.YearEndClosingService, error) {
	provider, ok := engine.(interface {
		YearEndClosing() publicapplication.YearEndClosingService
	})
	if !ok || provider.YearEndClosing() == nil {
		return nil, types.ErrCapabilityNotImplemented
	}
	return provider.YearEndClosing(), nil
}

// SubledgerARAP retrieves the optional AR/AP use-case service from an engine.
// It returns ErrCapabilityNotImplemented when the capability was not enabled.
func SubledgerARAP(engine Engine) (publicsubledger.ARAPService, error) {
	provider, ok := engine.(interface {
		SubledgerARAP() publicsubledger.ARAPService
	})
	if !ok || provider.SubledgerARAP() == nil {
		return nil, types.ErrCapabilityNotImplemented
	}
	return provider.SubledgerARAP(), nil
}

