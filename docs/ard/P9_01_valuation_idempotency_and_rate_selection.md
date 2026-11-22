# ADR P9-01 — اختيار أسعار الصرف وتكرار إعادة التقييم

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernal/valuation` + مضيف التخزين
- **المرجع:** [الخطة، المرحلة 9](../KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [مصفوفة قواعد التقييم](../KERNEL_VALUATION_RULES_MATRIX.md)
- **ذو صلة:** [ADR P7-01](./P7_01_public_surface_decision.md)، [عقد idempotency](../KERNEL_IDEMPOTENCY_CONTRACT.md)، [سجل الفجوات](../KERNEL_REMEDIATION_GAP_REGISTER.md)

## السياق

1. `RateRegistry` يخزّن عدة أسعار لزوج العملات ونوع السعر، لكن الاختبارات السابقة لم تثبت اختيار «أحدث سعر لا يتأخر عن تاريخ التقييم» عند تعدد الأسعار، ولم يكن هناك هدف Fuzz يختار نوع السعر والتاريخ معاً.
2. `RevaluationRegistry` كان يمنع تسجيل النتيجة مرتين لنفس مفتاح العملية، لكنه كان يعيد النتيجة المخزنة دون فحص الحمولة: إعادة المفتاح بنتيجة مختلفة كانت تمرّ بصمت. كما أن `posting.IdempotencyRevaluation` كان معرّفاً وغير مستخدم، فلم يكن هناك كائن نطاق محايد. النطاق أصبح متاحاً الآن، ووُصل بعقد SPI وطبقة التخزين في المرحلة 11 عبر `storage.OperationRevaluation`.
3. قيد إعادة التقييم يُنشأ بمعرّفات عشوائية في كل استدعاء، لذا لا يصلح digest طلب الترحيل الكامل كبصمة لتكرار عملية التقييم نفسها؛ ما يحدد هوية العملية هو المجال، لا معرف القيد العشوائي.
4. `BuildRevaluationReversalAt` ينتج مسودة عكس بتاريخ صريح؛ فترة العكس لا تُحل داخل الدالة بل في `PostEntry`، وهذا يحتاج دليلاً صريحاً حتى لا يُفترض أن القيد يحمل الفترة الصحيحة قبل الترحيل.

## القرار

1. **اختيار السعر:** `RateRegistry.GetRate` يختار أحدث سعر نافذ بـ `EffectiveDate <= asOf` بعد ترتيب الأسعار تصاعدياً عند التسجيل. السعر المستقبلي والسعر غير الموجب والمرجع غير المتطابق مرفوضة. تسجيل سعرين لنفس الزوج والنوع واليوم — أو للزوج وعكسه — مرفوض بـ `ErrDuplicateRate`. الأسعار الأربعة `SPOT`/`CLOSING`/`AVERAGE`/`HISTORICAL` تبقى أنواعاً صالحة، بينما تستخدم إعادة التقييم `CLOSING` تحديداً.
2. **تاريخان مستقلان:** `FXResult.RateDate` يسجّل تاريخ نفاذ السعر المرجعي، و`GenerateRevaluationEntryAt` يستخدم تاريخ التقييم للقيد. لا يُستبدل أحدهما بالآخر.
3. **اتجاه الخصوم:** فرق القيمة بالعملة الوظيفية يُحدد الربح/الخسارة مع عكس الإشارة للحالات الالتزامية؛ قيد التقييم يستخدم حسابي `UnrealizedGainID` و`UnrealizedLossID` ويبقى قيد تسوية (`adjusting/accrual`) متوازناً وغير صفري. الأرباح والخسائر المحققة عند التسوية ليست ضمن ما تنفذه النواة هنا، وتبقى قرار ترحيل للمضيف.
4. **نطاق التكرار:** `RevaluationIdempotencyScope(company, account, currency, date)` يشتق من `RevaluationOperationKey` ويعيد `posting.IdempotencyScope{CompanyID, Operation: revaluation, Key}` بعد التحقق من `Validate()`. `currency` هي عملة المعاملة الأجنبية للمركز لا العملة الوظيفية، و`date` يوم التقييم؛ لذلك لا ينتج عن المركز نفسه مفتاحان مختلفان. لا تُغيَّر قواعد `BuildIdempotencyScope` الخاصة بالترحيل والعكس؛ العملية الثالثة تبقى منفصلة عند حد التفرد في التخزين، وقد وُصلت في المرحلة 11 عبر تلميح نطاق موثَّق في `PostOptions.Scope` دون تغيير توقيع `PostEntry` أو توسيع `PostingCommit`.
5. **بصمة النتيجة:** `RevaluationRegistry` يحفظ بصمة SHA-256 إصدار 2 لكل الحقول الدلالية المستقرة، بترميز canonical ذي حقول مسبوقة بالطول لمنع التباس الفواصل، وبأسطر مرتبة بعد الترميز: الشركة، اليومية، الفترة، المرجع، أرقام القيد، التصنيف، التاريخان الكاملان، العملة، السعر، بصمة السياسة، الأبعاد، ومبالغ الأسطر. المبالغ تستخدم `Money.ExactString()` لا `Money.String()` لأن الأخيرة تقريب عرض. معرّفات القيد والأسطر والطوابع الزمنية والحالة فقط هي المستبعدة لأنها تتغير عند إعادة توليد النتيجة نفسها. إعادة المفتاح بالبصمة نفسها replay غير خطأ؛ إعادة المفتاح ببصمة مختلفة تُرفض بـ `ErrRevaluationAlreadyApplied` وتُعاد النتيجة المخزنة. الخطأ نفسه هو مدخل كتالوج `posting.ErrRevaluationAlreadyApplied` وغير قابل لإعادة المحاولة؛ النسخ الدفاعي يمنع تعديل النتيجة الداخلية عبر القيمة المُعادة.
6. **حدود الضمان:** `RevaluationRegistry` في الذاكرة أداة حماية داخل العملية فقط؛ الضمان النهائي ضد التكرار بعد إعادة تشغيل العملية أو انقطاع الاتصال هو أن يستخدم المضيف نطاق `revaluation` في تخزين idempotency الذري وفق المرحلة 7. المرحلة 9 أنتجت كائن النطاق، والمرحلة 11 أغلقت الجانب المضيف: `pkg/spi/storage` يعرف العملية الثالثة، والمحولان يخزنان النطاق والبصمة ذرياً، وترجمة التعارض إلى `ErrConflict` تتم في SDK. لا يوجد مسار عام جديد لإنشاء قيد إعادة تقييم؛ النواة لا تملك قاعدة بيانات ولا مزود أسعار خارجي ولا اتصال شبكي.
7. **العكس:** مسودة العكس تحمل `EffectiveDate` تاريخ العكس و`ReversalOfID` وتترك `FiscalPeriodID` صفراً؛ `BuildRevaluationReversalAt` يفوّض إلى `posting.BuildReversalEntryAt`، و`PostEntry` هو من يشتق فترة العكس من التاريخ قبل التفويض، كما يثبت `TestReversalDraftResolvesDestinationPeriodDuringPosting`. ربط الأصل بالعكس يُتحقق منه بـ `journal.ValidateReversalLinkage`.

## البدائل المرفوضة

| البديل | سبب الرفض |
| --- | --- |
| استخدام `FingerprintRequest` من `posting` كبصمة لإعادة التقييم | يتضمن معرّفات القيد والأسطر العشوائية، فيُرفض replay لنتيجة محاسبية متطابقة ويخلط بين بصمة الحمولة ونطاق الترحيل. |
| إعادة استخدام نطاق `post` أو `reverse` لإعادة التقييم | يسمح بتعارض مفتاح نصي مع عملية ترحيل أو عكس ذات الصلاحية المختلفة؛ المرحلة 7 تعتمد `Operation` جزءاً من التفرد. |
| اعتبار `RevaluationRegistry` ضماناً دائماً ضد التكرار | من الذاكرة وغير مشترك بين العمليات؛ لا يصمد أمام إعادة تشغيل ولا يغطي انقطاع الاتصال بعد Commit. |
| إسقاط الفرق الصفري أو السماح بقيد غير متوازن لتسهيل التسوية | يخالف `RULE-KERNEL-024` ويخفي أثر التقييم. |
| إضافة مزود أسعار شبكي أو سعر حي داخل `internal/kernal` | يخالف حدود Embedded Core؛ الأسعار تُحقن عبر `RateProvider`. |

## التبعات والمسؤوليات

- **النواة:** قواعد اختيار السعر، بصمة نتيجة مستقرة، نطاق idempotency محايد، رفض التعارض، واختبارات وحدة/Fuzz/تزامن قابلة لإعادة التشغيل.
- **المضيف/المحولات:** تخزين نطاق `revaluation` والبصمة ذرياً مع القيد (`G-024`) عبر `storage.OperationRevaluation`، وترجمة التعارض في SDK، وتوفير `RateProvider` موثوق من خارج النواة.
- **الوثائق:** [مصفوفة قواعد التقييم](../KERNEL_VALUATION_RULES_MATRIX.md)، [تقرير المرحلة 9](../KERNEL_REMEDIATION_PHASE9_REPORT.md)، وثوابت النواة ومصفوفة التتبع في نفس التغيير.

## الأدلة التنفيذية

`TestRateRegistrySelectsLatestEffectiveRateAndRejectsFuture`، `TestRateRegistryRejectsMissingAndDuplicateEffectiveDates` (بما فيه الزوج المعكوس)، `TestLiabilityRevaluationDirectionIsOppositeToAsset`، `TestRevaluationIdempotencyScopeIsDeterministicAndOperationScoped`، `TestRevaluationRegistryRejectsConflictingPayloadForSameKey` (بما فيه أربع طفرات حقلية)، `TestTransactionErrorClassification`، `TestRevaluationRegistryConcurrentRegisterHasSingleResult`، `TestRevaluationReversalDraftCarriesDateForPeriodResolution`، `TestReversalDraftResolvesDestinationPeriodDuringPosting`، `TestRevaluationOnlyBooksUnrealizedFXDifference`، `FuzzRateRegistryEffectiveSelectionNeverPanics`. وتُكملها في المرحلة 11 اختبارات `TestPhase11_Revaluation*` على Memory وPostgreSQL مع `docs/ard/P11_01_revaluation_idempotency_spi.md`؛ وتُغلق معاً ضمان idempotency على مستوى التخزين.
