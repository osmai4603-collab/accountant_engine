// Package postgres provides the sole production-certified storage adapter
// for the accountant engine, backing all core accounting capabilities with
// ACID guarantees, immutable ledger records, cryptographic audit chaining,
// and database-enforced row level security (RLS).
//
// Charter & Guarantees:
//   - Exclusive production adapter (GuaranteesDurability: true)
//   - Multi-tenant isolation enforced via PostgreSQL Row Level Security (RLS)
//   - WORM compliance: modifications or deletions to posted entries/lines and
//     financial audit logs are blocked at the database engine level via triggers.
//   - Numbered, checksum-validated schema migrations embedded via embed.FS.
//
// Baseline: AE-PLAN-STEP-P3 / P3-02, P3-03.
package postgres
