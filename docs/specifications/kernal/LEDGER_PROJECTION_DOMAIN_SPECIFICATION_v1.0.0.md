# Ledger Projection Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** GL source-of-truth projection boundary, deterministic total-order movement projection, functional-currency balance reconstruction, balance-reader query semantics, and year-end closing boundary.

---

## 1. Purpose

This document defines the externally observable domain contract for ledger projections and deterministic balance reconstruction within the accounting kernel. It establishes that the kernel's `ledger` domain owns deterministic read projections derived from posted journal entries—and public-API reference DTOs—without owning physical balance tables, movement storage, or financial reporting.

This document does not prescribe SQL views, materialized balance tables, database triggers, or financial statement formatting.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs deterministic movement projection, balance reconstruction, and balance-reader semantics. Canonical entry storage belongs to `JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.0.0.md` and host executors. Posting execution belongs to `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md`. Physical `account_balances` tables, triggers, trial-balance/income-statement/balance-sheet presentation, and multi-book/consolidation ledgers belong outside the kernel.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–7.
2. MUST derive all movement projections and reconstructed balances solely from `Posted` (and `Reversed`) journal entries.
3. MUST apply a deterministic total ordering independent of map iteration or wall-clock time.
4. MUST NOT store mutable physical balance state inside the kernel.
5. MUST enforce consistent `asOf` and empty-currency resolution semantics across balance-reader implementations.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- deterministic rebuild tests producing identical balances regardless of input slice order;
- parity tests comparing `RebuildAccountBalances` against host-stored balances;
- `BalanceReader` parity tests for `asOf` (`PostingDate` vs. `EffectiveDate` fallback) and empty currency resolution;
- structural reflection/drift tests ensuring `ledger` reference DTOs remain aligned with `masterdata`.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Posted Movement (`PostedMovement`):** A deterministic, read-only projection of a single `EntryLine` belonging to a `Posted` or `Reversed` `JournalEntry`.
- **Derived Balance:** An account balance computed by aggregating `FunctionalDebit - FunctionalCredit` across projected movements.
- **Total-Order Key:** The deterministic sort key `(EffectiveDate UTC, SequenceNumber, JournalEntryID)` for entries and `EntryLine.ID` for lines within an entry.
- **Balance Reader (`BalanceReader`):** The host-implemented query port for retrieving derived account balances as of a given timestamp and currency.

---

## 4. Source of truth and deterministic projection

### LDG-001 — Derived projection, not a second source of truth

`PostedMovement` projections and reconstructed balances MUST be derived strictly from `Posted` and `Reversed` `JournalEntry` aggregates. `Draft` and `Cancelled` entries MUST be excluded. The kernel MUST NOT maintain mutable balance storage or treat derived balances as an authoritative source of truth.

**Acceptance criteria**

- Excluding `Draft` and `Cancelled` entries while including `Posted` and `Reversed` entries produces the exact net impact (including linked reversals).

### LDG-002 — Deterministic total-order movement projection

`ProjectPostedMovements` MUST project lines in a deterministic total order independent of input slice ordering or map iteration:

1. Entries ordered by `(EffectiveDate UTC, SequenceNumber, JournalEntryID)`.
2. Lines within each entry ordered by `EntryLine.ID`.

**Acceptance criteria**

- Shuffling the input list of posted entries and lines produces the exact same output sequence of `PostedMovement` items.

### LDG-003 — Deterministic functional-currency balance reconstruction

`RebuildAccountBalances` MUST aggregate `FunctionalDebit - FunctionalCredit` per `(CompanyID, AccountID, FunctionalCurrency)` over the projected movements. Reversal entries MUST be applied at their own `EffectiveDate` without modifying the original entry's historical movement. Normal balance (`Debit` vs. `Credit`) MAY be applied only for display sign convention, never to alter underlying debit/credit totals.

**Acceptance criteria**

- Rebuilding balances from posted entries matches incremental host-maintained balances to the exact minor unit.

---

## 5. Balance reader and reference model semantics

### LDG-004 — `BalanceReader` `asOf` and currency resolution parity

Host `BalanceReader` implementations MUST apply consistent query semantics:

1. `asOf` MUST filter against the kernel-stamped `PostingDate` (falling back to `EffectiveDate`/`entry_date` only for legacy rows lacking `PostingDate`).
2. `Draft` and `Cancelled` entries MUST be excluded; `Posted` entries (and `Reversed` entries alongside their posted reversal entries) MUST be included so net balances are accurate.
3. An empty currency filter MUST resolve to the company's `FunctionalCurrency` (never an implicit "all currencies" wildcard), and aggregated amounts MUST be in `FunctionalCurrency` (`FunctionalDebit - FunctionalCredit`).

**Acceptance criteria**

- Querying `BalanceReader` with an empty currency returns the functional-currency balance, identical across in-memory and relational adapters.

### LDG-005 — Reference DTO parity without drift

Lightweight reference DTOs in `ledger` (used for public API compatibility) MUST remain structurally aligned with canonical `masterdata` types. Unmapped fields MUST be explicitly documented, and `ledger` DTOs MUST NOT evolve competing domain validation rules.

**Acceptance criteria**

- Automated reflection/parity checks fail if `masterdata` and `ledger` reference models drift silently.

---

## 6. Year-end closing boundary

### LDG-006 — Year-end closing via balanced closing entries

Year-end closing (zeroing `Revenue` and `Expense` accounts into `Retained Earnings` and carrying forward `Asset`, `Liability`, and `Equity` balances) MUST be expressed as balanced `Closing` / `Opening` journal entries processed through the central posting pipeline, preserving full auditability and immutability.

**Acceptance criteria**

- Year-end closing leaves all temporary (income-statement) accounts with zero net balance via posted `Closing` entries rather than direct balance mutation.

---

## 7. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| `ErrCompanyScopeRequired` | Zero `CompanyID` in projection or balance query | Pre-projection / query |
| Currency / Account Mismatch | Inconsistent currency or invalid entry in projection | Projection build |

---

## 8. References

### REF-001 — Accountant Engine GL Source-of-Truth & Balance ADRs

- **Internal sources:** `docs/ard/P1_02_gl_source_of_truth.md`, `docs/ard/P1_03_reference_model_duality.md`, `docs/ard/P11_03_balance_reader_semantics.md`, `docs/KERNEL_ARCHITECTURE_MAP.md` (§3), `docs/ACCOUNTING_CAPABILITIES_INVENTORY.md` (`CAP-KERNAL-07`).
- **Used for:** GL source of truth, total-order projection, `BalanceReader` parity, and year-end closing boundary (`LDG-001–006`).

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| LDG-001–003 | REF-001 (ADR P1-02) | Canonical GL truth and deterministic rebuild |
| LDG-004 | REF-001 (ADR P11-03) | Cross-adapter `BalanceReader` semantics |
| LDG-005 | REF-001 (ADR P1-03) | Reference DTO duality |
| LDG-006 | REF-001 (`CAP-KERNAL-07`) | Year-end closing via posted entries |

---

## 9. Appendices

### Appendix A — Informative implementation guidance

- Because `RebuildAccountBalances` is deterministic, host `account_balances` caches or triggers can always be truncated and rebuilt from `account_moves` and `account_move_lines` without data loss.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| LDG-001 | Derived projection, not second truth | PROJECT-DECISION | ADR P1-02 | Projection unit tests |
| LDG-002 | Total-order movement projection | PROJECT-DECISION | ADR P1-02 | Order-permutation tests |
| LDG-003 | Functional-currency balance rebuild | PROJECT-DECISION | ADR P1-02 | Rebuild parity tests |
| LDG-004 | `BalanceReader` `asOf` & currency parity | PROJECT-DECISION | ADR P11-03 | `TestPhase11_BalanceReaderAsOfAndCurrencyParity` |
| LDG-005 | Reference DTO parity without drift | PROJECT-DECISION | ADR P1-03 / P3-01 | `TestReferenceModelsDoNotDriftSilently` |
| LDG-006 | Year-end closing via posted entries | PROJECT-DECISION | `CAP-KERNAL-07` | Year-end closing tests |

---

## 10. Version history

### v1.0.0

- Initial standalone normative specification for the kernel ledger projection domain.

