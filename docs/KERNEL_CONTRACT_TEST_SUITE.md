# مجموعة اختبارات العقد — Contract Test Suite

- **رمز الوثيقة:** `AE-P8-CONTRACT-SUITE`
- **المرحلة:** 7–11 — المعاملات والذرية ومنع التكرار، التزامن والعزل متعدد الشركات، ثم تكافؤ المحولات والعقود
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)، المهام 12–15 (المرحلة 7)، 1–15 (المرحلة 8)، و1–17 (المرحلة 11)
- **الموقع:** `test/contract_parity/phase7_transaction_contract_test.go`، `test/contract_parity/phase8_concurrency_isolation_test.go`، و`test/contract_parity/phase11_*.go`
- **الوثائق المرتبطة:** [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md)، [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md)، [عقد ذرية العكس](./KERNEL_REVERSAL_ATOMICITY_CONTRACT.md)، [عقد القفل](./KERNEL_LOCKING_CONTRACT.md)، [نموذج التزامن](./KERNEL_CONCURRENCY_MODEL.md)، [تقرير العزل](./reports/multi_company_isolation_report.md)، [تقرير حقن الأعطال](./reports/fault_injection_report.md)

---

## 1. المبدأ

المجموعة محايدة عن نوع قاعدة البيانات: تُكتب مرة ضد `storage.Ports`، وتُشغَّل على Memory وPostgreSQL من خلال `ForEachStorageAdapter`. لا تعرف الاختبارات SQL ولا أسماء جداول ولا محرك تخزين. أي Adapter جديد يمر بالمنافذ نفسها يمر بنفس المجموعة دون تعديل الاختبار.

## 2. متطلبات Harness

| المنفذ | الغرض في المجموعة |
|---|---|
| `storage.PostingWriter` | تنفيذ `CommitPosting` ذرياً |
| `storage.EntryReader` | قراءة القيد بعد الالتزام أو بعد الفشل للتأكد من عدم وجود أثر |
| `storage.BalanceReader` | التأكد من أن الأثر المالي واحداً أو صفراً |
| `storage.IdempotencyLookup` | استعلام آمن بعد replay وunknown outcome |
| `storage.WithFaultInjector` | حقن أعطال محايدة في نقاط حدود المعاملة |
| `storage.WithLockObserver` (عبر `context`) | قياس ترتيب `Acquired`/`Released` الفعلي داخل المحول دون كشف SQL |
| `storage.WithCompanyContext` | ربط المعاملة بشركة واحدة؛ أي انحراف نطاق يُرفض بـ `ErrScopeViolation` |
| `storage.PortCatalog()` | قائمة مرجعية آلية تُقارن بحقول `storage.Ports` لاكتشاف منفذ غير موثّق |
| `storage.ValidateAdapterCapabilities()` | فحص صارم لإصدار العقد وهوية المحول قبل قبوله كدليل تكافؤ |
| `storage.PolicyStore` (عبر `Ports.Private`) | إلحاق نسخ السياسات وسردها مع منع التداخل وعزل الشركة |
| منافذ `pkg/spi/masterdata` الاختيارية | النسخة النافذة ومراكز التكلفة والأبعاد؛ غيابها يبقى فشلاً مغلقاً في SDK |

يُنشئ `NewPostgresStorageHarness` مخططاً معزولاً لكل حالة، ويُطبّق الهجرات المطلوبة، ثم يُسقط المخطط عند التنظيف. إذا تعذّر الاتصال بقاعدة الاختبار دون `POSTGRES_URL` صريح، يُتخطى PostgreSQL؛ هذا التخطي ليس دليلاً نهائياً، والدليل المقبول يتطلب تشغيل PostgreSQL فعلي.

## 3. سيناريوهات المرحلة 7

| الاختبار | السيناريو | ما يثبته |
|---|---|---|
| `TestPhase7_IdempotentReplayAndPayloadConflict` | `IdempotentReplayAndPayloadConflict` | نفس المفتاح والبصمة يُعيد القيد نفسه؛ بصمة مختلفة تُرفض؛ السجل المكتمل قابل للاستعلام |
| `TestPhase7_KernelStampedDatesArePreserved` | `KernelStampedDates` | `PostingDate` و`PostedAt` المختومان في النواة يُحفظان ويُعادان كما هما |
| `TestPhase7_FaultInjectionAtomicity` | `FaultInjectionAtomicity` | الأعطال قبل Commit لا تترك أثراً؛ الخطأ بعد Commit يُحل بالاستعلام وإعادة النتيجة |
| `TestPhase7_ReversalAtomicity` | `ReversalAtomicity` | العكس والربط وحالة الأصل ذرية؛ العكس الثاني مرفوض |
| `TestPhase7_ConcurrentReplayHasSingleEffect` | `ConcurrentReplay` | طلبات متزامنة بالمفتاح نفسه تُنتج قيداً واحداً وأثراً واحداً |
| `TestPhase7_CompanyScopeIsolation` | `CompanyScopeIsolation` | سجل idempotency لا يتسرب بين الشركات |

### نقاط الحقن المغطاة

| نقطة | الموضع | التوقع |
|---|---|---|
| `FaultBeforeIdempotencyClaim` | قبل حجز المفتاح | لا سجل ولا قيد ولا أثر |
| `FaultBeforeAudit` | قبل تثبيت التدقيق | Rollback كامل؛ لا حركة بلا تدقيق |
| `FaultBeforeCommit` | بعد تجهيز كل الكتابات وقبل ظهورها | لا أثر مرئي، الأصل لا يتغير في العكس |
| `FaultAfterCommit` | بعد الالتزام وقبل الاستجابة | الالتزام دائم؛ lost response؛ replay يعيد `EntryID` |

### 3.1 سيناريوهات المرحلة 8

كل سيناريو في الجدول أدناه يُشغَّل عبر `ForEachStorageAdapter` على Memory وPostgreSQL، ما لم يُذكر خلاف ذلك:

| الاختبار | السيناريو | ما يثبته |
|---|---|---|
| `TestPhase8_LockOrderEnforced` | `LockOrderEnforced` | المحول يحجز فعلياً بالترتيب القانوني ويُحرِّر بالترتيب العكسي؛ `LockObserver` يستدعى من داخل دالة الحجز لا من ترتيب المصفوفة |
| `TestPhase8_SameAccountConcurrentPosting` | `SameAccountConcurrentPosting` | قيدان بمفتاحين مختلفين يلمسان الحساب نفسه؛ كلاهما يُرحَّل والأثر 200، فلا يُخلط serialization مع منع التكرار |
| `TestPhase8_CrossAccountReverseOrderNoDeadlock` | `CrossAccountReverseOrderNoDeadlock` | أسطر معكوسة ومراقب مستقل لكل طلب؛ لا Deadlock قبل انتهاء السياق المحدد |
| `TestPhase8_ConcurrentReplayAtScale` | `ConcurrentReplayAtScale` | 16 منافساً بالمفتاح نفسه؛ `EntryID` واحد وأثر 100 واحد |
| `TestPhase8_ConcurrentReversalSingleWinner` | `ConcurrentReversalSingleWinner` | 12 منافساً يعكسون الأصل نفسه؛ عكس واحد، الأصل مرتبط مرة واحدة، والباقي replay ناجح |
| `TestPhase8_InterleavedCompanyIsolation` | `InterleavedCompanyIsolation` | شركتان متداخلتان بمعرّفات متشابهة؛ سجلات idempotency وأرصدة وأقفال كل شركة داخل نطاقها |
| `TestPhase8_CompanyScopeOnEveryPort` | `CompanyScopeOnEveryPort` | `EntryReader` و`IdempotencyLookup` و`BalanceReader` و`MasterDataReader` لا تعيد بيانات الشركة الأخرى؛ لا fallback عالمي |
| `TestPhase8_FrozenSnapshotRemainsStableUnderConcurrentStateChange` | `FrozenSnapshotRemainsStable` | إضافة فترة مالية بعد بدء الطلب لا تُغيّر `FiscalPeriodID` أو `PostingDate` أو `PostedAt` المختومة |
| `TestPhase8_ScopeViolationRejected` | `ScopeViolationRejected` | نطاق idempotency مزوّر لشركة أخرى يُرفض بـ `ErrScopeViolation` قبل كتابة قيد أو سجل مفتاح |

فحص SPI مستقل في الملف نفسه: `TestPhase8_CanonicalCompanyLockOrder` يثبت أن هوية القفل تشمل `CompanyID`، وأن `OrderLockTargets` يوحّد ويرتّب دون تعديل الإدخال. اختبار PostgreSQL إضافي خارج المجموعة المشتركة: `TestPhase8PostgresRLSRuntimeIsolation` في `storage/postgres/phase8_isolation_test.go` يثبت عزل RLS وقت التشغيل بدور غير superuser.

### 3.2 سيناريوهات المرحلة 11

| الاختبار | السيناريو | ما يثبته |
|---|---|---|
| `TestPhase11_PortCatalogCoversEveryPort` | `PortCatalog` | كل حقل في `storage.Ports` له مدخل موثّق في القائمة المرجعية |
| `TestPhase11_PortsValidateChecksEveryRequiredPort` | `PortsValidate` | إزالة أي منفذ إلزامي تُرفض مع تسميته صراحةً |
| `TestPhase11_ContractVersionRejectsIncompatibleAdapters` | `AdapterCapabilitiesValidated` | رفض أي إصدار عقد غير `storage.spi/v1` على المحولين |
| `TestPhase11_BoundedRetryOnAdapterUnitOfWork` | `BoundedRetryOnUnitOfWork` | التعارف العابر يُعاد محاولته مرة واحدة ويُنتج أثراً واحداً |
| `TestPhase11_ConnectionInterruptionBeforeCommit` | `ConnectionInterruptionBeforeCommit` | انقطاع قبل Commit لا يترك أثراً، وإعادة المحاولة تُكمل مرة واحدة |
| `TestPhase11_CompanyScopeOnEveryMasterDataPort` | `CompanyScopeOnEveryMasterDataPort` | كل طرق `MasterDataReader` ترفض بيانات شركة أخرى |
| `TestPhase11_BalanceReaderAsOfAndCurrencyParity` | `BalanceReaderAsOfAndCurrency` | `asOf` والعملة الفارغة لهما الدلالة نفسها على المحولين |
| `TestPhase11_PolicyVersionHistoryIsAppendOnlyAndNonOverlapping` | `PolicyVersionHistory` | رفض التداخل وتكرار بداية النفاذ، وحفظ الحمولة بلا إعادة كتابة |
| `TestPhase11_PolicyVersionCompanyScope` | `PolicyVersionCompanyScope` | سجل السياسات معزول بالشركة |
| `TestPhase11_RevaluationReplayWithSamePayload` | `RevaluationReplay` | نفس نطاق/حمولة إعادة التقييم يُعيد القيد نفسه |
| `TestPhase11_RevaluationPayloadConflictIsTerminal` | `RevaluationPayloadConflict` | الحمولة المختلفة تُرفض بلا أثر إضافي |
| `TestPhase11_RevaluationConcurrentReplayHasSingleWinner` | `RevaluationConcurrentReplay` | منافسون متعددون، قيد واحد وأثر واحد |
| `TestPhase11_RevaluationFaultAfterCommitLookupAndReplay` | `RevaluationUnknownOutcome` | فقدان الاستجابة بعد Commit يُحل بالاستعلام وإعادة النتيجة |
| `TestPhase11_RevaluationScopeIsSeparateFromPostAndReverse` | `RevaluationScopeSeparation` | العملية الثالثة مستقلة عن `post` و`reverse` |
| `TestPhase11_RevaluationCompanyScopeIsolation` | `RevaluationCompanyScope` | نطاق إعادة التقييم معزول بالشركة |
| `TestPhase11_RevaluationAuditAction` | `RevaluationAuditAction` | سجل التدقيق يحمل `Action=revaluation` |

يُكمل هذا الجدول اختبارات `G-019`/`G-022` للنسخة النافذة ومراكز التكلفة والأبعاد، واختبارات SDK لتحميل سياسات كل الإصدارات وعدم ابتلاع الأخطاء. تُنشر النتائج الفعلية من تشغيل المجموعة كاملة فقط.

## 4. اختبارات النواة والـ SPI المرافقة

`internal/kernal/posting/transaction_contract_test.go` يثبت الجانب الذي تملكه النواة:

- تحقق النطاق الثلاثي ورفض العناصر الناقصة.
- حتمية `FingerprintRequest` وحساسية البصمة للتغييرات المالية وعدم حساسيتها لترتيب الأسطر أو الـ correlation.
- ختم `Scope` و`RequestFingerprint` و`CallerFingerprint` فوق قيم المتصل قبل التفويض.
- تصنيف الأخطاء القابلة وغير القابلة لإعادة المحاولة.

اختبارات المرحلة 8 المرافقة:

- `internal/kernal/posting/lock_manager_test.go`: `TestOrderedAccountKeysAreDeterministicAndScoped` و`TestOrderedAccountKeysHandlesEmptyEntries` و`TestOrderedAccountKeysAreStatelessUnderConcurrency` تثبت أن النواة تُنتج مفاتيح مرتبة ومعزولة بالشركة بلا حالة قفل أو حجز فعلي.
- `pkg/spi/storage/locks_test.go`: `TestLockTargetsAreCanonicalAndSorted` و`TestLockTargetRejectsMissingScope` و`TestPostingLockTargetsCoverAllConcurrencyKeys` و`TestWithOrderedLocksAcquiresAndReleasesInOrder` و`TestWithOrderedLocksReleasesAcquiredTargetsOnFailure` و`TestValidateIdempotencyScopeRejectsForeignCompany` و`TestRetryPolicyIsBounded`.
- `pkg/sdk/classify_error_phase8_test.go` يثبت أن `storage.ErrScopeViolation` يُترجم إلى `publicapplication.ErrForbidden` ولا يُعامل كخطأ عابر.

هذه الاختبارات لا تستورد SQL أو حزم تخزين ملموسة؛ بوابة `verify_architecture.sh` تمنع ذلك آلياً.

## 5. التشغيل

```bash
# النواة فقط
go test ./internal/kernal/posting -count=1

# المجموعة على المحولين
go test ./test/contract_parity -count=1 -v
# أو مع DSN صريح:
POSTGRES_URL='postgres://...' go test ./test/contract_parity -count=1 -v

# سيناريوهات المرحلة 8 فقط
go test ./test/contract_parity -run '^TestPhase8_' -count=1 -v
go test ./storage/postgres -run '^TestPhase8PostgresRLSRuntimeIsolation$' -count=1 -v
go test ./internal/kernal/posting ./pkg/spi/storage -count=1 -race

# سيناريوهات المرحلة 11 فقط (لا تُحدّث التقرير)
go test ./test/contract_parity -run '^TestPhase11_' -count=1 -v

# المستودع كاملاً
go test ./... -count=1
go test -race ./... -count=1
```

تُكتب تقارير التكافؤ تلقائياً إلى:

- `docs/reports/contract_parity_report.md`
- `test/contract_parity/parity_report.md`

هذه المخرجات **مولّدة آلياً**؛ لا تُحرر يدوياً. منع `TestMain` تشغيلات `-test.run` الفرعية من إعادة كتابة التقرير، حتى لا تُمحى أدلة السيناريوهات غير المنفذة. الدليل النهائي يعتمد على تشغيل `go test ./... -count=1` كاملاً. هذا السلوك موثّق في `G-010`.

## 6. الحكم

المجموعة ناجحة عندما:

- كل سيناريو `PASS` على Memory وPostgreSQL بلا اختلاف دلالي.
- لا يوجد قيد أو مفتاح أو رصيد أو تدقيق بعد الأعطال السابقة لـ Commit.
- نتيجة ما بعد Commit قابلة للاستعادة بالمفتاح.
- لا يستورد كود النواة أي تفاصيل قاعدة بيانات.
- المحول يُثبت حجزاً فعلياً بالترتيب القانوني وتحريراً عكسياً عبر `LockObserver`، ولا يكتفي بترتيب مصفوفة في النواة.
- لا Deadlock تحت الترتيب العدائي، ولا ترحيل/عكس/إعادة تقييم مكرر تحت الضغط، ولا تسريب بين الشركات، ولا TOCTOU بلا سياسة لقطة مجمدة.
- سجل السياسات غير قابل لإعادة التفسير، و`BalanceReader` متطابق دلالياً بين المحولين، وأي فرق مقصود مُعلَن صراحةً عبر `DeclareSemanticDifference` لا مُخترَع في المولّد.
- `go test -race ./... -count=1` نظيف، وتقرير التكافؤ المولّد يعيد جميع السيناريوهات المشتركة PASS على المحولين بعد تشغيل كامل مع PostgreSQL فعلي.

النتائج الفعلية لهذه الجلسة في [تقرير التكافؤ](./reports/contract_parity_report.md)، [تقرير حقن الأعطال](./reports/fault_injection_report.md)، [تقرير العزل متعدد الشركات](./reports/multi_company_isolation_report.md)، و[تقرير المرحلة 8](./KERNEL_REMEDIATION_PHASE8_REPORT.md).
