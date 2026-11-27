# ADR P1-03 — النماذج المرجعية المكررة: `masterdata` مقابل `ledger`

- **الحالة:** مقبول (Accepted) — يُثبّت الوضع القائم ويمنع استخدام النماذج المرجعية كبديل عن المصدر المخول.
- **التاريخ (UTC):** 2026-11-21T21:04:06Z
- **المالك:** مالك معمارية النواة + مالك السطح العام
- **المرجع:** [الخطة، المرحلة 1 مهام 9–10](../plans/KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [ADR P1-01](./P1_01_kernel_package_map.md)، [ADR P1-02](./P1_02_gl_source_of_truth.md)، [ADR P7-01](./P7_01_public_surface_decision.md)
- **ذو صلة:** [قواعد الثوابت 016–020](../../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)

## السياق

يوجد نموذجان للحساب ونموذجان للفترة المالية داخل النواة:

| النموذج المخول | النموذج المرجعي | دليل التنفيذ |
|---|---|---|
| `masterdata.Account` (`masterdata/types.go:79-103`) | `ledger.Account` (`ledger/account.go:24-36`) | الأول يحمل 23 حقلاً منها `EffectiveFrom/To`، `IsControlAccount`، `IsContraAccount`، `RequiresCostCenter`، `IsCashEquivalent`، `CashFlowActivity`، `IFRSCategory`، `DisplayOrder`؛ الثاني يحمل 11 حقلاً فقط. |
| `masterdata.FiscalPeriod` / `FiscalYear` (`masterdata/types.go:105-124`) | `ledger.FiscalPeriod` / `FiscalYear` (`ledger/period.go:33-53`) | الأول تنتجه `GenerateFiscalYear` وتتحقق منه `ValidateFiscalYear` في `masterdata/fiscal_calendar.go`؛ الثاني يعرّف `Contains` فقط ويرجع `false` لفترات التسوية 13/14. |

البحث في المستودع يثبت أن `ledger` **لا يُستخدم داخل النواة إطلاقاً**: المستهلك الوحيد هو الجسر العام `pkg/api/v1/ledger` عبر أسماء مستعارة (`pkg/api/v1/ledger/account.go:3-18`, `period.go:3-23`)، وهو استثناء مسموح به بموجب ADR P7-01. لا توجد دالة تحويل (`converter`) بين النموذجين في أي مكان.

القاعدة 3.1/5 من الخطة تمنع حذف أي نوع أو حقل قبل تحليل الاستخدامات وتوثيق خطة الترحيل؛ لذلك لا تُحذف الأنواع المرجعية في هذه المرحلة.

## القرار

1. **مصدر الحقيقة:** `masterdata` هو المصدر المخول الوحيد لقواعد شجرة الحسابات، صلاحية الحساب، نفاذه الزمني، التقويم المالي، حالات الإقفال، والفترتين 13/14. كل مسار ترحيل يتحقق من هذه القواعد عبر `masterdata` (`posting/pipeline.go:149,185,205-216,254,261`) ولا عبر `ledger`.
2. **دور `ledger`:** حزمة DTO مرجعية للتوافق مع `pkg/api/v1/ledger` فقط. لا تُستخدم للتحقق من الأهلية ولا كسجل حركة ولا كبديل عن `masterdata` (انظر ADR P1-02).
3. **قواعد التحويل بين النموذجين (إلزامية لأي محول جديد):**
   - الحقول المطابقة (`ID`, `CompanyID`, `Code`, `Name`, `Type`, `NormalBalance`, `Currency`, `ParentID`, `IsReconcilable`, `IsActive`, `SubClassification`) تُنقل بالقيمة وبعد التحقق من تطابق قيم التعداد.
   - حقول `masterdata.Account` التي لا نظير لها في `ledger.Account` لا تُسقَط بصمت في سياق المجال؛ أي إسقاط مقصود يجب أن يكون في طبقة API/المضيف مع توثيق صريح، لأن هذه الحقول تحمل قواعد أهلية (`RULE-KERNEL-017`) وامتثال (`RULE-KERNEL-019`).
   - `AccountTypeRevenue` يبقى اسماً مستعاراً لـ `AccountTypeIncome` بقيمة `"income"` (`ledger/account.go:13-14`)، ويُعاد تصديره في `pkg/api/v1/ledger/account.go:15`. لا يُغيَّر ولا يُحذف دون ADR جديد وخطة هجرة.
   - `ledger.FiscalPeriod.Contains` لا تُستخدم لاكتشاف فترة الترحيل؛ الترحيل يستخدم `masterdata.ValidatePeriodForPostingWithSoftClose` و`ValidateAdjustmentPeriodForPostingWithSoftClose`.
   - لا تحويل ضمني ولا إعادة تعريف للأنواع داخل kernel؛ أي تحويل مستقبلي يجب أن يكون دالة صريحة في طبقة الجسر العام مع اختبار يثبت حفظ الحقول الدلالية.
4. **منع الانحراف:** إذا أُضيف حقل إلى `masterdata.Account` أو `FiscalPeriod`، يجب تقييم أثره على DTO العام في نفس التغيير؛ ويُسجَّل أي إسقاط مقصود في السجل العام. لا تُضاف حقول أعمال إلى `ledger` بحجة "التكافؤ".

## البدائل المرفوضة

| البديل | سبب الرفض |
|---|---|
| توحيد النموذجين في نوع واحد | كسر توافق `pkg/api/v1/ledger`؛ النوع المرجعي أخف عمداً لأغراض العرض. |
| جعل `ledger` هو المصدر | فقدان قواعد النفاذ والأهلية والدورة المحاسبية التي يحملها `masterdata`. |
| حذف `AccountTypeRevenue` فوراً | اسم عام مستعار وموروث؛ يتطلب ADR هجرة منفصلاً. |

## التبعات والمخاطر المتبقية

- لا تغيير في الشيفرة في هذه المرحلة؛ التحديث يقتصر على `ledger/doc.go` ووثائق المرحلة 1.
- خطر انحراف الحقول بين النموذجين قائم لأن لا أتمتة تثبت التطابق اليوم؛ يُسجَّل كفجوة `G-016` بخطورة Medium ومالك `kernel/ledger + api/v1` ومرحلة معالجة 3/11، ولا يُغلق بوثيقة فقط.

## ملحق المرحلة 3 — أتمتة منع الانحراف (2026-11-22)

أُضيف اختبار `TestReferenceModelsDoNotDriftSilently` في `internal/kernal/masterdata/masterdata_integrity_test.go` ليفحص انعكاسياً أن:

1. كل حقل في `ledger.Account` و`ledger.FiscalPeriod` يوجد بنفس الاسم ونوع Kind في `masterdata`؛
2. الحقول الـ12 غير المنقولة من `masterdata.Account` هي بالضبط المجموعة الموثقة أعلاه، وأي حقل جديد يُضاف إلى المصدر دون تحديث هذه المجموعة يفشل الاختبار؛
3. الحقل `ledger.FiscalYear.Periods` هو الاستثناء الوحيد المسموح به في DTO السنة المالية.

هذا يغطي الجانب النووي من `G-016` فقط؛ التحقق من جسر `pkg/api/v1/ledger` نفسه يبقى ضمن المرحلة 11. لم يُضف أي محوّل أو حقل أعمال إلى `ledger`، واستمرار `AccountTypeRevenue` كاسم مستعار دون تغيير.
