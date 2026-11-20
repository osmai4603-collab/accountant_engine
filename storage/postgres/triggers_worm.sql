-- triggers_worm.sql
-- Comprehensive consolidated reference script documenting and verifying
-- Write Once, Read Many (WORM) and immutability triggers for financial entries
-- and audit logs in the accountant engine.

-- 1. Prevent modification or deletion of posted entries and lines
CREATE OR REPLACE FUNCTION prevent_posted_move_mutation() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status = 'posted' THEN
            RAISE EXCEPTION 'cannot delete posted entry: immutable by accounting policy'
                USING ERRCODE = '55000';
        END IF;
        RETURN OLD;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF OLD.status = 'posted' THEN
            -- Only allow reversing_move_id updates when linking reversal, or reject any changes
            IF NEW.id = OLD.id
               AND NEW.company_id = OLD.company_id
               AND NEW.journal_id = OLD.journal_id
               AND NEW.fiscal_period_id = OLD.fiscal_period_id
               AND NEW.entry_number = OLD.entry_number
               AND NEW.entry_date = OLD.entry_date
               AND NEW.posted_at = OLD.posted_at
               AND NEW.posted_by = OLD.posted_by
               AND NEW.status = OLD.status
               AND (OLD.reversing_entry_id IS NULL OR NEW.reversing_entry_id = OLD.reversing_entry_id) THEN
                RETURN NEW;
            END IF;
            RAISE EXCEPTION 'cannot update posted entry: immutable by accounting policy'
                USING ERRCODE = '55000';
        END IF;
        RETURN NEW;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_posted_move_mutation ON account_moves;
CREATE TRIGGER trg_prevent_posted_move_mutation
    BEFORE UPDATE OR DELETE ON account_moves
    FOR EACH ROW EXECUTE FUNCTION prevent_posted_move_mutation();

-- 2. Prevent direct insert of posted entries (must be created as draft or committed atomically)
CREATE OR REPLACE FUNCTION prevent_direct_posted_move_insert() RETURNS trigger AS $$
BEGIN
    IF NEW.status = 'posted' AND (NEW.posted_at IS NULL OR NEW.posted_by IS NULL) THEN
        RAISE EXCEPTION 'posted entry requires valid posted_at and posted_by'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_direct_posted_move_insert ON account_moves;
CREATE TRIGGER trg_prevent_direct_posted_move_insert
    BEFORE INSERT ON account_moves
    FOR EACH ROW EXECUTE FUNCTION prevent_direct_posted_move_insert();

-- 3. Prevent modification or deletion of financial audit logs (Strict WORM)
CREATE OR REPLACE FUNCTION prevent_financial_audit_log_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'financial audit logs are strictly append-only and cannot be updated or deleted'
        USING ERRCODE = '55000';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_financial_audit_log_mutation ON financial_audit_logs;
CREATE TRIGGER trg_prevent_financial_audit_log_mutation
    BEFORE UPDATE OR DELETE ON financial_audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_financial_audit_log_mutation();

DROP TRIGGER IF EXISTS trg_prevent_financial_audit_log_truncate ON financial_audit_logs;
CREATE TRIGGER trg_prevent_financial_audit_log_truncate
    BEFORE TRUNCATE ON financial_audit_logs
    FOR EACH STATEMENT EXECUTE FUNCTION prevent_financial_audit_log_mutation();

