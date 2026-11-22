# تقرير المرحلة 9 — التقييم والعملات الأجنبية

- **رمز الوثيقة:** `AE-REMED-P9-REPORT`
- **المرحلة:** 9 — التقييم والعملات الأجنبية
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556` (`kernel-v12-final`)
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md) (قسم المرحلة 9، المهام 1–15)
- **القرار:** [ADR P9-01](./ard/P9_01_valuation_idempotency_and_rate_selection.md)
- **المخرجات:** [مصفوفة قواعد التقييم](./KERNEL_VALUATION_RULES_MATRIX.md) · [سجل الفجوات](./KERNEL_REMEDIATION_GAP_REGISTER.md) · [مصفوفة التتبع](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md)§1.8 · [ثوابت النواة](../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)
- **البيئة:** `go version go1.26.3 linux/amd64`، مطابق لتوجيه `toolchain go1.26.3` في `go.mod`
- **النتيجة:** `PHASE 9 GATE: PASS-SCOPED (داخل نطاق النواة)`
- **مراجعة الخطة:** [AE-REMED-PLAN-REVIEW](./KERNEL_REMEDIATION_PLAN_REVIEW.md) — القاعدة المقترحة 6 تعرّف `PASS-SCOPED` بأنه استيفاء نطاق المرحلة مع عمل مؤجل مصنّف بمالك ومرحلة إغلاق، ولا يُستخدم لفتح مرحلة تعتمد عليه.

> الحكم الأولي كان `PASS` اعتماداً على أربع بوابات تقنية فقط. أعيد التحقق مقابل المهام 1–15 فتبيّن أن المرحلة كانت **جزئية**: اتجاه الخصوم واختيار السعر متعدد الأسعار غير مُثبتين، والتكرار داخلي فقط، وفترة العكس غير موثقة، وهدف Fuzz لاختيار السعر ناقص، والتقرير لا يستوفي قالب التقارير السابقة. أُكملت هذه الفجوات داخل المرحلة 9 دون توسيع إلى المرحلتين 10–12 أو إلى المحولات.
>
> ثم راجع مستقلان: الأول للـ diff مقابل بوابات المرحلة، والثاني للخطة كاملة. عالجت المراجعة الأولى ملاحظات جوهرية في بصمة الحمولة وادعاء توصيل النطاق بالمضيف والزوج المعكوس وغموض عملة المفتاح؛ وعالجت الثانية تصنيف الخطأ وصياغة الحكم قالب التقرير. تفاصيل ذلك في القسم 9 ووثيقة مراجعة الخطة.

## 1. الهدف والنطاق

تثبيت اختيار سعر الصرف وإعادة التقييم بسلوك حتمي قابل لإعادة الإنتاج، ومنع تكرار الأثر المالي عند إعادة العملية، وإثبات اتجاه الأصول والالتزامات وعكس القيد المرحّل.

النطاق المنفّذ:

- `internal/kernal/valuation/revaluation.go` و`exchange_rate.go`، مع اختبارات في `phase9_completion_test.go` و`ias21_test.go`.
- `internal/kernal/posting/transaction.go` و`transaction_contract_test.go`: إضافة مدخل كتالوج `ErrRevaluationAlreadyApplied` وتصنيفه غير قابل لإعادة المحاولة؛ لا تغيير في سلوك `IsRetryable` للرصيد الموجود.
- لا مزود أسعار خارجي ولا اتصال شبكي داخل النواة؛ لا تعديل على `pkg` أو `storage` أو المرحلتين 10–12.
- لا تغيير في عقد `posting.PostEntry` أو `BuildIdempotencyScope`؛ استُخدم `posting.IdempotencyRevaluation` الموجود عبر دالة نواة جديدة **تُنتج كائن نطاق فقط** دون توصيله بمسار الترحيل أو SPI.

## 2. تغطية المهام 1–15

| المعرّف | المهمة من الخطة | التنفيذ والدليل | الحالة |
| --- | --- | --- | --- |
| P9-1 | جرد أنواع الأسعار ودلالاتها | أربعة أنواع في `exchange_rate.go`؛ إعادة التقييم تطلب `CLOSING`؛ [مصفوفة التقييم](./KERNEL_VALUATION_RULES_MATRIX.md) | PASS |
| P9-2 | تدقيق صلاحية السعر وإيجابيته والعملتين | `ExchangeRate.Validate` و`validateRevaluationRate`؛ `TestRevaluationRejectsNonPositiveProviderRate`, `TestRevaluationValidatesRateReferenceAgainstRequest`, `TestRevaluationValidatesInvertedAndSameCurrencyRates` | PASS |
| P9-3 | منع تعارض الأسعار للزوج والنوع والتاريخ والنطاق | `RegisterRate` يرفض `ErrDuplicateRate` للزوج نفسه وللزوج المعكوس في اليوم نفسه؛ `TestRateRegistryRejectsMissingAndDuplicateEffectiveDates` يتضمن subtest للزوج المعكوس | PASS |
| P9-4 | Effective Date ورفض السعر اللاحق | `findEffectiveRate` يمرّ على الأسعار المرتبة ويأخذ آخر سعر ≤ تاريخ التقييم؛ `TestRateRegistrySelectsLatestEffectiveRateAndRejectsFuture`, `TestRevaluationValidatesRateReferenceAgainstRequest` | PASS |
| P9-5 | اختيار السعر التاريخي عند تعدد الأسعار | سعران 3.70 في 2026-11-30 و3.75 في 2026-12-31: 3.70 في 2026-12-15، و3.75 في يومه، و`ErrRateNotFound` قبل الأول | PASS (كانت فجوة) |
| P9-6 | التحويل بين المعاملة والوظيفية بالدقة | `Convert` و`ConvertForDate` decimal-only مع Banker's rounding؛ أدلة الملحق 1.2 من مصفوفة التتبع و`conversion_precision_test.go` | PASS |
| P9-7 | same-currency conversion = 1 | `TestSameCurrencyConversionKeepsUnitRateExact`, `TestSameCurrencyRateValidatesCodeAndDateBeforeReturningOne` | PASS |
| P9-8 | تاريخ السعر المرجعي منفصل عن تاريخ التقييم | `FXResult.RateDate` و`GenerateRevaluationEntryAt`؛ `TestIAS21ExchangeRateAndRevaluation` | PASS |
| P9-9 | فروق الأصول والالتزامات واتجاه المدين/الدائن | `TestLiabilityRevaluationDirectionIsOppositeToAsset`: أصل ربح (مدين أصل/دائن ربح)، أصل خسارة، التزام زيادة = خسارة، التزام نقص = ربح؛ توازن 500/500 في الحالات الأربع | PASS (كانت فجوة) |
| P9-10 | الأرباح والخسائر المحققة وغير المحققة ضمن المنفّذ | `TestRevaluationOnlyBooksUnrealizedFXDifference` يثبت قيد `adjusting/accrual` على حسابي `UnrealizedGainID`/`UnrealizedLossID`؛ المحقق عند التسوية خارج نطاق الحزمة وموثق صراحةً | PASS داخل النطاق |
| P9-11 | قيد متوازن غير صفري | `posting.ValidateEntryDoubleEntry` داخل `GenerateRevaluationEntryAt`؛ `TestIAS21ExchangeRateAndRevaluation`, `TestValuationCoverageBranches` | PASS |
| P9-12 | منع ازدواج الأثر عند إعادة العملية | `RevaluationIdempotencyScope` (عملة المعاملة) + بصمة canonical v2 + `ErrRevaluationAlreadyApplied` غير القابل لإعادة المحاولة؛ `TestRevaluationRegistryRejectsConflictingPayloadForSameKey` بأربع طفرات، `TestRevaluationRegistryConcurrentRegisterHasSingleResult` (16 منافساً، فائز واحد)، `TestTransactionErrorClassification` | PASS داخل النواة؛ التوصيل والتخزين `G-024` |
| P9-13 | عكس القيد وربطه بالفترة الصحيحة | `BuildRevaluationReversalAt` يفوّض إلى `posting.BuildReversalEntryAt` ويبقي `FiscalPeriodID` صفراً؛ `TestRevaluationReversalDraftCarriesDateForPeriodResolution` يتحقق من التاريخ 2027-01-31 والربط، و`TestReversalDraftResolvesDestinationPeriodDuringPosting` يثبت حل الفترة في `PostEntry` | PASS (كانت فجوة توثيق) |
| P9-14 | Property/Fuzz للأسعار والحالات الحدية | `FuzzFXConversionNeverPanics` وهدف جديد `FuzzRateRegistryEffectiveSelectionNeverPanics` يختبر أربعة أنواع أسعار وتواريخ موجبة/سالبة وأسعار موجبة؛ المعيار PASS لا عدد التنفيذات | PASS (كان هدف Fuzz ناقصاً) |
| P9-15 | تحديث القواعد 021–024 والمصفوفة والثوابت | صفوف القواعد في `KERNEL_TRACEABILITY_MATRIX.md` وملحق 1.8، و`KERNEL_ACCOUNTING_INVARIANTS.md` قسم Phase 9، و`KERNEL_VALUATION_RULES_MATRIX.md`، و[ADR P9-01](./ard/P9_01_valuation_idempotency_and_rate_selection.md)، و[مراجعة الخطة](./KERNEL_REMEDIATION_PLAN_REVIEW.md) | PASS |

## 3. التغييرات الإنتاجية

| الملف | التغيير | السبب |
|---|---|---|
| `internal/kernal/valuation/revaluation.go` | `RevaluationIdempotencyScope` يربط مفتاح العملية بـ `posting.IdempotencyScope` و`IdempotencyRevaluation` | كان الثابت معرّفاً وغير مستخدم؛ لا يوجد نطاق قابل لتمريره إلى المضيف |
| `internal/kernal/valuation/revaluation.go` | `revaluationPayloadFingerprint` الإصدار 2: حقول مسبوقة بالطول، مبالغ بـ `ExactString`، تواريخ كاملة، وأبعاد ومعرّفات مستقرة؛ `Register` يرفض التعارض بـ `ErrRevaluationAlreadyApplied` ويعيد نسخة دفاعية | المراجعة المستقلة وجدت أن الإصدار 1 يستبعد حقولاً دلالية ويقتطع التواريخ ويستخدم فواصل ملتبسة |
| `internal/kernal/valuation/revaluation.go` | `RevaluationOperationKey` يوثّق أن `currency` هي عملة المعاملة؛ `RevaluationIdempotencyScope` يوثّق أنه غير موصول بـ `PostEntry`/SPI؛ `valuationSystemClock` يحصر قراءة الوقت في boundary | إزالة الغموض الذي أثارته المراجعة، والامتثال لقاعدةClock في الخطة |
| `internal/kernal/posting/transaction.go`, `transaction_contract_test.go` | `ErrRevaluationAlreadyApplied` في كتالوج أخطاء المرحلة 7، غير قابل لإعادة المحاولة، مع اختبار تصنيف | كان الخطأ الجديد بلا تصنيف موحد (`G-024` لا يزال مفتوحاً في SDK) |
| `internal/kernal/valuation/phase9_completion_test.go` *(جديد)* | سبعة اختبارات وهدف Fuzz جديد؛ طفرات في المبلغ والبعد ووقت اليوم وبصمة السياسة، وتأكيد اختلاف مفتاح العملة | إثبات المهام 5 و9 و12–14 التي كانت بلا دليل |
| `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` | قسم توضيحات المرحلة 9 (غير مُنشئ لقاعدة 28) | ربط القواعد 021–024 بالتنفيذ والاختبار وحدود المضيف |
| `internal/kernal/KERNEL_TRACEABILITY_MATRIX.md` | تحديث صفوف 021–024 وإضافة الملحق 1.8 | بوابة الأتمتة `TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests` تتطلب اسماً وتفصيلاً محدثين |
| `docs/KERNEL_VALUATION_RULES_MATRIX.md`, `docs/KERNEL_REMEDIATION_GAP_REGISTER.md`, `docs/ard/P9_01_*` | مصفوفة قواعد موسّعة، فجوة `G-024`، وقرار معماري | قاعدة 3.6.1 من الخطة: الوثائق والـ ADR في نفس تغيير العقد |
| `docs/KERNEL_REMEDIATION_PLAN_REVIEW.md` *(جديد)* | مراجعة الخطة كاملة: 2 Critical، 6 High، 10 Medium، 4 Low، و12 قاعدة مقترحة | طلب المستخدم مراجعة الخطة دون تعديلها |

> لم تُضف حزمة جديدة، ولا قاعدة محاسبية 28، ولا استيراد إلى `pkg` أو `storage` أو الشبكة. لم يتغير توقيع `PostEntry` أو `GenerateRevaluationEntryAt` أو `BuildRevaluationReversalAt`.

## 4. منهجية Test-First والأدلة الحمراء

1. كُتب `phase9_completion_test.go` قبل أي تعديل إنتاجي. سُجّل الفشل الأحمر المتوقع:

   ```text
   internal/kernal/valuation/phase9_completion_test.go:188:16: undefined: RevaluationIdempotencyScope
   ... (أربعة مواضع)
   FAIL github.com/osmai4603-collab/accounting_engine/internal/kernal/valuation [build failed]
   ```

2. بعد إضافة الدالة ظهر فشل أحمر ثانٍ في اختبار النسخ الدفاعي:

   ```text
   --- FAIL: TestRevaluationRegistryRejectsConflictingPayloadForSameKey
       registry leaked a mutable result: ...
   ```

   السبب: `Register` كان يعيد المرجع الداخلي المخزّن نفسه؛ أُصلح بإعادة `stored.Clone()` عند التسجيل الأول و`existing.entry.Clone()` عند replay أو التعارض.

3. بعد الإصلاح: `go test ./internal/kernal/valuation -count=1` → `ok`، ثم `go test -race ./internal/kernal/valuation ./internal/kernal/posting -count=1` → `ok`.

## 5. بوابات القبول

| البوابة | الدليل | النتيجة |
|---|---|---|
| G1 — Scope | تغييرات داخل `valuation` وكتالوج أخطاء `posting` ووثائق المرحلة 9؛ لا `pkg`/`storage`؛ architecture 11/11 | PASS |
| G2 — Architecture | `./scripts/verify_architecture.sh` → `RESULT: PASS - clean architectural base` | PASS |
| G3 — Accounting | جدول P9-1..P9-15 أعلاه، مصفوفة التتبع §1.8، وثوابت `RULE-KERNEL-021..024` | PASS |
| G4 — Precision | `TestKernelFinancialCodeHasNoFloatConversions` داخل `go test ./internal/kernal/types ./internal/kernal/valuation`؛ decimal و`ExactString` فقط في البصمة | PASS |
| G5 — Lifecycle | N/A — لا تغيير في آلة حالات القيد؛ مسودة العكس تستخدم البناء الحالي (`journal.EntryDraft`) | N/A |
| G6 — Posting | N/A — لم يُعدّل `PostEntry` أو خط أنابيب الترحيل؛ لا مسار ترحيل جديد لهذه المرحلة | N/A |
| G7 — Atomicity | N/A — لا تنفيذ تخزين أو معاملة جديدة؛ الذرية الدائمة للنطاق مؤجلة (`G-024`) | N/A |
| G8 — Idempotency | نفس الحمولة replay؛ أربع طفرات تُرفض بـ `ErrRevaluationAlreadyApplied` غير القابل لإعادة المحاولة؛ 16 منافساً فائز واحد؛ التوصيل والتخزين `G-024` | PASS داخل النواة |
| G9 — Reversal | `journal.ValidateReversalLinkage` + تاريخ العكس + `TestReversalDraftResolvesDestinationPeriodDuringPosting` لحل الفترة في `PostEntry` | PASS |
| G10 — Isolation | N/A — لا تغيير في Company Scope؛ أثبتته المرحلة 8 ولا يوجد منفذ جديد في المرحلة 9 | N/A |
| G11 — Concurrency | `go test -race ./internal/kernal/valuation ./internal/kernal/posting -count=1` نظيف؛ 16 منافساً على مفتاح التسجيل واحد. فحص المستودع الكامل مُعطَّل بفشلين خارج النطاق (آخر القسم 6) | PASS داخل النطاق |
| G12 — Audit | N/A — لا تغيير في سلسلة التدقيق أو البصمات المعتمدة؛ القيد مولّد داخل الحزمة | N/A |
| G13 — Rebuild | N/A — لا تغيير في إعادة بناء GL أو الإسقاط | N/A |
| G14 — Adapter Parity | N/A — لا محولات داخل النطاق؛ تكافؤ تخزين نطاق `revaluation` مشروط بـ `G-024` | N/A |
| G15 — Evidence | أوامر القسم 6 مع إصدار Go والالتزام المرجعي، وأدلة RED في القسم 4؛ السجلات الخام وcorpus Fuzz غير محفوظة (`G-010`, `G-011`)؛ الفشلان خارج النطاق موثّقان في القسم 6 | PASS داخل النطاق |
| Fuzz (إضافي) | هدفان بمدة 5s لكل منهما؛ لا panic ولا فشل؛ المعيار `PASS` لا عدد التنفيذات | PASS |

## 6. أوامر التحقق النهائية

| الأمر | النتيجة |
|---|---|
| `gofmt -l internal/kernal/` | لا إخراج |
| `go vet ./internal/kernal/valuation/...` | PASS |
| `go test ./internal/kernal/valuation -count=1` | ok |
| `go test -race ./internal/kernal/valuation ./internal/kernal/posting -count=1` | ok |
| `go test ./internal/kernal/valuation ./internal/kernal/posting ./internal/kernal/types ./internal/kernal/journal -count=1` | ok |
| `go test ./internal/kernal/... -count=1` | فشل واحد خارج النطاق في `doc_test.go` (انظر التفاصيل أدناه)؛ بقية حزم النواة ok |
| `go test ./... -count=1` | فشلان خارج النطاق: `internal/kernal` و`test/dr`؛ بقية الحزم ok |
| `go test -race ./internal/kernal/valuation ./internal/kernal/posting -count=1` | PASS، بلا بيانات متسابقة |
| `./scripts/verify_architecture.sh` | PASS — 11/11 دون تخفيض |
| `go test ./internal/kernal/valuation -run='^$' -fuzz='^FuzzFXConversionNeverPanics$' -fuzztime=5s` | `PASS` — بلا panic؛ 112,163 تنفيذ، العدد متغير مع الـ cache |
| `go test ./internal/kernal/valuation -run='^$' -fuzz='^FuzzRateRegistryEffectiveSelectionNeverPanics$' -fuzztime=5s` | `PASS` — بلا panic؛ 94,081 تنفيذ، 0 interesting جديد؛ corpus غير محفوظ (`G-011`) |
| `git diff --check` | نظيف |

**فشلان خارج نطاق المرحلة 9 في شجرة العمل الحالية (لم يُعدّلا هنا):**

1. `TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests` (`internal/kernal/doc_test.go:267`): صفّا `RULE-KERNEL-025` و`RULE-KERNEL-026` في مصفوفة التتبع يشيران إلى `TestMemoryAuditPhase10AtomicAppendAndCheckpoint` و`TestMemoryAuditTailDeletionIsDetectedByCheckpoint` و`TestMemoryAuditPhase10WithoutCheckpointHasNoTailAnchor` و`TestPostgresAuditPhase10AtomicSnapshotAndCheckpoint`. هذه الاختبارات موجودة فعلاً في `storage/memory` و`storage/postgres`، لكن الفاحص يفحص داخل `internal/kernal` فقط، فيراها غير معرّفة. هذا من عمل Phase 10 غير المُودَع، ولا علاقة له بتعديلات المرحلة 9 على الصف 024 وملحق 1.8.
2. `TestDRDrill_FullRecovery/05_audit_chain` (`test/dr/snapshot_test.go:59`): المتحقق يفترض صيغة تجزئة **v1**، بينما `storage/postgres/audit.go:66` يكتب **v2** عبر `kernalgovernance.CalculateHash` بعد عمل المراحل السابقة. المطابقة تفشل 3/3 ببيانات عشوائية مختلفة؛ السبب فرق الصيغة لا عشوائية البيانات. لا ملف من ملفات المرحلة 9 داخل مسار التجزئة.

> لم أُصلح هذين الفشلين لأنهما يعودان لملاك المرحلة 10/DR، وإصلاحهما توسيع نطاق غير مسموح به لهذه المرحلة. الحكم `PASS-SCOPED` يعكس ذلك صراحةً.

> تقرير التكافؤ `docs/reports/contract_parity_report.md` و`test/contract_parity/parity_report.md` يعاد توليده بواسطة `go test ./...`؛ يعاد إيداع النسخة المتتبعة بعد التشغيل (`G-010`).

## 7. حالة الفجوات

| الفجوة | الحالة السابقة | الحالة بعد المرحلة 9 |
|---|---|---|
| `G-024` — تخزين نطاق `revaluation` ذرياً في Adapter | غير موجودة | **Open (High)** — أُضيفت: الجانب النووي منفّذ ومُختبر؛ تحتاج المرحلة 11 |
| `G-011` — corpus الـ Fuzz غير محفوظ | Open | بدون تغيير؛ أحدث تشغيلين سجّلا 0 interesting جديد، ولا توجد مدخلات محفوظة في corpus |
| `G-001`, `G-014`, `G-023`, `G-002` | Closed | بدون تغيير |
| `G-004`, `G-007`, `G-012`, `G-013`, `G-016–G-022` | Open/Partially Closed | بدون تغيير؛ مالكوها خارج المرحلة 9 |

## 8. المخاطر المتبقية وحدود الضمان

- `RevaluationRegistry` في الذاكرة؛ لا يحمي بعد إعادة تشغيل العملية ولا يغطي انقطاع الاتصال بعد Commit. `RevaluationIdempotencyScope` غير موصول بـ `PostEntry` أو SPI، وترجمة `ErrRevaluationAlreadyApplied` إلى `ErrConflict` في SDK مؤجلة؛ الضمان الدائم مشروط بـ `G-024`.
- لا يوجد مزود أسعار حي داخل النواة؛ دقة السعر ومصدره مسؤولية المضيف عبر `RateProvider`.
- غلاف `GenerateRevaluationEntry` كان يقرأ وقت النظام مباشرة؛ حُصر الآن في `valuationSystemClock`، والدليل الحتمي هو `GenerateRevaluationEntryAt` مع `now` صريح. لا استثناء مفتوح في هذه المرحلة.
- الأرباح والخسائر المحققة عند التسوية ليست ضمن التنفيذ الحالي؛ لا يُدعى أن النواة تحسبها.
- جودة الـ assertions الدلالية لا تزال مراجعة بشرية موثقة (`G-012`)، رغم أتمتة وجود القواعد وصحة مراجع الاختبارات.

## 9. المراجعة المستقلة

| المراجع | قائمة التحقق | النتيجة |
|---|---|---|
| مراجعة diff مستقلة (READ-ONLY) | مطابقة P9-1..P9-15، قواعد 3.1–3.6، بوابات G1–G15، سلامة الـ diff وعدم توسيع النطاق؛ إعادة تشغيل gofmt/vet/tests/race/architecture/Fuzz واختبار مصفوفة التتبع | **PASS مع ملاحظات**؛ لا مشاكل Critical |
| مراجعة خطة مستقلة (READ-ONLY) | الخطة كاملة (986 سطراً) مقابل تقارير المراحل 0–9 وسجل الفجوات | 2 Critical، 6 High، 10 Medium، 4 Low + 12 قاعدة مقترحة؛ أُثبتت في [مراجعة الخطة](./KERNEL_REMEDIATION_PLAN_REVIEW.md) |

معالجات مراجعة الـ diff:

| الملاحظة | المعالجة في هذا التغيير |
|---|---|
| High — ادعاء «تمرير النطاق للمضيف» غير مطابق للكود | صُحّحت الصياغة في التقرير وADR وسجل الفجوات والثوابت والمصفوفة إلى «كائن نطاق متاح وغير موصول بـ `PostEntry`/SPI»، وأُدرج التوصيل صراحةً في `G-024` |
| Medium — بصمة غير مكتملة وغير canonical | إصدار 2 بحقول مسبوقة بالطول، `ExactString`، تواريخ RFC3339Nano كاملة، أبعاد ومعرّفات مستقرة، وأربع طفرات سلبية |
| Medium — الادعاء بمنع الزوج المعكوس بلا اختبار | subtest في `TestRateRegistryRejectsMissingAndDuplicateEffectiveDates` |
| Medium — غموض عملة مفتاح العملية | تعريف صريح أنها عملة المعاملة، مع اختبار اختلاف المفتاح عن العملة الوظيفية وتصحيح `ias21_test.go` |
| Medium — دليل فترة العكس أقوى من الاختبار | الاستشهاد بـ `TestReversalDraftResolvesDestinationPeriodDuringPosting` في المصفوفة والتقرير والـ ADR، وتوثيق أن مسودة التقييم تفوّض إلى نفس البناء |
| Low — أرقام Fuzz غير قابلة لإعادة الإنتاج، عدد الأدلة، وقت النظام، تناقض 021، تصنيف الخطأ | توثيق `PASS` بدل الأعداد، تصحيح «سبعة اختبارات وهدف Fuzz»، حصر الوقت في `valuationSystemClock`، إعادة صياغة `RULE-KERNEL-021`، وإدراج الخطأ في كتالوج `posting` مع اختبار تصنيف |

## 10. الحكم

`PHASE 9 GATE: PASS-SCOPED (داخل نطاق النواة)`. وفق القاعدة المقترحة 6 في [مراجعة الخطة](./KERNEL_REMEDIATION_PLAN_REVIEW.md)، يعني هذا أن المهام P9-1..P9-15 استُوفيت داخل النواة بأدلة تشغيل قابلة لإعادة الإنتاج، وأن العمل المؤجل مصنّف بفجوة ومالك ومرحلة إغلاق. `G-024` فجوة High على عاتق المضيف/المحولات والـ SDK في المرحلة 11: توصيل النطاق بـ `PostEntry` وتوسيع SPI وترجمة الخطأ وتخزينه ذرياً. لا تُعتبر idempotency إعادة التقييم مكتملة على مستوى الإنتاج قبل إغلاقها، ولا يُترجم هذا الحكم إلى بوابة المرحلة 11 أو 12.
