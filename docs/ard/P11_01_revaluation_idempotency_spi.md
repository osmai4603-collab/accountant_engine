# ADR P11-01 — توصيل idempotency إعادة التقييم بعقد التخزين

- **الحالة:** مقبول (Accepted) — يغلق `G-024`
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernal/posting` + مالك `internal/kernal/valuation` + مضيف التخزين/SDK
- **المرجع:** [المرحلة 11](../KERNEL_REMEDIATION_PHASE11_REPORT.md)، [سجل الفجوات](../KERNEL_REMEDIATION_GAP_REGISTER.md)، [عقد idempotency](../KERNEL_IDEMPOTENCY_CONTRACT.md)
- **ذو صلة:** [ADR P9-01](./P9_01_valuation_idempotency_and_rate_selection.md)، [عقد المعاملات](../KERNEL_POSTING_TRANSACTION_CONTRACT.md)

## السياق

1. أنتجت المرحلة 9 `RevaluationOperationKey` و`RevaluationIdempotencyScope` وبصمة الحمولة v2 داخل `RevaluationRegistry`، لكنها صرّحت بأن النطاق **غير موصول** بـ `PostEntry` ولا بعقد SPI؛ وبقي الضمان الدائم ضد التكرار هو `G-024`.
2. المرحلة 7 تعتمد `company/operation/key` كهوية وحيدة للتفرّد في المخزن؛ المحولات (Memory وPostgreSQL) لا تحتاج معرفة دلالة العملية، بل تحتاج فقط أن يصلها `PostingCommit.Operation` و`PostingIdempotency.Scope` و`PayloadHash` متطابقة. عمود `operation varchar(64)` موجود أصلاً، فلا migration جديدة.
3. قيد إعادة التقييم يُولَّد بمعرّفات عشوائية في كل مرة، لذلك `FingerprintRequest` الخاص بالترحيل — الذي يشتمل على المعرّفات — غير صالح كبصمة لتكرار نتيجة تقييم متطابقة. البصمة الصالحة هي v2 الخاصة بالتقييم في `internal/kernal/valuation`.
4. توسيع توقيع `PostEntry` أو `PostingExecutor` لإضافة معامل `revaluation` يكسر واجهة النواة ويلزم كل مضيف بتغيير توقيعه، بينما `PostOptions.Scope` موجود أصلاً كحامل محايد للنطاق.

## القرار

1. **العملية الثالثة في SPI:** يُضاف الثابت `storage.OperationRevaluation PostingOperation = "revaluation"` إلى `pkg/spi/storage/ports.go` إلى جانب `post` و`reverse`. لا تتغير أي توقيعات أو حقول أو واجهات؛ `PostingCommit.Operation` و`IdempotencyRecord.Operation` و`AuditRecord.Action` تحمل القيمة الجديدة كما هي، وتنعكس في سجل التدقيق كـ `Action=revaluation`. لا migration: عمود `operation varchar(64)` يتسع للقيمة.
2. **الحامل `PostOptions.Scope`:** يمرّر المضيف `RevaluationIdempotencyScope(company, account, currency, date)` في `opts.Scope` مع `opts.IdempotencyKey = scope.Key` قبل `PostEntry`. يحترم `BuildIdempotencyScope` هذا التلميح عند `opts.Scope.Operation == IdempotencyRevaluation` فقط، وبعد أن يتحقق من أن `opts.Scope.CompanyID` يطابق شركة القيد وأن `opts.Scope.Key` يطابق المفتاح (بعد `TrimSpace`)؛ أي تلميح مزيف يُرفض بـ `ErrIdempotencyScopeRequired`. في كل الأحوال تبقى الشركة من `entry` والمفتاح من `opts.IdempotencyKey`، وتُشتق `post`/`reverse` من القيد كما كانت. `PostEntry` يختم النطاق الناتج على `opts.Scope` قبل التفويض، فيصل إلى المضيف دون توقيع جديد.
3. **مصدر البصمة v2:** يستدعي المضيف `valuation.RevaluationPayloadFingerprint(entry)` — غلاف رقيق على الخوارزمية الحالية `revaluationPayloadFingerprint` — ويضع نتيجته في `storage.PostingIdempotency.PayloadHash` و`IdempotencyRecord.PayloadHash`. الخوارزمية لم تتغير: بادئة `fx-revaluation-payload-v2|`، حقول مسبوقة بالطول، مبالغ `ExactString`، أسطر مرتبة بعد الترميز، واستبعاد معرّفات القيد/الأسطر والحالة والطوابع الزمنية للتوليد. لا يُستخدم `FingerprintRequest` ولا `FingerprintCallerPayload` لإعادة التقييم، لأنهما يصفان طلب الترحيل لا نتيجة التقييم.
4. **عدم توسيع `PostEntry`:** لا يوجد معامل جديد، ولا مسار executor رابع، ولا تغيير في توقيع `PostingExecutor` أو `PostingReversalExecutor` أو أي منفذ `pkg/spi/storage`. التغيير الوحيد في النواة هو احترام تلميح محقَّق في `BuildIdempotencyScope` وتصدير البصمة؛ الباقي تجميع في المضيف.
5. **الذانية والفصل:** يخزّن المحول النطاق والبصمة ذرياً مع القيد والأثر والتدقيق (`CommitPosting`). نفس نص المفتاح مع `post` أو `reverse` يعطي سجلات ومفاتيح مختلفة؛ `FaultAfterCommit` يُحل بالـ `Lookup` ثم replay يعيد القيد نفسه؛ والتعارض غير قابل لإعادة المحاولة (`storage.ErrIdempotencyPayloadConflict` / `posting.ErrRevaluationAlreadyApplied`). `RevaluationRegistry` يبقى حماية داخل العملية فقط ولا يُستبدل بالتخزين.

## مسؤولية المضيف (خارج ملفاتي)

`pkg/sdk/application.go` — سيُدمج بواسطة مالكه — يجب أن:

- في `commitIdempotency`: لا يُجبر `scope.Operation` على `post`؛ يستخدم `opts.Scope.Operation` الخاتم من النواة (مع ربطه بـ `storage.OperationRevaluation` عند `posting.IdempotencyRevaluation`) ولا يرجع إلى `post` إلا للمتصلين القدامى الذين لا نطاق لهم. يُفضَّل تمرير البصمة كوسيط صريح:`payloadHash`؛ للترحيل والعكس تبقى `opts.CallerFingerprint`، ولإعادة التقييم تُمرَّر نتيجة `valuation.RevaluationPayloadFingerprint`.
- في `checkIdempotencyBeforePost`: عند كون النطاق `revaluation` تحسب البصمة عبر `valuation.RevaluationPayloadFingerprint` بدل `posting.FingerprintCallerPayload`، مع إبقاء `Lookup` بلا أثر جانبي. **تنبيه:** `PostEntry` يعيد ختم `opts.CallerFingerprint` بـ `FingerprintCallerPayload`، لذلك لا يكفي تعيين بصمة v2 في الخيارات قبل الاستدعاء؛ يجب حساب v2 من لقطة canonical موحّدة في نقطتي الفحص والالتزام (أو مقارنتها بالقيد المخزَّن عند replay) حتى تتطابق البصمتان.
- في `classifyError`: يضيف `errors.Is(err, posting.ErrRevaluationAlreadyApplied)` (و`posting.ErrIdempotencyConflict` إن وُجد) إلى فرع `publicapplication.ErrConflict`.
- في مسار إعادة التقييم نفسه: يبني `scope := valuation.RevaluationIdempotencyScope(...)`، ويمرّرها في `PostOptions.Scope` مع `IdempotencyKey = scope.Key`، ثم يستدعي `commitIdempotency` ببصمة v2 المحسوبة من القيد النهائي بعد تثبيت الحقول التي تختمها النواة والمضيف.

## البدائل المرفوضة

| البديل | سبب الرفض |
| --- | --- |
| إضافة `Revaluation` إلى توقيع `PostEntry` أو توقيع `PostingExecutor` | كسر توافق كل المضيفين والاختبارات، مع أن `PostOptions.Scope` حامل كافٍ ومحايد. |
| استخدام `FingerprintRequest` أو `FingerprintCallerPayload` كبصمة للتقييم | يتضمنان معرّفات عشوائية/حقول ترحيل، فيُرفض replay لنتيجة محاسبية متطابقة. |
| الثقة بـ `opts.Scope` كما هي | تلميح مزيف بشركة أو مفتاح آخر يسمح بسرقة نطاق عملية أو تضارب مفاتيح. |
| migration جديدة لإضافة `operation` | العمود موجود أصلاً بسعة `varchar(64)`؛ التغيير الوحيد ثابت في SPI. |
| الاعتماد على `RevaluationRegistry` وحده | في الذاكرة ولا يصمد أمام إعادة التشغيل أو انقطاع الرد بعد الالتزام. |

## الأدلة التنفيذية

اختبارات وحدة جديدة: `TestBuildIdempotencyScopeHonorsValidatedRevaluationHint`، `TestBuildIdempotencyScopeRejectsForgedRevaluationHint`، `TestPostEntryPropagatesRevaluationScopeHintToExecutor`، `TestPostEntryRejectsForgedRevaluationScopeHint`، `TestRevaluationPayloadFingerprintExportsStableV2Digest`، `TestRevaluationScopeHintRoundTripsThroughPosting`.

اختبارات تكافؤ مشتركة على المحولين عبر `ForEachStorageAdapter`: `TestPhase11_RevaluationReplayWithSamePayload`، `TestPhase11_RevaluationPayloadConflictIsTerminal`، `TestPhase11_RevaluationConcurrentReplayHasSingleWinner`، `TestPhase11_RevaluationFaultAfterCommitLookupAndReplay`، `TestPhase11_RevaluationScopeIsSeparateFromPostAndReverse`، `TestPhase11_RevaluationCompanyScopeIsolation`، `TestPhase11_RevaluationAuditAction`.
