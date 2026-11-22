# تقرير المرحلة 6 — الترحيل والعكس المحاسبي (Posting & Reversal Integrity)

- **رمز الوثيقة:** `AE-REMED-P6-REPORT`
- **المرحلة:** 6 — الترحيل والعكس المحاسبي
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md) (قسم المرحلة 6، المهام 1–15)
- **سجل الفجوات:** [`KERNEL_REMEDIATION_GAP_REGISTER.md`](./KERNEL_REMEDIATION_GAP_REGISTER.md)
- **مواصفة التدفق:** [`KERNEL_POSTING_FLOW_SPECIFICATION.md`](./KERNEL_POSTING_FLOW_SPECIFICATION.md)
- **مصفوفة التتبع:** [`../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md`](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md)
- **قرار التصميم:** [ADR P1-02، ملحق المرحلة 6](./ard/P1_02_gl_source_of_truth.md)، مع [ADR P3-01](./ard/P3_01_masterdata_effective_dating.md) و[ADR P5-01](./ard/P5_01_policy_selection_and_historical_interpretation.md)
- **النتيجة:** `PHASE 6 GATE: PASS`

> هذا الملف هو سجل **Posting & Reversal Integrity** المطلوب في مخرجات المرحلة 6، ويُقرأ مع مواصفة التدفق ومصفوفة القواعد/الاختبارات.

## 1. الهدف والنطاق

عالجت المرحلة 6 السؤال المطروح في الخطة: هل يمر كل مسار ترحيل بجميع الثوابت، وهل يمكن للعكس أن يتجاوزها أو يغيّر الأثر التاريخي؟

النطاق المنفّذ:

- `internal/kernal/posting/**` بالكامل من ناحية التدفق والاختبارات.
- التفاعل الضروري مع `journal` و`masterdata` و`policy` و`types` و`valuation`.
- لا تخزين ملموس، ولا SQL/drivers، ولا HTTP/gRPC، ولا RBAC، ولا تقارير، ولا أرصدة مادية داخل النواة.

## 2. تغطية المهام 1–15

| # | المهمة من الخطة | التنفيذ والدليل | الحالة |
| ---: | --- | --- | --- |
| 1 | رسم تدفق `PostEntry` الفعلي وربط كل خطوة بقاعدة واختبار | [مواصفة التدفق](./KERNEL_POSTING_FLOW_SPECIFICATION.md) تصف 16 خطوة بالترتيب التنفيذي، مع الدالة والأخطاء والقاعدة والاختبار لكل خطوة | PASS |
| 2 | التحقق من ترتيب الهوية/التصنيف/الحالة/الشركة/العملة/اليومية/الفترة/السياسات/الحسابات/الأبعاد/التوازن | الجدول في القسم 3 من المواصفة؛ `TestPostEntryIdentityAndPolicyRejectMatrix`، `TestPostingAdversarialRepositoryFailures`، `TestPostEntryCompanyCurrencyAndTransactionRejectMatrix` | PASS |
| 3 | فحص مسارات الخروج ومنع Executor عند أي فشل | `TestPostEntryExecutorNeverRunsForRejectedInputs` يغطي ثماني حالات رفض ويؤكد 0 استدعاء؛ `TestPostEntryRejectsDraftWithPreexistingReversalLink` يؤكد 0/0 للمنفذين؛ `TestPostEntryStopsWhenAtomicExecutorFails` يثبت إعادة خطأ Executor كما هو | PASS |
| 4 | Clone قبل أي تعديل داخلي | `TestCloneIsDeepAndCoversPointerFields` و`TestPostEntryExecutesFrozenValidatedSnapshot` يثبتان أن المتصل يستطيع تعديل كائنه بعد Clone بينما يصل Executor إلى النسخة المجمدة | PASS |
| 5 | التوازن بعد التقريب والتسوية الصريحة | `RoundEntryWithRoundingPolicy` ثم `ValidateEntryDoubleEntry`؛ `TestPostEntryNormalizesBothCurrenciesAndSettlesRounding`، `TestRoundBalancedEntrySettlesRoundedFunctionalDifference`، `FuzzRoundingResidualRequiresExplicitSettlement` | PASS |
| 6 | حساب فروق التقريب بسياسة صريحة | `roundingAccountID` لا يُرجع معرفاً إلا عند `AutoSettleRoundingDifferences`؛ `TestPostEntryRejectsRoundingWithoutPolicySettlement` يمنع بلوغ Executor بدون تسوية صريحة | PASS |
| 7 | تطابق عملات السطر والعملة الوظيفية | `TestPostEntryCompanyCurrencyAndTransactionRejectMatrix`، `TestEntryLineRejectsTransactionCurrencyMismatch`، `FuzzEntryLineRejectsCurrencyAndExchangeRateMismatch` | PASS |
| 8 | التصنيف والفترتان 13 و14 | `TestPostingOpeningAndAuditAdjustmentPeriodBranches`، `TestAdjustingEntriesRequireMetadataAtValidation`، `TestPostingResolvesAdjustmentPeriodByAccountingMetadata` | PASS |
| 9 | `BuildReversalEntry` / `BuildReversalEntryAt` وحفظ الأبعاد والعملات وتبديل المدين والدائن | `TestReversalLifecycle`؛ `TestReversalBuilderLeavesOriginalImmutable` يفحص الآن كل سطر: `AccountID`، `PartnerID`، `CostCenterID`، `ProjectID`، `SegmentID`، `BusinessUnitID`، `MatchingNumber`، عملة/مبلغ المعاملة، سعر الصرف، ومبادلة المدين/الدائن | PASS |
| 10 | تاريخ العكس واشتقاق فترته | `BuildReversalEntryAt` يوحّد التاريخ إلى UTC ويرفض الأسبقية يومياً؛ `PostEntry` يحل الفترة من تاريخ العكس. الدليل: `TestReversalLifecycle`، `TestReversalDraftResolvesDestinationPeriodDuringPosting` | PASS |
| 11 | أثر العكس متوازن وبدون تعديل الأصل | `journal.ValidateImmutability` داخل اختبار العكس، و`FuzzReversalNetImpactIsZeroPerAccount`، و`TestRebuildIncludesReversedEntriesAndReversalNetImpact` | PASS |
| 12 | اختبارات TOCTOU العدائية للشركة/الحساب/العملة/الفترة | `TestPostEntryExecutesFrozenValidatedSnapshot` يعدّل كائن المتصل بعد Clone؛ `TestPostEntryUsesEffectiveAccountSnapshotForEligibility` يثبت قراءة حساب واحدة نافذة زمنياً؛ `TestPostEntryCompanyScopeAdversarialBranches`، `TestPostingPolicyAndClockAdversarialBranches` | PASS داخل النواة |
| 13 | فشل Reader/Policy/Executor بلا أثر مالي قبل التفويض | `TestPostingAdversarialRepositoryFailures`، `TestPostEntryFailsClosedWithoutRegisteredPolicies`، `TestPostEntryStopsWhenAtomicExecutorFails`، `TestPostEntryHonorsCancelledContextInExecutor` | PASS |
| 14 | اختبار أو ربط لكل قاعدة من القواعد الـ27 | مصفوفة التتبع تغطي 001–027 بدالة مسؤولة واختباري قبول ورفض لكل صف؛ `TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests` يفحص ذلك آلياً | PASS |
| 15 | تحديث مصفوفة التتبع وربط كل فرع تحقق باختبار | ملحق المرحلة 6 (1.5) في `KERNEL_TRACEABILITY_MATRIX.md`، و`doc_test.go` يتحقق من مطابقة كل مرجع `Test`/`Fuzz` لدالة موجودة | PASS |

## 3. التغييرات الإنتاجية

| الملف | التغيير |
| --- | --- |
| `internal/kernal/masterdata/coa.go` | دالة `ResolvePostableAccountAsOf` تُرجع نسخة الحساب التي تحققت منها النواة؛ `ValidateCanPostToAccountAsOf` تفوّض إليها، و`ValidateCanPostToAccountWithClock` يضيف ساعة قابلة للحقن |
| `internal/kernal/posting/pipeline.go` | قراءة حساب واحدة نافذة زمنياً لكل سطر تغذّي التحقق والأهلية معاً؛ لا `GetAccount` ثانية في المسار المرحّل. ختم البصمة/التواريخ يبقى بعد اكتمال التحقق وقبل التفويض |
| `internal/kernal/posting/entry_validation.go` | رفض مسودة تحمل `ReversedByID` بـ `ErrInvalidReversal`؛ `NewJournalEntry` يستخدم `systemClock{}.Now()` |
| `internal/kernal/posting/reversal.go` | `BuildReversalEntry` يستخدم `systemClock{}.Now()`؛ `BuildReversalEntryAt` تبقى نقطة الزمن الصريح |
| `internal/kernal/posting/doc.go` | تحديث charter الحزمة بنطاق المرحلة 6 وحدود المسؤولية |

الوصول إلى الساعة الحقيقية في شيفرة `posting` الإنتاجية محصور الآن في `systemClock{}.Now()`؛ الاختبارات والمنادون الحتميون يستخدمون `WithClock` أو `BuildReversalEntryAt`/`NewJournalEntryAt`.

## 4. منهجية Test-First والأدلة الحمراء

1. كُتبت اختبارات المرحلة 6 قبل الإصلاح في `internal/kernal/posting/posting_integrity_test.go` و`internal/kernal/doc_test.go`.
2. الفشل الأحمر المرصود قبل التغيير الإنتاجي:
   - `PostEntry error = <nil>, want ErrInvalidReversal` — كانت مسودة تحمل رابط عكس تُقبل.
   - `entry_validation.go calls time.Now directly at entry_validation.go:266`.
   - `reversal.go calls time.Now directly at reversal.go:19`.
   - فشل اختبار اللقطة النافذة لأن المسار كان يعيد قراءة `GetAccount` بعد التحقق، فيرى نسخة مختلفة عن نسخة `EffectiveAccountReader`.
3. نُفّذ الإصلاح الإنتاجي الأدنى، فتحوّلت الاختبارات إلى أخضر دون تخفيف تأكيد. لاحقاً قُوّي `TestReversalBuilderLeavesOriginalImmutable` ليفحص جميع الأبعاد والحقول المحفوظة لكل سطر لا مجرد الشريك وتبادل المدين/الدائن؛ أُعيد تشغيل الاختبار المستهدف ثم المجموعة الكاملة.
4. لا `float32`/`float64`، ولا أخطاء متجاهلة في مسارات التحقق/الختم/التفويض، ولا استدعاء `time.Now()` مباشر في منطق `posting` الإنتاجي. البوابتان `TestKernelFinancialCodeHasNoFloatConversions` و`TestPostingProductionCodeUsesOnlyInjectedClocks` تجعلان ذلك آلياً.

## 5. القرارات المحفوظة

- **لا ADR جديد:** القرار موثّق كملحق للمرحلة 6 في ADR P1-02، لأن مصدر الحقيقة بقي القيد المرحّل وأسطره؛ تغيّر ترتيب القراءة لا تعريف المصدر.
- **النسخة النافذة حاكمة:** وفق ADR P3-01، تُستخدم نتيجة `GetAccountAsOf` نفسها للتحقق ولأهلية الحساب. لا قراءة ثانية بتوقيت مختلف (TOCTOU).
- **حالة مستحيلة تُرفض:** مسودة بـ `ReversedByID` مرفوضة؛ العكس هو الذي يشير إلى الأصل، ولا يعود الأصل إلى Pipeline بعد الترحيل.
- **الزمن:** `systemClock` هو المنفذ الوحيد للساعة الحقيقية في الإنتاج؛ deterministic callers يمررون الزمن صراحةً. لا يسبق تاريخ العكس تاريخ الأصل، وتُحل فترة العكس من تاريخه لا من فترة الأصل.
- **حد الضمان:** النواة تجمّد المدخلات وتقرر وتتحقق فقط. الذرية، idempotency التخزين، retry/unknown-outcome، القفل، وتكافؤ المحولات مسؤولية المرحلة 7/11 وليست ضمن هذه البوابة.
- **Fuzz:** اعتمدنا `FUZZTIME=10s` كخط أساس. تشغيل `2s` أولي سجّل `context deadline exceeded` عابراً في `FuzzReversalNetImpactIsZeroPerAccount`؛ أُعيد التشغيل عند 10s ومرة منفردة عند 2s فنجحا. سكربت Baseline لا يرفع `rc` من حلقة Fuzz، لذا يُعتمد على سطور `EXIT=0` في السجل.

## 6. الأدلة التشغيلية

| الأمر | النتيجة |
| --- | --- |
| `go version` | `go version go1.26.3 linux/amd64` (و`go.mod`: `go 1.26.0` / `toolchain go1.26.3`) |
| `gofmt -l internal/kernal/` | لا إخراج |
| `go test ./internal/kernal/... -count=1` | `ok` لجميع الحزم العشر |
| `go test -race ./internal/kernal/... -count=1` | `ok` لجميع الحزم العشر |
| `./scripts/verify_architecture.sh` | `RESULT: PASS - clean architectural base` — 11/11 قاعدة دون تخفيض |
| `./scripts/check_api_compat.sh` | PASS |
| `go test ./... -count=1` | PASS للمستودع كاملاً |
| `FUZZTIME=10s ./scripts/verify_kernel_baseline.sh /tmp/opencode/phase6` | PASS؛ 9 أهداف Fuzz بـ `EXIT=0` |
| `COVERAGE_MIN=90 ./scripts/check_coverage.sh /tmp/opencode/phase6/kernel.cover` | `total coverage: 92.3% (minimum 90%)` |
| `git diff --check` | لا إخراج |
| `git status --short` بعد الاختبار الكامل | تقريرا التكافؤ المتتبعان غير ظاهرين؛ أُعيدت نسختهما المحفوظة في `/tmp/opencode/phase6/` بعد تشغيل `go test ./...` (`G-010`) |

سجلات التشغيل الخام محفوظة في `/tmp/opencode/phase6/logs/`. لا تُستخدم هذه السجلات بديلاً عن إعادة التشغيل عند إغلاق فجوة؛ قاعدة السجل تظل «دليل جديد في المرحلة المالكة».

## 7. الوثائق

- أُنشئت [مواصفة تدفق الترحيل والعكس](./KERNEL_POSTING_FLOW_SPECIFICATION.md): 16 خطوة، حدود مسؤولية، قواعد Clone، بناء العكس، نموذج TOCTOU، وسلوك الفشل.
- أُضيف ملحق المرحلة 6 إلى [مصفوفة التتبع](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md) (القسم 1.5)، وتحديث مسؤوليات القواعد 001–015 و027.
- أُضيف قسم توضيحات غير معدّل للقواعد في `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md`.
- أُضيف ملحق المرحلة 6 إلى [ADR P1-02](./ard/P1_02_gl_source_of_truth.md).
- حُدّث `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` بحالة `G-012` وأدلة المرحلة 6 والفجوتين `G-022` و`G-023` اللتين ظهرتا أثناء التدقيق ولا تعنيان فشل بوابة النواة.

## 8. الفجوات المتبقية والقيود

| ID | الأولوية | الخلاصة | المالك | المرحلة |
| --- | --- | --- | --- | --- |
| `G-012` | Medium | الأتمتة الاسمية والمرجعية للقواعد الـ27 منفذة؛ جودة الـ assertions الدلالية تبقى مراجعة بشرية موثقة | kernel / documentation owner | 6 (إغلاق جزئي) |
| `G-001`, `G-002`, `G-014` | High | الذرية، idempotency التخزين، القفل، وتكافؤ Adapter عند الفشل ليست من مسؤولية النواة | storage adapters | 7 |
| `G-017`, `G-019`, `G-020`, `G-021` | High/Medium | روابط العكس، النسخ المؤرخة للحساب، سلاسل السياسات، وأخطاء تهيئة السياسة في SDK | storage adapters + host | 7/11/12 |
| `G-022` | High | لا يوجد Adapter أو منفذ SPI ينفّذ قارئي الأبعاد المطلوبين؛ القيود ذات الأبعاد تفشل مغلقاً بـ `ErrDimensionReaderRequired` في المسار الحقيقي | sdk / storage adapters | 11 |
| `G-023` | High | `storage/memory/posting.go` يحل محل `PostingDate`/`PostedAt` المختومين في النواة بـ `time.Now()` عند الحفظ | storage adapters | 7 أو 11 |
| `G-007` | Medium | بوابة `verify_integrity.sh` تفشل خارج النواة لأسباب تنسيق/فحص مجلدات لا علاقة لها بالمرحلة 6 | repo owner / CI | خارج نطاق النواة |

لا تُغلق هذه الفجوات داخل المرحلة 6 لأن نطاقها إما خارج `internal/kernal` أو مؤجّل إلى بوابات لاحقة. النواة تفشل مغلقاً عند غياب readers الأبعاد، ولا تدّعي حفظ الطوابع الزمنية بعد التفويض.

## 9. الدليل النهائي

أُنجزت المهام 1–15، مع اختبارات حمراء ثم خضراء، وتقوية اختبار حفظ الأبعاد في العكس، وتحقق Race، وبوابة معمارية، وفحص توافق API، واختبار مستودع كامل، وBaseline Fuzz عند 10s، وتغطية 92.3%. تغطي المصفوفة القواعد الـ27 وكل مرجع اختبار مذكور فيها. لا قاعدة محاسبية جديدة، ولا توسيع إلى طبقة التخزين أو Adapter في هذه المرحلة. تبقى التعديلات كلها في شجرة العمل للمراجعة (بدون commit أو push).

**PHASE 6 GATE: PASS**
