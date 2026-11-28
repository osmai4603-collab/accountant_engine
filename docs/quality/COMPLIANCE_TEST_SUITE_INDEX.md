# Accounting Kernel Quality Standard — Document Set v2.0.0

> **الحالة:** حزمة وثائق مشتقة من معيار 2.0.0؛ معتمدة ومكتملة الأدلة للتشغيل `RUN-20261128-FINAL` (Commit `90c121e`).
> **قاعدة الحفظ:** المعرفات والمتطلبات والمصطلحات المحاسبية المنقولة من المصدر لا تتغير دون Change Request رسمي.

# فهرس حزمة اختبارات المطابقة (Compliance Test Suite Index)

**المرجع الحاكم:** [معيار جودة النواة المحاسبية](ACCOUNTING_KERNEL_QUALITY_STANDARD.md) — الإصدار 2.0.0.

**الغرض:** ربط المتطلبات والثوابت والبوابات والاختبارات المرجعية مع سجل التشغيل الفعلي الموثق بالأدلة في القسم 5.

> [!NOTE]
> **سجل التشغيل موثق (`RUN-20261128-FINAL`):** تم تنفيذ جميع أوامر التحقق العشرة على الشجرة المجمدة (`90c121e`) وحفظ مخرجاتها في [`docs/evidence/2026-11-28-final/`](../evidence/2026-11-28-final/). حالة `PERF-001` موثقة كخط أساس `E3` على بيئة التطوير المحلية مع استثناء `E4` المعتمد لبيئة الإنتاج.

## 1. سياسة معرفات الاختبارات

تُحفظ معرفات ACC / DOM / TXN / MON / LIFE / AUD / ARC / ISO / TEST / PERF / OPS كما وردت في المعيار. معرفات REF-... هي معرفات اختبار مرجعية وليست أسماء ملفات مفروضة؛ يجب ربط كل معرف بمسار اختبار فعلي في المشروع.

## 2. كتالوج الثوابت المحاسبية

التعريفات والمعايير الحاكمة للثوابت ACC-001 إلى ACC-013 موجودة في القسم 5.1 من [المعيار الأساسي](ACCOUNTING_KERNEL_QUALITY_STANDARD.md). هذا الفهرس لا يكرر تعريفاتها؛ ويستخدم معرفاتها الثابتة في مصفوفة التتبع أدناه.

# 3. مصفوفة تتبع المتطلبات (Requirement Traceability Matrix)

يحدد الجدول المعيار الأساسي الذي تُحتسب فيه درجة المتطلب، مع السماح بربطه بأكثر من بوابة. يمنع هذا الفصل ازدواج خصم المتطلب نفسه في الدرجة النهائية.

> **الربط مع الفهارس الأخرى:** معرفات `REF-*` أدناه مرتبطة بقواعد النواة وأسماء اختباراتها وسيناريوهات العقد في [فهرس التتبع الموحّد](TEST_TRACEABILITY_INDEX.md). هذا الفهرس يملك مصفوفة المتطلبات/البوابات فقط، ولا يكرر قوائم أسماء الاختبارات.

| Requirement ID | المتطلب | حرج؟ | محور الدرجة الأساسي | البوابات | الحد الأدنى للدليل | مرجع الاختبار |
| --- | --- | --- | --- | --- | --- | --- |
| ACC-001 | توازن القيد | نعم | 5.1 | G01, G10 | E3 | REF-ACC-001 |
| ACC-002 | سلامة سطر القيد | نعم | 5.1 | G01, G10 | E3 | REF-ACC-002 |
| ACC-003 | إشارة المبلغ | نعم | 5.1 | G01, G10 | E3 | REF-ACC-003 |
| ACC-004 | قابلية الترحيل | نعم | 5.1 / 5.5 | G01, G05 | E3 | REF-ACC-004 |
| ACC-005 | صلاحية الفترة | نعم | 5.5 | G05 | E3 | REF-ACC-005 |
| ACC-006 | عدم تعديل التاريخ | نعم | 5.5 / 5.6 | G05, G06 | E3 | REF-ACC-006 |
| ACC-007 | Idempotency | نعم | 5.3 | G03, G10 | E3 | REF-ACC-007 |
| ACC-008 | اتساق العملة | نعم عند تعدد العملات | 5.4 | G04, G10 | E3 | REF-ACC-008 |
| ACC-009 | عزل الدفتر | نعم عند Multi-Ledger | 5.8 | G08, G10 | E3 | REF-ACC-009 |
| ACC-010 | عزل المستأجر | نعم عند Multi-Tenancy | 5.8 | G08, G10 | E3 | REF-ACC-010 |
| ACC-011 | تتبع المصدر | حسب فئة المصدر | 5.13 | G06, G10 | E2/E3 | REF-ACC-011 |
| ACC-012 | اتساق الرصيد | نعم | 5.13 | G01, G07, G10 | E3 | REF-ACC-012 |
| ACC-013 | التوازن الوظيفي | نعم عند تعدد العملات | 5.4 | G01, G04, G10 | E3 | REF-ACC-013 |
| DOM-001 | حماية Aggregate Invariants | نعم | 5.2 | G02, G10 | E3 | REF-DOM-001 |
| TXN-001 | Atomicity / Rollback | نعم | 5.3 | G03, G07 | E3 | REF-TXN-001 |
| TXN-002 | Concurrency Control | نعم | 5.3 | G03, G07 | E3 | REF-TXN-002 |
| MON-001 | Monetary Representation & Rounding | نعم | 5.4 | G04, G10 | E3 | REF-MON-001 |
| LIFE-001 | Lifecycle & Period Rules | نعم | 5.5 | G05 | E3 | REF-LIFE-001 |
| AUD-001 | Audit & Tamper Evidence | حسب النطاق | 5.6 | G06 | E3 | REF-AUD-001 |
| ARC-001 | Dependency Direction | نعم | 5.7 | G09 | E2/E3 | REF-ARC-001 |
| ISO-001 | Tenant / Ledger Isolation | عند إعلانها | 5.8 | G08 | E3 | REF-ISO-001 |
| TEST-001 | Reference / Property / Failure Tests | نعم | 5.10 | G10 | E3 | REF-TEST-001 |
| PERF-001 | Performance SLO | حسب النطاق | 5.11 | G11 | E4 للهدف | REF-PERF-001 |
| OPS-001 | Recovery / Migration / Compatibility | حسب النطاق | 5.13 | G07, G11 | E3/E4 | REF-OPS-001 |

هذه مصفوفة الحد الأدنى. يجب إضافة صف لكل متطلب إلزامي تفصيلي في Scope Profile أو Compliance Test Suite قبل التقييم، وتحديد Criticality ووزن البند واختباره المرجعي. لا يُحتسب أي متطلب في أكثر من محور واحد.

## 4. فهرس مجموعات الاختبار

| Test Group | الغرض | متطلبات مرجعية | نوع الاختبار المتوقع | مسار التنفيذ الفعلي | الحالة / آخر تشغيل |
| --- | --- | --- | --- | --- | --- |
| Accounting Invariants | إثبات ACC-001–ACC-013 | ACC-* | Unit / Property / Reference | `internal/kernel/posting/*_test.go`, `internal/kernel/journal/*_test.go` | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Domain Invariants | حماية Aggregate boundaries | DOM-001 | Unit / Model-Based | `internal/kernel/journal/entry_test.go`, `internal/kernel/journal/state_machine_test.go` | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Transaction Contracts | Atomicity / Rollback / Concurrency | TXN-001, TXN-002 | Integration / Failure Injection / Concurrency | `test/atomic/...`, `test/concurrency/...`, `test/contract_parity/...` | `Pass (E3, RUN-20261128-FINAL, 90c121e)` |
| Monetary & FX | Precision / Rounding / Allocation / Functional Currency | MON-001, ACC-008, ACC-013 | Property / Reference | `internal/kernel/types/*_test.go`, `internal/kernel/valuation/*_test.go`, `internal/kernel/posting/rounding*_test.go` | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Lifecycle & Periods | Posting / Reversal / Close / Reopen | LIFE-001, ACC-004–006 | State / Integration | `internal/kernel/masterdata/fiscal_calendar*_test.go`, `internal/kernel/posting/*_test.go` | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Audit & Source | Audit trail / Tamper evidence / Source traceability | AUD-001, ACC-011 | Integration / Negative | `internal/kernel/governance/*_test.go`, `internal/kernel/posting/entry_invariants_test.go` | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Architecture | Dependency direction / boundary rules | ARC-001 | Static Architecture Tests | `internal/kernel/doc_test.go`, `scripts/verify_architecture.sh` | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Isolation | Tenant and Ledger isolation | ISO-001, ACC-009–010 | Integration / Security Negative | `internal/kernel/masterdata/masterdata_integrity_test.go`, `test/acid/...` | `Pass (CompanyID E3, 90c121e; LedgerID Out of Scope)` |
| Verification Harness | Reference / Property / Failure suites | TEST-001 | Test Harness / Mutation | `internal/kernel/doc_test.go` (`TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests`) | `Pass (RUN-20261128-FINAL, 90c121e)` |
| Performance | Target SLO / Baseline | PERF-001 | Benchmark / Load | `internal/kernel/posting/benchmark_test.go`, `test/benchmark/...` | `Pass E3 Baseline / Waived E4 (Production Hardware)` |
| Operations | Recovery / Migration / Compatibility | OPS-001 | Migration / Restore / Compatibility | `test/dr/...`, `storage/postgres/migrations*_test.go` | `Pass E3 (DR Drill 90c121e) / Waived E4` |

## 5. سجل تشغيل الاختبارات

| Run ID | Commit | Environment | Test ID / Group | Command | Result | Evidence URI / Path | Date | Reviewer |
|---|---|---|---|---|---|---|---|---|
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `Full Repository Suite` | `go test ./... -count=1` | `Pass (all packages ok, 0 FAIL)` | `docs/evidence/2026-11-28-final/full_repo.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `ACC-001..004, ACC-012, ACC-013 (Accounting Invariants)` | `go test ./internal/kernel/... -count=1` | `Pass (10/10 kernel packages, 92.4% cov)` | `docs/evidence/2026-11-28-final/kernel_unit.log`, `kernel_coverage.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `DOM-001 (Domain & State Machine)` | `go test ./internal/kernel/... -shuffle=on -count=10` | `Pass (0 failures across 10 shuffles)` | `docs/evidence/2026-11-28-final/kernel_shuffle.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `ACC-007, TXN-001, TXN-002 (Contract Parity & Race)` | `go test ./test/contract_parity/... -count=1` & `go test -race ./internal/kernel/...` | `Pass (Memory + PostgreSQL 16, 0 races)` | `docs/evidence/2026-11-28-final/contract_parity.log`, `kernel_race.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `MON-001, ACC-008, ACC-013 (Monetary & FX)` | `go test ./internal/kernel/... -count=1` | `Pass (types 89.6%, valuation 92.7%, posting 89.9%)` | `docs/evidence/2026-11-28-final/kernel_coverage.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `LIFE-001, ACC-005, ACC-006 (Lifecycle & Periods)` | `go test ./internal/kernel/... -count=1` | `Pass (journal + masterdata + posting)` | `docs/evidence/2026-11-28-final/kernel_unit.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `AUD-001, ACC-011 (Audit & Source)` | `go test ./internal/kernel/... -count=1` | `Pass (governance + posting invariants)` | `docs/evidence/2026-11-28-final/kernel_unit.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `ISO-001, ACC-010 (Company Isolation; ACC-009 Out of Scope)` | `go test ./internal/kernel/... ./test/contract_parity/... -count=1` | `Pass (CompanyID isolated; Multi-Ledger Out of Scope)` | `docs/evidence/2026-11-28-final/kernel_unit.log`, `contract_parity.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `ARC-001 (Architecture, API Compat & Static Analysis)` | `./scripts/verify_architecture.sh`, `./scripts/check_api_compat.sh`, `go vet ./...`, `gofmt -l internal/kernel/` | `Pass (clean arch, 0 vet issues, 0 fmt diffs)` | `docs/evidence/2026-11-28-final/architecture.log`, `api_compat.log`, `go_vet.log`, `gofmt.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-FINAL` | `90c121e` | `linux/amd64, go1.26.3, PostgreSQL 16` | `OPS-001 (DR Drill)` | `go test -v ./test/dr/...` | `Pass (7/7 subtests ok, 4.788s)` | `docs/evidence/2026-11-28-final/dr_drill.log` | `2026-11-28` | `Kernel Architecture Review Board` |
| `RUN-20261128-04` | `e4e671c+` | `linux/amd64, go1.26.3, PostgreSQL 16` | `PERF-001 (Load SLO Baseline)` | `POSTGRES_URL=... POSTING_SLO_WORKERS=16 POSTING_SLO_OPS=1000 go test -v ./test/benchmark -run TestPostingLatencySLO` | `Measured E3 (164.02 TPS, P50=92.55ms); E4 Waived` | `docs/reports/performance_samples.json` | `2026-11-28` | `Automated Engineering Audit` |

## 6. قائمة المراجعة السريعة

## ملحق A: Quick Review Checklist — 10 بنود

1. [x] هل تم اعتماد Scope Profile للإصدار وCommit محل التقييم؟
2. [x] هل ثوابت المحاسبة والقيود والتوازن محددة ومحمية؟
3. [x] هل نموذج العملات يحدد العملة الوظيفية والتحويل والتقريب؟
4. [x] هل دورة حياة القيد تمنع تعديل التاريخ المرحل؟
5. [x] هل المعاملات والتخزين وIdempotency تحمي من الأثر الجزئي أو المكرر؟
6. [x] هل عزل Tenant وLedger مثبت باختبارات سلبية؟
7. [x] هل يمكن تتبع القيود ومصالحة الأرصدة وإعادة بنائها؟
8. [x] هل توجد اختبارات مرجعية وفشل وتزامن وترحيل؟
9. [x] هل اجتازت جميع Quality Gates المنطبقة؟
10. [x] هل الأدلة والدرجات والاستثناءات وموافقات الاعتماد مكتملة؟

هذه القائمة للفرز الأولي فقط، ولا تحل محل المطابقة الكاملة.

## 7. قائمة المطابقة الكاملة

## ملحق B: Full Compliance Checklist — القائمة الكاملة

تستخدم القائمة التالية مع Requirement Traceability Matrix والأقسام التفصيلية. يجب تسجيل نتيجة كل بند ورابط الدليل والاختبار والبوابة.

- [ ] تحديد إصدار المستودع وCommit وTarget Environment.
- [ ] اعتماد Scope Profile ونوع التقييم وتصنيف Core / Optional / Orbital / Out of Scope.
- [ ] تحديد الأساس المحاسبي والإطار المحاسبي وحدود الاعتراف والقياس.
- [ ] فحص كتالوج الثوابت ACC-001 إلى ACC-013 وتحديد طبقة فرض كل ثابت.
- [ ] فحص صحة القيد المزدوج والمعادلة المحاسبية وطبيعة وتصنيف الحسابات.
- [ ] فحص Posting Rules وصلاحية الحساب والفترة وحالات الترحيل.
- [ ] فحص Domain Model وحدود Entities وValue Objects وAggregates وRepositories.
- [ ] فحص دورة حياة Chart of Accounts ومنع الحذف أو التغيير المدمر تاريخياً.
- [ ] إثبات Atomicity وConsistency وIsolation وDurability وRollback.
- [ ] فحص Idempotency Scope وRequest Hash وسلوك التزامن وإعادة المحاولة.
- [ ] فحص Transactional Outbox عند الحاجة إلى نشر أحداث خارجية.
- [ ] فحص Persistence SPI وContract Tests لكل Adapter مدعوم.
- [ ] فحص التمثيل المالي وCurrency Scale وInternal Precision وRounding وAllocation.
- [ ] فحص FX Rate ومصدره وتاريخه واتجاهه والعملة الوظيفية ومبلغ العملية.
- [ ] إثبات ACC-013 واختبارات التوازن الوظيفي وفروقات التقريب.
- [ ] فحص Accounting Lifecycle وReversal وAdjustment وVoid.
- [ ] فحص Fiscal Period Management والإغلاق وإعادة الفتح.
- [ ] فحص Audit Events وTamper Evidence وRetention.
- [ ] فحص Source Document Integrity وسياسات القيود اليدوية والآلية والتسويات.
- [ ] فحص Temporal Integrity وعدم إعادة كتابة التاريخ.
- [ ] فحص Balance Reconciliation ومصدر الحقيقة والإسقاطات المتزامنة/غير المتزامنة وإعادة البناء.
- [ ] فحص Migration & Compatibility وخطط التراجع عند الحاجة.
- [ ] فحص Error Taxonomy وعدم تجاهل الأخطاء أو فقدان سياقها.
- [ ] فحص استقلال المجال واتجاه الاعتماد وقابلية تبديل التخزين والتضمين.
- [ ] فحص Multi-Ledger وMulti-Tenancy كلٌ على حدة ضمن النطاق.
- [ ] فحص حدود النواة وعقود الوحدات المحيطة وSubledgers.
- [ ] تشغيل Unit وIntegration وContract وProperty-Based وModel-Based وMutation وFailure-Injection Tests حسب المتطلبات.
- [ ] تشغيل Concurrency وIsolation وRounding وIdempotency وMigration Tests.
- [ ] فحص Performance Baseline وSLO وp50/p95/p99 وThroughput وMemory.
- [ ] فحص Maintainability وDocumentation وVersioning وBackward Compatibility.
- [ ] تسجيل Evidence Level لكل متطلب وعدم الخلط بين E0–E4.
- [ ] حساب الدرجات الهرمية وتوثيق الأوزان وN/A وNot Verified وFailed وWaived.
- [ ] اجتياز G01–G11 المنطبقة وفق شروط القبول الصريحة.
- [ ] استكمال المراجعة المستقلة وRACI وقرار جهة الاعتماد.
- [ ] إصدار تقرير يوضح النتيجة والقيود والفجوات والاستثناءات وما لم يتم التحقق منه.

**ملاحظة حاكمة:** لا يُسجل أي معيار على أنه مستوفى اعتماداً على اسم مكون أو واجهة أو وثيقة فقط. يجب أن يستند الحكم إلى تنفيذ قابل للتتبع واختبار أو دليل مناسب، مع التصريح بوضوح عندما يتعذر التحقق.

## 8. قواعد التحديث

1. لا يُحذف Requirement ID أو Test ID مستخدم؛ يُعلّم Deprecated مع مرجع بديل عند الضرورة.
2. كل متطلب جديد يحصل على معرف ثابت، ومحور درجة أساسي، وبوابة، ومستوى دليل، واختبار مرجعي أو سبب موثق لعدم وجود اختبار آلي.
3. لا يثبت وجود الاختبار بمجرد وجود هذا الفهرس؛ يجب تسجيل مسار التنفيذ ونتيجة تشغيل مرتبطة بـ Commit وبيئة.
4. لا يجوز تغيير معيار القبول في هذا الفهرس بما يخالف المعيار الأساسي أو مواصفة البوابات.
