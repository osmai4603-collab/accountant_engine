package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/osmai4603-collab/accounting_engine/pkg/api/v1/journal"
	publicevents "github.com/osmai4603-collab/accounting_engine/pkg/spi/events"
)

var (
	_ publicevents.IdempotencyStore       = (*Driver)(nil)
	_ publicevents.ScopedIdempotencyStore = (*Driver)(nil)
	_ publicevents.MappingStore           = (*Driver)(nil)
)

const publicEventIngestionColumns = `company_id, idempotency_key, source_system, event_type,
	payload_hash, payload_hash_version, entry_id, entry_snapshot, status,
	failure_class, failure_reason, created_at, updated_at`

func (d *Driver) GetPublicEventIngestion(ctx context.Context, companyID uuid.UUID, idempotencyKey string) (publicevents.Ingestion, bool, error) {
	return d.GetPublicEventIngestionForSource(ctx, companyID, publicevents.DefaultSourceSystem, idempotencyKey)
}

func (d *Driver) GetPublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, sourceSystem, idempotencyKey string) (publicevents.Ingestion, bool, error) {
	var rec publicevents.Ingestion
	err := d.WithCompanyTx(ctx, companyID, func(tx pgx.Tx) error {
		var entryID *uuid.UUID
		var snapshot []byte
		var createdAt, updatedAt time.Time
		scanErr := tx.QueryRow(ctx,
			`SELECT `+publicEventIngestionColumns+`
			   FROM public_event_ingestions
			  WHERE company_id=$1 AND source_system=$2 AND idempotency_key=$3`,
			companyID, publicevents.NormalizeSourceSystem(sourceSystem), strings.TrimSpace(idempotencyKey),
		).Scan(
			&rec.CompanyID, &rec.IdempotencyKey, &rec.SourceSystem, &rec.EventType,
			&rec.PayloadHash, &rec.PayloadHashVersion, &entryID, &snapshot, &rec.Status,
			&rec.FailureClass, &rec.FailureReason, &createdAt, &updatedAt,
		)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		if scanErr != nil {
			return scanErr
		}
		if entryID != nil {
			rec.EntryID = *entryID
		}
		if len(snapshot) > 0 && string(snapshot) != "{}" && string(snapshot) != "null" {
			if err := json.Unmarshal(snapshot, &rec.Entry); err != nil {
				return fmt.Errorf("decode public event ingestion entry snapshot: %w", err)
			}
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return publicevents.Ingestion{}, false, nil
	}
	if err != nil {
		return publicevents.Ingestion{}, false, err
	}
	return rec, true, nil
}

func (d *Driver) ClaimPublicEventIngestion(ctx context.Context, record publicevents.Ingestion) (publicevents.Ingestion, bool, error) {
	record.SourceSystem = publicevents.NormalizeSourceSystem(record.SourceSystem)
	record.IdempotencyKey = strings.TrimSpace(record.IdempotencyKey)
	record.Status = strings.TrimSpace(record.Status)
	if record.Status == "" {
		record.Status = publicevents.StatusStarted
	}
	snapshot, err := json.Marshal(record.Entry)
	if err != nil {
		return publicevents.Ingestion{}, false, fmt.Errorf("encode snapshot: %w", err)
	}
	if len(snapshot) == 0 || string(snapshot) == "null" {
		snapshot = []byte("{}")
	}

	const claimSQL = `
WITH target AS (
    SELECT company_id, idempotency_key, source_system, event_type, payload_hash,
           payload_hash_version, entry_id, entry_snapshot, status, failure_class,
           failure_reason, created_at, updated_at
      FROM public_event_ingestions
     WHERE company_id=$1 AND source_system=$2 AND idempotency_key=$3
       FOR UPDATE
), reclaimed AS (
    UPDATE public_event_ingestions t
       SET status=$9, failure_reason='', failure_class='', updated_at=NOW()
      FROM target
     WHERE t.company_id=target.company_id
       AND t.source_system=target.source_system
       AND t.idempotency_key=target.idempotency_key
       AND target.payload_hash=$4
       AND target.status IN ('retryable_failure', 'failed')
    RETURNING t.company_id, t.idempotency_key, t.source_system, t.event_type,
              t.payload_hash, t.payload_hash_version, t.entry_id, t.entry_snapshot,
              t.status, t.failure_class, t.failure_reason, t.created_at, t.updated_at,
              true AS claimed
), inserted AS (
    INSERT INTO public_event_ingestions(
        company_id, idempotency_key, source_system, event_type, payload_hash,
        payload_hash_version, entry_id, entry_snapshot, status, failure_class,
        failure_reason, created_at, updated_at)
    SELECT $1, $3, $2, $5, $4, $6,
           NULLIF($7, '00000000-0000-0000-0000-000000000000'::uuid),
           $8, $9, '', '', NOW(), NOW()
     WHERE NOT EXISTS (SELECT 1 FROM target)
    ON CONFLICT (company_id, source_system, idempotency_key) DO NOTHING
    RETURNING company_id, idempotency_key, source_system, event_type, payload_hash,
              payload_hash_version, entry_id, entry_snapshot, status, failure_class,
              failure_reason, created_at, updated_at, true AS claimed
)
SELECT * FROM inserted
UNION ALL
SELECT * FROM reclaimed
UNION ALL
SELECT target.company_id, target.idempotency_key, target.source_system, target.event_type,
       target.payload_hash, target.payload_hash_version, target.entry_id, target.entry_snapshot,
       target.status, target.failure_class, target.failure_reason, target.created_at,
       target.updated_at, false AS claimed
  FROM target;`

	var existing publicevents.Ingestion
	var claimed bool
	err = d.WithCompanyTx(ctx, record.CompanyID, func(tx pgx.Tx) error {
		var entryID *uuid.UUID
		var sn []byte
		var cAt, uAt time.Time
		scanErr := tx.QueryRow(ctx, claimSQL,
			record.CompanyID, record.SourceSystem, record.IdempotencyKey,
			record.PayloadHash, record.EventType, record.PayloadHashVersion,
			record.EntryID, sn, record.Status,
		).Scan(
			&existing.CompanyID, &existing.IdempotencyKey, &existing.SourceSystem,
			&existing.EventType, &existing.PayloadHash, &existing.PayloadHashVersion,
			&entryID, &sn, &existing.Status, &existing.FailureClass, &existing.FailureReason,
			&cAt, &uAt, &claimed,
		)
		if scanErr != nil {
			return scanErr
		}
		if entryID != nil {
			existing.EntryID = *entryID
		}
		if len(sn) > 0 && string(sn) != "{}" && string(sn) != "null" {
			_ = json.Unmarshal(sn, &existing.Entry)
		}
		return nil
	})
	if err != nil {
		return publicevents.Ingestion{}, false, err
	}
	return existing, claimed, nil
}

func (d *Driver) CompletePublicEventIngestion(ctx context.Context, companyID uuid.UUID, idempotencyKey string, entry journal.JournalEntry) error {
	return d.CompletePublicEventIngestionForSource(ctx, companyID, publicevents.DefaultSourceSystem, idempotencyKey, "", entry)
}

func (d *Driver) CompletePublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, sourceSystem, idempotencyKey, payloadHash string, entry journal.JournalEntry) error {
	snapshot, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return d.WithCompanyTx(ctx, companyID, func(tx pgx.Tx) error {
		res, err := tx.Exec(ctx, `UPDATE public_event_ingestions
			SET status='completed', entry_id=$1, entry_snapshot=$2, updated_at=NOW()
			WHERE company_id=$3 AND source_system=$4 AND idempotency_key=$5`,
			uuid.UUID(entry.ID), snapshot, companyID, publicevents.NormalizeSourceSystem(sourceSystem), strings.TrimSpace(idempotencyKey))
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return publicevents.ErrRecordNotFound
		}
		return nil
	})
}

func (d *Driver) FailPublicEventIngestion(ctx context.Context, companyID uuid.UUID, idempotencyKey, reason string) error {
	return d.FailPublicEventIngestionForSource(ctx, companyID, publicevents.DefaultSourceSystem, idempotencyKey, publicevents.FailureClassTransient, reason)
}

func (d *Driver) FailPublicEventIngestionForSource(ctx context.Context, companyID uuid.UUID, sourceSystem, idempotencyKey, failureClass, reason string) error {
	status := publicevents.StatusRetryableFailure
	if failureClass == publicevents.FailureClassTerminal {
		status = publicevents.StatusTerminalFailure
	}
	return d.WithCompanyTx(ctx, companyID, func(tx pgx.Tx) error {
		res, err := tx.Exec(ctx, `UPDATE public_event_ingestions
			SET status=$1, failure_class=$2, failure_reason=$3, updated_at=NOW()
			WHERE company_id=$4 AND source_system=$5 AND idempotency_key=$6`,
			status, failureClass, reason, companyID, publicevents.NormalizeSourceSystem(sourceSystem), strings.TrimSpace(idempotencyKey))
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return publicevents.ErrRecordNotFound
		}
		return nil
	})
}

func (d *Driver) GetPublicEventAccountMapping(ctx context.Context, companyID uuid.UUID, eventType, ledgerID string) (publicevents.AccountMapping, bool, error) {
	var m publicevents.AccountMapping
	err := d.WithCompanyTx(ctx, companyID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT company_id, event_type, ledger_id, debit_account_code, credit_account_code, description_template, journal_code
			FROM public_event_account_mappings WHERE company_id=$1 AND event_type=$2 AND ledger_id=$3`,
			companyID, eventType, ledgerID).Scan(
			&m.CompanyID, &m.EventType, &m.LedgerID, &m.DebitAccountCode,
			&m.CreditAccountCode, &m.DescriptionTemplate, &m.JournalCode)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return publicevents.AccountMapping{}, false, nil
	}
	if err != nil {
		return publicevents.AccountMapping{}, false, err
	}
	return m, true, nil
}

func (d *Driver) SavePublicEventAccountMapping(ctx context.Context, mapping publicevents.AccountMapping) error {
	return d.WithCompanyTx(ctx, mapping.CompanyID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO public_event_account_mappings(
			company_id, event_type, ledger_id, debit_account_code, credit_account_code, description_template, journal_code
		) VALUES($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(company_id, event_type, ledger_id) DO UPDATE SET
			debit_account_code=EXCLUDED.debit_account_code,
			credit_account_code=EXCLUDED.credit_account_code,
			description_template=EXCLUDED.description_template,
			journal_code=EXCLUDED.journal_code`,
			mapping.CompanyID, mapping.EventType, mapping.LedgerID,
			mapping.DebitAccountCode, mapping.CreditAccountCode, mapping.DescriptionTemplate, mapping.JournalCode)
		return err
	})
}
