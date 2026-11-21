# مصفوفة التتبع المحاسبي — Kernel Traceability Matrix v10

- **رمز الوثيقة:** `AE-TRACE-KERNEL-V10`
- **الإصدار المستهدف:** Kernel v10
- **النطاق الملزم:** `internal/kernal/` حصراً
- **الحالات المسموحة:** `Implemented`, `Partially Implemented`, `Missing`, `Host Responsibility`, `Out of Kernel`

---

## 1. مصفوفة تتبع القواعد الـ 27 الحاكمة للنواة (RULE-KERNEL-001 .. 027)

| Rule ID | نص المتطلب المحاسبي | الحزمة والملف | الدالة المسؤولة | اختبار القبول | اختبار الرفض | الحالات الحدية | اعتماد المضيف | الحالة الفعلية |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `RULE-KERNEL-001` | توازن القيد المزدوج في العملة الوظيفية بعد التقريب والتسوية الصريحة | `posting/entry_validation.go`, `posting/rounding.go` | `ValidateBalanced`, `RoundEntryToMinorUnits` | `TestValidateBalanced_Success` | `TestValidateBalanced_UnbalancedFails` | تسوية الفروق الصغرى على حساب التسوية | لا يوجد | Implemented |
| `RULE-KERNEL-002` | حد أدنى سطرين وطرفين متقابلين، ووحدة العملة في العملية | `posting/entry_validation.go` | `ValidateBalanced`, `validateBasicEntry` | `TestEntryValidation_TwoSides` | `TestEntryValidation_OneSidedEntry` | سطر صفري أحادي الطرف | لا يوجد | Implemented |
| `RULE-KERNEL-003` | حظر المبالغ السالبة والصفرية حيث يُشترط مبلغ موجب | `posting/entry_validation.go` | `ValidateBalanced` | `TestValidateBalanced_PositiveAmounts` | `TestValidateBalanced_RejectsNegativeAmount` | أسطر صفرية تكميلية | لا يوجد | Implemented |
| `RULE-KERNEL-004` | حظر عملة `XXX` والعملات المجهولة | `types/currency.go` | `IsValidCurrency` | `TestCurrencyValidation` | `TestCurrencyValidation_RejectsXXX` | عملات قديمة أو غير قياسية | لا يوجد | Implemented |
| `RULE-KERNEL-005` | سقف 28 رقماً ذا دلالة، وحظر الأسس المتطرفة | `types/money.go` | `NewMoneyFromString`, `NewMoney` | `TestMoney_SignificantDigits` | `TestInvalidAmounts` | `1e-100000`, `1e999999` | لا يوجد | Implemented |
| `RULE-KERNEL-006` | مقياس داخلي 12 وتطبيع المبالغ للكسور الصغرى للعملة | `types/money.go`, `posting/rounding.go` | `InternalScale`, `RoundForPosting` | `TestMoney_InternalScale` | `TestMoney_InvalidFraction` | عملات بدون كسور (JPY) وثلاث منازل (KWD) | لا يوجد | Implemented |
| `RULE-KERNEL-007` | `MinorUnitsExact` لا يُسقط كسوراً صامتة | `types/money.go` | `MinorUnitsExact` | `TestMoney_MinorUnitsExact` | `TestMoney_MinorUnitsExact_ErrorOnFraction` | كسر أصغر من الفلس | لا يوجد | Implemented |
| `RULE-KERNEL-008` | تطابق أسعار الصرف، وسعر العملة لنفسها = 1 تماماً | `valuation/exchange_rate.go`, `valuation/conversion.go` | `GetRate`, `Convert`, `ConvertForDate` | `TestExchangeRate_SameCurrencyIsOne` | `TestExchangeRate_MismatchRejection` | تقارب زمني وتحويل عكسي | لا يوجد | Implemented |
| `RULE-KERNEL-009` | دخول Pipeline لمسودات `Draft` فقط، وفرض تصنيف معتمد | `posting/entry_validation.go`, `posting/pipeline.go` | `validateBasicEntry`, `PostEntry` | `TestPipeline_AcceptsDraft` | `TestPipeline_RejectsNonDraft`, `TestPipeline_RejectsEmptyClassification` | تصنيف غير معروف | لا يوجد | Implemented |
| `RULE-KERNEL-010` | اكتمال بيانات القيد، وفرض `IdempotencyKey` غير فارغ | `posting/pipeline.go` | `PostEntry` | `TestPostingPipelineEndToEnd` | `TestPostingPipeline_RejectsEmptyIdempotencyKey` | مفتاح يحوي مسافات فقط | فحص التكرار والحفظ الذري في قاعدة البيانات | Implemented |
| `RULE-KERNEL-011` | منع إعادة ترحيل المرحل، وحصر العكس على القيود المرحّلة | `posting/reversal.go`, `posting/pipeline.go` | `BuildReversalEntry`, `PostEntry` | `TestReversal_PostedEntry` | `TestReversal_RejectsDraftOrAlreadyReversed` | قيد معكوس مسبقاً | منع التزامن أثناء القيد | Implemented |
| `RULE-KERNEL-012` | العكس مرة واحدة، وسلامة معرفات العكس والسبب الإلزامي | `posting/reversal.go` | `BuildReversalEntry` | `TestReversal_ValidMetadata` | `TestReversal_RejectsMissingReason` | سبب فارغ أو مسافات | قفل صف القيد الأصلي ذرياً | Implemented |
| `RULE-KERNEL-013` | ثبات الأبعاد والعملات وتبديل المدين والدائن في القيد العكسي | `posting/reversal.go` | `BuildReversalEntry` | `TestReversal_SwapsDebitCreditPreservesDimensions` | `TestReversal_DimensionTamper` | أبعاد متعددة ومراكز تكلفة | لا يوجد | Implemented |
| `RULE-KERNEL-014` | تاريخ العكس لا يسبق الأصل، وتحديد فترة العكس بتاريخه | `posting/reversal.go` | `BuildReversalEntry` | `TestReversal_DateValidation` | `TestReversal_RejectsPrecedingDate` | تاريخ العكس في فترة مغلقة | لا يوجد | Implemented |
| `RULE-KERNEL-015` | قصر فترة 13 على تسويات الإقفال و14 على التدقيق (`audit_adjustment`) | `posting/entry_validation.go`, `posting/adjusting.go` | `ValidateEntryClassificationMetadata`, `ValidateAdjustingEntry` | `TestAdjusting_Period13and14` | `TestAdjusting_RejectsAuditOutsidePeriod14` | توافق قراءة "audit" وتطبيعها لكتابة "audit_adjustment" | لا يوجد | Implemented |
| `RULE-KERNEL-016` | انتماء الحسابات واليوميات والأبعاد لنفس شركة القيد | `masterdata/account.go`, `policy/eligibility.go` | `ValidateAccountCompany`, `ValidateDimensionCompany` | `TestMasterData_CompanyOwnership` | `TestMasterData_RejectsCrossCompany` | أبعاد مشتركة | التحقق من نطاق المستأجر (Multi-tenancy RLS) | Implemented |
| `RULE-KERNEL-017` | حظر الترحيل على حسابات الحاوية (Header) أو المغلقة/الملغاة | `policy/eligibility.go` | `ValidateAccountEligibility` | `TestAccountEligibility_ActivePostingAccount` | `TestAccountEligibility_RejectsHeaderOrInactive` | حساب منتهي الصلاحية بتاريخ القيد | لا يوجد | Implemented |
| `RULE-KERNEL-018` | اكتمال وترتيب وعدم تداخل فترات السنة المالية | `masterdata/fiscal_calendar.go` | `GenerateFiscalYear`, `ValidateFiscalYear` | `TestFiscalCalendar_Contiguity` | `TestValidateFiscalYear_RejectsInvalidStructures` | سنة كبيسة وسنة تبدأ في 15 من الشهر | لا يوجد | Implemented |
| `RULE-KERNEL-019` | إنفاذ حالات إقفال الفترات (سماح SoftClosed بالسياسة فقط، وحظر HardClosed) | `policy/posting_rules.go` | `ValidatePostingPolicy` | `TestPolicy_SoftClosedPostingAllowed` | `TestPolicy_HardClosedRejected` | إقفال أثناء محاولة الترحيل | لا يوجد | Implemented |
| `RULE-KERNEL-020` | الحفاظ على يوم بداية السنة المالية وتوليد فترات متصلة دون فجوات | `masterdata/fiscal_calendar.go` | `addFiscalMonths`, `GenerateFiscalYear` | `TestGenerateFiscalYearPreservesCustomStartDay` | `FuzzGenerateFiscalYearMaintainsContinuityAndContainment` | بداية في 31 يناير وبداية في 29 فبراير | لا يوجد | Implemented |
| `RULE-KERNEL-021` | حصر إعادة التقييم بالبنود النقدية وبأسعار الصرف الصالحة | `valuation/revaluation.go` | `GenerateRevaluationEntry` | `TestRevaluation_MonetaryOnly` | `TestRevaluation_RejectsNonMonetaryClosingRate` | بنود ذات طبيعة هجينة | تصنيف الحسابات في الدليل | Implemented |
| `RULE-KERNEL-022` | حظر أسعار الصرف المستقبلية بالنسبة لتاريخ التقييم | `valuation/revaluation.go`, `valuation/exchange_rate.go` | `GetRateAsOf` | `TestExchangeRate_AsOfDate` | `TestExchangeRate_RejectsFutureRate` | سعر لنفس اليوم وسعر سابق نافذ | توفير مزود أسعار معتمد | Implemented |
| `RULE-KERNEL-023` | فروق التقييم للأصول والخصوم تنتج الأثر الصحيح (مدين/دائن) | `valuation/revaluation.go` | `GenerateRevaluationEntry` | `TestRevaluation_GainLossDirection` | `TestRevaluation_InvertedDirectionFails` | مراكز دائنة في حسابات الأصول | لا يوجد | Implemented |
| `RULE-KERNEL-024` | فروق إعادة التقييم غير صفرية ومتوازنة في العملة الوظيفية | `valuation/revaluation.go` | `GenerateRevaluationEntry` | `TestRevaluation_BalancedEntry` | `TestRevaluation_RejectsZeroDifference` | فرق أقل من MinorUnit | لا يوجد | Implemented |
| `RULE-KERNEL-025` | تسلسل سجلات التدقيق وارتباط الهاش بكل الحقول الدلالية | `governance/audit_chain.go` | `Append`, `AppendAt`, `CalculateHash` | `TestCryptographicHashChaining` | `TestAuditSnapshotVerificationDetectsTampering` | حمولة فارغة أو أحرف خاصة | الحفظ المتسلسل الذري (Atomic Append) | Implemented |
| `RULE-KERNEL-026` | التحقق من التدقيق عبر لقطة متسقة ونقطة تفتيش مستقلة | `governance/verifier.go`, `governance/audit_chain.go` | `VerifyAuditIntegrity`, `GetSnapshot` | `TestAuditSnapshot_ConsistentVerification` | `TestAuditSnapshotRejectsCheckpointHashMismatch` | تعارض تزامن أثناء أخذ اللقطة | حفظ Checkpoint خارج قاعدة البيانات | Implemented |
| `RULE-KERNEL-027` | مسؤولية الذرية والحفظ والقفل على عاتق منفذ المضيف (Host Executor) | `posting/pipeline.go`, `posting/reversal.go` | `PostingExecutor`, `PostingReversalExecutor` | `TestPipeline_CallsExecutorOnSuccess` | `TestPostEntryStopsWhenAtomicExecutorFails` | فشل قاعدة البيانات أو الشبكة | تنفيذ ACID و RLS والتراجع عند الخطأ | Host Responsibility |

---

## 2. قواعد خارج النطاق المباشر للنواة (ACC-028 .. 040)

- `ACC-028 .. 040`: تخص المدارات الفرعية (Subledgers, Inventory, Fixed Assets, IFRS 15/16/9, Reporting). حالتها: `Out of Kernel`.
- `ACC-041 .. 043`: تخص سجل التدقيق والنزاهة وحفظ الأدلة؛ مرتبطة بحزمة `governance` كما هو مفصّل في القواعد 025 و026 و027 أعلاه.
