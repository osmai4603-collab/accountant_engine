// Package ledger defines the foundational domain models for chart-of-accounts and fiscal periods
// referenced across engine layers and API contracts (e.g., pkg/api/v1/ledger).
//
// Architectural Boundaries (DEC-05 / RULE-KERNEL-023 / TARGET_ACCOUNTING_KERNAL):
//
//  1. General Ledger & Reporting Exclusion:
//     The kernel is strictly responsible for double-entry invariants, transaction validation,
//     normalization, and cryptographic audit chaining. Materialized balances, Trial Balance,
//     Balance Sheets, P&L, Cash Flow, and financial reporting belong to internal/reporting and
//     the host storage layer. The ledger package must not contain SetBalance, balance calculators,
//     or reporting queries.
//
// 2. Domain Models vs. Master Data:
//   - ledger.Account vs. masterdata.Account:
//     masterdata.Account is the authoritative entity for Chart of Accounts governance,
//     enforcing lifecycle dates (EffectiveFrom/EffectiveTo), control account flags,
//     cash-equivalence rules, contra-account relationships, and parent-child hierarchy validation.
//     ledger.Account represents a lightweight domain transfer structure.
//   - ledger.FiscalPeriod vs. masterdata.FiscalPeriod:
//     masterdata.FiscalPeriod is the authoritative fiscal calendar model, managed by
//     masterdata.GenerateFiscalYear and validated with strict contiguity and containment rules.
//     ledger.FiscalPeriod is the corresponding lightweight reference representation.
//
// 3. Backward Compatibility & Aliases:
//   - AccountTypeRevenue is a documented alias for AccountTypeIncome ("income") to support
//     callers and API schemas expecting revenue nomenclature. It must not be altered silently.
package ledger
