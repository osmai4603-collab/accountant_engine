# عقد القفل — Kernel Locking Contract

- **رمز الوثيقة:** `AE-P8-LOCKING-CONTRACT`
- **المرحلة:** 8 — التزامن والعزل متعدد الشركات
- **التاريخ:** 2026-11-22
- **القرار المعتمد:** [ADR P8-01](../architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md#adr-p8-01)
- **الخطة:** [المرحلة 8، المهام 1–4 و14–15](../plans/KERNEL_REMEDIATION_EXECUTION_PLAN.md)
- **الوثائق المرتبطة:** [نموذج التزامن](./KERNEL_CONCURRENCY_MODEL.md)، [تقرير العزل متعدد الشركات](../reports/multi_company_isolation_report.md)، [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md)، [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md)

---

## 1. القاعدة الحاكمة

النواة تنتج **ترتيباً حتمياً لمفاتيح القفل**، والمحول (أو المضيف) ينفّذ **القفل الفعلي** داخل معاملته. لا يوجد قفل داخل `internal/kernal`، ولا SQL، ولا `pgx`، ولا حالة قفل مشتركة بين الطلبات.

الفجوة `G-002` كانت تنشأ من وصف دالة `ExtractAndSortAccountIDs` بأنها «تضمن قفل صفوف خالٍ من الجمود» بينما هي `sort.Slice` فقط. بعد المرحلة 8 أصبح الوصف والسلوك متطابقين:

- `AccountLockKey` و`OrderedAccountKeys` في `internal/kernal/posting/lock_manager.go` يُنتجان مفاتيح نصية مرتبة، بلا حجز.
- `storage.LockTarget` و`WithOrderedLocks` في `pkg/spi/storage/locks.go` يُعرّفان كيف يُنفّذ المحول الحجز الفعلي.
- `storage/memory` و`storage/postgres` يستدعيان العقد نفسه داخل `CommitPosting`.

## 2. مفاتيح التزامن

### 2.1 الفئات الأربع

| `LockKind` | الهوية (`ID`) | لماذا تُقفل |
|---|---|---|
| `account` | `AccountID` لكل سطر في القيد | منع الترتيب العدائي بين قيدين يشتركان في حساب أو يعكسان ترتيب أسطرهما |
| `entry` | `JournalEntryID` للقيد المرحّل | منع تعديل قيد واحد من مسارين في اللحظة نفسها |
| `original` | `JournalEntryID` للقيد الأصلي عند العكس | فرض فائز واحد عند ربط العكس بالأصل |
| `idempotency` | نطاق idempotency الكامل `company=...\|operation=...\|key=...` | منع الترحيل المكرر لنفس العملية حتى لو اختلفت هوية القيد |

قفل idempotency يستخدم **النطاق الكامل** لا نص المفتاح وحده؛ لذلك يمكن أن يتشارك مفتاح نصي بين `post` و`reverse` دون تضارب زائف.

### 2.2 الشكل القانوني

```text
v1|company=<companyID>|kind=<kind>|id=<value>
```

- `v1` إصدار ثابت للبروتوكول؛ أي تغيير في شكل المفتاح يجب أن يرفع الإصدار.
- `companyID` جزء من الهوية؛ لا يجوز قفل معرّف حساب أو قيد دون شركة.
- المقارنة والفرز نصيان على سلسلة UTF-8؛ لا يُشتق الترتيب من `map` ولا من ترتيب الأسطر الوارد.
- `OrderLockTargets` يُعيد نسخة جديدة، ويحذف التكرار، ويرتّب تصاعدياً، ولا يعدّل الإدخال.

## 3. مسؤوليات الطبقات

| الطبقة | تملك | لا تملك |
|---|---|---|
| `internal/kernal/posting` | اشتقاق مفاتيح الحسابات، ترتيبها، تجميد القيد وختم النطاق/البصمات | معاملة، قفل فعلي، retry، SQL، معرفة محرك التخزين |
| `pkg/spi/storage` | `LockTarget`، `OrderLockTargets`، `WithOrderedLocks`، `LockObserver`، `ValidateIdempotencyScope`، أخطاء محايدة | أي تنفيذ قفل خاص بمحول |
| `storage/memory` | أقفال مسماة داخل العملية عبر `sync.Map` وقنوات حساسة للـ context | إعادة تعريف الترتيب أو تجاهل نطاق الشركة |
| `storage/postgres` | `pg_advisory_xact_lock(hashtextextended(canonical, salt))` داخل المعاملة | هجرة مخطط جديدة؛ الأقفال الاستشارية لا تغيّر المخطط |
| `pkg/sdk` | نسخ الحقول المختومة إلى `PostingCommit`، تصنيف الأخطاء، سياسة retry محدودة | قفل داخل النواة أو retry غير محدود |

## 4. سلسلة الحجز في `CommitPosting`

1. التحقق من `ctx`.
2. `storage.ValidateIdempotencyScope(companyID, operation, idempotency)`؛ أي انحراف يُرفض بـ `ErrScopeViolation` قبل أي كتابة.
3. اشتقاق `storage.PostingLockTargets(commit)` من الحقول المختومة.
4. `storage.WithOrderedLocks(ctx, targets, acquire, fn)`:
   - يُعيد الترتيب والتوحيد داخل العقد؛
   - يتحقق من كل هدف؛
   - يحجز بالترتيب التصاعدي؛
   - يُنفّذ `fn`؛
   - يُحرّر بالترتيب العكسي حتى عند الفشل؛
   - لا يُنفّذ `fn` إطلاقاً إذا فشل أي حجز.
5. داخل `fn`: حجز idempotency، تسلسل اليومية، كتابة القيد والأثر والتدقيق، ثم الربط النهائي في معاملة واحدة.

### 4.1 التنفيذ في الذاكرة

`acquireNamedLock` يستخدم `postingLocks sync.Map` بمفتاح `target.Canonical()`، وقناة بسعة واحدة. الحجز هو إرسال إلى القناة مع `select` على `ctx.Done()`؛ التحرير هو استلام من القناة. لا يمكن لقفل أن يُترك محتجزاً بعد عودة `WithOrderedLocks`.

### 4.2 التنفيذ في PostgreSQL

`acquireOrderedAdvisoryLock` يتحقق من وجود `transactionScope` ومن تطابق شركة الهدف مع شركة المعاملة، ثم ينفّذ:

```sql
SELECT pg_advisory_xact_lock(hashtextextended($1, $2))
```

- `$1` = `target.Canonical()`
- `$2` = salt ثابت `0x5048415345`، مختلف عن salt قفل idempotency القديم (`0`) وقفل شركة/يومية، لمنع تصادم الهويات.
- التحرير تلقائي عند `COMMIT`/`ROLLBACK`؛ دالة التحرير `no-op` تُمرَّر إلى `WithOrderedLocks` حتى يُسجَّل التحرير بالترتيب العكسي للمراقب.

## 5. مراقب القفل

`LockObserver` هو hook اختباري يُمرَّر عبر `storage.WithLockObserver(ctx, observer)`:

```go
type LockObserver interface {
    Acquired(target LockTarget)
    Released(target LockTarget)
}
```

الإنتاج لا يثبّت مراقباً. اختبارات العقد تستخدمه لتثبت أن المحول حجز فعلياً، بالترتيب القانوني، ويُحرِّر بالترتيب العكسي. ترتيب المصفوفة وحده لا يكفي؛ المراقب يستدعى من داخل دالة الحجز نفسها.

## 6. Company Scope في القفل

- كل هدف يحمل `CompanyID`؛ مفاتيح الشركة A لا تُطابق مفاتيح الشركة B حتى لو تساوت معرفات الحساب/القيد/المفتاح نصياً.
- في PostgreSQL، محاولة حجز هدف بشركة مختلفة عن شركة المعاملة تُرفض بـ `ErrScopeViolation` ولا تُحوَّل إلى خطأ قفل عابر.
- في الذاكرة، قفل idempotency ونطاق الشركة كلاهما جزء من مفتاح idempotency؛ لا يوجد fallback يبحث في كل الشركات.

## 7. تصنيف الأخطاء وإعادة المحاولة

| الخطأ | قابل لإعادة المحاولة | المعنى |
|---|---|---|
| `ErrDeadlockDetected` (40P01) | نعم — تحذير تصميمي | ترتيب قفل غير قانوني أو مسار كتابة خارج العقد |
| `ErrSerializationFailure` (40001) | نعم | تعارض قراءة/كتابة في مستوى عزل أعلى |
| `ErrConcurrentModification` | نعم | تعديل متزامن لنفس الحالة |
| `ErrIdempotencyInProgress` | نعم | حجز مفتاح قيد التنفيذ |
| `ErrUniqueConstraint` (23505) | لا | تعارض نهائي في هوية/قيد فريد |
| `ErrEntryAlreadyReversed` | لا | العكس مُطبَّق بالفعل |
| `ErrIdempotencyPayloadConflict` | لا | نفس المفتاح بحمولة مختلفة |
| `ErrScopeViolation` | لا | خرق حد مستأجر؛ يُترجم في SDK إلى `ErrForbidden` |
| `ErrInvalidLockTarget` | لا | خطأ برمجة في بناء الهدف |
| `ErrAtomicCommitFailed` | لا | خرق ذرية؛ يجب تحقيقه لا إعادة تشغيله |

إعادة المحاولة حصراً في المضيف:

```text
storage.DefaultRetryPolicy(): MaxRetries=3, InitialBackoff=10ms, MaxBackoff=100ms
storage.MaxAllowedRetries = 5
RetryPolicy.Validate() يرفض الحدود غير المنضبطة
ExecuteWithRetry(ctx, runner, opts, policy, fn)
```

لا `for {}` داخل النواة أو المحول، ولا retry بعد نتيجة غير معروفة دون `IdempotencyLookup` آمن أولاً.

## 8. دليل الامتثال

- `internal/kernal/posting/lock_manager_test.go`: حتمية الترتيب، عزل الشركة، عدم تغيير الإدخال، وعدم حمل حالة تحت التزامن.
- `pkg/spi/storage/locks_test.go`: الشكل القانوني، التحقق، التوحيد، الحجز/التحرير، فشل الحجز، نطاق idempotency، وحد retry.
- `test/contract_parity/phase8_concurrency_isolation_test.go`: إثبات الحجز الفعلي على المحولين، نفس الحساب، الترتيب العكسي، عاصفة replay، العكس المتزامن، تداخل الشركات، كل المنافذ، اللقطة المجمدة، وخرق النطاق.
- `storage/postgres/phase8_isolation_test.go`: عزل RLS وقت التشغيل بمستخدم غير superuser.

## 9. الحكم

العقد مكتمل: الترتيب حتمي، الحجز فعلي في المحولين، النطاق إلزامي، التحرير مضمون، والتصنيف يمنع إعادة المحاولة على أخطاء نهائية. إغلاق `G-002` مشروط بنجاح هذه الاختبارات مع `go test -race`، وقد تحقق ذلك في [تقرير المرحلة 8](../reports/KERNEL_FINAL_VERIFICATION_REPORT.md).
