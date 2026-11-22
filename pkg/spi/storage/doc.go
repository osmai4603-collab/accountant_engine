// Package storage: Abstract persistence ports (UnitOfWork, EntryStore, AccountStore and friends).
//
// Phase 7 contract: PostingCommit carries the kernel-stamped idempotency scope
// and payload digest. Adapters must persist them with the entry and required
// audit effect in one atomic unit, return adapter-neutral outcome sentinels,
// and support context-carried test fault points. IdempotencyLookup is the
// side-effect-free read used to resolve an unknown outcome after commit.
//
// Phase 8 concurrency contract: LockTarget and OrderLockTargets define a
// neutral canonical key (v1|company=<id>|kind=<kind>|id=<value>); WithOrderedLocks
// makes adapters acquire account, entry, original and idempotency locks in
// ascending order and release them in reverse order, with an optional test-only
// LockObserver supplied through context. Adapters, not the kernel, perform the
// actual locking; CompanyID is mandatory, ErrScopeViolation is non-retryable,
// and bounded host retry is validated by MaxAllowedRetries/RetryPolicy.Validate.
//
// Import charter: Depends only on pkg/api/v1/* and stdlib; no concrete pgx or database/sql code.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package storage
