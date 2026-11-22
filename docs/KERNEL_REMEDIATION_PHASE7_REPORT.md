# تقرير المرحلة 7 — المعاملات والذرية ومنع التكرار (Transactions, Atomicity & Idempotency)

- **رمز الوثيقة:** `AE-REMED-P7-REPORT`
- **المرحلة:** 7 — المعاملات والذرية ومنع التكرار
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md) (قسم المرحلة 7، المهام 1–16)
- **سجل الفجوات:** [`KERNEL_REMEDIATION_GAP_REGISTER.md`](./KERNEL_REMEDIATION_GAP_REGISTER.md)
- **القاعدة الحاكمة:** [`RULE-KERNEL-027`](../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md) ومصفوفة التتبع [القسم 1.6](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md)
- **مخرجات العقد:** [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md) · [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md) · [ذرية العكس](./KERNEL_REVERSAL_ATOMICITY_CONTRACT.md) · [مجموعة اختبارات العقد](./KERNEL_CONTRACT_TEST_SUITE.md) · [تقرير حقن الأعطال](./reports/fault_injection_report.md)
- **النتيجة:** `PHASE 7 GATE: PASS`

> هذا هو سجل المرحلة 7 المطلوب في الخطة. يُقرأ مع العقود الخمسة المذكورة أعلاه؛ لا يُعدّ وجود الواجهة أو التعليق دليلاً على الذرية.

## 1. الهدف والنطاق

حوّلت المرحلة 7 عبارة «الذرية على عاتق Host Executor» في `RULE-KERNEL-027` إلى عقد قابل للقياس يعمل على المحولين الحاليين. السؤال الحاكم: هل يمكن أن يظهر نجاح جزئي، أو قيد/حركة مالية ثانية، أو عكس مكرر، أو نتيجة مفقودة بعد Commit؟

النطاق المنفّذ:

- عقود النواة في `internal/kernal/posting` دون SQL أو `pgx` أو معرّفات جداول.
- منافذ SPI محايدة في `pkg/spi/storage`، من ضمنها sentinels للأخطاء ونقاط حقن الأعطال.
- `pkg/sdk` بوصفه جسراً ينسخ العقد المختوم ولا يعيد تعريفه.
- Adapter الذاكرة وAdapter PostgreSQL واختبارات العقد المشتركة بينهما.
- لا قفل صفوف ولا سياسة Deadlock كاملة؛ ذلك متروك للمرحلة 8 (`G-002`).

## 2. تغطية المهام 1–16

| # | المهمة من الخطة | التنفيذ والدليل | الحالة |
| ---: | --- | --- | --- |
| 1 | مواصفة Transaction Boundary | [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md) يحدد ترتيب الالتزام من التحقق إلى Commit، ويمنع نمط «اكتب القيد ثم ألحق التدقيق لاحقاً» | PASS |
| 2 | عناصر وحدة العمل | القيد وأسطره، حالة `posted`، تواريخ النواة، الأثر القابل لإعادة البناء، سجل idempotency، روابط العكس، والتدقيق الإلزامي موثقة في القسم 3 من العقد ومنفذة في كلا المحولين | PASS |
| 3 | Audit داخل المعاملة أم بروتوكول موثوق | التدقيق أصبح جزءاً من نفس الالتزام في `storage/memory/posting.go` و`storage/postgres/uow.go`؛ SDK لم يعد يلحق سجلاً خارجياً أو يتجاهل خطأه | PASS |
| 4 | عقد idempotency كامل | `internal/kernal/posting/transaction.go` يعرّف `IdempotencyScope` و`BuildIdempotencyScope` وبصمتين canonical؛ `storage.PostingIdempotency` يحمل `Key` و`Scope` و`PayloadHash` | PASS |
| 5 | المفتاح نفسه والحمولة نفسها | `TestPhase7_IdempotentReplayAndPayloadConflict` و`TestPhase7_ConcurrentReplayHasSingleEffect` يثبتان replay بلا كتابة جديدة وبنفس `EntryID` | PASS |
| 6 | المفتاح نفسه وحمولة مختلفة | `storage.ErrIdempotencyPayloadConflict` يُرفض صراحةً في النواة والمحولين والـ SDK؛ `classifyError` يترجمه إلى `ErrConflict` | PASS |
| 7 | طلبان متزامنان بالمفتاح نفسه | قفل استشاري + قيود فريدة في PostgreSQL، وقفل المحول + لقطة في الذاكرة؛ `TestPhase7_ConcurrentReplayHasSingleEffect` على المحولين | PASS |
| 8 | timeout/connection loss بعد Commit | `FaultAfterCommit` يحاكي فقدان الاستجابة؛ `IdempotencyLookup.Lookup` و`FindPostedEntryByIdempotencyKey` يسترجعان النتيجة دون تخمين | PASS |
| 9 | تصنيف الأخطاء | `posting.IsRetryable` و`storage.IsRetryable` و`RetryableError.Unwrap`؛ تعارض الحمولة والعكس المكرر وخرق الذرية non-retryable | PASS |
| 10 | ذرية العكس | `TestPhase7_ReversalAtomicity`: العكس والربط وحالة الأصل والمفتاح والتدقيق وحدة واحدة، والمحاولة الثانية مرفوضة بـ `ErrEntryAlreadyReversed` | PASS |
| 11 | قيود التخزين المطلوبة | هجرة PostgreSQL `202611220001_phase7_idempotency_scope.sql` تضيف `scope` وتعيد ملء الصفوف القديمة وتنشئ PK `(company_id, scope)` وفهرس `(company_id, operation, idempotency_key)`؛ لا SQL داخل النواة | PASS |
| 12 | Contract Suite محايد | `test/contract_parity/phase7_transaction_contract_test.go` مكتوب ضد `storage.Ports` ويُشغَّل عبر `ForEachStorageAdapter` | PASS |
| 13 | التشغيل على المحولين | التقرير المولّد يسجّل 24 حالة PASS (12 سيناريو × Memory وPostgreSQL) بلا اختلاف دلالي | PASS |
| 14 | حقن الأعطال عند كل نقطة | `FaultBeforeIdempotencyClaim` و`FaultBeforeAudit` و`FaultBeforeCommit` و`FaultAfterCommit` عبر `storage.WithFaultInjector`؛ [تقرير حقن الأعطال](./reports/fault_injection_report.md) | PASS |
| 15 | ما قبل Commit بلا أثر، وما بعده قابل للاسترجاع | `TestPhase7_FaultInjectionAtomicity` يفحص صفر قيد/مفتاح/رصيد/تدقيق قبل Commit، واستعادة `EntryID` بعده | PASS |
| 16 | تحديث القاعدة والوثائق | `RULE-KERNEL-027` وُضّحت، وأُضيف [القسم 1.6](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md)، وcharters الحزم، وملحق ADR P1-02، وملخص الفجوات | PASS |

## 3. التغييرات الإنتاجية

| الملف | التغيير |
| --- | --- |
| `internal/kernal/posting/transaction.go` *(جديد)* | نطاق idempotency ثلاثي، بصمتا `FingerprintRequest`/`FingerprintCallerPayload`، أخطاء الحالات، و`IsRetryable`/`RetryableError` |
| `internal/kernal/posting/pipeline.go` | يختم `Scope` و`RequestFingerprint` و`CallerFingerprint` فوق قيم المتصل بعد اكتمال التحقق وقبل تفويض Executor |
| `internal/kernal/posting/doc.go` | charter الحزمة يوضّح أن النواة تعرّف العقد ولا تملك معاملة أو قفلاً أو تخزيناً |
| `pkg/spi/storage/ports.go` | يضيف `PostingIdempotency` و`Validate()` و`IdempotencyLookup` و`Scope` في `IdempotencyRecord` دون كسر `IdempotencyStore` |
| `pkg/spi/storage/errors.go` *(جديد)* | ستة sentinels محايدة: تعارض حمولة، قيد التنفيذ، مخزن غير متاح، عكس مكرر، تعارض متزامن، خرق ذرية |
| `pkg/spi/storage/faults.go` *(جديد)* | أربع نقاط حقن محايدة تُمرَّر عبر `context`؛ الإنتاج لا يمرّر حقناً |
| `pkg/spi/storage/transaction.go` | توسيع `IsRetryable` ليشمل `ErrIdempotencyInProgress` و`ErrConcurrentModification` |
| `pkg/sdk/application.go` | نسخ العقد المختوم إلى `PostingCommit`، preflight آمن للقراءة فقط، حذف external Claim/Complete/Fail وحذف إلحاق التدقيق المتجاهل للخطأ، وتصنيف أخطاء التخزين |
| `storage/memory/driver.go`, `idempotency.go` | مفتاح idempotency يشمل `scope`؛ `Lookup` للقراءة فقط؛ كشف تعارض البصمة |
| `storage/memory/posting.go` | لقطة كاملة للقيد/الأرصدة/التدقيق مع rollback، حجز داخل نفس الوحدة، فائز واحد للعكس، الحفاظ على طوابع النواة، ونقاط الحقن الأربع |
| `storage/memory/transaction_ports.go` | تمرير `Lookup` عبر `transactionAwareDriver` |
| `storage/postgres/idempotency.go` | حجز/استعلام/إكمال بالنطاق الكامل، قفل استشاري، وترجمة تعارض 23505 إلى `ErrConcurrentModification` |
| `storage/postgres/posting.go`, `uow.go` | Commit واحد يضم idempotency والقيد والتدقيق؛ قفل `FOR UPDATE` للأصل؛ أسماء JSON البديلة `OriginalEntryID`/`ReversedEntryID`؛ نقاط الحقن؛ الحفاظ على التواريخ المختومة |
| `storage/postgres/migrations/202611220001_phase7_idempotency_scope.sql` *(مع `.down.sql`)* | توسيع نطاق التفرد في قاعدة PostgreSQL؛ عدد الهجرات 32→33 مع تحديث اختبارات دورة الهجرة |
| `internal/kernal/posting/transaction_contract_test.go` *(جديد)*، `internal/kernal/doc_test.go` | اختبارات النواة للنطاق والبصمات والختم والتصنيف، وتحقق آلي من مصفوفة التتبع |
| `test/contract_parity/phase7_transaction_contract_test.go` *(جديد)* | ستة سيناريوهات DB-agnostic تعمل على المحولين |
| `docs/api/v1.snapshot` | تحديث مقصود بـ `--update` لأن حقول struct أُضيفت؛ لا إزالة ولا تغيير توقيع |

لم يُضف أي استيراد `database/sql` أو `pgx` أو `storage/*` إلى `internal/kernal`؛ بوابة `verify_architecture.sh` تؤكد R3c/R3d. لا `float32`/`float64` في مسارات المبالغ، ولا `time.Now()` مباشر في منطق النواة الإنتاجي، ولا أخطاء تدقيق متجاهلة.

## 4. منهجية Test-First والأدلة الحمراء

1. كُتبت اختبارات العقد قبل الإصلاح في `internal/kernal/posting/transaction_contract_test.go` و`test/contract_parity/phase7_transaction_contract_test.go`.
2. الحالات الحمراء المرصودة قبل التنفيذ:
   - لا يوجد `Scope` ولا `PayloadHash` في `PostingCommit`، فكان إعادة الطلب بالمفتاح نفسه تُنشئ قيداً ثانياً.
   - `pkg/sdk` كان يستدعي `Claim`/`Complete`/`Fail` خارج معاملة الحفظ، ويلحق التدقيق بـ `_ = ...` بعد نجاح الحفظ.
   - كان `storage/memory` يستبدل `PostingDate`/`PostedAt` المختومين بـ `time.Now().UTC()`.
   - لم تكن هناك نقاط حقن، فلم يمكن إثبات أن الفشل قبل Commit بلا أثر.
   - كان نفس نص المفتاح في `post` و`reverse` يتشارك هوية/idempotency في PostgreSQL.
3. نُفّذ الإصلاح الأدنى لكل حالة حمراء، ثم أُعيد تشغيل الاختبار المستهدف. أُضيف `FaultBeforeIdempotencyClaim` إلى حلقة الحقن، وأُضيف `TestPhase7_KernelStampedDatesArePreserved` مع استيراد `time`، ونجحا على Memory وPostgreSQL.
4. قُوّي التقرير المولّد بإعادة تشغيل `go test ./... -count=1` كاملاً؛ تشغيل حزمة فرعية يعيد كتابة تقريرين بعدد السيناريوهات التي شغّلتها فقط، ولذلك لا يُعتمد لتوليد الدليل النهائي (`G-010`).
5. أُضيفت الحقول إلى snapshot API compatibility بصورة إضافية فقط؛ `check_api_compat.sh` نجح بعد تحديث خط الأساس، دون حذف أو تغيير توقيع.

## 5. القرارات المحفوظة

- **النطاق والبصمات حاكمان:** النواة تختم `(CompanyID, Operation, Key)` والبصمتين في `PostOptions` قبل Executor؛ أي قيمة من المتصل تُكتب فوقها. هذا يمنع اختيار شركة أو نوع عملية أجنبي للمفتاح الدائم.
- **بصمتان لا واحدة:** `RequestFingerprint` يصف اللقطة المجمدة بعد الختم، أما `CallerFingerprint` فيبقى ثابتاً عبر التحويل ويُحفظ كـ `PayloadHash`. استخدام إحداهما مكان الأخرى يسبب تعارضاً زائفاً بعد أن تملأ النواة الفترة والسياسة والتاريخ.
- **منفذ اختياري متوافق:** `IdempotencyLookup` أُضيف بدل تغيير توقيع `IdempotencyStore`، حتى تبقى التطبيقات والـ fakes القديمة قابلة للترجمة.
- **Audit داخل الالتزام:** لا يوجد تدقيق خارجي متجاهل الخطأ. أي فشل في `AppendAudit` يُلغي كل وحدة العمل.
- **حقن محايد:** نقاط الحقن تعيش في `pkg/spi/storage` وتُمرَّر عبر `context`؛ لا تخص PostgreSQL ولا الذاكرة، ولا توجد أعطال دائمة في قاعدة الشيفرة.
- **توافق PostgreSQL مع WORM:** `storage/postgres/posting.go` يضيف `OriginalEntryID` و`ReversedEntryID` إلى JSON ويحذف `ReversedByID` من JSON المخزّن، ثم يترجم الأسماء عند القراءة. بهذا أُغلقت `G-017` دون نقل تفاصيل قاعدة بيانات إلى النواة.
- **تخصيص السلسلة:** بقي في SDK قبل الالتزام لأن اختبارات `test/atomic` الحالية تعتمد عليه؛ هذا قيد متبقٍ موثق أدناه ولا يترك قيداً أو أثراً مالياً عند الفشل.
- **لا ADR جديد:** القرارات امتداد لـ `RULE-KERNEL-027` وADR P1-02؛ لا تغيير في تعريف مصدر الحقيقة المحاسبي.

## 6. الأدلة التشغيلية النهائية

| الأمر | النتيجة |
| --- | --- |
| `go version` | `go version go1.26.3 linux/amd64` |
| `gofmt -l` على ملفات Go المعدّلة في المرحلة 7 | لا إخراج |
| `go test ./internal/kernal/posting -count=1` | PASS — عقود النطاق والبصمات والختم والتصنيف |
| `go test ./internal/kernal -run '^TestKernel(TraceabilityMatrixCoversAllRulesAndNamedTests\|InvariantDocumentIsPresentAndScoped)$' -count=1` | PASS |
| `go test ./test/contract_parity -run '^TestPhase7_' -count=1 -v` | PASS على Memory وPostgreSQL، مع كل نقاط الحقن الأربع |
| `go test ./... -count=1` | PASS للمستودع كاملاً؛ PostgreSQL نُفّذ فعلياً في هذه البيئة |
| `go test -race ./... -count=1` | PASS لكل الحزم؛ بلا بيانات متسابقة |
| `./scripts/verify_architecture.sh` | `RESULT: PASS - clean architectural base` — 11/11 قاعدة دون تخفيض |
| `./scripts/check_api_compat.sh` | PASS — لا removals ولا تغيير توقيع |
| `git diff --check` | لا إخراج |

تقرير التكافؤ المولّد في [`docs/reports/contract_parity_report.md`](./reports/contract_parity_report.md) (و`test/contract_parity/parity_report.md`) يسجّل **24 passed / 0 failed / 0 skipped**:

- السيناريوهات الستة الجديدة: `IdempotentReplayAndPayloadConflict`، `KernelStampedDates`، `FaultInjectionAtomicity`، `ReversalAtomicity`، `ConcurrentReplay`، `CompanyScopeIsolation`.
- كل سيناريو PASS على `memory` و`postgres` بلا «semantic differences».
- تفاصيل الأعطال والنتائج في [تقرير حقن الأعطال](./reports/fault_injection_report.md). هذه التقارير مولّدة آلياً ولا تُحرر يدوياً.

## 7. الوثائق

- أُنشئ [عقد معاملة الترحيل](./KERNEL_POSTING_TRANSACTION_CONTRACT.md): المسؤوليات، عناصر وحدة العمل، ترتيب الالتزام، الضمانات، والقيود المطلوبة من Adapter.
- أُنشئ [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md): النطاق الثلاثي، البصمتان، الحالات، إعادة الطلب، التزامن، unknown outcome، وتصنيف الأخطاء.
- أُنشئ [عقد ذرية العكس](./KERNEL_REVERSAL_ATOMICITY_CONTRACT.md): وحدة عمل العكس، فائز واحد، سلوك الفشل، وتوافق أسماء WORM.
- أُنشئ [دليل مجموعة اختبارات العقد](./KERNEL_CONTRACT_TEST_SUITE.md): المتطلبات، السيناريوهات الستة، نقاط الحقن الأربع، وطريقة التشغيل.
- أُنشئ [تقرير حقن الأعطال](./reports/fault_injection_report.md) بتفصيل النقاط والنتائج على المحولين.
- حُدّثت `RULE-KERNEL-027` في [ثوابت المحاسبة](../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)، وأُضيف [القسم 1.6](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md) إلى مصفوفة التتبع. تُراجَع أسماء اختبارات العقد الخارجية هناك دون backticks حتى يبقى تحقق المصفوفة داخل `internal/kernal`.
- حُدّثت charters: `internal/kernal/posting/doc.go`، `pkg/spi/storage/doc.go`، `test/contract_parity/doc.go`، مع [مواصفة التدفق](./KERNEL_POSTING_FLOW_SPECIFICATION.md) (القسم 10) و[ملحق ADR P1-02](./ard/P1_02_gl_source_of_truth.md).
- حُدّث [سجل الفجوات](./KERNEL_REMEDIATION_GAP_REGISTER.md) بإغلاق `G-001` و`G-014` و`G-017` و`G-023` بأدلة تشغيل المرحلة المالكة.

## 8. الفجوات والقيود المتبقية

| ID / قيد | الخلاصة | المالك | المرحلة |
| --- | --- | --- | --- |
| `G-002` | Lock Manager يرتب المعرفات فقط؛ لا قفل صفوف ولا سياسة Deadlock كاملة | kernel/posting + storage adapters | 8 |
| `G-019`–`G-022` | النسخ المؤرخة للحساب، سلاسل السياسات، أخطاء تهيئة السياسة، وقارئات الأبعاد في المحول/SDK | sdk / storage adapters | 11–12 |
| `G-007`، `G-010`، `G-012` | بوابة المستودع، إعادة توليد التقارير، وجودة assertions الدلالية | repo / QA | 12 أو إغلاق جزئي |
| `Lookup` بالمفتاح المجرد | الاستعلام الآمن يأخذ `(companyID, key)` لا العملية صراحةً؛ نطاق التفرد ثلاثي ومفروض في الحجز، لكن استخدام نفس نص المفتاح لعمليتي `post` و`reverse` يُرجع أحدث سجل عند الاستعلام. التوصية: نص مفتاح مختلف لكل نوع عملية عند الاعتماد على الاستعلام | sdk / host | موثق في عقد idempotency |
| تخصيص السلسلة | يسبق الالتزام في SDK؛ قد يترك فجوة في أرقام القيود عند فشل ما قبل Commit، دون قيد أو أثر مالي | sdk | residual مقصود |
| انقطاع شبكة فعلي | `FaultAfterCommit` يحاكي فقدان الاستجابة داخل العملية، لا قطع اتصال حقيقي؛ الاستنتاج المنطقي هو نفسه لأن الحالة دائمة وقابلة للاستعلام | QA | 11 إن لزم |

لا تُخفض بوابة المرحلة 7 لتغطي قفل الصفوف أو اختبار انقطاع الشبكة أو جودة assertions؛ تلك مراحل/fجوات أخرى.

## 9. الدليل النهائي

أُنجزت المهام 1–16. وُجد عقد قابل للقياس، ومُنع النجاح الجزئي، ومنع التكرار والتعارض صريحاً، وأصبحت نتيجة unknown outcome قابلة للاستعلام، وأصبح العكس وربطه وحالة الأصل وحدة واحدة. مرّ المحولان بالمجموعة نفسها: 24 حالة PASS. Race والهندسة وتوافق API وفحص المسافات البيضاء كلها PASS بعد إعادة التشغيل الكاملة. جميع التعديلات لا تزال في شجرة العمل للمراجعة (بدون commit أو push).

**PHASE 7 GATE: PASS**
