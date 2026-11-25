# Master Data Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Company scope, Chart of Accounts, account classification and hierarchy, journals, fiscal calendars, adjustment periods (13 and 14), analytical dimensions, and effective-dating contracts.

---

## 1. Purpose

This document defines the externally observable domain contract for accounting master data within the kernel. It governs the structural validity, company isolation, hierarchical integrity, fiscal calendar boundaries, and effective-dated lookup semantics for companies, accounts, journals, fiscal years/periods, cost centers, and analytical dimensions.

This document does not prescribe a programming language, database schema, caching mechanism, or adapter implementation.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs master data domain rules and read-port contracts within the accounting kernel. Posting pipeline execution belongs to `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md`. Policy selection and account eligibility engines belong to `ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.0.0.md`. Physical storage, SQL schemas, Row-Level Security (RLS), and master data CRUD APIs belong to the host/adapter layers.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–8.
2. MUST enforce company-scope isolation before any repository read.
3. MUST enforce closed/non-inverted effective-date ranges and acyclic hierarchies.
4. MUST treat adjustment periods 13 and 14 as procedural containers excluded from calendar date matching.
5. MUST fail closed when required master data or readers are missing, inactive, or mismatched.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- zero `CompanyID` fail-closed tests before reader access;
- COA hierarchy cycle-detection and cross-company rejection tests;
- fiscal calendar boundary, non-1st-of-month start, and period 13/14 `Contains(date)` tests;
- close-state matrix tests (`OPEN`, `SOFT_CLOSED`, `HARD_CLOSED`) for both period and year;
- effective-dating boundary tests using injected clocks and versioned account readers.

### 2.3 Partial conformance

An implementation that omits `EffectiveAccountReader` or `AdjustmentPeriodReader` MUST disclose that historical account versioning or adjustment periods 13/14 are unsupported, and MUST fail closed when those capabilities are invoked.

---

## 3. Terms and definitions

- **Chart of Accounts (COA):** The hierarchical catalog of accounts belonging to a single company.
- **Header Account:** A non-postable structural/grouping account (`IsHeader == true`) used for hierarchy and reporting aggregation.
- **Leaf (Postable) Account:** A non-header account (`IsHeader == false`) eligible to receive journal entry postings.
- **Control Account:** A postable account (`IsControlAccount == true`) reserved for specialized subledger integration and prohibited from general-ledger direct posting.
- **Fiscal Year:** A contiguous accounting period span for a company with explicit `StartDate`, `EndDate`, and close state.
- **Fiscal Period:** A numbered accounting period within a fiscal year. Regular periods are typically 1–12; adjustment periods are 13 and 14.
- **Adjustment Period (13 / 14):** Procedural year-end (13) or external-audit (14) period anchored to the fiscal year end date and selectable only by explicit target period number.
- **Effective-Dating:** Associating master data records with `[EffectiveFrom, EffectiveTo]` validity bounds so historical transactions are validated against the version active at their `EffectiveDate`.

---

## 4. Company scope and isolation

### MD-001 — Fail-closed company scope

Every master data validation or read path MUST reject a zero `CompanyID` with `ErrCompanyScopeRequired` before invoking any external reader or repository.

**Acceptance criteria**

- Passing a zero `CompanyID` to scope, account, journal, or period validation returns `ErrCompanyScopeRequired` without calling the reader.

### MD-002 — Mandatory functional currency on company

Every valid company record MUST define a valid, active ISO 4217 `FunctionalCurrency` (excluding `XXX`). A company missing a functional currency or configured with `XXX` MUST fail validation.

**Acceptance criteria**

- A company with empty or `XXX` functional currency is rejected before posting.

### MD-003 — Cross-company entity rejection

Any account, journal, fiscal year, fiscal period, cost center, or analytical dimension whose `CompanyID` does not match the target `CompanyID` MUST be rejected explicitly (e.g., `ErrAccountIdentityMismatch` or `ErrCompanyScopeRequired`).

**Acceptance criteria**

- An account belonging to Company B cannot be resolved or posted to in an operation scoped to Company A.

---

## 5. Chart of Accounts and hierarchies

### MD-004 — Five-element accounting classification

Every account MUST belong to exactly one of the five canonical account types with its standard normal balance:

1. `Asset` (Debit normal)
2. `Liability` (Credit normal)
3. `Equity` (Credit normal)
4. `Revenue` (Credit normal)
5. `Expense` (Debit normal)

**Acceptance criteria**

- An account with an unrecognized or empty account type fails validation.

### MD-005 — Header accounts are non-postable

Header accounts (`IsHeader == true`) MUST NOT accept postings. Posting validation MUST reject any line referencing a header account (`ErrHeaderAccountPosting`).

**Acceptance criteria**

- Attempting to validate or post to an account with `IsHeader == true` fails explicitly.

### MD-006 — Acyclic single-company hierarchy

A Chart of Accounts hierarchy and Cost Center hierarchy MUST be a forest/tree free of cycles (direct self-parenting or multi-hop cycles). Furthermore, a parent account or cost center MUST belong to the same `CompanyID` as its child; cross-company parent links MUST be rejected (`ErrAccountIdentityMismatch`).

**Acceptance criteria**

- A cycle `A -> B -> A` or `A -> A` is detected and rejected in both COA and Cost Center hierarchies.
- A COA snapshot mixing accounts from two different companies is rejected before tree construction.

### MD-007 — Account effective-dating range

Account validity windows `[EffectiveFrom, EffectiveTo]` MUST NOT be inverted (`EffectiveTo < EffectiveFrom` fails with `ErrInvalidEffectiveRange`). Both bounds are inclusive; `EffectiveTo == nil` indicates open-ended validity.

**Acceptance criteria**

- An account with `EffectiveTo` earlier than `EffectiveFrom` fails `Validate()`.
- An account queried at `asOf` outside `[EffectiveFrom, EffectiveTo]` fails with `ErrAccountNotCurrent`.

### MD-008 — Versioned account resolution without system clock

Domain validation MUST NOT read `time.Now()` directly; it MUST use the transaction's `EffectiveDate` (via `ValidateCanPostToAccountAsOf`) or an injected `Clock`. When the adapter implements `EffectiveAccountReader`, the kernel MUST resolve the account version via `GetAccountAsOf(ctx, companyID, accountID, asOf)`.

**Acceptance criteria**

- Historical posting uses the account version effective at `EffectiveDate`, not the current wall-clock state.

---

## 6. Journals and analytical dimensions

### MD-009 — Journal scope, currency, and allow-list enforcement

Every posting MUST reference a valid, active journal belonging to the same `CompanyID` and effective at `EffectiveDate`. If the journal defines currency or account allow-lists (non-empty), every line's transaction currency and account MUST be permitted by those lists; duplicate entries in allow-lists MUST be rejected. An empty allow-list imposes no restriction.

**Acceptance criteria**

- A journal from another company or outside its effective window fails validation.
- A line using a currency or account excluded from a non-empty journal allow-list fails with `ErrJournalCurrency` or `ErrJournalAccount`.

### MD-010 — Cost centers and analytical dimensions

When a line specifies a `CostCenterID` or other analytical dimension IDs, the corresponding repository MUST be available and MUST confirm that the dimension exists, belongs to the same `CompanyID`, and is active (`IsActive == true`) at the transaction date.

**Acceptance criteria**

- Specifying a dimension ID when its repository is nil fails with `ErrDimensionReaderRequired`.
- Specifying an inactive or cross-company dimension fails with `ErrInactiveFinancialDimension` or a scope mismatch error.

---

## 7. Fiscal calendars, close states, and adjustment periods

### MD-011 — Dual fiscal period and year validation

Every posting MUST fall within a valid, company-scoped `FiscalPeriod` and its parent `FiscalYear`. Validation MUST verify that:

1. `CompanyID`, `FiscalPeriodID`, and `FiscalYearID` are non-zero and mutually consistent.
2. The period's date range lies within the fiscal year's `[StartDate, EndDate]`.
3. For regular periods (1–12), `EffectiveDate` falls within the period's `[StartDate, EndDate]`.

**Acceptance criteria**

- A period belonging to a different year or company, or whose boundaries exceed its fiscal year, is rejected.

### MD-012 — Calendar continuity and non-1st-of-month fiscal years

Fiscal calendars MUST support fiscal years starting on any calendar day (not only the 1st of a month) without month-end drift, overlapping periods, or uncovered date gaps.

**Acceptance criteria**

- A fiscal year starting mid-month produces contiguous, non-overlapping periods covering the entire year without gaps.

### MD-013 — Close-state matrix (`OPEN`, `SOFT_CLOSED`, `HARD_CLOSED`)

Both `FiscalPeriod` and `FiscalYear` support three close states:

- `OPEN`: posting is permitted.
- `SOFT_CLOSED`: posting is rejected by default (`ErrClosedPeriod`) and permitted **only** when the effective `PostingPolicy` explicitly sets `AllowSoftClosedPosting == true`. No user-role or manager-approval check exists in the kernel.
- `HARD_CLOSED`: posting is unconditionally prohibited.

**Acceptance criteria**

- `HARD_CLOSED` on either the period or the year always rejects posting regardless of policy.
- `SOFT_CLOSED` rejects posting unless `AllowSoftClosedPosting == true` in the selected policy.

### MD-014 — Audited period reopening without kernel RBAC

Reopening a closed period within the kernel MUST record `ActorID` and a non-empty `Reason` for auditability. The kernel MUST NOT enforce RBAC or approval workflows for reopening; authorization belongs to the host.

**Acceptance criteria**

- Reopening without an `ActorID` or `Reason` is rejected.

### MD-015 — Adjustment periods 13 and 14 invariant

Adjustment periods 13 (year-end adjustments) and 14 (external audit adjustments) are procedural containers anchored to `FiscalYear.EndDate` rather than calendar date ranges:

1. `FiscalPeriod.Contains(date)` MUST always return `false` for periods 13 and 14.
2. Regular calendar-date period lookup MUST NOT select periods 13 or 14.
3. Periods 13 and 14 MUST ONLY be targeted by explicit `TargetPeriodNumber` (13 or 14) paired with compatible entry classification metadata.

**Acceptance criteria**

- Calling `Contains(date)` on period 13 or 14 returns `false` for all dates, including `FiscalYear.EndDate`.
- Posting to period 13 or 14 without explicit target period number and matching classification fails with `ErrInvalidAdjustmentPeriod`.

---

## 8. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| `ErrCompanyScopeRequired` | Zero `CompanyID` or cross-company mismatch | Before any reader call |
| `ErrAccountIdentityMismatch` | Cross-company account or parent link | Account/COA validation |
| `ErrHeaderAccountPosting` | Posting to account with `IsHeader == true` | Account eligibility/validation |
| `ErrInvalidEffectiveRange` | `EffectiveTo < EffectiveFrom` | Master data validation |
| `ErrAccountNotCurrent` | `asOf` outside account `[EffectiveFrom, EffectiveTo]` | Account validation |
| `ErrJournalReaderRequired` / `ErrInvalidJournal` | Missing journal reader, unknown journal, or out-of-range journal | Journal validation |
| `ErrJournalCurrency` / `ErrJournalAccount` | Line violates journal allow-list | Journal allow-list check |
| `ErrDimensionReaderRequired` / `ErrInactiveFinancialDimension` | Missing dimension reader or inactive/invalid dimension | Dimension validation |
| `ErrClosedPeriod` / `ErrPeriodNotFound` | Period/year closed or no matching period | Fiscal period validation |
| `ErrInvalidAdjustmentPeriod` | Invalid period 13/14 targeting or classification mismatch | Adjustment period validation |

---

## 9. References

### REF-001 — Accountant Engine Business Rules & Architecture

- **Internal sources:** `docs/ACCOUNTING_BUSINESS_RULES.md` (§§5–6), `docs/KERNEL_ARCHITECTURE_MAP.md`, `docs/ard/P3_01_masterdata_effective_dating.md`, `docs/ard/P1_03_reference_model_duality.md`.
- **Used for:** Company scope, COA hierarchy, effective-dating ports, fiscal calendar, and adjustment period invariants (`MD-001–015`).

### REF-002 — Conceptual Framework for Financial Reporting (IASB)

- **Title:** Conceptual Framework for Financial Reporting (2018), Chapter 4 (The Elements of Financial Statements).
- **Used for:** Five-element classification (Assets, Liabilities, Equity, Income/Revenue, Expenses) in `MD-004`.

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| MD-001–003 | REF-001 | Project multi-tenancy and fail-closed scope policy |
| MD-004 | REF-002 | Standard accounting element classification |
| MD-005–015 | REF-001 | Project COA, journal, dimension, and fiscal calendar rules |

---

## 10. Appendices

### Appendix A — Informative implementation guidance

- Host adapters should store effective-dated snapshots of accounts, journals, and periods so historical postings remain reproducible without mutating past interpretations.
- Reference DTOs in `ledger` exist for public API compatibility and must not drift from `masterdata` canonical models.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| MD-001 | Fail-closed zero CompanyID | PROJECT-DECISION | ADR P3-01 | `TestCompanyScopeFailsClosedWithoutRepositoryAccess` |
| MD-002 | Valid functional currency on company | PROJECT-DECISION | AE-RULE-ACC-001 §1 | Company validation tests |
| MD-003 | Cross-company entity rejection | PROJECT-DECISION | ADR P3-01 | Scope mismatch tests |
| MD-004 | Five-element account classification | SOURCE-MANDATED | REF-002 Ch. 4 | Account type validation tests |
| MD-005 | Header accounts non-postable | PROJECT-DECISION | AE-RULE-ACC-001 §5 | Header posting rejection tests |
| MD-006 | Acyclic single-company hierarchy | PROJECT-DECISION | ADR P3-01 | Cycle & cross-company tests |
| MD-007 | Non-inverted effective range | PROJECT-DECISION | ADR P3-01 | `FuzzEffectiveRangeValidationNeverPanics` |
| MD-008 | Injected clock & versioned reader | PROJECT-DECISION | ADR P3-01 | Effective-dating tests |
| MD-009 | Journal scope & allow-lists | PROJECT-DECISION | AE-RULE-ACC-001 §5 | Journal allow-list tests |
| MD-010 | Dimension reader & active check | PROJECT-DECISION | Posting flow step 13 | Dimension rejection tests |
| MD-011 | Dual period & year validation | PROJECT-DECISION | AE-RULE-ACC-001 §6 | Fiscal calendar matrix tests |
| MD-012 | Non-1st-of-month continuity | PROJECT-DECISION | AE-RULE-ACC-001 §6 | Calendar boundary tests |
| MD-013 | Close-state matrix | PROJECT-DECISION | ADR P3-01 / P5-01 | Close-state matrix tests |
| MD-014 | Audited reopening without RBAC | PROJECT-DECISION | AE-RULE-ACC-001 §6–7 | Reopen audit tests |
| MD-015 | Adjustment periods 13/14 invariant | PROJECT-DECISION | AE-RULE-ACC-001 §6 | Adjustment period tests |

---

## 11. Version history

### v1.0.0

- Initial standalone normative specification for the kernel master data domain.

