// Package valuation: Historical exchange rates and IAS 21 revaluation logic.
//
// Boundaries: valuation depends only on types, journal, masterdata, and abstract
// interfaces (RateProvider, EntryValidator, ReversalBuilder) with no direct
// dependency on posting.
//
// Import charter: Go standard library, sibling internal/kernal/* modules, uuid and decimal only.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package valuation
