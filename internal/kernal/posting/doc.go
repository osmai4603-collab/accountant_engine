// Package posting: Atomic posting pipeline, double-entry validation, reversals and ordered account lock keys.
//
// Source of truth: PostEntry is the sole central path that validates and stamps a
// journal entry as posted, then delegates atomic persistence to the host-provided
// PostingExecutor / PostingReversalExecutor. The posted entry with its lines is
// the canonical accounting fact; no separate ledger movement or stored balance is
// created by the kernel. See docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md.
// ExtractAndSortAccountIDs only produces a deterministic key order for the host
// adapter to lock; it does not acquire a lock itself. IdempotencyKey is an
// execution-control input whose uniqueness scope and payload fingerprints are
// stamped by the kernel; the adapter persists and enforces that contract.
//
// Journal entries require a recognized, non-blank classification.
// AdjustingEntryAudit is valid only for target period 14; period 14 rejects all
// non-audit adjusting types. Reversing entries require a non-blank reason in
// both the reversal builder and classification metadata validation.
//
// Phase 6 posting charter: PostEntry validates identity, classification,
// status, scope, currencies, journal, period, policies, accounts, dimensions,
// rounding and double-entry balance before either executor is called. It clones
// the entry before repository access, resolves each account once as of the
// entry date and feeds that exact snapshot to eligibility checks, then hands a
// frozen, stamped entry to the host. Reversal builders never mutate the posted
// original; wall-clock access is confined to the Clock/systemClock boundary.
//
// Phase 7 transaction charter: before executor dispatch PostEntry derives the
// authoritative IdempotencyScope from company, operation and key, then stamps
// RequestFingerprint and CallerFingerprint over the frozen request and the
// stable caller payload. Caller-supplied scope or fingerprint values are
// overwritten. The kernel classifies retryable outcomes but opens no
// transaction, takes no row lock, performs no retry and knows no storage
// technology; the host must persist the scope, digest, result, audit effect
// and reversal link in one atomic unit.
//
// Phase 8 concurrency charter: AccountLockKey and OrderedAccountKeys derive
// deterministic, company-scoped key strings; ExtractAndSortAccountIDs remains
// as a compatibility wrapper. None of them acquires a lock. The adapter uses
// storage.LockTarget/WithOrderedLocks to acquire account, entry, original and
// idempotency locks in canonical order inside its own transaction. The kernel
// freezes the validated period, policy fingerprint, dates and amounts before
// dispatch so no adapter can reinterpret a posted entry with newer master
// data; company scope is never inferred from ActorID.
//
// Import charter: Go standard library, sibling internal/kernal/* modules, uuid and decimal only.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package posting
