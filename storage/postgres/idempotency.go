package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

func (d *Driver) Claim(ctx context.Context, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	var result storage.IdempotencyRecord
	var claimed bool
	err := d.WithCompanyTx(ctx, uuid.UUID(record.CompanyID), func(tx pgx.Tx) error {
		var txErr error
		result, claimed, txErr = claimIdempotencyTx(ctx, tx, record)
		return txErr
	})
	return result, claimed, err
}

func (d *Driver) Lookup(ctx context.Context, companyID types.CompanyID, key string) (storage.IdempotencyRecord, bool, error) {
	var result storage.IdempotencyRecord
	var found bool
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var lookupErr error
		result, found, lookupErr = lookupIdempotencyTx(ctx, tx, companyID, key)
		return lookupErr
	})
	return result, found, err
}

func (d *Driver) Complete(ctx context.Context, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	return d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		return completeIdempotencyTx(ctx, tx, companyID, key, payloadHash, entryID, response)
	})
}

func (d *Driver) Fail(ctx context.Context, companyID types.CompanyID, key, payloadHash, status string) error {
	return d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		return failIdempotencyTx(ctx, tx, companyID, key, payloadHash, status)
	})
}

func claimIdempotencyTx(ctx context.Context, tx pgx.Tx, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	if record.Key == "" || record.Operation == "" {
		return storage.IdempotencyRecord{}, false, errors.New("postgres: idempotency key and operation required")
	}

	// The kernel-stamped scope is the database lock identity. Legacy callers
	// that do not supply a scope fall back to company+key, which was the
	// historical uniqueness key.
	// Kernel-stamped scopes have a unique (company_id, scope) key. An
	// INSERT ... ON CONFLICT claim is atomic on that key and removes two
	// round-trips (advisory lock pre-check) from the posting path. Legacy
	// callers without a scope keep the historical advisory-lock path.
	if record.Scope != "" {
		return claimScopedIdempotencyTx(ctx, tx, record)
	}
	lockIdentity := fmt.Sprintf("company=%s|key=%s", record.CompanyID.String(), record.Key)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockIdentity); err != nil {
		return storage.IdempotencyRecord{}, false, err
	}

	var existing storage.IdempotencyRecord
	var entryID *uuid.UUID
	var scanErr error
	if record.Scope == "" {
		scanErr = tx.QueryRow(ctx, `SELECT company_id, idempotency_key, operation, COALESCE(scope, ''), payload_hash,
			status, entry_id, response, created_at, updated_at
			FROM posting_idempotency WHERE company_id=$1 AND idempotency_key=$2
			ORDER BY created_at ASC LIMIT 1`,
			uuid.UUID(record.CompanyID), record.Key).Scan(
			&existing.CompanyID, &existing.Key, &existing.Operation, &existing.Scope, &existing.PayloadHash,
			&existing.Status, &entryID, &existing.Response, &existing.CreatedAt, &existing.UpdatedAt)
	} else {
		scanErr = tx.QueryRow(ctx, `SELECT company_id, idempotency_key, operation, COALESCE(scope, ''), payload_hash,
			status, entry_id, response, created_at, updated_at
			FROM posting_idempotency WHERE company_id=$1 AND scope=$2`,
			uuid.UUID(record.CompanyID), record.Scope).Scan(
			&existing.CompanyID, &existing.Key, &existing.Operation, &existing.Scope, &existing.PayloadHash,
			&existing.Status, &entryID, &existing.Response, &existing.CreatedAt, &existing.UpdatedAt)
	}

	switch {
	case scanErr == nil:
		if entryID != nil {
			eid := types.JournalEntryID(*entryID)
			existing.EntryID = &eid
		}
		if existing.PayloadHash != "" && record.PayloadHash != "" && existing.PayloadHash != record.PayloadHash {
			return storage.IdempotencyRecord{}, false, storage.ErrIdempotencyPayloadConflict
		}
		return existing, false, nil
	case !errors.Is(scanErr, pgx.ErrNoRows):
		return storage.IdempotencyRecord{}, false, scanErr
	}

	now := time.Now().UTC()
	record.Status = storage.IdempotencyStarted
	record.CreatedAt = now
	record.UpdatedAt = now
	_, err := tx.Exec(ctx, `INSERT INTO posting_idempotency(
		company_id, idempotency_key, operation, scope, payload_hash, status, created_at, updated_at
	) VALUES($1, $2, $3, $4, $5, $6, $7, $8)`,
		uuid.UUID(record.CompanyID), record.Key, record.Operation, record.Scope, record.PayloadHash,
		record.Status, now, now)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return storage.IdempotencyRecord{}, false, fmt.Errorf("%w: %v", storage.ErrConcurrentModification, err)
		}
		return storage.IdempotencyRecord{}, false, err
	}
	return record, true, nil
}

// claimScopedIdempotencyTx claims a kernel-scoped key with one atomic
// INSERT ... ON CONFLICT round-trip. A conflict means another caller owns the
// scope; the existing row is then read and validated exactly as in the legacy
// path. The (company_id, scope) primary key is the authoritative lock, so no
// additional advisory lock is needed.
func claimScopedIdempotencyTx(ctx context.Context, tx pgx.Tx, record storage.IdempotencyRecord) (storage.IdempotencyRecord, bool, error) {
	now := time.Now().UTC()
	record.Status = storage.IdempotencyStarted
	record.CreatedAt = now
	record.UpdatedAt = now

	var claimedScope string
	err := tx.QueryRow(ctx, `INSERT INTO posting_idempotency(
		company_id, idempotency_key, operation, scope, payload_hash, status, created_at, updated_at
	) VALUES($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (company_id, scope) DO NOTHING
	RETURNING scope`,
		uuid.UUID(record.CompanyID), record.Key, record.Operation, record.Scope, record.PayloadHash,
		record.Status, now, now).Scan(&claimedScope)
	if err == nil {
		return record, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return storage.IdempotencyRecord{}, false, err
	}

	var existing storage.IdempotencyRecord
	var entryID *uuid.UUID
	scanErr := tx.QueryRow(ctx, `SELECT company_id, idempotency_key, operation, COALESCE(scope, ''), payload_hash,
		status, entry_id, response, created_at, updated_at
		FROM posting_idempotency WHERE company_id=$1 AND scope=$2`,
		uuid.UUID(record.CompanyID), record.Scope).Scan(
		&existing.CompanyID, &existing.Key, &existing.Operation, &existing.Scope, &existing.PayloadHash,
		&existing.Status, &entryID, &existing.Response, &existing.CreatedAt, &existing.UpdatedAt)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		return storage.IdempotencyRecord{}, false, fmt.Errorf("%w: scoped idempotency claim conflicted without a visible record", storage.ErrConcurrentModification)
	}
	if scanErr != nil {
		return storage.IdempotencyRecord{}, false, scanErr
	}
	if entryID != nil {
		eid := types.JournalEntryID(*entryID)
		existing.EntryID = &eid
	}
	if existing.PayloadHash != "" && record.PayloadHash != "" && existing.PayloadHash != record.PayloadHash {
		return storage.IdempotencyRecord{}, false, storage.ErrIdempotencyPayloadConflict
	}
	return existing, false, nil
}

// lookupIdempotencyTx is side-effect free: it never claims a key. It is used
// after an unknown outcome to discover whether the operation became durable.
func lookupIdempotencyTx(ctx context.Context, tx pgx.Tx, companyID types.CompanyID, key string) (storage.IdempotencyRecord, bool, error) {
	var record storage.IdempotencyRecord
	var entryID *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT company_id, idempotency_key, operation, COALESCE(scope, ''), payload_hash,
		status, entry_id, response, created_at, updated_at
		FROM posting_idempotency WHERE company_id=$1 AND idempotency_key=$2
		ORDER BY created_at DESC LIMIT 1`,
		uuid.UUID(companyID), key).Scan(
		&record.CompanyID, &record.Key, &record.Operation, &record.Scope, &record.PayloadHash,
		&record.Status, &entryID, &record.Response, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return storage.IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return storage.IdempotencyRecord{}, false, err
	}
	if entryID != nil {
		eid := types.JournalEntryID(*entryID)
		record.EntryID = &eid
	}
	return record, true, nil
}

func completeIdempotencyTx(ctx context.Context, tx pgx.Tx, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	return completeIdempotencyRecordTx(ctx, tx, storage.IdempotencyRecord{
		CompanyID:   companyID,
		Key:         key,
		PayloadHash: payloadHash,
	}, entryID, response)
}

// completeIdempotencyRecordTx completes a claimed record using the full kernel
// scope when available. It is called inside the posting transaction so the
// durable result and the idempotency completion cannot diverge.
func completeIdempotencyRecordTx(ctx context.Context, tx pgx.Tx, record storage.IdempotencyRecord, entryID types.JournalEntryID, response []byte) error {
	now := time.Now().UTC()
	res, err := tx.Exec(ctx, `UPDATE posting_idempotency
		SET status=$1, entry_id=$2, response=$3, updated_at=$4
		WHERE company_id=$5 AND idempotency_key=$6
		  AND ($7='' OR scope=$7)
		  AND ($8='' OR payload_hash=$8)`,
		storage.IdempotencyCompleted, uuid.UUID(entryID), response, now,
		uuid.UUID(record.CompanyID), record.Key, record.Scope, record.PayloadHash)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("postgres: idempotency record not found for completion")
	}
	return nil
}

func failIdempotencyTx(ctx context.Context, tx pgx.Tx, companyID types.CompanyID, key, payloadHash, status string) error {
	now := time.Now().UTC()
	res, err := tx.Exec(ctx, `UPDATE posting_idempotency
		SET status=$1, updated_at=$2
		WHERE company_id=$3 AND idempotency_key=$4 AND ($5='' OR payload_hash=$5)`,
		status, now, uuid.UUID(companyID), key, payloadHash)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("postgres: idempotency record not found for failure")
	}
	return nil
}
