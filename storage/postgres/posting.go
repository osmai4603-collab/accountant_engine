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
)

var (
	ErrEntryNotFound = fmt.Errorf("postgres: %w", storage.ErrEntryNotFound)
	ErrInvalidEntry  = errors.New("postgres: invalid journal entry")
)

// SaveDraft saves a draft entry in a company transaction.
func (d *Driver) SaveDraft(ctx context.Context, entry journal.JournalEntry) error {
	return d.WithCompanyTx(ctx, uuid.UUID(entry.CompanyID), func(tx pgx.Tx) error {
		uow := &postgresUnitOfWork{driver: d, tx: tx}
		return uow.SaveDraft(ctx, entry)
	})
}

// CancelDraft transitions an existing unposted draft entry to cancelled in a company transaction.
func (d *Driver) CancelDraft(ctx context.Context, entry journal.JournalEntry) error {
	return d.WithCompanyTx(ctx, uuid.UUID(entry.CompanyID), func(tx pgx.Tx) error {
		uow := &postgresUnitOfWork{driver: d, tx: tx}
		return uow.CancelDraft(ctx, entry)
	})
}

// GetEntry returns a journal entry by ID.
func (d *Driver) GetEntry(ctx context.Context, companyID types.CompanyID, entryID types.JournalEntryID) (journal.JournalEntry, error) {
	var entry journal.JournalEntry
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		uow := &postgresUnitOfWork{driver: d, tx: tx}
		var getErr error
		entry, getErr = uow.GetEntry(ctx, companyID, entryID)
		return getErr
	})
	return entry, err
}

// NextSequence allocates the next sequence number for a journal.
func (d *Driver) NextSequence(ctx context.Context, companyID types.CompanyID, journalID types.JournalID) (int64, error) {
	var seq int64
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		uow := &postgresUnitOfWork{driver: d, tx: tx}
		var seqErr error
		seq, seqErr = uow.NextSequence(ctx, companyID, journalID)
		return seqErr
	})
	return seq, err
}

// CommitPosting commits a journal entry atomically. The driver owns the real
// transaction boundary: if the transaction commits and the response is then
// lost, FaultAfterCommit models the unknown-outcome window that idempotency
// replay resolves.
func (d *Driver) CommitPosting(ctx context.Context, commit storage.PostingCommit) (journal.JournalEntry, error) {
	var result journal.JournalEntry
	if commit.Idempotency != nil {
		if err := commit.Idempotency.Validate(); err != nil {
			return journal.JournalEntry{}, err
		}
	}
	if err := storage.InjectFault(ctx, storage.FaultBeforeIdempotencyClaim); err != nil {
		return journal.JournalEntry{}, err
	}
	companyCtx := storage.WithCompanyContext(ctx, commit.Entry.CompanyID)
	if scope, inTx := ctx.Value(transactionContextKey{}).(transactionScope); inTx {
		if scope.companyID != uuid.UUID(commit.Entry.CompanyID) {
			return journal.JournalEntry{}, fmt.Errorf("%w: commit company %s does not match transaction company %s", storage.ErrScopeViolation, commit.Entry.CompanyID, scope.companyID)
		}
		uow := &postgresUnitOfWork{driver: d, tx: scope.tx}
		var postErr error
		result, postErr = uow.CommitPosting(companyCtx, commit)
		if postErr != nil {
			return journal.JournalEntry{}, postErr
		}
	} else {
		err := d.ExecuteWithOptions(companyCtx, storage.TxOptions{}, func(txCtx context.Context, uow storage.UnitOfWork) error {
			var postErr error
			// The transaction-aware context must reach CommitPosting: ordered
			// advisory locks are acquired through the active transaction.
			result, postErr = uow.Posting().CommitPosting(txCtx, commit)
			return postErr
		})
		if err != nil {
			return journal.JournalEntry{}, err
		}
	}
	if err := storage.InjectFault(ctx, storage.FaultAfterCommit); err != nil {
		return journal.JournalEntry{}, err
	}
	return result, nil
}

// marshalEntryForStorage emits the public entry plus the two legacy link keys
// checked by the WORM trigger when a posted move is frozen by its reversal.
// The kernel fields remain authoritative; these aliases only satisfy the
// database's immutability proof.
func marshalEntryForStorage(entry journal.JournalEntry) ([]byte, error) {
	data, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	// The database enforces the original-side link through ReversedEntryID and
	// the reversed_move_id column. Keeping ReversedByID out of the stored JSON
	// lets the WORM trigger prove that no other posted fact changed.
	delete(object, "ReversedByID")
	if entry.Status == journal.EntryReversed && entry.ReversedByID != nil {
		alias, err := json.Marshal(entry.ReversedByID.String())
		if err != nil {
			return nil, err
		}
		object["ReversedEntryID"] = alias
	}
	if entry.ReversalOfID != nil {
		alias, err := json.Marshal(entry.ReversalOfID.String())
		if err != nil {
			return nil, err
		}
		object["OriginalEntryID"] = alias
	}
	return json.Marshal(object)
}

// unmarshalEntryFromStorage restores the kernel entry from its durable JSON,
// including the legacy reversal link aliases used by the WORM trigger.
func unmarshalEntryFromStorage(data []byte) (journal.JournalEntry, error) {
	var entry journal.JournalEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return journal.JournalEntry{}, err
	}
	var aliases struct {
		ReversedEntryID *string `json:"ReversedEntryID"`
		OriginalEntryID *string `json:"OriginalEntryID"`
	}
	if err := json.Unmarshal(data, &aliases); err != nil {
		return journal.JournalEntry{}, err
	}
	if entry.ReversedByID == nil && aliases.ReversedEntryID != nil {
		parsed, err := uuid.Parse(*aliases.ReversedEntryID)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		id := types.JournalEntryID(parsed)
		entry.ReversedByID = &id
	}
	if entry.ReversalOfID == nil && aliases.OriginalEntryID != nil {
		parsed, err := uuid.Parse(*aliases.OriginalEntryID)
		if err != nil {
			return journal.JournalEntry{}, err
		}
		id := types.JournalEntryID(parsed)
		entry.ReversalOfID = &id
	}
	return entry, nil
}

func saveEntryTx(ctx context.Context, tx pgx.Tx, entry journal.JournalEntry) error {
	if entry.ID == (types.JournalEntryID{}) || entry.CompanyID == (types.CompanyID{}) || entry.JournalID == (types.JournalID{}) {
		return ErrInvalidEntry
	}

	// Auto-lookup fiscal period if missing
	if entry.FiscalPeriodID == (types.FiscalPeriodID{}) {
		date := entry.EffectiveDate
		if date.IsZero() {
			date = time.Now().UTC()
		}
		var pid uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM fiscal_periods
			WHERE company_id=$1 AND start_date <= $2 AND end_date >= $2 AND period_type='normal'
			ORDER BY period_number ASC LIMIT 1`, uuid.UUID(entry.CompanyID), date).Scan(&pid)
		if err == nil {
			entry.FiscalPeriodID = types.FiscalPeriodID(pid)
		}
	}

	data, err := marshalEntryForStorage(entry)
	if err != nil {
		return err
	}

	classification := string(entry.Classification)
	if classification == "" {
		classification = "regular"
	}

	effectiveDate := entry.EffectiveDate
	if effectiveDate.IsZero() {
		effectiveDate = time.Now().UTC()
	}

	var postingDate any
	if !entry.PostingDate.IsZero() {
		postingDate = entry.PostingDate
	}

	var reversedMoveID *uuid.UUID
	if entry.ReversalOfID != nil {
		u := uuid.UUID(*entry.ReversalOfID)
		reversedMoveID = &u
	} else if entry.ReversedByID != nil {
		u := uuid.UUID(*entry.ReversedByID)
		reversedMoveID = &u
	}

	if entry.Status == journal.EntryPosted || entry.Status == journal.EntryReversed {
		// When committing or reversing, ensure the draft move exists first if not present
		var exists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_moves WHERE id=$1 AND company_id=$2)`,
			uuid.UUID(entry.ID), uuid.UUID(entry.CompanyID)).Scan(&exists)
		if !exists {
			draftEntry := entry
			draftEntry.Status = journal.EntryDraft
			draftData, err := json.Marshal(draftEntry)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO account_moves(
				id, company_id, journal_id, period_id, entry_date, posting_date,
				entry_number, status, classification, reversed_move_id, entry
			) VALUES($1, $2, $3, $4, $5, NULL, $6, 'draft', $7, $8, $9)`,
				uuid.UUID(entry.ID), uuid.UUID(entry.CompanyID), uuid.UUID(entry.JournalID),
				uuid.UUID(entry.FiscalPeriodID), effectiveDate, entry.SequenceNumber,
				classification, reversedMoveID, draftData); err != nil {
				return err
			}
			// Insert draft lines
			if err := insertMoveLines(ctx, tx, entry); err != nil {
				return err
			}
		}

		expectedStatus := "draft"
		if entry.Status == journal.EntryReversed {
			expectedStatus = "posted"
		}
		result, err := tx.Exec(ctx,
			`UPDATE account_moves SET journal_id=$3,period_id=$4,entry_date=$5,posting_date=$6,entry_number=$7,
			 status=$8,classification=$9,reversed_move_id=$10,entry=$11
			 WHERE id=$1 AND company_id=$2 AND status=$12`,
			uuid.UUID(entry.ID), uuid.UUID(entry.CompanyID), uuid.UUID(entry.JournalID), uuid.UUID(entry.FiscalPeriodID),
			effectiveDate, postingDate, entry.SequenceNumber, string(entry.Status), classification, reversedMoveID, data, expectedStatus)
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("postgres: entry %s cannot transition to %s (expected %s)", entry.ID, entry.Status, expectedStatus)
		}
		return nil
	}

	// Draft insertion/update
	result, err := tx.Exec(ctx, `INSERT INTO account_moves(
		id, company_id, journal_id, period_id, entry_date, posting_date,
		entry_number, status, classification, reversed_move_id, entry
	) VALUES($1, $2, $3, $4, $5, NULL, $6, 'draft', $7, $8, $9)
	ON CONFLICT(id) DO UPDATE SET
		journal_id=EXCLUDED.journal_id,
		period_id=EXCLUDED.period_id,
		entry_date=EXCLUDED.entry_date,
		posting_date=EXCLUDED.posting_date,
		entry_number=EXCLUDED.entry_number,
		status=EXCLUDED.status,
		classification=EXCLUDED.classification,
		reversed_move_id=EXCLUDED.reversed_move_id,
		entry=EXCLUDED.entry
	WHERE account_moves.status='draft'`,
		uuid.UUID(entry.ID), uuid.UUID(entry.CompanyID), uuid.UUID(entry.JournalID),
		uuid.UUID(entry.FiscalPeriodID), effectiveDate, entry.SequenceNumber,
		classification, reversedMoveID, data)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("postgres: draft entry update affected %d rows", result.RowsAffected())
	}

	return insertMoveLines(ctx, tx, entry)
}

func insertMoveLines(ctx context.Context, tx pgx.Tx, entry journal.JournalEntry) error {
	if _, err := tx.Exec(ctx, `DELETE FROM account_move_lines WHERE move_id=$1`, uuid.UUID(entry.ID)); err != nil {
		return err
	}

	for i, line := range entry.Lines {
		lineID := uuid.UUID(line.ID)
		if lineID == uuid.Nil {
			lineID = uuid.New()
			entry.Lines[i].ID = types.JournalEntryLineID(lineID)
		}
		lineData, err := json.Marshal(line)
		if err != nil {
			return err
		}

		fDebitMinor, err := moneyMinorUnits(line.FunctionalDebit)
		if err != nil {
			return err
		}
		fCreditMinor, err := moneyMinorUnits(line.FunctionalCredit)
		if err != nil {
			return err
		}
		txMinor, err := moneyMinorUnits(line.TransactionAmount)
		if err != nil {
			return err
		}
		if line.FunctionalCredit.Amount().IsPositive() {
			txMinor = -txMinor
		}

		_, err = tx.Exec(ctx, `INSERT INTO account_move_lines (
			move_id, company_id, line_id, account_id,
			debit, credit, amount_currency, currency_id, transaction_amount_minor,
			transaction_currency, exchange_rate, functional_debit_minor, functional_credit_minor, line_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			uuid.UUID(entry.ID), uuid.UUID(entry.CompanyID), lineID, uuid.UUID(line.AccountID),
			line.FunctionalDebit.Amount(), line.FunctionalCredit.Amount(), line.TransactionAmount.Amount(),
			line.TransactionCurrency, txMinor, line.TransactionCurrency, line.ExchangeRate,
			fDebitMinor, fCreditMinor, lineData)
		if err != nil {
			return err
		}
	}
	return nil
}

func moneyMinorUnits(m types.Money) (int64, error) {
	if m.Currency() == "" && m.Amount().IsZero() {
		return 0, nil
	}
	return m.MinorUnitsExact()
}
