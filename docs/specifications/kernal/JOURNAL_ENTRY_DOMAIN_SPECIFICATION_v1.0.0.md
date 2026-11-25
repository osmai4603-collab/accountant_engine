# Journal Entry Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Journal entry aggregate structure, entry lines, entry classifications, lifecycle states, reversal links, defensive cloning, and post-posting immutability.

---

## 1. Purpose

This document defines the externally observable domain contract for the `JournalEntry` aggregate and its constituent `EntryLine` items within the accounting kernel. It establishes the canonical accounting record structure, permitted lifecycle transitions, line-level structural invariants, and immutability guarantees.

This document does not prescribe a programming language, database table layout (`account_moves` / `account_move_lines`), JSON column encoding, or persistence adapter.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs the `JournalEntry` aggregate definition, line structure, lifecycle state machine, defensive copying, and immutability checks. Double-entry balancing, rounding settlement, policy evaluation, and the posting execution pipeline belong to `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md`. Monetary representation belongs to `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md`. Persistence triggers (WORM) and storage mapping belong to host adapters.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–7.
2. MUST enforce strict line-level structural invariants (single-sided, positive amounts, non-zero IDs).
3. MUST enforce the `Draft -> Posted -> Reversed` lifecycle and prohibit mutation or deletion of posted/reversed entries.
4. MUST provide deep defensive cloning so caller mutations after submission cannot alter validated kernel state.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- line-level negative, zero-sided, and two-sided rejection tests;
- lifecycle state transition and immutability rejection tests;
- deep-clone isolation tests covering all pointer fields and line slices;
- classification and reversal-link consistency tests.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Journal Entry:** The canonical accounting aggregate representing a balanced economic event, consisting of a header and two or more `EntryLine` items.
- **Entry Line:** An individual debit or credit leg referencing an account, monetary amounts (transaction and functional), currency, and optional analytical dimensions.
- **Entry Status:** The lifecycle state of a journal entry (`Draft`, `Posted`, `Reversed`, `Cancelled`).
- **Entry Classification:** The accounting purpose of the entry (e.g., `Standard`, `Opening`, `Adjusting`, `Closing`, `Reversing`).
- **Immutability:** The invariant that once an entry reaches `Posted` (or `Reversed`), its financial, structural, and audit-stamped fields cannot be modified or deleted.

---

## 4. Aggregate and line structure

### JRN-001 — Canonical source of truth

A posted `JournalEntry` (including `Reversed` entries) and its `EntryLine` items constitute the sole canonical source of accounting truth within the kernel. Derived balances, ledger projections, and audit logs are secondary projections or evidence and MUST NOT override or contradict posted journal entries.

**Acceptance criteria**

- Account balances and movement projections are deterministically derivable from posted journal entries alone.

### JRN-002 — Minimum entry composition

A valid `JournalEntry` MUST contain at least **two** `EntryLine` items, including at least one debit line and at least one credit line. Single-line, zero-line, debit-only, or credit-only entries MUST be rejected.

**Acceptance criteria**

- An entry with 0 or 1 lines, or 2 debit lines and 0 credit lines, fails structural validation.

### JRN-003 — Single-sided positive line invariant

Every `EntryLine` MUST be strictly single-sided:

- It MUST have either a strictly positive `FunctionalDebit` (`> 0`) with zero `FunctionalCredit`, **or** a strictly positive `FunctionalCredit` (`> 0`) with zero `FunctionalDebit`.
- Two-sided lines (both debit and credit non-zero) and zero-sided lines (both zero) MUST be rejected.
- `TransactionAmount` MUST be strictly positive (`> 0`). Negative amounts MUST NOT be used to reduce or correct accounts; corrections MUST use opposite-sided positive lines or a linked reversal entry.

**Acceptance criteria**

- A line with negative `TransactionAmount`, zero debit and zero credit, or both debit and credit populated is rejected explicitly.

### JRN-004 — Line currency consistency

On every `EntryLine`, the currency of `TransactionAmount` MUST equal `TransactionCurrency`, and the currency of `FunctionalDebit` and `FunctionalCredit` MUST equal the company's `FunctionalCurrency`.

**Acceptance criteria**

- A mismatch between `TransactionAmount.Currency()` and `TransactionCurrency` fails with `ErrTransactionCurrencyMismatch`.
- A mismatch between `FunctionalDebit`/`FunctionalCredit` currency and `Company.FunctionalCurrency` fails with `ErrFunctionalCurrencyMismatch`.

### JRN-005 — Mandatory entry and line metadata

Every `JournalEntry` submitted for posting MUST have:

- a non-zero `ID`, `CompanyID`, and `JournalID`;
- a non-zero `EffectiveDate`;
- unique, non-zero `ID` and non-zero `AccountID` on every `EntryLine`.

**Acceptance criteria**

- Missing `ID`, `JournalID`, `EffectiveDate`, or `AccountID` fails validation before any persistence delegation.

---

## 5. Classifications, lifecycle, and immutability

### JRN-006 — Entry classification metadata integrity

Every `JournalEntry` MUST carry a valid `Classification`. Specialized classifications MUST satisfy their structural metadata requirements:

- `Reversing` entries MUST specify a non-zero `ReversalOfID` and a non-empty trimmed `ReversalReason`.
- `Adjusting` entries targeting adjustment periods MUST carry valid adjusting metadata (e.g., `audit_adjustment` for period 14; `adjusting`/`closing` for period 13).
- Legacy aliases (such as legacy audit adjustment labels) MAY be normalized only after defensive cloning.

**Acceptance criteria**

- A `Reversing` entry missing `ReversalOfID` or `ReversalReason` fails with `ErrInvalidReversal` or `ErrReversalReasonRequired`.
- An `Adjusting` entry missing required metadata fails with `ErrAdjustingEntryMetadataRequired`.

### JRN-007 — Strict lifecycle state machine

A `JournalEntry` MUST enter the posting pipeline exclusively in `Draft` status. Permitted lifecycle transitions are:

- `Draft -> Posted` (only via the central posting pipeline)
- `Posted -> Reversed` (only via atomic linked reversal)
- `Draft -> Cancelled` (pre-posting cancellation)

Posting an entry in `Posted`, `Reversed`, or `Cancelled` status MUST fail with `ErrInvalidEntryState`. A `Draft` entry carrying a pre-existing `ReversedByID` MUST be rejected with `ErrInvalidReversal`.

**Acceptance criteria**

- Attempting to post an already `Posted`, `Reversed`, or `Cancelled` entry fails with `ErrInvalidEntryState`.
- A draft with `ReversedByID != nil` is rejected.

### JRN-008 — Post-posting immutability

Once a `JournalEntry` is `Posted` or `Reversed`, it MUST be immutable. Any attempt to modify its header, lines, amounts, dates, `PolicyFingerprint`, or audit stamps via domain mutation checks MUST fail with `ErrCannotModifyPostedMove`. Corrections MUST be performed solely by creating a new linked reversal or adjusting entry.

**Acceptance criteria**

- Calling `EnsureImmutable` on a `Posted` or `Reversed` entry returns `ErrCannotModifyPostedMove`.

### JRN-009 — Deep defensive cloning

`JournalEntry.Clone()` MUST produce a deep copy of the aggregate, including its line slice and all pointer fields (such as `ReversalOfID`, `ReversedByID`, `FiscalPeriodID`, `PostedAt`, `PostingDate`, and optional dimension IDs). Mutating the original caller-owned struct after `Clone()` MUST NOT affect the cloned snapshot.

**Acceptance criteria**

- Modifying a line amount, pointer field, or `CompanyID` on the caller's original `JournalEntry` after `Clone()` leaves the cloned entry unchanged.

---

## 6. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| `ErrMissingEntryID` / `ErrMissingLineID` / `ErrDuplicateLineID` | Zero entry/line ID or duplicate line ID | Identity validation |
| `ErrMissingJournalID` / `ErrZeroEntryDate` | Zero `JournalID` or zero `EffectiveDate` | Header validation |
| `ErrInvalidEntryState` | Entry is not in `Draft` status | Lifecycle check |
| `ErrCannotModifyPostedMove` | Attempted mutation of `Posted`/`Reversed` entry | Immutability check |
| `ErrInvalidEntry` / `ErrUnbalancedEntry` | Fewer than 2 lines, missing debit/credit, or invalid line | Structural validation |
| `ErrInvalidReversal` / `ErrReversalReasonRequired` | Invalid reversal links, `ReversedByID` on draft, or blank reason | Classification validation |
| `ErrAdjustingEntryMetadataRequired` | Missing metadata for adjusting entry | Classification validation |
| `ErrTransactionCurrencyMismatch` / `ErrFunctionalCurrencyMismatch` | Line currency inconsistency | Currency validation |

---

## 7. References

### REF-001 — Accountant Engine Architecture & Business Rules

- **Internal sources:** `docs/ACCOUNTING_BUSINESS_RULES.md` (§§1–2, §7), `docs/KERNEL_POSTING_FLOW_SPECIFICATION.md` (§§3–5), `docs/ard/P1_02_gl_source_of_truth.md`, `docs/ard/P5_01_policy_selection_and_historical_interpretation.md`.
- **Used for:** Canonical GL truth, line invariants, lifecycle state machine, defensive cloning, and immutability (`JRN-001–009`).

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| JRN-001 | REF-001 (ADR P1-02) | Project architectural decision on GL source of truth |
| JRN-002–005 | REF-001 (AE-RULE-ACC-001) | Double-entry and line structural invariants |
| JRN-006–009 | REF-001 (AE-POST-FLOW-V12) | Lifecycle, immutability, and defensive cloning |

---

## 8. Appendices

### Appendix A — Informative implementation guidance

- Host adapters storing `JournalEntry` in relational + JSON representations (e.g., `account_moves` + `account_move_lines`) must preserve `ReversalOfID` and `ReversedByID` without introducing conflicting truth models.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| JRN-001 | Posted entries as canonical GL truth | PROJECT-DECISION | ADR P1-02 | Projection parity tests |
| JRN-002 | Minimum 2 lines with debit and credit | SOURCE-MANDATED | Double-entry accounting / AE-RULE-ACC-001 | Structural validation tests |
| JRN-003 | Single-sided positive line amounts | PROJECT-DECISION | AE-RULE-ACC-001 §2 | Line amount tests |
| JRN-004 | Line currency consistency | PROJECT-DECISION | AE-RULE-ACC-001 §1 | Currency mismatch tests |
| JRN-005 | Mandatory header and line IDs | PROJECT-DECISION | Posting flow step 1, 3 | Identity validation tests |
| JRN-006 | Classification metadata integrity | PROJECT-DECISION | Posting flow step 2 | Classification tests |
| JRN-007 | Strict Draft-to-Posted lifecycle | PROJECT-DECISION | ADR P1-02 | Lifecycle state tests |
| JRN-008 | Post-posting immutability | PROJECT-DECISION | AE-RULE-ACC-001 §7 | `EnsureImmutable` tests |
| JRN-009 | Deep defensive cloning | DERIVED-REQUIREMENT | TOCTOU protection (Posting flow §4) | `TestCloneIsDeepAndCoversPointerFields` |

---

## 9. Version history

### v1.0.0

- Initial standalone normative specification for the kernel journal entry domain.

