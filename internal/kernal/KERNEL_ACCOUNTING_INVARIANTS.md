# Kernel Accounting Invariants

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

## Valuation and audit invariants

- `RULE-KERNEL-021`: Revaluation is limited to monetary positions and valid
  closing/historical/spot/average rate types.
- `RULE-KERNEL-022`: Rate effective dates may not be after the valuation date.
- `RULE-KERNEL-023`: Asset gains/losses and liability gains/losses produce
  opposite debit/credit position effects as appropriate.
- `RULE-KERNEL-024`: Revaluation journal differences must be nonzero and
  balanced in the functional currency.
- `RULE-KERNEL-025`: Audit logs must be sequential, company-scoped, linked to
  the previous hash, and have a hash covering every audited semantic field.
- `RULE-KERNEL-026`: Audit verification uses one consistent log snapshot and
  verifies an externally retained checkpoint against that snapshot.
- `RULE-KERNEL-027`: Atomic posting and audit persistence are host executor
  responsibilities. The kernel validates and returns the snapshot; it does not
  claim database or network transaction capabilities.

