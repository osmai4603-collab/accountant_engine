// Package types: Exact decimal money, ISO 4217 currency, banker's rounding primitives,
// and shared kernel identifiers / idempotency scope contracts.
//
// Error ownership: types/errors.go defines only monetary/currency representation
// sentinel errors; domain errors live in their owning packages (masterdata, journal, posting).
//
// Import charter: May import only the Go standard library, uuid and decimal.
//
// Monetary charter (Phase 2, ADR P2-01): no float32/float64 or float-derived
// decimals enter any monetary path, including tests. Banker's rounding (half
// to even) is the only monetary rounding mode; MinorUnitsExact fails closed on
// sub-minor-unit fractions. RoundForPosting and CurrencyMinorUnit are retained
// as deprecated compatibility aliases and are used by kernel tests only;
// production code in this kernel must call the checked variants.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package types
