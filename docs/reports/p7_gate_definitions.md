# `P7` Gate Definitions — `v1.0.0-kernal`

- **الإصدار:** 1.0.0
- **تاريخ الاعتماد:** 2026-11-21
- **الغرض:** تعريف آلي ومسبق لبوابات `G1`–`G6` قبل تشغيل حزمة الاختبارات `P7-04`، حتى لا يُجتهد قرار `GO/NO-GO` وقت التنفيذ.
- **المرجع الحاكم:** [خطة P7](../execution_plan_steps/08_PHASE_7_TESTING_HARNESS_AND_RELEASE.md) §3 و[بروتوكول الاختبارات](../execution_plan_steps/09_PHASE_7_TEST_EXECUTION.md).
- **قاعدة الترقيم:** تُستخدم تعريفات `G1`–`G6` الخاصة بهذا المشروع فقط؛ تقارير المشروع المرجعي `G1`–`G12` قوالب شكلية لا مرجع ترقيم (`F-05`).

## الثوابت المعتمدة (مكان واحد لكل عتبة)

| الثابت | القيمة | مصدر القراءة الآلي |
| :--- | :--- | :--- |
| `COVERAGE_MIN` | `80` (%) | `scripts/check_coverage.sh` (السطر: `minimum="${COVERAGE_MIN:-80}"`) |
| `LATENCY_P50_MAX` | `15` ms | `docs/performance_baseline_report.md` + `docs/reports/p7_gate_evidence.md` |
| `LATENCY_P95_MAX` | `50` ms | كما أعلاه |
| `TARGET_TPS` | `1000` transactions/s | مجمّع percentiles في `test/benchmark` (PostgreSQL فقط) |
| `DATA_RACES` | `0` | سجل `go test -race ./...` |
| `PARTIAL_WRITES` | `0` | سجل حقن الأعطال في `test/acid` / `test/atomic` |
| `AUDIT_CHAIN` | intact | سجل `governance.VerifyAuditIntegrity` + `test/dr` |

> لا يجوز تعديل العتبات في ملفات التقارير وحدها؛ أي تغيير يستوجب تعديل الموضع المذكور وإعادة `P7-03` ثم `P7-04` كاملة.

## مصفوفة البوابات

| البوابة | التعريف القابل للقياس | الأدلة المطلوبة (أسماء الملفات) | الخطوة المُنتِجة | قرار الفشل |
| :--- | :--- | :--- | :--- | :--- |
| `G1` — Architecture & API | `verify_architecture.sh` = PASS (R1–R6)؛ `check_api_compat.sh` يخرج بـ 0؛ لا حزم مرجعية مفقودة؛ لا مسارات `internal` خارج الجسر `pkg/api/v1`. | `docs/reports/architecture_purity_report.md`، `docs/ard/P7_01_public_surface_decision.md`، `docs/api/v1.snapshot`، سجل `T0`/`T1` في `p7_gate_evidence.md` | `P7-01`، `P7-02`، `P7-04`/`T0`–`T1` | `NO-GO` يعود إلى `P7-01`/`P7-02` ثم يعاد `P7-04` |
| `G2` — Accounting Integrity | صفر قيود غير متوازنة؛ صفر حفظ جزئي بعد حقن الأعطال؛ عكس كامل (Reversal)؛ تطابق مع تقرير تكافؤ `P3-04`. | `test/atomic/*`، `test/acid/*`، سجل `T3` + `docs/reports/contract_parity_report.md` | `P7-04`/`T3` | يعود إلى مالك `P2`/`P3` |
| `G3` — Multi-Tenant Isolation | فحص RLS على PostgreSQL ناجح؛ صفر قراءة/كتابة عابرة للشركات؛ إعادة تشغيل على القاعدة المستعادة ناجحة. | سجل `T2`، مخرجات فحوص RLS في `p3_02`/`p3_03`، سجل `T7` | `P7-04`/`T2`، `T7` | يعود إلى مالك `P3` |
| `G4` — Functional Coverage | كل اختبارات `modules/*` و`internal/orbitals/*` pass؛ المستهلك الخارجي يُشغّل صراحةً من وحدته المستقلة وينجح. | سجل `T2` + `T4`، `test/external_consumer/` | `P7-04`/`T2`، `T4` | يعود إلى مالك `P4`–`P6` |
| `G5` — Performance & Concurrency | `coverage ≥ 80%`؛ `P50 ≤ 15 ms`؛ `P95 ≤ 50 ms` تحت `1000 TPS` على PostgreSQL؛ صفر Data Races؛ صفر Partial Writes. | `docs/performance_baseline_report.md`، `coverage.out`، سجل `go test -race ./...`، `docs/reports/p7_gate_evidence.md` | `P7-04`/`T4`–`T6` | يعود إلى مالك الأداء/الاختبارات |
| `G6` — Operations & Recovery | تمرين `pg_dump`/`pg_restore` ناجح؛ `VerifyAuditIntegrity` ناجح؛ Runbooks معتمدة؛ لا مكونات HTTP/gRPC داخل النواة؛ قرار التوقيع موقّع. | `test/dr/dr_drill_test.go`، `docs/operations/runbooks/*`، `docs/release_signoff_v1.0.0.md`، سجل `T7` | `P7-03` + `P7-04`/`T7` | يعود إلى مالك التشغيل |

## قواعد التشغيل

1. **التسلسل:** `T0` ← `T1` ← `T2` ← `T3` ← `T4` ← `T5` ← `T6` ← `T7`؛ لا تُشغّل خطوة قبل إغلاق سابقتها.
2. **البيئة المعتمدة للأداء:** PostgreSQL فقط (`POSTGRES_URL` أو المقبس المحلي)؛ `storage/memory` مستبعد من شهادة الأداء ويُستخدم للذكاء/التطوير فقط.
3. **المستهلك الخارجي:** وحدة Go مستقلة بـ `go.mod` خاص؛ تُشغّل صراحةً ولا يغطيها `go test ./...` من الجذر.
4. **قاعدة الإصلاح:** أي فشل في `P7-04` يُعاد إلى المهمة المالكة للكود، ثم تُعاد `P7-04` من `T0`؛ لا يُوقّع إصدار جزئي.
5. **القياس:** مجمّع percentiles جديد لأن قياسات المرجع `ns/op` فقط ولا تنتج P50/P95/P99 أو TPS (`F-09`)؛ لا يُستخدم `benchstat` لغيابه في البيئة.

## سجل قرار `P7-04` (مُملأ بعد `T0`–`T7`)

| البوابة | النتيجة | الدليل | ملاحظة |
| :--- | :---: | :--- | :--- |
| `G1` | `GO` | `T0` + `T1` + `architecture_purity_report.md` + `v1.snapshot` | `R1`–`R6` PASS؛ توافق API PASS؛ external consumer PASS |
| `G2` | `GO` | `T3` + `contract_parity_report.md` | صفر Partial Writes؛ حقن الأعطال والتراجع/idempotency ناجحة |
| `G3` | `GO` | `T2` + `T7` | RLS/PostgreSQL وإعادة تشغيل على القاعدة المستعادة ناجحة |
| `G4` | `GO` | `T2` + `T4` + external consumer | modules/orbitals والمستهلك الخارجي ناجحة |
| `G5` | **`NO-GO`** | `T4` + `T5` + `T6` | صفر races، لكن `P50/P95/TPS` فاشلة والتغطية `52.7% < 80%` |
| `G6` | `GO` (تشغيلياً) | `T7` + Runbooks | DR وaudit chain و`RPO=0` ناجحة؛ لا ترفع القرار مع `G5` فاشلة |

- **القرار الإجمالي:** `NO-GO` — انظر [سجل الأدلة](p7_gate_evidence.md) و[تقرير الاعتماد](../release_signoff_v1.0.0.md).
