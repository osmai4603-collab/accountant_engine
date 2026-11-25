# Accounting Policy Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Company-scoped effective-dated posting and currency policies, non-overlapping policy registration, account and dimension eligibility rules, and canonical policy fingerprinting.

---

## 1. Purpose

This document defines the externally observable domain contract for accounting policies and eligibility evaluation within the kernel. It specifies how `PostingPolicy` and `CurrencyPolicy` versions are registered, selected deterministically for a given `(CompanyID, EffectiveDate)`, validated against overlaps or missing coverage, and fingerprinted onto posted entries for historical auditability.

This document does not prescribe a programming language, policy storage backend, or user-facing policy administration UI.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs policy versioning, deterministic policy resolution, account/dimension eligibility checks, and canonical policy fingerprinting within the kernel. Fiscal period close-state ownership belongs to `MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md`. Rounding arithmetic belongs to `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md`, while entry-level rounding settlement execution belongs to `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md`. Role-based access control (RBAC) and approval workflows belong outside the kernel.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–7.
2. MUST enforce half-open effective-date windows `[EffectiveDate, EffectiveTo)` per company.
3. MUST reject overlapping or duplicate policy effective dates upon registration.
4. MUST fail closed when no policy covers `(CompanyID, EffectiveDate)`.
5. MUST produce deterministic, order-independent canonical `PolicyFingerprint` digests.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- effective-date boundary tests (`start <= asOf < end`, adjacent windows, and uncovered gaps);
- overlap and duplicate registration rejection tests;
- fail-closed resolution tests when no policy is registered;
- account eligibility decision-matrix tests (active, effective-dated, control account, cost center, currency);
- canonical fingerprint stability and permutation-invariance tests;
- concurrent read/registration safety tests.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Posting Policy:** A company-scoped, effective-dated rule set governing posting constraints such as `AllowSoftClosedPosting`, mandatory dimension requirements, and account-type/posting rules.
- **Currency Policy:** A company-scoped, effective-dated rule set governing `FunctionalCurrency`, allowed transaction currencies, `AutoSettleRoundingDifferences`, and `RoundingDifferenceAccountID`.
- **Half-Open Effective Window (`[EffectiveDate, EffectiveTo)`):** An interval inclusive of `EffectiveDate` (start) and exclusive of `EffectiveTo` (end); `EffectiveTo == nil` denotes open-ended validity until a subsequent version or indefinitely.
- **Policy Fingerprint:** A canonical `sha256:<hex>` digest of the resolved `PostingPolicy` and `CurrencyPolicy` fields stamped onto a posted `JournalEntry` for historical reproducibility.

---

## 4. Policy versioning, selection, and isolation

### POL-001 — Company-scoped policy isolation

All `PostingPolicy` and `CurrencyPolicy` registrations and lookups MUST be strictly scoped by a non-zero `CompanyID`. A policy registered for Company A MUST NEVER be resolved for Company B.

**Acceptance criteria**

- Querying a policy with zero `CompanyID` or for a company with no registered policies fails closed (`ErrCompanyScopeRequired` or `ErrPolicyNotFound`).

### POL-002 — Half-open effective window `[EffectiveDate, EffectiveTo)`

Policy validity windows MUST be evaluated as half-open intervals `[EffectiveDate, EffectiveTo)` in UTC:

- A policy matches `asOf` if and only if `EffectiveDate <= asOf` and (`EffectiveTo == nil` or `asOf < EffectiveTo`).
- Adjacent windows where `prev.EffectiveTo == next.EffectiveDate` MUST be permitted without overlap.
- Uncovered gaps between windows MUST be permitted at registration and MUST fail closed with `ErrPolicyNotFound` when queried.

**Acceptance criteria**

- At `asOf == EffectiveTo`, the prior policy is no longer selected and the adjacent successor (if any) is selected.
- Querying a date inside a gap returns `ErrPolicyNotFound`.

### POL-003 — Non-overlapping deterministic registration

For a given `CompanyID` and policy kind, registering a policy whose `EffectiveDate` duplicates an existing version or whose `[EffectiveDate, EffectiveTo)` window overlaps any existing version MUST be rejected with `ErrPolicyOverlap`. Concurrent registrations for the same effective start MUST yield at most one winner.

**Acceptance criteria**

- Registering two policies with identical `(CompanyID, EffectiveDate)` or overlapping ranges returns `ErrPolicyOverlap`.

### POL-004 — Fail-closed absence of default policy

The kernel MUST NOT synthesize a default or fallback `PostingPolicy` or `CurrencyPolicy` when none is registered for `(CompanyID, EffectiveDate)`. Absence MUST return `ErrPolicyNotFound`.

**Acceptance criteria**

- Posting without registered policies fails with `ErrPolicyNotFound` before any executor call.

### POL-005 — Defensive isolation of stored policies

Policy registration and lookup MUST defensively copy mutable slices and pointer fields (such as `AllowedCurrencies` and `EffectiveTo`) so caller mutations after registration or resolution cannot corrupt internal policy state.

**Acceptance criteria**

- Mutating the caller's `AllowedCurrencies` slice after registering a `CurrencyPolicy` does not alter subsequent policy evaluations.

---

## 5. Account, currency, and dimension eligibility

### POL-006 — Currency policy enforcement

Every line's `TransactionCurrency` MUST be permitted by the resolved `CurrencyPolicy`. If `AutoSettleRoundingDifferences` is enabled, `RoundingDifferenceAccountID` MUST be non-zero and eligible for posting; otherwise, rounding settlement MUST NOT proceed.

**Acceptance criteria**

- A transaction currency disallowed by `CurrencyPolicy` fails with `ErrCurrencyNotAllowed`.
- Enabling `AutoSettleRoundingDifferences` without a valid `RoundingDifferenceAccountID` fails rounding validation.

### POL-007 — Account posting eligibility

`CheckAccountEligibility` MUST evaluate the effective-dated account snapshot resolved at `entry.EffectiveDate` and MUST reject posting if any of the following hold:

1. The account belongs to a different `CompanyID` than the entry.
2. The account is not effective at `entry.EffectiveDate` (`ErrAccountNotEffective`).
3. The account is inactive (`IsActive == false`) or suspended.
4. The account is a header account (`IsHeader == true`).
5. The account is a control account (`IsControlAccount == true`) (`ErrControlAccountNotPostable`).
6. The account is restricted to a specific currency that does not match the line's currency.
7. The account specifies `RequiresCostCenter == true` (or policy requires cost centers) and the line lacks a valid `CostCenterID`.

**Acceptance criteria**

- A control account, inactive account, or out-of-window account is rejected explicitly even if the entry is balanced.
- Account-level `RequiresCostCenter` is enforced even when company-wide policy does not mandate cost centers globally.

### POL-008 — Soft-close policy delegation

The `PostingPolicy.AllowSoftClosedPosting` flag governs whether a `SOFT_CLOSED` fiscal period is accepted during posting. The policy engine MUST NOT enforce user roles, manager approvals, or `HARD_CLOSED` overrides; period close-state enforcement remains owned by `masterdata`.

**Acceptance criteria**

- `AllowSoftClosedPosting == true` permits posting into a `SOFT_CLOSED` period, whereas `HARD_CLOSED` remains unconditionally rejected.

---

## 6. Canonical policy fingerprinting

### POL-009 — Deterministic content-bound `PolicyFingerprint`

Upon successful validation, the posting pipeline MUST compute and stamp `PolicyFingerprint` (`sha256:<hex>`) onto the frozen `JournalEntry` from the canonical representation of the resolved `PostingPolicy` and `CurrencyPolicy` fields (company, effective dates, limits, rounding settings, allowed currencies, dimension rules, soft-close flag).

- The digest MUST be independent of slice ordering (e.g., `AllowedCurrencies` MUST be sorted canonically before hashing).
- `PolicyFingerprint` is audit evidence only and MUST be covered by post-posting immutability checks.

**Acceptance criteria**

- Reordering `AllowedCurrencies` produces the exact same `PolicyFingerprint`.
- Changing any semantic policy field produces a distinct `PolicyFingerprint`.

---

## 7. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| `ErrPolicyNotFound` | No policy covers `(CompanyID, EffectiveDate)` | Policy resolution |
| `ErrPolicyOverlap` | Duplicate `EffectiveDate` or overlapping window | Policy registration |
| `ErrCurrencyNotAllowed` | Line currency excluded by `CurrencyPolicy` | Line currency check |
| `ErrAccountNotEffective` | Account not effective at `entry.EffectiveDate` | Account eligibility |
| `ErrControlAccountNotPostable` | Direct posting to `IsControlAccount == true` | Account eligibility |
| `ErrCostCenterRequired` | Missing `CostCenterID` when required by account/policy | Account eligibility |

---

## 8. References

### REF-001 — Accountant Engine Policy ADR & Rules

- **Internal sources:** `docs/ard/P5_01_policy_selection_and_historical_interpretation.md`, `docs/ard/P11_04_versioned_policy_store.md`, `docs/ACCOUNTING_BUSINESS_RULES.md` (§5), `docs/KERNEL_POSTING_FLOW_SPECIFICATION.md` (§3).
- **Used for:** Half-open effective windows, overlap rejection, fail-closed resolution, eligibility matrix, and canonical policy fingerprinting (`POL-001–009`).

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| POL-001–005 | REF-001 (ADR P5-01, P11-04) | Project policy versioning and isolation rules |
| POL-006–008 | REF-001 (AE-RULE-ACC-001) | Project eligibility and soft-close delegation |
| POL-009 | REF-001 (ADR P5-01) | Project historical auditability fingerprint |

---

## 9. Appendices

### Appendix A — Informative implementation guidance

- Host adapters implementing `storage.PolicyStore` should persist policy versions in append-only, non-overlapping history tables so historical `PolicyFingerprint` values remain verifiable after process restarts.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| POL-001 | Company-scoped policy isolation | PROJECT-DECISION | ADR P5-01 | `TestPolicySelectionIsScopedAndFailsClosed` |
| POL-002 | Half-open `[EffectiveDate, EffectiveTo)` window | PROJECT-DECISION | ADR P5-01 | `TestPolicyEffectiveDateBoundaryMatrix` |
| POL-003 | Non-overlapping registration | PROJECT-DECISION | ADR P5-01 | `TestPolicyEngineRejectsUnresolvedOverlaps` |
| POL-004 | Fail-closed when no policy exists | PROJECT-DECISION | ADR P5-01 | `TestPostEntryFailsClosedWithoutRegisteredPolicies` |
| POL-005 | Defensive copy of stored policies | DERIVED-REQUIREMENT | Isolation invariant | `TestStoredPoliciesAreIsolatedFromSharedSlicesAndPointers` |
| POL-006 | Currency policy & rounding account check | PROJECT-DECISION | AE-RULE-ACC-001 §3–4 | `TestCurrencyPolicyValidationAndRoundingAudit` |
| POL-007 | Account eligibility matrix | PROJECT-DECISION | ADR P5-01 / AE-RULE-ACC-001 | `TestAccountEligibilityDecisionMatrix` |
| POL-008 | Soft-close delegation to policy | PROJECT-DECISION | ADR P5-01 | `TestSelectedPostingPolicyControlsSoftCloseOnly` |
| POL-009 | Canonical `PolicyFingerprint` | PROJECT-DECISION | ADR P5-01 | `TestPolicyFingerprintIsCanonicalAndContentBound` |

---

## 10. Version history

### v1.0.0

- Initial standalone normative specification for the kernel accounting policy domain.

