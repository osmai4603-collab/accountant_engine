package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	kernalgovernance "github.com/osmai4603-collab/accounting_engine/internal/kernal/governance"
	internaltypes "github.com/osmai4603-collab/accounting_engine/internal/kernal/types"
	publicgovernance "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/governance"
	publictypes "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
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
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, record.CompanyID.String()); err != nil {
		return err
	}

	var sequenceNumber uint64
	var previousHash string
	err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence_number),0),
		COALESCE((SELECT current_log_hash FROM financial_audit_logs
		 WHERE company_id=$1 ORDER BY sequence_number DESC LIMIT 1), $2)
		FROM financial_audit_logs WHERE company_id=$1`,
		uuid.UUID(record.CompanyID), genesisHash).Scan(&sequenceNumber, &previousHash)
	if err != nil {
		return err
	}

	timestamp := record.OccurredAt
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	} else {
		timestamp = timestamp.UTC()
	}
	payload := record.Payload
	if len(payload) == 0 {
		payload = []byte("null")
	}

	log := kernalgovernance.AuditLog{
		SequenceNumber:  sequenceNumber + 1,
		CompanyID:       internaltypes.CompanyID(record.CompanyID),
		EntityType:      record.EntityType,
		EntityID:        record.EntityID,
		Action:          record.Action,
		ActorID:         record.ActorID,
		Reason:          record.Reason,
		CorrelationID:   record.CorrelationID,
		Payload:         payload,
		Timestamp:       timestamp,
		PreviousLogHash: previousHash,
	}
	log.CurrentLogHash = kernalgovernance.CalculateHash(log)

	_, err = tx.Exec(ctx, `INSERT INTO financial_audit_logs(
		company_id, entity_type, entity_id, action, actor_id, reason, correlation_id,
		payload_after, timestamp, previous_log_hash, current_log_hash, sequence_number
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		uuid.UUID(record.CompanyID), log.EntityType, log.EntityID, log.Action,
		log.ActorID, log.Reason, log.CorrelationID, log.Payload, log.Timestamp,
		log.PreviousLogHash, log.CurrentLogHash, log.SequenceNumber)
	return err
}

func (d *Driver) AppendAuditLogAtomic(ctx context.Context, log publicgovernance.AuditLog) (publicgovernance.AuditLog, error) {
	if d == nil || d.pool == nil {
		return kernalgovernance.AuditLog{}, ErrDriverNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return kernalgovernance.AuditLog{}, err
	}
	var result kernalgovernance.AuditLog
	err := d.WithCompanyTx(ctx, uuid.UUID(log.CompanyID), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, log.CompanyID.String()); err != nil {
			return err
		}
		var sequenceNumber uint64
		var previousHash string
		err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence_number),0),
			COALESCE((SELECT current_log_hash FROM financial_audit_logs
			 WHERE company_id=$1 ORDER BY sequence_number DESC LIMIT 1), $2)
			FROM financial_audit_logs WHERE company_id=$1`,
			uuid.UUID(log.CompanyID), genesisHash).Scan(&sequenceNumber, &previousHash)
		if err != nil {
			return err
		}
		log.ID = int64(sequenceNumber + 1)
		log.SequenceNumber = sequenceNumber + 1
		log.PreviousLogHash = previousHash
		if log.Timestamp.IsZero() {
			log.Timestamp = time.Now().UTC()
		} else {
			log.Timestamp = log.Timestamp.UTC()
		}
		log.CurrentLogHash = kernalgovernance.CalculateHash(log)

		_, err = tx.Exec(ctx, `INSERT INTO financial_audit_logs(
			company_id, entity_type, entity_id, action, actor_id, reason, correlation_id,
			payload_after, timestamp, previous_log_hash, current_log_hash, sequence_number
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			uuid.UUID(log.CompanyID), log.EntityType, log.EntityID, log.Action, log.ActorID,
			log.Reason, log.CorrelationID, log.Payload, log.Timestamp,
			log.PreviousLogHash, log.CurrentLogHash, log.SequenceNumber)
		if err != nil {
			return err
		}
		result = log
		return nil
	})
	if err != nil {
		return kernalgovernance.AuditLog{}, err
	}
	return result, nil
}

func (d *Driver) AppendAuditLog(ctx context.Context, log publicgovernance.AuditLog) error {
	_, err := d.AppendAuditLogAtomic(ctx, log)
	return err
}

func (d *Driver) GetLastAuditLogForCompany(ctx context.Context, companyID publictypes.CompanyID) (*publicgovernance.AuditLog, error) {
	if d == nil || d.pool == nil {
		return nil, ErrDriverNotInitialized
	}
	var log kernalgovernance.AuditLog
	err := d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT id, sequence_number, company_id, entity_type, entity_id, action,
			actor_id, reason, correlation_id, payload_after, timestamp,
			previous_log_hash, current_log_hash
			FROM financial_audit_logs WHERE company_id=$1
			ORDER BY sequence_number DESC LIMIT 1`,
			uuid.UUID(companyID))
		return scanAuditLog(row, &log)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func scanAuditLog(row pgx.Row, log *kernalgovernance.AuditLog) error {
	var id int64
	var sequence int64
	var companyID uuid.UUID
	var payload []byte
	if err := row.Scan(
		&id, &sequence, &companyID, &log.EntityType, &log.EntityID, &log.Action,
		&log.ActorID, &log.Reason, &log.CorrelationID, &payload, &log.Timestamp,
		&log.PreviousLogHash, &log.CurrentLogHash,
	); err != nil {
		return err
	}
	log.ID = id
	log.SequenceNumber = uint64(sequence)
	log.CompanyID = internaltypes.CompanyID(companyID)
	log.Payload = append([]byte(nil), payload...)
	return nil
}

func (d *Driver) IterateAuditLogs(ctx context.Context, companyID publictypes.CompanyID, fn func(publicgovernance.AuditLog) error) error {
	if d == nil || d.pool == nil {
		return ErrDriverNotInitialized
	}
	if fn == nil {
		return kernalgovernance.ErrInvalidAuditSnapshot
	}
	return d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, sequence_number, company_id, entity_type, entity_id, action,
			actor_id, reason, correlation_id, payload_after, timestamp,
			previous_log_hash, current_log_hash
			FROM financial_audit_logs WHERE company_id=$1
			ORDER BY sequence_number ASC, id ASC`,
			uuid.UUID(companyID))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			var log kernalgovernance.AuditLog
			if err := scanAuditLog(rows, &log); err != nil {
				return err
			}
			if err := fn(log); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

func (d *Driver) GetAuditVerificationSnapshot(ctx context.Context, companyID publictypes.CompanyID) (publicgovernance.AuditVerificationSnapshot, error) {
	if d == nil || d.pool == nil {
		return kernalgovernance.AuditVerificationSnapshot{}, ErrDriverNotInitialized
	}
	if err := ctx.Err(); err != nil {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_company_id',$1,true)`, companyID.String()); err != nil {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}

	logs := make([]kernalgovernance.AuditLog, 0)
	rows, err := tx.Query(ctx, `SELECT id, sequence_number, company_id, entity_type, entity_id, action,
		actor_id, reason, correlation_id, payload_after, timestamp,
		previous_log_hash, current_log_hash
		FROM financial_audit_logs WHERE company_id=$1
		ORDER BY sequence_number ASC, id ASC`,
		uuid.UUID(companyID))
	if err != nil {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}
	for rows.Next() {
		var log kernalgovernance.AuditLog
		if err := scanAuditLog(rows, &log); err != nil {
			rows.Close()
			return kernalgovernance.AuditVerificationSnapshot{}, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}
	rows.Close()

	checkpoint := kernalgovernance.AuditCheckpoint{
		CompanyID: companyID, SequenceNumber: 0, Version: 0,
		LogHash: kernalgovernance.GenesisHash,
	}
	var sequence int64
	var version int64
	var logHash string
	err = tx.QueryRow(ctx, `SELECT sequence_number, version, log_hash
		FROM external_audit_checkpoints WHERE company_id=$1`,
		uuid.UUID(companyID)).Scan(&sequence, &version, &logHash)
	checkpointPresent := false
	if err == nil {
		checkpointPresent = true
		checkpoint.SequenceNumber = uint64(sequence)
		checkpoint.Version = uint64(version)
		checkpoint.LogHash = logHash
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return kernalgovernance.AuditVerificationSnapshot{}, err
	}
	return kernalgovernance.AuditVerificationSnapshot{
		CompanyID: companyID, SnapshotSequence: uint64(len(logs)),
		Checkpoint: checkpoint, CheckpointPresent: checkpointPresent, Logs: logs,
	}, nil
}

func (d *Driver) SetExternalAuditCheckpoint(ctx context.Context, companyID publictypes.CompanyID, checkpoint publicgovernance.AuditCheckpoint) error {
	if d == nil || d.pool == nil {
		return ErrDriverNotInitialized
	}
	checkpoint.CompanyID = companyID
	if err := kernalgovernance.ValidateCheckpoint(checkpoint); err != nil {
		return err
	}
	return d.WithCompanyTx(ctx, uuid.UUID(companyID), func(tx pgx.Tx) error {
		var currentSequence, currentVersion int64
		err := tx.QueryRow(ctx, `SELECT sequence_number, version FROM external_audit_checkpoints
			WHERE company_id=$1`, uuid.UUID(companyID)).Scan(&currentSequence, &currentVersion)
		if err == nil && (checkpoint.Version <= uint64(currentVersion) ||
			checkpoint.SequenceNumber < uint64(currentSequence)) {
			return kernalgovernance.ErrInvalidAuditCheckpoint
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_audit_checkpoints
			(company_id, sequence_number, version, log_hash)
			VALUES($1,$2,$3,$4)
			ON CONFLICT (company_id) DO UPDATE SET
				sequence_number=EXCLUDED.sequence_number,
				version=EXCLUDED.version,
				log_hash=EXCLUDED.log_hash,
				updated_at=now()`,
			uuid.UUID(companyID), checkpoint.SequenceNumber, checkpoint.Version, checkpoint.LogHash)
		return err
	})
}
