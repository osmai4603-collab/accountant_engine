# مصفوفة قواعد التقييم — FX & Revaluation

- **رمز الوثيقة:** `AE-REMED-P9-MATRIX`
- **المرحلة:** 9 — التقييم والعملات الأجنبية
- **النطاق:** `internal/kernal/valuation`، والتكامل مع `types` و`journal` و`posting`
- **القرار المرجعي:** [ADR P9-01](./ard/P9_01_valuation_idempotency_and_rate_selection.md)
- **تقرير المرحلة:** [KERNEL_REMEDIATION_PHASE9_REPORT.md](./KERNEL_REMEDIATION_PHASE9_REPORT.md)

| البند | القاعدة | التنفيذ | دليل الاختبار | الحالة |
|---|---|---|---|---|
| أنواع السعر | `SPOT`, `CLOSING`, `AVERAGE`, `HISTORICAL`؛ إعادة التقييم تطلب `CLOSING` | `exchange_rate.go`، `revaluation.go` (`validateRevaluationRate`) | `TestIAS21ExchangeRateAndRevaluation`, `TestRevaluationValidatesRateReferenceAgainstRequest` | Implemented |
| صلاحية السعر | رقم موجب، عملتان صالحتان، Effective Date ونوع صالحان، ومرجع مطابق لعملية التحويل بما في حالة الانعكاس | `ExchangeRate.Validate`, `validateRevaluationRate` | `TestRateRegistryRejectsMissingAndDuplicateEffectiveDates`, `TestRevaluationRejectsNonPositiveProviderRate`, `TestRevaluationValidatesInvertedAndSameCurrencyRates` | Implemented |
| تعارض السعر | لا يقبل سعران لنفس الزوج والنوع واليوم؛ ولا يسمح بتسجيلة زوج وعكس لنفس اليوم | `RateRegistry.RegisterRate` | `TestRateRegistryRejectsMissingAndDuplicateEffectiveDates` (subtest للزوج `USD→SAR` ثم `SAR→USD` في اليوم نفسه ينتظر `ErrDuplicateRate`) | Implemented |
| Effective Date واختيار السعر | عند تعدد الأسعار يُختار أحدث سعر لا يتأخر عن تاريخ التقييم؛ لا يُقبل سعر مستقبلي | `findEffectiveRate`, `RateRegistry.GetRate`, `Calculate` | `TestRateRegistrySelectsLatestEffectiveRateAndRejectsFuture` (3.70 بين التاريخين، 3.75 في يومه، رفض ما قبل أول سعر)، `FuzzRateRegistryEffectiveSelectionNeverPanics` | Implemented |
| تاريخ مرجعي مستقل | النتيجة تحفظ `RateDate` منفصلًا عن `date` قيد الترحيل | `FXResult.RateDate`، `GenerateRevaluationEntryAt` | `TestIAS21ExchangeRateAndRevaluation` | Implemented |
| نفس العملة | السعر يجب أن يساوي `1` بالضبط، مع رفض `XXX` والتاريخ الصفري | `GetRate`, `Convert`, `Calculate` | `TestSameCurrencyConversionKeepsUnitRateExact`, `TestSameCurrencyRateValidatesCodeAndDateBeforeReturningOne` | Implemented |
| فروق الأصول والخصوم | الأصل: الزيادة ربح والانخفاض خسارة؛ الالتزام: الزيادة خسارة والنقص ربح؛ اتجاه المدين/الدائن ينعكس تبعاً لذلك | `Calculate`, `GenerateRevaluationEntryAt` | `TestLiabilityRevaluationDirectionIsOppositeToAsset` (أربع حالات، توازن 500/500 في كل حالة)، `TestIAS21ExchangeRateAndRevaluation` | Implemented |
| الأرباح والخسائر غير المحققة | القيد تسوي (`adjusting/accrual`) ويستخدم حسابي `UnrealizedGainID`/`UnrealizedLossID`؛ الأرباح المحققة عند التسوية ليست ضمن ما تنفذه النواة | `GenerateRevaluationEntryAt` | `TestRevaluationOnlyBooksUnrealizedFXDifference` | Implemented ضمن حدود النواة |
| قيد متوازن | debit وcredit متساويان ولا يقبل فرقًا صفريًا | `GenerateRevaluationEntryAt`, `posting.ValidateEntryDoubleEntry` | `TestIAS21ExchangeRateAndRevaluation`, `TestValuationCoverageBranches` | Implemented |
| منع التكرار | مفتاح عملية الشركة والحساب وعملة **المعاملة** واليوم، مرجع قيد حتمي، ونطاق `posting.IdempotencyRevaluation`؛ نفس الحمولة replay، وحمولة مختلفة تُرفض بـ `ErrRevaluationAlreadyApplied` غير القابل لإعادة المحاولة | `RevaluationOperationKey`, `RevaluationReferenceID`, `RevaluationIdempotencyScope`, `RevaluationRegistry.Register`، بصمة canonical v2 بحقول مسبوقة بالطول | `TestRevaluationReferenceIsDeterministicAndRegisteredOnce`, `TestRevaluationIdempotencyScopeIsDeterministicAndOperationScoped`, `TestRevaluationRegistryRejectsConflictingPayloadForSameKey` (أربع طفرات: مبلغ، بُعد، وقت يوم، بصمة سياسة)، `TestRevaluationRegistryConcurrentRegisterHasSingleResult` (16 منافساً، فائز واحد)، `TestTransactionErrorClassification` | Implemented داخل النواة؛ النطاق غير موصول بـ `PostEntry`/SPI والتخزين الدائم للمضيف (`G-024`) |
| النسخ الدفاعي | registry لا يعيد مرجعاً قابلاً للتعديل إلى نتيجته الداخلية | `RevaluationRegistry.Register`, `Lookup` | `TestRevaluationRegistryRejectsConflictingPayloadForSameKey` | Implemented |
| العكس | يستخدم عكس قيد النواة، يحافظ على تاريخ العكس ورابط الأصل، ويترك الفترة لتُحل في `PostEntry` من تاريخ العكس | `BuildRevaluationReversalAt` → `posting.BuildReversalEntryAt` | `TestRevaluationReverseKeepsReversalDateAndPeriod`, `TestRevaluationReversalDraftCarriesDateForPeriodResolution`, `TestReversalDraftResolvesDestinationPeriodDuringPosting` (إثبات حل الفترة عبر `PostEntry`) | Implemented |
| الحالات الحدية | Property/Fuzz بلا panic ولا float؛ اختيار السعر ونوعه وتاريخه | `FuzzFXConversionNeverPanics`, `FuzzRateRegistryEffectiveSelectionNeverPanics` | الأمر مع `-fuzztime=5s` لكل هدف → `PASS` في 2026-11-22؛ عدد التنفيذات يتغير مع الـ cache والبذور ولا يُعدّ معيار قبول (`G-011`) | PASS |

## حدود النطاق

- لا مزود أسعار خارجي ولا اتصال شبكي داخل النواة؛ `RateProvider` يُحقن من المضيف.
- `RevaluationRegistry` ذاكري ويحمي عملية واحدة فقط؛ `RevaluationIdempotencyScope` يُنتج كائن نطاق دون توصيله بـ `PostEntry` أو SPI. الضمان الدائم ضد التكرار بعد إعادة التشغيل أو انقطاع الاتصال مشروط بتوسيع العقد وتخزين نطاق `revaluation` ذرياً في المضيف (`G-024`).
- النواة لا تحسب أرباحاً أو خسائر محققة عند تسوية المركز؛ هذا قرار ترحيل للمضيف خارج هذه المرحلة.
