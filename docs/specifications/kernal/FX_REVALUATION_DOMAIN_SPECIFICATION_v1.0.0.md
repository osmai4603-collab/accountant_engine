# FX Revaluation Domain Specification

**Version:** 1.0.0  
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Scope:** Accounting foreign-currency valuation and period-end revaluation: rate-type selection, effective-date rate lookup, asset vs. liability FX gain/loss direction, unrealized revaluation entry generation, revaluation idempotency keys/fingerprints, and revaluation reversals.

---

## 1. Purpose

This document defines the externally observable domain contract for accounting foreign-exchange (FX) valuation and period-end revaluation of monetary balances within the kernel. It specifies how exchange rates are selected by rate type and effective date, how unrealized FX gains and losses are calculated for assets and liabilities, how balanced adjusting revaluation entries are generated, and how duplicate revaluations are prevented.

This document does not prescribe external FX rate-provider APIs, network clients, or persistent database storage.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms per RFC 2119 and RFC 8174:

- **MUST / MUST NOT:** absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs accounting FX valuation and period-end unrealized revaluation within the kernel. Primitive `Money`, `ExchangeRate` numeric validation (12-decimal scale, positive rate), and currency conversion rounding belong to `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md`. Posting execution belongs to `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md`. Realized FX gain/loss calculation upon settlement of open items and external rate-feed ingestion belong outside this kernel specification.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§4–7.
2. MUST enforce rate-type matching (`CLOSING` for period-end revaluation) and reject future-dated rates.
3. MUST invert debit/credit gain/loss direction appropriately between asset and liability accounts.
4. MUST generate balanced adjusting entries booking only unrealized FX differences.
5. MUST enforce deterministic revaluation operation keys and payload fingerprints to prevent conflicting duplicate revaluations.

### 2.2 Conformance evidence

Conformance can be demonstrated through:

- rate registry effective-date selection and duplicate/inverse-pair rejection tests;
- same-currency unit-rate (`1`) validation tests;
- asset vs. liability revaluation direction tests across appreciation and depreciation scenarios;
- zero-difference rejection tests and generated-entry double-entry balance tests;
- revaluation idempotency replay, conflicting-payload rejection, and concurrent-winner tests;
- revaluation reversal period-resolution tests.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

---

## 3. Terms and definitions

- **Rate Type:** Classification of an exchange rate: `SPOT`, `CLOSING`, `AVERAGE`, or `HISTORICAL`.
- **Closing Rate:** The spot exchange rate at the end of the reporting period (IAS 21), required for period-end monetary item revaluation.
- **Rate Date (`RateDate`):** The effective date of the selected reference exchange rate, preserved independently from the revaluation posting date.
- **Unrealized FX Gain / Loss:** The functional-currency difference arising from remeasuring a foreign-currency monetary balance at the closing rate without settling the underlying position.
- **Revaluation Operation Key:** Deterministic key scoped by `(CompanyID, AccountID, TransactionCurrency, RevaluationDate)` identifying a unique revaluation operation.

---

## 4. Rate selection and registry invariants

### VAL-001 — Rate types and closing-rate requirement for revaluation

The valuation domain MUST support `SPOT`, `CLOSING`, `AVERAGE`, and `HISTORICAL` rate types. Period-end revaluation MUST require a `CLOSING` rate and MUST reject rates of another type or rates that do not match the requested currency pair (including validated inversion).

**Acceptance criteria**

- Supplying a `SPOT` or `AVERAGE` rate to period-end revaluation fails validation.

### VAL-002 — Duplicate and inverse-pair rate conflict rejection

Within a rate registry, registering two rates for the same `(BaseCurrency, QuoteCurrency, RateType, EffectiveDate)` OR registering an inverse pair `(QuoteCurrency, BaseCurrency, RateType, EffectiveDate)` on the same effective date MUST fail with `ErrDuplicateRate`.

**Acceptance criteria**

- Registering `USD -> SAR` and then `SAR -> USD` for the same rate type and day fails with `ErrDuplicateRate`.

### VAL-003 — Latest-effective-date lookup without future leakage

When resolving a rate as of `valuationDate`, the registry MUST select the latest registered rate whose `EffectiveDate <= valuationDate`. Future-dated rates (`EffectiveDate > valuationDate`) MUST NEVER be selected; if no rate exists on or before `valuationDate`, lookup MUST fail explicitly.

**Acceptance criteria**

- Given rates on day 1 and day 10, querying day 5 selects day 1's rate; querying day 0 fails explicitly.

### VAL-004 — Independent `RateDate` preservation

Valuation results and generated revaluation entries MUST preserve `RateDate` (the effective date of the selected rate) separately from the valuation/posting `EffectiveDate`.

**Acceptance criteria**

- When day 5 uses a rate effective on day 1, `FXResult.RateDate` records day 1 while the entry `EffectiveDate` records day 5.

### VAL-005 — Same-currency unit rate after validation

When source and target currencies are identical, the valuation engine MUST still validate both currency codes (rejecting invalid codes and `XXX`) and the non-zero valuation date before returning an exact unit rate of `1`.

**Acceptance criteria**

- Same-currency conversion with `XXX` or a zero date fails explicitly rather than returning `1`.

---

## 5. Revaluation calculation and entry generation

### VAL-006 — Opposite FX gain/loss direction for assets vs. liabilities

For a foreign-currency monetary balance revalued to the functional currency:

- **Asset accounts:** an increase in functional-currency value is an **unrealized FX gain** (Debit Asset, Credit Unrealized Gain); a decrease is an **unrealized FX loss** (Debit Unrealized Loss, Credit Asset).
- **Liability accounts:** an increase in functional-currency value is an **unrealized FX loss** (Debit Unrealized Loss, Credit Liability); a decrease is an **unrealized FX gain** (Debit Liability, Credit Unrealized Gain).

**Acceptance criteria**

- Under identical rate appreciation, an Asset account books a gain while a Liability account books a loss of equal magnitude.

### VAL-007 — Balanced adjusting entry for unrealized differences only

`GenerateRevaluationEntryAt` MUST produce a balanced `Adjusting` journal entry (`Draft` status) using `UnrealizedGainID` or `UnrealizedLossID`, and MUST reject zero-difference revaluations. The kernel MUST NOT book realized settlement gains/losses.

**Acceptance criteria**

- A zero functional-currency difference is rejected without generating an entry.
- Every generated revaluation entry passes double-entry balance validation.

### VAL-008 — Revaluation reversal and destination period resolution

Reversing a posted revaluation entry MUST delegate to the kernel's semantic reversal builder (`BuildReversalEntryAt`), preserving the reversal date and `ReversalOfID` link while leaving `FiscalPeriodID` unset so `PostEntry` resolves the destination period from the reversal date.

**Acceptance criteria**

- A revaluation reversal draft resolves its fiscal period from the reversal date during `PostEntry`.

---

## 6. Revaluation idempotency and defensive isolation

### VAL-009 — Deterministic revaluation key, reference, and operation scope

Every revaluation operation MUST derive:

1. A deterministic `RevaluationOperationKey` from `(CompanyID, AccountID, TransactionCurrency, RevaluationDate)`.
2. A deterministic `RevaluationReferenceID`.
3. An idempotency scope with operation `revaluation` (`company=<CompanyID>|operation=revaluation|key=<Key>`), distinct from `post` and `reverse` scopes.

**Acceptance criteria**

- Using the same text key for `post` and `revaluation` produces distinct idempotency scopes.

### VAL-010 — Canonical v2 payload fingerprint and conflict rejection

Revaluation idempotency MUST compute a length-prefixed canonical v2 `sha256:<hex>` payload fingerprint covering all semantic revaluation inputs (amounts, currencies, account, dimensions, date, policy fingerprint). Replaying the same key with an identical payload MUST return the existing result (defensively copied); replaying the same key with a different payload MUST fail with non-retryable `ErrRevaluationAlreadyApplied`. Concurrent registrations for the same key MUST yield a single winner.

**Acceptance criteria**

- Mutating amount, dimension, date, or policy fingerprint under the same key fails with `ErrRevaluationAlreadyApplied`.
- Concurrent goroutines/workers registering the same revaluation key produce a single winning registration.

---

## 7. Error Contract

| Error Category | Trigger Condition | Detection Timing |
| --- | --- | --- |
| `ErrDuplicateRate` | Duplicate or inverse-pair rate on same date | Rate registration |
| Missing/Future Rate | No rate on or before `valuationDate` | Rate lookup |
| Invalid Revaluation Rate | Non-positive rate, wrong rate type, or pair mismatch | Rate validation |
| Zero Revaluation Difference | Functional difference equals zero | Entry generation |
| `ErrRevaluationAlreadyApplied` | Same revaluation key with conflicting payload (non-retryable) | Revaluation registration / commit |

---

## 8. References

### REF-001 — IAS 21 — The Effects of Changes in Foreign Exchange Rates

- **Authority:** International Accounting Standards Board (IASB).
- **Used for:** Period-end translation of monetary items at the closing rate and recognition of unrealized exchange differences (`VAL-001`, `VAL-006`, `VAL-007`).

### REF-002 — Accountant Engine Valuation Matrix & ADRs

- **Internal sources:** `docs/KERNEL_VALUATION_RULES_MATRIX.md`, `docs/ard/P9_01_valuation_idempotency_and_rate_selection.md`, `docs/ard/P11_01_revaluation_idempotency_spi.md`, `docs/KERNEL_IDEMPOTENCY_CONTRACT.md` (§12).
- **Used for:** Rate registry conflict/lookup rules, `RateDate` separation, asset/liability direction, and revaluation idempotency (`VAL-001–010`).

### Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| VAL-001, VAL-006, VAL-007 | REF-001, REF-002 | IAS 21 closing-rate revaluation semantics |
| VAL-002–005, VAL-008–010 | REF-002 (AE-REMED-P9-MATRIX, ADR P9-01, P11-01) | Project rate registry, reversal, and idempotency contracts |

---

## 9. Appendices

### Appendix A — Informative implementation guidance

- While `RevaluationRegistry` provides in-process protection, durable cross-restart idempotency requires persisting the `revaluation` idempotency scope atomically in the host storage adapter (`storage.OperationRevaluation`).

### Appendix B — Rule traceability

| Rule ID | Summary | Classification | Source / Decision | Verification |
| --- | --- | --- | --- | --- |
| VAL-001 | Rate types & `CLOSING` requirement | SOURCE-MANDATED | REF-001 (IAS 21) / REF-002 | `TestRevaluationValidatesRateReferenceAgainstRequest` |
| VAL-002 | Duplicate & inverse-pair rate rejection | PROJECT-DECISION | ADR P9-01 | `TestRateRegistryRejectsMissingAndDuplicateEffectiveDates` |
| VAL-003 | Latest effective rate, no future rate | PROJECT-DECISION | ADR P9-01 | `TestRateRegistrySelectsLatestEffectiveRateAndRejectsFuture` |
| VAL-004 | Independent `RateDate` preservation | PROJECT-DECISION | Valuation matrix | `TestIAS21ExchangeRateAndRevaluation` |
| VAL-005 | Same-currency validation before unit rate | PROJECT-DECISION | Valuation matrix | `TestSameCurrencyRateValidatesCodeAndDateBeforeReturningOne` |
| VAL-006 | Opposite FX direction for assets/liabilities | SOURCE-MANDATED | Accounting / IAS 21 | `TestLiabilityRevaluationDirectionIsOppositeToAsset` |
| VAL-007 | Balanced adjusting entry, unrealized only | SOURCE-MANDATED | REF-001 / REF-002 | `TestRevaluationOnlyBooksUnrealizedFXDifference` |
| VAL-008 | Revaluation reversal period resolution | DERIVED-REQUIREMENT | Posting flow | `TestRevaluationReverseKeepsReversalDateAndPeriod` |
| VAL-009 | Deterministic revaluation key & scope | PROJECT-DECISION | ADR P9-01 / P11-01 | `TestRevaluationIdempotencyScopeIsDeterministicAndOperationScoped` |
| VAL-010 | Canonical v2 fingerprint & conflict reject | PROJECT-DECISION | ADR P9-01 / P11-01 | `TestRevaluationRegistryRejectsConflictingPayloadForSameKey` |

---

## 10. Version history

### v1.0.0

- Initial standalone normative specification for the kernel FX revaluation domain.
