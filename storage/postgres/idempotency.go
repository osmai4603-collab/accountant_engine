package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	if record.Key == "" || record.Operation == "" || record.PayloadHash == "" {
		return storage.IdempotencyRecord{}, false, errors.New("postgres: idempotency key, operation, and payload hash required")
	}

	// Lock on (company, key)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`,
		record.CompanyID.String(), record.Key); err != nil {
		return storage.IdempotencyRecord{}, false, err
	}

	var existing storage.IdempotencyRecord
	var entryID *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT company_id, idempotency_key, operation, payload_hash, status, entry_id, response, created_at, updated_at
		FROM posting_idempotency WHERE company_id=$1 AND idempotency_key=$2`,
		uuid.UUID(record.CompanyID), record.Key).Scan(
		&existing.CompanyID, &existing.Key, &existing.Operation, &existing.PayloadHash,
		&existing.Status, &entryID, &existing.Response, &existing.CreatedAt, &existing.UpdatedAt)

	if err == nil {
		if entryID != nil {
			eid := types.JournalEntryID(*entryID)
			existing.EntryID = &eid
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return storage.IdempotencyRecord{}, false, err
	}

	now := time.Now().UTC()
	record.Status = storage.IdempotencyStarted
	record.CreatedAt = now
	record.UpdatedAt = now

	_, err = tx.Exec(ctx, `INSERT INTO posting_idempotency(
		company_id, idempotency_key, operation, payload_hash, status, created_at, updated_at
	) VALUES($1, $2, $3, $4, $5, $6, $7)`,
		uuid.UUID(record.CompanyID), record.Key, record.Operation, record.PayloadHash,
		record.Status, now, now)
	if err != nil {
		return storage.IdempotencyRecord{}, false, err
	}

	return record, true, nil
}

func completeIdempotencyTx(ctx context.Context, tx pgx.Tx, companyID types.CompanyID, key, payloadHash string, entryID types.JournalEntryID, response []byte) error {
	now := time.Now().UTC()
	res, err := tx.Exec(ctx, `UPDATE posting_idempotency
		SET status=$1, entry_id=$2, response=$3, updated_at=$4
		WHERE company_id=$5 AND idempotency_key=$6 AND payload_hash=$7`,
		storage.IdempotencyCompleted, uuid.UUID(entryID), response, now,
		uuid.UUID(companyID), key, payloadHash)
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
		WHERE company_id=$3 AND idempotency_key=$4 AND payload_hash=$5`,
		status, now, uuid.UUID(companyID), key, payloadHash)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("postgres: idempotency record not found for failure")
	}
	return nil
}

