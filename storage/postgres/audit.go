package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/spi/storage"
)

const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// AppendAudit records an immutable audit log entry.
func (d *Driver) AppendAudit(ctx context.Context, record storage.AuditRecord) error {
	return d.WithCompanyTx(ctx, uuid.UUID(record.CompanyID), func(tx pgx.Tx) error {
		return appendAuditLogTx(ctx, tx, record)
	})
}

func appendAuditLogTx(ctx context.Context, tx pgx.Tx, record storage.AuditRecord) error {
	// Lock audit chain for company
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, record.CompanyID.String()); err != nil {
		return err
	}

	var previousHash string
	err := tx.QueryRow(ctx, `SELECT current_log_hash FROM financial_audit_logs
		WHERE company_id=$1 ORDER BY id DESC LIMIT 1`, uuid.UUID(record.CompanyID)).Scan(&previousHash)
	if errors.Is(err, pgx.ErrNoRows) {
		previousHash = genesisHash
	} else if err != nil {
		return err
	}

	timestamp := record.OccurredAt
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	entityUUID := uuid.Nil
	if parsed, err := uuid.Parse(record.EntityID); err == nil {
		entityUUID = parsed
	}

	actorUUID := uuid.Nil
	if parsed, err := uuid.Parse(record.ActorID); err == nil {
		actorUUID = parsed
	}

	payload := record.Payload
	if len(payload) == 0 {
		payload = []byte("null")
	}

	// Compute SHA-256 current_log_hash
	h := sha256.New()
	h.Write([]byte(previousHash))
	h.Write([]byte(record.CompanyID.String()))
	h.Write([]byte(record.EntityType))
	h.Write([]byte(record.EntityID))
	h.Write([]byte(record.Action))
	h.Write([]byte(record.ActorID))
	h.Write(payload)
	currentHash := hex.EncodeToString(h.Sum(nil))

	_, err = tx.Exec(ctx, `INSERT INTO financial_audit_logs(
		company_id, entity_type, entity_id, action, actor_id,
		payload_after, timestamp, previous_log_hash, current_log_hash
	) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		uuid.UUID(record.CompanyID), record.EntityType, entityUUID, record.Action,
		actorUUID, json.RawMessage(payload), timestamp, previousHash, currentHash)
	return err
}
