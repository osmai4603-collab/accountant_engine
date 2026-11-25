# Governance and Audit Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Tamper-evident audit hash chain, unambiguous canonical hash encoding, per-company monotonic sequence numbering, actor and timestamp attribution, and external audit checkpoint verification.

---

## 1. Purpose

This document defines the externally observable domain contract for forensic audit logging and cryptographic integrity verification within the accounting kernel. It specifies how audit records are chained per company using canonical, collision-resistant hash calculation, and how external checkpoints prevent silent tail-truncation or chain regression.

This document does not prescribe a specific database table (`financial_audit_logs`), WORM trigger syntax, or external KMS/notary transport.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs audit chain structure, canonical hash calculation, chain verification, and external checkpoint validation within the kernel. Journal entry posting atomicity (requiring audit append inside the posting unit of work) belongs to `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md`. Physical WORM triggers, PostgreSQL RLS, and external checkpoint custody storage belong to host/adapter layers.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–6.
2. MUST enforce per-company monotonic sequencing starting at `1` linked from `GenesisHash`.
3. MUST compute `CurrentLogHash` over all semantic fields using unambiguous structural encoding.
4. MUST fail closed when verifying against a missing, malformed, regressed, or mismatched external checkpoint.
5. MUST treat audit chain verification failures as tamper evidence rather than silently repairing or truncating history.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- hash-sensitivity tests mutating every semantic field (actor, action, entity, timestamp, payload, sequence, previous hash);
- delimiter-collision / structural-ambiguity tests;
- tamper-detection tests (modified payload, broken `PreviousLogHash`, sequence gap);
- tail-truncation tests failing closed against an external checkpoint;
- checkpoint validation and anti-regression tests;
- concurrent per-company append tests producing a single contiguous `1..N` chain.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Audit Chain:** A per-company, append-only sequence of audit records cryptographically linked via `PreviousLogHash -> CurrentLogHash`.
- **Genesis Hash (`GenesisHash`):** The fixed, well-known initial hash anchor used as `PreviousLogHash` for sequence number `1`.
- **Canonical Hash (`CalculateHash`):** The deterministic SHA-256 digest over length-prefixed or unambiguously delimited semantic fields of an audit record.
- **External Audit Checkpoint:** A trusted `(Version, SequenceNumber, LogHash)` anchor stored outside the mutable audit table to detect tail-truncation attacks.
- **Verification Snapshot (`AuditVerificationSnapshot`):** A consistent point-in-time read of a company's audit logs, `SnapshotSequence`, and optional external checkpoint.

---

## 4. Hash chain and attribution invariants

### GOV-001 — Company-scoped monotonic sequence

Every audit chain MUST be strictly partitioned by non-zero `CompanyID`. Sequence numbers MUST start at `1` and increment monotonically by `1` (`1, 2, ..., N`) without gaps or duplicates. Concurrent appends for the same company MUST serialize to form a single linear chain.

**Acceptance criteria**

- Concurrent audit appends for Company A produce a contiguous sequence `1..N` with zero duplicates or forks.

### GOV-002 — Genesis anchor and hash linkage

For sequence `1`, `PreviousLogHash` MUST equal `GenesisHash`. For every subsequent record `k > 1`, `PreviousLogHash` MUST equal the `CurrentLogHash` of record `k - 1`.

**Acceptance criteria**

- Any break in `PreviousLogHash` linkage is detected immediately by chain verification.

### GOV-003 — Unambiguous canonical hash over all semantic fields

`CalculateHash` MUST compute a deterministic digest covering `CompanyID`, `SequenceNumber`, `ActorID`, `Action`, `EntityType`, `EntityID`, UTC nanosecond timestamp, structured payload, and `PreviousLogHash` using unambiguous framing (such as length-prefixed or escaped field boundaries) so that moving characters between adjacent fields cannot produce a hash collision.

**Acceptance criteria**

- Altering any semantic field changes `CurrentLogHash`.
- Two records with concatenated-equal but differently partitioned fields (e.g., `Action="AB", Entity="C"` vs. `Action="A", Entity="BC"`) produce distinct hashes.

### GOV-004 — Mandatory attribution and defensive payload copy

Every audit record MUST carry a non-empty `ActorID`, `Action`, and valid UTC timestamp. Adapters returning or storing audit records MUST defensively copy mutable payload bytes so caller mutations cannot alter chained records.

**Acceptance criteria**

- Mutating the caller's payload slice after `AppendAuditLog` does not alter the stored audit record or its hash.

---

## 5. Verification and external checkpoint custody

### GOV-005 — Full chain integrity verification

`VerifyAuditIntegrity` MUST verify:

1. Contiguous sequence numbering `1..N` with no gaps.
2. `PreviousLogHash` linkage starting from `GenesisHash`.
3. Exact recomputation of `CurrentLogHash == CalculateHash(record)` for every record.

Any mismatch MUST fail explicitly with tamper-detection error details.

**Acceptance criteria**

- Modifying any historical row's payload, timestamp, or hash causes `VerifyAuditIntegrity` to fail.

### GOV-006 — Fail-closed external checkpoint verification

Verification against an external checkpoint MUST fail closed (`ErrAuditCheckpointMismatch`) if:

1. `CheckpointPresent == false` (no trusted checkpoint exists).
2. The checkpoint fails `ValidateCheckpoint` (e.g., zero sequence, negative/zero version, or malformed hash).
3. `SnapshotSequence != checkpoint.SequenceNumber`, `len(Logs) != SnapshotSequence`, or the final log's `CurrentLogHash != checkpoint.LogHash` (detecting tail-truncation or partial snapshots).

**Acceptance criteria**

- Deleting the last N audit records (tail-truncation) causes checkpoint verification to fail with `ErrAuditCheckpointMismatch`.
- Missing external checkpoint fails closed rather than passing by default.

### GOV-007 — Anti-regression checkpoint custody

Storing a new external audit checkpoint MUST pass `ValidateCheckpoint` and MUST reject any checkpoint whose `Version` or `SequenceNumber` is less than or equal to the currently stored checkpoint (preventing rollback/regression attacks).

**Acceptance criteria**

- Attempting to overwrite a checkpoint with an equal or lower `Version` or `SequenceNumber` is rejected explicitly.

---

## 6. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| `ErrCompanyScopeRequired` | Zero `CompanyID` in append/verify/snapshot | Pre-operation |
| `ErrInvalidAuditCheckpoint` | Malformed checkpoint (bad version, sequence, or hash) | `ValidateCheckpoint` / `SetExternalAuditCheckpoint` |
| `ErrAuditCheckpointMismatch` | Missing checkpoint, sequence mismatch, or hash mismatch | `VerifyWithCheckpoint` |
| Chain Integrity Error | Sequence gap, broken `PreviousLogHash`, or hash mismatch | `VerifyAuditIntegrity` |
| Checkpoint Regression Error | New checkpoint `Version` or `SequenceNumber` <= current | `SetExternalAuditCheckpoint` |

---

## 7. References

### REF-001 — Accountant Engine Audit & Threat Model Contracts

- **Internal sources:** `docs/KERNEL_AUDIT_REPOSITORY_CONTRACT.md`, `docs/KERNEL_AUDIT_THREAT_MODEL.md`, `docs/ard/P10_01_external_audit_checkpoint_custody.md`, `docs/ACCOUNTING_CAPABILITIES_INVENTORY.md` (`CAP-KERNAL-10`).
- **Used for:** Canonical hash framing, monotonic sequencing, tail-truncation defense, and external checkpoint custody (`GOV-001–007`).

### REF-002 — NIST FIPS 180-4 (Secure Hash Standard — SHA-256)

- **Authority:** National Institute of Standards and Technology (NIST).
- **Used for:** Collision-resistant SHA-256 hashing primitive used in `CalculateHash`.

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| GOV-001–004 | REF-001, REF-002 | Per-company monotonic hash chain and framing |
| GOV-005–007 | REF-001 (ADR P10-01, Audit Contract) | Fail-closed verification and anti-regression checkpoints |

---

## 8. Appendices

### Appendix A — Informative implementation guidance

- External checkpoints should be replicated or anchored in an independent custody store outside the primary database transaction boundary so a database-level tail-truncation cannot simultaneously erase the checkpoint.

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| GOV-001 | Company-scoped monotonic sequence | PROJECT-DECISION | Audit contract §1 | `TestPostgresAuditPhase10ConcurrentAppendSequenceAndChain` |
| GOV-002 | Genesis anchor and hash linkage | PROJECT-DECISION | Audit contract §2 | Chain verification tests |
| GOV-003 | Unambiguous canonical hash framing | PROJECT-DECISION | `CAP-KERNAL-10` | `TestCalculateHashProtectsEverySemanticField` |
| GOV-004 | Mandatory attribution & defensive copy | PROJECT-DECISION | Audit contract §3 | `TestMemoryPhase10AuditLogPayloadIsDefensivelyCopied` |
| GOV-005 | Full chain integrity verification | PROJECT-DECISION | Audit contract §2 | `TestPostgresAuditPhase10TamperingIsDetected` |
| GOV-006 | Fail-closed checkpoint verification | PROJECT-DECISION | ADR P10-01 | `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint` |
| GOV-007 | Anti-regression checkpoint custody | PROJECT-DECISION | ADR P10-01 | `TestPostgresAuditPhase10CheckpointCannotRegress` |

---

## 9. Version history

### v1.0.0

- Initial standalone normative specification for the kernel governance and audit domain.
