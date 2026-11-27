# مواصفة تدفق الترحيل والعكس المحاسبي — Posting Flow Specification

- **رمز الوثيقة:** `AE-POST-FLOW-V12`
- **المرحلة:** 6 — الترحيل والعكس المحاسبي
- **النطاق:** `internal/kernal/posting/**` والتفاعل الضروري مع `journal` و`masterdata` و`policy` و`types`
- **المرجع:** [خطة المعالجة، المرحلة 6](../plans/KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [ثوابت النواة](../../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)، [مصفوفة التتبع](../../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md)
- **القرار المعماري:** [ADR P1-02](../ard/P1_02_gl_source_of_truth.md) و[ADR P3-01](../ard/P3_01_masterdata_effective_dating.md) و[ADR P5-01](../ard/P5_01_policy_selection_and_historical_interpretation.md)
- **قاعدة حاكمة:** هذه مواصفة سلوكية للنواة فقط. لا تحتوي على SQL أو تخزين ملموس أو HTTP/gRPC أو RBAC أو أرصدة مادية أو تقارير.

---

## 1. الغرض

تثبت هذه المواصفة أن `PostingPipeline.PostEntry` هو المسار المركزي الوحيد الذي يحوّل مسودة `journal.JournalEntry` إلى قيد مرحّل داخل النواة، وأن أي فشل تحقق يمنع استدعاء `PostingExecutor` أو `PostingReversalExecutor`. كما تحدد كيفية بناء القيد العكسي دون تعديل الأصل، وكيف تُجمَّد المدخلات قبل التفويض.

القيد المرحّل وأسطره هو مصدر الحقيقة المحاسبي وفق ADR P1-02. المنفذ (Host Executor) مسؤول عن الذرية والحفظ ومنع تكرار الأثر فعلياً؛ النواة تملك القرار والتحقق والنسخة النهائية فقط.

---

## 2. حدود المسؤولية

| الطرف | يملك | لا يملك |
|---|---|---|
| Kernel `posting` | ترتيب التحقق، Clone، التقريب والتسوية، التوازن، اختيار السياسة، لقطة الحساب النافذة، البصمة، ختم الحالة، بناء العكس، وختم نطاق idempotency وبصماتها قبل التفويض | فتح معاملة قاعدة بيانات، قفل صف، حفظ idempotency، إعادة محاولة بعد unknown outcome، منح صلاحيات |
| Host Executor | الحفظ الذري للقيد والأسطر والربط ومفتاح منع التكرار وبصمته ونتيجته وأثر التدقيق، وقفل صف الأصل عند العكس | تغيير مبالغ القيد أو تصنيفه أو تواريخه بعد التحقق، إعادة إدخال مسودة مرحّلة إلى Pipeline |
| Adapters | توفير `EffectiveAccountReader` وReader للسياسات والفترات، وتثبيت النسخ المؤرخة | إرجاع نسخة "حالية" مختلفة عن نسخة تاريخ القيد بعد التحقق |

---

## 3. تدفق `PostEntry` بالترتيب التنفيذي

الترتيب أدناه مطابق لـ `internal/kernal/posting/pipeline.go`. أي خطأ في أي خطوة يُعاد فوراً قبل أي تفويض.

| # | الخطوة | الدالة / الفحص | القاعدة | نتيجة الفشل النموذجية | دليل اختبار |
|---:|---|---|---|---|---|
| 0 | فحص المت receiver والتبعيات | `p != nil`، `ctx != nil`، `reader`، `policyEngine`، `clock`، وجود executor مناسب حسب `ReversalOfID` | `RULE-KERNEL-010`, `RULE-KERNEL-027` | `ErrPostingPipelineRequired`, `ErrPostingContextRequired`, `ErrPostingReaderRequired`, `ErrPolicyEngineRequired`, `ErrPostingClockRequired`, `ErrPostingExecutorRequired`, `ErrReversalExecutorRequired` | `TestPostEntryRejectsUninitializedPipelineDependencies`, `TestPostingExecutorAndOptionBranches` |
| 1 | هوية القيد وأسطره | `ValidateEntryIdentity`: معرف غير صفري ومعرفات أسطر فريدة | `RULE-KERNEL-009` | `ErrMissingEntryID`, `ErrMissingLineID`, `ErrDuplicateLineID` | `TestEntryIdentityValidationRequiresUniqueLineIDs`, `TestPostingRejectsMissingEntryIdentityBeforeValidation` |
| 2 | بيانات التصنيف والربط العكسي | `ValidateEntryClassificationMetadata`: تصنيف معروف، بيانات adjusting، ربط `ReversalOfID`، سبب العكس، ورفض `ReversedByID` على مسودة | `RULE-KERNEL-009`, `RULE-KERNEL-011`, `RULE-KERNEL-012`, `RULE-KERNEL-015` | `ErrInvalidEntry`, `ErrAdjustingEntryMetadataRequired`, `ErrInvalidAdjustingEntryPeriod`, `ErrInvalidReversal`, `ErrReversalReasonRequired` | `TestAdjustingEntriesRequireMetadataAtValidation`, `TestReversalClassificationAndReferenceMustMatch`, `TestPostEntryRejectsDraftWithPreexistingReversalLink` |
| 3 | حالة القيد والمعرّفات الأساسية | `Status == EntryDraft`، `JournalID` غير صفري، `ActorID` و`IdempotencyKey` غير فارغين بعد Trim | `RULE-KERNEL-009`, `RULE-KERNEL-010` | `ErrInvalidEntryState`, `ErrMissingJournalID`, `ErrPosterIDRequired`, `ErrIdempotencyKeyRequired` | `TestPostingPipelineEndToEnd`, `TestPostEntryIdentityAndPolicyRejectMatrix` |
| 4 | منع تعديل مرحّل وتاريخ صفر | `EnsureImmutable`، `EffectiveDate.IsZero` | `RULE-KERNEL-009`, `RULE-KERNEL-011` | `ErrCannotModifyPostedMove`, `ErrZeroEntryDate` | `TestValidateImmutabilityRejectsPostedChanges`, `TestAdjustingEntriesAndImmutabilityCoverage` |
| 5 | نسخ دفاعي قبل أي تعديل داخلي | `entry = entry.Clone()` ثم تطبيع الاسم القديم `AdjustingEntryAuditLegacy` | `RULE-KERNEL-009`, `RULE-KERNEL-011` | لا يوجد؛ إجراء وقائي | `TestCloneIsDeepAndCoversPointerFields`, `TestPostEntryExecutesFrozenValidatedSnapshot` |
| 6 | نطاق الشركة والعملة الوظيفية | `ValidateCompanyScope`، وجود `FunctionalCurrency`، `LookupCurrency` ورفض `XXX` | `RULE-KERNEL-016`, `RULE-KERNEL-004` | `ErrCompanyScopeRequired`, `ErrMissingFunctionalCurrency`, `types.ErrInvalidCurrency` | `TestPostingRejectsMissingCompanyFunctionalCurrency`, `TestPostEntryCompanyScopeAdversarialBranches` |
| 7 | سياسات التاريخ | `PostingPolicyFor` و`CurrencyPolicyFor` بـ `(CompanyID, EffectiveDate)`، ثم مطابقة العملة الوظيفية | `RULE-KERNEL-010`, `RULE-KERNEL-019` | `ErrPolicyNotFound`, `ErrPolicyOverlap`, `ErrFunctionalCurrencyMismatch` | `TestPolicySelectionIsScopedAndFailsClosed`, `TestPostEntryFailsClosedWithoutRegisteredPolicies` |
| 8 | عملات الأسطر | لكل سطر: تطابق `FunctionalDebit/Credit` مع العملة الوظيفية، تطابق `TransactionAmount` مع `TransactionCurrency`، وسماح سياسة العملة | `RULE-KERNEL-001`, `RULE-KERNEL-002`, `RULE-KERNEL-004` | `ErrFunctionalCurrencyMismatch`, `ErrTransactionCurrencyMismatch`, `policy.ErrCurrencyNotAllowed` | `TestPostEntryCompanyCurrencyAndTransactionRejectMatrix`, `TestPostEntryExecutorNeverRunsForRejectedInputs` |
| 9 | اليومية | capability check إلى `JournalRepository`، `GetJournalByID`، مطابقة الهوية والشركة، `Validate`، `IsCurrent(EffectiveDate)` | `RULE-KERNEL-010`, `RULE-KERNEL-016` | `masterdata.ErrJournalReaderRequired`, `masterdata.ErrInvalidJournal`, `masterdata.ErrJournalNotCurrent` | `TestPostingRequiresJournalRepository`, `TestPostingEnforcesJournalCompanyAndAllowLists` |
| 10 | الفترة والتصنيف 13/14 | قصر 13 على adjusting/closing، قصر 14 على `audit_adjustment`، تحقق الفترة العادية أو التعديلية مع `AllowSoftClosedPosting`، قواعد `EntryClassOpening`، ثم ختم `FiscalPeriodID` على النسخة | `RULE-KERNEL-013`, `RULE-KERNEL-014`, `RULE-KERNEL-015`, `RULE-KERNEL-019`, `RULE-KERNEL-020` | `masterdata.ErrInvalidAdjustmentPeriod`, `masterdata.ErrClosedPeriod`, `masterdata.ErrPeriodNotFound` | `TestPostingOpeningAndAuditAdjustmentPeriodBranches`, `TestPostingResolvesAdjustmentPeriodByAccountingMetadata`, `TestReversalDraftResolvesDestinationPeriodDuringPosting` |
| 11 | التقريب والتسوية والتوازن | `RoundEntryWithRoundingPolicy` باستخدام `RoundingDifferenceAccountID` فقط عند `AutoSettleRoundingDifferences`، ثم `ValidateEntryDoubleEntry` | `RULE-KERNEL-001`, `RULE-KERNEL-005`, `RULE-KERNEL-006`, `RULE-KERNEL-007` | `ErrRoundingAccountRequired`, `ErrRoundingPolicyRequired`, `ErrUnbalancedEntry`, `types.ErrInvalidAmount` | `TestPostEntryNormalizesBothCurrenciesAndSettlesRounding`, `TestPostEntryRejectsRoundingWithoutPolicySettlement`, `TestRoundEntryRoundsSubMinorDifferenceBeforeSettling` |
| 12 | قوائم اليومية السماحية | لكل سطر: `AllowsCurrency` و`AllowsAccount` | `RULE-KERNEL-010`, `RULE-KERNEL-016` | `masterdata.ErrJournalCurrency`, `masterdata.ErrJournalAccount` | `TestPostingAcceptsEntryWithinJournalAllowLists`, `TestJournalAllowListRejectsDuplicatesAndEmptyListIsUnrestricted` |
| 13 | الحساب والأبعاد والأهلية | `ResolvePostableAccountAsOf` مرة واحدة لكل سطر، `validateFinancialDimensions`، ثم `policy.CheckAccountEligibility` على اللقطة نفسها | `RULE-KERNEL-016`, `RULE-KERNEL-017` | `ErrAccountIdentityMismatch`, `ErrHeaderAccountPosting`, `masterdata.ErrAccountNotCurrent`, `policy.ErrControlAccountNotPostable`, `ErrDimensionReaderRequired`, `ErrInactiveFinancialDimension` | `TestPostEntryUsesEffectiveAccountSnapshotForEligibility`, `TestAccountEligibilityDecisionMatrix`, `TestPostingRejectsInactiveCostCentersAndDimensions` |
| 14 | سياسة الترحيل النهائية | `ValidatePostingPolicy` باستخدام أنواع الحسابات من اللقطة نفسها و`clock.Now()` | `RULE-KERNEL-010`, `RULE-KERNEL-019` | أي خطأ من السياسة المختارة | `TestPostingPolicyAndClockAdversarialBranches`, `TestSelectedPostingPolicyControlsSoftCloseOnly` |
| 15 | ختم Audit Trace وعقد idempotency | `policy.Fingerprint`، `PostingDate`، `PostedBy`، `PostedAt`، `Status = EntryPosted`، ثم `BuildIdempotencyScope` و`FingerprintRequest` و`FingerprintCallerPayload` فوق النسخة المجمدة | `RULE-KERNEL-010`, `RULE-KERNEL-025`, `RULE-KERNEL-027` | لا يوجد؛ يحدث بعد اكتمال كل التحقق | `TestPostEntryStampsPolicyFingerprintAndKeepsHistoryStable`, `TestPostEntryUsesOneInjectedClockForPolicyAndPostingMetadata`, `TestPostEntryStampsIdempotencyContractOnExecutorOptions` |
| 16 | التفويض فقط | `executeReversal(ctx, *ReversalOfID, entry, opts)` أو `execute(ctx, entry, opts)`، مع `Scope`/`RequestFingerprint`/`CallerFingerprint` الحاكمة في `opts` | `RULE-KERNEL-027` | يُعاد خطأ Executor كما هو؛ لا توجد كتابة إضافية داخل النواة | `TestPostEntryStopsWhenAtomicExecutorFails`, `TestPostEntryHonorsCancelledContextInExecutor`, `TestPostEntryExecutorNeverRunsForRejectedInputs` |

> لا يوجد مسار بديل يتخطى الخطوات 1–15. المنفذ لا يُستدعى من أي فرع `return` يسبق الخطوة 16.

---

## 4. قواعد قبل النسخ وبعده

1. **قبل Clone:** فحوص المتغيرات المحلية والهوية والتصنيف والحالة والمعرّفات وبيانات الاعتماد. هذه لا تحتاج إلى تعديل القيد.
2. **بعد Clone:** كل فحص يقرأ Reader، وكل تعديل على `FiscalPeriodID` أو المبالغ أو التصنيف القديم، وكل ختم نهائي. بهذا لا يمكن لعملية adversarial تعديل مدخلات المتصل أثناء التحقق أن تغيّر ما يصل إلى Executor.
3. **لقطة الحساب:** `ResolvePostableAccountAsOf` تُرجع نفس `masterdata.Account` التي تحققت منها النواة، وتُستخدم مباشرة في `CheckAccountEligibility`. لا توجد قراءة ثانية لـ `GetAccount` في المسار المرحّل.
4. **الوقت:** الوصول إلى الساعة الحقيقية في شيفرة `posting` الإنتاجية يتم فقط عبر `systemClock{}.Now()` داخل `pipeline.go`؛ بقية الدوال تستخدم `Clock` أو قيمة `now` صريحة عبر `BuildReversalEntryAt` و`NewJournalEntryAt`.

---

## 5. بناء القيد العكسي

`BuildReversalEntryAt(original, reversalDate, actorID, reason, now)`:

1. يرفض الأصل غير المرحّل (`ErrEntryNotPosted`) أو المعكوس مسبقاً (`ErrEntryAlreadyReversed`).
2. يتحقق من Actor وسبب غير فارغين.
3. ينشئ نسخة دفاعية من الأصل قبل أي استخدام.
4. يوحّد `reversalDate` إلى UTC؛ إذا كانت صفرية يستخدم `now`؛ ويمنع تاريخاً سابقاً لتاريخ الأصل بمقارنة يومية UTC (`ErrReversalDateBeforeOriginal`).
5. ينشئ قيداً جديداً بمعرّف جديد، `Status = Draft`، `Classification = Reversing`، `ReversalOfID` يشير إلى الأصل، و`ReversalReason` محفوظاً.
6. لا ينسخ `FiscalPeriodID` من الأصل؛ الفترة تُحل لاحقاً من تاريخ العكس عبر `PostEntry`.
7. ينسخ كل سطر مع الحفاظ على `AccountID`, `PartnerID`, `CostCenterID`, `ProjectID`, `SegmentID`, `BusinessUnitID`, `MatchingNumber`, عملة المعاملة وسعر الصرف، ويبدّل `FunctionalDebit` مع `FunctionalCredit`.
8. يتحقق من توازن العكس بـ `ValidateEntryDoubleEntry` قبل إرجاعه.

`BuildReversalEntry` يستدعي `BuildReversalEntryAt` مع `systemClock{}.Now()` فقط؛ لا يوجد `time.Now()` مباشر في المنطق.

الدليل: `TestReversalLifecycle`, `TestReversalBuilderLeavesOriginalImmutable`, `TestReversalClassificationAndReferenceMustMatch`, `TestReversalDraftResolvesDestinationPeriodDuringPosting`, `FuzzReversalNetImpactIsZeroPerAccount`.

---

## 6. نموذج TOCTOU وحمايته

| التغيير العدائي بين التحقق والتنفيذ | الحماية داخل النواة | ما يبقى على المضيف |
|---|---|---|
| تغيير `CompanyID` أو `AccountID` أو العملة أو رقم الفترة في كائن المتصل | `entry.Clone()` قبل كل قراءة Reader؛ Executor يستقبل النسخة المجمدة | لا شيء إضافي لهذا النوع |
| تغيير سجل الحساب في المحول بعد التحقق | قراءة واحدة نافذة زمنياً عبر `ResolvePostableAccountAsOf` واستخدام اللقطة نفسها للأهلية | توفير `EffectiveAccountReader` وتخزين نسخ مؤرخة (`G-019`) |
| تغيير السياسة بعد التحقق | انتقاء حتمي بتاريخ القيد + `PolicyFingerprint` محفوظ في القيد | حفظ سلاسل إصدارات السياسات ذرياً (`G-020`) |
| تعديل مخزن الحقائق بعد التفويض أو عكس القيد مرتين | خارج قدرة النواة؛ النواة تختم النطاق والبصمة قبل التفويض وتجمّد اللقطة | قفل صف/معاملة/first-writer-wins وفق [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md) و[عقد القفل](./KERNEL_LOCKING_CONTRACT.md)؛ `G-001` و`G-014` أُغلقتا في المرحلة 7، و`G-002` في المرحلة 8 |

الدليل التنفيذي: `TestPostEntryExecutesFrozenValidatedSnapshot`, `TestPostEntryUsesEffectiveAccountSnapshotForEligibility`, `TestPostEntryStampsPolicyFingerprintAndKeepsHistoryStable`, `TestPostEntryExecutorNeverRunsForRejectedInputs`.

---

## 7. فشل Reader أو Policy Executor

- أي خطأ من `GetCompany` أو `GetAccountAsOf` أو `GetJournalByID` أو Readers الأبعاد أو الفترات أو السياسات يُعاد دون استدعاء Executor.
- أي خطأ من `PostingExecutor` أو `PostingReversalExecutor` يُعاد كما هو؛ النواة لا تكتب Compensation ولا تحاول Retry.
- لا توجد أخطاء متجاهلة (`_ = err`) في مسارات التحقق أو الختم أو التفويض.

الدليل: `TestPostingAdversarialRepositoryFailures`, `TestPostEntryStopsWhenAtomicExecutorFails`, `TestPostEntryFailsClosedWithoutRegisteredPolicies`.

---

## 8. مطابقة مهام المرحلة 6

| مهمة الخطة | مكان الدليل |
|---:|---|
| 1–3 | الجدول في القسم 3 + `TestPostEntryExecutorNeverRunsForRejectedInputs` |
| 4 | القسم 4 + `TestPostEntryExecutesFrozenValidatedSnapshot` |
| 5–8 | خطوات 8، 10، 11 + اختبارات التقريب والفترات والسياسات |
| 9–11 | القسم 5 + اختبارات العكس و`FuzzReversalNetImpactIsZeroPerAccount` |
| 12 | القسم 6 + اختبارات TOCTOU |
| 13 | القسم 7 + اختبارات الفشل العدائية |
| 14–15 | `internal/kernal/doc_test.go` (`TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests`) + ملحق المرحلة 6 في مصفوفة التتبع |

---

## 9. الحكم

تنجح بوابة المرحلة 6 إذا تحققت كل الشروط التالية آلياً:

- `gofmt -l internal/kernal/` بلا إخراج.
- `go test ./internal/kernal/... -count=1` و`go test -race ./internal/kernal/... -count=1` PASS.
- `./scripts/verify_architecture.sh` PASS without lowering the 11 rules.
- مصفوفة التتبع تغطي 27 قاعدة وكل مرجع `Test`/`Fuzz` موجود فعلاً.
- لا استدعاء Executor في أي اختبار رفض.

لا تُعتبر هذه البوابة وحدها إثباتاً للذرية أو idempotency أو قفل التخزين؛ تلك انتقلت إلى بوابة المرحلة 7 الموثقة أدناه. كما لا تُثبت توفر قارئي الأبعاد في Adapter فعلي (`G-022`)؛ أما طوابع `PostingDate`/`PostedAt` فقد أثبت المحولان حفظها في المرحلة 7 (`G-023`).

---

## 10. ملحق المرحلة 7 — من قرار النواة إلى وحدة العمل الذرية

تحتفظ المرحلة 6 بحدودها، وتضيف المرحلة 7 العقد التنفيذي التالي دون إدخال SQL إلى النواة:

| الضابط | ما يحدث بعد الخطوة 16 |
|---|---|
| النطاق الحاكم | `IdempotencyScope = CompanyID + Operation + Key`؛ قيم المتصل تُكتب فوقها |
| البصمتان | `RequestFingerprint` للقيد المجمد، و`CallerFingerprint` لحمولة المتصل المستقرة والمحفوظة كـ `PayloadHash` |
| وحدة العمل | القيد والأسطر + الأثر المالي + سجل idempotency بنتيجته + أثر التدقيق + رابط/حالة الأصل عند العكس |
| الفشل قبل Commit | Rollback كامل؛ لا قيد، لا مفتاح، لا رصيد، لا تدقيق |
| الفشل بعد Commit | الالتزام دائم؛ الاستعلام بالمفتاح يُعيد النتيجة؛ إعادة الطلب لا تضيف أثراً |
| العكس | قفل الأصل، فائز واحد، ربط ذري، ورفض صريح للمحاولة الثانية |

الأدلة: [عقد المعاملة](./KERNEL_POSTING_TRANSACTION_CONTRACT.md)، [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md)، [عقد ذرية العكس](./KERNEL_REVERSAL_ATOMICITY_CONTRACT.md)، [مجموعة اختبارات العقد](./KERNEL_CONTRACT_TEST_SUITE.md)، [تقرير حقن الأعطال](../reports/fault_injection_report.md)، و[تقرير المرحلة 7](../reports/KERNEL_FINAL_VERIFICATION_REPORT.md). تصنيف `RULE-KERNEL-027` في مصفوفة النواة يبقى `Host Responsibility` لأن الذرية نفسها لا تزال في المحول.
