# مقارنة وثائق `internal/kernal/` بوثائق `docs/` — تضاربات وتكرار

**التاريخ:** 2026-11-27
**النطاق:** ملفات Markdown السبعة في `internal/kernal/` مقابل جميع ملفات `docs/`.
**المنهجية:** قراءة مقارنة، فحص الروابط على القرص، وتحقق آلي من أسماء الاختباسات والتغطية وأهداف Fuzz في شجرة العمل الحالية.
**الحالة:** وثيقة تحليلية. لا تُنشئ قرار اعتماد ولا تُغيّر متطلباً محاسبياً.
**ملاحظة:** شجرة العمل تحتوي تعديلات غير ملتزمة؛ الأرقام المقيسة أدناه هي حالة العمل لا حالة أي Commit مُوسوم.

---

## 1. الملخص التنفيذي

نعم، يوجد تضارب فعلي وتكرار غير ضروري. أخطر ما في المقارنة أن الطرف الذي يبدو أدق تقنياً (`kernal`) يحمل حقائق مؤرخة: حالة `G-024`، أرقام التغطية، عدد أهداف Fuzz، وأرقام التتبع. في المقابل، `docs/` حسمت حالة الاعتماد (`SUPERSEDED`) لكنها تركت فجوة في `quality/COMPLIANCE_TEST_SUITE_INDEX.md` وسجل بوابات متعدد الترقيم.

| # | الشدة | التضارب | الموضع |
| --- | --- | --- | --- |
| 1 | 🔴 | `G-024` ما زال مفتوحاً في النواة بينما ADR P11-01 والكود يعلنان إغلاقه | `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md:346-350`، `KERNEL_TRACEABILITY_MATRIX.md:166` ↔ `docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md:260-270`، `docs/contracts/KERNEL_VALUATION_RULES_MATRIX.md:20,28` |
| 2 | 🔴 | وثائق `kernal` تعطي PASS/معتمد بينما قرار الإصدار مُلغى | `KERNEL_RELEASE_BASELINE.md:6,18-32`، `KERNEL_COVERAGE_EXCEPTIONS.md:6`، `KERNEL_MUTATION_REPORT.md:7`، `KERNEL_TRACEABILITY_MATRIX.md:4`، `KERNEL_COMPLETION_REPORT.md:231` ↔ `docs/reports/KERNEL_RELEASE_SIGNOFF.md:3,57`، `KERNEL_FINAL_VERIFICATION_REPORT.md:93-99` |
| 3 | 🟠 | تغطية 93.1% مقابل 92.4% بقوائم استثناءات مختلفة من نفس الأمر | `KERNEL_COMPLETION_REPORT.md:117-130` ↔ `KERNEL_RELEASE_BASELINE.md:38` و`KERNEL_COVERAGE_EXCEPTIONS.md:11-24` |
| 4 | 🟠 | Fuzz: 9 / 6 / 16 في المستندات نفسها | `KERNEL_COMPLETION_REPORT.md:99,113,178`، `KERNEL_RELEASE_BASELINE.md:21,41` ↔ `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md:38` |
| 5 | 🟠 | أربعة أنظمة بوابات: A–O وG1–G15 وG01–G11 وG1–G6 | `KERNEL_COMPLETION_REPORT.md:192-210`، `KERNEL_RELEASE_BASELINE.md:14-32`، `docs/quality/QUALITY_GATES_SPECIFICATION.md:23-35`، `docs/rules/ACCOUNTING_ENGINE_ACCEPTANCE_CRITERIA.md:135` |
| 6 | 🟠 | سبعة روابط من `kernal` إلى `docs/plans/` (مجلد فارغ) ومسارات نصية خاطئة | `KERNEL_COMPLETION_REPORT.md:3`، `KERNEL_RELEASE_BASELINE.md:9`، `KERNEL_TRACEABILITY_MATRIX.md:117,136,164,180`، `KERNEL_V12_CHANGELOG.md:3`، `KERNEL_ACCOUNTING_INVARIANTS.md:33,136,279,318` |
| 7 | 🟡 | رقم «27 قاعدة» يستخدمهkernal لـ `RULE-KERNEL` بينما 13 `ACC` و47 `RULE-ACC` في docs | `KERNEL_ACCOUNTING_INVARIANTS.md:3` ↔ `docs/quality/SCOPE_PROFILE_KERNEL_v1.md:58` و`docs/rules/ACCOUNTING_BUSINESS_RULES.md:310` |

---

## 2. جدول المقارنة حسب الملف

| ملف `kernal` | نظيره في `docs` | التصنيف |
| --- | --- | --- |
| `KERNEL_ACCOUNTING_INVARIANTS.md` | `docs/specifications/kernel/*` + `docs/contracts/*` + ADR | قواعد 001–027 ملزمة ومفيدة؛ ملاحق المراحل 2–10 (`non-normative`) تعيد شرح العقود وقد انحرفت في `G-024` |
| `KERNEL_COMPLETION_REPORT.md` | `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` + `KERNEL_RELEASE_SIGNOFF.md` | تكرار متعارض: A–O و93.1% و9 Fuzz مقابل G1–G15 و92.4% و16، مع وسم `Superseded` في السطر 3 فقط |
| `KERNEL_TRACEABILITY_MATRIX.md` | `docs/contracts/KERNEL_CONTRACT_TEST_SUITE.md` + `docs/quality/COMPLIANCE_TEST_SUITE_INDEX.md` | نظام تتبع ثالث موازٍ؛ أرقام 66/160 متجاوزة؛ حكم «0 مفقود» داخل النواة ما زال صحيحاً |
| `KERNEL_RELEASE_BASELINE.md` | `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md` + `KERNEL_RELEASE_SIGNOFF.md` | جدول G1–G15 ومقاييس مكررة شبه حرفياً بلا وسم إلغاء |
| `KERNEL_COVERAGE_EXCEPTIONS.md` | `docs/reports/KERNEL_RELEASE_SIGNOFF.md:35,49` | نفس أمر التغطية بنتيجتين مختلفتين وقائمتَي استثناءات مختلفتين |
| `KERNEL_MUTATION_REPORT.md` | `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md:39` + `KERNEL_RELEASE_SIGNOFF.md:30` | تكرار مطابق 8/8 في أربعة مستندات؛ التكرار مبرر جزئياً لكن الأفضل الإحالة إلى تقرير واحد |
| `KERNEL_V12_CHANGELOG.md` | `docs/analysis/DOCS_CONFLICT_ANALYSIS.md` | سجل تاريخي موسوم `Superseded` لكنه يحيل إلى `docs/plans/` المفقود |

---

## 3. تضاربات دلالية: من هو مصدر الحقيقة؟

| التعارض | طرف `docs` | طرف `kernal` | الحكم |
| --- | --- | --- | --- |
| كشف بتر ذيل التدقيق | `ACCOUNTING_BUSINESS_RULES.md:203-204` ينفي الكشف وحده، مقابل `:302` و`ACCEPTANCE_CRITERIA.md:73` و`CAPABILITIES_INVENTORY.md:88` يدّعون الكشف الفوري | `KERNEL_ACCOUNTING_INVARIANTS.md:375-383` يصرّح بعدم قبول بادئة صالحة كدليل على غياب الحذف | **`kernal` هو الصحيح**؛ تُصحَّح ادعاءات docs |
| Maker-Checker داخل النواة | `CAPABILITIES_INVENTORY.md:49` و`ACCEPTANCE_CRITERIA.md:51,70` مقابل `ACCOUNTING_BUSINESS_RULES.md:198` | `KERNEL_COMPLETION_REPORT.md:138` و`INVARIANTS:193-197` يضعان التفويض عند المضيف | **`kernal` متسق مع قواعد العمل** |
| أسماء أقفال الفترات | `CAPABILITIES_INVENTORY.md:66` `SOFT_LOCKED/CLOSED` مقابل `BUSINESS_RULES.md:167-169` `SOFT_CLOSED/HARD_CLOSED` | `INVARIANTS:84-85,108-109,128-129,241` تستخدم `SoftClosed/HardClosed` | **`kernal` متسق مع `BUSINESS_RULES`** |
| من يملك القفل | `NEW_PROJECT_STRUCTURE.md:81` يصف مدير أقفال داخل النواة | `INVARIANTS:282-296` و`RULE-KERNEL-027` (`TRACEABILITY:41`) يؤكدان أن النواة ترتب فقط | **`kernal` متسق مع ADR P8-01 و`KERNEL_LOCKING_CONTRACT.md`** |
| إشارة `TransactionAmount` | `JOURNAL_ENTRY_...:95` موجب فقط مقابل `LEDGER_PROJECTION_...:94` موجب/سالب | لا نص حاسم في `INVARIANTS` | يلزم ADR؛ لا يُحتذى بـ `kernal` هنا |
| `G-024` | ADR P11-01 ومصفوفة التقييم تقولان مغلق | `INVARIANTS:346-350` و`TRACEABILITY:166` تقولان مفتوح | **`docs` والكود هما الصحيحان؛ `kernal` متأخر** |

---

## 4. التكرار غير الضروري

| التكرار | الأدلة | الإجراء |
| --- | --- | --- |
| جدول التغطية مرتين | `COMPLETION:115-130` ↔ `COVERAGE:11-24` | جدول واحد مولَّد من تشغيل موثّق، والباقي إحالة |
| جدول الطفرات أربع مرات | `MUTATION:12-36`، `COMPLETION:146-159`، `FINAL_VERIFICATION:39`، `SIGNOFF:30` | تقرير الطفرات وحده + إحالات |
| جدول البوابات أربع مرات | `COMPLETION:192-210`، `BASELINE:14-32`، `FINAL_VERIFICATION:55-73`، `SIGNOFF:18-38` | الرسمي `QUALITY_GATES_SPECIFICATION.md` (G01–G11)؛ البقية إحالة تاريخية |
| ملاحق المراحل 2–10 | `INVARIANTS:29-396` ↔ `docs/contracts/*` | الإبقاء على 27 قاعدة نصًّا ملزِماً؛ الملاحق تحيل إلى العقود بدل نسخها |
| ثلاثة فهارس اختبارات | `TRACEABILITY:11-182`، `KERNEL_CONTRACT_TEST_SUITE.md:39-92`، `COMPLIANCE_TEST_SUITE_INDEX.md:58-70` | جدول ربط واحد بين `REF-*` و`TestPhase*` وأسماء `doc_test.go` |

---

## 5. روابط مكسورة

### 5.1 من `kernal` إلى `docs/plans/` (المجلد موجود لكنه فارغ)

`COMPLETION:3`، `RELEASE_BASELINE:9`، `TRACEABILITY:117,136,164,180`، `V12_CHANGELOG:3`.

### 5.2 إحالات نصية إلى مسارات خاطئة

`INVARIANTS:33` `docs/KERNEL_MONETARY_PRECISION_MATRIX.md`، `:136` `docs/KERNEL_POSTING_FLOW_SPECIFICATION.md`، `:279` `docs/KERNEL_LOCKING_CONTRACT.md` و`docs/KERNEL_CONCURRENCY_MODEL.md`، `:318` `docs/KERNEL_VALUATION_RULES_MATRIX.md`. المسارات الصحيحة تحت `docs/contracts/`. كذلك إحالات «تقرير المرحلة N» في `:92,115,136,203,248,280,319,364` تشير إلى تقارير محذوفة؛ البديل الحالي `docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md`.

### 5.3 تحقق سلبي

روابط `#adr-p*` صالحة بفضل المراسٍ الصريحة في `KERNEL_ARCHITECTURE_DECISION_RECORDS.md`، وروابط `docs/contracts/*.md` و`docs/reports/*.md` من `kernal` موجودة على القرص. لا حاجة لتغييرها.

---

## 6. التحقق من الحالة الراهنة (شجرة العمل)

- `go test ./internal/kernal/... -count=1 -coverprofile=...` → **FAIL عابر** في أول تشغيل عند `TestPostEntryCachesRepeatedAccountReads` (`posting/pipeline_invariants_test.go:936`)، ثم **PASS 10/10** في تشغيلين متتاليين؛ التغطية الكلية المستقرة **91.8%**. التذبذب يستحق تحقيقاً منفصلاً في الكود، ولم يُعالَج ضمن هذا التقرير التحليلي.
- `journal 86.5%`، `ledger 86.8%`، `types 92.7%`، `governance 98.5%`، `policy 94.5%`، `valuation 94.1%`، `masterdata 91.7%`، `posting 90.1%`.
- عدد دوال `Fuzz...` الفعلية = **16**؛ عدد أسماء `Test/Fuzz` داخل backticks في المصفوفة = **155**؛ عدد التعريفات الفعلية = **264** (منها 16 Fuzz).
- لذلك لا يجوز استخدام 93.1% أو 92.4% أو 9/6 أو 66/160 كأدلة سارية قبل تشغيل جديد موثّق بـ Commit وRun ID.

---

## 7. ترتيب الإصلاحات ومصدر الحقيقة

### الأولوية

1. **P0** — حسم `G-024` في `INVARIANTS` و`TRACEABILITY` وفق ADR P11-01.
2. **P0** — وسم ملفات `kernal` غير الموسومة كتاريخية/مُلغاة تماشياً مع قرار `SIGNOFF`.
3. **P0** — توحيد أرقام التغطية والاختبارات من تشغيل واحد، أو إزالة الادعاءات الرقمية وتسجيل حالتها الفعلية.
4. **P0** — بيان أن A–O وG1–G15 أنظمة تاريخية وأن G01–G11 هو الرسمي.
5. **P1** — إصلاح الروابط الـ7 والمسارات النصية الأربعة.
6. **P1** — تحديث 9/6 إلى 16، و66/160 إلى الأرقام المقيسة مع بيان الاستثناء.
7. **P1** — إضافة تنبيه في `INVARIANTS` بأن العقود هي مصدر الحقيقة للملاحق.
8. **P2** — دمج فهارس الاختبارات الثلاثة.

### مصدر الحقيقة عند التعارض

| المجال | المرجع الحاكم |
| --- | --- |
| السلوك المحاسبي | `docs/specifications/kernel/*` و`docs/rules/ACCOUNTING_BUSINESS_RULES.md`؛ أي تعارض يُسجَّل بـ ADR |
| قواعد النواة 001–027 | `KERNEL_ACCOUNTING_INVARIANTS.md` — القسم الملزم فقط |
| التدفق والقفل والتكرار والتدقيق | `docs/contracts/*` + `KERNEL_ARCHITECTURE_DECISION_RECORDS.md` |
| البوابات | `docs/quality/QUALITY_GATES_SPECIFICATION.md` (G01–G11) |
| النتائج والأدلة | تشغيل جديد موثّق بـ Run ID/Environment/Commit، لا تقارير مؤرخة |
| قرار الإصدار | لا يوجد `GO` ساري؛ وثيقة جديدة فقط بعد تصحيح التضاربات |

**القاعدة الحاكمة:** عند تعارض رقم بين تقريرين يُحتسب الرقم المستخرج من أمر مُثبَت في ملف واحد محدَّث، لا الأقرب زمنياً ولا الأعلى؛ وعند تعارض قاعدة بين `docs` و`kernal` يُحتكم إلى العقد/ADR ثم إلى الكود واختباره.

---

## 8. سجل التطبيق (2026-11-27)

**تم تطبيقه:**

1. `KERNEL_ACCOUNTING_INVARIANTS.md`: تنبيه ملكية الملاحق non-normative، تصحيح أربعة مسارات إلى `docs/contracts/`، استبدال إحالات «تقرير المرحلة» بـ `KERNEL_FINAL_VERIFICATION_REPORT.md`، وتحديث حالة `G-024` إلى **مغلق** وفق ADR P11-01.
2. `KERNEL_TRACEABILITY_MATRIX.md`: وسم تاريخي، تحديث `G-024`، واستبدال أربعة روابط إلى `docs/plans/` بمصادر موجودة.
3. `KERNEL_COMPLETION_REPORT.md`، `KERNEL_RELEASE_BASELINE.md`، `KERNEL_COVERAGE_EXCEPTIONS.md`، `KERNEL_MUTATION_REPORT.md`، `KERNEL_V12_CHANGELOG.md`: لافتات حالة، تصحيح الروابط المكسورة، وتمييز الأرقام التاريخية عن القيم المقيسة (16 هدف Fuzz).
4. `docs/quality/QUALITY_GATES_CROSSWALK.md`: إضافة نظام `A–O` كنظام رابع تاريخي وبيان عدم سريان قرار اعتماد.
5. `docs/quality/COMPLIANCE_TEST_SUITE_INDEX.md`: تنبيه أن حالات `Pass` في القسم 4 تاريخية/مرشحة وأن سجل التشغيل قالب فارغ؛ وتحديث `MANIFEST.sha256` تبعاً لذلك.
6. **حسم التذبذب:** السبب الحقيقي كان `TestProjectionAndRebuildEdgeCases` في `internal/kernal/ledger/projection_test.go` — كان يفترض أن السطر المُعطى الأبعاد هو `mvs[0]` بينما `ProjectPostedMovements` يرتّب الأسطر حسب `LineID` العشوائي. أُصلح الاختبار للبحث عبر `LineID`؛ تحقّق بـ 8 تشغيلات بالتغطية + 10 جولات `-shuffle=on`، كلها PASS.
7. **P2 — الفهرس الموحّد:** إنشاء `docs/quality/TEST_TRACEABILITY_INDEX.md` بربط `REF-*` ↔ `RULE-KERNEL` ↔ أسماء الاختبارات الفعلية (57 اسماً تحقّق من وجودها في `*_test.go`) ↔ سيناريوهات `TestPhase*`؛ مع إحالات من الفهارس الثلاثة القديمة إليه بدل تكرار القوائم.
8. **وثيقة القرار الجديدة:** إنشاء `docs/reports/KERNEL_RELEASE_DECISION_2026-11-27.md` بحالة **`NO-GO`** وجدول بوابات `G01–G11`، مدعومة بـ 10 ملفات أدلة في `docs/evidence/2026-11-27/`.

**تنبيهات ختامية:**

- أثناء جمع الأدلة كانت ملفات Go تُعدَّل من قِبل عامل آخر في نفس المستودع (حالات `EntryCancelled`، حقول `IsMonetary`، تعديلات `masterdata`). اختلفت بصمة الشجرة بين بداية الجمع ونهايته، فسُجّلت كل النتائج كـ «لقطة لحظية» لا كدليل قابل للإعادة من `HEAD`.
- لذلك فشل `TestPhase11_PublicLedgerBridgePreservesReferenceFieldContract` في اللقطة (`IsMonetary is silently dropped from Account`)، وتراوحت التغطية 90.7–92.2% بين اللقطات. لا يجوز منح `GO` قبل تجميد الشجرة وإعادة التشغيل.
- الوضع الحالي: `G07` و`G10` = `Fail`، و`G01` و`G03` و`G05` و`G08` و`G11` = `Not Verified`؛ بقية البوابات `Pass` كلقطة فقط.
