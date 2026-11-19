// Package storage: Abstract persistence ports (UnitOfWork, EntryStore, AccountStore and friends).
//
// Import charter: Depends only on pkg/api/v1/* and stdlib; no concrete pgx or database/sql code.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package storage
