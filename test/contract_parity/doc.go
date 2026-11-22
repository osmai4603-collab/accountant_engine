// Package contract_parity: Adapter-neutral contract parity suite: memory adapter versus PostgreSQL.
//
// Phase 7 adds scope-aware idempotency replay/conflict, unknown-outcome fault
// injection, reversal atomicity, concurrent single-winner checks, company
// scope isolation and preservation of kernel-stamped dates. Every scenario is
// executed against both adapters through the same harness; generated parity
// reports are an output, not a substitute for the suite.
//
// Import charter: Both adapters must satisfy the identical pkg/spi/storage contracts.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package contract_parity
