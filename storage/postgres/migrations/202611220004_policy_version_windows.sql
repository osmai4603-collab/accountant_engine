-- Phase 11: enforce one policy version per effective start inside a company
-- and policy family. This closes the duplicate-start race between concurrent
-- host writers. Arbitrary overlapping windows with different starts are still
-- rejected by AppendVersion's advisory lock plus overlap check; raw SQL
-- writers bypassing that method are not covered by this index.
CREATE UNIQUE INDEX IF NOT EXISTS uq_accounting_policy_effective_from
    ON accounting_policy_versions (company_id, policy_kind, effective_from);
