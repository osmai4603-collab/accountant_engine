// Package postgres: Production PostgreSQL adapter implementing pkg/spi/storage with RLS and WORM policies.
//
// Import charter: May import pkg/api/v1/*, pkg/spi/* and pgx; never internal/core/*.
//
// Baseline: AE-PLAN-STEP-P0 / P0-02, NEW_PROJECT_STRUCTURE.md,
// ARCHITECTURAL_PRINCIPLES_CHARTER.md.
package postgres
