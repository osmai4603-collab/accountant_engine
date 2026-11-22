# تقرير المرحلة 5 — السياسات والأهلية المحاسبية

- **رمز الوثيقة:** `AE-REMED-P5-REPORT`
- **المرحلة:** 5 — السياسات والأهلية المحاسبية
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md) (قسم المرحلة 5، المهام 1–13)
- **سجل الفجوات:** [`KERNEL_REMEDIATION_GAP_REGISTER.md`](./KERNEL_REMEDIATION_GAP_REGISTER.md)
- **قرار التصميم:** [ADR P5-01](./ard/P5_01_policy_selection_and_historical_interpretation.md)
- **النتيجة:** `PHASE 5 GATE: PASS`

## 1. الهدف والنطاق

عالجت المرحلة 5 المشكلة المحددة في الخطة: وجود سياسات منفصلة لا يكفي إذا كان اختيارها غير حتمي، أو إذا أمكن تداخل سياستين نافذتين، أو إذا تغيّر تفسير قيد تاريخي بعد تعديل السياسة.

النطاق المنفّذ:

- `internal/kernal/policy/**` (الجرد، النوافذ، الانتقاء، البصمة، الأهلية).
- موصلات السياسة في `posting` و`masterdata` بالقدر المطلوب.
- لا موافقات أو صلاحيات مستخدمين، ولا تخزين ملموس، ولا قواعد محاسبية جديدة.

## 2. تغطية المهام 1–13

| # | المهمة من الخطة | التنفيذ والدليل | الحالة |
| ---: | --- | --- | --- |
| 1 | جرد أنواع السياسات وحقول النطاق وتاريخ النفاذ | `policy/posting_rules.go` و`policy/doc.go` يحددان `PostingPolicy` و`CurrencyPolicy`، ونطاق `CompanyID`، و`EffectiveDate`، و`EffectiveTo`، مع توثيق في [ADR P5-01](./ard/P5_01_policy_selection_and_historical_interpretation.md) | PASS |
| 2 | تعريف أولوية اختيار السياسة عند تعدد المرشحين | الانتقاء يُرتب الإصدارات حسب `EffectiveDate` بتوقيت UTC؛ النافذة نصف المفتوحة `[EffectiveDate, EffectiveTo)`، و`EffectiveTo == nil` تنتهي عند الإصدار التالي المسجّل. الدليل: `TestPolicyEffectiveDateBoundaryMatrix` | PASS |
| 3 | رفض التداخل أو التعارض غير المحسوم | `ErrPolicyOverlap` و`checkWindowRegistration` يرفضان التداخل؛ تكرار تاريخ البداية يبقى `ErrInvalidPolicy` توافقاً مع السلوك السابق. الدليل: `TestPolicyEngineRejectsUnresolvedOverlaps` | PASS |
| 4 | اختبار غياب سياسة نافذة (Fail Closed) | `ErrPolicyNotFound` عند الفجوة أو الغياب؛ الدليل: `TestPolicySelectionIsScopedAndFailsClosed` و`TestPostEntryIdentityAndPolicyRejectMatrix` | PASS |
| 5 | اختبار حدود Effective Date | مصفوفة قبل البداية/عند البداية/داخل المدة/عند النهاية/بعد النهاية مطبّقة على كلا النوعين؛ الدليل: `TestPolicyEffectiveDateBoundaryMatrix` | PASS |
| 6 | تدقيق سياسة العملة والعملة الوظيفية والمسموحة والدقة والتقريب | `TestCurrencyPolicyValidationAndRoundingAudit` يغطي `FunctionalCurrency` و`AllowedCurrencies` و`Precision` و`Rounding`؛ بوابةFloat موجودة في `doc_test.go` | PASS |
| 7 | تدقيق سياسة الترحيل والفترات Soft/Hard Closed | `AllowSoftClosedPosting` من السياسة المختارة فقط؛ `HardClosed` ممنوع في `masterdata`. الدليل: `TestSelectedPostingPolicyControlsSoftCloseOnly` و`TestSoftClosedPostingUsesPolicyOnly` | PASS |
| 8 | تدقيق أهلية الحساب والأبعاد ومركز التكلفة | `policy/eligibility.go` يضيف `ErrNilJournalEntry` و`ErrAccountNotFound` و`ErrAccountInactive` و`ErrAccountNotEffective` ويفحص انتماء الشركة وتاريخ النفاذ؛ الدليل: `TestAccountEligibilityDecisionMatrix` و`TestAccountEligibilityRejectsControlAccounts` | PASS |
| 9 | اختبار عدم تعديل السياسة المحفوظة عبر slice/pointer مشترك | نسخ دفاعي عند التسجيل والإرجاع (`AllowedCurrencies`، `DimByAccountType`، `EffectiveTo`). الدليل: `TestStoredPoliciesAreIsolatedFromSharedSlicesAndPointers` | PASS |
| 10 | تحديد نسخة/مرجع/بصمة السياسة مع ADR | `policy.Fingerprint(posting, currency)` بصمة `sha256:` حتمية، تُختَم في `journal.JournalEntry.PolicyFingerprint` بعد كل التحقق وقبل Executor، وتدخل `ValidateImmutability`. القرار في [ADR P5-01](./ard/P5_01_policy_selection_and_historical_interpretation.md) | PASS |
| 11 | اختبار تغيير السياسة بعد الترحيل وعدم تغيير الأثر السابق | `TestPostEntryStampsPolicyFingerprintAndKeepsHistoryStable` يثبت أن القيد المرحّل يحتفظ بالبصمة وأن تعديل السياسة اللاحق لا يعيد تفسيره | PASS |
| 12 | اختبارات التعارض والتزامن قراءة/تحديث في Adapter | `TestPolicyEngineConcurrentResolveAndVersionUpdate` و`TestConcurrentRegistrationOfSameEffectiveStartHasOneWinner`؛ جانب Adapter/المضيف موثّق كفجوة `G-020` (خارج نطاق النواة) | PASS داخل النواة؛ `G-020` مفتوحة |
| 13 | تحديث `RULE-KERNEL-010` و`RULE-KERNEL-019` ومصفوفة التتبع | `KERNEL_TRACEABILITY_MATRIX.md`: القاعدتان 010 و019 محدّثتان، والقسم 1.4 يلخّص أدلة المرحلة 5؛ `KERNEL_ACCOUNTING_INVARIANTS.md` يضيف قسم توضيحات المرحلة 5 غير المعدّل للقواعد | PASS |

## 3. التغييرات الإنتاجية

| الملف | التغيير |
| --- | --- |
| `internal/kernal/policy/posting_rules.go` | حقول `EffectiveTo *time.Time`، تطبيع UTC، نوافذ نصف مفتوحة، فرز الإصدارات، رفض التداخل `ErrPolicyOverlap`، `ErrPolicyNotFound` عند الغياب، نسخ دفاعي لكل المدخلات والمخرجات القابلة للتعديل |
| `internal/kernal/policy/fingerprint.go` (جديد) | `Fingerprint(posting, currency)` وترميز حتمي للحقول بما فيها `EffectiveTo` والعملات والدقة والتقريب والقيم المنطقية |
| `internal/kernal/policy/eligibility.go` | أخطاء صريحة جديدة، فحص انتماء الشركة، وفحص نفاذ الحساب بتاريخ `entry.EffectiveDate` |
| `internal/kernal/policy/doc.go` | تحديث charter الحزمة بمسؤوليات المرحلة 5 وحدودها |
| `internal/kernal/journal/entry.go` | حقل `PolicyFingerprint` وإدراجه في `entriesEqual`/`ValidateImmutability` |
| `internal/kernal/posting/pipeline.go` | ختم البصمة بعد اكتمال كل التحقق وقبل استدعاء `PostingExecutor`، دون تغيير ترتيب الفحوصات القائمة |
| `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` | قسم 14: توضيح نافذة السياسة، الفشل المغلق، البصمة، وأهلية الحساب (غير تعديل للقواعد) |
| `internal/kernal/KERNEL_TRACEABILITY_MATRIX.md` | تحديث 010/019 وملحق أدلة 1.4 |
| `docs/ard/P5_01_policy_selection_and_historical_interpretation.md` (جديد) | قرار النوافذ، الانتقاء، رفض التداخل، والبصمة |
| `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` | تحديث التوزيع إلى 21 فجوة، وإضافة `G-020` و`G-021` وأدلة التشغيل |

الإضافات عامة متوافقة مع الإصدارات السابقة: حقول جديدة فقط (`EffectiveTo`، `PolicyFingerprint`) وخطأ جديد (`ErrPolicyOverlap`)؛ لا حذف ولا تغيير توقيع دالة عامة.

## 4. منهجية Test-First

1. كُتبت الاختبارات الحمراء أولاً في `internal/kernal/policy/policy_integrity_test.go` و`internal/kernal/posting/policy_fingerprint_test.go`.
2. فشل التجميع قبل التنفيذ كما هو متوقع: `unknown field EffectiveTo`، و`undefined: ErrPolicyOverlap`، و`undefined: policy.Fingerprint`.
3. نُفّذت التغييرات الإنتاجية أعلاه حتى تحوّلت الاختبارات إلى أخضر دون تخفيف أي تأكيد (assertion).
4. أُصلح خلل في إعداد اختبار Soft-close كان يستخدم `masterdata.FiscalYearSoftClosed`؛ يُستخدم الآن `FiscalYearOpen` حتى يبقى الخطأ المتوقع `ErrClosedPeriod` من مستوى السنة لا من مستوى السياسة.
5. لم يُضف `time.Now()` في منطق النواة الجديد؛ الاختبارات تستخدم أوقاتاً ثابتة، ولا توجد قيم Float أو أخطاء متجاهلة في ملفات الإنتاج الجديدة.

## 5. القرارات المحفوظة

- **النافذة:** `[EffectiveDate, EffectiveTo)`؛ `EffectiveTo == nil` تنتهي عند الإصدار التالي المسجّل. حد النهاية حصري، فلا تداخل عند التماس.
- **الحتمية:** الترتيب حسب UTC `EffectiveDate`؛ تكرار البداية أو تداخل النوافذ خطأ صريح، وليس "آخر كاتب يفوز".
- **الفشل المغلق:** غياب السياسة أو الفجوة أو التداخل يمنع الترحيل؛ لا سياسة افتراضية صامتة.
- **التاريخ:** البصمة تُختَم في القيد المرحّل وتدخل التحقق من عدم التعديل؛ تغيير السياسة لاحقاً لا يعيد كتابة القديم.
- **الإقفال:** `masterdata` يملك قرار Soft/Hard، والسياسة تمنح `AllowSoftClosedPosting` فقط للفترة/السنة المختارة.
- **الأهلية:** فحص الحساب يشمل انتماء الشركة ونفاذ الحساب في تاريخ القيد إضافةً إلى النشاط ونوع التحكم.

## 6. الأدلة التشغيلية

| الأمر | النتيجة |
| --- | --- |
| `go version` | `go version go1.26.3 linux/amd64` (و`go.mod`: `go 1.26.0` / `toolchain go1.26.3`) |
| `gofmt -l internal/kernal/` (قبل التغيير) | لا إخراج |
| `go test ./internal/kernal/... -count=1` (قبل التغيير) | `ok` لجميع الحزم العشر |
| `gofmt -l internal/kernal/` (بعد التغيير) | لا إخراج |
| `go test ./internal/kernal/... -count=1` | `ok` لجميع الحزم العشر؛ `policy 0.009s`، `posting 0.014s`، `journal 0.005s` |
| `go test -race ./internal/kernal/policy ./internal/kernal/posting -count=1` | `ok` |
| `go test -race ./internal/kernal/... -count=1` | `ok` لجميع الحزم العشر (النواة كاملة تحت Race Detector) |
| `./scripts/verify_architecture.sh` | `RESULT: PASS - clean architectural base` — 11/11 قاعدة |
| `go test ./... -count=1` | PASS للمستودع كاملاً، بما فيه `storage/postgres` واختبارات `test/acid` و`test/atomic` و`test/concurrency` و`test/contract_parity` و`test/dr` |
| `git status --short` بعد الاختبار الكامل | تقريرا التكافؤ المتتبعان غير ظاهرين؛ أي تعديل مؤقت من `go test ./...` أُعيد نسخه من `/tmp/opencode/phase5/parity{1,2}.md` (انظر `G-010`) |

## 7. الوثائق والـ ADR

- أُنشئ [ADR P5-01](./ard/P5_01_policy_selection_and_historical_interpretation.md) لأن القرار يمس نموذج `journal.JournalEntry` بإضافة حقل البصمة، ويحدد بديلاً مرفوضاً (الاعتماد على "آخر سياسة محفوظة" دون بصمة).
- أُضيف قسم «Phase 5 policy and accounting eligibility clarifications» في `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` كتوضيح غير تعديل للقواعد 010 و017 و019.
- لا توجد قرارات تصميم إضافية خارج [ADR P5-01](./ard/P5_01_policy_selection_and_historical_interpretation.md)؛ التقرير يوثّق إصلاح إعداد اختبار واحد فقط (انظر القسم 4).

## 8. الفجوات المتبقية والقيود

| ID | الأولوية | الخلاصة | المالك | المرحلة |
| --- | --- | --- | --- | --- |
| `G-020` | High | المضيف/المحولات لا تحفظ سلاسل إصدارات السياسات ولا تضمن تحديثها ذرياً؛ النواة تضمن الانتقاء والبصمة لكنها لا تثبت عدم إعادة التفسير مع Adapter فعلي | storage adapters + host | 11–12 |
| `G-021` | Medium | `pkg/sdk/application.go:266-285` يتجاهل أخطاء `SetCurrencyPolicy`/`SetPolicy` (`_ = ...`)؛ يجب إرجاعها أو تسجيلها صراحةً مع اختبار SDK | sdk / host | 11–12 |

لا تُغلق هاتان الفجوتان داخل المرحلة 5 لأن نطاقها `internal/kernal/policy/**` وموصلاتها فقط. النواة تبقى آمنة بالفشل المغلق عند غياب الإعداد.

## 9. الدليل النهائي

تم التنفيذ الكامل للمهام 1–13 داخل نطاق المرحلة 5، مع اختبارات حمراء ثم خضراء، وتحقق Race، وبوابة معمارية، واختبار مستودع كامل. لا توجد قاعدة محاسبية جديدة ولا تعديل على القواعد القائمة، وتبقى التعديلات كلها في شجرة العمل للمراجعة (بدون commit أو push).

**PHASE 5 GATE: PASS**
