# ADR P11-01 — توصيل idempotency إعادة التقييم بعقد التخزين

- **الحالة:** مقبول (Accepted) — يغلق `G-024`
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernal/posting` + مالك `internal/kernal/valuation` + مضيف التخزين/SDK
- **المرجع:** [المرحلة 11](../reports/KERNEL_FINAL_VERIFICATION_REPORT.md)، [سجل الفجوات](../plans/KERNEL_REMEDIATION_GAP_REGISTER.md)، [عقد idempotency](../contracts/KERNEL_IDEMPOTENCY_CONTRACT.md)
- **ذو صلة:** [ADR P9-01](./P9_01_valuation_idempotency_and_rate_selection.md)، [عقد المعاملات](../contracts/KERNEL_POSTING_TRANSACTION_CONTRACT.md)

## السياق

1. أنتجت المرحلة 9 `RevaluationOperationKey` و`RevaluationIdempotencyScope` وبصمة الحمولة v2 داخل `RevaluationRegistry`، لكنها صرّحت بأن النطاق **غير موصول** بـ `PostEntry` ولا بعقد SPI؛ وبقي الضمان الدائم ضد التكرار هو `G-024`.
2. المرحلة 7 تعتمد `company/operation/key` كهوية وحيدة للتفرّد في المخزن؛ المحولات (Memory وPostgreSQL) لا تحتاج معرفة دلالة العملية، بل تحتاج فقط أن يصلها `PostingCommit.Operation` و`PostingIdempotency.Scope` و`PayloadHash` متطابقة. عمود `operation varchar(64)` موجود أصلاً، فلا migration جديدة.
3. قيد إعادة التقييم يُولَّد بمعرّفات عشوائية في كل مرة، لذلك `FingerprintRequest` الخاص بالترحيل — الذي يشتمل على المعرّفات — غير صالح كبصمة لتكرار نتيجة تقييم متطابقة. البصمة الصالحة هي v2 الخاصة بالتقييم في `internal/kernal/valuation`.
4. توسيع توقيع `PostEntry` أو `PostingExecutor` لإضافة معامل `revaluation` يكسر واجهة النواة ويلزم كل مضيف بتغيير توقيعه، بينما `PostOptions.Scope` موجود أصلاً كحامل محايد للنطاق.

## القرار

1. **العملية الثالثة في SPI:** يُضاف الثابت `storage.OperationRevaluation PostingOperation = "revaluation"` إلى `pkg/spi/storage/ports.go` إلى جانب `post` و`reverse`. لا تتغير أي توقيعات أو حقول أو واجهات؛ `PostingCommit.Operation` و`IdempotencyRecord.Operation` و`AuditRecord.Action` تحمل القيمة الجديدة كما هي، وتنعكس في سجل التدقيق كـ `Action=revaluation`. لا migration: عمود `operation varchar(64)` يتسع للقيمة.
2. **الحامل `PostOptions.Scope`:** يمرّر المضيف `RevaluationIdempotencyScope(company, account, currency, date)` في `opts.Scope` مع `opts.IdempotencyKey = scope.Key` قبل `PostEntry`. يحترم `BuildIdempotencyScope` هذا التلميح عند `opts.Scope.Operation == IdempotencyRevaluation` فقط، وبعد أن يتحقق من أن `opts.Scope.CompanyID` يطابق شركة القيد وأن `opts.Scope.Key` يطابق المفتاح (بعد `TrimSpace`)؛ أي تلميح مزيف يُرفض بـ `ErrIdempotencyScopeRequired`. في كل الأحوال تبقى الشركة من `entry` والمفتاح من `opts.IdempotencyKey`، وتُشتق `post`/`reverse` من القيد كما كانت. `PostEntry` يختم النطاق الناتج على `opts.Scope` قبل التفويض، فيصل إلى المضيف دون توقيع جديد.
3. **تصدير بصمة v2 دون use-case عام:** تُصدَّر `valuation.RevaluationPayloadFingerprint(entry)` كغلاف رقيق على الخوارزمية الحالية `revaluationPayloadFingerprint`، وهي بادئة `fx-revaluation-payload-v2|` مع حقول مسبوقة بالطول ومبالغ `ExactString` وأسطر مرتبة بعد الترميز، مع استبعاد معرّفات القيد/الأسطر والحالة والطوابع الزمنية للتوليد. هذا التصدير هو ما يسمح لأي مضيف يرسل قيد إعادة تقييم عبر SPI بتثبيت بصمة نتيجة التقييم بدل بصمة طلب الترحيل. **المرحلة 11 لا تضيف use-case عاماً لإعادة التقييم في `pkg/sdk`**، كما أنها لا توسّع `PostEntry`؛ لذلك لا تُقدَّم أدلة تشغيل SDK لهذا المسار، ويقتصر إثبات `G-024` على عقد التخزين والمحولات (الفقرة 5). لا يُستخدم `FingerprintRequest` ولا `FingerprintCallerPayload` كبصمة لإعادة التقييم.
4. **عدم توسيع `PostEntry`:** لا يوجد معامل جديد، ولا مسار executor رابع، ولا تغيير في توقيع `PostingExecutor` أو `PostingReversalExecutor` أو أي منفذ `pkg/spi/storage`. التغيير الوحيد في النواة هو احترام تلميح محقَّق في `BuildIdempotencyScope` وتصدير البصمة؛ الباقي تجميع في المضيف.
5. **الذانية والفصل:** يخزّن المحول النطاق والبصمة ذرياً مع القيد والأثر والتدقيق (`CommitPosting`). نفس نص المفتاح مع `post` أو `reverse` يعطي سجلات ومفاتيح مختلفة؛ `FaultAfterCommit` يُحل بالـ `Lookup` ثم replay يعيد القيد نفسه؛ والتعارض غير قابل لإعادة المحاولة (`storage.ErrIdempotencyPayloadConflict` / `posting.ErrRevaluationAlreadyApplied`). `RevaluationRegistry` يبقى حماية داخل العملية فقط ولا يُستبدل بالتخزين.

## ما تم توصيله فعلياً في SDK (`pkg/sdk/application.go`)

- `commitIdempotency` ينسخ نطاق النواة الخاتم كما هو، بما فيه `operation=revaluation` حين يصل تلميح محقَّق من النواة؛ لا يُجبر على `post`. المتصلون القدامى بلا نطاق يحصلون على `post`/`reverse` من العملية التنفيذية. البصمة المخزَّنة هي `opts.CallerFingerprint` التي يختمها الترحيل (`FingerprintCallerPayload`)، وهي مستقرة عبر التحويل ومتطابقة في الفحص والالتزام لمساري `post` و`reverse`.
- `checkIdempotencyBeforePost` يبقى فحصاً بلا أثر جانبي، ويستخدم البصمة نفسها للبحث وكشف التعارض قبل الترحيل. لا يدّعي SDK حساب بصمة v2 لإعادة التقييم.
- `classifyError` يصنّف `posting.ErrRevaluationAlreadyApplied` و`storage.ErrIdempotencyPayloadConflict` كـ `ErrConflict`، و`posting.ErrPolicyNotFound`/`ErrPolicyNotEffective` كـ `ErrNotFound`.

## خارج نطاق المرحلة 11 (بلا أدلة تشغيل)

- لا يوجد use-case عام لإنشاء قيد إعادة تقييم في `pkg/sdk`، ولا يُبني `RevaluationIdempotencyScope` داخل SDK. أي مضيف يرسل قيد إعادة تقييم عبر `storage.PostingSPI` مباشرةً ملزم باستخدام `valuation.RevaluationIdempotencyScope` و`valuation.RevaluationPayloadFingerprint` ووضع نتيجته في `storage.PostingIdempotency.PayloadHash`. هذا التزام وثائقي وليس دليلاً تشغيلاً في هذه المرحلة.
- الفشل المغلق محفوظ: التلميح المزيف يُرفض في `BuildIdempotencyScope`، والمحولات تحفظ النطاق والبصمة ذرياً بغض النظر عن مصدرهما.

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
