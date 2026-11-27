# مواصفات مجال السياسات المحاسبية (Accounting Policy Domain Specification)

**الإصدار:** 1.2.0  
**نوع المستند:** مواصفات مجال مستقلة (Standalone domain specification)  
**حالة النضج:** مواصفة معيارية مرشحة للاعتماد (`Review Candidate`)  
**حالة المطابقة:** معياري إلزامي (Normative)  
**المالك / المعتمد:** مالك معمارية النواة ومالك مجال السياسات (`policy`)  
**تاريخ التحديث:** 2026-11-27  
**النطاق:** سياسات الترحيل والعملات المقيدة بالشركة والمؤرخة بالنفاذ (`Effective-dated posting and currency policies`)، وتسجيل السياسات غير المتداخل، وقواعد أهلية الحسابات والأبعاد، والبصمة القانونية للسياسة (`Canonical policy fingerprinting`).

---

## 1. الغرض (Purpose)

يُعرّف هذا المستند العقد المجالي القابل للملاحظة خارجياً للسياسات المحاسبية وتقييم الأهلية داخل النواة (`Kernel`). وهو يحدد كيفية تسجيل إصدارات `PostingPolicy` و`CurrencyPolicy`، واختيارها بشكل حتمي لزوج `(CompanyID, EffectiveDate)` محدد، والتحقق من عدم تداخلها أو غياب تغطيتها، وختم بصمتها على القيود المرحّلة لضمان قابلية التدقيق التاريخي (`Historical auditability`).

لا يفرض هذا المستند لغة برمجة، أو محرك تخزين للسياسات، أو واجهة مستخدم لإدارة السياسات.

### 1.1 اللغة المعيارية (Normative language)

تُفسَّر المصطلحات **يجب (MUST)**، و**يجب ألّا (MUST NOT)**، و**ينبغي (SHOULD)**، و**ينبغي ألّا (SHOULD NOT)**، و**يجوز (MAY)** بوصفها مصطلحات متطلبات معيارية وفق `RFC 2119` و`RFC 8174`:

- **يجب / يجب ألّا (MUST / MUST NOT):** متطلب أو حظر مُطلَق للمطابقة.
- **ينبغي / ينبغي ألّا (SHOULD / SHOULD NOT):** متطلب مُوصى به؛ وأي انحراف عنه يتطلب سبباً موثقاً.
- **يجوز (MAY):** سلوك مسموح به.

### 1.2 حدود النطاق (Scope boundary)

تحكم هذه المواصفات إصدار السياسات، والاستخلاص الحتمي للسياسات، وفحوصات أهلية الحسابات والأبعاد، والبصمة القانونية للسياسة داخل النواة. تعود ملكية حالة إقفال الفترات المالية وقواعد تعريف شجرة الحسابات إلى `MASTERDATA_DOMAIN_SPECIFICATION_v1.2.0.md`. وتعود حسابات التقريب إلى `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md`، بينما يعود تنفيذ تسوية التقريب على مستوى القيد إلى `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.2.0.md`. أما التحكم في الوصول القائم على الأدوار (`RBAC`) وسير عمل الموافقات فيقعان خارج النواة.

---

## 2. المطابقة (Conformance)

لا يُعدّ أي تنفيذ مطابقاً لهذه المواصفات إلّا عند استيفاء جميع المتطلبات المعيارية المنطبقة.

### 2.1 متطلبات المطابقة (Conformance requirements)

التنفيذ المطابق:

1. **يجب (MUST)** أن يستوفي كل قاعدة معيارية في الأقسام من §4 إلى §7.
2. **يجب (MUST)** أن يفرض نوافذ سريان نصف مفتوحة `[EffectiveDate, EffectiveTo)` لكل شركة.
3. **يجب (MUST)** أن يرفض تواريخ سريان السياسات المتداخلة أو المكررة عند التسجيل.
4. **يجب (MUST)** أن يفشل مغلقاً (`Fail closed`) عند عدم تغطية أي سياسة للزوج `(CompanyID, EffectiveDate)`.
5. **يجب (MUST)** أن يُنتج خلاصات `PolicyFingerprint` قانونية حتمية مستقلة عن الترتيب وعن رقم إصدار المخزن.

### 2.2 أدلة المطابقة (Conformance evidence)

يمكن إثبات المطابقة من خلال:

- اختبارات حدود تاريخ السريان (`start <= asOf < end`، النوافذ المتجاورة، والفجوات غير المغطاة)؛
- اختبارات رفض التسجيل المتداخل والمكرر؛
- اختبارات الفشل المغلق عند عدم تسجيل أي سياسة؛
- اختبارات مصفوفة قرار أهلية الحساب (نشط، مؤرخ النفاذ، حساب رقابة، مركز تكلفة، عملة)؛
- اختبارات استقرار البصمة القانونية وعدم تأثرها بتبديل الترتيب أو رقم الإصدار؛
- اختبارات سلامة القراءة والتسجيل المتزامن.

### 2.3 المطابقة الجزئية (Partial conformance)

**يجب ألّا (MUST NOT)** يُوصَف التنفيذ الذي يستوفي مجموعة فرعية فقط من المتطلبات بأنه مطابق بالكامل. و**يجب (MUST)** أن يُحدّد أي ادعاء بالمطابقة الجزئية القواعد المُغفلة والسلوكيات المتأثرة.

---

## 3. المصطلحات والتعاريف (Terms and definitions)

- **سياسة الترحيل (`Posting Policy`):** مجموعة قواعد مقيدة بالشركة ومؤرخة بالنفاذ تحكم قيود الترحيل مثل `AllowSoftClosedPosting`، ومتطلبات الأبعاد الإلزامية، وقواعد نوع الحساب والترحيل.
- **سياسة العملة (`Currency Policy`):** مجموعة قواعد مقيدة بالشركة ومؤرخة بالنفاذ تحكم `FunctionalCurrency`، وعملات المعاملات المسموحة، و`AutoSettleRoundingDifferences`، و`RoundingDifferenceAccountID`.
- **نافذة النفاذ نصف المفتوحة (`Half-Open Effective Window [EffectiveDate, EffectiveTo)`):** فترة تشمل `EffectiveDate` (البداية) وتستثني `EffectiveTo` (النهاية)؛ وتدل قيمة `EffectiveTo` غير المعيّنة على صلاحية مفتوحة النهاية حتى الإصدار اللاحق أو إلى ما لا نهاية.
- **بصمة السياسة (`Policy Fingerprint`):** خلاصة قانونية بصيغة `sha256:<hex>` لحقول `PostingPolicy` و`CurrencyPolicy` المحلولة تُختم على `JournalEntry` المرحّل لضمان قابلية إعادة الإنتاج التاريخية.

---

## 4. إصدار السياسات واختيارها وعزلها

### POL-001 — عزل السياسات على مستوى الشركة (Company-scoped policy isolation)

**يجب (MUST)** أن تكون جميع عمليات تسجيل وبحث `PostingPolicy` و`CurrencyPolicy` مقيدةً بصرامة بمعرّف `CompanyID` غير صفري. و**يجب ألّا (MUST NOT)** تُحل سياسة مسجلة للشركة (أ) أبداً لصالح الشركة (ب).

**معايير القبول**

- يفشل الاستعلام عن سياسة بمعرّف `CompanyID` صفري أو لشركة ليس لها سياسات مسجلة فشلاً مغلقاً (`ErrCompanyScopeRequired` أو `ErrPolicyNotFound`).

### POL-002 — نافذة النفاذ نصف المفتوحة `[EffectiveDate, EffectiveTo)`

**يجب (MUST)** تقييم نوافذ صلاحية السياسة بوصفها فترات نصف مفتوحة `[EffectiveDate, EffectiveTo)` بتوقيت UTC:

- تطابق السياسة التاريخ `asOf` إذا وفقط إذا كان `EffectiveDate <= asOf` و(`EffectiveTo` غير معيّنة أو `asOf < EffectiveTo`).
- **يجب (MUST)** السماح بالنوافذ المتجاورة حيث `prev.EffectiveTo == next.EffectiveDate` دون تداخل.
- **يجب (MUST)** السماح بالفجوات غير المغطاة بين النوافذ عند التسجيل و**يجب (MUST)** أن تفشل مغلقاً بخطأ `ErrPolicyNotFound` عند الاستعلام ضمنها.

**معايير القبول**

- عند `asOf == EffectiveTo`، لا يعود اختيار السياسة السابقة ويُختار الخلف المجاور (إن وُجد).
- يُرجع الاستعلام عن تاريخ يقع داخل فجوة الخطأ `ErrPolicyNotFound`.

### POL-003 — التسجيل الحتمي غير المتداخل (Non-overlapping deterministic registration)

بالنسبة إلى `CompanyID` ونوع سياسة محددين، **يجب (MUST)** رفض تسجيل سياسة يكرر تاريخها `EffectiveDate` إصداراً قائماً أو تتداخل نافذتها `[EffectiveDate, EffectiveTo)` مع أي إصدار قائم، وذلك بخطأ `ErrPolicyOverlap`. و**يجب (MUST)** أن تسفر عمليات التسجيل المتزامنة لنفس بداية النفاذ عن فائز واحد على الأكثر.

**معايير القبول**

- يُرجع تسجيل سياستين لهما نفس `(CompanyID, EffectiveDate)` أو نطاقان متداخلان الخطأ `ErrPolicyOverlap`.

### POL-004 — الفشل المغلق عند غياب السياسة الافتراضية (Fail-closed absence of default policy)

**يجب ألّا (MUST NOT)** تُصنّع النواة سياسة `PostingPolicy` أو `CurrencyPolicy` افتراضية أو بديلة عند عدم وجود سياسة مسجلة للزوج `(CompanyID, EffectiveDate)`. و**يجب (MUST)** أن يُرجع الغياب الخطأ `ErrPolicyNotFound`.

**معايير القبول**

- يفشل الترحيل دون سياسات مسجلة بخطأ `ErrPolicyNotFound` قبل أي استدعاء للمنفّذ (`Executor`).

### POL-005 — العزل الدفاعي للسياسات المخزنة (Defensive isolation of stored policies)

**يجب (MUST)** أن يقوم تسجيل السياسات والبحث عنها بنسخ القوائم القابلة للتعديل والحقول المرجعية دفاعياً (مثل `AllowedCurrencies` و`EffectiveTo`) بحيث لا تؤدي تعديلات المستدعي بعد التسجيل أو الحل إلى إفساد حالة السياسة الداخلية.

**معايير القبول**

- لا يؤدي تعديل قائمة `AllowedCurrencies` لدى المستدعي بعد تسجيل `CurrencyPolicy` إلى تغيير تقييمات السياسة اللاحقة.

---

## 5. أهلية الحساب والعملة والأبعاد

### POL-006 — إنفاذ سياسة العملة (Currency policy enforcement)

**يجب (MUST)** أن تكون عملة المعاملة `TransactionCurrency` لكل سطر مسموحاً بها بموجب `CurrencyPolicy` المحلولة. وإذا كان خيار `AutoSettleRoundingDifferences` مفعّلاً، **يجب (MUST)** أن يكون `RoundingDifferenceAccountID` غير صفري ومؤهلاً للترحيل؛ وإلا **يجب ألّا (MUST NOT)** تمضي تسوية التقريب قدماً.

**معايير القبول**

- تفشل عملة المعاملة غير المسموحة في `CurrencyPolicy` بخطأ `ErrCurrencyNotAllowed`.
- يفشل تفعيل `AutoSettleRoundingDifferences` دون `RoundingDifferenceAccountID` صالح في التحقق من التقريب.

### POL-007 — أهلية ترحيل الحساب (Account posting eligibility)

**يجب (MUST)** أن يقيّم فحص أهلية الحساب لقطة الحساب مؤرخة النفاذ المحلولة عند `entry.EffectiveDate` و**يجب (MUST)** أن يرفض الترحيل إذا تحقق أي مما يلي (مع بقاء ملكية قواعد تعريف الحساب في `MASTERDATA` وإنفاذها في محرك الأهلية):

1. ينتمي الحساب إلى `CompanyID` مختلف عن القيد (`ErrAccountIdentityMismatch`).
2. الحساب غير نافذ عند `entry.EffectiveDate` (`ErrAccountNotEffective`، المكافئ في محرك السياسات لـ `ErrAccountNotCurrent` في `MASTERDATA`).
3. الحساب غير نشط (`IsActive == false`) أو موقوف (`Suspended`).
4. الحساب حساب تجميعي رئيسي (`IsHeader == true`) (`ErrHeaderAccountPosting`).
5. الحساب حساب رقابة (`IsControlAccount == true`) (`ErrControlAccountNotPostable`).
6. الحساب مقيد بعملة محددة لا تطابق عملة السطر.
7. الحساب يحدد `RequiresCostCenter == true` (أو تشترط السياسة مراكز التكلفة) ويفتقر السطر إلى `CostCenterID` صالح (`ErrCostCenterRequired`).

**معايير القبول**

- يُرفض حساب الرقابة أو الحساب غير النشط أو الخارج عن نافذة النفاذ صراحةً حتى لو كان القيد متوازناً.
- يُفرض شرط `RequiresCostCenter` على مستوى الحساب حتى عندما لا تفرض سياسة الشركة مراكز التكلفة بصورة عامة.

### POL-008 — تفويض سياسة الإقفال الناعم (Soft-close policy delegation)

تحكم الراية `PostingPolicy.AllowSoftClosedPosting` ما إذا كانت الفترة المالية بحالة `SOFT_CLOSED` مقبولة أثناء الترحيل. و**يجب ألّا (MUST NOT)** يفرض محرك السياسات أدوار المستخدمين، أو موافقات المديرين، أو تجاوزات `HARD_CLOSED`؛ وتظل ملكية إنفاذ حالة إقفال الفترة تابعةً لحزمة `masterdata`.

**معايير القبول**

- تسمح `AllowSoftClosedPosting == true` بالترحيل في فترة `SOFT_CLOSED`، بينما تظل `HARD_CLOSED` مرفوضة دون شروط.

---

## 6. البصمة القانونية للسياسة (Canonical policy fingerprinting)

### POL-009 — بصمة `PolicyFingerprint` الحتمية المرتبطة بالمحتوى

عند نجاح التحقق، **يجب (MUST)** أن يحسب أنبوب الترحيل ويثبت `PolicyFingerprint` (`sha256:<hex>` ببادئة `policy-fingerprint-v1|` وبصيغة `<key>=<utf8_value>|` باستخدام `UTC RFC3339Nano` للتواريخ و`-` للنهايات غير المعيّنة) على القيد المجمد من الحقول الدلالية التالية في `PostingPolicy` و`CurrencyPolicy` المحلولتين:

- `CompanyID`، وتاريخا `EffectiveDate` و`EffectiveTo` لكل سياسة؛
- `AllowSoftClosedPosting`، وضوابط التأريخ الرجعي (`MaxBackdatingDays`, `AllowBackdatingException`)، وإلزامية الأبعاد لحسابات الإيرادات والمصروفات (`RequireCostCenterForIncomeExpense`, `RequireAnalyticalDimensionForIncomeExpense`)؛
- `FunctionalCurrency`، و`PresentationCurrency`، وقائمة `AllowedCurrencies` (مطبّعةً بأحرف كبيرة ومفروزةً معجمياً ومفصولةً بفواصل)، و`AutoSettleRoundingDifferences`، و`RoundingDifferenceAccountID`، وطريقة التقريب `RoundingMethod` (تُحل عند الفراغ إلى التقريب المصرفي `Half-to-Even` وفق `MONEY-019`).

**يجب ألّا (MUST NOT)** يدخل رقم إصدار التخزين (`Version` في `PolicyVersion`) ضمن مدخلات البصمة (`ADR P11-04 §7`)، و**يجب (MUST)** أن تكون `PolicyFingerprint` دليل تدقيق فقط تشمله فحوصات عدم القابلية للتعديل بعد الترحيل.

**معايير القبول**

- ينتج عن إعادة ترتيب `AllowedCurrencies` أو تغيير رقم إصدار التخزين دون تغيير دلالي نفس `PolicyFingerprint` بالضبط.
- ينتج عن تغيير أي حقل دلالي في السياسة بصمة `PolicyFingerprint` مختلفة.

---

## 7. عقد الأخطاء (Error Contract)

| فئة الخطأ | شرط التحفيز | توقيت الاكتشاف |
| --- | --- | --- |
| `ErrPolicyNotFound` | لا توجد سياسة تغطي `(CompanyID, EffectiveDate)` | حل السياسة |
| `ErrPolicyOverlap` | تكرار `EffectiveDate` أو تداخل نافذة | تسجيل السياسة |
| `ErrCurrencyNotAllowed` | عملة السطر مستبعدة من `CurrencyPolicy` | فحص عملة السطر |
| `ErrAccountNotEffective` | الحساب غير نافذ عند `entry.EffectiveDate` (مكافئ `ErrAccountNotCurrent` في `MASTERDATA`) | أهلية الحساب |
| `ErrHeaderAccountPosting` | الترحيل المباشر على حساب تجميعي (`IsHeader == true`) | أهلية الحساب |
| `ErrControlAccountNotPostable` | ترحيل مباشر على `IsControlAccount == true` | أهلية الحساب |
| `ErrCostCenterRequired` | غياب `CostCenterID` عند إلزاميته بالحساب أو السياسة | أهلية الحساب |

---

## 8. المراجع (References)

### REF-001 — RFC 2119 & RFC 8174

- **العنوان الكامل:** Key words for use in RFCs to Indicate Requirement Levels (RFC 2119); Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words (RFC 8174).
- **الجهة الناشرة:** Internet Engineering Task Force (IETF).
- **الإصدار والتاريخ:** RFC 2119 (مارس 1997)، RFC 8174 (مايو 2017)؛ روجعتا في 2026-11-27.
- **الروابط الرسمية:** <https://www.rfc-editor.org/rfc/rfc2119>, <https://www.rfc-editor.org/rfc/rfc8174>
- **القيود:** يحدد اللغة المعيارية فقط.

### REF-002 — قرارات الهندسة المعمارية وقواعد السياسات في Accountant Engine

- **المصادر الداخلية:** `docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md#adr-p5-01`، `docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md#adr-p11-04`، `docs/ACCOUNTING_BUSINESS_RULES.md` (§5)، `docs/KERNEL_POSTING_FLOW_SPECIFICATION.md` (§3).
- **مستخدم في:** نوافذ النفاذ نصف المفتوحة، رفض التداخل، الحل الفاشل مغلقاً، مصفوفة الأهلية، وبصمة السياسة القانونية (`POL-001–009`).

### فهرس ربط المراجع بالقواعد

| القاعدة/القواعد | المرجع الأساسي | طبيعة العلاقة |
| --- | --- | --- |
| POL-001–005 | REF-002 (ADR P5-01, P11-04) | قواعد المشروع لإصدار السياسات وعزلها |
| POL-006–008 | REF-002 (AE-RULE-ACC-001) | قواعد المشروع للأهلية وتفويض الإقفال الناعم |
| POL-009 | REF-002 (ADR P5-01, ADR P11-04 §7) | بصمة المشروع لقابلية التدقيق التاريخي واستبعاد رقم الإصدار |

---

## 9. الملاحق (Appendices)

### الملحق أ — سجل القرارات الهندسية (Decision Log)

- **DEC-POL-001 (النوافذ نصف المفتوحة):**
  - **القرار:** اعتماد `[EffectiveDate, EffectiveTo)` للسياسات (`ADR P5-01`) لتسهيل سلاسل الإصدارات المتعاقبة والفجوات الفاشلة مغلقاً.
  - **البدائل المدروسة:** النوافذ المغلقة أو الاحتفاظ بـ"أحدث تاريخ" ضمنياً؛ رُفضا لعدم إمكانية اختبار حد النهاية والفجوات.
  - **المالك والتاريخ ومُحفّز المراجعة:** مالك `policy` (2026-11-22)؛ ثابت عقدي.
- **DEC-POL-002 (استبعاد رقم الإصدار من `PolicyFingerprint`):**
  - **القرار:** تضمين الحقول الدلالية فقط دون رقم إصدار قاعدة البيانات (`ADR P11-04 §7`).
  - **البدائل المدروسة:** تضمين رقم الإصدار؛ رُفض لأنه يبطل بصمات القيود التاريخية عند الهجرة أو إعادة التسجيل المتطابق دلالياً.
  - **المالك والتاريخ ومُحفّز المراجعة:** مالك المعمارية ومالك `policy` (2026-11-22)؛ ثابت عقدي.

### الملحق ب — تتبع القواعد (Rule traceability)

| معرّف القاعدة | الملخص | التصنيف | المصدر / القرار | معيار التحقق |
| --- | --- | --- | --- | --- |
| POL-001 | عزل السياسة على مستوى الشركة | PROJECT-DECISION | ADR P5-01 | اختبار فشل `CompanyID` صفري أو غير مسجل |
| POL-002 | نافذة `[EffectiveDate, EffectiveTo)` نصف مفتوحة | PROJECT-DECISION | ADR P5-01 | اختبار حدود `asOf == EffectiveTo` والفجوات |
| POL-003 | تسجيل غير متداخل | PROJECT-DECISION | ADR P5-01 | اختبار رفض التداخل والتكرار والتزامن |
| POL-004 | فشل مغلق عند غياب السياسة | PROJECT-DECISION | ADR P5-01 | اختبار غياب السياسة دون تصنيع افتراضي |
| POL-005 | نسخ دفاعي للسياسات المخزنة | DERIVED-REQUIREMENT | ثابت العزل | اختبار تعديل شريحة المستدعي بعد التسجيل |
| POL-006 | سياسة العملة وفحص حساب التقريب | PROJECT-DECISION | AE-RULE-ACC-001 §3–4 | اختبار العملة الممنوعة وحساب التقريب |
| POL-007 | مصفوفة أهلية الحساب | PROJECT-DECISION | ADR P5-01 / AE-RULE-ACC-001 | اختبار مصفوفة شروط الأهلية السبعة |
| POL-008 | تفويض الإقفال الناعم للسياسة | PROJECT-DECISION | ADR P5-01 | اختبار `SOFT_CLOSED` مقابل `HARD_CLOSED` |
| POL-009 | بصمة `PolicyFingerprint` القانونية | PROJECT-DECISION | ADR P5-01 / P11-04 | اختبار ثبات البصمة واستقلالها عن الترتيب |

---

## 10. سجل الإصدارات (Version history)

### v1.2.0

- حصر الحقول الفعلية الداخلة في `PolicyFingerprint` (`policy-fingerprint-v1|`) وتوضيح حل `RoundingMethod` إلى `Half-to-Even` وفق `MONEY-019`.
- إضافة `ErrHeaderAccountPosting` إلى جدول الأخطاء §7 وتوثيق التكافؤ بين `ErrAccountNotEffective` و`ErrAccountNotCurrent`.
- استيفاء ترويسة النضج والمالك وحقول `NSM-001` في المراجع وسجل القرارات.

### v1.1.0

- حصر الحقول الدلالية الداخلة في `PolicyFingerprint` وتوثيق استبعاد رقم الإصدار في `POL-009` (`ADR P11-04 §7`).
- استيفاء مراجع `RFC 2119/8174` وتجريد المتن من أسماء دوال واختبارات Go، وإضافة سجل القرارات.

### v1.0.0

- المواصفة المعيارية المستقلة الأولية لمجال السياسات المحاسبية في النواة.
