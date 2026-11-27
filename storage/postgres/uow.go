package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/ledger"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
	"github.com/shopspring/decimal"
)

type postgresUnitOfWork struct {
	driver *Driver
	tx     pgx.Tx
}

var (
	_ storage.UnitOfWork               = (*postgresUnitOfWork)(nil)
	_ storage.DraftStore               = (*postgresUnitOfWork)(nil)
	_ storage.EntryReader              = (*postgresUnitOfWork)(nil)
	_ storage.PostingWriter            = (*postgresUnitOfWork)(nil)
	_ storage.SequenceAllocator        = (*postgresUnitOfWork)(nil)
	_ storage.AuditWriter              = (*postgresUnitOfWork)(nil)
	_ storage.IdempotencyStore         = (*postgresUnitOfWork)(nil)
	_ storage.IdempotencyLookup        = (*postgresUnitOfWork)(nil)
	_ storage.BalanceReader            = (*postgresUnitOfWork)(nil)
	_ storage.TransactionBalanceReader = (*postgresUnitOfWork)(nil)
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

func (u *postgresUnitOfWork) CancelDraft(ctx context.Context, entry journal.JournalEntry) error {
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) {
		return ErrInvalidEntry
	}
	entry.Status = journal.EntryCancelled
	data, err := marshalEntryForStorage(entry)
	if err != nil {
		return err
	}
	var currentStatus string
	err = u.tx.QueryRow(ctx, `SELECT status FROM account_moves WHERE company_id=$1 AND id=$2 FOR UPDATE`,
		uuid.UUID(entry.CompanyID), uuid.UUID(entry.ID)).Scan(&currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return translateTransactionError(err)
	}
	if currentStatus != "draft" {
		return fmt.Errorf("%w: postgres: cannot cancel non-draft journal entry %s with status %s", types.ErrCannotModifyPostedMove, entry.ID, currentStatus)
	}
	result, err := u.tx.Exec(ctx, `UPDATE account_moves SET status='cancelled', entry=$3 WHERE company_id=$1 AND id=$2 AND status='draft'`,
		uuid.UUID(entry.CompanyID), uuid.UUID(entry.ID), data)
	if err != nil {
		return translateTransactionError(err)
	}
	if result.RowsAffected() != 1 {
		return storage.ErrConcurrentModification
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
	entry, err := unmarshalEntryFromStorage(data)
	if err != nil {
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

func (u *postgresUnitOfWork) acquireOrderedAdvisoryLock(ctx context.Context, target storage.LockTarget) (func(), error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	scope, ok := ctx.Value(transactionContextKey{}).(transactionScope)
	if !ok {
		return nil, errors.New("postgres: ordered lock requires an active transaction")
	}
	if scope.companyID != uuid.UUID(target.CompanyID) {
		return nil, fmt.Errorf("%w: lock company %s does not match transaction company %s", storage.ErrScopeViolation, target.CompanyID, scope.companyID)
	}
	// A non-zero salt keeps Phase 8 identities separate from the Phase 7
	// idempotency and audit advisory locks while staying stable across calls.
	const phase8Salt int64 = 0x5048415345
	if _, err := scope.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, target.Canonical(), phase8Salt); err != nil {
		return nil, translateTransactionError(err)
	}
	// Transaction-scoped advisory locks are released by commit/rollback. The
	// returned no-op still lets WithOrderedLocks notify the observer in reverse
	// acquisition order.
	return func() {}, nil
}

// CommitPosting acquires the canonical company-scoped lock set inside the
// transaction before any sequence, idempotency, entry or audit write. The
// kernel only supplies the order; this adapter performs the actual locking.
func (u *postgresUnitOfWork) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	if err := storage.ValidateIdempotencyScope(commit.Entry.CompanyID, commit.Operation, commit.Idempotency); err != nil {
		return journal.JournalEntry{}, err
	}
	var result journal.JournalEntry
	err := storage.WithOrderedLocks(ctx, storage.PostingLockTargets(commit), u.acquireOrderedAdvisoryLock, func() error {
		var err error
		result, err = u.commitPostingLocked(ctx, commit)
		return err
	})
	return result, err
}

func (u *postgresUnitOfWork) commitPostingLocked(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
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

	var idempotency storage.IdempotencyRecord
	if commit.Idempotency != nil {
		if err := commit.Idempotency.Validate(); err != nil {
			return journal.JournalEntry{}, err
		}
		idempotency = storage.IdempotencyRecord{
			CompanyID:   entry.CompanyID,
			Key:         commit.Idempotency.Key,
			Operation:   string(commit.Operation),
			Scope:       commit.Idempotency.Scope,
			PayloadHash: commit.Idempotency.PayloadHash,
		}
		existing, claimed, err := claimIdempotencyTx(ctx, u.tx, idempotency)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		if !claimed {
			if existing.Status == storage.IdempotencyCompleted && existing.EntryID != nil {
				replayed, getErr := u.GetEntry(ctx, entry.CompanyID, *existing.EntryID)
				if getErr != nil {
					return journal.JournalEntry{}, fmt.Errorf("%w: replay entry missing: %v", storage.ErrAtomicCommitFailed, getErr)
				}
				return replayed, nil
			}
			if existing.Status == storage.IdempotencyStarted {
				return journal.JournalEntry{}, storage.ErrIdempotencyInProgress
			}
			return journal.JournalEntry{}, fmt.Errorf("%w: idempotency key is %s", storage.ErrIdempotencyPayloadConflict, existing.Status)
		}
	}

	var committed journal.JournalEntry
	var err error
	if commit.Operation == storage.OperationReverse {
		if commit.OriginalEntryID == nil {
			return journal.JournalEntry{}, errors.New("postgres: reversal requires original entry id")
		}
		committed, err = u.commitReversal(ctx, entry, *commit.OriginalEntryID, commit)
	} else {
		entry.Status = journal.EntryPosted
		// The kernel stamps posting time from its injected clock. Preserve it;
		// only legacy direct callers without a stamp receive an adapter time.
		if entry.PostingDate.IsZero() {
			entry.PostingDate = time.Now().UTC()
		}
		if entry.PostedAt == nil {
			postedAt := entry.PostingDate
			entry.PostedAt = &postedAt
		}
		if err = saveEntryTx(ctx, u.tx, entry); err == nil {
			committed = entry
		}
	}
	if err != nil {
		return journal.JournalEntry{}, translateTransactionError(err)
	}

	if err := storage.InjectFault(ctx, storage.FaultBeforeAudit); err != nil {
		return journal.JournalEntry{}, err
	}
	auditRec := storage.AuditRecord{
		CompanyID:     committed.CompanyID,
		EntityType:    "journal_entry",
		EntityID:      committed.ID.String(),
		Action:        string(commit.Operation),
		ActorID:       commit.ActorID,
		Reason:        commit.Reason,
		CorrelationID: commit.CorrelationID,
		Payload:       fmt.Appendf(nil, `{"entry_id":"%s","sequence":%d}`, committed.ID.String(), committed.SequenceNumber),
		OccurredAt:    committed.PostingDate,
	}
	if err := u.AppendAudit(ctx, auditRec); err != nil {
		return journal.JournalEntry{}, translateTransactionError(err)
	}

	if err := storage.InjectFault(ctx, storage.FaultBeforeCommit); err != nil {
		return journal.JournalEntry{}, err
	}
	if commit.Idempotency != nil {
		if err := completeIdempotencyRecordTx(ctx, u.tx, idempotency, committed.ID, []byte(committed.ID.String())); err != nil {
			return journal.JournalEntry{}, err
		}
	}
	return committed, nil
}

func (u *postgresUnitOfWork) commitReversal(
	ctx context.Context,
	reversal journal.JournalEntry,
	originalID types.JournalEntryID,
	commit storage.PostingCommit,
) (journal.JournalEntry, error) {
	var originalData []byte
	if err := u.tx.QueryRow(ctx, `SELECT entry FROM account_moves WHERE company_id=$1 AND id=$2 FOR UPDATE`,
		uuid.UUID(reversal.CompanyID), uuid.UUID(originalID)).Scan(&originalData); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return journal.JournalEntry{}, ErrEntryNotFound
		}
		return journal.JournalEntry{}, err
	}
	original, err := unmarshalEntryFromStorage(originalData)
	if err != nil {
		return journal.JournalEntry{}, err
	}
	if original.Status != journal.EntryPosted || original.ReversedByID != nil {
		return journal.JournalEntry{}, storage.ErrEntryAlreadyReversed
	}

	reversal.Classification = journal.EntryClassReversing
	reversal.ReversalOfID = &originalID
	if commit.Reason != "" {
		reversal.ReversalReason = commit.Reason
	}
	if commit.ActorID != "" {
		reversal.PostedBy = commit.ActorID
	}
	if reversal.PostingDate.IsZero() {
		reversal.PostingDate = time.Now().UTC()
	}
	if reversal.PostedAt == nil {
		postedAt := reversal.PostingDate
		reversal.PostedAt = &postedAt
	}
	reversal.Status = journal.EntryPosted

	// The reversal entry, the original entry's frozen state and the audit
	// record are all staged in this same unit of work.
	if err := saveEntryTx(ctx, u.tx, reversal); err != nil {
		return journal.JournalEntry{}, err
	}
	original.ReversedByID = &reversal.ID
	original.Status = journal.EntryReversed
	original.UpdatedAt = reversal.PostingDate
	if err := saveEntryTx(ctx, u.tx, original); err != nil {
		return journal.JournalEntry{}, err
	}

	return reversal, nil
}

func (u *postgresUnitOfWork) GetAccountBalance(ctx context.Context, companyID types.CompanyID, accountID types.AccountID, asOf time.Time, currency string) (decimal.Decimal, error) {
	// An empty currency is resolved to the company's functional currency, the
	// same deterministic rule used by the memory adapter.
	if currency == "" {
		if err := u.tx.QueryRow(ctx, `SELECT functional_currency FROM companies WHERE id = $1`,
			uuid.UUID(companyID)).Scan(&currency); err != nil {
			return decimal.Zero, translateTransactionError(err)
		}
	}

	var asOfArg any
	if !asOf.IsZero() {
		asOfArg = asOf
	}
	var balance decimal.Decimal
	err := u.tx.QueryRow(ctx, `SELECT COALESCE(SUM(l.debit - l.credit), 0)
		FROM account_move_lines l
		JOIN account_moves m ON m.company_id = l.company_id AND m.id = l.move_id
		WHERE l.company_id = $1 AND l.account_id = $2 AND m.status = 'posted'
		  AND ($4::timestamptz IS NULL OR COALESCE(m.posting_date::timestamptz, m.entry_date) <= $4)
		  AND l.currency_id = $3`,
		uuid.UUID(companyID), uuid.UUID(accountID), currency, asOfArg).Scan(&balance)
	return balance, translateTransactionError(err)
}

func (u *postgresUnitOfWork) GetTransactionBalances(ctx context.Context, filter ledger.ForeignCurrencyBalanceFilter) ([]ledger.ForeignCurrencyBalance, error) {
	if filter.CompanyID == (types.CompanyID{}) {
		return nil, errors.New("postgres: company_id is required")
	}
	var asOfArg any
	if !filter.AsOf.IsZero() {
		asOfArg = filter.AsOf.UTC()
	}
	rows, err := u.tx.Query(ctx, `SELECT entry FROM account_moves
		WHERE company_id = $1 AND status IN ('posted', 'reversed')
		  AND ($2::timestamptz IS NULL OR COALESCE(posting_date::timestamptz, entry_date) <= $2)`,
		uuid.UUID(filter.CompanyID), asOfArg)
	if err != nil {
		return nil, translateTransactionError(err)
	}
	defer rows.Close()

	var entries []journal.JournalEntry
	for rows.Next() {
		var data []byte
		if scanErr := rows.Scan(&data); scanErr != nil {
			return nil, translateTransactionError(scanErr)
		}
		entry, unmarshalErr := unmarshalEntryFromStorage(data)
		if unmarshalErr != nil {
			return nil, unmarshalErr
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, translateTransactionError(err)
	}
	balances, err := ledger.RebuildForeignCurrencyBalances(entries, filter)
	if err != nil {
		return nil, err
	}
	return ledger.SortedForeignCurrencyBalances(balances), nil
}

func (u *postgresUnitOfWork) AppendAudit(ctx context.Context, record storage.AuditRecord) error {
	return appendAuditLogTx(ctx, u.tx, record)
}

func (u *postgresUnitOfWork) Claim(ctx context.Context, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	return claimIdempotencyTx(ctx, u.tx, record)
}

func (u *postgresUnitOfWork) Lookup(ctx context.Context, companyID types.CompanyID, key string) (storage.IdempotencyRecord, bool, error) {
	return lookupIdempotencyTx(ctx, u.tx, companyID, key)
}

func (u *postgresUnitOfWork) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	return completeIdempotencyTx(ctx, u.tx, companyID, key, payloadHash, entryID, response)
}

func (u *postgresUnitOfWork) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	return failIdempotencyTx(ctx, u.tx, companyID, key, payloadHash, status)
}
