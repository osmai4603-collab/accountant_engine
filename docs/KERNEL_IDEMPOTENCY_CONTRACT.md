# عقد منع التكرار — Idempotency Contract

- **رمز الوثيقة:** `AE-IDEMPOTENCY-CONTRACT`
- **المرحلة:** 7 و11 — المعاملات والذرية ومنع التكرار، ثم تكافؤ نطاق `revaluation` بين المحولات
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)، المهام 4–9 (المرحلة 7) والمهمة 17 (المرحلة 11)
- **التنفيذ المرجعي:** `internal/kernal/posting/transaction.go`، `internal/kernal/posting/pipeline.go`، `pkg/spi/storage/ports.go`، `pkg/spi/storage/errors.go`
- **الوثائق المرتبطة:** [عقد معاملة الترحيل](./KERNEL_POSTING_TRANSACTION_CONTRACT.md)، [ذرية العكس](./KERNEL_REVERSAL_ATOMICITY_CONTRACT.md)، [مجموعة اختبارات العقد](./KERNEL_CONTRACT_TEST_SUITE.md)

---

## 1. الغرض

يمنع هذا العقد أن ينتج عن إعادة طلب الترحيل أو العكس قيد أو حركة مالية ثانية، ويحدد سلوك التكرار والتعارض والتزامن وفقدان الاستجابة بعد الالتزام. النواة تملك تعريف النطاق والبصمة؛ المحول يملك الحجز الدائم والفهرسة الفريدة والاستعلام الآمن.

القاعدة الأساسية: **المفتاح وحده ليس هوية العملية**. الهوية الكاملة هي ثلاثة عناصر يختمها النواة قبل التفويض.

## 2. نطاق التفرد

```text
IdempotencyScope = CompanyID + Operation + Key
Scope.String()   = "company=<id>|operation=<post|reverse|revaluation>|key=<trimmed key>"
```

| العنصر | المصدر | القاعدة |
| --- | --- | --- |
| `CompanyID` | `entry.CompanyID` بعد Clone والتحقق | لا يُقبل نطاق شركة فارغ أو مختلف عن القيد |
| `Operation` | `post` افتراضياً، و`reverse` إذا كان `ReversalOfID != nil`، و`revaluation` عند تلميح نطاق موثَّق في `PostOptions.Scope` | نفس نص المفتاح في الترحيل والعكس وإعادة التقييم يمثل ثلاث عمليات مختلفة |
| `Key` | `strings.TrimSpace(opts.IdempotencyKey)` | مفتاح فارغ أو مسافات يُرفض بـ `ErrIdempotencyKeyRequired` (أو `ErrIdempotencyScopeRequired` عند بناء النطاق) |

`BuildIdempotencyScope` هو المصدر الوحيد لهذه القيم. يُكتب فوق `Scope` الذي يرسله المتصل قبل استدعاء Executor، باستثناء تلميح `revaluation` الموثَّق: يقبل النواة العملية الثالثة فقط إذا تطابق `Scope.CompanyID` مع شركة القيد و`Scope.Key` مع `opts.IdempotencyKey`؛ وأي انحراف يُرفض بـ `ErrIdempotencyScopeRequired`. شركة النطاق ومفتاحه لا يُؤخذان أبداً من المتصل. الدليل: `TestPostEntryStampsIdempotencyContractOnExecutorOptions`، `TestBuildIdempotencyScopeHonorsValidatedRevaluationHint` و`TestBuildIdempotencyScopeRejectsForgedRevaluationHint`.

## 3. بصمة الطلب

هناك بصمتان canonical بصيغة `sha256:<hex>`:

| البصمة | الوظيفة | من يستخدمها |
| --- | --- | --- |
| `RequestFingerprint` | digest للنسخة المجمدة بعد التحقق والختم، بما يشمل `FiscalPeriodID` و`PolicyFingerprint` و`PostingDate` و`PostedAt` | `(PostOptions.RequestFingerprint)` يُسلَّم إلى Executor لوصف اللقطة التي قررتها النواة بالضبط |
| `CallerFingerprint` | digest للحقول التي يملكها المتصل قبل التحويل، ويبقى ثابتاً عبر خطوات الترحيل | `PostingCommit.Idempotency.PayloadHash` في SDK، ويُستخدم للكشف عن إعادة المفتاح بحمولة مختلفة قبل وبعد الترحيل |

قواعدCanonical موحدة:

- إدخال صريح الحقول؛ لا تسلسل map ولا اعتماد على ترتيب الشرائح.
- الأسطر تُفرز بـ `ID` قبل التجزئة؛ ترتيب الأسطر في الطلب لا يغيّر البصمة.
- الأزمنة UTC بتنسيق RFC3339Nano؛ القيم الصفرية والـ nil تمثّل بـ `-`.
- المبالغ Decimal بصيغة نصية canonical؛ لا.float64 في أي مسار مالي.
- `CorrelationID` بيانات نقل مستبعدة؛ مفتاح idempotency نفسه مستبعد من بصمة الحمولة لأنه يعرّف العملية لا محتواها.
- تغيير المبلغ أو الحساب أو العملة أو التاريخ أو الفاعل أو السبب يغيّر البصمة.

الدليل: `TestRequestFingerprintIsCanonicalAndPayloadSensitive` و`TestRequestFingerprintIgnoresLineOrder`.

> الفرق بين البصمتين مقصود: `RequestFingerprint` يصف «ماذا حُفظ فعلاً»، و`CallerFingerprint` يصف «ماذا أرسل المتصل». استخدام إحداهما مكان الأخرى يسبب تعارضاً زائفاً بعد أن تملأ النواة حقول الفترة والسياسة والتاريخ.

## 4. حالة التنفيذ

| الحالة | المعنى | نهائية؟ |
| --- | --- | --- |
| `started` | المفتاح محجوز والتنفيذ جارٍ داخل معاملة غير مكتملة | لا |
| `completed` | وحدة العمل التزمت بالكامل؛ `EntryID` و`Response` محفوظان | نعم |
| `retryable_failure` | فشل مؤقت؛ يمكن إعادة المحاولة تحت سياسة مضيف محدودة | لا |
| `terminal_failure` | فشل نهائي لا تتغير نتيجته بإعادة الطلب | نعم |

لا يوجد انتقال من `started` إلى `completed` خارج نفس معاملة الحفظ. لا يوجد سجل `started` يبقى مرئياً بعد Rollback. لا يُنشأ سجل نهائي قبل Commit.

## 5. سلوك إعادة الطلب

| الحالة المرصودة | السلوك |
| --- | --- |
| لا يوجد سجل | المتابعة إلى التحقق والتفويض؛ المحول يحجز المفتاح ذرياً داخل التنفيذ |
| `completed` والبصمة نفسها | إعادة `EntryID` والقيد المحفوظ؛ صفر كتابة جديدة؛ `IdempotentReplay = true` |
| أي حالة والبصمة مختلفة | `ErrIdempotencyPayloadConflict`؛ لا كتابة، ولا محاولة «تحديث» السجل |
| `started` | `ErrIdempotencyInProgress`؛ إعادة محاولة محدودة بعد فحص آمن، لا تنفيذ موازٍ |
| `terminal_failure` | فشل نهائي؛ لا يُعاد استخدام المفتاح بنفس البصمة |
| `retryable_failure` | الفشل مغلقاً؛ يقرر المضيف إعادة المحاولة وفق سياسته |

يُجري SDK فحصاً آمنة عبر `storage.IdempotencyLookup.Lookup` قبل تكرار منطق السياسة؛ هذا الفحص للقراءة فقط ولا يحجز مفتاحاً. الحجز النهائي وإطلاقه مملوكان لـ `CommitPosting` داخل المحول. واجهة `FindPostedEntryByIdempotencyKey` توفر الاستعلام العام عن نتيجة مكتملة.

## 6. التزامن وفائز واحد

- التفرد على `(company_id, scope)` وعلى `(company_id, operation, idempotency_key)` يمنع سجلين لنفس العملية.
- في PostgreSQL يُستخدم قفل استشاري داخل المعاملة على النطاق، ثم قيود فريدة؛ تعارض الإدراج يُترجم إلى `ErrConcurrentModification`.
- في الذاكرة يُستخدم قفل المحول ونسخة لقطة؛ الطلب الثاني يرى السجل المكتمل ويعيد نفس `EntryID`.
- الدليل: `TestPhase7_ConcurrentReplayHasSingleEffect` على المحولين، بمجموعة طلبات متزامنة وأثر مالي واحد.

العكس له قاعدة إضافية: حتى المفتاح المختلف لا يسمح بعكس الأصل مرتين؛ يُرفض بـ `ErrEntryAlreadyReversed`. تفاصيله في [عقد ذرية العكس](./KERNEL_REVERSAL_ATOMICITY_CONTRACT.md).

## 7. unknown outcome بعد Commit

إذا انقطع الاتصال أو انتهت المهلة بعد أن أصبح الالتزام دائماً:

1. يُعاد خطأ للمتصل؛ لا يُفترض أن العملية لم تحدث.
2. يُجرى `Lookup(companyID, key)`؛ لا تُستخدم تخمينات ولا إنشاء قيد جديد.
3. إذا كانت الحالة `completed` يُعاد `EntryID` المحفوظ، وتُعيد إعادة الطلب نفس النتيجة.
4. إذا لم يُعثر على سجل، فالفشل حدث قبل Commit ولم يترك أثراً.

نقطة الحقن `FaultAfterCommit` تحاكي فقدان الاستجابة بعد الالتزام. الدليل: `TestPhase7_FaultInjectionAtomicity`.

## 8. تصنيف الأخطاء

| الخطأ | التصنيف | سبب التصنيف |
| --- | --- | --- |
| `ErrIdempotencyInProgress` | retryable | قد ينتهي التنفيذ الآخر ويصبح نتيجته متاحة |
| `ErrIdempotencyStorageUnavailable` | retryable | المخزن قد يعود؛ الترحيل لا يتم بدونه |
| `ErrConcurrencyConflict` | retryable | تعارض تسلسلي يُعاد تشغيله من بداية وحدة العمل |
| `context.DeadlineExceeded` | retryable | يجب أولاً التحقق بالمفتاح قبل أي إعادة إرسال |
| `ErrIdempotencyConflict` | non-retryable | تغيير البصمة مستحيل دون طلب جديد صريح |
| `ErrReversalAlreadyApplied` | non-retryable | العكس الثاني محظور نهائياً |
| `ErrRevaluationAlreadyApplied` | non-retryable | نطاق إعادة التقييم استُخدم بنتيجة محاسبية مختلفة |
| `ErrAtomicityViolation` | non-retryable | خلل في المحول لا تحلّه إعادة المحاولة |

`posting.IsRetryable` و`storage.IsRetryable` ينفّذان التصنيف عبر sentinels وواجهة `Retryable() bool`؛ `RetryableError.Unwrap` يحفظ سبب الخطأ الأصلي.

## 9. العقد المطلوب من المحول

```go
type PostingIdempotency struct {
    Key         string // مفتاح المتصل بعد Trim
    Scope       string // company=<id>|operation=<...>|key=<...>
    PayloadHash string // CallerFingerprint من النواة
}
```

- `PostingCommit.Idempotency` اختياري توافقياً، لكن خط أنابيب النواة يملؤه دائماً؛ وجوده يستوجب `Validate()` ناجحة.
- `IdempotencyStore.Claim/Complete/Fail` تعمل داخل معاملة `CommitPosting`؛ لا تُستدعى بشكل منفصل من SDK بعد الإصلاح.
- `IdempotencyLookup` منفذ اختياري للقراءة الآمنة؛ مطلوب لاختبارات العقد وللاستعلام بعد المجهول.
- الأخطاء الخاصة بالمحول أو السائق لا تُكشف؛ تُترجم إلى `pkg/spi/storage` sentinels.

## 10. القيود المتبقية

- تخصيص رقم السلسلة في SDK يبقى قبل الالتزام؛ فشل ما قبل Commit قد يترك فجوة في أرقام القيود، لكنه لا يترك قيداً أو أثراً مالياً. هذا محفوظ لضمان توافق اختبارات atomic الحالية، وموثّق كقيد residual في [تقرير المرحلة 7](./KERNEL_REMEDIATION_PHASE7_REPORT.md).
- `Lookup` الحالية تأخذ `(companyID, key)` لا العملية صراحةً؛ نطاق التفرد نفسه ثلاثي ومفروض في الحجز، لكن عند استخدام نفس نص المفتاح لعمليات `post` و`reverse` و`revaluation` فإن الاستعلام بالمفتاح المجرد يُرجع أحدث سجل. التوصية للمضيف: مفاتيح نصية مختلفة لكل نوع عملية عند الاعتماد على الاستعلام؛ مفتاح إعادة التقييم يبدأ بادئة `fx-revaluation|` لضمان ذلك.

## 11. الحكم

يُعد العقد مستوفى عندما تمر اختبارات النواة ومجموعة العقد على كل Adapter، ويُثبت أن same-key/same-payload يُعيد النتيجة، وsame-key/different-payload يُرفض صراحةً، والفائز المتزامن واحد، ونتيجة ما بعد Commit قابلة للاسترجاع دون تخمين.

## 12. أدلة المرحلة 11 لـ `revaluation`

- `storage.OperationRevaluation` هو الثابت العام الوحيد المضاف؛ لم تتوسّع `PostEntry` ولا `PostingCommit`.
- `internal/kernal/valuation.RevaluationPayloadFingerprint` يوفّر بصمة v2 المستقرة للمضيف، دون تغيير خوارزمية البصمة.
- `TestPhase11_Revaluation*` يغطي على Memory وPostgreSQL: replay، تعارض الحمولة، فائزاً واحداً تحت التزامن، unknown outcome، فصل النطاق عن `post`/`reverse`، عزل الشركة، و`Action=revaluation` في التدقيق.
- قرار التصميم في [ADR P11-01](./ard/P11_01_revaluation_idempotency_spi.md).
