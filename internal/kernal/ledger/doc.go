// Package ledger defines lightweight reference models for chart-of-accounts and
// fiscal periods, used across engine layers and API contracts (pkg/api/v1/ledger),
// and provides deterministic read-only projections derived from canonical posted
// journal entries.
//
// Architectural Boundaries (ADR P1-01 / P1-02 / P1-03):
//
//  1. Canonical source of truth. Posted JournalEntry and EntryLine records owned
//     by journal and validated/posted by posting are the canonical accounting
//     facts. This package does not own a second stored ledger movement entity.
//     PostedMovement and balance rebuild helpers are pure projections derived from
//     posted entries; they hold no persistent state, no materialized balance
//     store, no SetBalance, and no reporting engine.
//
//  2. Domain models vs. master data. masterdata.Account is the authoritative
//     chart-of-accounts entity (lifecycle dates, control/contra/cash flags,
//     hierarchy rules); masterdata.FiscalPeriod is the authoritative calendar
//     entity (generation, contiguity, containment, close states). ledger.Account,
//     ledger.FiscalYear and ledger.FiscalPeriod are compatibility DTOs for public
//     API shapes — never use them for posting eligibility or fiscal validation.
//     Conversion rules are recorded in docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md.
//
//  3. Scope keys. All ledger models and projections carry CompanyID. Multi-book
//     / multi-ledger isolation (BookID / LedgerID) is governed by ADR P12-01
//     (docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md#adr-p12-01)
//     and propagated via storage.WithLedgerContext with "primary" as the
//     backward-compatible default book.
//
//  4. Backward compatibility. AccountTypeRevenue is a documented alias for
//     AccountTypeIncome ("income") retained for callers and public API schemas.
//     It must not be altered silently.
//
// Import charter: Go standard library and sibling kernel modules only.
package ledger
