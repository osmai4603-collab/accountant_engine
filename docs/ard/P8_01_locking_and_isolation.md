# ADR P8-01 — ترتيب الأقفال،Snapshotالترحيل، والعزل متعدد الشركات

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernal/posting` + مالكو المحولات
- **المرجع:** [الخطة، المرحلة 8](../plans/KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [الفجوة G-002](../plans/KERNEL_REMEDIATION_GAP_REGISTER.md)
- **ذو صلة:** [ADR P1-02](./P1_02_gl_source_of_truth.md)، [ADR P5-01](./P5_01_policy_selection_and_historical_interpretation.md)، [عقد المعاملة](../contracts/KERNEL_POSTING_TRANSACTION_CONTRACT.md)، [عقد idempotency](../contracts/KERNEL_IDEMPOTENCY_CONTRACT.md)

## السياق

1. `internal/kernal/posting/lock_manager.go` يرتب معرفات الحسابات ولا يحجز أي قفل؛ تعليقه القديم كان يدّعي أنه «يضمن قفل صفوف خالٍ من الجمود». هذه فجوة `G-002` ذات أولوية High.
2. النواة لا تملك معاملة أو قفل أو SQL؛ الذرية منفذة في `storage/memory` و`storage/postgres` وفق المرحلة 7. لكن الترتيب الحتمي للقفل لم يكن عقداً قابلاً للرصد في المحولين.
3. صحة Company Scope كانت تُختبر في النواة وفي سجل idempotency، لكن لم يكن هناك اختبار مشترك يغطي كل منافذ القراءة والكتابة مع طلبات متداخلة لشركتين، ولا خطأ محايد لخرق النطاق.
4. تزامن تغيير حالة الفترة/السياسة مع الترحيل يحتاج سياسة Snapshot صريحة؛ لا يجوز أن يُعاد تفسير قيد مرحّل بقراءة حيّة لبيانات رئيسية تغيّرت بعد التحقق.

## القرار

1. **النواة توعّد ولا تُقفل:** `ExtractAndSortAccountIDs` و`OrderedAccountKeys` في `posting/lock_manager.go` ينتجان مفاتيح فريدة مرتبة حتمياً. لا يقفلان، لا يحجزان حالة، ولا يستدعيان وقتاً أو عشوائية. أي قفل فعلي مسؤولية Adapter/Host فقط.
2. **مفاتيح التزامن:** تغطي وحدة عمل الترحيل أربع فئات: الحسابات (كل حساب يلمسه سطر)، القيد نفسه، القيد الأصلي عند العكس، ومفتاح idempotency بالنطاق الكامل. التمثيل القانوني للمفتاح هو `v1|company=<id>|kind=<kind>|id=<value>` ويُفرز كنص UTF-8. لا يُشتق الترتيب من `map` ولا من ترتيب الأسطر الوارد.
3. **عقد SPI محايد:** تُضاف في `pkg/spi/storage` أنواع `LockTarget` و`LockKind` و`OrderLockTargets` و`WithOrderedLocks` و`PostingLockTargets`، مع مراقب قفل اختياري يُمرَّر عبر `context` لقياس الترتيب الفعلي في الاختبارات. لا يعرف العقد SQL ولا أي محرك تخزين.
4. **المحولات تُنفّذ الترتيب فعلياً:** `storage/memory` يحجز أقفالاً مسماة بالترتيب القانوني قبل قفل الوحدة؛ `storage/postgres` يحجز أقفالاً استشارية داخل معاملة الترحيل بنفس الترتيب قبل أي كتابة. قفل idempotency الحالي وفلح الأصل عند العكس يبقى داخل نفس المعاملة وبنفس عقد المرحلة 7. لا تُضاف هجرة: الأقفال الاستشارية لا تغير المخطط.
5. **Snapshot policy:** التحقق من البيانات الرئيسية والسياسات يتم مرة واحدة داخل `PostingPipeline`، وتُجمّد النتيجة في القيد (بما فيها `FiscalPeriodID` و`PolicyFingerprint` والتواريخ والمبالغ). المحول يحفظ النسخة المجمّدة ولا يعيد قراءة البيانات الرئيسية لتفسيرها عند Commit. لذلك لا يوجد TOCTOU محاسبي: أي تغيير لاحق في الفترة أو السياسة يؤثر على العمليات الجديدة فقط. PostgreSQL يعمل بمستوى `READ COMMITTED` افتراضي لأن القيد المجمّد لا يُعاد تفسيره؛ إذا احتاج مضيف لضمانات قراءة أعلى لعملياته الخاصة، يطلب صراحةً `IsolationRepeatableRead` أو `IsolationSerializable` عبر `TxOptions`.
6. **Scope Violation خطأ محايد جديد:** `storage.ErrScopeViolation` يُرفض به أي قيمة نطاق لا تطابق شركة القيد (بما فيها بصمة idempotency المزوّرة). غير قابل لإعادة المحاولة، ويُترجم في SDK إلى `ErrForbidden`. لا يُعتبر `ActorID` تفويضاً.
7. **تصنيف الأخطاء المتزامنة:** `ErrDeadlockDetected` و`ErrSerializationFailure` و`ErrConcurrentModification` و`ErrIdempotencyInProgress` قابلة لإعادة المحاولة بسياسة مضيف محدودة؛ `ErrUniqueConstraint` و`ErrEntryAlreadyReversed` و`ErrIdempotencyPayloadConflict` و`ErrScopeViolation` و`ErrAtomicCommitFailed` نهائية.
8. **إعادة المحاولة في المضيف فقط:** `storage.DefaultRetryPolicy` و`ExecuteWithRetry` هما الموضع المعتمد، بحد أقصى صريح وزمن رجوع محدود وسياق ملغي. لا `for {}` ولا retry داخل `internal/kernal` ولا داخل محول يخفي نتيجة غير معروفة؛ الاستعلام الآمن بالمفتاح يسبق أي إعادة تشغيل بعد انقطاع.

## البدائل المرفوضة

| البديل | سبب الرفض |
| --- | --- |
| جعل `lock_manager.go` يحجز أقفالاً process-local | ينقل قفلاً وهمياً إلى النواة ويخالف حدود Embedded Core؛ لن يحمي أكثر من عملية/مثيل PostgreSQL. |
| وضع SQL `FOR UPDATE` داخل `internal/kernal/posting` | ينتهي R3c/R3d من بوابة المعمارية ويكسر استقلال التخزين. |
| الاعتماد على قفل idempotency وحده لمنع الجمود بين الحسابات | قفل المفتاح لا يحمي حساباً مشتركاً بين مفتاحين مختلفين. |
| إعادة قراءة الفترة/السياسة داخل Adapter قبل Commit | يكرر منطق المجال في المحول، ويسمح بتغيير التفسير التاريخي؛ ممنوع في عقد المرحلة 7. |
| اعتبار RLS بديلاً عن تحقق النواة من النطاق | RLS آلية دفاع في المحول؛ النواة تظل مسؤولة عن رفض الكيانات ذات النطاق المغاير. |
| retry غير محدد داخل المحول | يخفي الأخطاء النهائية وقد يضاعف الأثر بعد فقدان الاستجابة. |

## التبعات والمسؤوليات

- **النواة:** مفاتيح مرتبة حتمية، توثيق أنها ليست أقفالاً، اختبارات خصائص الترتيب، وتثبيت اللقطة قبل التفويض؛ لا تعرّف أخطاء قفل أو نطاق خاصة بقاعدة بيانات.
- **SPI:** عقد قفل محايد ومراقب اختباري و`ErrScopeViolation` وسياسة retry محدودة.
- **المحولات:** تطبيق الترتيب داخل وحدة العمل، Company Scope في كل منفذ، واختبارات تشغيل العقد نفسها على الذاكرة وPostgreSQL.
- **المضيف/SDK:** ترجمة الأخطاء، سياسة إعادة محاولة محدودة، واستعلام آمن بالمفتاح عند unknown outcome.
- **الوثائق:** [Concurrency Model](../contracts/KERNEL_CONCURRENCY_MODEL.md)، [Locking Contract](../contracts/KERNEL_LOCKING_CONTRACT.md)، وتقرير [العزل متعدد الشركات](../reports/multi_company_isolation_report.md).

## الأدلة التنفيذية

`TestOrderedAccountKeysAreDeterministicAndScoped`، `TestLockTargetsAreCanonicalAndSorted`، `TestWithOrderedLocksAcquiresAndReleasesInOrder`، واختبارات `TestPhase8_*` في `test/contract_parity/phase8_concurrency_isolation_test.go` على المحولين. لا تُعتبر الفجوة `G-002` مغلقة إلا بنجاح هذه الاختبارات على المحولين مع `go test -race`.
