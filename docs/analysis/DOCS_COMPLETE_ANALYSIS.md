# التحليل الشامل لوثائق مجلد `docs/`

**نوع المستند:** تقرير تحليلي مستقل (جرد وظيفي + خريطة علاقات + حالة راهنة)
**تاريخ التحليل:** 2026-11-27
**النطاق:** جميع الملفات الموجودة داخل `docs/` عند بدء القراءة، مع فحص السكربتات والأكواد التي تستخدمها (`scripts/`, `test/`) وحالة Git.
**ملاحظة منهجية:** لمُجرَ أي تعديل على ملفات المشروع أثناء هذا التحليل؛ التعديلات الظاهرة في `git status` كانت موجودة مسبقاً. هذا الملف نفسه هو الإضافة الوحيدة.
**تنبيه مرجعي:** هذا التقرير جرد وظيفي واسع؛ التقرير المعنيّ بتصنيف **تضاربات الوثائق** ومواضعها هو [`DOCS_CONFLICT_ANALYSIS.md`](DOCS_CONFLICT_ANALYSIS.md)، وهو المرجع المذكور في وثيقة الاعتماد.

---

## 0) مقدمة وتصحيح عدّي

قرأتُ محتويات `docs` بالكامل، ثم راجعتُ السكربتات والاختبارات التي تستهلك هذه الوثائق للتحقق من أدوارها العملية.

**العدد الصحيح:** عند بدء القراءة كان المجلد يحتوي **57 ملفاً**. في رسائل سابقة ذُكر الرقم 59 ووُصفت أعداد المجلدات بصورة غير دقيقة؛ التصحيح هو:

- `specifications/kernel/` = **10 ملفات** (لا 11): 9 مواصفات مجال + ملحق ISO 4217.
- `contracts/` = **11 ملفاً** (لا 10).
- أثناء التحليل اختفى الملف غير المتتبع `docs/analysis/DOCS_CONFLICT_ANALYSIS.md` من القرص (مجلد `analysis/` صار فارغاً قبل إنشاء هذا التقرير)، فصار العدد **56 ملفاً** + هذا الملف الجديد.
- مجلد `plans/` فارغ لأنه حُذف تاريخياً، وما زالت إحالاته مشطوبة في عدة وثائق.

| المجلد | عدد الملفات عند البدء | الوظيفة |
| --- | ---: | --- |
| `README.md` | 1 | فهرس التوثيق ومصدر الحقيقة لكل فئة |
| `specifications/kernel/` | 10 | **ماذا يجب أن يحدث** — مواصفات معيارية (`MUST`) |
| `contracts/` | 11 | **كيف يُنفَّذ ويُرصد** — عقود تشغيلية/تقنية |
| `architecture/` | 7 | **لماذا صُمّم هكذا** — قرارات ومعمارية وSDK |
| `rules/` | 5 | **قواعد العمل + حصر القدرات + القبول/النجاح** |
| `quality/` | 10 | **كيف يُقيَّم ويُعتمد** — معيار v2.0.0 |
| `reports/` | 8 | **ما الذي ثبت بالتنفيذ** — أدلة ونتائج |
| `operations/runbooks/` | 2 | **ماذا نفعل عند الكارثة/الإصدار** |
| `api/` | 1 | لقطة السطح العام لفحص توافق API |
| `references/` | 1 | منهجية صياغة المواصفات `NSM-001` |
| `analysis/` | 1 (اختقى) | تدقيق اتساق الوثائق |

---

## 1) الخريطة العامة: كيف تتكامل هذه الوثائق؟

```text
references/NSM-001   ← تشرح كيف تُكتب المواصفة
        ↓
specifications/kernel/*   ← "ماذا" (قواعد MUST لكل مجال نواة)
        ↓
contracts/*               ← "كيف" (ذرية، قفل، idempotency، تدقيق، تكافؤ محولين)
        ↓
architecture/*            ← "لماذا" (ADRs P1–P11، خريطة حزم مطابقة للكود، SDK)
        ↓
rules/* + quality/*       ← "بماذا نقبل وبماذا نُعتمد"
        ↓
api/v1.snapshot + reports/*  ← "الدليل" (نتائج فعلية، تقارير، أرقام)
        ↓
operations/runbooks/*     ← "التشغيل والتعافي والتراجع"
        ↓
analysis/*                ← "هل كل ما سبق متسق؟"
```

القاعدة الذهبية المتكررة: **النواة تقرر وتتحقق، والمضيف/المحول ينفّذ ويحفظ**؛ و**الوجود لا يساوي دليلاً** (اسم واجهة أو بوابة في تقرير لا يثبت اجتيازاً).

---

## 2) تفصيل كل ملف على حدة

### أ) `docs/README.md`

**ما هو:** فهرس من 20 سطراً، لكنه «البوابة التنظيمية» لكل التوثيق.

**دوره بالضبط:** يعلن لكل مجلد **مصدر الحقيقة** الخاص به، وفق `NSM-001` (قاعدة واحدة في موضع واحد). يميّز: `specifications` = المعياري، `contracts` = التشغيلي، `architecture` = القرارات، `rules` = القواعد، `quality` = المعيار v2.0.0، `reports` = الأدلة، `operations` = التشغيل، `api` = لقطة التوافق، `references` = المنهجية. سطر `plans/` مشطوب حالياً مع عبارة «حُذف — الروابط المعلّقة صُححت».

### ب) `architecture/` — 7 ملفات

| الملف | ما هو | دوره بالضبط |
| --- | --- | --- |
| `ARCHITECTURAL_PRINCIPLES_CHARTER.md` | `AE-ARCH-PRIN-001` v1.0.0، «الدستور المعماري الأعلى» | يفرض مبادئ غير قابلة للتفاوض: `PRIN-INFRA-01..04` (لا SQL/pgx داخل النواة، لا إجراءات مخزّنة إلا مشغلات WORM الحمائية)، `PRIN-INTEG-01..03` (لا HTTP/gRPC/JWT؛ استثناء `encoding/json` لـ `Money/Decimal/Currency`)، `PRIN-DOMAIN-01..03` (حياد قطاعي، لا «مريض/طن أسمنت/فاتورة بيع» داخل النواة)، `PRIN-SANCT-01..06` (قدسية القيد، لا `SetBalance`، لا `float`، لا إسقاط صامت للفروق)، و`PRIN-ENG-01..05` (Library-First، Explicit Opt-in، حتمية، لا تأثيرات جانبية عند البناء، Tamper-Evident). يضيف `ENF-01..03` ومصفوفة امتثال بآلية فحص ومسؤول لكل مبدأ. |
| `TARGET_ACCOUNTING_KERNEL.md` | `AE-ARCH-KERNEL-001` v1.1.0، الرؤية المستهدفة | يعرّف النواة كـ **Deterministic Financial Measurement Engine** مدمجة (`Embedded Go Library`): 9 حزم + جذر، مدارات (`subledgers/reporting/modules/*`)، ومصدر الحقيقة `account_moves.entry` مع `account_move_lines` كإسقاط علائقي — «لا سجل GL ثالث». يحدّد ما **لا** تملكه النواة (خوادم، مصادقة، UI، عمليات قطاعية) ويوضّح أن `CreatedBy` للتدقيق فقط لا للتفويض. الإصدار 1.1.0 صريح في تجاوز قائمة السبع حزم القديمة. |
| `KERNEL_ARCHITECTURE_MAP.md` | `AE-ARCH-MAP-P1`، الخريطة **المطابقة للكود** | مبدأها: «الحقائق من `go list` وجرد الملفات لا من الادعاءات». تعطي جدول الحزم التسع بملفاتها، عقود `Reader` (إلزامي/اختياري)، مخطط الاعتماد `L0..L4`، ومسار الحقيقة `masterdata → posting → account_moves → projection → audit`. تُغلق `G-003/004/008` وتبقي `G-016` مفتوحة (ثم أُغلقت لاحقاً بـ `P1-03`). |
| `NEW_PROJECT_STRUCTURE.md` | `AE-STRUC-NEW-001`، خريطة المشروع المادية | أين تسكن كل حزمة (`pkg/sdk`, `pkg/api/v1`, `pkg/spi`, `internal/kernel`, `internal/orbitals`, `storage/postgres`, `modules/`, `test/`)؛ سجل **الاستبعاد** (`cmd/accounting-service`, `proto/`, `internal/serviceauth`...) ولماذا؛ `go.mod` (uuid/decimal/pgx/x-text فقط)؛ قواعد الاستيراد؛ ومثال تكامل كامل بـ `sdk.NewBuilder(...).Build(ctx)`. |
| `SDK_OVERVIEW.md` | الوثيقة التمهيدية لـ `pkg/sdk` (أقسام 1–6 و12 و16) | يعرّف السطح العام الوحيد الذي يراه المضيف: 4 مسؤوليات (تركيب، تجريد، حراسة، استيعاب أحداث) و4 منعيات (لا قواعد محاسبية جديدة، لا تحقق توازن، لا قاعدة بيانات مباشرة، لا JWT). يشرح دورة `NewBuilder→preflight→Build` (5 منافذ إلزامية، اكتشاف القدرات، `errors.Join`، rollback LIFO) وخدمات `Engine` الست و`Close` بـ `sync.Once`. |
| `SDK_INTERNALS_AND_LIFECYCLE.md` | الوثيقة الداخلية (أقسام 7–11 و13–15) — «قراءة شفرة فعلية» | يفصّل `application service` كقناة إلزامية للترحيل، جسر `masterDataReaderAdapter`، مسار `Claim→Execute→Complete/Fail`، واستيعاب الأحداث (895 سطراً) ببصمة `publicEventFingerprint` (SHA-256 لحمولة حتمية)، آلة حالات الـ idempotency مع التعافي من `started` دون ترحيل مزدوج، 9 خيارات بناء، `sdk/templates` (Retail/Agriculture/SaaS)، ثم **12 ملاحظة حالة راهنة** (مثل `GetDisclosures` غير منفّذ، `YearEndClosing()` يُرجع خطأ) و8 قواعد صيانة. |
| `KERNEL_ARCHITECTURE_DECISION_RECORDS.md` | 13 سجلاً `P1-01` … `P11-04` | «الذاكرة القرارية»: لكل قرار الحالة، المالك، القرار، والبدائل المرفوضة. أهمها: `P1-02` (القيود المرحّلة مصدر الحقيقة)، `P2-01` (Scale 4/12 وBanker's)، `P7-01` (جسر إعادة تصدير واحد في `pkg/api/v1`)، `P8-01` (النواة ترتب والمحول يحجز)، `P9-01` (CLOSING وRateDate مستقل)، `P10-01` (نقطة التحقق External)، `P11-01/04` (عملية `revaluation` ومخزن السياسات المؤرخ). لا يوجد P4 ولا P6 في هذه الوثيقة. |

**ملاحظات على المعمارية:** ترقيم الطبقات مختلف بين الميثاق (`Layer1..4`) والخريطة (`L0..L4`)؛ وتعارض «API→Internal» بين `NEW_PROJECT_STRUCTURE` و`P7-01` يُحسم لصالح ADR؛ وإملاء `Kernel/Kernel` مضطرب في بعض الإحالات.

### ج) `contracts/` — 11 ملفات (طبقة «كيف يُثبَت»)

| الملف | دوره بالضبط |
| --- | --- |
| `KERNEL_POSTING_FLOW_SPECIFICATION.md` | أنبوب `PostingPipeline.PostEntry` بـ **17 خطوة (0–16)**؛ «لا مسار بديل يتخطى الخطوات 1–15، والمنفذ لا يُستدعى قبل الخطوة 16». يفرض `Clone()` دفاعياً قبل أي قراءة، يشرح بناء العكس (لا وراثة `FiscalPeriodID`)، وجدول `TOCTOU`، ويُبقي الذرية والحفظ على المضيف. |
| `KERNEL_POSTING_TRANSACTION_CONTRACT.md` | يحوّل «الذرية على عاتق المضيف» إلى 8 خطوات قابلة للاختبار: حجز المفتاح → تثبيت القيد **بتواريخ النواة** → تثبيت العكس/الأثر/التدقيق → إكمال idempotency قبل `Commit`. يمنع نمط «اكتب القيد ثم ألحق التدقيق لاحقاً»، ويفرض تفرداً على `(company,scope)` و`(company,operation,key)`. |
| `KERNEL_REVERSAL_ATOMICITY_CONTRACT.md` | وحدة عمل عكس خماسية، **فائز واحد** حتى بمفتاح مختلف (`ErrEntryAlreadyReversed`)، وجدول فشل قبل/بعد `Commit`، وترجمة أسماء `OriginalEntryID/ReversedByID` بين نموذج النواة وJSON الخاص بـ WORM حتى «لا توجد نسختان متعارضتان من الحقيقة». |
| `KERNEL_IDEMPOTENCY_CONTRACT.md` | «المفتاح وحده ليس هوية العملية»: النطاق = `CompanyID + Operation + Key`؛ بصمتان منفصلتان (`RequestFingerprint` لما حُفظ، `CallerFingerprint` لما أُرسل) مع قواعد canonical؛ حالات `started/completed/retryable_failure/terminal_failure`؛ replay يعيد نفس `EntryID` بلا كتابة؛ و**قيود متبقية صريحة**: `Lookup(companyID,key)` لا يمرّر العملية ويعتمد على بادئة `fx-revaluation | `. |
| `KERNEL_CONCURRENCY_MODEL.md` | 5 طبقات، تجميد الحقول مرة واحدة لمنع TOCTOU دون رفع العزل، قاعدة «كل من يكتب يحجز نفس الهويات بنفس الترتيب»، تمييز دقيق بين `serialization` و`منع التكرار`، سياسة إعادة محاولة المضيف، و`ErrScopeViolation` نهائي. |
| `KERNEL_LOCKING_CONTRACT.md` | يفصل **اشتقاق الترتيب** في النواة عن **الحجز الفعلي** في المحول. شكل القفل `v1 | company=... | kind=... | id=...`، أربع فئات (`account/entry/original/idempotency`)،`WithOrderedLocks` (حجز تصاعدي وتحرير عكسي)، ذاكرة بـ `sync.Map` وPostgreSQL بـ `pg_advisory_xact_lock` مع salt `0x5048415345`، و`LockObserver` اختباري فقط. |
| `KERNEL_MONETARY_PRECISION_MATRIX.md` | يثبّت `FinancialScale=4` (عرض فقط)، `InternalScale=12`، حدود `28`، حظر `float` حتى في الاختبارات بفحص AST، مصفوفة الوحدة الصغرى (`JPY=0`, `USD=2`, `KWD=3`, `CLF=4`)، تقريب نصف-إلى-زوجي، وحتمية (360 قسطاً من `1000/360` تُجمَع إلى `1000.00000000008`). |
| `KERNEL_VALUATION_RULES_MATRIX.md` | أسعار `SPOT/CLOSING/AVERAGE/HISTORICAL`، `CLOSING` إلزامي لإعادة التقييم، رفض المكرر/المستقبلي، `RateDate` مستقل، عكس إشارة الالتزامات، بصمة `fx-revaluation-payload-v2 | `، وربط بـ`storage.OperationRevaluation`. |
| `KERNEL_AUDIT_REPOSITORY_CONTRACT.md` | 6 منافذ (`AppendAuditLogAtomic`، `GetLast...`، `Iterate...`، `GetSnapshot`، `SetCheckpoint`)؛ تسلسل يبدأ من 1، `ValidateCheckpoint` حكم واحد في النواة والمحول، غياب النقطة ⇒ فشل مغلق، مصفوفة ذاكرة/PostgreSQL (RLS + WORM + RepeatableRead). |
| `KERNEL_AUDIT_THREAT_MODEL.md` | 13 تهديداً `T-01..T-13`: تغيير حقل، إعادة ترتيب، إدراج/حذف وسط، **حذف الذيل** (يُكتشف فقط بنقطة خارجية موثوقة)، تزوير نقطة، تراجع نقطة، تسريب عبر RLS (43 جدولاً)، ترحيل v1. الحكم: الضمان هو **الاكتشاف عند التحقق** لا المنع المطلق. |
| `KERNEL_CONTRACT_TEST_SUITE.md` | أكبر ملف: اختبارات محايدة عن قاعدة البيانات تكتب مرة ضد `storage.Ports` وتُشغَّل على Memory وPostgreSQL عبر `ForEachStorageAdapter`؛ تغطي المراحل 7–11 (حقن الأعطال، قفل، عزل، إعادة تقييم). يحذّر: التقارير مولّدة آلياً، وتشغيل `-test.run` جزئي يعيد كتابتها بناقص السيناريوهات. |

### د) `specifications/kernel/` — 10 ملفات («ماذا»)

كلها تحمل حالة `Review Candidate` + `Normative`، وتاريخ `2026-11-27`؛ وباستثناء `MONEY` فالإصدار `v1.2.0`.

| الملف | الإصدار/القواعد | دوره بالضبط |
| --- | --- | --- |
| `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md` | v2.6.1 / `MONEY-001..034` | أكبر ملف (827 سطراً): التمثيل العشري، العملة جزء من الهوية، 28 رقماً معنوياً و28 صحيحاً وأُس `[-28,+28]`، فشل صريح بلا بتر، `MONEY-017` (قسمة `Half Away From Zero` عند 12 خانة) مقابل `MONEY-019` (تقريب نقدي `Half-to-Even`)، تحويل بسعر موجب واتجاه صريح، JSON كسلاسل عشرية. **MONEY-035 سُحب في v2.6.1**. |
| `ISO_4217_SNAPSHOT_AND_OVERRIDES.md` | ملحق بلا إصدار | «البيانات» خلف قواعد MONEY: عملات 0/2/3/4 وحدات فرعية، `XCG` بديل `ANG`، عملات Historical، والرموز المحظورة `XXX/XTS/XAU/XAG/XPT/XPD/XDR`؛ جدول التجاوزات فارغ. |
| `IDENTIFIERS_AND_ERRORS_DOMAIN_SPECIFICATION_v1.2.0.md` | `ID-001..006` + `ERR-001..004` | طبقة «النحو» المشتركة: عدم صفرية المعرّفات قبل أي Reader، `ActorID` للتدقيق لا للتفويض، 6 رموز `CodedError`، ترجمة SQLSTATE إلى أخطاء محايدة، وتصنيف حتمي للقابلية لإعادة المحاولة مع فشل مغلق للغير مصنّف. |
| `MASTERDATA_DOMAIN_SPECIFICATION_v1.2.0.md` | `MD-001..015` | الشركة، COA الخماسي، منع Header، دورات الشجرة/مراكز التكلفة، نوافذ `[EffectiveFrom, EffectiveTo]` **مغلقة الطرفين**، اليوميات، التحقق ثنائي الفترة/السنة، `MD-013` (HARD_CLOSED مرفوض، SOFT_CLOSED بسياسة)، والفترتان 13/14 بـ `Contains()==false`. |
| `ACCOUNTING_POLICY_DOMAIN_SPECIFICATION_v1.2.0.md` | `POL-001..009` | سياسات مؤrخة بنوافذ **نصف مفتوحة** `[EffectiveDate, EffectiveTo)`، رفض التداخل، فشل مغلق بلا سياسة افتراضية، نسخ دفاعي، أهلية الحساب، تفويض الإقفال الناعم، وبصمة `policy-fingerprint-v1 | ` تستبعد رقم الإصدار. |
| `JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.2.0.md` | `JRN-001..009` | بنية `JournalEntry/EntryLine` (سطر موجب أحادي الجانب)، جدول تصنيف القيود (`Regular/Opening/Closing/Adjusting/Reversing`)، آلة `Draft→Posted→Reversed` و`Draft→Cancelled`، حظر تعديل المرحّل، و`Clone` عميق. **لا** يتولى التوازن/التقريب. |
| `POSTING_AND_REVERSAL_DOMAIN_SPECIFICATION_v1.2.0.md` | `PST-001..011` | أنبوب 8 مراحل، فشل قبل التفويض ⇒ صفر استدعاءات Executor، تطبيع وتسوية التقريب بلا إسقاط صامت، حماية TOCTOU، بناء العكس، نطاق `company | operation | key`، بصمتا posting، ومفاتيح قفل مرتّبة. يجمع كل المجالات وقت التشغيل. |
| `LEDGER_PROJECTION_DOMAIN_SPECIFICATION_v1.2.0.md` | `LDG-001..006` | **لا جداول أرصدة مادية**: الأرصدة إسقاط مشتق من `Posted/Reversed` بترتيب `(EffectiveDate, SequenceNumber, JournalEntryID)` ثم `EntryLine.ID`؛ إعادة بناء بالعملة الوظيفية **و**عملة المعاملة لـ IAS 21؛ `asOf` على `PostingDate` والعملة الفارغة تعني الوظيفية لا «الكل». |
| `FX_REVALUATION_DOMAIN_SPECIFICATION_v1.2.0.md` | `VAL-001..010` | اختيار `CLOSING`، السعر المعكوس بـ12 خانة، فقط البنود النقدية (`MD-004.IsMonetary`)، قيد `Adjusting` متوازن غير محقق، عكسه، مفتاح `(company,account,currency,dimensions,date)`، وبصمة v2 بترميز `<name>=<byte_length>:<utf8>;`. |
| `GOVERNANCE_AND_AUDIT_DOMAIN_SPECIFICATION_v1.2.0.md` | `GOV-001..007` | سلسلة إلحاقية لكل شركة، `GenesisHash`، تجزئة `v2 length-prefixed` من 12 عنصراً (الطابع مُبتَر إلى **µs**)، أفعال مسموحة (`post/reverse/revaluation/reopen_period/...`)، تحقق كامل، ونقطة خارجية بفشل مغلق يكشف حذف الذيل. |

**استثناءات مقصودة لا تناقض:** `MONEY` وحده v2.6.1؛ نوافذ السياسات نصف مفتوحة مقابل نوافذ الحسابات مغلقة؛ خمس بصمات مختلفة لكل غرض؛ رقم تسلسل التدقيق يبدأ من 1 مقابل رقم تسلسل القيد في `LDG`؛ مرادفات موثّقة (`Income≡Revenue`, `Regular≡Standard`, `ErrAccountNotCurrent≡ErrAccountNotEffective`) مؤجّلة للإزالة إلى v2.0.0.

### هـ) `rules/` — 5 ملفات

| الملف | دوره بالضبط |
| --- | --- |
| `ACCOUNTING_BUSINESS_RULES.md` | `AE-RULE-ACC-001`، **43 قاعدة** (`RULE-ACC-001..043`): توازن بدقة `0.0000`، سطر موجب أحادي الجانب، حظر `float`، تسوية التقريب الصريحة، ISO 4217/IAS 21، منع Header/الفترات المقفلة، حرمة ما بعد `POSTED`، **فصل النواة عن Maker-Checker/RBAC**، العكس، تسوية الدفاتر، إقفال السنة، معايير IFRS 15/16/9/IAS 12 (Opt-in)، وتأطير سجل التدقيق v2. |
| `ACCOUNTING_CAPABILITIES_INVENTORY.md` | 34 قدرة (11 نواة + 4 دفاتر + 6 تقارير + 2 أحداث + 2 مطابقة + 1 توحيد + 4 معايير + 4 bootstrap) مع مصفوفة تربط كل قدرة بالحزمة وتفعيلها (`Opt-in` مقابل «إلزامي دائماً»). |
| `ACCOUNTING_ENGINE_ACCEPTANCE_CRITERIA.md` | 47 معياراً + 13 شرطاً حتمياً + بوابات `G1–G6` للقبول (ISO/IEC 25010)، مع مصفوفة تحقق وموقّعين؛ «العقد التعاقدي قبل التسليم». |
| `ACCOUNTING_ENGINE_SUCCESS_CRITERIA.md` | 22 معياراً بعد التشغيل (`SC-...`): صفر اختلال، `P95≤50ms`، `1000 TPS`، `RPO=0/RTO≤30s`، دمج مضيف في ≤3 أيام — «هل تحققت المنفعة؟» لا «هل نجيز الإصدار؟». |
| `ENGINEERING_BUSINESS_RULES.md` | 27 قاعدة هندسية (`RULE-ENG-001..027`): اتجاه التبعية، 8 خطوات ذرية، idempotency، عزل RLS، ترتيب الأقفال، `ctx` أول معامل، هرم اختبارات من 4 طبقات، و`P95≤50ms`. مصادرها الأصلية (`project_specification_and_architecture.md` و`ENGINEERING_AND_ACCOUNTING_STANDARD.md`) مفقودة. |

### و) `references/` — ملف واحد

`NORMATIVE_SPECIFICATION_METHODOLOGY.md` (`NSM-001` v1.0.0): **ما وراء المواصفات**. يشرح كيف تُبحث وتُصاغ وتُراجع المواصفة: 10 مبادئ، تراتبية مصادر، سجل مصادر بـ10 حقول، 7 تصنيفات للقاعدة، خطوات حل التضارب بلا «حل صامت»، لغة `MUST/SHOULD/MAY`، تتبع ثنائي الاتجاه، 5 حالات نضج، وقاعدة ختامية: **المواصفة هي التعريف المعتمد للسلوك؛ الكود يُقيَّم مقابلها لا العكس**. يمنع صراحةً كتابة `implemented/passing/نتائج تغطية` داخل المتن المعياري — وهو المعيار الذي استُخدم لإدانة بعض الوثائق الأخرى.

### ز) `quality/` — 10 ملفات («كيف يُقيَّم»)

| الملف | دوره بالضبط |
| --- | --- |
| `ACCOUNTING_KERNEL_QUALITY_STANDARD.md` | أكبر ملف (1252 سطراً): 12 محوراً + `5.13` عابرة، **13 ثابتاً `ACC-001..013`** (أُضيف `ACC-013` التوازن الوظيفي بعد `ACC-012`)، تعريف القدرات المجاورة (Tax Posting نواة / Tax Calculation مدار)، 8 سيناريوهات حرجة، وقاعدة `Invalid Accounting States Unrepresentable`. |
| `QUALITY_GATES_SPECIFICATION.md` | 11 بوابة رسمية **`G01–G11`** بشروط Pass/Fail (مثلاً `G01` = 100% من `ACC-001..013` وصفر قيد غير متوازن قابل للترحيل؛ `G10` = صفر متطلب حرج `Not Verified`). خمس حالات: Pass/Fail/Not Verified/N/A/Waived، والـ Waiver لا يُخفى داخل Pass. |
| `EVIDENCE_AND_SCORING_GUIDE.md` | أوزان المحاور الـ12 (20/12/12/10/7/8/10/5/4/7/3/2 = 100%)، صيغة `FinalScore = Σ W×(Axis/10)`، تجميع هرمي + `Weakest-Link` للحرجة، إعادة تطبيع عند N/A، مستويات `E0..E4` (E4 لا يُمنح على جهاز المطور)، وRACI يمنع أن يعتمد صاحب المشروع تقييمه منفرداً. |
| `SCOPE_PROFILE_TEMPLATE.md` | قالب يُملأ **قبل** التقييم: 11 قدرة، بيئة هدف بـ10 أبعاد، قاعدة «Optional المعلنة كمدعومة تُقيَّم ولو كانت معطّلة». |
| `SCOPE_PROFILE_KERNEL_v1.md` | نسخة معبّأة لنواة `internal/kernel` عند `e3318e4`: 4 قدرات core، 5 مدارات، 2 خارج النطاق؛ يحوّل ACID/Locks/RLS/Auth للمضيف؛ **Independent Reviewer وApproval Authority = Pending**. يتوقف عند القسم 6 من القالب. |
| `COMPLIANCE_TEST_SUITE_INDEX.md` | مصفوفة تتبع من 24 صفاً (`ACC-001 → G01+G10 → E3 → REF-ACC-001`) + فهرس 11 مجموعة اختبار + قوائم مراجعة من 10 و35 بنداً. سجل التشغيل ما زال قالباً فارغاً. |
| `DOCUMENTATION_ARCHITECTURE_SPECIFICATION.md` | «دستور الحزمة»: 6 مبادئ (مصدر واحد، معرّفات ثابتة، تبعيات صريحة، إصدارات متسقة، بلا تغيير دلالي صامت، تتبع)، شجرة 6+README، ملكية كل وثيقة لما تملكه، و8 فحوص آلية قبل الإصدار. |
| `QUALITY_GATES_CROSSWALK.md` | أُضيف لاحقاً (غير متتبع في Git): يربط ثلاثة أنظمة ترقيم `G01–G11` الرسمي ↔ `G1–G15` التشغيلي ↔ `G1–G6` للقبول، ويأمر باستخدام `G01–G11` في أي وثيقة جديدة. الربط ليس واحداً بواحد. |
| `README.md` (داخل quality) | واجهة الحزمة وسجل تغييراتها `CHG-001..013`؛ يعلن الحالة «مرشحة للمراجعة، ليست شهادة اجتياز» وامتثال المشروع `not assessed by this documentation build`. |
| `MANIFEST.sha256` | 9 إدخالات تُفحص بـ `sha256sum -c docs/quality/MANIFEST.sha256`؛ **8 سطور OK وواحدة FAILED** (`QUALITY_GATES_CROSSWALK.md` المُعدَّل بعد تسجيل الـ hash). |

### ح) `reports/` — 8 ملفات («الدليل»)

| الملف | ما يثبته / يقيسه |
| --- | --- |
| `architecture_purity_report.md` | `./scripts/verify_architecture.sh` ⇒ **PASS** بـ11/11 قاعدة، 0 مخالفة؛ 299 تعرية داخلية سقطت بعد إعادة `pkg/spi/masterdata` إلى `pkg/api/v1/types`؛ 309 حزمة تبعية، 0 دورة. |
| `contract_parity_report.md` | مولّد آلياً: **90 PASS** = 45 سيناريو × محولين (memory/postgres)، 0 FAIL؛ الذاكرة 0–1ms وPostgreSQL 386–732ms؛ يكتبه `test/contract_parity` تلقائياً. |
| `fault_injection_report.md` | المرحلة 7: الفشل قبل `Commit` = صفر أثر، وبعده = نتيجة محفوظة (`Lookup` يعيد نفس `EntryID` والرصيد 100 لا 200)؛ 6 سيناريوهات PASS على المحولين. |
| `KERNEL_FINAL_VERIFICATION_REPORT.md` | التقرير المركزي (2026-11-22) بحالة `PASS`: gofmt/vet، `-race`، 16/16 fuzz، 8/8 طفرات، 11/11 معمارية، 90 تكافؤ، وجدول تحسين أداء (الترحيل 40.83–85.50ms ← **12.55ms**). يعترف بثلاثة أنظمة بوابات. |
| `KERNEL_RELEASE_SIGNOFF.md` | اعتماد `v1.0.0` القديم (`GO`، تغطية 92.4%). **ترويسته الآن `SUPERSEDED`** بعد تضاربات 27 نوفمبر، لكن متنه ما زال يحتفظ بقرار `GO` — تناقض داخلي. |
| `KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md` | تدقيق معياري لـ10 مواصفات مقابل المصادر الرسمية: **27 ملاحظة** (8 حرجة/عالية، 12 متوسطة، 5 حوكمة NSM-001، 2 حدودية)؛ السبب المباشر لإلغاء الاعتماد. |
| `multi_company_isolation_report.md` | 6/6 PASS؛ جدول RLS وقت التشغيل (A→B = 0 صفوف، B→A = 0 صفوف) بدور غير superuser؛ «النواة خط الدفاع الأول والمحول/RLS الثاني». |
| `performance_samples.json` | 293 بايتاً: 6000 عيّنة / 1m34 = **63.3 TPS**، `p50=238.7ms`, `p95=337.7ms`, `p99=554.8ms` — مقاس حمل مختلف عن أرقام ms/op في التقرير النهائي. |

### ط) `operations/runbooks/` — ملفان

- `disaster_recovery_runbook.md` (`RB-OPS-001`): استعادة من `pg_dump`/PITR إلى قاعدة نظيفة، **11 فحصاً إلزامياً** (`go test -v ./test/dr`) قبل إعادة الكتابة؛ `RPO=0` و`RTO<5s` داخل التمرين؛ يمنع تطبيق هجرات إضافية بعد الاستعادة.
- `release_rollback_runbook.md` (`RB-OPS-002`): ثلاثة مسارات تراجع (كود فقط / مع تغيير مخطط / بوابة API)، `pg_dump` إلزامي قبل التراجع، تفضيل `roll-forward` عند اعتماد البيانات على المخطط الجديد، واللقطة لا تُرجَّع إلا بقرار SemVer Major موثّق.

### ي) `api/v1.snapshot`

لقطة سطح عام نصية (~418KB، 1924 سطراً) من `go/types` لـ19 حزمة؛ كل سطر = رمز مُصدَّر (`package · symbol · kind · signature`). `scripts/check_api_compat.sh` يقارنها بالحالي: **الإضافة مسموحة (MINOR)، والحذف/تغيير التوقيع كسر MAJOR يفشل الفحص**، ولا تُحدَّث إلا بـ `--update`. هي الدليل المادي للبوابة `G10` ولدليل التراجع. **السطر 1064 مُفسَد** بنص عربي مقحم داخل مسار الحزمة، والفاحص يفشل اليوم بخروج 1.

### ك) `analysis/DOCS_CONFLICT_ANALYSIS.md` (كان موجوداً، اختفى أثناء العمل)

تقرير (276 سطراً، 2026-11-27) فحص 55 ملفاً وأعلن **تضارباً واسعاً يجعل قرار الاعتماد غير قابل للتصديق**: 6 findings في حالة الاعتماد، 19 رابطاً مكسوراً/8 ملفات مفقودة، 10 في idempotency، 12 داخل المواصفات، 15 بين القواعد والجودة، 8 في أرقام التقارير، 5 في حزمة الجودة. خطة علاجه `P0/P1/P2`. **جزء كبير عولج بعده** (روابط plans مشطوبة، `SUPERSEDED`، توحيد µs وv2 hash) لكن **بعضه ما زال مفتوحاً** (السطر 1064، ثلاثية البوابات، مسارات اختبارات غير موجودة). الملف لم يكن متتبعاً في Git وليس في المجلد الآن؛ هذا التقرير الجديد يحل محله كمرجع جرد شامل.

---

## 3) الحالة الراهنة: ما الذي يستحق انتباه؟

1. **23 ملفاً تحت `docs` معدّلة وغير مُودَعة**، إضافة إلى `QUALITY_GATES_CROSSWALK.md` غير متتبع. أي أن «الحالة» الحقيقية تُقرأ من الملفات لا من التقارير المؤرّخة.
2. **`check_api_compat.sh` يفشل فعلياً اليوم** بسبب سطر 1064 في `v1.snapshot` — بينما التقرير النهائي والاعتماد القديم يدّعيان «صفر كسور».
3. **`MANIFEST.sha256` غير متزامن** مع ملف واحد (`QUALITY_GATES_CROSSWALK.md`).
4. **`KERNEL_RELEASE_SIGNOFF.md` ملغى (`SUPERSEDED`) لكن متنه يقول `GO`**؛ البوابات مرجعية قديمة.
5. **ثلاثة أنظمة ترقيم بوابات** (`G01–G11`، `G1–G15`، `G1–G6`) رغم وجود Crosswalk؛ و**13 مسار اختبار مذكور غير موجود فعلياً**.
6. **`plans/` حُذف** وما زالت إحالات مشطوبة منتشرة؛ مصادر `ENGINEERING_BUSINESS_RULES` مفقودة؛ `SCOPE_PROFILE` و`quality/README` لا يزالان في حالة `Pending/مرشح`.
7. **تناقضات أرقام**: 90 حالة تكافؤ حديثة مقابل 42 في تقرير العزل؛ 63.3 TPS مقابل ادعاءات 12.55ms/1000 TPS؛ commits مختلفة (`94054d9`, `c28151d`, `e3318e4`, `de7e525`).
8. معظمها **مشكلات توثيقية/حوكمية** لا تعني بالضرورة أن الكود خاطئ — لكنها تعني أن أي ادعاء «جاهز للإصدار» يجب إعادة التحقق منه.

---

## 4) كيف تقرأ هذه الوثائق عملياً (ترتيب مقترح)

1. `docs/README.md` → تعرف أي مجلد يملك السلطة.
2. `references/NORMATIVE_SPECIFICATION_METHODOLOGY.md` → تفهم معنى `MUST` وحالة `Review Candidate`.
3. `specifications/kernel/MONEY…` ثم باقي المواصفات العشر → تعرف السلوك المطلوب.
4. `contracts/KERNEL_POSTING_FLOW…` + `…TRANSACTION…` + `…IDEMPOTENCY…` + `…LOCKING…` → تعرف كيف يُنفَّذ فعلياً.
5. `architecture/KERNEL_ARCHITECTURE_MAP.md` + `KERNEL_ARCHITECTURE_DECISION_RECORDS.md` → تعرف أين يسكن كل شيء ولماذا.
6. `SDK_OVERVIEW.md` ثم `SDK_INTERNALS_AND_LIFECYCLE.md` → للمستهلك/المطوّر.
7. `quality/*` → للتقييم والاعتماد؛ `rules/*` → للقبول والنجاح.
8. `reports/*` + `api/v1.snapshot` → للأدلة، مع قراءة `KERNEL_SPECIFICATIONS_GAP…` قبل تصديق أي `GO`.
9. `operations/runbooks/*` → للتشغيل.

---

## 5) الخلاصة

هذه المجموعة ليست «ملاحظات عشوائية» بل منظومة حوكمة متكاملة لنواة محاسبية:

- **مواصفات** تُعرّف السلوك،
- **عقود** تُقيّد التنفيذ،
- **قرارات معمارية** تحمي الحدود،
- **قواعد ومعايير جودة** تحدد القبول،
- **تقارير ولقطة API** تقدّم الأدلة،
- **طبقة تدقيق ذاتي** (`KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md`، وسابقاً `DOCS_CONFLICT_ANALYSIS.md`) اكتشفت أن أدلة الاعتماد القديمة لم تعد كافية، وما زالت بعض إصلاحاتها ناقصة حتى اليوم.

الرسالة العملية: **لا تعتمد على عنوان «PASS» أو «GO» في وثيقة قديمة؛ تحقّق من الملف الحالي ومن الفاحص الآلي أولاً.**
