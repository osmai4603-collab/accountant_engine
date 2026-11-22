-- Roll back the full account-snapshot columns; the original version identity
-- (company, account, effective_from) and date bounds remain untouched.
ALTER TABLE account_versions
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS display_order,
    DROP COLUMN IF EXISTS sub_classification,
    DROP COLUMN IF EXISTS cash_flow_activity,
    DROP COLUMN IF EXISTS is_non_cash_adjustment,
    DROP COLUMN IF EXISTS is_cash_equivalent,
    DROP COLUMN IF EXISTS requires_cost_center,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS is_contra_account,
    DROP COLUMN IF EXISTS is_control_account,
    DROP COLUMN IF EXISTS is_reconcilable,
    DROP COLUMN IF EXISTS parent_id,
    DROP COLUMN IF EXISTS currency_id,
    DROP COLUMN IF EXISTS normal_balance,
    DROP COLUMN IF EXISTS code;
