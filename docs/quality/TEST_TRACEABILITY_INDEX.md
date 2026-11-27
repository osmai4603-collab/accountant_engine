# فهرس التتبع الموحّد للاختبارات — Unified Test Traceability Index

**الغرض:** جدول ربط واحد بين متطلبات الجودة (`ACC/DOM/TXN/MON/LIFE/AUD/ARC/ISO/TEST/PERF/OPS`)، وقواعد النواة القابلة للاختبار (`RULE-KERNEL-001..027`)، وأسماء اختبارات `internal/kernal`، وسيناريوهات عقود `test/contract_parity`، ومسار الدليل.

**الحالة:** فهرس تنقّل/ربط فقط. **لا يُثبت نجاحاً**؛ الحالة والتغطية تُؤخذ من تشغيل موثّق بـ Run ID/Commit/Environment/Evidence URI وفق [مواصفة البوابات](QUALITY_GATES_SPECIFICATION.md) و[وثيقة الاعتماد](../reports/KERNEL_RELEASE_SIGNOFF.md). لا يوجد قرار اعتماد سارٍ حالياً.

**المصادر التي يُوحّدها هذا الفهرس:**

- [مصفوفة تتبع النواة](../../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md) — أسماء `Test...`/`Fuzz...` داخل `internal/kernal`.
- [مجموعة اختبارات العقد](../contracts/KERNEL_CONTRACT_TEST_SUITE.md) — سيناريوهات `TestPhase7_*` و`TestPhase8_*` و`TestPhase11_*` على المحولين.
- [فهرس حزمة اختبارات المطابقة](COMPLIANCE_TEST_SUITE_INDEX.md) — معرفات `REF-*` ومصفوفة المتطلبات/البوابات.

---

## 1. جدول الربط الموحّد

> كل اسم في العمودين الثالث والرابع هو دالة اختبار فعلية في شجرة العمل. عمود «الدليل» يشير إلى المكان الذي يُسجَّل فيه تشغيل الاختبار، لا إلى نتيجة مضمونة.

| Requirement ID | REF-ID | RULE-KERNEL ذو الصلة | اختبار(ات) النواة الممثِّلة | سيناريو العقد المرابط | مسار الدليل |
| --- | --- | --- | --- | --- | --- |
| `ACC-001` توازن القيد | `REF-ACC-001` | 001، 002 | `TestDoubleEntryMathematicalValidation`، `TestRoundBalancedEntrySettlesRoundedFunctionalDifference` | — | `internal/kernal/posting/*_test.go`، `internal/kernal/types/*_test.go` |
| `ACC-002` سلامة سطر القيد | `REF-ACC-002` | 003، 009 | `TestPostingValidationLineAndDoubleEntryBranches`، `TestAdjustingEntriesRequireMetadataAtValidation` | — | `internal/kernal/posting/entry_invariants_test.go` |
| `ACC-003` إشارة المبلغ | `REF-ACC-003` | 003 | `TestComparisonsAndNegative`، `TestInvalidAmounts` | — | `internal/kernal/types/money_test.go` |
| `ACC-004` قابلية الترحيل | `REF-ACC-004` | 010، 017، 019 | `TestPostEntryIdentityAndPolicyRejectMatrix`، `TestPostEntryExecutorNeverRunsForRejectedInputs` | `TestPhase7_KernelStampedDatesArePreserved` | `internal/kernal/posting/pipeline_invariants_test.go`، `test/contract_parity/phase7_transaction_contract_test.go` |
| `ACC-005` صلاحية الفترة | `REF-ACC-005` | 018، 019، 020 | `TestFiscalCalendarBoundaryMatrix`، `TestValidatePeriodForPostingRejectsDateOutsideYear` | — | `internal/kernal/masterdata/masterdata_integrity_test.go` |
| `ACC-006` عدم تعديل التاريخ | `REF-ACC-006` | 009، 012 | `TestValidateImmutabilityRejectsPostedChanges`، `TestReversalBuilderLeavesOriginalImmutable` | `TestPhase7_KernelStampedDatesArePreserved`، `TestPhase8_FrozenSnapshotRemainsStableUnderConcurrentStateChange` | `internal/kernal/journal/state_machine_test.go`، `test/contract_parity/phase8_concurrency_isolation_test.go` |
| `ACC-007` Idempotency | `REF-ACC-007` | 010، 027 | `TestRequestFingerprintIsCanonicalAndPayloadSensitive`، `TestIdempotencyScopeValidatesAllUniquenessComponents` | `TestPhase7_IdempotentReplayAndPayloadConflict`، `TestPhase11_RevaluationReplayWithSamePayload` | `internal/kernal/posting/transaction_contract_test.go`، `test/contract_parity/phase11_revaluation_contract_test.go` |
| `ACC-008` اتساق العملة | `REF-ACC-008` | 004، 006، 008 | `TestMoneyOperationsPreserveCurrency`، `TestSameCurrencyRateValidatesCodeAndDateBeforeReturningOne` | — | `internal/kernal/types/money_integrity_test.go`، `internal/kernal/valuation/conversion_precision_test.go` |
| `ACC-009` عزل الدفتر | `REF-ACC-009` | 016 | `TestCompanyScopeValidation`، `TestProjectPostedMovementsIsDeterministic` | `TestPhase8_InterleavedCompanyIsolation` | `internal/kernal/masterdata/masterdata_integrity_test.go`، `docs/reports/multi_company_isolation_report.md` |
| `ACC-010` عزل المستأجر | `REF-ACC-010` | 016، 027 | `TestCompanyScopeFailsClosedWithoutRepositoryAccess` | `TestPhase8_CompanyScopeOnEveryPort`، `TestPhase11_CompanyScopeOnEveryMasterDataPort` | `internal/kernal/masterdata/masterdata_integrity_test.go`، `test/contract_parity/phase8_concurrency_isolation_test.go` |
| `ACC-011` تتبع المصدر | `REF-ACC-011` | 025، 026 | `TestCalculateHashGoldenVector`، `TestAuditCheckpointDetectsDeletedTail` | `TestPhase11_RevaluationAuditAction` | `internal/kernal/governance/*_test.go`، `docs/contracts/KERNEL_AUDIT_THREAT_MODEL.md` |
| `ACC-012` اتساق الرصيد | `REF-ACC-012` | 006، 007، 024 | `TestMinorUnitsContractAcrossRegistry`، `TestRebuildAccountBalancesDeterministic` | `TestPhase11_BalanceReaderAsOfAndCurrencyParity` | `internal/kernal/ledger/projection_test.go`، `test/contract_parity/phase11_g019_g022_contract_test.go` |
| `ACC-013` التوازن الوظيفي | `REF-ACC-013` | 001، 002، 021، 022، 023، 024 | `TestDoubleEntryMathematicalValidation`، `TestLiabilityRevaluationDirectionIsOppositeToAsset` | — | `docs/contracts/KERNEL_VALUATION_RULES_MATRIX.md` |
| `DOM-001` حماية Aggregate | `REF-DOM-001` | 009، 011، 012، 013 | `TestEntryStatusStateMachine`، `TestJournalEntryEnsureMutable` | — | `internal/kernal/journal/state_machine_test.go` |
| `TXN-001` Atomicity/Rollback | `REF-TXN-001` | 027 | `TestPostEntryStopsWhenAtomicExecutorFails` | `TestPhase7_FaultInjectionAtomicity`، `TestPhase7_ReversalAtomicity` | `docs/reports/fault_injection_report.md` |
| `TXN-002` Concurrency Control | `REF-TXN-002` | 027 | `TestOrderedAccountKeysAreDeterministicAndScoped` | `TestPhase8_LockOrderEnforced`، `TestPhase8_ConcurrentReplayAtScale`، `TestPhase8_CanonicalCompanyLockOrder` | `docs/contracts/KERNEL_LOCKING_CONTRACT.md` |
| `MON-001` التمثيل والتقريب | `REF-MON-001` | 005، 006، 007 | `TestMoneySignificantDigitsBoundary`، `TestBankersRoundingTieToEvenProperty` | — | `docs/contracts/KERNEL_MONETARY_PRECISION_MATRIX.md` |
| `LIFE-001` دورة الحياة والفترات | `REF-LIFE-001` | 011، 012، 013، 014، 015 | `TestReversalLifecycle`، `TestPostingOpeningAndAuditAdjustmentPeriodBranches` | `TestPhase7_ReversalAtomicity` | `internal/kernal/posting/reversal.go`، `docs/contracts/KERNEL_POSTING_FLOW_SPECIFICATION.md` |
| `AUD-001` التدقيق ومنع العبث | `REF-AUD-001` | 025، 026 | `TestAuditSnapshotVerificationDetectsTampering`، `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint` | `TestPhase11_RevaluationAuditAction` | `docs/contracts/KERNEL_AUDIT_REPOSITORY_CONTRACT.md` |
| `ARC-001` اتجاه الاعتماد | `REF-ARC-001` | — | `TestKernelArchitectureUsesPureInterfaces`، `TestKernelFinancialCodeHasNoFloatConversions` | `scripts/verify_architecture.sh` | `internal/kernal/doc_test.go` |
| `ISO-001` عزل Tenant/Ledger | `REF-ISO-001` | 016 | `TestMasterDataAdversarialInputsFailClosed` | `TestPhase8_ScopeViolationRejected` | `docs/reports/multi_company_isolation_report.md` |
| `TEST-001` إطار المرجع/الخاصية/الفشل | `REF-TEST-001` | 001..027 | `TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests` | كل سيناريوهات `TestPhase*` في [مجموعة العقد](../contracts/KERNEL_CONTRACT_TEST_SUITE.md) | `internal/kernal/doc_test.go` |
| `PERF-001` SLO الأداء | `REF-PERF-001` | — | `benchmark` في `internal/kernal/posting/benchmark_test.go` | `test/benchmark/...` | `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` — **`E4 Not Verified`** |
| `OPS-001` الاستعادة/الترحيل/التوافق | `REF-OPS-001` | — | `TestDRDrill_FullRecovery` (خارج `internal/kernal`) | `test/dr/...` | `docs/operations/runbooks/` — **`E4 Not Verified`** |

## 2. قواعد النواة الأخرى دون مقابل `ACC` مباشر

| القاعدة | الاختبار(ات) الممثِّلة | سيناريو العقد أو المرجع |
| --- | --- | --- |
| `RULE-KERNEL-014` تاريخ العكس | `TestReversalDraftResolvesDestinationPeriodDuringPosting` | `docs/contracts/KERNEL_POSTING_FLOW_SPECIFICATION.md` |
| `RULE-KERNEL-015` الفترتان 13/14 | `TestPostingOpeningAndAuditAdjustmentPeriodBranches` | `docs/specifications/kernel/JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.2.0.md` |
| `RULE-KERNEL-020` توليد السنة المالية | `TestGenerateFiscalYearPreservesCustomStartDay` | `docs/specifications/kernel/MASTERDATA_DOMAIN_SPECIFICATION_v1.2.0.md` |
| `RULE-KERNEL-026` نقطة التفتيش | `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint` | `docs/contracts/KERNEL_AUDIT_THREAT_MODEL.md` |
| `RULE-KERNEL-027` ذرية المضيف | `TestPostEntryStampsIdempotencyContractOnExecutorOptions` | `TestPhase7_FaultInjectionAtomicity` |

## 3. قواعد الاستخدام

1. هذا جدول ربط لا مصدر أدلة؛ لا تُضاف حالة `Pass` إليه قبل تسجيل تشغيل في [سجل التشغيل](COMPLIANCE_TEST_SUITE_INDEX.md).
2. عند تعارض اسم اختبار بين المصادر الثلاثة، يُعتمد الاسم الموجود فعلياً في `*_test.go` ويُحدَّث المصدر الآخر.
3. عند إضافة متطلب أو سيناريو جديد، تُضاف صف واحد هنا أولاً، ثم يُحال إليه من الفهارس الثلاثة.
4. الرقم «27» يخص `RULE-KERNEL-001..027` فقط، ولا يُخلط بـ 13 ثابت `ACC` أو 47 معرّف `RULE-ACC` في [قواعد المحاسبة](../rules/ACCOUNTING_BUSINESS_RULES.md).
