# ADR P3-01 — سلامة البيانات الرئيسية:Nطاق الشركة، شجرة الحسابات، والتأريخ النافذ

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernal/masterdata`
- **المرجع:** [الخطة، المرحلة 3](../plans/KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [تقرير المرحلة 3](../reports/KERNEL_FINAL_VERIFICATION_REPORT.md)
- **ذو صلة:** [ADR P1-03](./P1_03_reference_model_duality.md)، [قواعد الثوابت 016–020](../../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)

## السياق

أثبتت مراجعة المرحلة 3 أربع فجوات قابلة للإثبات في `internal/kernal/masterdata`:

1. مسارات `ValidateCompanyScope` و`ValidatePeriodForPostingWithSoftClose` و`ValidateAdjustmentPeriodForPostingWithSoftClose` كانت تمرر `CompanyID` صفرياً إلى Reader قبل رفضه؛ أي محول معيب كان يستطيع إرجاع سجلات بمعرّف شركة صفري وقبولها ضمنياً.
2. `DetectCyclicHierarchy` و`BuildHierarchyTree` يفحصان الحلقات والأبوة المفقودة فقط، ولا يمنعان أباً من شركة أخرى ولا مزج شركتين في لقطة COA واحدة.
3. `ValidateCanPostToAccount` يستدعي `time.Now()` مباشرة؛ مسار الترحيل نفسه يستخدم `ValidateCanPostToAccountAsOf`، لكن القاعدة 11 من الخطة تمنع الوقت الحالي المباشر داخل منطق المجال.
4. `ValidateCanPostToAccountAsOf` يمرر تاريخ القيد إلى `IsCurrent` فقط، لا إلى Reader؛ لذلك يعتمد تفسير التاريخ على المحول أن يُرجع النسخة النافذة، دون عقد صريح بذلك. إضافةً إلى ذلك، لا توجد أتمتة تمنع انحراف حقول `masterdata` عن DTOs في `ledger` (G-016).

## القرار

1. **الفشل المغلق للنطاق (RULE-KERNEL-016):** أي `CompanyID` صفري يُرفض قبل أي وصول إلى Reader في `ValidateCompanyScope`، و`ValidateAccountScope`، و`ValidateCanPostToAccountAsOf`، و`ValidatePeriodForPostingWithSoftClose`، و`ValidateAdjustmentPeriodForPostingWithSoftClose`، و`ValidateJournalScope`. الخطأ المعتمد `ErrCompanyScopeRequired`.
2. **وحدة نطاق شجرة الحسابات (RULE-KERNEL-016):** لقطة COA الكاملة تنتمي لشركة واحدة؛ الأب الموجود داخل اللقطة يجب أن يحمل نفس `CompanyID` الذي يحمله الابن. المخالفة تُرفض بـ`ErrAccountIdentityMismatch` قبل بناء الشجرة. الحسابات التي تحمل معرّف شركة صفري تُقبل فقط إذا كانت اللقطة كلها بلا نطاق (توافق مع مدخلات قديمة)، ولا تُمزج مع حسابات منشأة.
3. **الوقت عبر عقد قابل للحقن (قاعدة الخطة 11):** يُضاف `masterdata.Clock` و`SystemClock`، وتُضاف `ValidateCanPostToAccountWithClock`. الدالة القديمة `ValidateCanPostToAccount` تبقى كغلاف توافق لكنها تفوّض إلى `SystemClock` بدل استدعاء `time.Now()` داخل منطق التحقق. مسار الترحيل ملتزم بـ`ValidateCanPostToAccountAsOf`.
4. **منفذ النسخة النافذة:** يُضاف منفذ اختياري `EffectiveAccountReader.GetAccountAsOf(ctx, companyID, accountID, asOf)`. إذا وفّره المحول، تستخدمه `ValidateCanPostToAccountAsOf` لاختيار نسخة الحساب الصحيحة زمنياً؛ وإلا يُستخدم `GetAccount` مع فشل مغلق عبر `IsCurrent`. لا يتغير توقيع `Reader`، لذلك لا تنكسر المحولات الحالية.
5. **نافذة النفاذ (RULE-KERNEL-017):** `Account.Validate` و`IsValid` يرفضان نافذة معكوسة (`EffectiveTo < EffectiveFrom`) بـ`ErrInvalidEffectiveRange`. الحدّان مغلقان، و`EffectiveTo == nil` يعني "ما زال نافذاً".
6. **هوية اليومية (RULE-KERNEL-016):** تُضاف `ValidateJournalScope` ليفحص المحول قبل الوثوق بسياسات اليومية؛ لا يوجد تعريف ضمني لليومية من شركة أخرى.
7. **منع انحراف النماذج المرجعية (G-016):** اختبار انعكاسي آلي يقارن حقول `masterdata.Account`/`FiscalPeriod`/`FiscalYear` بنظائرها في `ledger`، ويتطلب أن تكون الحقول غير المنقولة مُدرجة صراحةً في مجموعة موثقة في ADR P1-03. لا يُضاف محوّل داخل النواة؛ تحويل السطح العام يبقى في طبقة الجسر وفق P1-03.

## البدائل المرفوضة

| البديل | سبب الرفض |
| --- | --- |
| تغيير توقيع `Reader.GetAccount` لإضافة `asOf` | كسر جميع محولات SPI الحالية؛ المنفذ الاختياري يحقق الهدف دون كسر توافق. |
| حذف `ValidateCanPostToAccount` | سلوك عام مستخدم خارج النواة؛ القاعدة 3.1/5 تمنع الحذف قبل خطة هجرة. |
| إسقاط `ledger.Account` أو توحيده مع `masterdata.Account` | يتعارض مع ADR P1-03 ويكسر `pkg/api/v1/ledger`. |
| قبول لقطة COA متعددة الشركات بشرط نطاق لاحق | يسمح بمرور أبوية عبر المستأجرين وينقل العبء إلى المحول (القاعدة 3.1/4). |

## التبعات والمسؤوليات

- **على النواة:** فحص النطاق والهوية ونافذة النفاذ قبل أي أثر مالي؛ لا SQL ولا أقفال ولا وقت حالي مباشر.
- **على المضيف/المحول:** يجب أن يحفظ نسخاً مؤرخة من الحساب واليومية والفترة، وأن يطبّق `EffectiveAccountReader` حتى لا تُعاد تفسير قيود سابقة عند تعديل البيانات الرئيسية. هذا الالتزام مسجّل كفجوة `G-019` بمالك مخزّن/Adapter ومرحلة لاحقة.
- **على السطح العام:** لا تغيير في التوقيعات؛ الإضافات فقط. لا يُضعف استثناء `pkg/api/v1` في ADR P7-01.

## الأدلة

`TestCompanyScopeFailsClosedWithoutRepositoryAccess`, `TestAccountHierarchyRejectsCrossCompanyLinks`, `TestAccountValidateRejectsReversedEffectiveRange`, `TestEffectiveDatingUsesInjectedClockAndVersionedReader`, `TestJournalReaderScopeIsValidated`, `TestFiscalCalendarBoundaryMatrix`, `TestCloseStateMatrixForPeriodAndYear`, `TestReferenceModelsDoNotDriftSilently`, `TestMasterDataAdversarialInputsFailClosed`, `FuzzEffectiveRangeValidationNeverPanics`.
