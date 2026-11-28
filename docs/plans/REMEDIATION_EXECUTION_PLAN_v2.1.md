# الخطة التنفيذية الشاملة لإصلاح نقاط الضعف ورفع جاهزية المشروع (`v2.1.0`)

- **رمز الوثيقة:** `AE-PLAN-REMEDIATION-v2.1`
- **المرجع الأساسي:** إعادة التقييم العميق للحالة الفعلية للمستودع (`HEAD: cf46864`, Tag `v2.0.0`)
- **التاريخ:** `2026-11-28`
- **الحالة:** مسودة تنفيذية جاهزة للتطبيق (`Ready for Execution`)

---

## 1. الهدف ونطاق الخطة

تهدف هذه الخطة إلى معالجة جميع نقاط الضعف الهندسية والتشغيلية والاختبارية التي كشفها التقييم العميق للمشروع، دون المساس بنقاء النواة المحاسبية (`internal/kernal`) أو كسر عقود السطح العام (`pkg/api/v1`, `pkg/spi`, `pkg/sdk`).

### مؤشرات النجاح المستهدفة عند إغلاق الخطة

1. نجاح جميع سكربتات التحقق بنسبة 100%:
   - `./scripts/verify_integrity.sh` -> `PASS`
   - `./scripts/verify_architecture.sh` -> `PASS`
   - `./scripts/check_api_compat.sh` -> `PASS`
   - `./scripts/check_coverage.sh coverage.out` -> `>= 80%` للمستودع كاملاً (حالياً `52.5%`).
2. شجرة Git نظيفة بعد تشغيل `go test ./... -race -count=1` دون تعديل ملفات متتبعة تلقائياً.
3. إغلاق فجوات تغطية الاختبارات في المدارات والتخزين (`reporting`, `postingcompat`, `subledgers`, `pkg/sdk`, `storage/*`).
4. معالجة تسرب الذاكرة في `storage/memory/multi_ledger.go` وتحسين مسار الأداء في `storage/postgres` تمهيداً لإغلاق بوابة `E4` (`1000 TPS`, `P50 <= 15ms`, `P95 <= 50ms`).
5. إضافة قابلية المراقبة (`Observability Hooks`) والتخطيط الآمن لإعادة تسمية `internal/kernal` -> `internal/kernel`.

---

## 2. مصفوفة المراحل والأولويات

| المرحلة | الاسم | الأولوية | الهدف الرئيسي | المخرجات القابلة للقياس |
| :---: | :--- | :---: | :--- | :--- |
| **P0** | **تنظيف الشيفرة وسلامة المستودع** | حرج (فوري) | إصلاح `gofmt`، `doc.go`، ونظافة شجرة Git | `verify_integrity.sh` = `PASS`، `git status` نظيف |
| **P1** | **إصلاحات الموثوقية والذاكرة** | عالٍ | منع تسرب `sync.Map` وتغليف أخطاء/سلوكيات الحواف | اختبارات ذاكرة وإغلاق نظيف لـ `memory.Driver` |
| **P2** | **سد فجوة التغطية في المدارات وSDK** | عالٍ | رفع تغطية المستودع من `52.5%` إلى `>= 80%` | `check_coverage.sh` = `PASS` (`reporting >= 80%`) |
| **P3** | **تحسين الأداء وإغلاق بوابة `E4`** | متوسط-عالٍ | تقليص `P50` من `92.5ms` إلى `<= 15ms` ورفع `TPS` | ملف `performance_samples.json` محدث ومعتمد |
| **P4** | **المراقبة التشغيلية (`Observability`)** | متوسط | إضافة واجهات `Metrics` و`Tracing` اختيارية بدون تبعيات ثقيلة | دعم خطافات القياس في `pkg/sdk` و`storage/postgres` |
| **P5** | **توحيد التسمية (`kernal` -> `kernel`)** | منخفض-متوسط | إزالة الخطأ الإملائي عبر 258 موضعاً بأمان تام | `go test ./...` + `check_api_compat.sh` = `PASS` |

---

## 3. التفصيل التنفيذي لكل مرحلة

---

### المرحلة P0: تنظيف الشيفرة وسلامة المستودع (`Repository Hygiene`)

**المشكلة:**

- فشل `./scripts/verify_integrity.sh` في فحصين:
  1. `21` ملف Go غير منسق بـ `gofmt` (منها `6` ملفات داخل `internal/kernal/`).
  2. `12` مجلداً تفتقد `doc.go` (منها `8` حزم Go فعلية و`4` مجلدات بيانات/سكربتات مثل `testdata` و`storage/postgres/scripts`).
- تشغيل `go test ./test/contract_parity/...` يعدّل ملفات `test/contract_parity/parity_report.md` و`docs/reports/contract_parity_report.md` تلقائياً (السطر `36` من `test/contract_parity/contract_parity_test.go`) مما يلوث شجرة Git.

**المهام التفصيلية:**

1. **تنسيق جميع ملفات Go:**
   - تشغيل `gofmt -w .` على كامل المستودع والتحقق من أن `gofmt -l .` يعيد مخرجات فارغة.
   - الملفات المتأثرة تشمل:
     - `internal/kernal/{journal/errors.go, masterdata/errors.go, posting/errors.go, posting/valuation_adapter.go, types/idempotency.go, valuation/errors.go}`
     - `modules/{ias12,ifrs15,ifrs16,ifrs9}/*.go`
     - `pkg/spi/{consolidation/consolidation.go, reconciliation/reconciliation.go, storage/closing.go}`
     - `storage/memory/multi_ledger.go`
2. **استكمال مواثيق الحزم (`doc.go`):**
   - إنشاء ملف `doc.go` موجز وواضح المسؤولية في الحزم الثماني الفعلية:
     - `pkg/spi/consolidation/doc.go`
     - `pkg/spi/reconciliation/doc.go`
     - `pkg/api/v1/governance/doc.go`
     - `internal/orbitals/reporting/taxonomy/doc.go`
     - `internal/orbitals/postingcompat/doc.go`
     - `storage/masterdatacontract/doc.go`
     - `test/concurrency/doc.go`
     - `test/dr/doc.go`
3. **تحديث فلتر مجلدات `verify_integrity.sh`:**
   - تعديل السطر `69` في `scripts/verify_integrity.sh` ليستثني المجلدات التي لا تحتوي على ملفات `.go` (مثل `testdata`, `testdata/fuzz/...`, `storage/postgres/scripts`) لأن `testdata` محجوزة في Go toolchains وليست حزماً.
4. **منع تلويث شجرة Git عند تشغيل اختبارات التكافؤ:**
   - تعديل `test/contract_parity/contract_parity_test.go` بحيث يكتب التقارير إلى `parity_report.md` و`docs/reports/contract_parity_report.md` **فقط** عند ضبط متغير البيئة `WRITE_PARITY_REPORT=1` (أو كتابتها في `t.TempDir()` افتراضياً أثناء الاختبار العادي).

**معيار القبول (`P0`):**

- `./scripts/verify_integrity.sh` يخرج برمز `0` (`RESULT: PASS`).
- `git status --short` نظيف تماماً بعد تشغيل `go test ./... -count=1`.

---

### المرحلة P1: إصلاحات الموثوقية وإدارة الذاكرة (`Memory & Reliability`)

**المشكلة:**

- في `storage/memory/multi_ledger.go` (السطر `14`)، يُستخدم متغير عام:
  `var multiLedgerEntryOwners sync.Map` لتخزين خرائط مرتبطة بمؤشرات `*Driver`.
- عدم إزالة `*Driver` عند انتهاء استخدامه يسبب بقاء المرجع في الذاكرة مدى حياة العملية (Memory Leak)، خصوصاً في الاختبارات المكثفة أو عند تعدد مثيلات `Driver`.

**المهام التفصيلية:**

1. **إعادة توطين حالة `multiLedgerEntryOwners`:**
   - نقل تتبع ملكية الدفاتر (`multiLedgerEntryOwners`) ليصبح حقلاً داخلياً غير مُصدَّر داخل `memory.Driver` (أو ربطه بدالة `Close()` لحذف المدخل من `sync.Map` إذا كان الحفاظ على بنية `Driver` ضرورياً، مع التحقق من `check_api_compat.sh`).
   - الأفضل: إضافة حقل غير مُصدّر في `Driver` لأن الحقول غير المُصدّرة لا تكسر التوافق الثنائي/السطحي، والتأكد عبر `./scripts/check_api_compat.sh`.
2. **إضافة اختبار تسرب ودورة حياة (`Lifecycle Test`):**
   - كتابة اختبار في `storage/memory` يتحقق من عدم تراكم المراجع بعد `Driver.Close()` وتمرير `go test -race ./storage/memory/...`.
3. **مراجعة مسارات الإغلاق الآمن (`Graceful Close`):**
   - التحقق من أن `Engine.CloseWithContext(ctx)` في `pkg/sdk/engine.go` يغلق جميع المخازن والمهام المجدولة بشكل حتمي دون تسريب Goroutines.

**معيار القبول (`P1`):**

- `go test -race ./storage/memory/... ./pkg/sdk/...` ينجح بـ `0` سباقات.
- `./scripts/check_api_compat.sh` ينجح بدون كسر للسطح العام.

---

### المرحلة P2: سد فجوة تغطية الاختبارات في المدارات وSDK (`Coverage Remediation`)

**المشكلة:**

- التغطية الإجمالية في `coverage.out` هي **`52.5%`** بينما يشترط `scripts/check_coverage.sh` حد **`80%`** للمستودع كاملاً.
- التغطية المنخفضة تتركز في:
  - `internal/orbitals/reporting`: **`14.2%`**
  - `internal/orbitals/postingcompat`: **`0.0%`**
  - `storage/postgres`: **`37.6%`**
  - `storage/memory`: **`38.7%`**
  - `internal/orbitals/subledgers`: **`50.3%`**
  - `pkg/sdk`: **`52.1%`**
  - `pkg/spi/application`: **`52.4%`**

**المهام التفصيلية:**

1. **رفع تغطية `internal/orbitals/reporting` من `14.2%` إلى `>= 80%`:**
   - كتابة اختبارات وحدة شاملة لـ:
     - `BalanceSheetEngine`, `IncomeStatementEngine`, `CashFlowEngine`, `EquityStatementEngine`, `TrialBalanceEngine`, `GeneralLedgerEngine`.
     - `ResolvePresentationCurrency` و`GenerateBundleForCompanyContext` في `engine.go`.
     - `multi_currency_trial_balance.go`, `currency_translation.go`, `statement_reconciliation.go`, `ratios.go`.
     - حالات الفشل: غياب `policyUnit`، عملة غير مسموح بها، فترات متداخلة، عدم توازن الميزانية.
2. **معالجة `internal/orbitals/postingcompat` (`0.0%`):**
   - تقييم ما إذا كانت الحزمة مهجورة (`deprecated`)؛ إن كانت مستخدمة فيُضاف لها اختبارات تغطي `>= 80%`، وإن كانت غير مستخدمة فيتم توثيق إزالتها أو تغليفها باختبارات توافق أساسية.
3. **رفع تغطية `internal/orbitals/subledgers` من `50.3%` إلى `>= 80%`:**
   - تغطية مسارات `fixed_assets.go`, `asset_revaluation.go`, `asset_schedule.go`, `writeoff.go`, `treasury.go`, `reconcile_control.go`, `reconciliation_exceptions.go`.
4. **رفع تغطية `pkg/sdk` و`pkg/spi/application` من `~52%` إلى `>= 80%`:**
   - اختبار جميع خيارات `EngineBuilder` (`options.go`), `bootstrap_capabilities.go`, `year_end_closing.go`, `cash_flow.go`, وحالات الأخطاء في `errors.go`.
5. **رفع تغطية `storage/memory` و`storage/postgres` إلى `>= 75%`:**
   - تغطية دوال `uow.go` غير المختبرة (السطور `42–390` في `storage/postgres/uow.go` التي أظهرت `0.0%` في `coverage.out` مثل `SaveDraft`, `CommitPosting`, `commitReversal`, `GetAccountBalance`, `GetTransactionBalances`).
   - استخدام `go test -coverprofile=coverage.out ./...` لتوحيد قياس التغطية عبر اختبارات الوحدة والتكامل معاً.

**معيار القبول (`P2`):**

- `./scripts/check_coverage.sh coverage.out` ينجح بتغطية إجمالية **`>= 80.0%`**.
- عدم وجود أي حزمة تنفيذية نشطة بتغطية أقل من `70%`.

---

### المرحلة P3: تحليل وتحسين الأداء التشغيلي (`E4 Performance SLO`)

**المشكلة:**

- القياس الحالي على PostgreSQL المحلي (`performance_samples.json`):
  - `164.02 TPS` | `P50 = 92.55 ms` | `P95 = 128.49 ms` | `P99 = 172.55 ms`.
- الهدف التشغيلي المعتمد: `1000 TPS` | `P50 <= 15 ms` | `P95 <= 50 ms`.

**المهام التفصيلية:**

1. **تشخيص عنق الزجاجة (`Profiling`):**
   - تشغيل `go test -bench=BenchmarkPosting -cpuprofile=cpu.prof -memprofile=mem.prof ./test/benchmark` وتحليل توزيع الزمن في `commitPostingLocked` (`storage/postgres/uow.go`).
   - حساب عدد جولات الشبكة (`SQL round-trips`) لكل عملية ترحيل واحدة (`PostEntry`).
2. **تقليل جولات SQL داخل المعاملة الواحدة:**
   - دمج الاستعلامات المتتابعة (مثل فحص `Idempotency`، جلب التسلسل `NextSequence`، إدراج `account_moves`، وإدراج `account_move_lines` عبر `pgx.Batch` أو `COPY` / `UNNEST` للأسطر المتعددة).
   - مراجعة إعدادات تجمع الاتصالات (`pgxpool.Config`: `MaxConns`, `MinConns`, `MaxConnLifetime`) واختبار أثر `Prepared Statements`.
3. **إعادة قياس حمل `E4`:**
   - تنفيذ `TestPostingLatencySLO` على بيئة PostgreSQL مهيأة للإنتاج (أو عتاد معزول مضبوط)، وتوثيق النتائج في `docs/reports/performance_samples.json` وإغلاق الاستثناء `Waived (E4)` في `KERNEL_RELEASE_DECISION_FINAL.md`.

**معيار القبول (`P3`):**

- تقليص `P50` إلى `<= 15 ms` و`P95` إلى `<= 50 ms` عند `1000 TPS` على بيئة القياس المستهدفة.
- اجتياز جميع اختبارات التكافؤ (`test/contract_parity`) دون أي تراجع وظيفي.

---

### المرحلة P4: إضافة المراقبة التشغيلية (`Observability`)

**المشكلة:**

- لا توجد واجهات موحدة لجمع المقاييس (`Metrics`) أو التتبع (`Tracing`) في طبقة الـ SDK أو محول PostgreSQL، مع الحفاظ على قاعدة `verify_architecture.sh` (`R5`: بقاء `go.mod` بأقل عدد ممكن من الاعتماديات بدون إقحام SDKs ثقيلة).

**المهام التفصيلية:**

1. **تصميم واجهات مراقبة خفيفة (`Zero-Dependency Interfaces`):**
   - تعريف واجهة `Observer` أو `MetricsRecorder` في `pkg/sdk/options.go` (أو `pkg/spi/application`) بدون أي اعتماديات خارجية في `go.mod`:
     - `ObservePosting(ctx, companyID, duration, err)`
     - `ObserveReversal(ctx, companyID, duration, err)`
     - `ObserveStorageTx(ctx, opName, duration, err)`
2. **تضمين الخطافات (`Instrumentation`):**
   - استدعاء الـ `Observer` اختيارياً (بنمط `No-Op` افتراضياً) داخل `pkg/sdk` و`storage/postgres`.
   - تسجيل بيانات الارتباط (`CorrelationID` الموجود بالفعل في `PostOptions` و`AuditLog`) بشكل متسق.

**معيار القبول (`P4`):**

- بقاء عدد الاعتماديات المباشرة في `go.mod` أقل من `6` (`verify_integrity.sh` = `PASS`).
- اختبار وحدة يثبت استدعاء `Observer` بالمدة والحالة الصحيحة عند النجاح والفشل.

---

### المرحلة P5: توحيد التسمية الهيكلية (`internal/kernal` -> `internal/kernel`)

**المشكلة:**

- استخدام التهجئة غير الصحيحة `internal/kernal` في `258` موضع استيراد عبر ملفات Go والوثائق.

**المهام التفصيلية (تُنفذ في Commit مستقل معزول):**

1. إعادة تسمية المجلد `internal/kernal` إلى `internal/kernel` عبر `git mv`.
2. تحديث جميع مسارات الاستيراد (`github.com/osmai4603-collab/accounting_engine/internal/kernal` -> `.../internal/kernel`) في:
   - جميع ملفات `.go`
   - السكربتات (`scripts/verify_architecture.sh`, `scripts/check_api_compat.sh`, إلخ)
   - الوثائق في `docs/`
3. التحقق من أن `pkg/api/v1` (السطح العام) لم يتغير توقيعه عبر `./scripts/check_api_compat.sh`.

**معيار القبول (`P5`):**

- `grep -rn "internal/kernal" --include="*.go" .` يعيد `0` نتائج.
- نجاح كامل لجميع الاختبارات والسكربتات (`verify_architecture.sh`, `verify_integrity.sh`, `check_api_compat.sh`, `go test -race ./...`).

---

## 4. جدول التحقق النهائي بعد إتمام الخطة

| الفحص | الأمر المرجعي | النتيجة الحالية | المستهدف بعد الخطة |
| :--- | :--- | :---: | :---: |
| سلامة المصدر | `./scripts/verify_integrity.sh` | `FAIL (2)` | `PASS` |
| النقاء المعماري | `./scripts/verify_architecture.sh` | `PASS (12/12)` | `PASS (12/12)` |
| توافق السطح العام | `./scripts/check_api_compat.sh` | `PASS` | `PASS` |
| الاختبارات + السباقات | `go test -race ./... -count=1` | `PASS` | `PASS` |
| التغطية الكلية للمستودع | `./scripts/check_coverage.sh coverage.out` | `52.5% (FAIL)` | `>= 80.0% (PASS)` |
| الأداء الإنتاجي (`E4`) | `TestPostingLatencySLO` | `164 TPS / P50=92.5ms` | `1000 TPS / P50<=15ms` |
| نظافة شجرة Git | `git status --short` | `2 modified` | `clean` |
