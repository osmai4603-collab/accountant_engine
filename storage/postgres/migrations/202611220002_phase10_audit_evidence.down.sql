DROP TABLE IF EXISTS external_audit_checkpoints;

DROP FUNCTION IF EXISTS prevent_audit_checkpoint_regression();

DROP INDEX IF EXISTS uq_audit_chain_company_sequence;

DROP TRIGGER IF EXISTS trg_prevent_financial_audit_log_mutation ON financial_audit_logs;
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_truncate ON financial_audit_logs;

-- Restore the append-only triggers owned by 202611100005_phase3_governance.
-- The up migration drops them while rebuilding the v2 chain; leaving them
-- dropped would make rollback silently disable audit immutability.
CREATE OR REPLACE FUNCTION prevent_financial_audit_log_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'financial_audit_logs is append-only'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_update ON financial_audit_logs;
CREATE TRIGGER trg_financial_audit_logs_no_update
    BEFORE UPDATE ON financial_audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_financial_audit_log_mutation();
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_delete ON financial_audit_logs;
CREATE TRIGGER trg_financial_audit_logs_no_delete
    BEFORE DELETE ON financial_audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_financial_audit_log_mutation();
DROP TRIGGER IF EXISTS trg_financial_audit_logs_no_truncate ON financial_audit_logs;
CREATE TRIGGER trg_financial_audit_logs_no_truncate
    BEFORE TRUNCATE ON financial_audit_logs
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_financial_audit_log_mutation();

ALTER TABLE financial_audit_logs
    DROP COLUMN IF EXISTS sequence_number,
    DROP COLUMN IF EXISTS reason,
    DROP COLUMN IF EXISTS correlation_id,
    ALTER COLUMN entity_id TYPE uuid USING entity_id::uuid,
    ALTER COLUMN actor_id TYPE uuid USING actor_id::uuid;
