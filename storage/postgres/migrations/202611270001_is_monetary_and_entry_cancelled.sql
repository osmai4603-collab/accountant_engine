-- MD-004 / VAL-006: explicit IAS 21 monetary classification on accounts and effective versions.
ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS is_monetary boolean;

ALTER TABLE account_versions
    ADD COLUMN IF NOT EXISTS is_monetary boolean;

-- JRN-007: allow draft -> cancelled lifecycle transitions on unposted moves.
ALTER TABLE account_moves
    DROP CONSTRAINT IF EXISTS chk_move_status;
ALTER TABLE account_moves
    ADD CONSTRAINT chk_move_status CHECK (status IN ('draft', 'posted', 'reversed', 'cancelled'));

