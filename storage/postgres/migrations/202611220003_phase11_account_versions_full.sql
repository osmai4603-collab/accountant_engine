-- G-019: carry a complete, immutable account snapshot in every effective-dated
-- row. Bounds remain UTC calendar days (date columns, inclusive on read) so the
-- same window resolves identically in memory and PostgreSQL.
ALTER TABLE account_versions
    ADD COLUMN IF NOT EXISTS code varchar(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS normal_balance varchar(6) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS currency_id varchar(3) NOT NULL DEFAULT 'USD',
    ADD COLUMN IF NOT EXISTS parent_id uuid,
    ADD COLUMN IF NOT EXISTS is_reconcilable boolean NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_control_account boolean NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_contra_account boolean NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS requires_cost_center boolean NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_cash_equivalent boolean NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_non_cash_adjustment boolean NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS cash_flow_activity varchar(64) NOT NULL DEFAULT 'unclassified',
    ADD COLUMN IF NOT EXISTS sub_classification varchar(64) NOT NULL DEFAULT 'unclassified',
    ADD COLUMN IF NOT EXISTS display_order integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT NOW();
