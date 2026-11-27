package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

var (
	ErrEntryNotFound = fmt.Errorf("memory: %w", storage.ErrEntryNotFound)
	ErrInvalidEntry  = errors.New("memory: invalid journal entry")
)

// DraftStore implementation
func (d *Driver) SaveDraft(ctx context.Context, entry journal.JournalEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) || entry.JournalID == (types.JournalID{}) {
		return ErrInvalidEntry
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}

	eMap, ok := d.entries[entry.CompanyID]
	if !ok {
		eMap = make(map[types.JournalEntryID]journal.JournalEntry)
		d.entries[entry.CompanyID] = eMap
	}
	if existing, exists := eMap[entry.ID]; exists && existing.Status != journal.EntryDraft {
		return fmt.Errorf("memory: cannot modify non-draft journal entry %s with status %s", entry.ID, existing.Status)
	}

	entry.Status = journal.EntryDraft
	eMap[entry.ID] = cloneEntry(entry)
	d.setEntryLedger(ctx, entry.ID)
	return nil
}

// CancelDraft transitions an existing unposted draft to cancelled.
func (d *Driver) CancelDraft(ctx context.Context, entry journal.JournalEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) {
		return ErrInvalidEntry
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return ErrReadOnlyTransaction
	}

	eMap, ok := d.entries[entry.CompanyID]
	if !ok {
		return ErrEntryNotFound
	}
	existing, exists := eMap[entry.ID]
	if !exists {
		return ErrEntryNotFound
	}
	if existing.Status != journal.EntryDraft {
		return fmt.Errorf("%w: memory: cannot cancel non-draft journal entry %s with status %s", types.ErrCannotModifyPostedMove, entry.ID, existing.Status)
	}

	entry.Status = journal.EntryCancelled
	eMap[entry.ID] = cloneEntry(entry)
	return nil
}

// EntryReader implementation
func (d *Driver) GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error) {
	if err := ctx.Err(); err != nil {
		return journal.JournalEntry{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	eMap, ok := d.entries[companyID]
	if !ok {
		return journal.JournalEntry{}, ErrEntryNotFound
	}
	e, ok := eMap[entryID]
	if !ok || !d.entryVisibleInLedger(ctx, entryID) {
		return journal.JournalEntry{}, ErrEntryNotFound
	}
	return cloneEntry(e), nil
}

// SequenceAllocator implementation
func (d *Driver) NextSequence(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return 0, ErrReadOnlyTransaction
	}
	key := fmt.Sprintf("%s:%s", companyID.String(), journalID.String())
	d.journalSequences[key]++
	return d.journalSequences[key], nil
}

// postingSnapshot captures the complete state touched by one commit. It is the
// memory adapter's equivalent of a database transaction snapshot: any failure
// before the commit boundary restores it exactly, leaving no partial effect.
type postingSnapshot struct {
	entries             map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry
	balances            map[accountBalanceKey]decimal.Decimal
	auditRecords        []storage.AuditRecord
	auditLastHash       string
	governanceAuditLogs map[types.CompanyID][]publicgovernance.AuditLog
}

func (d *Driver) snapshotPostingState() postingSnapshot {
	snapshot := postingSnapshot{
		entries:             make(map[types.CompanyID]map[types.JournalEntryID]journal.JournalEntry, len(d.entries)),
		balances:            make(map[accountBalanceKey]decimal.Decimal, len(d.balances)),
		auditRecords:        append([]storage.AuditRecord(nil), d.auditRecords...),
		auditLastHash:       d.auditLastHash,
		governanceAuditLogs: make(map[types.CompanyID][]publicgovernance.AuditLog, len(d.governanceAuditLogs)),
	}
	for companyID, logs := range d.governanceAuditLogs {
		copied := make([]publicgovernance.AuditLog, len(logs))
		for i, log := range logs {
			copied[i] = cloneAuditLog(log)
		}
		snapshot.governanceAuditLogs[companyID] = copied
	}
	for companyID, companyEntries := range d.entries {
		copied := make(map[types.JournalEntryID]journal.JournalEntry, len(companyEntries))
		for entryID, entry := range companyEntries {
			copied[entryID] = cloneEntry(entry)
		}
		snapshot.entries[companyID] = copied
	}
	for key, balance := range d.balances {
		snapshot.balances[key] = balance
	}
	return snapshot
}

func (d *Driver) restorePostingState(snapshot postingSnapshot) {
	d.entries = snapshot.entries
	d.balances = snapshot.balances
	d.auditRecords = snapshot.auditRecords
	d.auditLastHash = snapshot.auditLastHash
	d.governanceAuditLogs = snapshot.governanceAuditLogs
}

// acquireNamedLock is the memory adapter's real lock primitive. It is
// context-aware so an adversarial ordering cannot block a caller forever.
func (d *Driver) acquireNamedLock(ctx context.Context, target storage.LockTarget) (func(), error) {
	raw, _ := d.postingLocks.LoadOrStore(target.Canonical(), make(chan struct{}, 1))
	gate := raw.(chan struct{})
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PostingWriter implementation. The entry, its balances, its audit effect and
// the idempotency record are committed as one in-process unit. Phase 8 adds
// canonical, company-scoped lock acquisition before that unit; the kernel only
// orders the keys and never acquires a lock itself.
func (d *Driver) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	if err := ctx.Err(); err != nil {
		return journal.JournalEntry{}, err
	}
	if err := storage.ValidateIdempotencyScope(commit.Entry.CompanyID, commit.Operation, commit.Idempotency); err != nil {
		return journal.JournalEntry{}, err
	}

	var result journal.JournalEntry
	err := storage.WithOrderedLocks(ctx, storage.PostingLockTargets(commit), d.acquireNamedLock, func() error {
		var err error
		result, err = d.commitPostingLocked(ctx, commit)
		return err
	})
	return result, err
}

func (d *Driver) commitPostingLocked(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	entry := commit.Entry
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) {
		return journal.JournalEntry{}, ErrInvalidEntry
	}
	if err := storage.InjectFault(ctx, storage.FaultBeforeIdempotencyClaim); err != nil {
		return journal.JournalEntry{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return journal.JournalEntry{}, ErrReadOnlyTransaction
	}

	var idempotencyRecordKey idempotencyKey
	if commit.Idempotency != nil {
		if err := commit.Idempotency.Validate(); err != nil {
			return journal.JournalEntry{}, err
		}
		idempotencyRecordKey = idempotencyKey{
			companyID: entry.CompanyID,
			key:       ledgerScopedIdempotencyKey(ctx, commit.Idempotency.Key),
			scope:     commit.Idempotency.Scope,
		}
		if existing, ok := d.idempotency[idempotencyRecordKey]; ok {
			if existing.PayloadHash != commit.Idempotency.PayloadHash {
				return journal.JournalEntry{}, storage.ErrIdempotencyPayloadConflict
			}
			if existing.Status == storage.IdempotencyCompleted && existing.EntryID != nil {
				stored, ok := d.entries[entry.CompanyID][*existing.EntryID]
				if !ok {
					return journal.JournalEntry{}, storage.ErrAtomicCommitFailed
				}
				return cloneEntry(stored), nil
			}
			return journal.JournalEntry{}, storage.ErrIdempotencyInProgress
		}
		now := time.Now().UTC()
		d.idempotency[idempotencyRecordKey] = storage.IdempotencyRecord{
			CompanyID:   entry.CompanyID,
			Key:         commit.Idempotency.Key,
			Operation:   string(commit.Operation),
			Scope:       commit.Idempotency.Scope,
			PayloadHash: commit.Idempotency.PayloadHash,
			Status:      storage.IdempotencyStarted,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	snapshot := d.snapshotPostingState()
	fail := func(err error) (journal.JournalEntry, error) {
		d.restorePostingState(snapshot)
		if commit.Idempotency != nil {
			delete(d.idempotency, idempotencyRecordKey)
		}
		return journal.JournalEntry{}, err
	}

	eMap, ok := d.entries[entry.CompanyID]
	if !ok {
		eMap = make(map[types.JournalEntryID]journal.JournalEntry)
		d.entries[entry.CompanyID] = eMap
	}

	// If reversal operation
	if commit.Operation == storage.OperationReverse {
		if commit.OriginalEntryID == nil {
			return fail(errors.New("memory: original entry id required for reversal"))
		}
		orig, ok := eMap[*commit.OriginalEntryID]
		if !ok {
			return fail(fmt.Errorf("memory: original entry %s not found", commit.OriginalEntryID.String()))
		}
		if orig.Status == journal.EntryReversed || orig.ReversedByID != nil {
			return fail(storage.ErrEntryAlreadyReversed)
		}
		if orig.Status != journal.EntryPosted {
			return fail(errors.New("memory: original entry not in posted state"))
		}
		// Link reversal and freeze the original in the same unit.
		orig.ReversedByID = &entry.ID
		orig.Status = journal.EntryReversed
		eMap[orig.ID] = orig

		entry.ReversalOfID = commit.OriginalEntryID
		entry.Classification = journal.EntryClassReversing
	} else if existing, exists := eMap[entry.ID]; exists && existing.Status != journal.EntryDraft {
		return fail(fmt.Errorf("memory: cannot modify non-draft journal entry %s with status %s", entry.ID, existing.Status))
	}

	// Kernel-stamped dates are authoritative. Only legacy callers that did not
	// stamp a posting time get an adapter-generated fallback.
	now := time.Now().UTC()
	if entry.PostingDate.IsZero() {
		entry.PostingDate = now
	}
	if entry.PostedAt == nil {
		stamped := entry.PostingDate
		entry.PostedAt = &stamped
	}
	entry.Status = journal.EntryPosted
	if commit.ActorID != "" {
		entry.PostedBy = commit.ActorID
	}

	// Update perpetual balances
	for _, line := range entry.Lines {
		cur := line.FunctionalDebit.Currency()
		if cur == "" {
			cur = line.FunctionalCredit.Currency()
		}
		if cur == "" {
			cur = line.TransactionCurrency
		}
		key := accountBalanceKey{
			companyID: entry.CompanyID,
			accountID: line.AccountID,
			currency:  cur,
		}
		current := d.balances[key]
		netEffect := line.FunctionalDebit.Amount().Sub(line.FunctionalCredit.Amount())
		d.balances[key] = current.Add(netEffect)
	}

	if err := storage.InjectFault(ctx, storage.FaultBeforeAudit); err != nil {
		return fail(err)
	}
	auditRec := storage.AuditRecord{
		CompanyID:     entry.CompanyID,
		EntityType:    "journal_entry",
		EntityID:      entry.ID.String(),
		Action:        string(commit.Operation),
		ActorID:       commit.ActorID,
		Reason:        commit.Reason,
		CorrelationID: commit.CorrelationID,
		Payload:       []byte(fmt.Sprintf(`{"entry_id":"%s","sequence":%d}`, entry.ID.String(), entry.SequenceNumber)),
		OccurredAt:    entry.PostingDate,
	}
	d.appendAuditInternal(auditRec)
	// The verifiable v2 chain is part of the same commit: a posting must never
	// leave a posted movement without hash-chained audit evidence, and a failed
	// audit append must roll back the whole posting (Phase 7 contract).
	if _, err := d.appendAuditLogAtomicLocked(publicgovernance.AuditLog{
		CompanyID:     auditRec.CompanyID,
		EntityType:    auditRec.EntityType,
		EntityID:      auditRec.EntityID,
		Action:        auditRec.Action,
		ActorID:       auditRec.ActorID,
		Reason:        auditRec.Reason,
		CorrelationID: auditRec.CorrelationID,
		Payload:       auditRec.Payload,
		Timestamp:     auditRec.OccurredAt,
	}); err != nil {
		return fail(err)
	}

	if err := storage.InjectFault(ctx, storage.FaultBeforeCommit); err != nil {
		return fail(err)
	}

	eMap[entry.ID] = cloneEntry(entry)
	if commit.Idempotency != nil {
		record := d.idempotency[idempotencyRecordKey]
		record.Status = storage.IdempotencyCompleted
		record.EntryID = &entry.ID
		record.Response = []byte(entry.ID.String())
		record.UpdatedAt = time.Now().UTC()
		d.idempotency[idempotencyRecordKey] = record
	}

	// After this point the unit is durable. An injected error models a lost
	// response: the call fails but a safe lookup by key returns the result.
	if err := storage.InjectFault(ctx, storage.FaultAfterCommit); err != nil {
		return journal.JournalEntry{}, err
	}
	return cloneEntry(entry), nil
}
