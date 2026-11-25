# Money Domain Specification

**Version:** 2.6.1
**Document type:** Standalone domain specification  
**Conformance status:** Normative  
**Supersedes:** v2.4 (implementation-alignment draft)  
**Scope:** Money value, currency identity, decimal precision, arithmetic, rounding, exchange rates, currency conversion, formatting, and serialization.

---

## 1. Purpose

This document defines the externally observable domain contract for monetary values and exchange rates. It is intended to be understandable and testable without access to a particular source-code implementation.

The specification defines:

- the identity and valid representation of a Money value;
- decimal precision and range constraints;
- currency identification and currency metadata;
- arithmetic and rounding behavior;
- exchange-rate validation and conversion;
- formatting and serialization behavior;
- failure conditions and conformance requirements.

This document does not prescribe a programming language, library, database, class structure, or implementation technique. An implementation may use decimal arithmetic, integer-scaled arithmetic, or another representation, provided that its observable behavior conforms to every applicable requirement. The choice of representation does not itself establish conformance: an implementation MUST disclose any representational limits that restrict the values or operations it can support, and MUST fail explicitly before any such limit can cause loss of monetary precision.

### 1.1 Normative language

The terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are to be interpreted as normative requirement terms:

- **MUST / MUST NOT:** absolute requirement for conformance.
- **SHOULD / SHOULD NOT:** recommended requirement; deviations require a documented reason.
- **MAY:** permitted behavior.

### 1.2 Scope boundary

This specification governs monetary domain behavior. Accounting recognition, measurement, posting, functional-currency policy, tax treatment, and financial-statement presentation belong to the relevant accounting or application specifications unless explicitly stated otherwise here.

---

## 2. Conformance

An implementation conforms to this specification only when all applicable normative requirements are satisfied.

### 2.1 Conformance requirements

A conforming implementation:

1. MUST satisfy every normative rule in §§3–8.
2. MUST produce the externally observable outcomes specified by the acceptance criteria.
3. MUST return a defined failure for invalid inputs and out-of-range results; it MUST NOT silently coerce, truncate, overflow, or substitute values.
4. MUST preserve the amount and currency identity through supported serialization round trips.
5. MUST apply rounding only at the boundaries defined by this specification or by an explicit operation contract.
6. MUST distinguish currency metadata sourced from ISO 4217 from project-specific engineering limits.
7. MUST NOT require a caller or auditor to inspect source code to determine the expected domain behavior.

### 2.2 Conformance evidence

Conformance can be demonstrated through externally observable tests, including:

- valid and invalid input examples;
- exact arithmetic examples;
- boundary-value tests;
- rounding tie cases, including positive and negative values;
- currency mismatch cases;
- conversion direction and target-minor-unit cases;
- serialization and parse/serialize round trips;
- error-category and error-timing tests.

A test suite is evidence of conformance; the existence or success of a test suite alone does not redefine this specification.

### 2.3 Partial conformance

An implementation that satisfies only a subset of the requirements MUST NOT be described as fully conformant. Any claimed partial conformance MUST identify the omitted rules and affected behaviors.

If the chosen numeric representation cannot exactly represent or process some values that otherwise satisfy MONEY-005 through MONEY-007, the implementation MUST disclose this as a representational limitation. The disclosure MUST state the maximum effective fractional scale supported, where applicable, and identify the affected value ranges and operations. Such a limitation does not change the normative numeric limits and MUST NOT be presented as an additional domain rule. The implementation MUST reject an unsupported value or result explicitly before returning, persisting, or otherwise exposing it as successful; it MUST NOT round, truncate, underflow, or coerce the value silently.

---

## 3. Core representation and precision

### MONEY-001 — Exact monetary representation

A Money amount MUST be represented and calculated without binary floating-point being the authoritative monetary value. An implementation MAY use exact decimal arithmetic or an integer-scaled representation.

A binary floating-point input MUST NOT be accepted as an authoritative monetary amount unless its exact intended decimal value is independently and unambiguously established before domain validation, for example through an explicit and documented decimal-conversion contract.

**Acceptance criteria**

- The decimal input `0.1` is represented as exactly one tenth, not as a binary approximation.
- Adding `0.1` and `0.2` produces the exact decimal result `0.3`.
- The domain MUST reject or explicitly convert an input whose monetary meaning cannot be recovered exactly.

### MONEY-002 — Currency is part of Money identity

A Money value consists of an amount and a currency identity. Two Money values with different currencies are not equal merely because their numeric amounts are equal.

**Acceptance criteria**

- `10.00 USD` and `10.00 EUR` are not equal as Money values.
- Currency identity is preserved by comparison, arithmetic validation, and serialization.

### MONEY-003 — Exact decimal input

Decimal monetary inputs MUST be parsed from exact decimal representations, such as decimal strings or integer-scaled values. Parsing MUST NOT introduce binary floating-point artifacts.

**Acceptance criteria**

- Parsing the string `"123.4500"` preserves its numeric value exactly.
- A malformed decimal string fails validation.
- A valid decimal input is never silently changed to a nearby representable value.

### MONEY-004 — Internal precision and external precision are distinct

Internal calculation precision and a currency's external minor-unit precision are separate concepts. Intermediate calculations MAY retain more fractional digits than the currency's minor-unit precision. Rounding to the currency minor unit MUST occur only at an explicitly defined external or domain boundary.

**Acceptance criteria**

- A calculation involving a currency with two minor-unit digits MAY retain additional intermediate fractional digits.
- Merely constructing or adding a Money value MUST NOT automatically round it to the currency's minor unit unless the operation contract explicitly requires that boundary.
- Conversion output is rounded according to MONEY-028.

### MONEY-005 — Internal significant-digit limit

The supported Money decimal context has a maximum of **28 significant decimal digits**. Inputs and operation results exceeding this limit MUST fail explicitly.

This is a project-level numeric policy. It is not attributed to ISO 4217, IAS 21, IEEE 754, or another external standard.

**Acceptance criteria**

- A value with 28 significant digits is accepted if it also satisfies all other range constraints.
- A value or result requiring 29 significant digits fails with a precision/range error.
- The implementation MUST NOT silently round a 29-digit value merely to make it fit.

### MONEY-006 — Internal integer-digit limit

The supported Money decimal context has a maximum of **28 integer digits**. Inputs and operation results exceeding this limit MUST fail explicitly.

This is a project-level numeric policy and is independent of currency minor-unit metadata.

**Acceptance criteria**

- A value with 28 integer digits is accepted if it satisfies the significant-digit and exponent constraints.
- A value requiring 29 integer digits fails explicitly.
- The implementation MUST NOT silently clamp or wrap the value.

### MONEY-007 — Internal exponent range

The supported Money decimal exponent range is **[-28, +28]**, inclusive. For purposes of this rule, **exponent** is the exponent applied to the most significant decimal digit of a non-zero value when written in normalized form (one non-zero digit before the decimal point).

Implementations that use a different exponent convention internally MUST map to this definition at the public domain boundary and apply the **[-28, +28]** bound to that mapped value. They MUST NOT rely on an undocumented library-specific interpretation.

This is a project-level numeric policy.

**Acceptance criteria**

- A non-zero value whose normalized most-significant-digit exponent is `-28` or `+28` satisfies the exponent bound, provided all other constraints are satisfied.
- `1e-28` satisfies the exponent bound.
- `1e28` satisfies the exponent bound but fails MONEY-006 because it requires 29 integer digits.
- A non-zero value whose normalized most-significant-digit exponent is below `-28` or above `+28` fails explicitly.
- Underflow MUST NOT silently turn a non-zero monetary value into zero.

**Informative note — derived effective fractional scale**

The combination of MONEY-005 and MONEY-007 permits, as a mathematical consequence, a value with 28 significant digits whose normalized most-significant-digit exponent is `-28`. In such a value, the 28th significant digit can occur at exponent `-55`, yielding an effective fractional scale of 55 places. For example, `1.234567890123456789012345678e-28` has 28 significant digits and an effective fractional scale of 55.

This is an explanatory consequence of the existing numeric limits, not a separate normative minimum, an independent acceptance threshold, or a new conformance rule. Whether a particular implementation can represent and process such a value exactly depends on its chosen representation. Any limitation that affects values otherwise permitted by the normative numeric limits MUST be disclosed under §2.3 and handled without silent loss of precision under MONEY-008 and §9.

### MONEY-008 — Overflow and precision-boundary handling

An input or operation result that exceeds the supported significant-digit limit, integer-digit limit, exponent range, or representational range MUST produce an explicit failure. Silent overflow, underflow, truncation, wraparound, and lossy coercion are prohibited.

**Acceptance criteria**

- An out-of-range operation returns a failure and does not expose a fabricated monetary result.
- A failed operation does not mutate an existing Money value.
- A non-zero value MUST NOT silently become zero through underflow.

---

## 4. Currency identity and registry

### MONEY-009 — Currency normalization and validation

Currency input MUST be normalized by:

1. trimming surrounding whitespace;
2. converting ASCII letters `a`–`z` to uppercase;
3. requiring the normalized value to match exactly `[A-Z]{3}`.

Digits, punctuation, currency symbols, non-ASCII letters, and values that do not produce exactly three ASCII letters MUST be rejected. Implementations MUST NOT transliterate, guess, or silently substitute currency codes.

Normalization establishes syntactic shape only; it does not establish that a code is assigned, current, or supported.

**Acceptance criteria**

- `" usd "` normalizes to `USD`.
- `"US1"`, `"US$"`, `"US"`, `"USDD"`, and a code containing non-ASCII letters are rejected.
- A syntactically valid but unassigned or unsupported code is rejected by registry validation.

### MONEY-010 — ISO 4217 currency-code lifecycle

Currency-code assignments and lifecycle metadata MUST be maintained against the authoritative ISO 4217 currency-code registry and its maintenance notices.

The registry data used by an implementation MUST be versioned or otherwise identifiable so that the applicable currency metadata can be determined. Where historical-date support is provided, the registry MUST distinguish a code's effective lifecycle dates from its current status.

The codes and lifecycle events mentioned in v2.4—including BGN, ANG/XCG, CUC, and SLL—are examples of changes that require registry maintenance, not a substitute for the current authoritative registry snapshot.

**Acceptance criteria**

- A currency code's supported status is determined from controlled registry data, not from a hard-coded assumption scattered across operations.
- Registry updates can be traced to a particular source snapshot or maintenance notice.
- A withdrawn or replaced code is not silently treated as a current code.

### MONEY-011 — Currency minor units

Currency minor-unit metadata MUST be explicit and sourced from the applicable ISO 4217 registry data. Implementations MUST NOT assume that every currency has two minor-unit digits.

The registry data includes currencies with different minor-unit counts; CLF and UYW are examples identified in the v2.4 source document as having four minor-unit digits. The applicable registry version governs.

**Acceptance criteria**

- Formatting and conversion use the configured minor-unit count for the currency.
- A currency with zero, two, or four minor-unit digits is not forced into a universal two-digit policy.
- The minor-unit count is traceable to the registry version in use.

### MONEY-012 — Currency registry lookup

Currency metadata, including minor-unit information and lifecycle status, MUST be obtained from a controlled registry. Unknown or unsupported currencies MUST fail explicit validation.

**Acceptance criteria**

- A code that passes the syntactic pattern but is absent from the supported registry fails domain validation.
- Arithmetic and formatting MUST NOT invent metadata for an unknown code.

### MONEY-013 — Minor-unit source of truth and overrides

ISO 4217 registry data is the default source of truth for currency minor-unit metadata. A disagreement with stale or custom data MUST NOT be resolved silently.

Any project override MUST be explicit, documented, versioned, and justified. It MUST identify:

- the affected currency;
- the prior value;
- the replacement value;
- the reason;
- the effective date.

**Acceptance criteria**

- An override can be identified independently of the base registry value.
- An undocumented override is non-conformant.
- Updating the base registry does not silently erase or alter a separately approved override.

### MONEY-014 — Unsupported and special currency codes

A code representing no currency or a special-purpose denomination, such as `XXX`, MUST NOT be accepted as an ordinary spendable currency unless the domain explicitly models that special-purpose meaning and defines its behavior.

**Acceptance criteria**

- `XXX` is rejected by ordinary spendable-Money construction.
- If a future domain explicitly supports a special-purpose code, its behavior is separately specified and cannot be inferred from ordinary currency behavior.

---

## 5. Arithmetic and rounding

### MONEY-015 — Currency compatibility for arithmetic

Addition, subtraction, comparison, and any other operation requiring a common denomination MUST reject currency mismatches unless an explicit currency-conversion operation is invoked.

**Acceptance criteria**

- Adding `10 USD` to `5 EUR` fails with a currency-mismatch error.
- Comparing `10 USD` and `10 EUR` as if they were the same denomination fails or returns a clearly defined non-equality result according to the comparison API; it MUST NOT treat the amounts as interchangeable.
- No implicit exchange rate is selected.

### MONEY-016 — Addition and subtraction

Addition and subtraction MUST preserve currency identity and use exact decimal arithmetic, subject to the precision and range limits in MONEY-005 through MONEY-008.

**Acceptance criteria**

- `10.25 USD + 2.50 USD = 12.75 USD`.
- `10.25 USD - 2.50 USD = 7.75 USD`.
- The same operations with different currencies fail under MONEY-015.
- An out-of-range result fails rather than being truncated.

### MONEY-017 — Division

Division MUST use an internal result scale of **12 fractional decimal places** and the **Half Away From Zero** rounding mode. The rounded result MUST then be checked against the significant-digit, integer-digit, exponent, and representational limits.

Division by zero MUST fail. A boundary violation MUST fail explicitly; silent truncation or overflow is prohibited.

**Acceptance criteria**

- `1 / 3` produces `0.333333333333` at the defined division scale.
- `1 / 8` produces `0.125000000000` when the division result is represented at the fixed 12-place scale; equivalent numeric serialization MAY omit insignificant trailing zeros if the serialization contract permits it.
- A tie case at the 12th fractional place is rounded Half Away From Zero for both positive and negative results.
- Division by zero fails before a result is returned.

**Policy note — rationale for operation-specific rounding**

Division uses **Half Away From Zero** at the 12-place intermediate scale, while MONEY-019 and MONEY-028 use **Half-to-Even** at their respective rounding boundaries. This distinction is intentional and is based on the different semantic roles of the results:

1. **Division produces an intermediate value.** A result at the 12-place scale is commonly an intermediate quantity—such as a per-unit amount or ratio—that may be used in subsequent multiplication or aggregation. It is not necessarily a final currency-denominated amount.
2. **The accumulation-bias concern differs.** Half-to-Even is used for final monetary rounding to reduce systematic cumulative bias. The fixed-scale division result is an intermediate value and is not ordinarily accumulated as a sequence of independently rounded final monetary amounts in the same way.
3. **The intermediate scale has no parity semantics for callers.** Both modes are symmetric with respect to sign. Half Away From Zero avoids making the intermediate tie result depend on the parity of the last retained digit, whose even/odd status has no intended domain meaning to callers at the 12-place intermediate scale.
4. **The behavior is explicit, not implicit.** MONEY-020 requires the operation-specific mode to be declared. Callers therefore cannot rely on an undocumented library default or infer a different mode.

Accordingly, applying the MONEY-019 default mode to division, or silently unifying all rounding operations under one mode, does not conform to MONEY-017. An implementation that does so MUST identify MONEY-017 as an omitted or non-conformant rule when making a partial-conformance claim under §2.3.

### MONEY-018 — Multiplication

Multiplication MUST use exact decimal arithmetic for the intermediate result. It MUST NOT round merely because the product has more fractional digits than the currency's minor unit. Rounding occurs only at a boundary explicitly defined by an operation contract. The result MUST satisfy the precision and range constraints.

**Acceptance criteria**

- `1.25 USD × 2 = 2.50 USD`.
- Multiplication preserves exact intermediate decimal value when within supported bounds.
- A product exceeding supported bounds fails explicitly.
- Multiplication does not apply an undocumented currency-minor-unit rounding step.

### MONEY-019 — Default rounding mode

The default rounding mode for operations that require rounding is **Half-to-Even (Banker's rounding)** unless a rule explicitly specifies another mode.

**Acceptance criteria**

- Under Half-to-Even, `2.5` rounded to an integer becomes `2`.
- Under Half-to-Even, `3.5` rounded to an integer becomes `4`.
- Under Half-to-Even, `-2.5` rounded to an integer becomes `-2`.
- Under Half-to-Even, `-3.5` rounded to an integer becomes `-4`.
- Negative tie cases follow the same nearest-even rule symmetrically.
- An operation-specific rounding rule takes precedence over this default only when it explicitly declares the alternative mode.

### MONEY-020 — Explicit rounding mode

Any operation using a rounding mode other than the default MUST declare that mode in its domain contract. Callers MUST NOT rely on undocumented or implicit rounding behavior.

**Acceptance criteria**

- The division contract explicitly identifies Half Away From Zero.
- The conversion contract explicitly identifies Half-to-Even.
- An operation that requires rounding but declares no mode uses MONEY-019.

### MONEY-021 — Rounding boundary

Rounding MUST occur only at an explicit operation or domain boundary. The currency minor-unit boundary is one such external boundary. Internal calculation precision remains governed by MONEY-004 through MONEY-008 and the specific operation contract.

**Acceptance criteria**

- Intermediate calculations do not repeatedly round after each sub-operation unless the operation contract explicitly requires it.
- Conversion rounds at the final target-currency boundary.
- Formatting rounds only the displayed representation and does not mutate the Money value.

### MONEY-022 — Rounding is not truncation

When a rule requires rounding, the declared rounding mode MUST be applied. Truncation MUST NOT be used as a substitute and MUST NOT occur silently.

**Acceptance criteria**

- A value requiring rounding is rounded according to the declared mode.
- A value is not shortened by simply discarding excess fractional digits.
- If an input exceeds a hard precision limit, it is rejected where the applicable rule requires rejection; it is not silently rounded to fit.

---

## 6. Exchange rates and currency conversion

### MONEY-023 — Exchange-rate positivity

An exchange rate MUST be strictly greater than zero. Zero and negative rates are invalid.

**Acceptance criteria**

- A positive rate is eligible for subsequent validation.
- Zero and negative rates fail validation.
- A failed rate validation cannot be used for conversion.

### MONEY-024 — Exact exchange-rate representation

An ExchangeRate value MUST use an exact decimal representation. Binary floating-point MUST NOT be its authoritative value.

**Acceptance criteria**

- The decimal string `"1.2345"` is represented exactly.
- Exchange-rate calculations do not introduce binary floating-point artifacts.
- An inexact floating-point value is not accepted as an authoritative rate without an exact conversion contract.

### MONEY-025 — Exchange-rate fractional precision limit

A validated exchange-rate value MUST contain no more than **12 fractional decimal places**. A value with more than 12 fractional decimal places MUST be rejected. Validation MUST NOT silently round or truncate the supplied value to fit the limit.

This is a project engineering policy chosen to bound and standardize exchange-rate input precision. It is not a precision requirement imposed by ISO 4217 or IAS 21.

MONEY-025 is the authoritative normative requirement. MONEY-034 records the decision history and does not create a second requirement.

**Acceptance criteria**

- `1.123456789012` is accepted if all other rate constraints are satisfied.
- `1.1234567890123` is rejected.
- The rejected value is not silently changed to `1.123456789012`.

### MONEY-026 — Exchange-rate magnitude and decimal bounds

An exchange rate MUST satisfy the supported decimal significant-digit, integer-digit, exponent, and magnitude bounds. A value outside those bounds MUST be rejected explicitly.

For this specification, the general decimal context limits in MONEY-005 through MONEY-007 apply to ExchangeRate unless a future, explicitly versioned rule defines a narrower ExchangeRate-specific bound. The additional 12-fractional-place limit in MONEY-025 also applies.

**Acceptance criteria**

- A rate satisfying the 12-fractional-place rule but exceeding the general decimal bounds is rejected.
- A rate satisfying the general decimal bounds but exceeding 12 fractional places is rejected.
- No rate is silently clamped to a supported magnitude.

### MONEY-027 — Conversion direction

A conversion MUST identify the source currency, target currency, and rate direction explicitly. The rate direction MUST define which currency units are represented by one unit of the other currency; for example, target-currency units per one source-currency unit.

Direction MUST NOT be inferred ambiguously from currency-code ordering or caller assumptions.

**Acceptance criteria**

- A conversion contract states whether the rate is target units per source unit or the inverse.
- Reversing source and target currencies requires an explicitly supplied inverse rate or an explicitly defined inversion operation.
- A rate with ambiguous direction is rejected or cannot be used.

### MONEY-028 — Conversion rounding

Currency conversion MUST calculate using the exact rate and preserve intermediate precision within the supported decimal context. The final converted amount MUST be rounded to the target currency's minor-unit precision using **Half-to-Even**, then validated against the supported result bounds.

**Acceptance criteria**

- Converting `10.00 USD` at `0.9 EUR per USD` produces `9.00 EUR`.
- If the target currency has three minor-unit digits, the final converted amount is rounded to three digits, not two.
- A tie at the target minor-unit boundary follows Half-to-Even.
- The source Money value is not mutated by conversion.

### MONEY-029 — Accounting boundary and IAS 21 integration

Functional-currency determination, foreign-operation translation, transaction-date and reporting-date rate selection, exchange differences, recognition, and accounting treatment under IAS 21 are accounting integration concerns, not rules of the pure Money primitive.

The Money domain MAY provide exact arithmetic, currency identity, and conversion primitives used by an accounting or valuation layer. The accounting layer MUST define the applicable accounting policy and rate-selection behavior.

**Acceptance criteria**

- The Money primitive does not independently decide which accounting-date exchange rate an entity must use.
- IAS 21 accounting treatment is specified in the relevant accounting/valuation specification.
- Integration code supplies an explicit rate and conversion direction to the Money operation.

---

## 7. Formatting and serialization

### MONEY-030 — String and display formatting

Display formatting MAY round to an explicitly requested scale or to a currency-defined display scale. Half-to-Even is the default rounding mode for display formatting unless the formatter contract explicitly declares another mode.

Formatting MUST NOT mutate the underlying Money value.

**Acceptance criteria**

- Formatting `12.345 USD` to two fractional digits produces the display string `12.34` under Half-to-Even.
- Formatting does not change the underlying amount from `12.345`.
- Formatting to a requested scale does not change the Money currency identity.

### MONEY-031 — Formatting scale

Formatting scale MUST be explicit or derived from currency metadata under a documented formatter contract. Formatting MUST NOT silently alter the stored amount or its scale/value.

**Acceptance criteria**

- A formatter using currency defaults obtains the minor-unit count from the controlled currency registry.
- A caller-requested display scale is applied only to the formatted representation.
- Formatting the same Money value at two display scales does not change the stored Money value.

### MONEY-032 — JSON representation

Money amounts in JSON MUST be serialized as exact decimal strings, not JSON floating-point numbers. Currency identity MUST be serialized explicitly according to the public API contract.

**Acceptance criteria**

- The amount `123.4500` is represented as a JSON string, for example `"123.4500"`, rather than a JSON numeric token.
- Currency identity is present as an explicit field or equivalent unambiguous contract element.
- A consumer does not need to infer the currency from locale or display text.

### MONEY-033 — Parse/serialize round trip

Parsing and serialization MUST preserve the exact monetary numeric value and currency identity within the supported domain. Invalid, out-of-range, or precision-exceeding input MUST fail rather than be coerced.

The serialization contract MUST state whether insignificant trailing fractional zeros are preserved lexically. Regardless of lexical scale preservation, the numeric value and currency identity MUST remain exact.

**Acceptance criteria**

- Serializing and parsing a valid Money value preserves its numeric value and currency.
- A value outside the supported domain fails during parsing or validation.
- A JSON numeric token used where a decimal string is required is rejected.
- If lexical scale preservation is part of the API contract, `"1.2300"` round-trips with that scale; otherwise the normalized decimal string still represents exactly the same value.

---

## 8. Decision record

### MONEY-034 — ExchangeRate precision decision

The project decision recorded in v2.2 sets the maximum fractional precision for ExchangeRate to **12 decimal places**. Values exceeding 12 fractional decimal places are rejected during validation; silent rounding or truncation is not permitted.

This is a project engineering policy, not a universal accounting or ISO requirement.

MONEY-025 is the normative requirement. MONEY-034 is the decision record and must not be interpreted as a second or competing requirement.

---

## 9. Error contract

### 9.1 General failure behavior

When an operation cannot satisfy a normative rule, it MUST fail explicitly. It MUST NOT return a plausible-looking but altered monetary value.

A failure MUST:

- identify the operation as unsuccessful;
- expose a stable, machine-identifiable error category or code;
- provide a human-readable explanation suitable for diagnosis;
- avoid exposing a successful result for the failed operation;
- leave input values unchanged.

An implementation MAY provide additional diagnostic details, but callers MUST NOT be required to parse human-readable text to determine the error category.

### 9.2 Error categories

The following semantic categories MUST be distinguishable, even if an implementation uses different public names:

| Error category | Meaning |
| --- | --- |
| Invalid decimal input | Input is malformed or cannot be parsed exactly |
| Invalid currency syntax | Currency code fails MONEY-009 |
| Unsupported currency | Currency code is not supported by the applicable registry |
| Currency mismatch | Operation requires a common currency but operands differ |
| Division by zero | Divisor is zero |
| Invalid exchange rate | Rate is zero, negative, or otherwise invalid |
| Exchange-rate precision exceeded | Rate has more than 12 fractional digits |
| Precision exceeded | Significant-digit limit is exceeded |
| Integer range exceeded | Integer-digit limit is exceeded |
| Exponent/range exceeded | Normative exponent or numeric range limit is exceeded |
| Representation scale capacity exceeded | The exact value or result exceeds the implementation's disclosed scale capacity; no successful or altered value may be returned |
| Invalid conversion direction | Source/target/rate direction is missing or ambiguous |
| Serialization contract violation | Serialized representation violates the required format |

Implementations MAY expose more granular errors, but MUST preserve these distinctions at the public domain boundary.

### 9.3 Error detection timing

Validation MUST occur no later than the earliest point at which the relevant invalid condition is knowable, and MUST occur before an invalid value can be used to produce or commit a domain result.

| Condition | Required detection point |
| --- | --- |
| Malformed decimal or currency syntax | During input parsing/construction |
| Unsupported currency or invalid special code | During currency validation/construction, before arithmetic |
| Currency mismatch | Before performing the incompatible arithmetic/comparison operation |
| Division by zero | Before division result production |
| Invalid or over-precision ExchangeRate | During rate construction/validation, before conversion |
| Missing or ambiguous conversion direction | Before conversion calculation |
| Precision, integer, exponent, or overflow boundary | Before returning the affected operation result |
| Implementation representation/scale capacity | During parsing/construction or operation evaluation, as soon as the capacity limit is knowable, and before returning, persisting, or exposing a result as successful |
| Invalid JSON amount representation | During deserialization, before a Money value is exposed |
| Invalid output representation | During serialization, before the serialized result is returned |

A system MAY perform earlier validation. It MUST NOT defer a known invalid condition until after a result has been returned, persisted, or otherwise treated as successful.

### 9.4 Atomicity

A failed domain operation MUST NOT partially mutate its input Money values or ExchangeRate values. If a higher-level operation persists results, persistence atomicity is governed by the relevant application or accounting specification.

---

## 10. Glossary

| Term | Definition |
| --- | --- |
| Money | A domain value composed of a numeric amount and a currency identity |
| Currency identity | The currency code associated with a Money value |
| Currency registry | Controlled data containing supported currency codes, lifecycle information, and metadata |
| Minor unit | The currency-defined number of fractional digits used for the currency's external unit |
| Internal precision | Precision retained for domain calculations before an explicit rounding boundary |
| Significant digits | Digits that contribute to the precision of a decimal value, under the decimal context's defined semantics |
| Integer digits | Digits to the left of the decimal point, excluding sign and leading zeros |
| Exponent | The power-of-ten component of the decimal representation, interpreted according to the specified decimal model |
| Rounding boundary | A defined point at which a value is rounded under an explicitly declared mode and scale |
| Half-to-Even | Rounding to the nearest value; exact ties go to the result whose final retained digit is even |
| Half Away From Zero | Rounding to the nearest value; exact ties go away from zero |
| ExchangeRate | A strictly positive exact-decimal rate relating two identified currencies |
| Rate direction | The explicit meaning of a rate, such as target-currency units per one source-currency unit |
| Exact decimal representation | A representation that preserves the intended base-10 value without binary floating-point approximation |
| Normative rule | A requirement that implementations must satisfy to claim conformance |
| Informative reference | A source used for context or explanation that does not itself create a project requirement |

---

## 11. References and source traceability

### 11.1 Reference-use policy

The references below provide standards, design patterns, and decimal-arithmetic background. They are informative unless a rule explicitly makes a source normative. ISO 4217 is the authoritative source for the currency registry metadata identified in MONEY-010 through MONEY-014.

Project-specific policies—including the 28-significant-digit limit, 28-integer-digit limit, exponent range [-28, +28], 12-fractional-place ExchangeRate limit, and the Half-Away-From-Zero division mode—MUST NOT be represented as requirements imposed by external sources unless the source explicitly says so.

Where a source is maintained or amended over time, the exact edition, registry snapshot, or maintenance notice used for a future update SHOULD be recorded.

### REF-001 — ISO 4217:2015

- **Title:** Codes for the representation of currencies
- **Edition:** Edition 8, ISO 4217:2015, August 2015; the v2.4 source document records ISO's 2021 review/confirmation.
- **Maintaining authority:** SIX Financial Information AG, ISO 4217 Maintenance Agency.
- **Official links:**
  - <https://www.iso.org/standard/64758.html>
  - <https://www.iso.org/iso-4217-currency-codes.html>
  - <https://www.six-group.com/en/products-services/financial-information/market-reference-data/data-standards.html>
- **Used for:** currency-code structure, identifiers, minor-unit metadata, registry lifecycle, and maintenance.
- **Mapped rules:** MONEY-009 through MONEY-014, especially MONEY-010, MONEY-011, and MONEY-013.
- **Scope note:** ISO registry data and lifecycle notices are authoritative for currency metadata. ISO does not mandate this project's internal decimal precision or ExchangeRate scale.

### REF-002 — IAS 21 / IFRS Accounting Standards

- **Title:** The Effects of Changes in Foreign Exchange Rates
- **Reference edition recorded in v2.4:** IFRS Accounting Standards — Required 2026 (standards required as at 1 January 2026); applicable amendments and effective dates must be checked before accounting-policy use.
- **Official links:**
  - <https://www.ifrs.org/issued-standards/list-of-standards/ias-21-the-effects-of-changes-in-foreign-exchange-rates/>
  - <https://www.ifrs.org/news-and-events/news/2026/01/now-available-ifrs-accounting-standards-required-2026-two-editions/>
- **Used for:** foreign-currency accounting and translation context only.
- **Mapped rule:** MONEY-029.
- **Scope note:** IAS 21 accounting treatments are outside pure Money primitives. This reference does not prescribe Money's data structure or the 12-place ExchangeRate limit.

### REF-003 — JSR 354: Money and Currency API

- **Title:** Money and Currency API
- **Version:** 1.1, Maintenance Release 1, May 2020.
- **Official links:**
  - <https://jcp.org/aboutJava/communityprocess/mrel/jsr354/index.html>
  - <https://jcp.org/en/jsr/detail?id=354>
  - <https://github.com/JavaMoney/jsr354-api>
- **Used for:** design context for monetary amount/currency abstractions, currency metadata, rounding, calculation precision, extensibility, and interoperability.
- **Mapped rules:** MONEY-002, MONEY-004, MONEY-009–013, MONEY-015–021, MONEY-028, MONEY-030–033.
- **Scope note:** JSR 354 is a Java API specification. This project does not claim API compatibility or conformance; the mapping is conceptual and informative.

### REF-004 — Martin Fowler, Money Pattern

- **Title:** Money — Patterns of Enterprise Application Architecture
- **Publication date recorded in v2.4:** 5 March 2003 (catalog entry).
- **Links:**
  - <https://martinfowler.com/eaaCatalog/money.html>
  - <https://martinfowler.com/bliki/CurrencyAsValue.html>
  - <https://martinfowler.com/bliki/ValueObject.html>
- **Used for:** modeling money as an amount plus currency, preventing currency-mismatch operations, and identifying rounding-related monetary errors.
- **Mapped rules:** MONEY-001, MONEY-002, MONEY-015, MONEY-016, MONEY-019–022.
- **Scope note:** This is design guidance, not a normative accounting or numerical standard.

### REF-005 — General Decimal Arithmetic Specification

- **Author:** Mike Cowlishaw
- **Version:** 1.70, 7 April 2009.
- **Links:**
  - <https://speleotrove.com/decimal/>
  - <https://speleotrove.com/decimal/decarith.html>
  - <https://speleotrove.com/decimal/decarith.pdf>
- **Used for:** decimal arithmetic model, precision/context, rounding modes, conversion behavior, exceptional conditions, overflow/underflow, and exact decimal arithmetic concepts.
- **Mapped rules:** MONEY-003, MONEY-005–008, MONEY-017–022, MONEY-024–026, MONEY-030–033.
- **Scope note:** This specification describes general decimal arithmetic. It does not establish this project's exact limits (28 significant digits, 28 integer digits, exponent [-28, +28], or ExchangeRate scale 12).

### REF-006 — IEEE 754-2019

- **Title:** IEEE Standard for Floating-Point Arithmetic
- **Edition:** IEEE 754-2019.
- **Official link:** <https://standards.ieee.org/standard/754-2019.html>
- **Used for:** definitions and behavior of binary and decimal floating-point formats, rounding, exceptional conditions, and numerical representation concepts.
- **Mapped rules:** MONEY-001, MONEY-003, MONEY-005–008, MONEY-017–022, MONEY-024–026.
- **Scope note:** IEEE 754 defines specific formats and arithmetic behavior. The project's limits must not be described as IEEE decimal64/decimal128 limits.

### 11.2 Reference-to-rule index

| Rule(s) | Primary reference(s) | Nature of relationship |
| --- | --- | --- |
| MONEY-001 | REF-004, REF-006 | Design guidance / numerical representation |
| MONEY-002 | REF-003, REF-004 | Money amount and currency model |
| MONEY-003–008 | REF-005, REF-006 | Decimal arithmetic background; numeric limits are project policies |
| MONEY-009–014 | REF-001, REF-003 | Currency codes and metadata; ISO is authoritative for registry metadata |
| MONEY-015–022 | REF-003, REF-004, REF-005 | Arithmetic and rounding design guidance |
| MONEY-023–028 | REF-005, REF-006 | Decimal arithmetic context; conversion behavior is project policy |
| MONEY-029 | REF-002 | Accounting integration boundary |
| MONEY-030–033 | REF-003, REF-005 | Formatting/serialization design guidance; API contract is project-specific |
| MONEY-025 / MONEY-034 | REF-005, REF-006 | 12-place limit is an explicit project engineering decision |

---

## 12. Appendices

### Appendix A — Informative implementation guidance

The following notes are informative and do not create additional normative requirements:

- A decimal library may be used, but its defaults do not override this specification.
- Integer-scaled arithmetic may be used if it can represent all required values and operations without loss.
- The internal decimal context and currency minor-unit metadata solve different problems and must not be conflated.
- Display formatting is not a substitute for domain arithmetic.
- Accounting policy and rate selection should be implemented outside the pure Money primitive.
- A registry update should retain its source snapshot or maintenance notice for auditability.

### Appendix B — Numeric examples

The examples below illustrate the normative rules; they do not replace the rule text.

| Rule | Input | Expected outcome |
| --- | --- | --- |
| MONEY-001 | `0.1 + 0.2` | Exact result `0.3` |
| MONEY-002 | `10.00 USD` vs `10.00 EUR` | Different Money identities |
| MONEY-005 | 28 significant digits | Accepted if other bounds pass |
| MONEY-005 | 29 significant digits | Explicit failure |
| MONEY-006 | 28 integer digits | Accepted if other bounds pass |
| MONEY-006 | 29 integer digits | Explicit failure |
| MONEY-007 | `1e-28` (MSD exponent `-28`) | Accepted if other bounds pass |
| MONEY-007 / MONEY-006 | `1e28` (MSD exponent `+28`) | Fails MONEY-006: 29 integer digits |
| MONEY-005 / MONEY-007 (informative derivation) | `1.234567890123456789012345678e-28` | Illustrates 28 significant digits and effective fractional scale 55; not a separate conformance threshold. Exact support depends on the implementation's disclosed representation capacity |
| MONEY-007 | MSD exponent outside `[-28, +28]` | Explicit failure |
| MONEY-009 | `" usd "` | Normalizes to `USD` |
| MONEY-009 | `"US$"` | Rejected |
| MONEY-015 | `10 USD + 5 EUR` | Currency-mismatch failure |
| MONEY-017 | `1 / 3` | `0.333333333333` |
| MONEY-019 | `2.5` to integer | `2` |
| MONEY-019 | `3.5` to integer | `4` |
| MONEY-019 | `-2.5` to integer | `-2` |
| MONEY-019 | `-3.5` to integer | `-4` |
| MONEY-025 | `1.123456789012` | Accepted if other bounds pass |
| MONEY-025 | `1.1234567890123` | Rejected |
| MONEY-028 | `10.00 USD × 0.9 EUR/USD` | `9.00 EUR` |
| MONEY-030 | `12.345` to 2 places | Display `12.34` under Half-to-Even |
| MONEY-032 | Amount `123.4500` | JSON decimal string, not JSON number |

### Appendix C — Rule traceability

| Rule group | Primary source or decision basis | Classification |
| --- | --- | --- |
| MONEY-001 | Money pattern and numerical representation principles | Design requirement |
| MONEY-002 | Money amount/currency model | Domain invariant |
| MONEY-003 | Exact decimal input requirement | Domain requirement |
| MONEY-004 | Separation of internal and external precision | Domain policy |
| MONEY-005–007 | Explicit project numeric limits recorded in v2.4 | Project engineering policy |
| MONEY-008 | Explicit failure at representational boundaries | Domain requirement |
| MONEY-009 | Currency syntax and normalization policy | Project validation policy |
| MONEY-010–013 | ISO 4217 registry and maintenance | External registry dependency / project registry policy |
| MONEY-014 | Special-purpose currency handling | Domain policy |
| MONEY-015–016 | Currency compatibility and exact arithmetic | Domain invariant |
| MONEY-017 | Division scale and operation-specific rounding; rationale adopted in v2.5.0 | Project arithmetic policy |
| MONEY-018 | Exact intermediate multiplication | Domain arithmetic policy |
| MONEY-019–022 | Rounding defaults, declaration, boundaries, and prohibition on truncation | Project arithmetic policy |
| MONEY-023–028 | ExchangeRate and conversion contracts | Domain and project policies |
| MONEY-029 | IAS 21 integration boundary | Accounting boundary |
| MONEY-030–033 | Formatting and serialization contracts | Public domain/API contract |
| MONEY-034 | ExchangeRate precision decision recorded in v2.2 | Decision record; MONEY-025 is normative |
| Representation capacity disclosure | Implementation limits that affect values permitted by MONEY-005 through MONEY-007 | Conformance transparency and explicit-failure behavior; not a separate numeric domain limit |

---

## 13. Version history

### v2.6.1

- Withdrew MONEY-035 as a standalone normative requirement; the identifier and its former requirement are not part of the current normative rule set.
- Retained the 55-place result as an informative mathematical derivation from MONEY-005 and MONEY-007, without turning it into an independent conformance threshold.
- Clarified in §§1 and 2.3 that implementation representation limits must be disclosed when they affect values or operations otherwise permitted by the normative numeric limits.
- Added a distinct representation-scale-capacity error category and its required detection timing in §§9.2–9.3.
- Updated Appendices B and C to classify the 55-place example as informative and to trace representation-capacity disclosure to the existing conformance and failure contracts.
- Preserved the v2.6.0 history below as a historical record; its description of MONEY-035 records what v2.6.0 introduced and is not a current requirement.
- The normative numeric limits in MONEY-005 through MONEY-008 and the operation-specific scale rules in MONEY-017 and MONEY-025 remain unchanged.

### v2.6.0

- Added MONEY-035 to define effective fractional scale and require support for at least 55 effective fractional places for full conformance.
- Derived the 55-place minimum explicitly from the 28-significant-digit limit in MONEY-005 and the normalized MSD exponent lower bound in MONEY-007.
- Clarified that insignificant trailing-zero preservation remains governed by MONEY-033 and is distinct from effective numeric scale.
- Clarified the required partial-conformance disclosure under §2.3 for implementations that cannot support the MONEY-035 minimum.
- Updated Appendix B with explicit `1e-28`, `1e28`, and 55-place boundary examples.
- Added MONEY-035 to Appendix C traceability.
- This release adds a normative conformance requirement. It does not impose a 28-place scale ceiling and does not change the operation-specific scale rules in MONEY-017 or MONEY-025.

### v2.5.1

- Defined MONEY-007 exponent semantics using the normalized most-significant-digit exponent at the public domain boundary.
- Clarified the interaction between MONEY-007 and the 28-integer-digit limit in MONEY-006 with explicit boundary examples.
- Clarified the documented decimal-conversion contract example in MONEY-001.
- Updated the ISO 4217 reference-use scope to include MONEY-014.
- Listed the Half-Away-From-Zero division mode among project-specific arithmetic policies.
- Recorded the v2.5.0 adoption of the MONEY-017 operation-specific rounding rationale in Appendix C.
- These changes preserve the numeric limits and arithmetic rounding rules established in v2.5.0 while standardizing the public interpretation of the existing exponent bound. Implementations using a different prior exponent convention SHOULD assess boundary compatibility before claiming conformance.

### v2.5.0

- Recast the document as a standalone, implementation-independent Money Domain Specification.
- Removed implementation-status labels, source-availability notes, and the implementation verification checklist.
- Retained the 34 numbered rules and clarified their normative behavior.
- Added conformance requirements and externally observable acceptance criteria.
- Added an Error Contract with semantic error categories and required detection timing.
- Added a glossary.
- Added numeric examples for critical precision, arithmetic, rounding, currency, and conversion rules.
- Documented the 28-significant-digit, 28-integer-digit, and [-28, +28] exponent limits as project policies rather than external-standard requirements.
- Clarified the 12-fractional-place ExchangeRate limit as a project engineering policy and retained MONEY-034 as its decision record.
- Preserved the reference set and rule traceability while clarifying the normative/informative distinction.
- Ratified and documented the rationale for the operation-specific division rounding rule: Half Away From Zero at the 12-place intermediate scale, distinct from the Half-to-Even default and conversion boundary.

### v2.4

- Added formal References and Source Traceability.
- Added ISO 4217, IAS 21 / IFRS Required 2026, JSR 354 1.1, Fowler Money pattern, General Decimal Arithmetic 1.70, and IEEE 754-2019 references.
- Added reference-to-rule mapping.
- Classified references as informative except where rules explicitly identify a normative source.
- Clarified that project precision bounds must not be misattributed to IEEE 754 or other sources.
- Did not establish implementation compliance or test status.
