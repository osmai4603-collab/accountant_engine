---
title: "تفاصيل المرحلة P7: التحصين والجودة والاعتماد النهائي — Hardening & Release"
subtitle: "حسم بوابات العزل والعقود والجاهزية التشغيلية أولاً، وتنفيذ حزمة الاختبارات كخطوة أخيرة"
document_code: "AE-PLAN-STEP-P7"
phase_id: "P7"
plan_type: "reengineering_embeddable_core"
version: "2.0.0"
date: "2026-11-20"
last_updated: "2026-11-21"
status: "منفذة — بوابة G5 NO-GO"
language: "ar"
technical_terms: "en"
---

# المرحلة P7: التحصين والجودة والاعتماد النهائي (Hardening & Release)

- **الهدف المحوري للمرحلة:** إغلاق بوابات العزل المعماري وثبات العقد العام والأدلة التشغيلية للنواة القابلة للتضمين، ثم إخضاع النتيجة النهائية لاختبارات الذرية والتزامن والأداء والتغطية، وتوقيع جاهزية الإصدار `v1.0.0-kernal`.
- **الاعتماديات العامة للمرحلة:** اكتمال واعتماد المرحلة `P6` رسمياً في [الخطة الرئيسية](00_MASTER_EXECUTION_PLAN.md).
- **بوابة اعتماد المرحلة:** إغلاق البوابات الست `G1` إلى `G6` الواردة في [ACCOUNTING_ENGINE_ACCEPTANCE_CRITERIA.md](../ACCOUNTING_ENGINE_ACCEPTANCE_CRITERIA.md) بنتيجة `GO`، مع تحقيق تغطية $\ge 80\%$، وزمن ترحيل $P95 \le 50\,\text{ms}$، وصفر سباقات بيانات، وصفر حالات حفظ جزئي.
- **النتيجة الفعلية (2026-11-21):** نُفّذت المهام الأربع والتسلسل `T0`–`T7` كاملاً؛ `G1`–`G4` و`G6` GO، لكن `G5` **`NO-GO`** (أداء + تغطية `52.7%`)؛ القرار النهائي `NO-GO`. الأدلة: [سجل البوابات](../reports/p7_gate_evidence.md) و[تقرير الأداء](../performance_baseline_report.md) و[تقرير الاعتماد](../release_signoff_v1.0.0.md).

## ترتيب التنفيذ الإلزامي

| الترتيب | المهمة | طبيعة العمل | هل تُشغّل اختبارات؟ |
| :---: | :--- | :--- | :---: |
| 1 | `P7-01` | حسم سياسة الاستيراد وتطهير فحوص العزل المعماري | لا |
| 2 | `P7-02` | تثبيت لقطة العقد العام وبوابة توافق الـ API | لا |
| 3 | `P7-03` | استكمال أدلة الجاهزية التشغيلية وحواجز القبول | لا |
| 4 | **`P7-04`** | **تنفيذ حزمة الاختبارات النهائية وإصدار شهادات الأداء والاعتماد** | **نعم — الخطوة الأخيرة** |

- **قيد تنظيمي صارم:** لا تُشغّل مهام `P7` بالتوازي، ولا تُطلق subagents متوازية أثناء بناء هذه الخطة ولا أثناء تنفيذها؛ التنفيذ تسلسلي (`P7-01` ← `P7-02` ← `P7-03` ← `P7-04`).
- **قيد اختباري صارم:** لا يُجرى أي `go test` / `-race` / benchmark قبل اكتمال `P7-01` إلى `P7-03` وإغلاق بواباتها. التفاصيل الكاملة لحزمة الاختبارات موجودة في الملف المستقل [09_PHASE_7_TEST_EXECUTION.md](09_PHASE_7_TEST_EXECUTION.md).
- **قيد تعديل صارم:** أثناء `P7-04` تُقرأ أكواد الإنتاج فقط؛ أي إصلاح عيب يستوجب العودة إلى المهمة المالكة للكود، ثم إعادة تشغيل `P7-04` من أول خطوة اختبارية.

---

## 1. مراجعة الخطة الرئيسية وسجل الفجوات

### 1.1 مراجعة [00_MASTER_EXECUTION_PLAN.md](00_MASTER_EXECUTION_PLAN.md)

| المعرّف | الملاحظة | الأثر على P7 | الإجراء المطلوب |
| :--- | :--- | :--- | :--- |
| `F-01` | سجل البوابات (§5.2) يصنّف `P2` و`P3` كـ«غير معتمدة بانتظار P1/P2»، بينما جدول حالة المهام (§2) يصنّف مهامها «مكتملة» بأدلة مفصّلة. | تعارض في سلسلة الاعتماد يمنع إثبات اكتمال `P6` شرعياً قبل `P7`. | يحدّث مالك الخطة سجل البوابات أو يرفق أدلة الاعتماد الناقصة قبل إطلاق `P7-01`. |
| `F-02` | جدول المهام يصنّف `P4`–`P6` «معلقة»، بينما الكود الفعلي موجود: `pkg/sdk/*`، `internal/orbitals/*`، و`modules/{ifrs15,ifrs16,ifrs9,ias12}` مع اختباراتها. | قد تُبنى حزمة P7 على مراحل غير موثّقة رسمياً. | تُغلق بوابات `P4`–`P6` توثيقياً أو تُحدّث حالتها إلى «كود مكتمل / بوابة الاعتماد معلّقة» قبل اعتماد P7. |
| `F-03` | كانت مهام P7 ثلاثاً فقط، والاختبارات موزّعة داخل `P7-01`/`P7-02` قبل تثبيت العقد والجاهزية. | لا يمكن إثبات أن الإصدار المُختبر هو نفسه المُعتمد. | أُضيفت `P7-04` كخطوة اختبارية أخيرة، وتُحدّث الخطة الرئيسية تبعاً لذلك. |
| `F-04` | بوابة `P1` تنص على «عدم استيراد أي internal»، بينما `scripts/verify_architecture.sh` (القاعدة `R2`) يسمح بذلك، وميثاق المبادئ `ENF-02` يمنع تعريض أنواع `internal/*` في السطح العام. | تعارض ثلاثي يمنع إغلاق بوابة `G1`. | تُحسم السياسة في `P7-01` بقرار معماري موثّق (الخيار الموصى به أدناه). |
| `F-05` | ترقيم بوابات المشروع المرجعي هو `G1`–`G12` في `docs/release_readiness_v0.2.0.md`، بينما بوابات المشروع المستهدف `G1`–`G6`. | خلط في الأدلة عند نسخ تقارير المرجع. | تُستخدم تعريفات `G1`–`G6` الخاصة بالمشروع المستهدف فقط، والمرجع يُستخدم كقالب شكلي لا كمرجع ترقيم. |

### 1.2 مراجعة المشروع المرجعي `accounting_engine` والفجوات المكتشفة

> **ملاحظة مسار:** المسار المرفق `/home/osm/StudioProjects/cashflow_system/accounting_engine/a` غير موجود على القرص. تمت المراجعة على جذر المشروع المرجعي `/home/osm/StudioProjects/cashflow_system/accounting_engine`. إذا كان المقصود فرعاً أو مجلداً آخر فيلزم تزويد مساره الصحيح لإعادة المراجعة.

| المعرّف | الفجوة في المشروع المستهدف `accountant_engine` | ما هو موجود في المرجع | المهمة المعالجة |
| :--- | :--- | :--- | :--- |
| `F-06` | لا يوجد `scripts/check_api_compat.sh` ولا `scripts/apisnapshot/` ولا `docs/api/v1.snapshot`. | `scripts/check_api_compat.sh`، `scripts/apisnapshot/main.go`، `docs/api/v1.snapshot`. | `P7-02` |
| `F-07` | `test/atomic/` و`test/acid/` و`test/benchmark/` تحتوي على `doc.go` فقط؛ لا توجد اختبارات فعلية. | اختبارات ذرية وعزل وتكافؤ وقياس أداء كاملة. | `P7-04` (الملف 09) |
| `F-08` | لا يوجد `test/concurrency/` في المشروع المستهدف. | `test/concurrency/posting_lock_test.go`. | `P7-04` (الملف 09) |
| `F-09` | بروتوكول القياس المرجعي يقيس `ns/op` فقط ولا ينتج P50/P95/P99 ولا TPS؛ وخط الأساس المرجعي يصرّح صراحة بأنه «observational, not a pass/fail latency SLO». | `test/benchmark/benchmarks_test.go`، `docs/performance/baseline_v0.2.0.md`. | `P7-03` (تعريف الحاجز) + `P7-04` (مجمّع percentiles جديد) |
| `F-10` | لا توجد أدلة `G6`: لا `test/dr/`، ولا runbooks، ولا تقرير ثبات سلسلة التدقيق، ولا `docs/release_signoff_v1.0.0.md`. | `test/dr/dr_drill_test.go`، `docs/operations/runbooks/`، `docs/release_readiness_v0.2.0.md`. | `P7-03` + `P7-04` |
| `F-11` | لا توجد بوابة تغطية: لا `coverage.out` ولا `scripts/check_coverage.sh` رغم اشتراط $\ge 80\%$. | `scripts/check_coverage.sh`. | `P7-03` (السكربت) + `P7-04` (القياس) |
| `F-12` | كان `pkg/sdk/consumer_contract_test.go` يفشل بـ `idempotency key is required` ضمن تعديلات غير ملتزمة. | حُسم اختبارياً في `P7-04`: تمرير `IdempotencyKey` صريح و`PresentationCurrency: "EUR"` في fixtures؛ `T0`/`T1` PASS. العيب الإنتاجي الكامن (`ensurePolicies`, `ReverseEntry`) ما زال مفتوحاً في سجل الفجوات النهائي. | إصلاح ضمن المهمة المالكة `P4`/`P2-06` قبل أي محاولة اعتماد جديدة. |
| `F-13` | `test/external_consumer/` وحدة Go مستقلة بـ `go.mod` خاص، لذا لا يغطيها `go test ./...` من الجذر. | نفس البنية + سكربت `check_external_adapter_consumer.sh`. | `P7-04` (تشغيل صريح منفصل) |
| `F-14` | مسار الموديول مُبقى `github.com/osmai4603-collab/accounting_engine`، وبعض أسماء الحزم اختلفت (`pkg/api/v1/subledger`, `pkg/spi/subledger`, لا يوجد `pkg/spi/legacy`). | قائمة حزم اللقطة في `check_api_compat.sh` تتضمن `pkg/spi/legacy`. | `P7-02` (تكييف قائمة الحزم) |

### 1.3 سجل فجوات التنفيذ النهائي (`P7-04`, 2026-11-21)

| المعرّف | الفجوة المكتشفة | الأثر | المالك | الحالة |
| :--- | :--- | :--- | :--- | :--- |
| `F-15` | `ensurePolicies` يرفض تطابق `PresentationCurrency` مع `FunctionalCurrency` في بعض fixtures. | احتاجت الاختبارات إلى عملة عرض `EUR`؛ عيب إنتاجي غير مُصلح. | `P2-06` | مفتوح |
| `F-16` | `ReverseEntry` لا يملأ `IdempotencyKey` تلقائياً. | احتاج المستهلك الخارجي إلى تمريره صراحةً. | `P4` | مفتوح |
| `F-17` | لا يُشحن `ReportingStore` ولا `GovernanceStore` مع المحرك. | `ErrCapabilityNotImplemented` موثّق؛ benchmark الـ cash-flow والـ year-end skipped. | `P4`/`P5` | مفتوح/موثّق |
| `F-18` | PostgreSQL يخزّن audit بصيغة v1 بينما مُدقق kernel يفحص v2. | `T7` استخدم مُدقق v1 في DR؛ لا يوجد تحقق v2 على قاعدة PostgreSQL. | `P2-07`/`P3-03` | مفتوح |
| `F-19` | Actor غير UUID يُخزّن كـ UUID بينما يُ hashed النص الأصلي. | DR استخدم UUID actors حتى يمكن التحقق بعد الاستعادة. | `P3-03` | مفتوح |
| `F-20` | قاعدة DR تتطلب `btree_gist` قبل `pg_restore`. | مطبّق في `T7` وموثّق في Runbook؛ prerequisite تشغيلي. | `P7-03`/`P3-02` | موثّق |
| `F-21` | أداء `posting_pipeline` عند `57–63 TPS` مع `P50 238–258 ms` و`P95 338–441 ms`. | `G5` NO-GO. | مالك الأداء / `P3-03` + `P4` | مفتوح |
| `F-22` | تغطية الجذر `52.7%` مقابل حد `80%`. | `G5` NO-GO. | مالك الاختبارات / `P7-04` | مفتوح |

> وفق [قاعدة الإصلاح](09_PHASE_7_TEST_EXECUTION.md#1-القواعد-الحاكمة)، أي إصلاح لهذه الفجوات يستوجب العودة إلى المهمة المالكة ثم إعادة `P7-04` كاملة من `T0`؛ لا يُوقَّع إصدار جزئي.

---

## 2. تفاصيل مهام المرحلة P7

### `P7-01` — حسم سياسة الاستيراد وتطهير فحوص العزل المعماري

- **المشكلة التي تحلها:** تعارض `F-04` بين نص بوابة `P1` وميثاق `ENF-02` وسكربت `verify_architecture.sh`، مع وجود استيرادات فعلية لـ `internal/kernal/*` من `pkg/api/v1/*` و`pkg/sdk` و`pkg/spi/masterdata` (غالباً كأسماء مستعارة `type Money = kernalTypes.Money`).
- **الهدف الذي تحققه:**
  1. اعتماد قرار معماري واحد يحدد من الذي يُسمح له باستيراد `internal/*` داخل السطح العام.
  2. تحديث `scripts/verify_architecture.sh` ليطبّق القرار آلياً بدل رسالة `R2` الحالية غير الحاسمة.
  3. إثبات نقاء النواة: خلو `internal/*` من `net/http` وgRPC وprotobuf، وخلو `internal/kernal/*` من `internal/orbitals/*` و`storage/*` و`pkg/*` و`database/sql`/`pgx`، وانعدام الدورات.
- **القرار الموصى به (يُوثّق في ARD):** يُسمح بجسر إعادة تصدير واحد فقط عبر `pkg/api/v1/*`، ويُحظر أي استيراد مباشر لـ `internal/*` من `pkg/spi/*`؛ أمّا `pkg/sdk` فيُسمح له باستيراد `internal/kernal/*` كتفصيل تنفيذي داخلي بشرط ألا يظهر أي نوع `internal/*` في توقيعاته المُصدّرة (ENF-02). وتُعدّل صياغة بوابة `P1` في الخطة الرئيسية لتقول: «عدم استيراد `pkg/spi` لأي حزمة `internal`، مع استثناء موثّق ومقصور على `pkg/api/v1`، وخلوّ التوقيعات المُصدّرة من أنواع `internal`». ويُرفق القرار بجدول زمني لهجرة الأسماء المستعارة إلى DTOs عامة إذا رُفض الاستثناء.
- **نطاق المهمة:** `scripts/verify_architecture.sh`، `docs/ard/P7_01_public_surface_decision.md`، `docs/reports/architecture_purity_report.md`، وتعديل بوابة `P1` في الخطة الرئيسية.
- **خطوات التنفيذ:**
  - **Build New:** قرار `ARD` موثّق + تقرير فحص معماري موقع يتضمن مخرجات `go list -deps ./...` و`go mod graph` ونتائج القواعد `R1`–`R6`.
  - **Copy & Modify:** تكييف القاعدة `R2` في السكربت لتفعيل حظر `pkg/spi`، والسماح المشروط لـ `pkg/api/v1`، وفحص توقيعات `pkg/sdk` المُصدّرة بحثاً عن أنواع `internal/*`.
  - **Verify & Modify:** إعادة توجيه أي استيراد شارد إلى `pkg/api/v1` أو إلى موانئ `pkg/spi/storage`، دون تغيير سلوك النواة.
- **الاعتماديات:** `P6` (مع معالجة `F-01` و`F-02`).
- **معايير القبول:**
  - [x] `./scripts/verify_architecture.sh` أخضر بالقواعد المحدّثة دون استثناءات (`T0`).
  - [x] انعدام أي مرجع `net/http` أو `google.golang.org/grpc` أو protobuf داخل `internal/**` و`go.mod` (`R1`, `R5`).
  - [x] انعدام أي استيراد `internal/*` من `pkg/spi/**` بعد التعديل، وخلوّ التوقيعات المُصدّرة في `pkg/sdk` و`pkg/api/v1` من أنواع `internal` غير مُعادة تصديرها عبر الجسر (`R2a`–`R2c`).
  - [x] تقرير العزل المعماري موقّع ومؤرخ في `docs/reports/architecture_purity_report.md`.

---

### `P7-02` — تثبيت لقطة العقد العام (API Snapshot) وبوابة توافق الواجهات

- **المشكلة التي تحلها:** غياب خط أساس للواجهات البرمجية في المشروع المستهدف (`F-06` و`F-14`)، واعتماد الخطة القديمة على سكربت غير موجود.
- **الهدف الذي تحققه:** توليد `docs/api/v1.snapshot` من الكود الفعلي، وتشغيل `scripts/check_api_compat.sh` ضده لمنع كسر غير معلن في التوافق العكسي.
- **نطاق المهمة:** `scripts/apisnapshot/main.go`، `scripts/check_api_compat.sh`، `docs/api/v1.snapshot`.
- **خطوات التنفيذ:**
  - **Copy & Modify:** نسخ `scripts/apisnapshot/main.go` و`scripts/check_api_compat.sh` من المشروع المرجعي وتكييف قائمة الحزم لتشمل فعلياً:
    `pkg/api/v1/{events,journal,ledger,reporting,subledger,types}`، `pkg/sdk`، `pkg/sdk/templates`، `pkg/spi/{application,events,masterdata,standards,storage,subledger}`، `storage/{memory,postgres}`، والحزمة الجذر.
  - **Build New:** توليد اللقطة لأول مرة عبر `./scripts/check_api_compat.sh --update` بعد انتهاء `P7-01`، ثم حذف أي إشارة إلى حزم `pkg/spi/legacy` غير الموجودة.
  - **Verify:** تشغيل `./scripts/check_api_compat.sh` بنتيجة خروج `0`، والتأكد من أن اللقطة لا تحتوي على مسارات `internal/*` إلا ضمن الاستثناء الموثّق في `P7-01`.
- **الاعتماديات:** `P7-01`.
- **معايير القبول:**
  - [x] `docs/api/v1.snapshot` موجود، مُولّد آلياً، ومطابق للعقود العامة للمشروع المستهدف.
  - [x] سكربت التوافق يمر بنسبة 100% (`T0`: no removals or signature changes) ويكشف أي تغيير مقصود (Smoke Test).
  - [x] لا توجد حزمة مرجعية غير موجودة (`pkg/spi/legacy` مستبعدة صراحةً).
  - [x] لم تُشغّل اختبارات `go test` في هذه المهمة؛ أُجّلت إلى `P7-04`.

---

### `P7-03` — استكمال أدلة الجاهزية التشغيلية وحواجز القبول (قبل الاختبارات)

- **المشكلة التي تحلها:** غياب أدلة `G6` وتعريف آلي لحواجز `G5` (`F-09`، `F-10`، `F-11`).
- **الهدف الذي تحققه:** تجهيز كل ما يلزم لكي تُترجم نتائج `P7-04` إلى قرار `GO/NO-GO` دون اجتهاد وقت التنفيذ.
- **نطاق المهمة:** `scripts/check_coverage.sh`، `docs/reports/p7_gate_definitions.md`، `docs/operations/runbooks/*`، قوالب `docs/performance_baseline_report.md` و`docs/release_signoff_v1.0.0.md`.
- **خطوات التنفيذ:**
  - **Copy & Modify:** تكييف `scripts/check_coverage.sh` من المرجع ليعمل مع `coverage.out` الناتج عن `go test -coverprofile` في جذر المشروع المستهدف.
  - **Build New:**
    - ملف تعريف الحواجز يتضمن القيم: تغطية $\ge 80\%$، $P50 \le 15\,\text{ms}$، $P95 \le 50\,\text{ms}$، حمل $1{,}000$ TPS، صفر Data Races، صفر Partial Writes، وثبات سلسلة التدقيق.
    - Runbooks التشغيلية الدنيا: `disaster_recovery_runbook.md` و`release_rollback_runbook.md` (تُستمد بنيتها من مرجع v0.2.0 مع حذف كل ما يخص الخدمة المستقلة HTTP/gRPC).
    - قوالب التقارير النهائية تُملأ لاحقاً في `P7-04` فقط.
  - **Verify:** مراجعة القوالب والقيود بلا تنفيذ أي اختبار فعلي.
- **الاعتماديات:** `P7-01`، `P7-02`.
- **معايير القبول:**
  - [x] `scripts/check_coverage.sh` قابل للتنفيذ ويقرأ العتبة $80\%$ من مكان واحد (`COVERAGE_MIN`).
  - [x] `docs/reports/p7_gate_definitions.md` يعرّف `G1`–`G6` بمخرجات قابلة للقياس وأسماء أدلة.
  - [x] Runbooks معتمدة من مالك التشغيل وتخلو من أي مكون شبكي خارج النواة.
  - [x] لم تُشغّل اختبارات في هذه المهمة؛ أُجّلت إلى `P7-04`.

---

### `P7-04` — تنفيذ حزمة الاختبارات النهائية وإصدار شهادات الأداء والاعتماد (الخطوة الأخيرة)

- **المشكلة التي تحلها:** إثبات أن النسخة المُعدّة في `P7-01`–`P7-03` هي نفسها النسخة المؤهلة للإنتاج، بعد تطبيق كل الاختبارات دفعة واحدة في نهاية الخطة.
- **الهدف الذي تحققه:** تنفيذ الذرية وحقن الأعطال، والسباق، والتكافؤ، والأداء تحت الحمل، والتغطية، ثم ملء شهادات `G1`–`G6` وتوقيع `docs/release_signoff_v1.0.0.md`.
- **نطاق المهمة:** `test/atomic/`, `test/acid/`, `test/concurrency/`, `test/benchmark/`, `test/external_consumer/`, وتقارير `docs/performance_baseline_report.md`, `docs/reports/p7_gate_evidence.md`, `docs/release_signoff_v1.0.0.md`.
- **خطوات التنفيذ:**
  - **Copy & Modify:** تكييف اختبارات المرجع وفق جدول النسخ في [09_PHASE_7_TEST_EXECUTION.md](09_PHASE_7_TEST_EXECUTION.md) مع استبدال مسارات `internal/adapters/*` و`internal/posting` القديمة بـ `storage/memory` و`storage/postgres` و`internal/kernal/*`.
  - **Build New:** مجمّع percentiles جديد (P50/P95/P99 + TPS) لأن قياسات المرجع لا تنتج percentile؛ ويُشغّل حصراً على `storage/postgres` في السيناريو المعتمد للإنتاج، ويُستبعد `storage/memory` من شهادة الأداء.
  - **Run:** تنفيذ التسلسل `T0`–`T7` المفصّل في الملف المستقل 09، بشكل تسلسلي، وتسجيل كل أمر ومخرجه.
- **الاعتماديات:** `P7-01`، `P7-02`، `P7-03` (كلها مغلقة).
- **معايير القبول:**
  - [x] `go test -race ./...` ناجح بصفر Data Races، واختبار المستهلك الخارجي يُشغّل صراحة من وحدته المستقلة (`T4`).
  - [x] صفر Partial Writes في حقن الأعطال، وتطابق تام مع تقرير تكافؤ `P3-04` (`T3`).
  - [ ] **فشل:** تقرير الأداء سجّل $P50 = 238.74–257.63\,\text{ms}$ و$P95 = 337.71–441.33\,\text{ms}$ عند $57–63$ TPS فقط؛ لم يتحقق $1{,}000$ TPS (`T5`).
  - [ ] **فشل:** التغطية $52.7\% < 80\%$ وفق `scripts/check_coverage.sh` (`T6`).
  - [x] `docs/release_signoff_v1.0.0.md` سُجّل فيه قرار `NO-GO` مع قائمة الموانع والأدلة.

---

## 3. مصفوفة الأدلة والبوابات `G1`–`G6`

| البوابة | التعريف المعتمد | الدليل النهائي | المنتِج |
| :--- | :--- | :--- | :--- |
| `G1` — Architecture & API | استقلالية النواة + ثبات العقود | `architecture_purity_report.md`، `v1.snapshot`، نتيجة `check_api_compat.sh` | `P7-01`, `P7-02`, `T0`/`T1` |
| `G2` — Accounting Integrity | توازن + ذرية + تراجع كامل | `test/atomic`، `test/acid`، تقرير `p7_gate_evidence.md` | `P7-04`/`T3` |
| `G3` — Multi-Tenant Isolation | RLS وعزل الشركات | أدلة `P3-02/03` + فحص إعادة تشغيل على PostgreSQL | `P7-04`/`T2` |
| `G4` — Functional Coverage | المعايير والوحدات المدارية | اختبارات `modules/*` و`internal/orbitals/*` والمستهلك الخارجي | `P7-04`/`T2` |
| `G5` — Performance & Concurrency | $P95 \le 50\,\text{ms}$، تغطية $\ge 80\%$، صفر سباقات | `performance_baseline_report.md`، `coverage.out`، سجل `-race` | `P7-04`/`T4`–`T6` |
| `G6` — Operations & Recovery | DR، ثبات سلسلة التدقيق، Runbooks | `test/dr`، Runbooks، تقرير سلامة Audit Chain، `release_signoff` | `P7-03` + `P7-04`/`T7` |

## 4. سجل بوابة اعتماد المرحلة P7

- **تاريخ الفحص المستهدف:** فور إغلاق `P7-01` إلى `P7-04` بالترتيب، وقبل أي إعلان عن الإصدار.
- **الأدلة المطلوبة:** تقرير فحص العزل المعماري + قرار `ARD`، لقطة `v1.snapshot` ونتيجة فحص التوافق، تعريف الحواجز والـ Runbooks، سجل الاختبارات (Race/Atomicity/Parity/External Consumer)، تقرير أداء بـ percentiles، تقرير تغطية، مصفوفة `G1`–`G6`، وتقرير الاعتماد الموقّع.
- **قاعدة الإغلاق:** أي `NO-GO` في أي بوابة يُعيد الخطة إلى المهمة المالكة، ثم تُعاد الخطوة الاختبارية `P7-04` كاملة؛ ولا يُوقّع الإصدار جزئياً.

### 4.1 نتيجة الفحص الفعلي (2026-11-21)

| البوابة | النتيجة | الدليل | ملاحظة |
| :--- | :---: | :--- | :--- |
| `G1` | `GO` | `T0` + `T1` + `architecture_purity_report.md` + `api/v1.snapshot` | عزل معماري وتوافق API ناجحان |
| `G2` | `GO` | `T3` + `contract_parity_report.md` | صفر Partial Writes؛ reversal/idempotency ناجحة |
| `G3` | `GO` | `T2` + `T7` | RLS/PostgreSQL وإعادة تشغيل على القاعدة المستعادة |
| `G4` | `GO` | `T2` + `T4` + external consumer | modules/orbitals والمستهلك الخارجي |
| `G5` | **`NO-GO`** | `T4` + `T5` + `T6` | صفر races، لكن الأداء والتغطية خارج العتبات |
| `G6` | `GO` (تشغيلياً) | `T7` + Runbooks | DR وaudit chain و`RPO=0` ناجحة |

- **القرار الإجمالي:** `NO-GO`؛ لا يُعتمد `v1.0.0-kernal`. انظر [تقرير الاعتماد](../release_signoff_v1.0.0.md) و[سجل الأدلة](../reports/p7_gate_evidence.md).
