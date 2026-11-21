package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

var (
	ErrEntryNotFound = errors.New("memory: entry not found")
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

	entry.Status = journal.EntryDraft
	eMap, ok := d.entries[entry.CompanyID]
	if !ok {
		eMap = make(map[types.JournalEntryID]journal.JournalEntry)
		d.entries[entry.CompanyID] = eMap
	}
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
	if !ok {
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

// PostingWriter implementation
func (d *Driver) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	if err := ctx.Err(); err != nil {
		return journal.JournalEntry{}, err
	}
	entry := commit.Entry
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) {
		return journal.JournalEntry{}, ErrInvalidEntry
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.readOnly {
		return journal.JournalEntry{}, ErrReadOnlyTransaction
	}

	eMap, ok := d.entries[entry.CompanyID]
	if !ok {
		eMap = make(map[types.JournalEntryID]journal.JournalEntry)
		d.entries[entry.CompanyID] = eMap
	}

	// If reversal operation
	if commit.Operation == storage.OperationReverse {
		if commit.OriginalEntryID == nil {
			return journal.JournalEntry{}, errors.New("memory: original entry id required for reversal")
		}
		orig, ok := eMap[*commit.OriginalEntryID]
		if !ok {
			return journal.JournalEntry{}, fmt.Errorf("memory: original entry %s not found", commit.OriginalEntryID.String())
		}
		if orig.Status != journal.EntryPosted {
			return journal.JournalEntry{}, fmt.Errorf("memory: original entry not in posted state")
		}
		// Link reversal
		orig.ReversedByID = &entry.ID
		orig.Status = journal.EntryReversed
		eMap[orig.ID] = orig

		entry.ReversalOfID = commit.OriginalEntryID
		entry.Classification = journal.EntryClassReversing
	}

	now := time.Now().UTC()
	entry.Status = journal.EntryPosted
	entry.PostingDate = now
	entry.PostedAt = &now
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

	// Record audit entry
	auditRec := storage.AuditRecord{
		CompanyID:     entry.CompanyID,
		EntityType:    "journal_entry",
		EntityID:      entry.ID.String(),
		Action:        string(commit.Operation),
		ActorID:       commit.ActorID,
		Reason:        commit.Reason,
		CorrelationID: commit.CorrelationID,
		Payload:       []byte(fmt.Sprintf(`{"entry_id":"%s","sequence":%d}`, entry.ID.String(), entry.SequenceNumber)),
		OccurredAt:    now,
	}
	d.appendAuditInternal(auditRec)

	eMap[entry.ID] = cloneEntry(entry)
	return entry, nil
}
