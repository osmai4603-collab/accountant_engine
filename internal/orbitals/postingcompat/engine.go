package posting

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	financialtypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
)

// Row is a minimal single-row reader exposed to attachments running against
// an SQL-backed transaction.
type Row interface {
	Scan(dest ...any) error
}

// AtomicTransaction represents the transaction handed to an orbital
// attachment. CustomMutation is the portable path used by the in-memory
// adapter; Exec/QueryRow are available to SQL-backed adapters.
type AtomicTransaction interface {
	Exec(ctx context.Context, sql string, args ...any) error
	QueryRow(ctx context.Context, sql string, args ...any) Row
	CustomMutation(ctx context.Context, target string, fn func(state any) error) error
}

// OpenLineMemoryStore is implemented by subledger stores that accept an open
// line created atomically with a journal entry.
type OpenLineMemoryStore interface {
	SaveOpenLineFromAttachment(ctx context.Context, attachment OpenLineAttachment, entry JournalEntry) error
}

// OpenLineAttachment carries the subledger line written with a posted entry.
type OpenLineAttachment struct {
	ID               uuid.UUID
	CompanyID        uuid.UUID
	JournalEntryID   uuid.UUID
	PartnerID        uuid.UUID
	ControlAccountID uuid.UUID
	Kind             string
	Amount           financialtypes.Money
	FunctionalAmount financialtypes.Money
	PostingDate      time.Time
	DocumentDate     time.Time
	DueDate          time.Time
	Reference        string
}

// LedgerAttachment lets an orbital ledger mutate operational state atomically
// with the journal entry (COS O IC-IF Principle 10 / single-writer pattern).
type LedgerAttachment interface {
	AttachmentKind() string
	ValidateAttachment(ctx context.Context, entry JournalEntry) error
	ExecuteAtomicMutation(ctx context.Context, tx AtomicTransaction, entry JournalEntry) error
}

// OpenLineLedgerAttachment adapts the legacy open-line attachment shape to
// LedgerAttachment.
type OpenLineLedgerAttachment struct {
	Attachment OpenLineAttachment
}

func (a *OpenLineLedgerAttachment) AttachmentKind() string { return "open_line" }

func (a *OpenLineLedgerAttachment) ValidateAttachment(ctx context.Context, entry JournalEntry) error {
	line := a.Attachment
	if entry.CompanyID == uuid.Nil || line.ID == uuid.Nil || line.CompanyID != entry.CompanyID ||
		line.JournalEntryID != entry.ID || line.PartnerID == uuid.Nil ||
		line.ControlAccountID == uuid.Nil {
		return ErrInvalidEntry
	}
	return nil
}

func (a *OpenLineLedgerAttachment) ExecuteAtomicMutation(
	ctx context.Context, tx AtomicTransaction, entry JournalEntry,
) error {
	return tx.CustomMutation(ctx, "memory", func(state any) error {
		store, ok := state.(OpenLineMemoryStore)
		if !ok {
			return ErrUnsupportedMutationBackend
		}
		return store.SaveOpenLineFromAttachment(ctx, a.Attachment, entry)
	})
}

// EngineStore is the minimal persistence surface required by the orbital
// posting facade. Adapters may implement it over any storage backend.
type EngineStore interface {
	SaveDraft(ctx context.Context, entry JournalEntry) error
	SavePosted(ctx context.Context, entry JournalEntry) error
	GetEntry(ctx context.Context, companyID, entryID uuid.UUID) (JournalEntry, error)
}

// memoryStore is the dependency-free default used by orbital unit tests.
type memoryStore struct {
	mu      sync.RWMutex
	entries map[uuid.UUID]JournalEntry
}

func newMemoryStore() *memoryStore { return &memoryStore{entries: make(map[uuid.UUID]JournalEntry)} }

func (s *memoryStore) SaveDraft(ctx context.Context, entry JournalEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.ID] = entry
	return nil
}

func (s *memoryStore) SavePosted(ctx context.Context, entry JournalEntry) error {
	return s.SaveDraft(ctx, entry)
}

func (s *memoryStore) GetEntry(ctx context.Context, companyID, entryID uuid.UUID) (JournalEntry, error) {
	if err := ctx.Err(); err != nil {
		return JournalEntry{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[entryID]
	if !ok || entry.CompanyID != companyID {
		return JournalEntry{}, ErrEntryNotFound
	}
	return entry, nil
}

// Engine is the orbital posting facade. It validates drafts against the
// double-entry invariant and applies attachments inside one atomic mutation.
type Engine struct {
	store EngineStore
	state any
}

// NewEngine creates an engine backed by an in-memory entry store.
func NewEngine() *Engine { return &Engine{store: newMemoryStore()} }

// NewEngineWithStore binds the facade to an adapter-provided store and state.
// The state value is handed to attachments via AtomicTransaction.CustomMutation.
func NewEngineWithStore(store EngineStore, state any) *Engine {
	if store == nil {
		store = newMemoryStore()
	}
	return &Engine{store: store, state: state}
}

type atomicTx struct {
	state any
}

func (tx atomicTx) Exec(context.Context, string, ...any) error {
	return errors.New("posting: Exec is unavailable in the orbital compatibility transaction")
}

func (tx atomicTx) QueryRow(context.Context, string, ...any) Row { return errorRow{} }

func (tx atomicTx) CustomMutation(_ context.Context, target string, fn func(state any) error) error {
	if target != "memory" {
		return ErrUnsupportedMutationBackend
	}
	return fn(tx.state)
}

type errorRow struct{}

func (errorRow) Scan(...any) error {
	return errors.New("posting: QueryRow is unavailable in the orbital compatibility transaction")
}

func (e *Engine) CreateDraftContext(ctx context.Context, entry JournalEntry) error {
	if e == nil || e.store == nil {
		return errors.New("posting: engine store is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if entry.Status == "" {
		entry.Status = EntryDraft
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.store.SaveDraft(ctx, entry)
}

func (e *Engine) PostEntryWithAttachmentContext(
	ctx context.Context, entry *JournalEntry, posterID string, attachments ...LedgerAttachment,
) error {
	if e == nil || e.store == nil || entry == nil {
		return ErrInvalidEntry
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.Status == EntryPosted || entry.Status == EntryReversed {
		return ErrInvalidEntryState
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if err := attachment.ValidateAttachment(ctx, *entry); err != nil {
			return err
		}
	}
	tx := atomicTx{state: e.state}
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if err := attachment.ExecuteAtomicMutation(ctx, tx, *entry); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	entry.Status = EntryPosted
	entry.PostedBy = posterID
	entry.PostedAt = &now
	if entry.PostingDate.IsZero() {
		entry.PostingDate = now
	}
	return e.store.SavePosted(ctx, *entry)
}

func (e *Engine) PostEntryWithOpenLineContext(
	ctx context.Context, entry *JournalEntry, line OpenLineAttachment, posterID string,
) error {
	return e.PostEntryWithAttachmentContext(ctx, entry, posterID, &OpenLineLedgerAttachment{Attachment: line})
}

// GetEntry reads an entry previously stored through this engine.
func (e *Engine) GetEntry(ctx context.Context, companyID, entryID uuid.UUID) (JournalEntry, error) {
	if e == nil || e.store == nil {
		return JournalEntry{}, ErrEntryNotFound
	}
	return e.store.GetEntry(ctx, companyID, entryID)
}

// ErrInvalidEntryState is returned when a terminal entry is mutated.
var ErrInvalidEntryState = errors.New("invalid journal entry state")
