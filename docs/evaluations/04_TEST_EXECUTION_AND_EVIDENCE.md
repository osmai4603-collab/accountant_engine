# 04 — سجل الاختبارات الفعلية والأدلة (Test Execution & Evidence Register)

## 1. سجل التشغيل الفعلي في هذه الجلسة

جميع الأوامر أدناه تم تشغيلها فعلياً على الـ Commit `f637cafd5309b363f928e810fce1d69c3d4bfef5` بتاريخ `2026-11-25`:

| Run ID | النطاق / المجموعة | الأمر المنفذ | النتيجة الفعلية | الملاحظات |
| --- | --- | --- | :---: | --- |
| **RUN-01** | اختبارات النواة + كشف السباق | `go test -v -race ./internal/kernal/...` | **PASS** | نجاح جميع الحزم (10 حزم) بـ 0 data races |
| **RUN-02** | تغطية كود النواة | `go test -cover ./internal/kernal/...` | **PASS** | `apperrors` 100%, `governance` 98.5%, `types` 98.3%, `policy` 95.5%, `valuation` 93.7%, `masterdata` 92.3%, `posting` 90.3%, `journal` 88.9%, `ledger` 74.5% |
| **RUN-03** | التحقق المعماري | `./scripts/verify_architecture.sh` | **PASS** | اجتياز 11/11 قاعدة (`R1`–`R6`) |
| **RUN-04** | استيرادات النواة | `go list -f '{{.ImportPath}} -> {{.Imports}}' ./internal/kernal/...` | **PASS** | 0 استيرادات خارج `stdlib`, `decimal`, `uuid` |
| **RUN-05** | اختبارات ACID / Atomic / Concurrency / Parity | `go test ./test/acid/... ./test/atomic/... ./test/concurrency/... ./test/contract_parity/...` | **PASS (Memory)** | `acid` 0.144s, `atomic` 1.250s, `concurrency` 0.104s, `contract_parity` 21.608s |
| **RUN-06** | اختبار الاستعادة DR | `go test ./test/dr/...` | **FAIL** | `TestDRDrill_FullRecovery` فشل: `stat /tmp/opencode: no such file or directory` |
| **RUN-07** | قياسات الأداء وSLO | `go test -bench=. -benchtime=50ms -run=^$ ./test/benchmark/...` و`go test -v ./test/benchmark/...` | **PASS / SKIP** | Microbenchmarks الـ 5 عملت؛ `TestPostingLatencySLO` تم تخطيه (`POSTGRES_URL` غير مضبوط) |

---

## 2. حالة قائمة المراجعة السريعة (Quick Review Checklist — ملحق A)

1. `[x]` تحديد النطاق والإصدار والـ Commit (`f637cafd`, `internal/kernal` فقط).
2. `[x]` ثوابت المحاسبة والتوازن محددة ومحمية (`ACC-001`–`ACC-008`, `ACC-012`, `ACC-013`).
3. `[x]` نموذج العملات والعملة الوظيفية والتقريب محدد ومختبر.
4. `[x]` دورة حياة القيد تمنع تعديل التاريخ المرحل (`ValidateImmutability` + `BuildReversalEntryAt`).
5. `[~]` المعاملات وIdempotency قوية في النواة والذاكرة، لكن E4 على PostgreSQL غير متحقق في هذه البيئة.
6. `[~]` عزل `CompanyID` مثبت باختبارات، لكن `TenantID` و`LedgerID` غير موجودين.
7. `[x]` إعادة بناء الأرصدة من الحركات المرحلة متوفرة (`RebuildAccountBalances`).
8. `[~]` اختبارات وحدات وفازينج وتزامن ناجحة؛ `test/dr` فشل محلياً وPostgres E4 متخطى.
9. `[ ]` لم تجتز جميع بوابات الجودة (`G02`, `G06`, `G08` بها فجوات، و`G03/G07/G11` غير متحققة لـ E4).
10. `[x]` تم توثيق الأدلة والدرجات والفجوات بموضوعية كاملة في هذا التقييم.
