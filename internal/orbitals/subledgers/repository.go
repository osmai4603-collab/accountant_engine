package subledgers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/osmai4603-collab/accounting_engine/internal/orbitals/postingcompat"
)

var (
	ErrNotFound                         = errors.New("subledger record not found")
	ErrAlreadyExists                    = errors.New("subledger record already exists")
	ErrInvalidAmount                    = errors.New("invalid subledger amount")
	ErrUnmatchedLine                    = errors.New("subledger line is not open")
	ErrInvalidAsset                     = errors.New("invalid fixed asset")
	ErrInvalidEvent                     = errors.New("invalid economic event")
	ErrInvalidBankAccount               = errors.New("invalid bank account")
	ErrMappingNotFound                  = errors.New("economic event mapping not found")
	ErrCompanyMismatch                  = errors.New("subledger records belong to different companies")
	ErrAtomicMatchRequired              = errors.New("atomic open-line updates are required for reconciliation")
	ErrAtomicFixedAssetRequired         = errors.New("atomic fixed-asset and journal-draft storage is required")
	ErrFixedAssetConfirmationRequired   = errors.New("atomic fixed-asset posting confirmation is required")
	ErrConcurrentAssetUpdate            = errors.New("fixed asset changed during processing")
	ErrConcurrentMatch                  = errors.New("open line changed during reconciliation")
	ErrConcurrentBankItem               = errors.New("bank item changed during reconciliation")
	ErrInvalidOpenLine                  = errors.New("invalid open line")
	ErrCompanyScopedOpenLinesRequired   = errors.New("company-scoped open-line storage is required")
	ErrCompanyScopedBankItemsRequired   = errors.New("company-scoped bank-item storage is required")
	ErrAtomicBankReconciliationRequired = errors.New("atomic bank-item updates are required for reconciliation")
	ErrAmbiguousBankReconciliation      = errors.New("multiple ledger groups match the bank statement")
	ErrBankReconciliationTooComplex     = errors.New("bank reconciliation candidate set is too large")
	ErrAtomicEventRequired              = errors.New("atomic event and journal-draft storage is required")
	ErrEventResultMissing               = errors.New("stored economic event has no result entry")
	ErrEventNotReplayable               = errors.New("economic event is not eligible for replay")
	ErrCompanyScopedEventsRequired      = errors.New("company-scoped event storage is required")
	ErrEventRulesRequired               = errors.New("persistent event accounting rules are required")
	ErrWriteOffPending                  = errors.New("subledger open line already has a pending write-off")
	ErrNoPendingWriteOff                = errors.New("subledger open line has no pending write-off")
	ErrCompanyScopeRequired             = errors.New("company scope is required")
)

type Repository interface {
	EventRepository
	EventAccountingRuleRepository
	AtomicEconomicEventRepository
	CompanyEventRepository
	OpenLineRepository
	CompanyOpenLineRepository
	AtomicOpenLineRepository
	ConditionalOpenLineRepository
	ConditionalOpenLinesAndDraftRepository
	AtomicOpenLineWriteOffConfirmationRepository
	PostedJournalEntryReader
	AtomicPostedOpenLineRepository
	FixedAssetRepository
	AtomicFixedAssetDraftRepository
	AtomicFixedAssetOperationDraftRepository
	AtomicFixedAssetConfirmationRepository
	BankItemRepository
	CompanyBankItemRepository
	ConditionalBankItemRepository
	ConditionalBankItemsAndDraftRepository
	ControlAccountReader
}

type EventRepository interface {
	SaveEvent(context.Context, EconomicEvent) error
	GetEventByIdempotencyKey(context.Context, string) (EconomicEvent, bool, error)
}

type OpenLineRepository interface {
	SaveOpenLine(context.Context, OpenLine) error
	GetOpenLine(context.Context, uuid.UUID, uuid.UUID) (OpenLine, error)
	ListOpenLines(context.Context, string) ([]OpenLine, error)
}

type FixedAssetRepository interface {
	SaveFixedAsset(context.Context, FixedAsset) error
	GetFixedAsset(context.Context, uuid.UUID, uuid.UUID) (FixedAsset, error)
	ListFixedAssets(context.Context) ([]FixedAsset, error)
}

type BankItemRepository interface {
	SaveBankItem(context.Context, BankItem) error
	ListBankItems(context.Context, string) ([]BankItem, error)
}

type CompanyEventRepository interface {
	GetEventByIdempotencyKeyForCompany(context.Context, uuid.UUID, string) (EconomicEvent, bool, error)
}

type EventLifecycleRepository interface {
	GetEventByIdentity(context.Context, uuid.UUID, string, string) (EconomicEvent, bool, error)
	GetEventByID(context.Context, uuid.UUID, uuid.UUID) (EconomicEvent, error)
	UpdateEventStatus(context.Context, EconomicEvent) error
}

type EventAccountingRuleRepository interface {
	SaveEventAccountingRule(context.Context, EventAccountingRule) error
	ListEventAccountingRulesForCompany(context.Context, uuid.UUID, string) ([]EventAccountingRule, error)
}

type CompanyOpenLineRepository interface {
	ListOpenLinesForCompanyContext(context.Context, uuid.UUID, string) ([]OpenLine, error)
}

type CompanyBankItemRepository interface {
	ListBankItemsForCompany(context.Context, uuid.UUID, string) ([]BankItem, error)
}

type ConditionalBankItemRepository interface {
	CompareAndSaveBankItems(context.Context, []BankItem, []BankItem) error
}

type AtomicOpenLineRepository interface {
	SaveOpenLines(context.Context, []OpenLine) error
}

type ConditionalOpenLineRepository interface {
	CompareAndSaveOpenLines(context.Context, []OpenLine, []OpenLine) error
}

type ConditionalOpenLinesAndDraftRepository interface {
	CompareAndSaveOpenLinesAndDraft(context.Context, []OpenLine, []OpenLine, posting.JournalEntry) error
}

type AtomicOpenLineWriteOffConfirmationRepository interface {
	ConfirmOpenLineWriteOff(context.Context, OpenLine, OpenLine, posting.JournalEntry) error
}

type ConditionalBankItemsAndDraftRepository interface {
	CompareAndSaveBankItemsAndDraft(context.Context, []BankItem, []BankItem, posting.JournalEntry) error
}

type AtomicEconomicEventRepository interface {
	SaveEventAndDraft(context.Context, EconomicEvent, posting.JournalEntry) error
}

type AtomicFixedAssetDraftRepository interface {
	SaveFixedAssetDraft(context.Context, FixedAsset, posting.JournalEntry) error
}

type AtomicFixedAssetOperationDraftRepository interface {
	SaveFixedAssetOperationDraft(context.Context, FixedAsset, FixedAsset, posting.JournalEntry) error
}

type AtomicFixedAssetConfirmationRepository interface {
	ConfirmFixedAssetMutation(context.Context, FixedAsset, FixedAsset, posting.JournalEntry) error
}
