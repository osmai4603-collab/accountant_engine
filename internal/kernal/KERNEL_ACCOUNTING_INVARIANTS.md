# Kernel Accounting Invariants

> **Status:** normative only for the `RULE-KERNEL-001`–`RULE-KERNEL-027` catalog below. The phase-by-phase "clarifications" are non-normative notes; the owning contracts in `docs/contracts/` and the ADRs in `docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md` are the source of truth when a note and a contract disagree. This document is not a release approval; see `docs/reports/KERNEL_RELEASE_SIGNOFF.md`.

This document defines the 27 accounting invariants enforced inside `internal/kernal`.
Authentication, RBAC, maker-checker approval, and external workflow approval are
outside this kernel. `ActorID`, `CreatedBy`, and `PostedBy` are audit attribution
only.

## Financial invariants

- `RULE-KERNEL-001`: A balanced journal entry requires equal total functional
  debit and functional credit after rounding and explicit settlement.
- `RULE-KERNEL-002`: A posted entry must be internally balanced in the
  functional currency and all money values must use the same currency within
  their operation.
- `RULE-KERNEL-003`: Negative or zero amounts are rejected wherever the
  operation contract requires positive money; explicit zero lines remain
  permitted only as zero-side placeholders.
- `RULE-KERNEL-004`: `XXX` and unknown currencies are invalid.
- `RULE-KERNEL-005`: `NewMoney` and `NewMoneyFromString` allow at most 28
  significant digits and reject unrepresentable exponents or invalid inputs.
- `RULE-KERNEL-006`: Internal arithmetic keeps `InternalScale = 12`; posted
  amounts are normalized to the currency `MinorUnit`.
- `RULE-KERNEL-007`: `MinorUnitsExact` never silently drops a sub-minor-unit
  fraction. Posting must round explicitly or settle the residual on the
  configured rounding account.
- `RULE-KERNEL-008`: Currency and exchange-rate mismatch is rejected. A same-
  currency FX request is exactly `1`.

### Phase 2 precision clarifications (non-normative)

The notes below explain the existing behavior of `RULE-KERNEL-001` through
`RULE-KERNEL-008`. They do not create a 28th invariant and do not change any
monetary rule; see ADR P2-01 and `docs/contracts/KERNEL_MONETARY_PRECISION_MATRIX.md`.

- `Banker's rounding` (`Half-to-Even`, `MONEY-019`) is the default monetary
  rounding mode for posting and conversion; intermediate division (`Money.Divide`
  and inverted FX rates at `InternalScale = 12`) uses `Half Away From Zero`
  (`decimal.DivRound`) per `MONEY-017`, and `CurrencyPolicy.RoundingMethod`
  (`Half-to-Even` / `Half-Up` per `MONEY-020` / `POL-009`) is passed
  consistently into `ValidateEntryDoubleEntry`. During multi-currency posting,
  `PST-004` rejects caller functional amounts whose unrounded deviation exceeds
  `0.5` functional minor unit or whose sign differs (`ErrInvalidAmount`).
  `decimal.Truncate` is used only inside exactness/overflow checks, never as a
  posting policy.
- `ValidateExchangeRatePrecision` enforces `MONEY-025` (at most `InternalScale = 12`
  fractional decimal digits) and `MONEY-026` (28 significant/integer digit bounds).
  ISO 4217 currency lookups exclude `XXX` and non-spendable special codes (`XTS`,
  `XAU`, `XAG`, `XPT`, `XPD`, `XDR`) across `types` and `masterdata`, while
  recognizing active ISO 4217 entries such as `XCG`.
- `InternalScale = 12` is an intermediate division scale. It does not permit
  persisting sub-minor-unit amounts; `MinorUnitsExact` fails closed on them and
  the residual is exposed by `RoundingDifference`.
- `MinorUnits()` is the explicit rounding path and may lose value only by that
  explicit rule; cross-currency `Add`/`Subtract` return `ErrCurrencyMismatch`,
  while cross-currency comparisons simply return `false`.
- Same-currency conversion uses the exact rate `1` but still normalizes the
  result to the currency minor unit, like any other conversion.
- `RoundForPosting()` and `CurrencyMinorUnit()` are deprecated compatibility
  aliases. They are retained for external callers and are restricted to kernel
  tests by an automated AST check; kernel production code must use the checked
  variants.
- No `float32`, `float64` or float-derived decimal may enter any monetary path,
  including tests. The precision matrix and its tests are the executable
  evidence for these clarifications.

## Entry lifecycle invariants

- `RULE-KERNEL-009`: Only an entry in `Draft` may enter the posting pipeline,
  and every entry must have an explicit classification. Kernel constructors
  initialize new entries as `Regular`; an empty classification is invalid.
- `RULE-KERNEL-010`: Posting requires a valid company, journal, actor,
  effective date, fiscal period, policy, account, eligible dimensions, and a
  non-empty `IdempotencyKey`.
- `RULE-KERNEL-011`: A posted entry may not be posted again; only posted entries
  may be reversed.
- `RULE-KERNEL-012`: An entry may be reversed once. `ReversedByID`,
  `ReversalOfID`, and `ReversalReason` must be mutually consistent.
- `RULE-KERNEL-013`: Reversal dimensions and accounting currencies are preserved
  while debit and credit sides are swapped.
- `RULE-KERNEL-014`: Reversal date may not precede the original effective date,
  and posting determines the reversal fiscal period from the reversal date.
- `RULE-KERNEL-015`: Period 13 is limited to ordinary adjusting or closing
  entries. Period 14 is limited to adjusting entries whose type is
  `AdjustingEntryAudit` (`audit_adjustment`); audit-adjusting entries are invalid
  in every other period. Any explicitly targeted period must match the resolved
  fiscal period.

## Master data and fiscal invariants

- `RULE-KERNEL-016`: Accounts, journals, companies, cost centers, and analytical
  dimensions must belong to the entry company.
- `RULE-KERNEL-017`: Posting to inactive, closed, header, or out-of-scope data
  is rejected.
- `RULE-KERNEL-018`: Fiscal years and periods must be complete, ordered,
  non-overlapping, and contained by the fiscal year.
- `RULE-KERNEL-019`: Period and fiscal-year close states are enforced. Soft close
  is honored only when the currency/posting policy allows it.
- `RULE-KERNEL-020`: Fiscal-year generation preserves the requested start day,
  clamps period boundaries to month ends, and does not create gaps or overlaps.

### Phase 3 master-data clarifications (non-normative)

These notes make `RULE-KERNEL-016` through `RULE-KERNEL-020` executable; they do
not add a 28th invariant. See ADR P3-01 and
`docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md`.

- A zero `CompanyID` is rejected before any reader call by company, account,
  journal, normal-period and adjustment-period validation (`ErrCompanyScopeRequired`).
- A COA snapshot has one company scope; a parent taken from the same snapshot
  must not belong to a different company.
- Account validity windows are inclusive on both ends; an inverted window is
  rejected by `Validate`/`IsValid`. Adjustment periods 13 and 14 are explicit
  targets only and never participate in date-based period discovery.
- `ValidateCanPostToAccountAsOf` uses the optional `EffectiveAccountReader` port
  when the adapter supplies it, so a later master-data edit cannot reinterpret
  a historical posting. Without that port the kernel still fails closed if the
  single returned record is not current on the posting date.
- `ValidateCanPostToAccountWithClock` and `masterdata.Clock` replace direct wall
  clock access in the current-date compatibility wrapper; postings themselves
  must pass the entry date explicitly.
- Soft close is honored only when the posting policy explicitly allows it; hard
  close is never bypassed at period or fiscal-year level.

### Phase 5 policy clarifications (non-normative)

These notes make the policy part of `RULE-KERNEL-010`, `RULE-KERNEL-017` and
`RULE-KERNEL-019` executable. They do not create a 28th invariant; see ADR P5-01
and `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md`.

- A policy is a company-scoped, versioned fact with a half-open effective
  window `[EffectiveDate, EffectiveTo)`. `EffectiveTo == nil` ends at the next
  version. Resolution by `(CompanyID, asOf)` is deterministic and independent
  of registration order; duplicate starts and overlapping windows are rejected.
- No effective policy means no posting: resolution returns `ErrPolicyNotFound`.
  Explicit gaps and dates before the first version fail closed.
- The posted entry records `PolicyFingerprint`, a canonical digest of the
  resolved posting/currency policy pair. It is immutable audit evidence; a later
  policy version cannot silently rewrite a past entry's interpretation.
- Eligibility decisions are date-aware and scope-aware: a foreign-company,
  inactive, expired, or directly posted control account is rejected.
- `AllowSoftClosedPosting` comes from the policy selected for the entry date and
  is passed to `masterdata`; hard close remains an unconditional prohibition.
  Policy code confers no user permissions.

### Phase 6 posting and reversal clarifications (non-normative)

These notes explain how `RULE-KERNEL-001` through `RULE-KERNEL-017` and
`RULE-KERNEL-027` are enforced by the posting pipeline. They do not create a
28th invariant. See `docs/contracts/KERNEL_POSTING_FLOW_SPECIFICATION.md` and
`docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` for the evidence.

- Every failure path in `PostEntry` returns before `PostingExecutor` or
  `PostingReversalExecutor` is called. The executor receives only a balanced,
  stamped copy of a draft whose validation has completed.
- The entry is defensively cloned before repository access. Caller-side changes
  to company, account, currency, or period after validation cannot alter the
  snapshot delivered to the executor.
- One effective-dated account snapshot per line feeds both account validation
  and `policy.CheckAccountEligibility`. A second, differently-timed account read
  is forbidden because it could validate one master-data version and persist
  another (ADR P3-01).
- A draft that already carries `ReversedByID` is invalid: only a posted entry
  may be reversed, and posted entries never re-enter the pipeline. Reversal
  classification, `ReversalOfID`, and `ReversalReason` must agree before any
  repository or executor call.
- Reversals preserve dimensions, transaction currencies, and account IDs, swap
  functional debit/credit, leave the original untouched, and leave the reversal
  fiscal period unset until `PostEntry` derives it from the reversal date.
- Rounding may create a settlement line only when the currency policy enables
  `AutoSettleRoundingDifferences` and supplies a rounding account. The entry is
  validated as balanced after normalization, never before.
- Wall-clock access in production `posting` code is confined to the
  `systemClock{}.Now()` implementation; deterministic callers use injected
  clocks or explicit `now` constructors. Atomic persistence, idempotency
  storage, and row locking remain host executor responsibilities.

## Valuation and audit invariants

- `RULE-KERNEL-021`: Revaluation is limited to monetary positions and valid
  rates. Four rate types are valid (`closing`, `historical`, `spot`, `average`);
  revaluation itself requires `CLOSING`.
- `RULE-KERNEL-022`: Rate effective dates may not be after the valuation date.
- `RULE-KERNEL-023`: Asset gains/losses and liability gains/losses produce
  opposite debit/credit position effects as appropriate.
- `RULE-KERNEL-024`: Revaluation journal differences must be nonzero and
  balanced in the functional currency.
- Revaluation retries use a deterministic company/account/currency/date
  operation key and reference; repeat registration returns the same entry.
- `RULE-KERNEL-025`: Audit logs must be sequential, company-scoped, linked to
  the previous hash, and have a hash covering every audited semantic field.
  Sequence and previous-hash assignment are atomic per company and are the
  adapter's responsibility; each adapter must accept the same canonical hash
  definition and reject malformed or regressive checkpoints through the shared
  `ValidateCheckpoint` helper.
- `RULE-KERNEL-026`: Audit verification uses one consistent log snapshot and
  verifies an externally retained checkpoint against that snapshot. The
  checkpoint must exist, have a valid shape, and equal both the snapshot
  sequence and the final chain hash. Absence or mismatch in
  `VerifyAuditIntegrityWithCheckpoint` fails closed with
  `ErrAuditCheckpointMismatch`; a malformed checkpoint is rejected by the
  shared `ValidateCheckpoint` helper with `ErrInvalidAuditCheckpoint`. The
  checkpoint-free `VerifyAuditIntegrity` is a lower-guarantee mode and must
  never claim detection of a deleted tail.
  Retention and custody of the checkpoint outside the mutable log are host
  responsibilities, not kernel or adapter policy.
- `RULE-KERNEL-027`: Atomic posting and audit persistence are host executor
  responsibilities. The kernel validates and returns the snapshot; it does not
  claim database or network transaction capabilities. Before delegation it
  stamps the authoritative idempotency scope and payload fingerprints that the
  host must persist atomically with the entry, its audit effect and its result.

### Phase 4 lifecycle and GL projection clarifications (non-normative)

These notes make `RULE-KERNEL-009` through `RULE-KERNEL-015` executable as an
explicit state machine and projection contract; they do not add a 28th
invariant. See ADR P1-02 addendum and
`docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md`.

- The journal lifecycle is `draft -> posted -> reversed`, plus `draft -> cancelled`
  (`EntryCancelled` via `Cancel()`, `JRN-007`) for unposted drafts; no other
  transition is allowed. `journal.ValidateTransition`, `AllowedTransitions`, and
  `JournalEntry.Cancel` are the domain sources of this rule.
- Posted, reversed, and cancelled entries are immutable facts/terminal states.
  Domain code rejects in-place mutation (`journal.ValidateImmutability` /
  `EnsureMutable`, including post-posting `MatchingNumber` and
  `ReconciledAmount` fields per `JRN-008`); corrections to posted entries are
  represented only by linked reversal/correction entries.
- Reversal linkage is one-to-one at domain level: a reversal references exactly
  one original (`ReversalOfID`), and a reversed original references exactly one
  reversal (`ReversedByID`). Reversing an existing reversing entry is rejected
  in `BuildReversalEntryAt` (`PST-008` / `ErrInvalidReversal`). Storage must
  enforce atomic uniqueness.
- Posted `JournalEntry` + `EntryLine` remain the canonical GL source of truth.
  `ledger.PostedMovement`, `RebuildAccountBalances`, and
  `RebuildForeignCurrencyBalances` (`LDG-003`) are deterministic read-only
  projections excluding `Draft` and `Cancelled` (`LDG-001`), not stored
  independent entities or materialized balance stores.
- Rebuild ordering is deterministic: `(EffectiveDate UTC, SequenceNumber,
  JournalEntryID.String())` for entries, then `EntryLine.ID.String()` for lines.

### Phase 5 policy and accounting eligibility clarifications (non-normative)

These notes make `RULE-KERNEL-010`, `RULE-KERNEL-017`, and `RULE-KERNEL-019`
testable without adding a new accounting rule. See ADR P5-01 and the Phase 5
report for the decision record.

- Policy versions use half-open UTC windows `[EffectiveDate, EffectiveTo)`;
  `EffectiveTo == nil` ends at the next registered version. Selection is
  deterministic by `(CompanyID, asOf)`, duplicate starts are invalid, overlaps
  are rejected with `ErrPolicyOverlap`, and gaps or absence fail closed with
  `ErrPolicyNotFound`.
- Registered policies are defensively copied on write and on read; shared
  slices and pointers from callers cannot mutate stored policy state.
- `journal.JournalEntry.PolicyFingerprint` is a canonical, content-bound
  digest of the resolved posting and currency policies. It is stamped after
  validation and before the executor, participates in immutability checks, and
  prevents silent historical reinterpretation when policies change later.
- `policy.CheckAccountEligibility` rejects missing/inactive/control accounts,
  accounts outside the journal company scope, and accounts not effective at
  `entry.EffectiveDate`. Soft-close permission comes only from the selected
  posting policy; hard close remains unconditionally forbidden by `masterdata`.

### Phase 7 transaction and idempotency clarifications (non-normative)

These notes make `RULE-KERNEL-010`, `RULE-KERNEL-011`, `RULE-KERNEL-012`,
`RULE-KERNEL-025` and `RULE-KERNEL-027` testable at the host boundary. They
do not add a 28th invariant and do not move ACID persistence into the kernel.
See `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` and the Phase 7
contracts in `docs/contracts/` for the evidence.

- The kernel derives the uniqueness scope from `(CompanyID, Operation, Key)`
  after validation and overwrites any caller-supplied `PostOptions.Scope`,
  `RequestFingerprint` or `CallerFingerprint` immediately before executor
  dispatch. A posting key and a reversal key with the same text are distinct
  operations at the storage uniqueness boundary.
- `RequestFingerprint` describes the frozen, already-stamped entry delivered
  to the executor. `CallerFingerprint` describes stable caller-controlled
  fields; it is the digest the SDK persists as the idempotency `PayloadHash`
  so that conflict detection works on both sides of the posting transformation.
- Same scope and same payload replay the stored result without a new posting
  effect; the same scope with a different payload is a non-retryable conflict.
  Concurrent executions have one winner and one retrievable result. A lost
  response after commit is resolved by a side-effect-free lookup, never by
  guessing or creating a replacement entry.
- The host atomic unit includes the entry and lines, the GL/balance effect,
  the idempotency record with its stored result, the audit effect, and — for
  reversals — the reverse entry, the original link and the original state.
  Audit is staged inside the same transaction; an ignored audit error is a
  contract violation.
- Retryable failures are transient storage/in-progress conditions. Payload
  conflict, an already-reversed original and an atomicity violation are
  terminal for the same request. Bounded retry policy belongs to the host;
  the kernel contains no retry loop and no database lock.

### Phase 8 concurrency and isolation clarifications (non-normative)

These notes make `RULE-KERNEL-010`, `RULE-KERNEL-016`, `RULE-KERNEL-027` and
the Phase 7 snapshot contract executable at the concurrency boundary. They do
not create a 28th invariant and do not move locking into the kernel. See ADR
P8-01, `docs/contracts/KERNEL_LOCKING_CONTRACT.md`,
`docs/contracts/KERNEL_CONCURRENCY_MODEL.md` and
`docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` for the evidence.

- `posting.OrderedAccountKeys` and the retained `ExtractAndSortAccountIDs`
  produce deterministic, company-scoped key strings only. They acquire no
  lock, hold no state and do not read a clock or random source. The old claim
  that sorting guaranteed deadlock-free row locking was incorrect and is the
  gap closed in Phase 8.
- The host adapter, not the kernel, performs actual locking. The neutral SPI
  contract is `storage.LockTarget` with canonical identity
  `v1|company=<id>|kind=<kind>|id=<value>`; `WithOrderedLocks` validates,
  deduplicates, sorts by canonical text, acquires in ascending order and
  releases in reverse order even on failure. `LockObserver` is a test-only
  hook that proves real acquisitions inside the adapter.
- Four lock classes protect one posting unit: every touched `account`, the
  posted `entry`, the `original` entry during reversal, and the full
  `idempotency` scope. Locking the key text alone is insufficient because the
  same text may name different operations in different companies.
- The snapshot policy forbids TOCTOU reinterpretation: the pipeline resolves
  master data and policy once and freezes `FiscalPeriodID`,
  `PolicyFingerprint`, effective/posting dates and amounts before delegation.
  The adapter persists the frozen entry without re-reading master data. Later
  master-data changes affect new requests only; PostgreSQL stays at
  `READ COMMITTED` by default and a host may request higher isolation through
  `TxOptions`.
- Company scope is part of every lock and every read port. A foreign
  idempotency scope, entry, balance or master-data query is rejected or
  unfindable; `storage.ErrScopeViolation` is non-retryable and maps to
  `ErrForbidden` in the SDK. `ActorID` is never authorization. RLS is an
  adapter defense in depth, not a kernel dependency.
- Retry remains host-only and bounded (`MaxAllowedRetries = 5`,
  `RetryPolicy.Validate`); deadlock, serialization, concurrent-modification
  and in-progress key outcomes may be retried under that policy, while unique,
  already-reversed, payload-conflict, scope and atomicity failures are
  terminal. The kernel contains no retry loop, no SQL and no database lock.

### Phase 9 valuation and FX clarifications (non-normative)

These notes make `RULE-KERNEL-021` through `RULE-KERNEL-024` executable and do
not add a 28th invariant. See ADR P9-01,
`docs/contracts/KERNEL_VALUATION_RULES_MATRIX.md` and
`docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` for the evidence.

- Four rate types (`SPOT`, `CLOSING`, `AVERAGE`, `HISTORICAL`) are valid;
  revaluation itself asks for `CLOSING`. For one pair and type the registry
  selects the latest effective date not after the valuation date and never
  answers from a future rate. A duplicate effective day for the same pair/type
  — including the inverse pair — is rejected.
- `FXResult.RateDate` records the source rate's effective date separately from
  the entry's valuation date. The rate reference, direction and positivity are
  validated before any amount is revalued; cross-currency conversion remains
  decimal-only and same-currency rate is exactly `1`.
- A liability's functional increase is an unrealized loss and its decrease is
  an unrealized gain; the position account therefore takes the opposite side
  from the asset case. Revaluation entries are nonzero, balanced adjusting
  entries posted to the designated unrealized gain/loss accounts. Realized FX
  on settlement is a host posting decision, not a kernel revaluation effect.
- `RevaluationIdempotencyScope` and `RevaluationIdempotencyScopeWithDimensions`
  (`VAL-009`) map the deterministic `(company, account, currency, dimensions,
  date)` operation key to the neutral `posting.IdempotencyScope` with operation
  `revaluation`; the posting and reversal scope derivations are unchanged.
  `ValidateAccountForRevaluation` and `CalculateForAccount` (`VAL-006` /
  `MD-004`) restrict closing-rate FX revaluation to Asset and Liability accounts
  explicitly marked `IsMonetary = true`, rejecting non-monetary and nominal
  accounts (`ErrInvalidAccountType`). Replaying the same stable payload
  returns the stored entry, while a different payload under the same key is
  `ErrRevaluationAlreadyApplied` — the valuation alias of the phase-7 catalog
  entry and non-retryable under `posting.IsRetryable`. `currency` is the foreign
  transaction currency, not the functional one. The canonical digest uses
  length-prefixed stable fields and exact money strings; only generated IDs,
  timestamps and status are excluded, so regenerating the same result can
  replay while a changed amount, dimension or date cannot.
- The in-memory `RevaluationRegistry` protects only one process. Since Phase 11
  the operation is wired end to end: `storage.OperationRevaluation` carries the
  `revaluation` scope through `PostEntry`, and `RevaluationPayloadFingerprint`
  supplies the canonical `fx-revaluation-payload-v2|` digest, so `G-024` is
  **closed** under ADR P11-01 and `docs/contracts/KERNEL_VALUATION_RULES_MATRIX.md`.
  Atomic, durable persistence across restarts and lost responses remains the
  host executor's responsibility under `RULE-KERNEL-027`.
- A revaluation reversal draft carries the reversal date and the link to the
  original, but leaves `FiscalPeriodID` unset; `BuildRevaluationReversalAt`
  delegates to `posting.BuildReversalEntryAt`, and `PostEntry` derives the
  destination period before executor delegation as proven by
  `TestReversalDraftResolvesDestinationPeriodDuringPosting`.
- The compatibility wrapper `GenerateRevaluationEntry` reads time only through
  the package's clock boundary; deterministic callers use
  `GenerateRevaluationEntryAt` with an explicit timestamp.

### Phase 10 audit-chain and evidence clarifications (non-normative)

These notes make `RULE-KERNEL-025` and `RULE-KERNEL-026` executable without
adding a 28th invariant. See ADR P10-01, the audit threat model, the audit
repository contract in `docs/contracts/` and
`docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` for evidence.

- The canonical hash covers version, sequence, company, entity type and id,
  action, actor, reason, correlation id, timestamp truncated to the storage
  precision, a SHA-256 digest of raw payload bytes, and the previous hash. All
  fields are length-prefixed so concatenation cannot be ambiguous; changing any
  semantic field changes the digest.
- Sequence numbers are company-scoped and start at one. The first log links to
  the genesis hash. Assignment of the sequence and its previous link happens in
  one atomic adapter operation; concurrent appends must produce one chain with
  no duplicate or skipped position.
- `AuditVerificationSnapshot.CheckpointPresent` is explicit. A missing
  checkpoint is `ErrAuditCheckpointMismatch` even for an empty chain; the
  verifier never substitutes a default genesis checkpoint and never accepts a
  valid prefix as proof that no tail was removed.
- A trusted checkpoint is bound to the tail: snapshot sequence, log count and
  checkpoint sequence must be equal, and the final computed hash must equal the
  checkpoint hash. Shape rules (non-zero company, 64-character hex SHA-256,
  genesis only at position zero, non-zero version above zero) are shared by the
  kernel and both adapters through `ValidateCheckpoint`.
- Checkpoints only move forward. A repeated version, lower sequence or lower
  version is rejected; PostgreSQL additionally enforces monotonicity with a
  database trigger and applies row-level security with `FORCE ROW LEVEL
  SECURITY`. The kernel stores no checkpoint and knows no storage technology.
- The audit effect of a posting is inside the same host unit of work. A failure
  before audit staging or before commit removes both the legacy audit record
  and the v2 hash-chained log together with the journal effect. PostgreSQL
  achieves the same guarantee in its posting transaction; the kernel only
  supplies the digest and the verification rules.
- "External" checkpoint means external to the mutable log, not necessarily
  external to the database. True custody independence, retention period and
  backup of the checkpoint are host/operations decisions, documented in ADR
  P10-01; the kernel provides no SIEM, key management or external provider.
