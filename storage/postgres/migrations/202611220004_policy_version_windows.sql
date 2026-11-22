-- Phase 11: enforce one policy version per effective start inside a company
-- and policy family. The kernel already rejects overlaps in memory; this index
-- makes the rule durable and protects concurrent host writers.
CREATE UNIQUE INDEX IF NOT EXISTS uq_accounting_policy_effective_from
    ON accounting_policy_versions (company_id, policy_kind, effective_from);
