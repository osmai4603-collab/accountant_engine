-- Allow cancelled unposted moves to have NULL posting_date (JRN-007).
ALTER TABLE account_moves
    DROP CONSTRAINT IF EXISTS chk_posted_move_posting_date;
ALTER TABLE account_moves
    ADD CONSTRAINT chk_posted_move_posting_date
    CHECK (status IN ('draft', 'cancelled') OR posting_date IS NOT NULL);

-- Index for fast lookup of fiscal periods by (company_id, year_id, period_number) including adjustment periods 13 and 14.
CREATE INDEX IF NOT EXISTS idx_fiscal_periods_company_year_number
    ON fiscal_periods(company_id, year_id, period_number);

-- Track audited year-end close and reopen lifecycles per company and fiscal year.
CREATE TABLE IF NOT EXISTS year_end_closings (
    company_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    fiscal_year_id uuid NOT NULL,
    next_fiscal_year_id uuid,
    closing_entry_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    opening_entry_id uuid,
    net_income text NOT NULL DEFAULT '0',
    policy text NOT NULL DEFAULT 'direct_to_retained_earnings',
    closed_by text NOT NULL,
    closed_at timestamptz NOT NULL,
    reason text NOT NULL DEFAULT '',
    reopened_by text NOT NULL DEFAULT '',
    reopened_at timestamptz,
    reopen_reason text NOT NULL DEFAULT '',
    approval_reference text NOT NULL DEFAULT '',
    PRIMARY KEY (company_id, fiscal_year_id),
    FOREIGN KEY (company_id, fiscal_year_id) REFERENCES fiscal_years(company_id, id) ON DELETE RESTRICT
);

ALTER TABLE year_end_closings ENABLE ROW LEVEL SECURITY;
ALTER TABLE year_end_closings FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_policy ON year_end_closings;
CREATE POLICY tenant_isolation_policy ON year_end_closings
    USING (company_id = app_current_company_id())
    WITH CHECK (company_id = app_current_company_id());

