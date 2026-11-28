package storage

import (
	"context"
	"errors"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/masterdata"
	"github.com/shopspring/decimal"
)

// This file defines the stable persistence ports consumed by the engine's
// application layer. Adapters implement these interfaces; application code
// never depends on pgx, sql.Tx, table names, or adapter-specific types.
//
// Guarantees that every adapter must provide:
//   - CommitPosting is atomic: the entry header, lines, status, number
//     allocation, balance effects, audit record and idempotency record either
//     all persist or none do.
//   - GetEntry never returns data belonging to another company.
//   - SaveDraft rejects a company/period/account mismatch before writing.
//   - NextSequence is unique per company+journal and must be reserved inside
//     the same transaction as the posting that consumes it.

// TransactionRunner executes fn inside one adapter transaction. The context
// passed to fn carries the transaction and any company/actor scope; adapters
// must not expose connection handles to callers.
type TransactionRunner interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// IsolationLevel is an adapter-neutral isolation request. Adapters may
// strengthen but must not weaken it.
type IsolationLevel string

const (
	IsolationReadCommitted  IsolationLevel = "read_committed"
	IsolationRepeatableRead IsolationLevel = "repeatable_read"
	IsolationSerializable   IsolationLevel = "serializable"
)

// DraftStore persists draft journal entries.
type DraftStore interface {
	SaveDraft(ctx context.Context, entry journal.JournalEntry) error
}

// DraftCanceller persists a draft-to-cancelled lifecycle transition atomically.
type DraftCanceller interface {
	CancelDraft(ctx context.Context, entry journal.JournalEntry) error
}

// EntryReader reads journal entries scoped to a company.
type EntryReader interface {
	GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error)
}

// PostingOperation distinguishes a first posting from a reversal or a
// revaluation posting. It is part of the canonical idempotency scope.
type PostingOperation string

const (
	OperationPost        PostingOperation = "post"
	OperationReverse     PostingOperation = "reverse"
	OperationRevaluation PostingOperation = "revaluation"
)

// PostingIdempotency is the kernel-stamped uniqueness contract of a commit.
// Scope is the canonical company/operation/key triple, and PayloadHash is the
// canonical digest of the frozen semantic payload. The adapter must persist
// both atomically with the entry and enforce their uniqueness at the storage
// layer; the bare key alone is not a sufficient idempotency guarantee.
type PostingIdempotency struct {
	Key         string
	Scope       string
	PayloadHash string
}

func (p PostingIdempotency) Validate() error {
	if p.Key == "" || p.Scope == "" || p.PayloadHash == "" {
		return errors.New("storage: idempotency key, scope and payload hash required")
	}
	return nil
}

// PostingCommit is a fully validated entry prepared by the application layer.
// The adapter persists it; it must not re-implement accounting policy, balance
// validation, approval, or reversal construction.
type PostingCommit struct {
	Entry           journal.JournalEntry
	Operation       PostingOperation
	OriginalEntryID *types.JournalEntryID
	// ActorID and Reason are persisted with the operation for audit purposes.
	ActorID string
	Reason  string
	// CorrelationID links the operation to the caller's request trace.
	CorrelationID string
	// Idempotency is optional for legacy callers, but the kernel pipeline
	// always supplies it. When present it is part of the atomic unit.
	Idempotency *PostingIdempotency
}

// PostingWriter commits a validated posting atomically.
type PostingWriter interface {
	CommitPosting(ctx context.Context, commit PostingCommit) (journal.JournalEntry, error)
}

// SequenceAllocator reserves the next journal entry number.
type SequenceAllocator interface {
	NextSequence(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (int64, error)
}

// MasterDataReader exposes the minimum master data required to validate and
// post an entry. It is read-only by design; provisioning master data is a
// separate capability.
type MasterDataReader interface {
	GetCompany(ctx context.Context, companyID types.CompanyID) (masterdata.Company, error)
	GetAccount(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (masterdata.Account, error)
	GetAccountByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Account, error)
	GetJournalByCode(ctx context.Context, companyID types.CompanyID, code string) (masterdata.Journal, error)
	HasAccountChildren(ctx context.Context, companyID types.CompanyID, accountID types.AccountID) (bool, error)
	GetFiscalYear(ctx context.Context, companyID types.CompanyID, fiscalYearID types.FiscalYearID) (masterdata.FiscalYear, error)
	GetPeriodForDate(ctx context.Context, companyID types.CompanyID, date time.Time) (masterdata.FiscalPeriod, error)
}

// BalanceReader exposes account balances for validation, reporting and
// reconciliation. Implementations must apply the requested functional
// currency and as-of date deterministically.
type BalanceReader interface {
	GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error)
}

// TransactionBalanceReader is an optional capability port (LDG-003 / DEC-LDG-001)
// returning deterministic transaction-currency balances alongside functional
// carrying amounts without altering BalanceReader semantics.
type TransactionBalanceReader interface {
	GetTransactionBalances(ctx context.Context, filter ledger.ForeignCurrencyBalanceFilter) ([]ledger.ForeignCurrencyBalance, error)
}

// AuditRecord is a single immutable audit entry appended by the engine.
type AuditRecord struct {
	CompanyID     types.CompanyID
	EntityType    string
	EntityID      string
	Action        string
	ActorID       string
	Reason        string
	CorrelationID string
	Payload       []byte
	OccurredAt    time.Time
}

// AuditWriter appends immutable audit evidence. Implementations must reject
// update and delete operations at the storage layer.
type AuditWriter interface {
	AppendAudit(ctx context.Context, record AuditRecord) error
}

// IdempotencyRecord is the durable state of one idempotency key.
type IdempotencyRecord struct {
	CompanyID types.CompanyID
	Key       string
	Operation string
	// Scope is the canonical company/operation/key triple supplied by the
	// kernel. Older records may have an empty scope; adapters must still scope
	// by company and key.
	Scope       string
	PayloadHash string
	Status      string
	EntryID     *types.JournalEntryID
	Response    []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	IdempotencyStarted   = "started"
	IdempotencyCompleted = "completed"
	IdempotencyRetryable = "retryable_failure"
	IdempotencyTerminal  = "terminal_failure"
)

// IdempotencyStore reserves and completes idempotency keys atomically with the
// operation they guard.
type IdempotencyStore interface {
	Claim(ctx context.Context, record IdempotencyRecord) (existing IdempotencyRecord, claimed bool, err error)
	Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error
	Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error
}

// IdempotencyLookup is the safe, side-effect-free read required after an
// unknown outcome (timeout or lost response). It is an optional capability so
// existing fakes and adapters remain source compatible.
type IdempotencyLookup interface {
	Lookup(ctx context.Context, companyID types.CompanyID, key string) (IdempotencyRecord, bool, error)
}

// Ports is the explicit dependency set required to build the kernel engine.
// Optional fields are capabilities: nil means the capability is disabled, not
// silently emulated.
type Ports struct {
	Drafts      DraftStore
	Entries     EntryReader
	Posting     PostingWriter
	Sequences   SequenceAllocator
	MasterData  MasterDataReader
	Balance     BalanceReader
	Audit       AuditWriter
	Idempotency IdempotencyStore
	TxRunner    TransactionRunner
	UOWRunner   UnitOfWorkRunner
	// Events exposes the optional public event-ingestion stores. When empty,
	// event ingestion rejects caller-supplied idempotency keys instead of
	// falling back to process-local memory.
	Events  publicevents.Stores
	Private PrivateExtensions
}

// PrivateExtensions lets an adapter expose engine-specific optional
// capabilities without widening the stable contracts. The engine must not
// require any entry from it.
type PrivateExtensions map[string]any

// Validate reports every missing required port at once.
func (p Ports) Validate() error {
	var errs []error
	if p.Drafts == nil {
		errs = append(errs, errors.New("storage: draft store is required"))
	}
	if p.Entries == nil {
		errs = append(errs, errors.New("storage: entry reader is required"))
	}
	if p.Posting == nil {
		errs = append(errs, errors.New("storage: posting writer is required"))
	}
	if p.Sequences == nil {
		errs = append(errs, errors.New("storage: sequence allocator is required"))
	}
	if p.MasterData == nil {
		errs = append(errs, errors.New("storage: master data reader is required"))
	}
	return errors.Join(errs...)
}
