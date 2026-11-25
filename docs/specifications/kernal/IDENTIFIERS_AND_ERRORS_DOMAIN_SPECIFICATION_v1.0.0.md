# Identifiers and Core Errors Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Kernel entity identifiers, company-scope identity invariants, zero/nil identifier rejection, structured error classification, and retryability semantics.

---

## 1. Purpose

This document defines the externally observable domain contract for core identifiers and structured error classification across the accounting kernel. It is intended to be understandable and testable without access to a particular source-code implementation.

The specification defines:

- structural validity and non-zero invariants for kernel entity identifiers;
- company/tenant scope identifier rules;
- structured error coding and semantic error classification;
- retryability classification boundaries for host/adapter consumers;
- conformance and acceptance criteria.

This document does not prescribe a programming language, UUID library, database column type, HTTP status code mapping, or logging framework.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs foundational identifier validity and domain error semantics within the accounting kernel. Monetary values and currency codes belong to `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md`. Domain-specific validation rules for master data, journal entries, policies, posting, valuation, ledger projections, and audit chains belong to their respective kernel domain specifications. Transport-level error envelopes (HTTP/gRPC) and authentication/authorization error policies belong to host or application layers.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§3–6.
2. MUST produce the externally observable outcomes specified by the acceptance criteria.
3. MUST fail closed when a required identifier is zero, nil, empty, or malformed.
4. MUST NOT silently substitute a default or synthetic identifier when a required identifier is missing.
5. MUST preserve error-code and retryability semantics across domain boundaries without exposing internal storage driver errors.

### 2.2 Conformance evidence

Conformance can be demonstrated through externally observable tests, including:

- zero/nil and whitespace-only identifier rejection tests;
- uniqueness validation tests for line and entity identifiers;
- coded error inspection and unwrapping tests;
- retryability classification tests distinguishing transient conflicts from terminal domain rejections.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Zero / Nil Identifier:** An all-zero UUID (`00000000-0000-0000-0000-000000000000`) or equivalent empty/unset opaque identifier.
- **Company Scope Identifier (`CompanyID`):** The mandatory tenant/legal-entity boundary identifier that partitions accounting data and operations within the kernel.
- **Actor Identifier (`ActorID`):** A non-empty attribution identifier representing the caller or system principal recorded for auditability.
- **Coded Error:** A structured error carrying a stable machine-readable error code, human-readable message, and optional underlying cause.
- **Retryable Error:** A transient or concurrency-related failure where re-executing the operation under host retry policy (after safe state lookup where applicable) may succeed.
- **Terminal (Non-Retryable) Error:** A domain, validation, or conflict failure whose outcome will not change upon identical re-execution.

---

## 4. Identifier invariants

### ID-001 — Non-zero mandatory entity identifiers

Every mandatory entity identifier—including `CompanyID`, `JournalEntryID`, `EntryLineID`, `AccountID`, `JournalID`, `FiscalPeriodID`, and `FiscalYearID`—MUST be non-zero when required by an operation. Zero or nil identifiers MUST be rejected explicitly before any repository read or state mutation.

**Acceptance criteria**

- Submitting a zero `CompanyID` to any scoped kernel operation fails immediately with a company-scope error without invoking external readers.
- Submitting a zero `JournalEntryID` or zero `EntryLineID` fails identity validation before posting.

### ID-002 — Actor identifier attribution without authorization

Operations that record accountability—such as posting, reversal, or audited period reopening—MUST require a non-empty `ActorID` (after trimming leading and trailing whitespace). The kernel MUST treat `ActorID` strictly as audit attribution and MUST NOT interpret it as an authorization credential, role, or permission proof.

**Acceptance criteria**

- An empty or whitespace-only `ActorID` is rejected explicitly.
- A non-empty `ActorID` is preserved in the resulting record or audit event without performing role-based access control checks.

### ID-003 — Distinctness of sibling identifiers

Within a single aggregate (such as lines within a journal entry), sibling entity identifiers MUST be unique. Duplicate line identifiers within the same entry MUST fail validation explicitly.

**Acceptance criteria**

- An entry containing two lines with the same `EntryLineID` is rejected before policy evaluation or persistence delegation.

### ID-004 — Optional analytical dimension identifier discipline

Optional analytical dimension identifiers (such as `CostCenterID`, `ProjectID`, `SegmentID`, `BusinessUnitID`, or `PartnerID`) MUST be either completely unset (nil/absent) or a valid non-zero identifier belonging to the same `CompanyID`. Passing an explicit zero/nil UUID as a populated dimension reference MUST be rejected.

**Acceptance criteria**

- An unset optional dimension (`nil`) is accepted unless required by account or policy rules.
- A non-nil pointer or wrapper containing a zero UUID is rejected explicitly.

### ID-005 — Trimmed non-empty string keys

Opaque string keys required by kernel contracts (such as `IdempotencyKey`, `ReversalReason`, and `AccountCode`) MUST be validated after trimming leading and trailing whitespace. A string consisting solely of whitespace MUST be treated as empty and rejected.

**Acceptance criteria**

- `"   "` is rejected identically to `""` for `IdempotencyKey` and `ReversalReason`.

---

## 5. Error classification and retryability

### ERR-001 — Structured coded error representation

Domain and public-compatibility errors MAY be wrapped in a `CodedError` carrying a stable `Code`, a descriptive `Message`, and an optional wrapped `Cause`. Error inspection MUST support unwrapping so callers can match both semantic sentinel errors and structured error codes.

**Acceptance criteria**

- Wrapping a sentinel error in a `CodedError` preserves matchability against both the error code and the underlying sentinel cause.

### ERR-002 — Neutral boundary error translation

Adapter and storage implementations MUST translate storage-engine-specific or driver-specific errors (such as SQLSTATE codes) into neutral domain or SPI sentinel errors before crossing the kernel/host boundary.

**Acceptance criteria**

- Raw database constraint or driver error types are never exposed directly as kernel domain errors.

### ERR-003 — Retryability classification

Kernel and storage boundary errors MUST be deterministically classified as either **retryable** or **non-retryable (terminal)**:

- **Retryable:** `ErrIdempotencyInProgress`, `ErrIdempotencyStorageUnavailable`, `ErrConcurrentModification` / `ErrConcurrencyConflict`, `ErrDeadlockDetected`, `ErrSerializationFailure`, and deadline/transport timeouts (subject to safe idempotency lookup).
- **Non-retryable (terminal):** validation errors, currency/scope/identity mismatches, `ErrIdempotencyPayloadConflict`, `ErrEntryAlreadyReversed`, `ErrRevaluationAlreadyApplied`, `ErrAtomicityViolation`, and `ErrScopeViolation`.

**Acceptance criteria**

- `IsRetryable` returns `true` for transient concurrency/availability failures and `false` for payload conflicts, double reversals, and domain validation failures.

### ERR-004 — Fail-closed default on unknown errors

If an error does not explicitly implement retryability or match a known retryable sentinel, `IsRetryable` MUST return `false` (fail closed).

**Acceptance criteria**

- An unclassified or unexpected internal error is treated as non-retryable by default.

---

## 6. Error Contract

| Error Category | Trigger Condition | Detection Timing | Retryable |
| --- | --- | --- | --- |
| Missing/Zero Scope ID | `CompanyID` is zero/nil | Pre-read / entry point | No |
| Missing/Zero Entity ID | Required entity or line ID is zero/nil | Pre-validation / identity check | No |
| Duplicate Sibling ID | Duplicate `EntryLineID` in an entry | Identity validation | No |
| Empty Attribution/Key | `ActorID`, `IdempotencyKey`, or reason is blank after trim | Pre-validation | No |
| Invalid Optional Dimension | Present optional dimension holds zero UUID | Line/dimension validation | No |
| Transient Concurrency | Lock conflict, serialization failure, or in-progress idempotency | Persistence / execution | Yes |
| Terminal Conflict | Payload conflict, already reversed, scope violation | Persistence / execution | No |

---

## 7. References

### REF-001 — RFC 2119 & RFC 8174

- **Title:** Key words for use in RFCs to Indicate Requirement Levels; Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words.
- **Links:** <https://www.rfc-editor.org/rfc/rfc2119>, <https://www.rfc-editor.org/rfc/rfc8174>
- **Used for:** Normative keyword definitions (§1.1).

### REF-002 — RFC 9562 (formerly RFC 4122) — Universally Unique Identifiers (UUIDs)

- **Title:** Universally Unique IDentifiers (UUIDs).
- **Link:** <https://www.rfc-editor.org/rfc/rfc9562>
- **Used for:** Nil UUID semantics (`00000000-0000-0000-0000-000000000000`) and identifier uniqueness concepts (`ID-001`, `ID-003`, `ID-004`).

### REF-003 — Accountant Engine Kernel Architecture & Contracts

- **Internal sources:** `docs/KERNEL_ARCHITECTURE_MAP.md`, `docs/KERNEL_IDEMPOTENCY_CONTRACT.md`, `docs/KERNEL_LOCKING_CONTRACT.md`, `docs/ard/P3_01_masterdata_effective_dating.md`.
- **Used for:** Project-defined company scope, actor attribution boundary, and retryability classification (`ID-001–005`, `ERR-001–004`).

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| ID-001, ID-003, ID-004 | REF-002, REF-003 | UUID nil semantics and project scope invariants |
| ID-002, ID-005 | REF-003 | Project attribution and key-trimming policy |
| ERR-001–004 | REF-003 | Project error-handling and retryability contract |

---

## 8. Appendices

### Appendix A — Informative implementation guidance

- Identifier validation should occur at the earliest possible boundary before any external reader or storage call.
- `ActorID` should be propagated alongside correlation metadata for forensic traceability, never as a substitute for host-layer authorization.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| ID-001 | Non-zero mandatory entity IDs | PROJECT-DECISION | ADR P3-01, REF-002 | Negative unit tests |
| ID-002 | Actor attribution without RBAC | PROJECT-DECISION | AE-RULE-ACC-001 §7 | Unit & boundary tests |
| ID-003 | Unique sibling line IDs | DERIVED-REQUIREMENT | Posting flow step 1 | Duplicate line ID test |
| ID-004 | Optional dimension nil-or-valid rule | PROJECT-DECISION | AE-RULE-ACC-001 §2 | Dimension validation tests |
| ID-005 | Trimmed non-empty string keys | PROJECT-DECISION | Idempotency contract §2 | Whitespace input tests |
| ERR-001 | CodedError wrapping and unwrapping | PROJECT-DECISION | `apperrors` contract | Error unwrapping tests |
| ERR-002 | Neutral SPI error translation | BOUNDARY/ASSUMPTION | Posting TX contract §6 | Contract parity tests |
| ERR-003 | Deterministic retryability classification | PROJECT-DECISION | Idempotency/Locking contracts | Classification matrix tests |
| ERR-004 | Fail-closed default on unknown errors | DERIVED-REQUIREMENT | Fail-closed principle | Unknown error test |

---

## 9. Version history

### v1.0.0

- Initial standalone normative specification for kernel identifiers and core error classification.

