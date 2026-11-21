# Release Sign-off — `v1.0.0-kernal`

- **Status:** `NO-GO` — نُفّذ `T0`–`T7` كاملاً، لكن بوابة `G5` فاشلة؛ لا يُمنح اعتماد إصدار.
- **تاريخ التقييم (UTC):** `2026-11-21`
- **الإصدار المستهدف:** `v1.0.0-kernal`
- **مسار الحزمة:** `github.com/osmai4603-collab/accounting_engine`
- **الشجرة المُقيّمة:** `HEAD = 9c9f831` (`2.9`) + 62 تغييراً غير ملتزم؛ لا توجد أكواد إنتاج مُعدَّلة أثناء التنفيذ النهائي.
- **المراجع الحاكمة:** [خطة P7](execution_plan_steps/08_PHASE_7_TESTING_HARNESS_AND_RELEASE.md)، [بروتوكول الاختبارات](execution_plan_steps/09_PHASE_7_TEST_EXECUTION.md)، [تعريفات البوابات](reports/p7_gate_definitions.md)، [سجل الأدلة](reports/p7_gate_evidence.md)، [تقرير الأداء](performance_baseline_report.md).

## 1. القرار

| البوابة | التعريف | النتيجة | الدليل |
| :--- | :--- | :---: | :--- |
| `G1` | Architecture & API | `GO` | `T0` + `T1`: `verify_architecture.sh` PASS (`R1`–`R6`)، `check_api_compat.sh` PASS، external consumer ناجح |
| `G2` | Accounting Integrity | `GO` | `T3`: حقن الأعطال، التراجع الكامل، مفتاح idempotency، صفر Partial Writes + `contract_parity_report.md` |
| `G3` | Multi-Tenant Isolation | `GO` | `T2` PostgreSQL/RLS + `T7` إعادة التشغيل على القاعدة المستعادة + أدلة `P3-02/03` |
| `G4` | Functional Coverage | `GO` | `T2` modules/orbitals + `T1`/`T4` المستهلك الخارجي من وحدته المستقلة |
| `G5` | Performance & Concurrency | **`NO-GO`** | `T4` بلا races، لكن `T5`: `P50 238.74–257.63 ms`، `P95 337.71–441.33 ms`، `57–63 TPS`؛ `T6`: `52.7% < 80%` |
| `G6` | Operations & Recovery | `GO` (تشغيلياً) | `T7`: backup/restore، 3 balanced moves، 6 audit-chain records، idempotent replay، `RPO=0` |

- **القرار النهائي:** **`NO-GO`**
- **رقم الإصدار المعتمد:** `—` (لم يُعتمد أي إصدار)
- **القاعدة المطبّقة:** لا إصدار جزئي؛ أي `NO-GO` في بوابة واحدة يُعيد الخطة إلى المهمة المالكة ثم يُعيد `P7-04` كاملة من `T0`.

## 2. موانع `NO-GO`

| # | المانع | البوابة | المالك | الإجراء المطلوب |
| :---: | :--- | :---: | :--- | :--- |
| 1 | `P50` بين `238.738 ms` و`257.625 ms` مقابل حد `15 ms`؛ `P95` بين `337.707 ms` و`441.330 ms` مقابل حد `50 ms`؛ throughput بين `57.057` و`63.305 TPS` مقابل `1000 TPS`. ثلاث تكرارات فاشلة على PostgreSQL. | `G5` | مالك الأداء / `P3-03` + `P4` | تحسين مسار `posting` وقياسه مجدداً بـ `T5` بعد إصلاح ملكية العيب، ثم إعادة `P7-04` من `T0`. |
| 2 | تغطية الجذر `52.7%` مقابل الحد `80%` (`scripts/check_coverage.sh` خرج بـ `1`). | `G5` | مالك الاختبارات / `P7-04` | رفع التغطية إلى ≥ `80%` وإعادة `T6` داخل تسلسل كامل يبدأ من `T0`. |
| 3 | فحص PostgreSQL الخام نفسه أعطى `275.0 TPS`؛ البيئة الحالية لا تُثبت إمكانية الوصول إلى `1000 TPS`. | `G5` | مالك البنية التحتية / الأداء | توفير بيئة قياس مؤهلة أو توثيق قيودها قبل اعتماد أي شهادة أداء. |

## 3. قيود معروفة إضافية (لا تمنع وحدها القرار، لكنها مفتوحة)

| # | القيد | المالك | الحالة |
| :---: | :--- | :--- | :--- |
| 1 | `ensurePolicies` يرفض تطابق `PresentationCurrency` مع `FunctionalCurrency` في بعض fixtures. | `P2-06` | مفتوح؛ جرى التغلب عليه اختبارياً بعملة عرض `EUR`. |
| 2 | `ReverseEntry` لا يملأ `IdempotencyKey` تلقائياً. | `P4` | مفتوح؛ تم تمريره صراحةً من المستهلك الخارجي. |
| 3 | لا يُشحن `ReportingStore` ولا `GovernanceStore` مع المحرك؛ degradation عبر `sdk.ErrCapabilityNotImplemented` موثّق. | `P4`/`P5` | مفتوح؛ benchmark الـ cash-flow والـ year-end skipped صراحةً. |
| 4 | PostgreSQL يخزّن audit بصيغة v1 بينما محقق kernel يفحص v2؛ مُدقق DR يتحقق من v1 فقط. | `P2-07`/`P3-03` | مفتوح؛ DR نجح عبر مُدقق v1. |
| 5 | Actor غير UUID يُخزّن كـ UUID في PostgreSQL بينما يُ hashed النص الأصلي. | `P3-03` | مفتوح؛ DR استخدم UUID actors. |
| 6 | قاعدة DR تتطلب تثبيت `btree_gist` قبل `pg_restore`. | `P7-03`/`P3-02` | موثّق في Runbook وطبّقه `T7`. |

## 4. ما اكتمل فعلياً

- `P7-01`، `P7-02`، `P7-03` مغلقة بـ ARD وتقرير العزل و`v1.snapshot` وحواجز `G1`–`G6` والـ Runbooks.
- حزمة `P7-04` نُفّذت بالكامل وبترتيب `T0` ← `T7`، مع لوغات كاملة في `/tmp/opencode/p7_evidence/`.
- صفر Data Races؛ صفر Partial Writes؛ audit chain سليمة؛ DR بـ `RPO=0`.
- لم تُجرَّ أي تعديلات على أكواد الإنتاج أثناء `P7-04`؛ جميع التكييفات كانت اختبارية/توثيقية.

## 5. قواعد التوقيع

1. أي `NO-GO` في أي بوابة يعيد الخطة إلى المهمة المالكة، ثم تُعاد `P7-04` كاملة من `T0`.
2. لا يُوقَّع إصدار جزئي؛ كل البوابات الست يجب أن تكون `GO` في نفس شجرة الكود المُختبرة.
3. يُرفق بالقرار سجل الأوامر والمخرجات الكامل في `docs/reports/p7_gate_evidence.md`.

- **مالك الإصدار:** `P7-04` gate review (لم يمنح اعتماداً)
- **التوقيع:** `NO-GO` — قرار آلي مستند إلى تعريفات `G1`–`G6` ومخرجات `T0`–`T7`
- **التاريخ (UTC):** `2026-11-21`
