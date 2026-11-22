CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE financial_audit_logs
    ADD COLUMN IF NOT EXISTS sequence_number bigint,
    ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS correlation_id text NOT NULL DEFAULT '';

ALTER TABLE financial_audit_logs
    ALTER COLUMN sequence_number DROP DEFAULT;

ALTER TABLE financial_audit_logs
    ALTER COLUMN entity_id TYPE text USING entity_id::text,
    ALTER COLUMN actor_id TYPE text USING actor_id::text;

CREATE TABLE IF NOT EXISTS external_audit_checkpoints (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    sequence_number bigint NOT NULL CHECK (sequence_number >= 0),
    version bigint NOT NULL CHECK (version >= 0),
    log_hash varchar(64) NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id)
);

DROP TRIGGER IF EXISTS trg_prevent_financial_audit_log_mutation ON financial_audit_logs;
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_update ON financial_audit_logs;
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_delete ON financial_audit_logs;
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_truncate ON financial_audit_logs;

WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY company_id ORDER BY id) AS sequence_number
    FROM financial_audit_logs
)
UPDATE financial_audit_logs AS audit_log
SET sequence_number = ranked.sequence_number
FROM ranked
WHERE audit_log.id = ranked.id;

ALTER TABLE financial_audit_logs
    ALTER COLUMN sequence_number SET NOT NULL;

DO $$
DECLARE
    audit_row record;
    current_company uuid;
    sequence_value bigint := 0;
    previous_hash text := '0000000000000000000000000000000000000000000000000000000000000000';
    previous_before text;
    payload_hash text;
    timestamp_text text;
    canonical text;
BEGIN
    FOR audit_row IN
        SELECT id, company_id, sequence_number, entity_type, entity_id, action, actor_id,
               reason, correlation_id, payload_after, timestamp
        FROM financial_audit_logs
        ORDER BY company_id, sequence_number, id
    LOOP
        IF current_company IS NULL OR current_company <> audit_row.company_id THEN
            current_company := audit_row.company_id;
            sequence_value := 0;
            previous_hash := '0000000000000000000000000000000000000000000000000000000000000000';
        END IF;

        sequence_value := sequence_value + 1;
        previous_before := previous_hash;
        payload_hash := encode(digest(COALESCE(audit_row.payload_after, '\x'::bytea), 'sha256'), 'hex');
        timestamp_text := (((extract(epoch FROM date_trunc('microseconds', audit_row.timestamp)) * 1000000000)::numeric(38,0)))::text;

        canonical :=
            octet_length('v2')::text || ':' || 'v2' ||
            octet_length(sequence_value::text)::text || ':' || sequence_value::text ||
            octet_length(audit_row.company_id::text)::text || ':' || audit_row.company_id::text ||
            octet_length(COALESCE(audit_row.entity_type, ''))::text || ':' || COALESCE(audit_row.entity_type, '') ||
            octet_length(COALESCE(audit_row.entity_id, ''))::text || ':' || COALESCE(audit_row.entity_id, '') ||
            octet_length(COALESCE(audit_row.action, ''))::text || ':' || COALESCE(audit_row.action, '') ||
            octet_length(COALESCE(audit_row.actor_id, ''))::text || ':' || COALESCE(audit_row.actor_id, '') ||
            octet_length(COALESCE(audit_row.reason, ''))::text || ':' || COALESCE(audit_row.reason, '') ||
            octet_length(COALESCE(audit_row.correlation_id, ''))::text || ':' || COALESCE(audit_row.correlation_id, '') ||
            octet_length(timestamp_text)::text || ':' || timestamp_text ||
            octet_length(payload_hash)::text || ':' || payload_hash ||
            octet_length(previous_before)::text || ':' || previous_before;

        previous_hash := encode(digest(canonical, 'sha256'), 'hex');
        UPDATE financial_audit_logs
        SET sequence_number = sequence_value,
            previous_log_hash = previous_before,
            current_log_hash = previous_hash
        WHERE id = audit_row.id;
    END LOOP;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_audit_chain_company_sequence
    ON financial_audit_logs(company_id, sequence_number);

DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_update ON financial_audit_logs;
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_delete ON financial_audit_logs;
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_truncate ON financial_audit_logs;

CREATE TRIGGER trg_prevent_financial_audit_log_mutation
    BEFORE UPDATE OR DELETE ON financial_audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_financial_audit_log_mutation();

CREATE TRIGGER trg_financial_audit_logs_no_truncate
    BEFORE TRUNCATE ON financial_audit_logs
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_financial_audit_log_mutation();

-- A checkpoint may advance, but must never be rolled back to an older trusted
-- position by a database-side writer. The adapter applies the same rule.
CREATE OR REPLACE FUNCTION prevent_audit_checkpoint_regression() RETURNS trigger AS $$
BEGIN
    IF NEW.version <= OLD.version OR NEW.sequence_number < OLD.sequence_number THEN
        RAISE EXCEPTION 'external audit checkpoint cannot move backwards'
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_external_audit_checkpoint_monotonic ON external_audit_checkpoints;
CREATE TRIGGER trg_external_audit_checkpoint_monotonic
    BEFORE UPDATE ON external_audit_checkpoints
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_checkpoint_regression();

ALTER TABLE external_audit_checkpoints ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_audit_checkpoints FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_policy ON external_audit_checkpoints;
CREATE POLICY tenant_isolation_policy ON external_audit_checkpoints
    USING (company_id = app_current_company_id())
    WITH CHECK (company_id = app_current_company_id());
