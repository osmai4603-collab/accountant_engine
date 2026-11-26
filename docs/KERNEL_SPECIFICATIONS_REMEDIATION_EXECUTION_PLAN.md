# خطة تنفيذية لمعالجة عيوب وفجوات مواصفات النواة المحاسبية (`docs/specifications/kernal`)

**تاريخ الإنشاء:** 2026-11-27  
**المرجع التحليلي:** [KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md](./KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md)  
**المنهجية الحاكمة:** [NORMATIVE_SPECIFICATION_METHODOLOGY.md](./references/NORMATIVE_SPECIFICATION_METHODOLOGY.md) (`NSM-001`)  
**الهدف:** إغلاق جميع الفجوات والتناقضات والعيوب الهيكلية الـ 27 المرصودة في مواصفات النواة العشر وفي الوثائق المرتبطة بها داخل `docs/`، وصولاً إلى خط أساس معياري متسق وقابل للاعتماد (`Approved Normative Specification`).

---

## نظرة عامة على المراحل (Phases Overview)

| المرحلة | التركيز | الأولوية | المخرجات الرئيسية |
| --- | --- | --- | --- |
| **المرحلة 1** | سد الفجوات المحاسبية والتشفيرية الحرجة | Critical / High | حسم `IAS 21`، مواصفة الترميز `v2`، توحيد دقة التدقيق، رصيد العملة الأجنبية للتقييم، وتسامح التقريب |
| **المرحلة 2** | توحيد العقود والأخطاء الحارسة وآلات الحالات | Medium | توحيد `Sentinels`، تعميم قيود `ISO 4217`، حسم `Standard/Regular` و`Cancelled`، وإغلاق فجوات السياسات والفترات |
| **المرحلة 3** | الضبط الهيكلي والحوكمي وفق `NSM-001` | Governance / Low | توحيد أسماء الملفات والإصدارات، إزالة ازدواجية ملف النقود، استكمال الترويسة وسجل القرارات، وتجريد المتن |
| **المرحلة 4** | المراجعة المتقاطعة وبوابة الاعتماد | Verification | اجتياز قائمة `NSM-001 §14`، والتحقق من صفر تعارضات مع `docs/` و`docs/ard/` |

> **قاعدة التنفيذ:** يمكن تنفيذ مهام المرحلة 1 والمرحلة 2 بالتوازي بعد حسم القرارات المعمارية المشتركة، وتُنفَّذ المرحلة 3 بعد استقرار التعديلات الموضوعية حتى تُرقّى الإصدارات وتُحدّث سجلات التغيير دفعة واحدة.

---

## المرحلة 1: سد الفجوات المحاسبية والتشفيرية الحرجة (`Critical / High`)

### P1-T1 — إضافة سمة البند النقدي (`IsMonetary`) في نموذج الحساب

- **الفجوة المغطاة:** `G-IAS21-1`
- **الملفات المستهدفة:**
  - [MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md) (`MD-004`)
  - [FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md) (`VAL-006`)
- **الإجراء التنفيذي:**
  1. تعريف سمة إلزامية أو صريحة في `MD-004` على مستوى `Account` تحدد طبيعة البند (`IsMonetary` / `MonetaryClassification`) لحسابات `Asset` و`Liability`.
  2. ربط `VAL-006` بهذه السمة كشرط موضوعي لرفض البنود غير النقدية عند التقييم بسعر الإقفال (`IAS 21.23(a)`).
- **معيار القبول:** وجود حقل/سمة معرّفة في `MD-004` واختبار قبول يثبت التمييز بين أصل نقدي وأصل غير نقدي دون افتراض ضمني.

### P1-T2 — حسم نطاق `IAS 21` وتوحيده عبر الوثائق

- **الفجوة المغطاة:** `G-IAS21-2`, `G-IAS21-3`, `G-IAS21-4`
- **الملفات المستهدفة:**
  - [FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md) (§1.2، `VAL-001`, `VAL-006`, `REF-001`)
  - [JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.0.0.md) (`JRN-004`)
  - [POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md) (`PST-004`)
  - [TARGET_ACCOUNTING_KERNAL.md](./TARGET_ACCOUNTING_KERNAL.md) (§3.4)
  - [ACCOUNTING_BUSINESS_RULES.md](./ACCOUNTING_BUSINESS_RULES.md) (`RULE-ACC-013`)
- **الإجراء التنفيذي:**
  1. تحديد قواعد اختيار السعر عند الاعتراف الأولي (`SPOT` بتاريخ المعاملة أو `AVERAGE` المعتمد وفق `IAS 21.21–22`) أو توثيق استبعاد التحقق من نوع السعر عند الترحيل اليومي كقرار مشروع صريح.
  2. توضيح حدود `IAS 21.23(c)` و`IAS 21.30–31` (البنود غير النقدية المقاسة بالقيمة العادلة وفروق `OCI`) في §1.2 من `FX_REVALUATION` بدلاً من الإيحاء بتغطيتها في `VAL-006`.
  3. حسم التناقض حول الفروق المحققة (`IAS 21.28`): إما تعديل `TARGET_ACCOUNTING_KERNAL.md` (§3.4) و`RULE-ACC-013` لحصر دور النواة في الفروق غير المحققة وإحالة المحققة إلى المدارات (`subledgers`)، أو إضافة عقد الفروق المحققة إلى `FX_REVALUATION`.
- **معيار القبول:** تطابق كامل في وصف حدود `valuation` بين `FX_REVALUATION` و`TARGET_ACCOUNTING_KERNAL.md` و`ACCOUNTING_BUSINESS_RULES.md`.

### P1-T3 — توثيق المواصفة البايتية الدقيقة للترميز القانوني `v2 length-prefixed`

- **الفجوة المغطاة:** `G-FIPS-1`
- **الملفات المستهدفة:**
  - [GOVERNANCE_AND_AUDIT_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/GOVERNANCE_AND_AUDIT_DOMAIN_SPECIFICATION_v1.0.0.md) (`GOV-003`)
  - [POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md) (`PST-009`)
  - [FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md) (`VAL-010`)
  - [ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.0.0.md) (`POL-009`)
- **الإجراء التنفيذي:**
  1. إضافة ملحق معياري أو قسم مخصص للترميز القانوني يحدد: نوع وحجم بادئة الطول (مثلاً `uint32` big-endian)، ترميز `UUID` (نص معياري أم 16 بايت)، صيغة الطابع الزمني (ميكروثانية UTC)، ترتيب الحقول، وصيغة `Payload digest`، مع متجه اختبار ذهبي (`Golden Test Vector`) وفق `NSM-001 §2(5)`.
- **معيار القبول:** تمكّن طرف ثالث من إعادة إنتاج نفس قيمة `sha256:<hex>` من المتن المعياري وحده دون قراءة شيفرة Go.

### P1-T4 — حل تعارض دقة الطابع الزمني وإطار التجزئة للتدقيق

- **الفجوة المغطاة:** تعارض `GOV-003` مع `RULE-ACC-041/042`
- **الملفات المستهدفة:**
  - [ACCOUNTING_BUSINESS_RULES.md](./ACCOUNTING_BUSINESS_RULES.md) (`RULE-ACC-041`, `RULE-ACC-042`)
- **الإجراء التنفيذي:**
  1. تحديث `RULE-ACC-041` لاعتماد التأطير مسبوق الطول (`v2 length-prefixed`) والحقول الـ 11 (بما فيها `Reason` و`CorrelationID`) بدلاً من الفواصل `v1|...`.
  2. تحديث `RULE-ACC-042` لاعتماد دقة الميكروثانية (`TIMESTAMPTZ` UTC) المتوافقة مع `GOV-003` و`KERNEL_AUDIT_THREAT_MODEL.md` (`T-01`).
- **معيار القبول:** خلو `ACCOUNTING_BUSINESS_RULES.md` من أي تعارض مع `GOVERNANCE_AND_AUDIT` ونموذج التهديد.

### P1-T5 — إضافة عقد استرجاع الرصيد بعملة المعاملة الأجنبية في `LEDGER_PROJECTION`

- **الفجوة المغطاة:** فجوة `LDG-003` / `LDG-004` لخدمة `FX_REVALUATION`
- **الملفات المستهدفة:**
  - [LEDGER_PROJECTION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/LEDGER_PROJECTION_DOMAIN_SPECIFICATION_v1.0.0.md) (`LDG-003`, `LDG-004`)
  - [FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md) (§1.2)
- **الإجراء التنفيذي:**
  1. تعريف عقد قراءة/إسقاط صريح يُرجع زوجاً من الأرصدة لكل `(AccountID, TransactionCurrency, Dimensions)`: صافي رصيد المعاملة (`TransactionAmount`) وصافي القيمة الدفترية الوظيفية (`FunctionalDebit - FunctionalCredit`)، مع إبقاء `BalanceReader` التقليدي مخصصاً للقياس الوظيفي وفق `ADR P11-03`.
- **معيار القبول:** وضوح مسار حصول `FX_REVALUATION` على رصيد العملة الأجنبية دون مخالفة `LDG-004`.

### P1-T6 — تحديد تسامح التقريب المسموح وعلاقة التطبيع في `PST-004`

- **الفجوة المغطاة:** غموض `PST-004` وتوقيت التطبيع مع `MONEY-028`
- **الملفات المستهدفة:**
  - [POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.0.0.md) (`PST-004`, §7)
- **الإجراء التنفيذي:**
  1. تحديد القيمة الرقمية الدقيقة لـ"تسامح التقريب المسموح" (مثلاً نصف أو وحدة صغرى للعملة الوظيفية) والخطأ الحارس المرجع عند تجاوز التسامح أو عكس الإشارة.
  2. النص صراحةً في `PST-004` على أن تطبيع `TransactionAmount` إلى وحدته الصغرى قبل الضرب في `ExchangeRate` هو تقييد ترحيلي خاص يسبق قاعدة `MONEY-028`.
- **معيار القبول:** قابلية اختبار حد الرفض في `PST-004` برقم محدد وخطأ محدد.

---

## المرحلة 2: توحيد العقود والأخطاء الحارسة وآلات الحالات (`Medium`)

### P2-T1 — توحيد الأخطاء الحارسة (`Sentinels`) وجداول `CodedError`

- **الملفات المستهدفة:**
  - [IDENTIFIERS_AND_ERRORS_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/IDENTIFIERS_AND_ERRORS_DOMAIN_SPECIFICATION_v1.0.0.md) (§5 `ERR-001`, §6)
  - [ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.0.0.md) (`POL-007`, §7)
  - [MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md) (`MD-007`, §8)
  - [FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md) (§7)
  - [KERNEL_IDEMPOTENCY_CONTRACT.md](./KERNEL_IDEMPOTENCY_CONTRACT.md) (§2)
- **الإجراء التنفيذي:**
  1. حسم التسمية المعتمدة بين `ErrAccountNotEffective` و`ErrAccountNotCurrent` (أو توثيق أحدهما كمرادف في `DEC-*`).
  2. حسم التسمية المعتمدة لمفتاح منع التكرار الفارغ بين `ErrIdempotencyKeyRequired` و`ErrIdempotencyScopeRequired` وتحديث `KERNEL_IDEMPOTENCY_CONTRACT.md` تبعاً لذلك.
  3. استكمال جدول أخطاء `FX_REVALUATION` §7 بأسماء `Sentinels` الصريحة (بما فيها `ErrInvalidAccountType`)، وإضافة `ErrHeaderAccountPosting` إلى `ACCOUNTING_POLICY` §7، واستكمال الأخطاء المفقودة في `IDENTIFIERS_AND_ERRORS` §6 مع جدول رموز `CodedError` (`ERR_CONFLICT`, `ERR_NOT_FOUND`, `ERR_FORBIDDEN`, `ERR_VALIDATION`).

### P2-T2 — تعميم قيود رموز `ISO 4217` الخاصة وتوثيق لقطة السجل

- **الملفات المستهدفة:**
  - [MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md) (`MD-002`, `MD-004`)
  - [MONEY_DOMAIN_SPECIFICATION_v2.6.1.md](./specifications/kernal/MONEY_DOMAIN_SPECIFICATION_v2.6.1.md) (`MONEY-010`, `MONEY-013`)
- **الإجراء التنفيذي:**
  1. تحديث `MD-002` و`MD-004` لاستثناء جميع الرموز غير القابلة للإنفاق وفق `MONEY-014` (`XXX`, `XTS`, `XAU`, `XAG`, `XDR`) وليس `XXX` فقط.
  2. توثيق رقم إصدار/تاريخ لقطة سجل `ISO 4217` المرجعية في ملحق `MONEY`.

### P2-T3 — حسم تصنيفات القيد (`Standard`/`Regular`، `Adjusting`، `Cancelled`، `Opening`)

- **الملفات المستهدفة:**
  - [JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.0.0.md) (`JRN-006`, `JRN-007`)
  - [FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/FX_REVALUATION_DOMAIN_SPECIFICATION_v1.0.0.md) (`VAL-007`)
  - [TARGET_ACCOUNTING_KERNAL.md](./TARGET_ACCOUNTING_KERNAL.md) (§3.8)
  - [internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md](../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md) (`RULE-KERNEL-009`)
- **الإجراء التنفيذي:**
  1. توحيد اسم التصنيف الافتراضي (`Standard` أو `Regular`) بين `JRN-006` و`RULE-KERNEL-009`.
  2. تحديد البيانات الوصفية الإلزامية (`AdjustingEntryType`) التي يختمها مولد قيد إعادة التقييم في `VAL-007` ليجتاز `JRN-006`.
  3. توثيق حالة `Cancelled` وحقولها (`CancelledAt`, `CancelledBy`, `CancellationReason`) وخطأ الإلغاء المكرر في `JRN-007` وتحديث ملخص `journal` في `TARGET_ACCOUNTING_KERNAL.md` (§3.8).
  4. تعريف مصطلح "الرصيد الافتتاحي المعتمد" في `JRN-006`.

### P2-T4 — توحيد `Revenue` مقابل `Income` وفق الإطار المفاهيمي 2018

- **الملفات المستهدفة:**
  - [MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/MASTERDATA_DOMAIN_SPECIFICATION_v1.0.0.md) (`MD-004`)
  - [LEDGER_PROJECTION_DOMAIN_SPECIFICATION_v1.0.0.md](./specifications/kernal/LEDGER_PROJECTION_DOMAIN_SPECIFICATION_v1.0.0.md) (`LDG-005`)
- **الإجراء التنفيذي:**
  1. توثيق الترادف بين `Revenue` و`Income` في `MD-004` و`LDG-005` بما يتوافق مع `Conceptual Framework 2018 Ch. 4` و`TARGET_ACCOUNTING_KERNAL.md` (§3.9).

### P2-T5 — إغلاق الفجوات المتبقية في `PST` و`POL` و`MD` و`LDG` و`GOV`

- **الإجراء التنفيذي:**
  1. **`PST-002`:** تعديل النص ليشمل الإجهاض عند فشل أي تحقق سابق للتفويض الفعلي للمنفذ (بما في ذلك المرحلة 8: `ValidatePostingPolicy` وتلميح `revaluation` المزور).
  2. **`POL-007` و`POL-009`:** توضيح حدود الملكية بين `policy` و`masterdata`، وتعريف "حدود الترحيل المعتمدة" وقيم "طريقة التقريب" المسموحة في `ACCOUNTING_POLICY`.
  3. **`MD-013`:** إضافة مصفوفة قرار صريحة لتداخل `(FiscalYear.State × FiscalPeriod.State)` خصوصاً لحالة `OPEN × SOFT_CLOSED`.
  4. **`LDG-002` و`LDG-006`:** تحديد مصدر `SequenceNumber` في `LDG-002` وحالته عند `0`، وحسم حدود مسؤولية النواة في `LDG-006` (التحقق من قيود `Closing`/`Opening` مقابل توليدها).
  5. **`GOV-004` و`GOV-006`:** حصر قائمة `Action` المعتمدة في `GOV-004`، وتحديد شرط `Version` عند الحالة الصفرية (`SequenceNumber == 0`) في `GOV-006`.

---

## المرحلة 3: الضبط الهيكلي والحوكمي للملفات (`NSM-001`)

### P3-T1 — مواءمة أسماء الملفات والإصدارات

- ترقية المواصفات الثماني إلى الإصدار الموحد (مثلاً `v1.2.0` بعد إدخال إصلاحات المرحلتين 1 و2) وتحديث أسماء الملفات والإحالات المتبادلة لتطابق الإصدار الداخلي في كل ملف.

### P3-T2 — حل ازدواجية ملف مواصفة النقود

- حذف النسخة المكررة `MONEY_DOMAIN_SPECIFICATION_v2.6.1.ar.md` أو تحويل `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md` إلى النسخة الإنجليزية الرسمية وإبقاء `.ar.md` للترجمة العربية المعتمدة، لمنع الانجراف الصامت.

### P3-T3 — استكمال الحوكمة وتجريد المتن وفق `NSM-001`

- إضافة حقول **حالة النضج** (`Approved Normative Specification` / `Review Candidate`)، و**المالك/المعتمد**، و**تاريخ الاعتماد** في ترويسة المواصفات العشر (`NSM-001 §15`).
- استكمال حقول `RFC 2119/8174` (الإصدار وتاريخ المراجعة) في `ACCOUNTING_POLICY` و`POSTING_AND_REVERSAL` و`LEDGER_PROJECTION` (`NSM-001 §4.3`).
- استكمال حقول سجل القرارات (`Alternatives considered`, `Owner/Approver`, `Revision triggers`) في ملاحق `DEC-*` (`NSM-001 §11`).
- استبدال المصطلحات اللغوية الخاصة بـ Go (`int64`, `nil`, `storage.OperationRevaluation`) بمصطلحات مجالية محايدة (`NSM-001 §13`).

---

## المرحلة 4: بوابة التحقق والاعتماد النهائي (`Verification Gate`)

تُعد الخطة مكتملةً عند استيفاء جميع الشروط التالية:

1. **اجتياز قائمة المراجعة (`NSM-001 §14`):** استيفاء جميع البنود الـ 19 في `NORMATIVE_SPECIFICATION_METHODOLOGY.md` لكل مواصفة من المواصفات التسع (بعد حل الازدواجية).
2. **صفر تناقضات متقاطعة:** تطابق تام في الأسماء والحدود والأخطاء الحارسة بين:
   - ملفات `docs/specifications/kernal/*` فيما بينها؛
   - `docs/ACCOUNTING_BUSINESS_RULES.md` و`docs/TARGET_ACCOUNTING_KERNAL.md` و`docs/KERNEL_*.md`؛
   - `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md`.
3. **إغلاق جميع البنود الـ 27:** تحديث سجل تحليل الفجوات ([KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md](./KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md)) أو إرفاق سجل إغلاق يثبت معالجة كل بند.
