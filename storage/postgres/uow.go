package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

type postgresUnitOfWork struct {
	driver *Driver
	tx     pgx.Tx
}

var (
	_ storage.UnitOfWork        = (*postgresUnitOfWork)(nil)
	_ storage.DraftStore        = (*postgresUnitOfWork)(nil)
	_ storage.EntryReader       = (*postgresUnitOfWork)(nil)
	_ storage.PostingWriter     = (*postgresUnitOfWork)(nil)
	_ storage.SequenceAllocator = (*postgresUnitOfWork)(nil)
	_ storage.AuditWriter       = (*postgresUnitOfWork)(nil)
	_ storage.IdempotencyStore  = (*postgresUnitOfWork)(nil)
	_ storage.BalanceReader     = (*postgresUnitOfWork)(nil)
)

func (u *postgresUnitOfWork) Drafts() storage.DraftStore            { return u }
func (u *postgresUnitOfWork) Entries() storage.EntryReader          { return u }
func (u *postgresUnitOfWork) Posting() storage.PostingWriter        { return u }
func (u *postgresUnitOfWork) Sequences() storage.SequenceAllocator  { return u }
func (u *postgresUnitOfWork) Audit() storage.AuditWriter            { return u }
func (u *postgresUnitOfWork) Idempotency() storage.IdempotencyStore { return u }
func (u *postgresUnitOfWork) Balance() storage.BalanceReader        { return u }

func (u *postgresUnitOfWork) SaveDraft(ctx context.Context, entry journal.JournalEntry) error {
	entry.Status = journal.EntryDraft
	if err := saveEntryTx(ctx, u.tx, entry); err != nil {
		return translateTransactionError(err)
	}
	return nil
}

func (u *postgresUnitOfWork) GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error) {
	var data []byte
	err := u.tx.QueryRow(ctx, `SELECT entry FROM account_moves WHERE company_id=$1 AND id=$2`,
		uuid.UUID(companyID), uuid.UUID(entryID)).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return journal.JournalEntry{}, ErrEntryNotFound
	}
	if err != nil {
		return journal.JournalEntry{}, translateTransactionError(err)
	}
	var entry journal.JournalEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return journal.JournalEntry{}, err
	}
	return entry, nil
}

func (u *postgresUnitOfWork) NextSequence(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (int64, error) {
	var sequence int64
	err := u.tx.QueryRow(ctx, `UPDATE journals SET current_sequence=current_sequence+1
		WHERE company_id=$1 AND id=$2 RETURNING current_sequence`,
		uuid.UUID(companyID), uuid.UUID(journalID)).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("postgres: journal %s not found for sequence", journalID)
	}
	return sequence, translateTransactionError(err)
}

func (u *postgresUnitOfWork) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	entry := commit.Entry
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) {
		return journal.JournalEntry{}, ErrInvalidEntry
	}
	if commit.ActorID != "" {
		entry.PostedBy = commit.ActorID
	}
	if commit.Reason != "" {
		entry.ReversalReason = commit.Reason
	}
	if entry.SequenceNumber == 0 {
		seq, err := u.NextSequence(ctx, entry.CompanyID, entry.JournalID)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		entry.SequenceNumber = seq
	}

	if commit.Operation == storage.OperationReverse {
		if commit.OriginalEntryID == nil {
			return journal.JournalEntry{}, errors.New("postgres: reversal requires original entry id")
		}
		return u.commitReversal(ctx, entry, *commit.OriginalEntryID)
	}

	entry.Status = journal.EntryPosted
	now := time.Now().UTC()
	entry.PostingDate = now
	entry.PostedAt = &now

	if err := saveEntryTx(ctx, u.tx, entry); err != nil {
		return journal.JournalEntry{}, translateTransactionError(err)
	}

	// Append audit
	auditRec := storage.AuditRecord{
		CompanyID:     entry.CompanyID,
		EntityType:    "journal_entry",
		EntityID:      entry.ID.String(),
		Action:        string(commit.Operation),
		ActorID:       commit.ActorID,
		Reason:        commit.Reason,
		CorrelationID: commit.CorrelationID,
		Payload:       fmt.Appendf(nil, `{"entry_id":"%s","sequence":%d}`, entry.ID.String(), entry.SequenceNumber),
		OccurredAt:    now,
	}
	if err := u.AppendAudit(ctx, auditRec); err != nil {
		return journal.JournalEntry{}, translateTransactionError(err)
	}

	return entry, nil
}

func (u *postgresUnitOfWork) commitReversal(ctx context.Context, reversal journal.JournalEntry, originalID types.JournalEntryID) (journal.JournalEntry, error) {
	var originalData []byte
	if err := u.tx.QueryRow(ctx, `SELECT entry FROM account_moves WHERE company_id=$1 AND id=$2 FOR UPDATE`,
		uuid.UUID(reversal.CompanyID), uuid.UUID(originalID)).Scan(&originalData); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return journal.JournalEntry{}, ErrEntryNotFound
		}
		return journal.JournalEntry{}, err
	}
	var original journal.JournalEntry
	if err := json.Unmarshal(originalData, &original); err != nil {
		return journal.JournalEntry{}, err
	}
	if original.Status != journal.EntryPosted || original.ReversedByID != nil {
		return journal.JournalEntry{}, fmt.Errorf("postgres: original entry already reversed or not posted")
	}

	reversal.Classification = journal.EntryClassReversing
	reversal.ReversalOfID = &originalID
	now := time.Now().UTC()
	reversal.Status = journal.EntryPosted
	reversal.PostingDate = now
	reversal.PostedAt = &now

	// Save reversal
	if err := saveEntryTx(ctx, u.tx, reversal); err != nil {
		return journal.JournalEntry{}, err
	}

	// Mark original as reversed
	original.ReversedByID = &reversal.ID
	original.Status = journal.EntryReversed
	original.UpdatedAt = now
	if err := saveEntryTx(ctx, u.tx, original); err != nil {
		return journal.JournalEntry{}, err
	}

	// Audit log
	if err := u.AppendAudit(ctx, storage.AuditRecord{
		CompanyID:  reversal.CompanyID,
		EntityType: "journal_entry",
		EntityID:   reversal.ID.String(),
		Action:     "reverse",
		ActorID:    reversal.PostedBy,
		Reason:     reversal.ReversalReason,
		OccurredAt: now,
	}); err != nil {
		return journal.JournalEntry{}, err
	}

	return reversal, nil
}

func (u *postgresUnitOfWork) GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error) {
	var balance decimal.Decimal
	var err error
	if asOf.IsZero() {
		err = u.tx.QueryRow(ctx, `SELECT COALESCE(SUM(l.debit - l.credit), 0)
			FROM account_move_lines l
			JOIN account_moves m ON m.company_id = l.company_id AND m.id = l.move_id
			WHERE l.company_id = $1 AND l.account_id = $2 AND m.status = 'posted'
			  AND ($3 = '' OR l.currency_id = $3)`,
			uuid.UUID(companyID), uuid.UUID(accountID), currency).Scan(&balance)
	} else {
		err = u.tx.QueryRow(ctx, `SELECT COALESCE(SUM(l.debit - l.credit), 0)
			FROM account_move_lines l
			JOIN account_moves m ON m.company_id = l.company_id AND m.id = l.move_id
			WHERE l.company_id = $1 AND l.account_id = $2 AND m.status = 'posted'
			  AND m.entry_date <= $3
			  AND ($4 = '' OR l.currency_id = $4)`,
			uuid.UUID(companyID), uuid.UUID(accountID), asOf, currency).Scan(&balance)
	}
	return balance, translateTransactionError(err)
}

func (u *postgresUnitOfWork) AppendAudit(ctx context.Context, record storage.AuditRecord) error {
	return appendAuditLogTx(ctx, u.tx, record)
}

func (u *postgresUnitOfWork) Claim(ctx context.Context, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	return claimIdempotencyTx(ctx, u.tx, record)
}

func (u *postgresUnitOfWork) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	return completeIdempotencyTx(ctx, u.tx, companyID, key, payloadHash, entryID, response)
}

func (u *postgresUnitOfWork) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	return failIdempotencyTx(ctx, u.tx, companyID, key, payloadHash, status)
}
