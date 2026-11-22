# نموذج التزامن — Concurrency Model

- **رمز الوثيقة:** `AE-P8-CONCURRENCY-MODEL`
- **المرحلة:** 8 — التزامن والعزل متعدد الشركات
- **التاريخ:** 2026-11-22
- **القرار المعتمد:** [ADR P8-01](./ard/P8_01_locking_and_isolation.md)
- **الوثائق المرتبطة:** [عقد القفل](./KERNEL_LOCKING_CONTRACT.md)، [تقرير العزل متعدد الشركات](./reports/multi_company_isolation_report.md)، [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md)

---

## 1. السؤال الذي يجيب عليه النموذج

كيف نضمن أن طلبين متزامنين لا يُنتجان قيداً مكرراً، ولا يُجمّدان قيداً أصلياً مرتين، ولا يتشاركان حساباً بترتيب قفل عكسي يؤدي إلى Deadlock، ولا يقرأ أحدهما بيانات شركة أخرى، دون أن تعرف النواة قاعدة البيانات أو تمتلك قفلاً؟

## 2. طبقات النموذج

```text
Caller / SDK
   │  يمرّر company scope و actor و correlation
   ▼
PostingPipeline (internal/kernal/posting)
   │  يتحقق مرة واحدة، يجمّد القيد، يختم الفترة والسياسة والتواريخ والبصمات
   ▼
PostingExecutor (host)
   │  يبني PostingCommit من النسخة المجمّدة فقط
   ▼
StorageAdapter.CommitPosting
   │  يحجز أقفالاً قانونية بالترتيب، ثم يلتزم ذرياً
   ▼
Commit / Rollback
```

النواة لا تملك سهم العودة من الطبقة الأخيرة؛ أي فشل أو نتيجة غير مؤكدة تُترجم إلى sentinel محايد.

## 3. دورة حياة الطلب المتزامن

1. **التحقق وال freezing:** `PostEntry` يقرأ البيانات الرئيسية مرة واحدة باستخدام ساعة محقونة، ويجمّد `FiscalPeriodID` و`PolicyFingerprint` و`EffectiveDate` و`PostingDate` والمبالغ والخطوط. لا يعيد المحول قراءة أي من هذه القيم.
2. **الختم:** `IdempotencyScope` و`RequestFingerprint` و`CallerFingerprint` تُشتق من النسخة المجمّدة وتكتب فوق أي قيمة قدمها المتصل.
3. **النطاق:** `storage.WithCompanyContext` يربط المعاملة بشركة واحدة؛ `ValidateIdempotencyScope` يرفض أي انحراف بـ `ErrScopeViolation`.
4. **الحجز:** `storage.PostingLockTargets` يُنتج أربع فئات؛ `WithOrderedLocks` يحجز تصاعدياً ويحرر تنازلياً.
5. **الالتزام:** حجز idempotency، تسلسل اليومية، القيد، الأرصدة، التدقيق، وروابط العكس تكتب في وحدة واحدة.
6. **النتيجة:** replay يعيد نفس `EntryID`؛ الفشل قبل Commit بلا أثر؛ الفشل بعد Commit يُحل بـ `IdempotencyLookup`.

## 4. سياسة اللقطة (Snapshot Policy)

| العنصر | متى يُجمَّد | من يعيد قراءته؟ |
|---|---|---|
| `FiscalPeriodID` | داخل `PostingPipeline` قبل تفويض Executor | أحد؛ المحول يحفظ القيمة المختومة كما هي |
| `PolicyFingerprint` | مع اختيار السياسة في اللحظة نفسها | أحد |
| `EffectiveDate` / `PostingDate` / `PostedAt` | من ساعة محقونة واحدة | أحد |
| المبالغ والعملات والأسطر | بعد التحقق وقبل الالتزام | أحد |
|Company scope| من هوية الطلب | لا يُخمَّن من المفتاح أو من `ActorID` |

إذا تغيّرت حالة الفترة أو السياسة بعد بدء الطلب، فالتغيير يسري على الطلبات الجديدة فقط. هذا يمنع TOCTOU المحاسبي دون رفع مستوى العزل الافتراضي.

### مستوى العزل

- PostgreSQL يعمل بـ `READ COMMITTED` افتراضياً لأن القيد المجمّد لا يُعاد تفسيره عند الكتابة.
- إذا احتاج المضيف ضمانات قراءة أعلى لعملياته الخاصة، يطلب صراحةً `IsolationRepeatableRead` أو `IsolationSerializable` عبر `storage.TxOptions`.
- لا يُخفى رفع المستوى أو تخفيضه داخل النواة.

## 5. منع الجمود

الشرط الحاكم: **كل من يكتب قيداً يحجز نفس المجموعة من الهويات بنفس الترتيب**.

- الترتيب مشتق من سلسلة قانونية: `v1|company=...|kind=...|id=...`.
- الترتيب لا يعتمد على ترتيب الأسطر، ولا على تاريخ إدراج الحساب، ولا على تكرار الـ `map` العشوائي.
- فئة `original` تُضاف قبل أي تعديل على القيد الأصلي؛ فئة `entry` تمنع مسارين من تعديل نفس القيد.
- فئة `idempotency` تمنع مسارين من حجز نفس العملية حتى لو اختلفت هوية القيد.
- كل حجز مرتبط بشركة؛ لا يتقاطع قفل الشركة A مع B حتى لو تساوت المعرفات نصياً.

الاختبار الحاكم `TestPhase8_CrossAccountReverseOrderNoDeadlock` يرسل قيدين بأسطر معكوسة في اللحظة نفسها؛ نجاح الاثنين قبل انتهاء السياق يعني أن الترتيب مُطبَّق فعلياً. مراقب القفل يثبت ذلك بترتيب `Acquired` و`Released` وليس بمجرد نجاح العملية.

## 6. منع الازدواج

| الحالة | الآلية | النتيجة |
|---|---|---|
| نفس المفتاح/الحمولة، طلبات متعددة | قفل `idempotency` + قيد فريد `(company_id, scope)` | كل الطلبات تُعيد نفس `EntryID` |
| نفس المفتاح، حمولة مختلفة | بصمة `PayloadHash` | `ErrIdempotencyPayloadConflict` |
| عكس نفس الأصل، مفاتيح مختلفة | قفل `original` + فحص حالة الأصل | فائز واحد والباقي `ErrEntryAlreadyReversed` |
| عكس نفس الأصل، نفس المفتاح | قفل `idempotency` | replay يعيد نفس العكس دون ربط ثانٍ |
| قيدان على نفس الحساب بمفتاحين مختلفين | قفل `account` | serialized، كلاهما يُرحَّل مرتين مشروعاً |

## 7. العزل متعدد الشركات

- النواة ترفض الكيانات ذات النطاق المغاير قبل أي استدعاء للمحول (`ValidateCompanyScope`، `ValidateAccountScope`، `ValidateJournalScope`).
- المحول يمرّر شركة المعاملة في `app.current_company_id` ويقفل مفاتيح داخل تلك الشركة فقط.
- كل منفذ قراءة يستلم `CompanyID` صريحاً؛ لا يوجد مسار fallback يبحث عالمياً أو يستنتج الشركة من المعرّف.
- PostgreSQL يفرض RLS على الجداول المالية؛ الذاكرة تعزل الخرائط حسب الشركة. RLS دفاع إضافي ولا يُعتبر بديلاً عن تحقق النواة.
- `ErrScopeViolation` خطأ نهائي، غير قابل لإعادة المحاولة، ويُترجم في SDK إلى `ErrForbidden`.

## 8. تصنيف الأخطاء والتعافي

### قابل لإعادة المحاولة (بحدود المضيف فقط)

`ErrDeadlockDetected`، `ErrSerializationFailure`، `ErrConcurrentModification`، `ErrIdempotencyInProgress`، و`context.DeadlineExceeded`.

### نهائي

`ErrUniqueConstraint`، `ErrEntryAlreadyReversed`، `ErrIdempotencyPayloadConflict`، `ErrScopeViolation`، `ErrAtomicCommitFailed`، `ErrInvalidLockTarget`.

### سياسة المضيف

```text
DefaultRetryPolicy = 3 retries, 10ms initial, 100ms cap, exponential backoff
MaxAllowedRetries  = 5
RetryPolicy.Validate() rejects negative/oversized values
ExecuteWithRetry(ctx, runner, opts, policy, fn) respects cancellation
```

بعد خطأ «نتيجة غير معروفة» (فقدان الاستجابة بعد Commit) يُجرى `IdempotencyLookup` آمن أولاً؛ لا يُعاد تنفيذ القيد إلا إذا ثبت أنه لم يُكمل. لا يوجد retry داخل `internal/kernal` ولا داخل المحول.

## 9. أدلة التشغيل

| البوابة | الدليل |
|---|---|
| لا Deadlock | `TestPhase8_CrossAccountReverseOrderNoDeadlock` + `TestPhase8_LockOrderEnforced` عبر `LockObserver` |
| لا ترحيل مكرر | `TestPhase8_ConcurrentReplayAtScale` و`TestPhase7_ConcurrentReplayHasSingleEffect` |
| لا عكس مكرر | `TestPhase8_ConcurrentReversalSingleWinner` و`TestPhase7_ReversalAtomicity` |
| لا تسريب | `TestPhase8_InterleavedCompanyIsolation`، `TestPhase8_CompanyScopeOnEveryPort`، `TestPhase8PostgresRLSRuntimeIsolation` |
| لا TOCTOU | `TestPhase8_FrozenSnapshotRemainsStableUnderConcurrentStateChange` |
| قفل فعلي | `TestPhase8_LockOrderEnforced` عبر `LockObserver` |
| Race | `go test -race ./... -count=1` |
| معمارية | `./scripts/verify_architecture.sh` → 11/11 PASS |

> ملاحظة: أسماء الاختبارات في الجدول أعلاه هي أسماء خارج `internal/kernal`؛ المرجع المُلزم لها هو [مجموعة اختبارات العقد](./KERNEL_CONTRACT_TEST_SUITE.md) وتقرير [المرحلة 8](./KERNEL_REMEDIATION_PHASE8_REPORT.md).

## 10. الحدود المتبقية

- المحول الذاكرة لا يوفّر متانة بعد إعادة التشغيل؛ ذلك مُعلَن في `AdapterCapabilities`.
- قفل idempotency الاستشاري في PostgreSQL يكفي داخل عنقود قاعدة بيانات واحد؛ التوسّع العابر لمناطق متعددة يحتاج قرار منفصل خارج نطاق المرحلة 8.
- RLS مُنفَّذ في المحول فقط؛ النواة لا تعرفه ولا تعتمد عليه.
- مراقب القفل اختباري فقط؛ لا يُستخدم في الإنتاج لقياس الأداء.
