// Package memory provides a volatile in-memory storage adapter for development,
// local testing, and CI pipelines ONLY.
//
// STRICT ARCHITECTURAL & OPERATIONAL PROHIBITION:
// This memory adapter is NOT durable, provides no cross-process concurrency locks,
// and enforces NO row-level security (RLS) or database-level WORM immutability triggers.
// IT IS STRICTLY FORBIDDEN TO USE THIS ADAPTER IN PRODUCTION ENVIRONMENTS.
// The sole production-certified adapter for the accountant engine is storage/postgres.
//
// Capabilities:
//   - GuaranteesDurability: false
//   - Atomic snapshot rollback for UnitOfWork / TxRunner.
//   - Thread-safe in-memory operations protected by RWMutex.
//
// Baseline: AE-PLAN-STEP-P3 / P3-01.
package memory
