// Package policy: Accounting posting rules and account eligibility policies.
//
// Import charter: Go standard library, sibling internal/kernel/* modules, uuid and decimal only.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
//
// Responsibilities (Phase 5, ADR P5-01):
//   - Versioned PostingPolicy and CurrencyPolicy registration scoped by company.
//   - Deterministic resolution for a company and date: the single policy version
//     whose half-open window [EffectiveDate, EffectiveTo) contains that date.
//     A nil EffectiveTo ends at the next registered version; gaps fail closed.
//   - Rejection of duplicate effective starts and unresolved window overlaps
//     (ErrPolicyOverlap) before any posting decision is taken.
//   - Defensive copies of every slice/pointer crossing the PolicyEngine boundary,
//     and RWMutex-protected concurrent registration/resolution.
//   - A canonical SHA-256 Fingerprint of the resolved policy pair, stamped on
//     posted journal entries as immutable audit evidence.
//   - Account eligibility for posting: company scope, effective window, active
//     status, control-account prohibition, cost center and currency rules.
//
// Boundaries: the package never grants permissions, never reads the wall clock
// during domain decisions, and never persists policies. Soft/hard period close
// remains masterdata's responsibility; this package only supplies the explicit
// AllowSoftClosedPosting decision for the selected date.
package policy
