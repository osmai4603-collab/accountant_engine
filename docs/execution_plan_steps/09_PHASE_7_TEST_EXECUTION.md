---
title: "تفاصيل تنفيذ حزمة اختبارات المرحلة P7 — Test Harness & Release Evidence"
subtitle: "الخطوة الأخيرة من الخطة: الذرية، السباق، التكافؤ، الأداء، التغطية، والاستعادة"
document_code: "AE-PLAN-STEP-P7-TEST"
phase_id: "P7"
task_id: "P7-04"
plan_type: "reengineering_embeddable_core"
version: "1.0.0"
date: "2026-11-21"
status: "منفذة — فشل شهادة G5 (NO-GO)"
language: "ar"
technical_terms: "en"
---

# حزمة اختبارات المرحلة P7 — التنفيذ النهائي (`P7-04`)

هذا الملف هو المرجع المستقل الوحيد لتنفيذ المهمة `P7-04`، وهي **آخر خطوة** في [خطة المرحلة P7](08_PHASE_7_TESTING_HARNESS_AND_RELEASE.md) و[الخطة الرئيسية](00_MASTER_EXECUTION_PLAN.md).

## 1. القواعد الحاكمة

1. لا يُشغّل أي أمر من هذا الملف قبل إغلاق `P7-01` و`P7-02` و`P7-03`؛ أي اختبار قبل ذلك يُعد خرقاً لترتيب الخطة.
2. التنفيذ تسلسلي (`T0` ← `T7`)، دون subagents متوازية ودون مهام P7 متزامنة.
3. لا تُعدّل أكواد الإنتاج أثناء التنفيذ؛ عند اكتشاف عيب يُوقف التسلسل، ويُصلح في المهمة المالكة، ثم يُعاد `P7-04` من `T0`.
4. `storage/memory` مخصص للتطوير والاختبار فقط، ويُستبعد تماماً من شهادة الأداء المعتمدة للإنتاج.
5. كل أمر يُنفّذ يُسجّل مع تاريخه وبيئته ومخرجه في `docs/reports/p7_gate_evidence.md`.

## 2. المتطلبات المسبقة

| المتطلب | التحقق |
| :--- | :--- |
| بوابات `P7-01`–`P7-03` مغلقة | تقرير العزل + `ARD` + `v1.snapshot` + تعريف الحواجز + Runbooks موجودة |
| شجرة عمل نظيفة أو تغييرات معروفة ومُقرّة | `git status --short` (توجد حالياً تعديلات غير ملتزمة؛ يجب حسمها قبل `T0`) |
| PostgreSQL للاختبار | `POSTGRES_URL` أو قاعدة `accounting_test` محلية؛ مخططات معزولة لكل تشغيل |
| أدوات الاستعادة | `pg_dump` و`pg_restore` متاحان لـ `T7` |
| وحدة المستهلك الخارجي | `test/external_consumer/go.mod` مع `replace ... => ../..` |
| إصلاح `F-12` | `pkg/sdk/consumer_contract_test.go` يجب أن يكون ناجحاً قبل `T0` |

## 3. جدول النسخ والتكييف من المشروع المرجعي

> المصدر: `/home/osm/StudioProjects/cashflow_system/accounting_engine` · الهدف: `/home/osm/StudioProjects/cashflow_system/accountant_engine`

| المصدر (مرجع) | الهدف (المستهدف) | التكييف الإلزامي |
| :--- | :--- | :--- |
| `test/atomic/posting_atomicity_test.go` | `test/atomic/atomicity_test.go` | استبدال `internal/adapters/memory` بـ `storage/memory`، و`internal/posting` بـ `internal/kernal/posting`؛ الإبقاء على `faultRunner` حول `storage.UnitOfWorkRunner`؛ إضافة سيناريو PostgreSQL بمخطط معزول. |
| `test/concurrency/posting_lock_test.go` | `test/concurrency/posting_lock_test.go` (جديد) | المجلد غير موجود حالياً؛ استبدال `internal/posting` بـ `internal/kernal/posting`، وربط القفل بمفتاح `(CompanyID, JournalID)`. |
| `test/acid/industry_independence_test.go`, `infrastructure_independence_test.go`, `external_consumption_test.go` | `test/acid/` | تكييف المسارات والخيارات: `sdk.WithCoreStorage` → `sdk.WithStoragePorts`؛ هذه اختبارات استقلالية وليست حقن أعطال قاعدة بيانات، لذا يُضاف سيناريو حقن الفشل في `test/atomic`. |
| `test/benchmark/benchmarks_test.go`, `benchmark_helpers_test.go` | `test/benchmark/` | تكييف مسارات `internal/*` إلى `internal/kernal/*`؛ الإبقاء على ستة أحمال العمل؛ إضافة مجمّع percentiles جديد لأن الأصل يقيس `ns/op` فقط. |
| `test/dr/dr_drill_test.go` | `test/dr/` (جديد) | تمرين `pg_dump`/`pg_restore` على قاعدة مستقلة + فحص `governance.VerifyAuditIntegrity`؛ حذف أي فحص يخص الخدمة المستقلة. |
| `docs/performance/baseline_v0.2.0.md` | `docs/performance_baseline_report.md` | **قالب فقط**؛ لا يُنسخ كدليل إنتاج لأنه يصرّح بأنه «observational, not a pass/fail latency SLO». |
| `scripts/check_coverage.sh` | `scripts/check_coverage.sh` | نُسخ وكيّف في `P7-03`؛ يُستخدم هنا فقط. |

## 4. تسلسل التنفيذ

### `T0` — فحوص ساكنة وإعادة تأكيد البوابات (قراءة فقط)

```sh
./scripts/verify_architecture.sh
./scripts/check_api_compat.sh
go vet ./...
go mod verify
go list -deps ./... >/dev/null
```

- **النتيجة المتوقعة:** خروج `0` من كل أمر، ومطابقة مخرجات العزل والتوافق لما سجّله `P7-01`/`P7-02`.
- **الدليل:** `docs/reports/p7_gate_evidence.md` (قسم `T0`).

### `T1` — الاختبارات العامة وتكافؤ التخزين والمستهلك الخارجي

```sh
go test ./... -count=1
go test ./test/contract_parity/... -count=1
(cd test/external_consumer && go test ./... -count=1)
```

- يستوجب نجاح `test/contract_parity` بنسبة 100% (المُنجز في `P3-04`) وتوليد تقرير التكافؤ عبر آلية `test/contract_parity/reporter.go`.
- وحدة `test/external_consumer` تُشغّل **صراحة** لأنها ليست ضمن `go test ./...` من الجذر (`F-13`).
- **الدليل:** سجل `T1` + تقرير التكافؤ المحدّث.

### `T2` — الوحدات المعيارية والمدارية والعزل متعدد المستأجرين

```sh
go test ./modules/... -count=1
go test ./internal/orbitals/... -count=1
POSTGRES_URL="${POSTGRES_URL:-postgres://localhost/accounting_test?sslmode=disable}" go test ./storage/postgres/... -count=1
```

- تغطي `G4` (شمول الوظائف) و`G3` (عزل RLS/الشركات) استناداً إلى أدلة `P3-02/P3-03`.
- **الدليل:** سجل `T2` في مصفوفة `G1`–`G6`.

### `T3` — الذرية وحقن الأعطال (`G2`)

```sh
go test ./test/atomic/... ./test/acid/... -count=1 -v
POSTGRES_URL="${POSTGRES_URL:?set POSTGRES_URL}" go test ./test/atomic/... -run 'Fault|Rollback|Idempotency' -count=1 -v
```

- **سيناريوهات إلزامية:** فشل `CommitPosting` بعد تخصيص التسلسل، انقطاع UoW أثناء الترحيل، تكرار مفتاح idempotency، قيد في فترة مغلقة.
- **شرط نجاح:** صفر Partial Writes، وتراجع كامل لكل الأثر المالي (sequence + draft + post + audit)، وتطابق النتيجة بين memory وPostgreSQL.

### `T4` — السباق والتزامن (`G5`)

```sh
go test -race ./... -count=1
go test -race ./test/benchmark -run '^$' -bench '^BenchmarkConcurrentPosting$' -benchtime=100ms -count=1
(cd test/external_consumer && go test -race ./... -count=1)
```

- **شرط نجاح:** Zero Data Races، صفر Deadlocks تحت 100 خيط متزامن، وصفر أخطاء قفل في `(CompanyID, JournalID)`.

### `T5` — الأداء وخط الأساس المعتمد (`G5`)

**بناء مطلوب قبل التشغيل:** `test/benchmark/latency_slo_test.go` يسجّل زمن كل عملية ترحيل عبر `pkg/sdk` على `storage/postgres`، ويحسب `P50`/`P95`/`P99` وعدد العمليات/ثانية، ويكتب العيّنات إلى `docs/reports/performance_samples.json`.

```sh
POSTGRES_URL="${POSTGRES_URL:?set POSTGRES_URL}" go test ./test/benchmark -run '^TestPostingLatencySLO$' -count=3 -v
POSTGRES_URL="${POSTGRES_URL:?set POSTGRES_URL}" go test ./test/benchmark -run '^$' -bench . -benchtime=1s -count=3
```

- **بروتوكول الحمل:** 1,000 TPS لمدة 60 ثانية على الأقل، مخطط PostgreSQL معزول، ثلاث تكرارات، مع تسجيل CPU/RAM/Go/PostgreSQL/OS في التقرير.
- **الحدود:** $P50 \le 15\,\text{ms}$ و$P95 \le 50\,\text{ms}$؛ أي تجاوز = `NO-GO` حتى لو نجحت قياسات `ns/op`.
- **المخرج:** `docs/performance_baseline_report.md` مملوء بالقيم والبيئة وطريقة إعادة الإنتاج، مع فصل واضح بين «قياس مرجعي» و«شهادة SLO».

### `T6` — التغطية (`G5`)

```sh
go test -coverprofile=coverage.out ./...
./scripts/check_coverage.sh
go tool cover -func=coverage.out | tail -1
```

- **شرط نجاح:** تغطية $\ge 80\%$ على وحدة الجذر؛ تُسجّل النسبة والأمر والرقم في `docs/reports/p7_gate_evidence.md`.

### `T7` — الاستعادة، سلسلة التدقيق، وشهادة الإصدار (`G6`)

```sh
POSTGRES_URL="${POSTGRES_URL:?set POSTGRES_URL}" go test ./test/dr/... -count=1 -v
```

- التحقق: `pg_dump -Fc` / `pg_restore` إلى قاعدة مستقلة، ثم `governance.VerifyAuditIntegrity`، ومطابقة الأرصدة مع تقرير التكافؤ، واستخدام Runbook المعتمدة من `P7-03`.
- بعد نجاح `T0`–`T7` تُملأ:
  - `docs/reports/p7_gate_evidence.md` (مصفوفة `G1`–`G6` مع `GO`/`NO-GO` لكل بوابة)؛
  - `docs/performance_baseline_report.md`؛
  - `docs/release_signoff_v1.0.0.md` بالتوقيع النهائي للإصدار `v1.0.0-kernal`.

## 5. تعريف الإنجاز (Definition of Done)

- [x] جميع الأوامر `T0`–`T7` نُفّذت بالترتيب وسُجّلت مخرجاتها في [سجل الأدلة](../reports/p7_gate_evidence.md).
- [ ] **جزئي:** صفر Data Races وصفر Partial Writes وصفر Deadlocks تحققت؛ لكن `TestPostingLatencySLO` فشل في `T5` و`check_coverage.sh` فشل في `T6`.
- [ ] **فشل:** $P50 = 238.74–257.63\,\text{ms}$ و$P95 = 337.71–441.33\,\text{ms}$ عند $57–63$ TPS؛ لم تتحقق $1{,}000$ TPS على PostgreSQL.
- [ ] **جزئي:** اختبار المستهلك الخارجي ناجح؛ التغطية `52.7% < 80%`.
- [x] تمرين DR ناجح وسلسلة التدقيق سليمة وRunbooks معتمدة (`T7`).
- [ ] **فشل:** `G1`–`G4` و`G6` GO، لكن `G5` NO-GO؛ سُجّل قرار `NO-GO` في [تقرير الاعتماد](../release_signoff_v1.0.0.md).

## 6. سجل التنفيذ الفعلي

نُفّذ التسلسل كاملاً في `2026-11-21` على `HEAD = 9c9f831` مع شجرة عمل غير ملتزمة. اللوغات الكاملة في `/tmp/opencode/p7_evidence/`. تفاصيل الأوامر، Exit codes، وعيّنات SLO والتغطية في [سجل الأدلة](../reports/p7_gate_evidence.md). أي إعادة تشغيل بعد إصلاح عيب مملوك يجب أن تبدأ من `T0`.
