# سجل القرارات المعمارية للنواة (`P1`–`P11`)

> هذه الوثيقة تجمع وتوحّد سجلات القرارات المعمارية (`ADR P1-01` حتى `ADR P11-04`) التي كانت موزعة في `docs/ard/`، لتكون المرجع الموحّد للقرارات المعمارية الملزمة والبدائل المرفوضة والتبعات والأدلة التنفيذية لمحرك المحاسبة.
>
> **ملاحظة الترقيم (2026-11-27):** لا توجد وثائق `ADR P4` أو `ADR P6` مستقلة؛ محتواها مدمج في `P1` (حزم النواة/دورة حياة القيد والفترات) و`P2` (عقد الدقة المالية)، مع إحالات تنفيذية إلى `P5`/`P11` بحسب الموضوع. لا يُستنتج من غيابهما وجود قرار أُهمل.

## فهرس القرارات

| المعرف | العنوان | الحالة |
| --- | --- | --- |
| [ADR P1-01](#adr-p1-01) | خريطة حزم النواة التسع وحدود المسؤولية | مقبول |
| [ADR P1-02](#adr-p1-02) | مصدر الحقيقة المحاسبي ومسار إعادة بناء GL | مقبول |
| [ADR P1-03](#adr-p1-03) | النماذج المرجعية المكررة: `masterdata` مقابل `ledger` | مقبول |
| [ADR P2-01](#adr-p2-01) | عقد الدقة المالية وتثبيت سلوك Money/Currency | مقبول |
| [ADR P3-01](#adr-p3-01) | سلامة البيانات الرئيسية: نطاق الشركة، شجرة الحسابات، والتأريخ النافذ | مقبول |
| [ADR P5-01](#adr-p5-01) | اختيار السياسات، منع التعارض، والبصمة التاريخية للقيد | مقبول |
| [ADR P7-01](#adr-p7-01) | حدود السطح العام وسياسة استيراد `internal/*` | مقبول |
| [ADR P8-01](#adr-p8-01) | ترتيب الأقفال، Snapshot الترحيل، والعزل متعدد الشركات | مقبول |
| [ADR P9-01](#adr-p9-01) | اختيار أسعار الصرف وتكرار إعادة التقييم | مقبول |
| [ADR P10-01](#adr-p10-01) | حجز سلسلة التدقيق ونقطة التفتيش الخارجية | مقبول |
| [ADR P11-01](#adr-p11-01) | توصيل idempotency إعادة التقييم بعقد التخزين | مقبول |
| [ADR P11-02](#adr-p11-02) | الحساب النافذ والأبعاد التحليلية على السطح العام | معتمد |
| [ADR P11-03](#adr-p11-03) | دلالة `BalanceReader` بين المحولات | معتمد |
| [ADR P11-04](#adr-p11-04) | مخزن السياسات المؤرخ واختيار النسخة النافذة | معتمد |

---

<a id="adr-p1-01"></a>

## ADR P1-01 — خريطة حزم النواة التسع وحدود المسؤولية

- **الحالة:** مقبول (Accepted) — يُطبَّق بالوثائق وبفحوص النقاء المعماري القائمة.
- **التاريخ (UTC):** 2026-11-21T21:04:06Z
- **المالك:** مالك معمارية النواة
- **المرجع:** ~~خطة المعالجة، المرحلة 1~~ *(حُذف مجلد `plans/`)*، ~~خط الأساس المرحلة 0~~ *(حُذف)*، الفجوة `G-003` في ~~سجل الفجوات~~ *(حُذف)*، [ADR P7-01](#adr-p7-01)
- **يُحل محل:** القائمة المكوّنة من سبع حزم في [TARGET_ACCOUNTING_KERNEL.md](./TARGET_ACCOUNTING_KERNEL.md) (النسخة 1.0.0).

### السياق

أثبت جرد المرحلة 0 وجود **تسع حزم فعلية** تحت `internal/kernel/**` إضافة إلى حزمة جذر تحمل charters الاستيراد فقط. مخطط الاعتماد المستخرج من `go list` يثبت أن البنية DAG بلا دورات، وأن `journal` و`ledger` تؤديان أدواراً مختلفة عن `masterdata` و`posting`.

### القرار

1. **الحزم المعتمدة (تسع + حزمة جذر):**
   - `internal/kernel` (doc-only): إعلان charter الاستيراد وحفظ الثوائق الاسمية (`doc.go`, `doc_test.go`).
   - `types`: القيم والمعرفات (Money، ISO 4217، التقريب، المعرفات الصلبة، الأخطاء المشتركة) — `@foundation`.
   - `apperrors`: تصنيف أخطاء مشفر (`CodedError`, `IsCode`) للتوافق العام؛ حزمة معزولة لا تستوردها حزم النواة حالياً.
   - `journal`: تعريف القيد كـ aggregate (`JournalEntry`, `EntryLine`, `EntryStatus`, `Clone` الدفاعي) — يعتمد على `types`.
   - `ledger`: نماذج مرجعية خفيفة (DTO) للتوافق مع `pkg/api/v1/ledger` وإسقاطات قراءة حتمية (`PostedMovement`)؛ **ليس** سجل GL ولا مخزن حركة — يعتمد على `journal, types` (بعد ملحق المرحلة 4).
   - `masterdata`: مصدر الحقيقة للحسابات والشركات واليوميات والتقويم والأبعاد وReader ports — يعتمد على `types`.
   - `governance`: سلسلة التدقيق، اللقطات المتسقة، التحقق مقابل checkpoint خارجي — يعتمد على `types`.
   - `policy`: سياسات الترحيل والعملات والأهلية، نافذة زمنياً ومحصورة بالشركة — يعتمد على `journal, masterdata, types`.
   - `posting`: نقطة التحقق المركزية (`PostEntry`، التقريب، العكس، ترتيب مفاتيح الأقفال، تفويض الحفظ) — يعتمد على `journal, masterdata, policy, types`.
   - `valuation`: أسعار الصرف، التحويل، إعادة التقييم وتوليد قيد التقييم — يعتمد على `journal, posting, types`.
2. **قواعد حدود ثابتة:**
   - لا تنشأ حزمة عاشرة دون إثبات أن المسؤولية لا تنتمي إلى حزمة قائمة.
   - `journal` لا يتحقق من التوازن ولا يحفظ؛ `posting` هو المسار المركزي الوحيد للترحيل والعكس.
   - `ledger` لا يملك سجل حركات ولا أرصدة؛ دوره نقل مرجعي وإسقاط قراءة حتمي فقط.
   - `apperrors` تبقى للتوافق العام؛ لا تُحذف ولا تُوسَّع دون ADR جديد.
   - charter الاستيراد الفعلي: **stdlib + uuid + decimal + حزم kernel الأخرى فقط**.
   - حدود `pkg/api/v1` و`pkg/spi` و`pkg/sdk` تبقى كما ثبّتها [ADR P7-01](#adr-p7-01).
3. **ملحق المرحلة 4 (2026-11-22):** أُضيف `internal/kernel/ledger/projection.go` لتوفير إسقاطات قراءة حتمية مشتقة من القيود المرحلة، فأصبح اعتماد `ledger` هو `journal, types` دون إنشاء دورة أو نقل مسؤولية التخزين.

### البدائل المرفوضة

- حذف `journal` ودمجه في `posting` (يكسر التوافق ويزيل فصل التعريف عن التحقق).
- حذف `ledger` أو اعتباره مالك GL (يكسر `pkg/api/v1/ledger` أو يخلق ازدواج مصادر الحقيقة).
- إضافة حزمة `gl`/`transaction` جديدة.

---

<a id="adr-p1-02"></a>

## ADR P1-02 — مصدر الحقيقة المحاسبي ومسار إعادة بناء GL

- **الحالة:** مقبول (Accepted) — قرار معماري ملزم؛ لا يغيّر قاعدة محاسبية قائمة.
- **التاريخ (UTC):** 2026-11-21T21:04:06Z
- **المالك:** مالك معمارية النواة + مالك عقود التخزين
- **المرجع:** ~~الخطة~~ *(حُذف مجلد `plans/`)*، الفجوة `G-004` في ~~سجل الفجوات~~ *(حُذف)*، [ثوابت النواة](../../internal/kernel/KERNEL_ACCOUNTING_INVARIANTS.md)

### القرار

1. **المصدر Canonical:** **القيود المرحلة (`journal.JournalEntry` بحالتي `posted`/`reversed` مع أسطرها `EntryLine`) هي مصدر الحقيقة المحاسبي الوحيد ضمن هذه الحدود.** تُخزَّن كزوج `account_moves` + `account_move_lines` في PostgreSQL أو كقيمة `JournalEntry` في الذاكرة. الأرصدة المشتقة (`account_balances`) وسجل التدقيق وسجل idempotency ليست مصادر حقيقة للأثر المالي.
2. **الانتقالات المسموحة:** `Draft → Posted → Reversed` (عبر قيد عكسي مرتبط فقط)، ويُضاف `Draft → Cancelled` للمسودات قبل الترحيل وفق [JRN-007](../specifications/kernel/JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.2.0.md#jrn-007) (بـ `ActorID` وسبب وختم `CancelledAt`/`CancelledBy`/`CancellationReason` عبر `Cancel()`، دون أثر مالي في الأستاذ العام، ويُرفض إلغاء قيد مرحّل أو معكوس أو ملغى مسبقاً، كما تتضمن قيود `account_moves` في PostgreSQL الحالة `'cancelled'`). لا تحديث ولا حذف للقيد المرحّل (`RULE-KERNEL-011`, `RULE-KERNEL-012`).
3. **إعادة بناء GL والرصيد بالعملة الأجنبية (`LDG-003`):** قراءة القيود المرحلة (`posted`/`reversed`) مع استبعاد `draft` و`cancelled` لكل نطاق شركة، وتطبيق ترتيب كلي حتمي `(EffectiveDate UTC, SequenceNumber, JournalEntryID.String())` للقيود و`EntryLine.ID.String()` للأسطر، ثم تجميع `FunctionalDebit` و`FunctionalCredit` لكل `AccountID` ولكل عملة وظيفية عبر `RebuildAccountBalances`؛ ولأغراض إعادة التقييم (`IAS 21.23(a)`) يوفر `RebuildForeignCurrencyBalances` (والميناء الاختياري `storage.TransactionBalanceReader`) صافي رصيد عملة المعاملة الموقّع إلى جانب الرصيد الوظيفي دون تغيير دلالات `BalanceReader` القائمة (`ADR P11-03`).
4. **نطاق البيانات:** مفتاح النطاق الملزم هو `CompanyID` (`masterdata.ValidateCompanyScope`). `JournalID` يقيّد اليومية والتسلسل داخل الشركة. لا يوجد `BookID` أو `LedgerID` في النواة.
5. **ملحق المرحلة 4 (2026-11-22):** إضافة `ledger.PostedMovement` و`ledger.ProjectPostedMovements` و`ledger.RebuildAccountBalances` كإسقاط قراءة حتمي وإعادة بناء للأرصدة دون تخزين رصيد مادي داخل `internal/kernel`.
6. **ملحق المرحلة 6 (2026-11-22):** حماية `PostEntry` عبر قراءة واحدة نافذة زمنياً (`ResolvePostableAccountAsOf`)، نسخة مجمدة دفاعياً قبل أي قراءة، عكس لا يكتب على الأصل ويحسب فترة العكس من تاريخه، وضبط الوقت عبر `systemClock{}.Now()` أو `Clock` المحقون.
7. **ملحق المرحلة 7 (2026-11-22):** تختم النواة `IdempotencyScope` وبصمتي الطلب وحمولة المتصل قبل التفويض؛ وحدة عمل المحول ذرية بالكامل دون نجاح جزئي.

### البدائل المرفوضة

- جعل `ledger` مالك حركة منفصلة، أو جعل `account_balances` أو سجل التدقيق مصدراً للحقيقة، أو إدخال `BookID`/`LedgerID` دون حاجة مثبتة.

---

<a id="adr-p1-03"></a>

## ADR P1-03 — النماذج المرجعية المكررة: `masterdata` مقابل `ledger`

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-21T21:04:06Z
- **المالك:** مالك معمارية النواة + مالك السطح العام
- **المرجع:** [ADR P1-01](#adr-p1-01)، [ADR P1-02](#adr-p1-02)، [ADR P7-01](#adr-p7-01)، [ثوابت النواة 016–020](../../internal/kernel/KERNEL_ACCOUNTING_INVARIANTS.md)

### القرار

1. **مصدر الحقيقة:** `masterdata` هو المصدر المخول الوحيد لقواعد شجرة الحسابات، صلاحية الحساب، نفاذه الزمني، التقويم المالي، حالات الإقفال، والفترتين 13/14.
2. **دور `ledger`:** حزمة DTO مرجعية للتوافق مع `pkg/api/v1/ledger` فقط. لا تُستخدم للتحقق من الأهلية ولا كبديل عن `masterdata`.
3. **قواعد التحويل ومنع الانحراف:**
   - الحقول المطابقة (`ID`, `CompanyID`, `Code`, `Name`, `Type`, `NormalBalance`, `Currency`, `ParentID`, `IsReconcilable`, `IsActive`, `SubClassification`) تُنقل بالقيمة بعد التحقق من التعدادات.
   - حقول `masterdata.Account` الـ13 الإضافية (`EffectiveFrom/To`, `IsControlAccount`, `IsContraAccount`, `RequiresCostCenter`, `IsMonetary`, `IsCashEquivalent`, `IsNonCashAdjustment`, `CashFlowActivity`, `IFRSCategory`, `DisplayOrder`, `CreatedAt`, `UpdatedAt`) لا تُسقَط بصمت في سياق المجال وتُوثّق صراحةً في اختبارَي التطابق الانعكاسي (`G-016`).
   - `AccountTypeRevenue` يبقى اسماً مستعاراً لـ `AccountTypeIncome` (`"income"`).
   - `ledger.FiscalPeriod.Contains` لا تُستخدم لاكتشاف فترة الترحيل.
4. **ملحق المرحلة 3 (2026-11-22):** أُضيف `TestReferenceModelsDoNotDriftSilently` في `internal/kernel/masterdata/masterdata_integrity_test.go` للتحقق الانعكاسي الآلي من عدم انحراف حقول `ledger` عن `masterdata`.

---

<a id="adr-p2-01"></a>

## ADR P2-01 — عقد الدقة المالية وتثبيت سلوك Money/Currency

- **الحالة:** مقبول (Accepted) — تثبيت وتوثيق لسلوك قائم؛ **لا تغيير في أي قاعدة محاسبية أو توقيع عام**.
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك نواة `types` + مالك بوابة جودة النواة
- **المرجع:** [مصفوفة الدقة المالية](../contracts/KERNEL_MONETARY_PRECISION_MATRIX.md)، [ثوابت النواة](../../internal/kernel/KERNEL_ACCOUNTING_INVARIANTS.md)

### القرار

1. **ثبات القواعد:** تُبقى `RULE-KERNEL-001` حتى `RULE-KERNEL-008` كما هي دون تغيير مقياس أو سقف (`FinancialScale = 4`، `InternalScale = 12`، حدود `28`، وسقف مقياس سعر الصرف `12` خانة عشرية وفق `MONEY-025`/`MONEY-026` في `ValidateExchangeRatePrecision`).
2. **التفسيرات الملزمة:**
   - **وضع التقريب (`MONEY-017` / `MONEY-019` / `POL-009`):** `Banker's Rounding / Half-to-Even` هو الوضع **الافتراضي** للعمليات والتحويل والترحيل (`MONEY-019`)؛ بينما تستخدم القسمة الوسيطة (`Money.Divide` وانعكاس سعر الصرف عند `InternalScale = 12`) تقريب **`Half Away From Zero`** (`decimal.DivRound`) وفق `MONEY-017`. تدعم سياسة العملة (`CurrencyPolicy.RoundingMethod`) الوضع الصريح (`Half-to-Even` أو `Half-Up` وفق `MONEY-020`/`POL-009`)، وتمرر `RoundEntryWithRoundingPolicy` دالة التقريب `round` نفسها إلى `ValidateEntryDoubleEntry` لضمان التطابق، كما يفرض مسار الترحيل تسامح `<= 0.5` وحدة صغرى وتطابق الإشارة وفق `PST-004` (`decimal.Truncate` للتحقق من التمثيل فقط).
   - **لقطة ISO 4217 والاستثناءات (`MONEY-014`):** تشمل عملات مثل `XCG` (`MinorUnit = 2`) وترفض الرموز الخاصة وغير القابلة للإنفاق (`XXX`, `XTS`, `XAU`, `XAG`, `XPT`, `XPD`, `XDR`) في `Money` و`MasterData`.
   - **الكسور الداخلية:** مسموحة حتى `InternalScale` أثناء القسمة/التجميع ولا تُحفظ قبل `Rounded()` أو `RoundForPostingChecked()`.
   - **الوحدة الصغرى:** `MinorUnitsExact()` يفشل مغلقاً بـ `ErrSubMinorUnitAmount`؛ `MinorUnits()` يقرّب ويُسجّل الفروق عبر `RoundingDifference()`.
   - **المقارنة والعمليات:** مقارنة عملتين مختلفتين تُعيد `false`؛ الجمع والطرح يُعيدان `ErrCurrencyMismatch`. سعر العملة لنفسها `1` مع بقاء التحقق من الرمز والتاريخ والنوع.
   - **القيم غير قابلة للتغيير:** `Money` و`Currency` قيَم immutable.
3. **حظر Float:** يُمنع `float32` و`float64` و`decimal.NewFromFloat` في **كل** ملفات النواة بما فيها الاختبارات (مفروض بفحص AST في `internal/kernel/doc_test.go`). الواجهتان المتساهلتان `Money.RoundForPosting()` و`CurrencyMinorUnit()` تبقيان للتوافق العام فقط ويُمنع استدعاؤهما من شيفرة النواة الإنتاجية.

---

<a id="adr-p3-01"></a>

## ADR P3-01 — سلامة البيانات الرئيسية: نطاق الشركة، شجرة الحسابات، والتأريخ النافذ

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernel/masterdata`
- **المرجع:** [تقرير التحقق النهائي](../reports/KERNEL_FINAL_VERIFICATION_REPORT.md)، [قواعد الثوابت 016–020](../../internal/kernel/KERNEL_ACCOUNTING_INVARIANTS.md)

### القرار

1. **الفشل المغلق للنطاق (`RULE-KERNEL-016`):** أي `CompanyID` صفري يُرفض بـ `ErrCompanyScopeRequired` قبل أي وصول إلى Reader في جميع مسارات التحقق من النطاق.
2. **وحدة نطاق شجرة الحسابات:** لقطة COA الكاملة تنتمي لشركة واحدة؛ الأب والابن يجب أن يحملا نفس `CompanyID` وإلا يُرفض بـ `ErrAccountIdentityMismatch`.
3. **حقن الوقت:** إضافة `masterdata.Clock` و`SystemClock` و`ValidateCanPostToAccountWithClock`؛ الدالة القديمة `ValidateCanPostToAccount` تفوّض إلى `SystemClock`.
4. **منفذ النسخة النافذة:** إضافة `EffectiveAccountReader.GetAccountAsOf(ctx, companyID, accountID, asOf)` كمنفذ اختياري؛ عند غيابه يُستخدم `GetAccount` مع فحص `IsCurrent`.
5. **نافذة النفاذ (`RULE-KERNEL-017`):** رفض النوافذ المعكوسة (`EffectiveTo < EffectiveFrom`) بـ `ErrInvalidEffectiveRange`. الحدّان مغلقان و`EffectiveTo == nil` يعني نافذاً.
6. **هوية اليومية:** `ValidateJournalScope` يفحص نطاق الشركة لليومية.
7. **منع انحراف النماذج المرجعية (`G-016` — مغلقة جزئياً/أُضيف اختبار انعكاسي):** أُضيف اختبار انعكاسي آلي يربط `masterdata` و`ledger` وفق [ADR P1-03](#adr-p1-03) (`TestReferenceModelsDoNotDriftSilently`)؛ يبقى تتبع الحالات المتبقية في سجل الفجوات.

---

<a id="adr-p5-01"></a>

## ADR P5-01 — اختيار السياسات، منع التعارض، والبصمة التاريخية للقيد

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernel/policy`
- **المرجع:** [تقرير التحقق النهائي](../reports/KERNEL_FINAL_VERIFICATION_REPORT.md)، [RULE-KERNEL-010/019](../../internal/kernel/KERNEL_ACCOUNTING_INVARIANTS.md)

### القرار

1. **نافذة النفاذ نصف المفتوحة:** إضافة `EffectiveTo *time.Time` إلى `PostingPolicy` و`CurrencyPolicy` بالنافذة `[EffectiveDate, EffectiveTo)`.
2. **أولوية اختيار حتمية:** فرز النسخ لكل شركة حسب `EffectiveDate` (UTC)، واختيار النسخة التي تحقق `start <= asOf < end`.
3. **رفض التعارض والفشل المغلق:** رفض التداخل بـ `ErrPolicyOverlap`، وغياب سياسة نافذة بـ `ErrPolicyNotFound` (لا سياسة افتراضية داخل النواة).
4. **بصمة السياسة على القيد:** `journal.JournalEntry.PolicyFingerprint` تُملأ في `PostingPipeline` ببصمة `sha256:` للتمثيل القانوني الثابت لحقول السياسات المحلولة، وتدخل في `ValidateImmutability`.
5. **الأهلية والإقفال الناعم:** `CheckAccountEligibility` يرفض الحساب من شركة أخرى أو غير النافذ بـ `ErrAccountNotEffective`. قرار `SoftClosed` يظل مملوكاً لـ `masterdata` ويُمرَّر من السياسة؛ `HardClosed` ممنوع دائماً.

---

<a id="adr-p7-01"></a>

## ADR P7-01 — حدود السطح العام وسياسة استيراد `internal/*`

- **الحالة:** مقبول (Accepted) — يُطبَّق آلياً بواسطة `scripts/verify_architecture.sh`.
- **التاريخ (UTC):** 2026-11-21T02:02:11Z
- **المالك:** مالك العقود العامة / بوابة `P7`
- **المرجع:** [تقرير النقاء المعماري](../reports/architecture_purity_report.md)

### القرار

1. **جسر إعادة تصدير واحد فقط:** `pkg/api/v1/...` هي الحزمة الوحيدة المسموح لها باستيراد `internal/kernel/*` عبر أسماء مستعارة (`type X = kernel...X`).
2. **حظر مطلق على `pkg/spi/*`:** لا تُستورد أي حزمة `internal/*` من `pkg/spi/**`.
3. **سماح مشروط لـ `pkg/sdk`:** يجوز لـ `pkg/sdk` استيراد `internal/kernel/*` داخلياً بشرط ألا يظهر أي نوع من `internal/*` في توقيعاته المُصدّرة إلا المعاد تصديره عبر الجسر (`R2c`).
4. **نظافة النواة:** تحظر `R1` و`R3a`–`R3d` أي اعتماد من `internal/kernel/*` على النقل أو `internal/orbitals/*` أو `pkg/*` أو `storage/*` أو `database/sql`/`pgx`.
5. **التنفيذ الآلي:** `R2a`، `R2b`، و`R2c` عبر `scripts/internalapi/main.go -bridge=...`.
6. **خطة الهجرة إذا رُفض الاستثناء مستقبلاً:** `v1.0.x` تثبيت الجسر واللقطة في `docs/api/v1.snapshot` ← `v1.1` تعريف DTOs مستقلة ← `v1.2` تحويل توقيعات `pkg/sdk` و`pkg/spi` ← `v2.0` حذف الأسماء المستعارة.

---

<a id="adr-p8-01"></a>

## ADR P8-01 — ترتيب الأقفال، Snapshot الترحيل، والعزل متعدد الشركات

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernel/posting` + مالكو المحولات
- **المرجع:** [نموذج التزامن](../contracts/KERNEL_CONCURRENCY_MODEL.md)، [عقد القفل](../contracts/KERNEL_LOCKING_CONTRACT.md)، [تقرير العزل متعدد الشركات](../reports/multi_company_isolation_report.md)، الفجوة `G-002`

### القرار

1. **النواة ترتب ولا تُقفل:** `ExtractAndSortAccountIDs` و`OrderedAccountKeys` في `posting/lock_manager.go` ينتجان مفاتيح مرتبة حتمياً فقط؛ القفل الفعلي مسؤولية Adapter/Host.
2. **مفاتيح التزامن:** تغطي الحسابات، القيد، القيد الأصلي عند العكس، ومفتاح idempotency بصيغة `v1|company=<id>|kind=<kind>|id=<value>` مرتبة بـ UTF-8.
3. **عقد SPI محايد:** `LockTarget`, `LockKind`, `OrderLockTargets`, `WithOrderedLocks`, `PostingLockTargets` في `pkg/spi/storage`.
4. **تنفيذ المحولات:** `storage/memory` يحجز أقفالاً مسماة بالترتيب القانوني؛ `storage/postgres` يحجز أقفالاً استشارية داخل معاملة الترحيل قبل أي كتابة.
5. **Snapshot policy:** التحقق من البيانات الرئيسية والسياسات يتم مرة واحدة داخل `PostingPipeline` وتُجمّد النتيجة في القيد؛ لا TOCTOU محاسبي. PostgreSQL يعمل افتراضياً بـ `READ COMMITTED` مع خيار `IsolationRepeatableRead` / `IsolationSerializable` عبر `TxOptions`.
6. **خطأ خرق النطاق وتصنيف التزامن:** `storage.ErrScopeViolation` غير قابل لإعادة المحاولة ويُترجم في SDK إلى `ErrForbidden`. أخطاء `ErrDeadlockDetected` / `ErrSerializationFailure` / `ErrConcurrentModification` / `ErrIdempotencyInProgress` قابلة لإعادة المحاولة في المضيف فقط عبر `DefaultRetryPolicy` و`ExecuteWithRetry`.

---

<a id="adr-p9-01"></a>

## ADR P9-01 — اختيار أسعار الصرف وتكرار إعادة التقييم

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + مالك `internal/kernel/valuation` + مضيف التخزين
- **المرجع:** [مصفوفة قواعد التقييم](../contracts/KERNEL_VALUATION_RULES_MATRIX.md)، [عقد idempotency](../contracts/KERNEL_IDEMPOTENCY_CONTRACT.md)

### القرار

1. **اختيار السعر:** `RateRegistry.GetRate` يختار أحدث سعر نافذ بـ `EffectiveDate <= asOf`. تُرفض الأسعار المستقبلية وغير الموجبة والمكررة (بما فيه الزوج المعكوس) بـ `ErrDuplicateRate`. إعادة التقييم تستخدم `CLOSING`.
2. **تاريخان مستقلان:** `FXResult.RateDate` لتاريخ نفاذ السعر، و`GenerateRevaluationEntryAt` لتاريخ القيد.
3. **اتجاه الخصوم:** عكس الإشارة للحالات الالتزامية باستخدام `UnrealizedGainID` و`UnrealizedLossID` في قيد تسوية متوازن وغير صفري.
4. **نطاق التكرار وبصمة النتيجة:** `RevaluationIdempotencyScope(company, account, currency, date)` يشتق نطاق `revaluation`. `RevaluationRegistry` يحفظ بصمة SHA-256 v2 (`fx-revaluation-payload-v2|`) بترميز canonical مسبوق بالطول ومبالغ `Money.ExactString()` مع استبعاد المعرّفات والطوابع الزمنية العشوائية؛ التعارض يُرفض بـ `ErrRevaluationAlreadyApplied`.
5. **حدود الضمان والعكس:** `RevaluationRegistry` حماية داخل العملية فقط، والضمان الدائم عبر التخزين مُغلق في [ADR P11-01](#adr-p11-01). مسودة العكس `BuildRevaluationReversalAt` تترك `FiscalPeriodID` صفراً ليُحل في `PostEntry`.

---

<a id="adr-p10-01"></a>

## ADR P10-01 — حجز سلسلة التدقيق ونقطة التفتيش الخارجية

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك `internal/kernel/governance` + مالكو المحولات + مالك العمليات
- **المرجع:** [نموذج تهديد التدقيق](../contracts/KERNEL_AUDIT_THREAT_MODEL.md)، [عقد مستودع التدقيق](../contracts/KERNEL_AUDIT_REPOSITORY_CONTRACT.md)، الفجوتان `G-013` و`G-025`

### القرار

1. **شكل النقطة الموحد:** `governance.ValidateCheckpoint` هو الحكم الوحيد لصحة `AuditCheckpoint` (شركة غير صفرية، SHA-256 بطول 64، وتسلسل صفر مع هاش التكوين فقط أو تسلسل موجب مع إصدار موجب).
2. **اللقطة الصارمة والفشل المغلق:** `AuditVerificationSnapshot` يشترط `len(Logs) == SnapshotSequence == checkpoint.SequenceNumber` ومطابقة `checkpoint.LogHash`؛ غياب النقطة (`CheckpointPresent == false`) يعيد `ErrAuditCheckpointMismatch`.
3. **الإضافة الذرية وحجز النقطة:** `AppendAuditLogAtomic` يعيّن التسلسل والهاش داخل معاملة/قفل شركة واحدة. `SetExternalAuditCheckpoint` يحفظ النقطة في علاقة مستقلة (`external_audit_checkpoints` مع `FORCE ROW LEVEL SECURITY` ومنع التراجع عبر trigger في PostgreSQL).
4. **سياسة الاحتفاظ والاستعادة (مسؤولية المضيف):** نسخ النقطة إلى مخزن مستقل الأذونات، تضمين السجل والنقطة معاً في النسخ الاحتياطي والاستعادة (DR).

---

<a id="adr-p11-01"></a>

## ADR P11-01 — توصيل idempotency إعادة التقييم بعقد التخزين

- **الحالة:** مقبول (Accepted) — يغلق `G-024`
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك معمارية النواة + `posting` + `valuation` + مضيف التخزين/SDK
- **المرجع:** [عقد idempotency](../contracts/KERNEL_IDEMPOTENCY_CONTRACT.md)، [ADR P9-01](#adr-p9-01)

### القرار

1. **العملية الثالثة في SPI:** إضافة `storage.OperationRevaluation PostingOperation = "revaluation"` في `pkg/spi/storage/ports.go` دون تغيير التوقيعات أو إضافة migration.
2. **الحامل `PostOptions.Scope`:** يحترم `BuildIdempotencyScope` تلميح `opts.Scope` عند `Operation == IdempotencyRevaluation` بعد التحقق من تطابق `CompanyID` و`Key`؛ أي تلميح مزيف يُرفض بـ `ErrIdempotencyScopeRequired`.
3. **تصدير بصمة v2:** تصدير `valuation.RevaluationPayloadFingerprint(entry)` (`fx-revaluation-payload-v2|`) لاستخدامها عبر SPI دون إضافة use-case عام لإعادة التقييم في `pkg/sdk` ودون توسيع `PostEntry`.
4. **الذرية في المحولات وSDK:** تخزين النطاق والبصمة ذرياً مع القيد والتدقيق (`Action=revaluation`)، وتصنيف `ErrRevaluationAlreadyApplied` و`ErrIdempotencyPayloadConflict` كـ `ErrConflict` في SDK.

---

<a id="adr-p11-02"></a>

## ADR P11-02 — الحساب النافذ والأبعاد التحليلية على السطح العام

- **الحالة:** معتمد في المرحلة 11 — يغلق `G-019` و`G-022` ويكمل جانب المضيف من `G-020`.

### القرار

1. تعريف واجهات اختيارية في `pkg/spi/masterdata` فقط: `EffectiveAccountReader.GetAccountAsOf`، `AccountVersionWriter.SaveAccountVersion`، `CostCenterReader`/`Writer`، و`AnalyticalDimensionReader`/`Writer` باستخدام أنواع عامة من `pkg/api/v1/types`.
2. `SaveAccountVersion` يحفظ لقطة كاملة غير قابلة للتعديل بحدود UTC شاملة (`EffectiveFrom` منتصف الليل، `EffectiveTo` آخر نانوسكند) متوافقة مع `Account.IsCurrent`، مع رفض التداخل والنوافذ المعكوسة والكتابة لشركة غريبة.
3. ربط الشركة الأساسية عند أول `CreateCompany` ناجح، واستخدام `storage.WithCompanyContext` لاختيار مستأجر آخر.
4. تخزين نسخ الحسابات في `account_versions` ومراكز التكلفة في `cost_centers` والقيم التحليلية في `financial_dimension_values` داخل `WithCompanyTx`.
5. اكتشاف القدرات الاختيارية في SDK عبر type assertions على `ports.MasterData` مع الحفاظ على الفشل المغلق (`ErrDimensionReaderRequired`)، وتعريض `storage.PolicyStore` عبر `Ports.Private[PolicyStoreExtensionKey]`.

---

<a id="adr-p11-03"></a>

## ADR P11-03 — دلالة `BalanceReader` بين المحولات

- **الحالة:** معتمد في المرحلة 11 — يلغي الفرق الدلالي غير الموثق بين Memory وPostgreSQL.

### القرار

1. `asOf` يُطبَّق على `posting_date` المختوم في النواة (مع الرجوع إلى `entry_date` للصفوف القديمة بلا طابع ترحيل)؛ القيود غير المرحلة لا تُحتسب، والقيود المرحلة المعكوسة (بحالة `posted`) تُحتسب.
2. العملة الفارغة تُحل إلى العملة الوظيفية للشركة في كلا المحولين ثم تُطبَّق على خطوط القيد بعملة المعاملة (أو الوظيفية عند غيابها)؛ لا يوجد وضع «كل العملات» ضمني.
3. المبالغ المجمّعة هي `FunctionalDebit - FunctionalCredit` بالعملة الوظيفية للشركة بشكل حتمي.

---

<a id="adr-p11-04"></a>

## ADR P11-04 — مخزن السياسات المؤرخ واختيار النسخة النافذة

- **الحالة:** معتمد في المرحلة 11 — يغلق جانب المضيف من `G-020` ويُغلق `G-021`؛ أخطاء التسجيل في السياسات/التدقيق لم تعد مُبتلعة.

### القرار

1. تعريف `storage.PolicyStore` (`AppendVersion`, `ListVersions`) عبر `Ports.Private[PolicyStoreExtensionKey]` بسجل عام `PolicyVersion` غير قابل للتعديل.
2. عند وجود نسخة واحدة على الأقل للشركة يصبح المتجر مصدر الحقيقة: يحمّل SDK كل الإصدارات مرة واحدة لكل شركة ويفشل مغلقاً عند غياب نسخة تغطي تاريخ القيد (لا سياسة `2000-01-01` اصطناعية).
3. عند وجود صفر نسخة للشركة يُسمح بمسار التوافق القدامي مع إرجاع أخطاء التسجيل دون ابتلاعها، وبمجرد إلحاق أول نسخة تُمسح السياسات الاصطناعية (`ClearCompanyPolicies`) ويصبح المتجر هو المصدر الوحيد.
4. منع تداخل النوافذ في المحولين وفهرس فريد على `(company_id, policy_kind, effective_from)` في PostgreSQL، مع قفل تحميل لكل شركة لمنع data race ومسح التسجيل الجزئي عند الفشل.
5. صيغة `policy.Fingerprint` لا تتغير ولا يدخلها رقم الإصدار، وتُترجم أخطاء التضارب والغياب في SDK إلى `ErrConflict` و`ErrNotFound`.

> **إغلاق `G-021`:** أخطاء `SetPolicy`/`AppendAudit` تُعاد وتُسجَّل صراحةً ولا تُبتلع؛ إلحاق التدقيق يتم ذرياً داخل محول التخزين. لا يُبقى هذا البند «مغلقاً جزئياً».

---

<a id="adr-p12-01"></a>

## ADR P12-01 — نموذج تعدد الدفاتر (`Multi-Ledger / BookID`)

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-28
- **المالك:** مالك معمارية النواة + `ledger` + `storage` + `sdk`
- **المرجع:** `internal/kernel/ledger/doc.go`، `pkg/spi/storage/context.go`

### السياق

كانت النواة ومحولات التخزين تفرض العزل على مستوى `CompanyID` فقط، مع توثيق غياب `BookID` / `LedgerID` كقيد مؤجل في `internal/kernel/ledger/doc.go`. تتطلب المؤسسات المتعددة المعايير (مثل دفتر `IFRS` ودفتر `Statutory/Tax` لنفس الشركة) عزلاً صريحاً في القيود والأرصدة ومفاتيح Idempotency والتدقيق والتقارير دون كسر المستهلكين الأحاديي الدفتر.

### القرار

1. **مفتاح النطاق (`LedgerID` / `BookID`):** إضافة `storage.WithLedgerContext(ctx, ledgerID)` و`storage.LedgerFromContext(ctx)` في `pkg/spi/storage/context.go` بحيث يكون الدفتر الافتراضي `"primary"` عند غياب السياق، مما يحفظ التوافق الرجعي بنسبة 100% لجميع المتصلين الحاليين.
2. **عزل القيود والأرصدة والتدقيق:** كل قيد يُحفظ أو يُرحَّل ضمن `LedgerID` نشط يُوسَم بذلك الدفتر؛ قراءة القيد (`GetEntry`) وحساب الرصيد (`GetAccountBalance`) والتقارير (`ListEntriesByDateRange`) تُعزل تلقائياً حسب `LedgerID` النشط ولا تسرب بيانات دفتر آخر لنفس الشركة.
3. **نطاق `Idempotency`:** عند وجود `LedgerID` غير افتراضي في السياق، يدخل `ledger:<ledger_id>` في مفتاح/نطاق Idempotency بحيث يمكن إعادة استخدام نفس المفتاح النصي بأمان بين دفترين مستقلين للشركة نفسها دون تعارض.
4. **عدم كسر واجهات النواة:** يبقى `CompanyID` هو المفتاح الإلزامي في تواقيع النواة (`pkg/api/v1`), بينما يُمرَّر `LedgerID` عبر السياق (`context.Context`) وبذلك تبقى جميع عقود `v1.snapshot` مستقرة.

---

<a id="adr-p13-01"></a>

## ADR P13-01 — ملكية أخطاء المجال وتنظيف `types/errors.go`

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-28
- **المالك:** معمارية النواة (`types`, `masterdata`, `journal`, `posting`)
- **المرجع:** `docs/plans/KERNEL_DDD_BOUNDARY_REMEDIATION_PLAN.md` (المرحلة 1)

### القرار

1. إبقاء `internal/kernel/types/errors.go` محصوراً في أخطاء التمثيل المالي والعملات الستة فقط (`ErrCurrencyMismatch`, `ErrInvalidCurrency`, `ErrInvalidAmount`, `ErrAmountExceedsPrecision`, `ErrSubMinorUnitAmount`, `ErrNegativeAmountProhibited`).
2. نقل أخطاء الفترات والحسابات إلى `internal/kernel/masterdata/errors.go`، وأخطاء القيد ودورة حياته إلى `internal/kernel/journal/errors.go`، وأخطاء التوازن وسطوح الترحيل إلى `internal/kernel/posting/errors.go`.
3. الحفاظ على التوافق الرجعي للسطح العام في `pkg/api/v1/types/errors.go` عبر إعادة التصدير من الحزم المالكة الجديدة، مع تحديث `pkg/sdk` والمستهلكين الداخليين.

---

<a id="adr-p13-02"></a>

## ADR P13-02 — فصل `valuation` عن `posting` ونقل عقود Idempotency إلى `types`

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-28
- **المالك:** معمارية النواة (`types`, `posting`, `valuation`)
- **المرجع:** `docs/plans/KERNEL_DDD_BOUNDARY_REMEDIATION_PLAN.md` (المرحلة 2)

### القرار

1. نقل `IdempotencyOperation`, `IdempotencyScope`, وثوابت العمليات (`IdempotencyPosting`, `IdempotencyReversal`, `IdempotencyRevaluation`) و`ErrIdempotencyScopeRequired` إلى `internal/kernel/types/idempotency.go` مع الإبقاء على aliases في `posting/transaction.go`.
2. إزالة استيراد `posting` من كود الإنتاج في `internal/kernel/valuation/`، وتعريف واجهتين مجردتين `EntryValidator` و`ReversalBuilder` في `valuation` مع محولات تنفيذية (`EntryValidatorAdapter`, `ReversalBuilderAdapter`) في `posting/valuation_adapter.go`.
3. نقل `ErrRevaluationAlreadyApplied` و`ErrInvalidAdjustingEntryMetadata` الخاصة بالتقييم إلى `internal/kernel/valuation/errors.go`.

---

<a id="adr-p13-03"></a>

## ADR P13-03 — توحيد أخطاء `apperrors` مع أخطاء المجال القانونية

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-28
- **المالك:** معمارية النواة (`apperrors`, `posting`, `masterdata`, `pkg/sdk`)
- **المرجع:** `docs/plans/KERNEL_DDD_BOUNDARY_REMEDIATION_PLAN.md` (المرحلة 3)

### القرار

1. تغليف `apperrors.ErrUnbalancedEntry` للأصل `posting.ErrUnbalancedMove`، وتغليف `apperrors.ErrPeriodClosed` للأصل `masterdata.ErrFiscalPeriodClosed` عبر حقل `Cause` في `CodedError`.
2. إزالة الفحوص المكررة في `pkg/sdk/application.go` اعتماداً على `errors.Is` عبر `Unwrap()`.
