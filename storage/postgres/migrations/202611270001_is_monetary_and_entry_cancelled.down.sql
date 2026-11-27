ALTER TABLE account_moves
    DROP CONSTRAINT IF EXISTS chk_move_status;
ALTER TABLE account_moves
    ADD CONSTRAINT chk_move_status CHECK (status IN ('draft', 'posted', 'reversed'));

ALTER TABLE account_versions
    DROP COLUMN IF EXISTS is_monetary;

ALTER TABLE accounts
    DROP COLUMN IF EXISTS is_monetary;

