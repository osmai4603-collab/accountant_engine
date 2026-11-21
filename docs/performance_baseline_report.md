# Performance Baseline Report — `v1.0.0-kernal`

- **Status:** `MEASURED — G5 NO-GO`
- **Classification:** `measurement only`؛ ليست شهادة SLO لأن العتبات لم تُحقق.
- **Gate:** `G5` في [تعريفات البوابات](reports/p7_gate_definitions.md).
- **Adapter:** `storage/postgres` فقط؛ `storage/memory` مستبعد من شهادة الأداء.
- **Protocol:** [T5 في خطة الاختبارات](execution_plan_steps/09_PHASE_7_TEST_EXECUTION.md#t5--الأداء-وخط-الأساس-المعتمد-g5).
- **Evidence:** [سجل البوابات](reports/p7_gate_evidence.md)، `/tmp/opencode/p7_evidence/T5_latency_slo.log`، `/tmp/opencode/p7_evidence/T5_benchmarks.log`، `docs/reports/performance_samples.json`.

## 1. Environment

| Component | Value |
| :--- | :--- |
| OS / architecture | `Linux 7.2.6-76070206-generic x86_64` |
| CPU | `Intel(R) Core(TM) i7-4610M CPU @ 3.00GHz` |
| Logical CPUs | `4` (`nproc`) |
| Memory | `15Gi` total (`free -h`) |
| Go | `go1.26.3 linux/amd64` |
| PostgreSQL | `PostgreSQL 18.6 (Ubuntu 18.6-1.pgdg24.04+2) on x86_64-pc-linux-gnu` |
| PostgreSQL access | `POSTGRES_URL='postgres://osm@localhost:5432/accounting_test?sslmode=disable&host=/var/run/postgresql'` |
| SLO command | `POSTGRES_URL="$PGURL" go test ./test/benchmark -run '^TestPostingLatencySLO$' -count=3 -v` |
| Benchmark command | `POSTGRES_URL="$PGURL" go test ./test/benchmark -run '^$' -bench . -benchtime=1s -count=3` |
| Supporting probe | `go run /tmp/opencode/pg_throughput.go` → `275.0 TPS` |
| Commit / tree state | `HEAD = 9c9f831` (`2.9`)؛ 62 changed/untracked entries؛ working tree uncommitted |
| `benchstat` | غير مثبت في البيئة؛ لا يُستخدم للمقارنة (موثّق في `F-09`/`P7-03`) |

## 2. Thresholds fixed by `P7-03`

| Metric | Limit | Best observed | Result | Evidence |
| :--- | :---: | :---: | :---: | :--- |
| `P50` latency | ≤ `15 ms` | `238.738 ms` | **`FAIL`** | `T5_latency_slo.log` run 3 |
| `P95` latency | ≤ `50 ms` | `337.707 ms` | **`FAIL`** | `T5_latency_slo.log` run 3 |
| `P99` latency | inform only | `471.505 ms` (min of 3) | `INFO` | `T5_latency_slo.log` |
| Sustained load | `1000 TPS` | `63.305 TPS` | **`FAIL`** | `T5_latency_slo.log` run 3 |
| Data races | `0` | `0` | `PASS` | `T4_race_all.log` + `T4_race_concurrent_benchmark.log` |
| Partial writes | `0` | `0` | `PASS` | `T3_atomic_acid_verbose.log` + `T3_fault_idempotency.log` |
| Coverage | ≥ `80%` | `52.7%` | **`FAIL`** | `T6_check_coverage.log` |

## 3. Measured SLO samples — `posting_pipeline` / PostgreSQL

كل عيّنة: `6,000` عملية ترحيل عبر `pkg/sdk` على `storage/postgres`، ثلاث تكرارات متسلسلة.

| Run | Duration | TPS | P50 | P95 | P99 | Min | Max | Recorded (UTC) | Result |
| :---: | :---: | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :---: |
| 1 | `1m45.158s` | `57.057` | `257.625 ms` | `441.330 ms` | `588.144 ms` | `183.789 ms` | `715.900 ms` | `2026-11-21T02:47:08Z` | `FAIL` |
| 2 | `1m42.645s` | `58.454` | `253.069 ms` | `392.604 ms` | `471.505 ms` | `188.873 ms` | `635.890 ms` | `2026-11-21T02:48:52Z` | `FAIL` |
| 3 | `1m34.779s` | `63.305` | `238.738 ms` | `337.707 ms` | `554.814 ms` | `183.212 ms` | `790.258 ms` | `2026-11-21T02:50:27Z` | `FAIL` |

الفشل صريح في كل تكرار:

```text
P50 257.62 ms exceeds 15.00 ms limit
P95 441.33 ms exceeds 50.00 ms limit
throughput 57.1 TPS below 1000.0 TPS target
--- FAIL: TestPostingLatencySLO
SLO_EXIT_STATUS=1
```

> `docs/reports/performance_samples.json` يحتفظ بالعيّنة الثالثة. الحمل الفعلي كان نحو `57–63 TPS`؛ لم يُحافظ على `1000 TPS` لمدة 60 ثانية، لذا لا يمكن إصدار شهادة SLO.

## 4. Microbenchmarks (`ns/op` only)

الأمر `go test -bench . -benchtime=1s -count=3` نجح (`BENCH_EXIT_STATUS=0`)، لكن هذه قياسات مرجعية ولا تُحوّل الفشل أعلاه إلى `GO`.

| Scenario | Adapter | Samples (ns/op) | Range | Result |
| :--- | :--- | :--- | :--- | :--- |
| Draft creation | postgres | `7,474,704` · `8,452,463` · `8,745,565` | `7.47–8.75 ms/op` | measurement |
| Posting pipeline | postgres | `85,499,131` · `44,932,532` · `40,829,793` | `40.83–85.50 ms/op` | measurement |
| Concurrent posting | postgres | `17,836,012` · `17,552,346` · `19,257,268` | `17.55–19.26 ms/op` | measurement |
| Balance retrieval | postgres | `344,264` · `326,023` · `325,093` | `0.325–0.344 ms/op` | measurement |
| Trial balance proxy | postgres | `1,296,344` · `1,376,867` · `1,280,758` | `1.28–1.38 ms/op` | measurement |
| Cash flow | postgres | — | skipped | `host-provided ReportingStore not shipped` |
| Year-end closing | postgres | — | skipped | `host-provided YearEndClosing service not shipped` |

> حتى أفضل `BenchmarkPostingPipeline` (`40.83 ms/op`) يتجاوز حد `P50 = 15 ms` وحد `P95 = 50 ms` قريباً، لكنه لا يمثل توزيعاً؛ التوزيع الحقيقي في القسم 3 وهو الحاكم.

## 5. Interpretation

- القيم خاصة بهذه البيئة وعدد العينات وحالة الذاكرة المؤقتة؛ ليست ضماناً محمولاً لأجهزة أخرى.
- مجمّع percentiles الجديد وفّر `P50/P95/P99 + TPS`، بعكس قالب المرجع الذي كان ينتج `ns/op` فقط (`F-09`).
- `benchstat` غير متوفر؛ لا توجد مقارنة إحصائية بين الإصدارات، وسُجّل الغياب صراحةً.
- فحص PostgreSQL خام بعملية autocommit أعطى `275.0 TPS` في التشغيل الداعم الأخير؛ أي أن البيئة نفسها لا تثبت جاهزية `1000 TPS` حتى قبل طبقة المحرك.
- نجاح `go test -bench` لا يعني نجاح `G5`؛ بوابة الأداء تعتمد على عيّنات SLO والتغطية، وكلاهما فاشل.
- أي تحسين أو إصلاح يستوجب العودة إلى المهمة المالكة ثم إعادة `P7-04` كاملة من `T0` وفق [قاعدة الإصلاح](reports/p7_gate_definitions.md#قواعد-التشغيل).

## 6. Signed measurement

- **Measured by:** `P7-04` / `T5` + `T6`.
- **Decision:** `NO-GO` for `G5`.
- **Date (UTC):** `2026-11-21`.
