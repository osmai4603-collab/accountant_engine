# Posting and Reversal Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Central posting pipeline, double-entry validation, entry-level rounding settlement, semantic reversal construction, TOCTOU protection, idempotency scope and request fingerprinting, deterministic lock-key ordering, and host-executor delegation boundary.

---

## 1. Purpose

This document defines the externally observable domain contract for posting and reversing journal entries within the accounting kernel. It establishes `PostEntry` as the single authoritative validation and decision path that transforms a `Draft` journal entry into a frozen, stamped `Posted` snapshot ready for atomic persistence by a host executor.

This document does not prescribe SQL statements, database transactions, row-level locking primitives, HTTP endpoints, or authorization policies.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs posting validation, rounding settlement, reversal construction, idempotency scope/fingerprint derivation, and deterministic lock-key ordering inside the kernel. Aggregate structure belongs to `JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.0.0.md`, master data to `MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md`, policy rules to `ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.0.0.md`, and primitive monetary arithmetic to `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md`. Physical database transactions, `SELECT ... FOR UPDATE`, advisory locks, and retry loops belong to the host/adapter layers.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–8.
2. MUST reject any invalid entry before invoking `PostingExecutor` or `PostingReversalExecutor`.
3. MUST enforce exact double-entry balance ($\sum \text{FunctionalDebit} = \sum \text{FunctionalCredit}$) after minor-unit normalization and explicit rounding settlement.
4. MUST protect against TOCTOU mutations via pre-read defensive cloning, single-read effective account snapshots, and injected clocks.
5. MUST stamp authoritative `IdempotencyScope`, `RequestFingerprint`, and `CallerFingerprint` before delegating to the executor.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- rejection-matrix tests verifying zero executor invocations on any validation, policy, or reader failure;
- rounding-settlement tests (with and without `AutoSettleRoundingDifferences`);
- TOCTOU tests mutating caller structs after submission;
- reversal lifecycle, period-resolution, and zero-net-impact fuzz/property tests;
- canonical idempotency scope and fingerprint stability tests;
- deterministic lock-key ordering and company-isolation tests.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Posting Pipeline:** The single central entry point (`PostEntry`) that validates, normalizes, stamps, and delegates a journal entry for persistence.
- **Rounding Residual:** The difference $\sum \text{Normalized Functional Debit} - \sum \text{Normalized Functional Credit}$ after normalizing all lines to currency minor units.
- **Semantic Reversal:** A new linked `Reversing` journal entry with `FunctionalDebit` and `FunctionalCredit` swapped relative to a `Posted` original entry, leaving the original immutable.
- **Idempotency Scope:** The composite operation identity `company=<CompanyID>|operation=<post|reverse|revaluation>|key=<TrimmedKey>`.
- **Request Fingerprint:** A canonical `sha256:<hex>` digest over the frozen, kernel-stamped entry snapshot.
- **Caller Fingerprint:** A canonical `sha256:<hex>` digest over the caller-supplied payload fields prior to kernel stamping.
- **Ordered Lock Keys:** A deduplicated, lexicographically sorted list of company-scoped lock target keys derived deterministically from entry and account IDs.

---

## 4. Central posting pipeline and double-entry invariants

### PST-001 — Single central posting path and fail-closed dependencies

All journal entry postings and reversals MUST pass through the central posting pipeline. Missing pipeline dependencies (`context`, `reader`, `policyEngine`, `clock`, or the required `PostingExecutor`/`PostingReversalExecutor`) MUST fail immediately before any validation or reader access.

**Acceptance criteria**

- Invoking `PostEntry` with a nil reader, nil policy engine, nil clock, or nil executor fails immediately.

### PST-002 — Fail-before-delegate invariant

Any failure in identity validation, lifecycle state, immutability, company scope, policy resolution, currency/journal/period/account/dimension validation, rounding settlement, or double-entry balance MUST abort the pipeline immediately. `PostingExecutor` and `PostingReversalExecutor` MUST NEVER be invoked for rejected inputs.

**Acceptance criteria**

- Across all negative/rejection test cases, executor invocation count is strictly zero.

### PST-003 — Strict double-entry balance

After minor-unit normalization and any permitted rounding settlement, the sum of `FunctionalDebit` across all lines MUST equal the sum of `FunctionalCredit` across all lines to the exact minor unit ($\sum D - \sum C = 0$). Unbalanced entries MUST fail with `ErrUnbalancedEntry`.

**Acceptance criteria**

- An entry with a 1-minor-unit imbalance (without permitted rounding settlement) fails with `ErrUnbalancedEntry`.

### PST-004 — Explicit minor-unit normalization and rounding settlement

Within the posting pipeline:

1. Each line's `TransactionAmount` MUST be normalized to its transaction currency's minor unit, and its functional amount MUST be recomputed from the normalized transaction amount and exchange rate and normalized to the company's `FunctionalCurrency` minor unit.
2. If the resulting rounding residual is non-zero, the pipeline MUST ONLY append a balancing rounding-difference line when the effective `CurrencyPolicy` enables `AutoSettleRoundingDifferences` and specifies a valid, eligible `RoundingDifferenceAccountID`.
3. Silent truncation or dropping of rounding residuals is prohibited. If settlement is disabled or the rounding account is missing/ineligible, the entry MUST fail (`ErrRoundingAccountRequired` / `ErrRoundingPolicyRequired`).

**Acceptance criteria**

- Sub-minor differences are settled via an explicit balancing line to `RoundingDifferenceAccountID` when policy permits, or rejected explicitly when policy forbids.

---

## 5. TOCTOU protection, clock discipline, and reversal construction

### PST-005 — Pre-read defensive cloning and frozen snapshot execution

The pipeline MUST validate caller-local invariants (identity, classification metadata, `Draft` status, non-zero `JournalID`, non-empty `ActorID` and `IdempotencyKey`, immutability, non-zero `EffectiveDate`) and then execute `entry = entry.Clone()` **before** performing any external reader call, rounding mutation, `FiscalPeriodID` stamping, or audit stamping. The executor MUST receive only the frozen cloned snapshot.

**Acceptance criteria**

- Mutating the caller's entry object concurrently or after submission cannot alter the snapshot passed to the executor.

### PST-006 — Single-read effective account snapshot

For each line, the pipeline MUST resolve the postable account once at `entry.EffectiveDate` via `ResolvePostableAccountAsOf` and pass that exact snapshot to `CheckAccountEligibility` and final policy validation. A second `GetAccount` read MUST NOT occur after eligibility validation.

**Acceptance criteria**

- Account eligibility and final posting policy evaluate the exact same account version returned by `ResolvePostableAccountAsOf`.

### PST-007 — Injected clock discipline

Production posting logic MUST obtain current time exclusively through an injected `Clock` abstraction; direct wall-clock calls inside domain validation or reversal helpers are prohibited.

**Acceptance criteria**

- `PostingDate`, `PostedAt`, and policy evaluation timestamps reflect the injected `Clock` deterministically.

### PST-008 — Semantic reversal construction

Constructing a reversal entry (`BuildReversalEntryAt`) MUST:

1. Reject any original entry that is not `Posted` (`ErrEntryNotPosted`) or is already reversed (`ErrEntryAlreadyReversed`).
2. Require non-empty `ActorID` and trimmed `ReversalReason`.
3. Defensively clone the original entry.
4. Normalize `reversalDate` to UTC (defaulting to `now` if zero) and reject a reversal date earlier than the original entry's date (`ErrReversalDateBeforeOriginal`).
5. Produce a new `Draft` entry with a new `ID`, `Classification = Reversing`, `ReversalOfID = original.ID`, `ReversalReason`, and **unset `FiscalPeriodID`** (so the destination period is resolved during `PostEntry` from `reversalDate`).
6. Preserve `AccountID`, analytical dimensions, `MatchingNumber`, transaction currency, and exchange rate on each line while swapping `FunctionalDebit` and `FunctionalCredit`.
7. Verify double-entry balance before returning.

**Acceptance criteria**

- Reversing an unposted or already-reversed entry fails explicitly.
- The original entry remains unmodified by reversal construction.
- The net balance impact of original + reversal per account is zero.

---

## 6. Idempotency, lock ordering, and executor boundary

### PST-009 — Authoritative idempotency scope and canonical fingerprints

Before delegating to the executor, the pipeline MUST overwrite any caller-supplied scope and compute:

1. `IdempotencyScope = CompanyID + Operation + TrimmedKey`, where `Operation` is `post` by default, `reverse` when `ReversalOfID != nil`, or `revaluation` only when a validated revaluation hint matches the entry's `CompanyID` and `IdempotencyKey`.
2. `CallerFingerprint` (`sha256:<hex>`) over canonical caller payload fields (lines sorted by `ID`, decimal amounts as canonical strings, UTC RFC3339Nano timestamps, excluding `CorrelationID` and `IdempotencyKey`).
3. `RequestFingerprint` (`sha256:<hex>`) over the frozen, kernel-stamped entry (including `FiscalPeriodID`, `PolicyFingerprint`, `PostingDate`, `PostedAt`).

**Acceptance criteria**

- Line order differences do not change `RequestFingerprint` or `CallerFingerprint`.
- Forged revaluation hints with mismatched `CompanyID` or `Key` fail with `ErrIdempotencyScopeRequired`.

### PST-010 — Deterministic lock-key derivation

The kernel MUST derive deterministic, deduplicated, lexicographically sorted (UTF-8) lock target keys (`v1|company=<CompanyID>|kind=<kind>|id=<value>`) for accounts, entries, original reversal targets, and idempotency scopes. The kernel MUST NOT acquire physical locks or retain shared lock state across requests; actual lock acquisition and reverse-order release belong to the host adapter.

**Acceptance criteria**

- Two entries referencing the same accounts in opposite line orders yield the exact same sorted lock-key sequence.

### PST-011 — Atomic delegation and error passthrough

The kernel MUST delegate persistence to `PostingExecutor` or `PostingReversalExecutor` exactly once at the final step of `PostEntry`. If the executor fails, the kernel MUST return the error without writing compensating entries or executing implicit retries.

**Acceptance criteria**

- Executor failure is propagated directly without partial kernel side effects or automatic retries.

---

## 7. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| Missing Dependency | Nil pipeline, context, reader, policy engine, clock, or executor | Step 0 (Pre-validation) |
| `ErrUnbalancedEntry` | $\sum \text{FunctionalDebit} \neq \sum \text{FunctionalCredit}$ | Balance validation |
| `ErrRoundingAccountRequired` / `ErrRoundingPolicyRequired` | Non-zero residual without valid policy/account | Rounding settlement |
| `ErrEntryNotPosted` / `ErrEntryAlreadyReversed` | Reversing non-posted or already-reversed entry | Reversal construction |
| `ErrReversalDateBeforeOriginal` | Reversal date earlier than original entry date | Reversal construction |
| `ErrIdempotencyKeyRequired` / `ErrIdempotencyScopeRequired` | Blank key or forged revaluation scope hint | Pre-validation / scope stamp |
| Executor / Storage Error | Atomicity, lock, or payload conflict returned by host | Final delegation |

---

## 8. References

### REF-001 — Accountant Engine Posting & Concurrency Contracts

- **Internal sources:** `docs/KERNEL_POSTING_FLOW_SPECIFICATION.md`, `docs/KERNEL_POSTING_TRANSACTION_CONTRACT.md`, `docs/KERNEL_REVERSAL_ATOMICITY_CONTRACT.md`, `docs/KERNEL_IDEMPOTENCY_CONTRACT.md`, `docs/KERNEL_LOCKING_CONTRACT.md`, `docs/ACCOUNTING_BUSINESS_RULES.md` (§§1, 3, 8).
- **Used for:** Pipeline ordering, rounding settlement, reversal construction, TOCTOU protection, idempotency scope/fingerprints, and lock-key ordering (`PST-001–011`).

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| PST-001–004 | REF-001 (AE-POST-FLOW-V12, AE-RULE-ACC-001) | Central pipeline, double-entry, rounding settlement |
| PST-005–008 | REF-001 (AE-POST-FLOW-V12, AE-P7-REVERSAL-CONTRACT) | TOCTOU, clock discipline, semantic reversal |
| PST-009–011 | REF-001 (AE-IDEMPOTENCY-CONTRACT, AE-P8-LOCKING-CONTRACT) | Idempotency, lock keys, executor boundary |

---

## 9. Appendices

### Appendix A — Informative implementation guidance

- Host executors must wrap entry persistence, GL balance/projection impact, idempotency completion, audit logging, and reversal linking in a single atomic transaction (`Unit of Work`).
- Host retry policies should only retry errors classified as retryable and must perform a safe `IdempotencyLookup` after unknown post-commit outcomes.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| PST-001 | Single central posting path | PROJECT-DECISION | ADR P1-02 / Posting flow | Dependency rejection tests |
| PST-002 | Fail-before-delegate invariant | PROJECT-DECISION | Posting flow §3 | `TestPostEntryExecutorNeverRunsForRejectedInputs` |
| PST-003 | Strict double-entry balance | SOURCE-MANDATED | Double-entry / AE-RULE-ACC-001 | Balance validation tests |
| PST-004 | Normalization & rounding settlement | PROJECT-DECISION | AE-RULE-ACC-001 §3 | `TestPostEntryNormalizesBothCurrenciesAndSettlesRounding` |
| PST-005 | Defensive clone & frozen snapshot | DERIVED-REQUIREMENT | TOCTOU protection | `TestPostEntryExecutesFrozenValidatedSnapshot` |
| PST-006 | Single-read effective account snapshot | PROJECT-DECISION | ADR P3-01 / P1-02 | `TestPostEntryUsesEffectiveAccountSnapshotForEligibility` |
| PST-007 | Injected clock discipline | PROJECT-DECISION | Posting flow §4 | `TestPostEntryUsesOneInjectedClockForPolicyAndPostingMetadata` |
| PST-008 | Semantic reversal construction | PROJECT-DECISION | Reversal contract | `FuzzReversalNetImpactIsZeroPerAccount` |
| PST-009 | Idempotency scope & fingerprints | PROJECT-DECISION | Idempotency contract | `TestPostEntryStampsIdempotencyContractOnExecutorOptions` |
| PST-010 | Deterministic lock-key ordering | PROJECT-DECISION | Locking contract | `lock_manager_test.go` |
| PST-011 | Atomic delegation & error passthrough | BOUNDARY/ASSUMPTION | Posting TX contract | `TestPostEntryStopsWhenAtomicExecutorFails` |

---

## 10. Version history

### v1.0.0

- Initial standalone normative specification for the kernel posting and reversal domain.

