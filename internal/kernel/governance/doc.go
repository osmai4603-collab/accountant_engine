// Package governance: Append-only audit chain and tamper-evident audit verification.
//
// Checkpoint verification requires an AuditVerificationSnapshotRepository that
// reads the company-scoped logs and external checkpoint from one stable view.
// The snapshot includes every sequence through SnapshotSequence and binds the
// checkpoint version and hash to that boundary. A repository that detects a
// concurrent checkpoint or boundary change returns ErrAuditSnapshotConflict;
// verification retries that conflict a bounded number of times and otherwise
// returns ErrAuditSnapshotConsistencyRetryable. A mismatch inside a stable
// snapshot is an integrity failure, not a retry signal.
//
// The checkpoint must be retained independently of the mutable audit log.
// Without a trusted checkpoint beyond a deleted suffix, a valid remaining
// prefix cannot prove that the suffix ever existed.
//
// Import charter: Go standard library, sibling internal/kernel/* modules, uuid and decimal only.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package governance
