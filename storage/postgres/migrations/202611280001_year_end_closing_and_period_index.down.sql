DROP POLICY IF EXISTS tenant_isolation_policy ON year_end_closings;
DROP TABLE IF EXISTS year_end_closings;
DROP INDEX IF EXISTS idx_fiscal_periods_company_year_number;
ALTER TABLE account_moves
    DROP CONSTRAINT IF EXISTS chk_posted_move_posting_date;
ALTER TABLE account_moves
    ADD CONSTRAINT chk_posted_move_posting_date
    CHECK (status = 'draft' OR posting_date IS NOT NULL);

