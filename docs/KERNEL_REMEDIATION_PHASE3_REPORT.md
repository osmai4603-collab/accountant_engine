# KERNEL Phase 3 Execution Report — Main-Data & Fiscal-Calendar Integrity

**Status: PASS**  
**Execution date:** 2026-11-22  
**Repository commit baseline:** `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`  
**Scope:** `internal/kernal/masterdata/**`, related ledger reference models, kernel Reader contracts, and Phase 3 documentation.

---

## 1. Objective

This report records the execution and verification of Phase 3 (*سلامة البيانات الرئيسية والتقويم المالي*) of the kernel remediation plan in `docs/KERNEL_REMEDIATION_EXECUTION_PLAN.md`.

Phase 3 hardens the kernel's master-data invariants: company-scoped identity, chart-of-accounts hierarchy integrity, effective-dated account validity, posting-time clock determinism, journal identity, fiscal-period boundaries, and close-state decisions. It also closes the reference-model drift loop for auto-generated ledger reference structs and documents the remaining host-adapter work.

---

## 2. Preconditions

| Gate | Evidence | Result |
|---|---|---|
| Phase 0 remediation complete | `docs/KERNEL_REMEDIATION_PHASE0_REPORT.md` — PASS | ✅ |
| Phase 1 remediation complete | `docs/KERNEL_REMEDIATION_PHASE1_REPORT.md` — PASS | ✅ |
| Phase 2 remediation complete | `docs/KERNEL_REMEDIATION_PHASE2_REPORT.md` — PASS | ✅ |
| No Phase 4 work started | Scope stayed within Phase 3 tasks 1–15; no storage/SQL adapter, no money/composite postings, no posting-engine orchestration changes | ✅ |

---

## 3. Task Coverage (Plan → Evidence)

| # | Required task | Execution evidence |
|---|---|---|
| 1 | Fail-closed company scope validation | `ValidateCompanyScope` rejects zero company before repository access (`ErrCompanyScopeRequired`); `TestCompanyScopeFailsClosedWithoutRepositoryAccess` |
| 2 | COA hierarchy cycle/duplicate/scope detection | `DetectCyclicHierarchy` detects cross-company parent identity mismatch and mixed-scope snapshots (`ErrAccountIdentityMismatch`); `TestAccountHierarchyRejectsCrossCompanyLinks` |
| 3 | Parent-account scope and self-reference checks | `ValidateAccountScope` rejects zero-scope and identity-mismatched records; enforced on the `ValidateCanPostToAccount*` paths (existing scope tests also cover mismatch cases) |
| 4 | Effective-dated account validity | `Account.Validate`/`IsValid` reject inverted windows via `ErrInvalidEffectiveRange`; `TestAccountValidateRejectsReversedEffectiveRange` and `FuzzEffectiveRangeValidationNeverPanics` |
| 5 | Deterministic posting-time clock | `Clock`/`SystemClock` port, `ValidateCanPostToAccountWithClock`, and `ValidateCanPostToAccountAsOf`; covered by `TestEffectiveDatingUsesInjectedClockAndVersionedReader` |
| 6 | Optional versioned effective account reader | `EffectiveAccountReader.GetAccountAsOf` added without breaking `Reader`; same reader test proves version selection and fail-closed legacy fallback; adapter gap opened as `G-019` |
| 7 | Journal identity validation | `ValidateJournalScope` + `ErrJournalIdentityMismatch` added; `TestJournalReaderScopeIsValidated`. The posting pipeline already performs an equivalent inline ID/company check in `posting/pipeline.go` |
| 8 | Fiscal calendar month-end and UTC boundaries | `TestFiscalCalendarBoundaryMatrix` |
| 9 | Leap-year day boundaries | Included in `TestFiscalCalendarBoundaryMatrix` (leap day and non-leap February) |
| 10 | Fiscal year, month, and 12-period validation | `TestFiscalCalendarBoundaryMatrix` plus the existing fiscal-calendar validation tests |
| 11 | Adjustment period 13/14 explicit handling | `TestFiscalCalendarBoundaryMatrix` asserts periods 13/14 have explicit adjustment types and are excluded from normal date search |
| 12 | Soft-close state matrix | `TestCloseStateMatrixForPeriodAndYear` |
| 13 | Reference-model drift automation | `TestReferenceModelsDoNotDriftSilently` compares authority and DTO field sets/kinds, with a documented drop set; P1-03 addendum records the convention |
| 14 | Adversarial input and panic-safety checks | `TestMasterDataAdversarialInputsFailClosed`, `FuzzEffectiveRangeValidationNeverPanics` |
| 15 | Phase 3 gate evidence and documentation | This report, ADR `P3_01`, invariant clarifications, traceability matrix, gap register, architecture map, and updated `doc.go` |

---

## 4. Production Changes

### 4.1 Fail-closed identity and hierarchy

- `internal/kernal/masterdata/company_scope.go`
  - Added `ErrCompanyScopeRequired`.
  - `ValidateCompanyScope` returns the error for zero company IDs instead of silently accepting empty scope.
- `internal/kernal/masterdata/coa.go`
  - Cross-company parent references and mixed-company snapshots in `DetectCyclicHierarchy` return `ErrAccountIdentityMismatch`.
  - `ValidateCanPostToAccountAsOf` rejects zero company scope before account lookup.
  - `ValidateAccountScope` now rejects zero-scope and identity-mismatched records.
- `internal/kernal/masterdata/journal.go`
  - Added `ValidateJournalScope` for zero-company and journal identity checks on reader-returned journals.
- `internal/kernal/masterdata/fiscal_calendar.go`
  - `ValidatePeriodForPostingWithSoftClose` and `ValidateAdjustmentPeriodForPostingWithSoftClose` reject zero company IDs before Reader calls.
- `internal/kernal/masterdata/types.go`
  - Added `ErrInvalidEffectiveRange` and `ErrJournalIdentityMismatch`.

### 4.2 Effective dating and deterministic time

- `internal/kernal/masterdata/ports.go`
  - Added `Clock`, `SystemClock`, and `EffectiveAccountReader`.
- `internal/kernal/masterdata/account.go`
  - `Account.Validate` and `Account.IsValid` reject inverted effective windows.
- `internal/kernal/masterdata/coa.go`
  - Added `ValidateCanPostToAccountWithClock`.
  - Added `ValidateCanPostToAccountAsOf`, which uses `EffectiveAccountReader` when implemented and remains fail-closed otherwise.
  - Kept the legacy `ValidateCanPostToAccount` signature for compatibility, routed through `SystemClock` and fail-closed fallback.

### 4.3 Reference-model drift

- `internal/kernal/masterdata/masterdata_integrity_test.go`
  - `TestReferenceModelsDoNotDriftSilently` uses reflection to compare authority model fields against `ledger` DTO fields and kinds, including an explicit documented drop set for intentional projections.
- `docs/ard/P1_03_reference_model_duality.md`
  - Phase 3 addendum documents the drop-set convention; new or removed authority fields now require an explicit test/ADR update instead of silently drifting.

---

## 5. Test-First Changes

All production fixes were preceded by failing tests in:

- `internal/kernal/masterdata/masterdata_integrity_test.go`

Coverage includes:

- Fail-closed zero-company scope.
- Cross-company COA hierarchy and journal identity mismatch.
- Reversed effective ranges.
- Injected clock and versioned reader behavior.
- Calendar boundaries (month end, leap year, UTC).
- Adjustment periods 13 and 14.
- Close-state decision matrix.
- Reference-model drift reflection.
- Adversarial inputs and fuzz-based panic detection.

No failing test was deleted or weakened.

---

## 6. Documentation and ADR Updates

| File | Change |
|---|---|
| `docs/ard/P3_01_masterdata_effective_dating.md` | New ADR for Phase 3 decisions and compatibility trade-offs |
| `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` | Added non-normative Phase 3 clarifications for rules 016–020 |
| `internal/kernal/KERNEL_TRACEABILITY_MATRIX.md` | Updated rows 016–020 and added Phase 3 appendix |
| `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` | Distribution updated to 0 Critical / 5 High / 10 Medium / 4 Low / 19 total; `G-016` partial; `G-019` opened |
| `docs/KERNEL_ARCHITECTURE_MAP.md` | Reader port table updated with `EffectiveAccountReader` |
| `docs/ard/P1_03_reference_model_duality.md` | Phase 3 drift-automation addendum |
| `internal/kernal/masterdata/doc.go` | Updated charter to describe Phase 3 guarantees |

No 28th invariant was added; Phase 3 semantics remain clarifications of existing rules 016–020.

---

## 7. Gap Status

| Gap | Status | Notes |
|---|---|---|
| `G-016` | Partial | Kernel-side reflection drift detection is implemented and passing. API bridge verification remains scheduled for Phase 11 as planned. |
| `G-019` | Open (High) | Host/storage adapters do not yet implement `EffectiveAccountReader` or retain effective-dated account versions. Kernel validation is fail-closed until an adapter supplies the optional interface. This is host/adapter work and is outside Phase 3 kernel scope. |

No new Critical gaps were introduced. The register distribution is now **0 Critical / 5 High / 10 Medium / 4 Low / 19 total**.

---

## 8. Verification Evidence

All commands were run from the repository root with Go `1.26.3` (`go.mod`: `go 1.26.0`, `toolchain go1.26.3`).

| Check | Command | Result |
|---|---|---|
| Formatting | `gofmt -l internal/kernal/` | PASS — no output |
| Kernel tests | `go test ./internal/kernal/... -count=1` | PASS — all 10 kernel packages |
| Race detector | `go test -race ./internal/kernal/... -count=1` | PASS — all 10 kernel packages |
| Baseline script | `FUZZTIME=3s ./scripts/verify_kernel_baseline.sh /tmp/opencode/phase3` | `RESULT: PASS` |
| API compatibility | `./scripts/check_api_compat.sh` | `API compatibility check: passed (no removals or signature changes)` |
| Whitespace/patch hygiene | `git diff --check` | PASS — no output |
| Architecture gate | Repository architecture gate script | PASS — 11/11 checks |
| Full repository tests | Full `go test` incl. external consumer module | PASS |
| Dedicated fuzz run | `FuzzEffectiveRangeValidationNeverPanics`, 5s | PASS — ~185K execs, 0 panic |
| Coverage | `go test -cover ./internal/kernal/...` | `total: 93.2%` (Phase 2 baseline: 93.1%) |
| Test inventory | `grep` counts | 174 `Test` functions, 14 `Fuzz` targets |

Verification artifacts are preserved under `/tmp/opencode/phase3/logs/` and `/tmp/opencode/phase3/kernel.cover`. The environment snapshot there predates this report file.

---

## 9. Residual Risks

1. **Host adapter adoption (`G-019`):** Until host adapters implement `EffectiveAccountReader`, as-of account validation is fail-closed. This is intentional and safe but reduces the number of successful historical postings that can be validated with a precise effective-dated account version.
2. **API bridge drift (`G-016`):** Kernel-side reflection drift detection is automated, but the runtime API bridge check remains a Phase 11 task.
3. **Legacy calling conventions:** `ValidateCanPostToAccount` is retained as a compatibility wrapper and uses `SystemClock`. New posting paths must use `ValidateCanPostToAccountWithClock` or `ValidateCanPostToAccountAsOf` to preserve determinism.

---

## 10. Gate Decision

All Phase 3 tasks 1–15 are implemented and verified. Phase 3 exit criteria are satisfied:

- No new Critical gaps.
- No weakening or deletion of failing tests.
- No float usage in kernel logic.
- No direct current time in domain logic.
- No map-order dependence.
- No ignored errors.
- Kernel scope preserved; no SQL/storage adapter implementation added.

**PHASE 3 GATE: PASS**

No commit or push was performed. Changes remain in the working tree for review.
