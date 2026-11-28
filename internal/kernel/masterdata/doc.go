// Package masterdata: Chart of accounts, fiscal calendar and tenant company scope.
//
// Authority: this package is the only kernel source for company scope, COA
// hierarchy and account eligibility, journals, cost centers, analytical
// dimensions, and the fiscal calendar including periods 13 and 14. Reference
// DTOs in internal/kernel/ledger are not a source of truth (ADR P1-03).
//
// Scope contract: every read must carry a trusted CompanyID. A zero company
// scope fails closed before repository access, and account, journal, period and
// hierarchy identities are checked against that scope (RULE-KERNEL-016).
// Effective dating: posting passes an explicit date; adapters that implement
// EffectiveAccountReader supply the version effective on that date, so later
// master-data edits cannot silently reinterpret posted history (ADR P3-01).
//
// Import charter: Go standard library, sibling internal/kernel/* modules, uuid and decimal only.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package masterdata
