# ADR P5-01 — اختيار السياسات، منع التعارض، والبصمة التاريخية للقيد

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernal/policy`
- **المرجع:** [الخطة، المرحلة 5](../KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [تقرير المرحلة 5](../KERNEL_REMEDIATION_PHASE5_REPORT.md)
- **ذو صلة:** [ADR P1-02](./P1_02_gl_source_of_truth.md)، [RULE-KERNEL-010/019](../../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)

## السياق

أثبتت مراجعة المرحلة 5 أربع فجوات قابلة للإثبات في `internal/kernal/policy`:

1. `PostingPolicyFor` و`CurrencyPolicyFor` يختاران أحدث سياسة بتاريخ نفاذ يسبق تاريخ القيد، دون تعريف صريح لنافذة النفاذ ولا رفض للتداخل. النتيجة حتمية عملياً لكنها غير موثقة كعقد، ولا يوجد سلوك معلوم عند إدخال نسخة جديدة في منتصف نافذة قائمة.
2. لا يوجد حقل نهاية نفاذ، ولذلك لا معنى اختبارياً لحدّ "عند النهاية"، ولا يمكن تمثيل الفجوات الزمنية التي يجب أن تفشل مغلقاً.
3. سياسة العملة / الترحيل تُحل في لحظة الترحيل فقط؛ لا يُسجَّل في القيد أي دليل على النسخة المستخدمة. أي تعديل لاحق في مخزن السياسات يجعل التفسير التاريخي غير قابل للتدقيق.
4. أهلية الحساب في `CheckAccountEligibility` لا تتحقق من نطاق الشركة ولا من نفاذ الحساب في تاريخ القيد، رغم أن مسار Pipeline يتحقق من ذلك عبر `masterdata`.

## القرار

1. **نافذة النفاذ (نصف مفتوحة):** تُضاف `EffectiveTo *time.Time` إلى `PostingPolicy` و`CurrencyPolicy`. النافذة `[EffectiveDate, EffectiveTo)`؛ القيمة `nil` تعني "حتى النسخة التالية". هذا يحافظ على سلوك "أحدث نسخة نافذة" الحالي عندما لا تُستخدم `EffectiveTo`.
2. **أولوية اختيار حتمية:** تُفرز النسخ لكل شركة حسب `EffectiveDate` (UTC) عند التسجيل. تُقبل النسخة التي تحقق `start <= asOf < end`. لا توجد حالة تعادل لأن تكرار `EffectiveDate` مرفوض، والتداخل يُرفض عند التسجيل بـ`ErrPolicyOverlap`.
3. **رفض التعارض:** تسجيل نسخة تتداخل مع نافذة سابقة صريحة، أو تمتد إلى ما بعد بداية الخليفة المباشر، يُرفض بـ`ErrPolicyOverlap`. النسخ المتجاورة (`end == next.start`) مسموحة، والفجوات مسموحة وتفشل مغلقاً بـ`ErrPolicyNotFound`.
4. **الفشل المغلق:** غياب أي سياسة نافذة للشركة/التاريخ المطلوب يُعيد `ErrPolicyNotFound`؛ لا سياسة افتراضية داخل النواة. عندما يوفّر المضيف مخزن سياسات مؤرخاً، يطبّق SDK القاعدة نفسها ولا ينشئ سياسة اصطناعية.
5. **بصمة السياسة على القيد:** تُضاف `journal.JournalEntry.PolicyFingerprint` وتُملأ في `PostingPipeline` بعد انتهاء كل التحقق وقبل التفويض إلى Executor. البصمة `sha256:` لتمثيل قانوني ثابت لحقول السياسات المحلولة (الشركة، التواريخ، الحدود، التقريب، العملات، الأبعاد، الإقفال). ترتيب `AllowedCurrencies` لا يؤثر عليها. الحقل بيانات تدقيق فقط، وليس تفويضاً، ويدخل في `ValidateImmutability`.
6. **الأهلية:** `CheckAccountEligibility` يرفض حساباً من شركة أخرى (عند توفر نطاق في القيد) أو حساباً غير نافذ في `entry.EffectiveDate` بـ`ErrAccountNotEffective`، إضافة إلى القواعد القائمة (نشط، غير رقابي، مركز تكلفة، عملة).
7. **الإقفال الناعم:** قرار السماح بالترحيل في فترة `SoftClosed` يظل مملوكاً لـ`masterdata` ويُمرَّر إليه صراحةً من السياسة المُحددة للتاريخ. `HardClosed` ممنوع دائماً. لا تُضاف موافقات أو صلاحيات مستخدمين.

## البدائل المرفوضة

| البديل | سبب الرفض |
| --- | --- |
| الاحتفاظ بـ"أحدث تاريخ" ضمنياً دون نهاية نفاذ | لا يوفّر حدّاً اختبارياً للنهاية ولا يمنع إعادة تفسير تاريخي صامتة عند إدخال نسخة جديدة. |
| رفض كل نسخة جديدة بعد أول نسخة مفتوحة | يجعل تطور السياسة مستحيلاً دون واجهة تحديث، ويعاقب الاستخدام المشروع. |
| تخزين نسخة كاملة من السياسة داخل كل سطر قيد | يضخم النموذج ويكرر بيانات قد تحتوي على قوائم/مؤشرات؛ البصمة تعطي الدليل المطلوب دون ازدواج. |
| حساب البصمة من شكل JSON العام بعد الترحيل | شكل JSON يتغير بإضافة حقول، ويعتمد على ترتيب الحقول واستهلاك المستهلكين الخارجيين؛ التمثيل القانوني الثابت أعم. |
| نقل قرار Soft/Hard Close إلى `policy` | يخالف ADR P3-01؛ الفترة وحالتها مملوكتان لـ`masterdata`. |

## التبعات والمسؤوليات

- **على النواة:** تسجيل السياسات بطريقة حتمية وغير متعارضة، فشل مغلق عند الغياب، احترام نطاق الشركة، وإنتاج بصمة قابلة لإعادة الإنتاج على القيد المرحّل.
- **على المضيف/المحول:** حفظ سجل نسخ السياسات واستعادته عند إعادة التشغيل؛ البصمة المحفوظة لا تكفي وحدها إذا فقدَ المضيف النسخ القديمة. أُغلقت المسؤولية في المرحلة 11 عبر `storage.PolicyStore` وADR `P11-04`، مع منع التداخل في المحولين.
- **على السطح العام:** إضافة حقول اختيارية (`EffectiveTo`، `PolicyFingerprint`) فقط؛ لا حذف ولا تغيير توقيع. أُضيف منفذ `storage.PolicyStore` عبر `Ports.Private` في المرحلة 11 دون توسيع `Ports`. استثناء `pkg/api/v1` في ADR P7-01 لم يُوسّع.
- **على المرحلة 6/7:** البصمة تُنشأ داخل المسار المركزي المعتمد، وتُحفظ ذرياً مع القيد بواسطة Executor وفق عقد المرحلة 7؛ لا معاملة جديدة داخل النواة.

## الأدلة التنفيذية

`TestPolicyEffectiveDateBoundaryMatrix`, `TestPolicyEngineRejectsUnresolvedOverlaps`, `TestPolicySelectionIsScopedAndFailsClosed`, `TestStoredPoliciesAreIsolatedFromSharedSlicesAndPointers`, `TestCurrencyPolicyValidationAndRoundingAudit`, `TestSelectedPostingPolicyControlsSoftCloseOnly`, `TestAccountEligibilityDecisionMatrix`, `TestPolicyFingerprintIsCanonicalAndContentBound`, `TestPolicyEngineConcurrentResolveAndVersionUpdate`, `TestConcurrentRegistrationOfSameEffectiveStartHasOneWinner`, `TestPostEntryStampsPolicyFingerprintAndKeepsHistoryStable`. وتكملها في المرحلة 11 اختبارات `TestPhase11_PolicyVersionHistoryIsAppendOnlyAndNonOverlapping` و`TestPhase11_PolicyVersionCompanyScope` واختبارات تحميل SDK.
