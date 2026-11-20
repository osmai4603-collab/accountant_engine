-- rls_policies.sql
-- Comprehensive consolidated reference script documenting and verifying
-- Row Level Security (RLS) policies and FORCE ROW LEVEL SECURITY for all
-- 42 company-scoped financial tables in the accountant engine.
--
-- Tenant isolation policy standard:
-- company_id = NULLIF(current_setting('app.current_company_id', true), '')::uuid

DO $$
DECLARE
    relation_name text;
    rls_tables text[] := ARRAY[
        'companies',
        'accounts',
        'fiscal_years',
        'fiscal_periods',
        'journals',
        'account_moves',
        'account_move_lines',
        'subledger_mappings',
        'event_accounting_rules',
        'subledger_events',
        'subledger_open_lines',
        'fixed_assets',
        'fixed_asset_depreciations',
        'bank_items',
        'exchange_rates',
        'financial_audit_logs',
        'accounting_configurations',
        'account_reconciliations',
        'account_reconciliation_items',
        'bank_reconciliations',
        'account_versions',
        'cost_centers',
        'governance_roles',
        'user_role_assignments',
        'approval_requests',
        'approval_steps',
        'financial_dimension_values',
        'posting_idempotency',
        'subledger_reconciliation_exceptions',
        'subledger_reconciliation_exception_events',
        'subledger_reconciliation_logs',
        'subledger_reconciliation_certificates',
        'governance_policies',
        'ifrs15_revenue_contracts',
        'public_event_ingestions',
        'public_event_account_mappings',
        'accounting_policy_versions',
        'accounting_policy_audit_logs',
        'financial_statement_notes',
        'reporting_taxonomies',
        'reporting_taxonomy_nodes',
        'reporting_account_tags'
    ];
BEGIN
    FOREACH relation_name IN ARRAY rls_tables LOOP
        IF EXISTS (
            SELECT 1 FROM information_schema.tables 
            WHERE table_schema = current_schema() AND table_name = relation_name
        ) THEN
            EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', relation_name);
            EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', relation_name);
            EXECUTE format('DROP POLICY IF EXISTS tenant_isolation_policy ON %I', relation_name);
            IF relation_name = 'companies' THEN
                EXECUTE format(
                    'CREATE POLICY tenant_isolation_policy ON %I FOR ALL USING (id = NULLIF(current_setting(''app.current_company_id'', true), '''')::uuid)',
                    relation_name
                );
            ELSE
                EXECUTE format(
                    'CREATE POLICY tenant_isolation_policy ON %I FOR ALL USING (company_id = NULLIF(current_setting(''app.current_company_id'', true), '''')::uuid)',
                    relation_name
                );
            END IF;
        END IF;
    END LOOP;
END;
$$;
