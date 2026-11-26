# تقرير خط الأساس المرحلة 0 — Kernel Remediation Baseline

- **رمز الوثيقة:** `AE-REMED-BASELINE-P0`
- **الخطة المرجعية:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)
- **سجل الفجوات المرافق:** [`KERNEL_REMEDIATION_GAP_REGISTER.md`](./KERNEL_REMEDIATION_GAP_REGISTER.md)
- **نافذة الفحص (UTC):** `2026-11-21T20:47:55Z` → `2026-11-21T20:54:53Z`
- **حالة الوثيقة:** خط أساس معاد إنتاجه فعلياً، وليس إعادة نقل من تقارير محفوظة
- **قرار بوابة المرحلة 0:** **PASS** — مع تسجيل الفجوات في السجل المرافق، وبدون بدء أي تعديل على منطق المجال

---

## 1. الغرض وحدود المرحلة 0

هذه المرحلة تثبت "حالة التشغيل الحالية" قبل أي تدخل في المراحل اللاحقة. لم يُعدّل أي ملف مصدر أو اختبار أو عقد أو وثيقة قائمة في النواة. الملفات الوحيدة المضافة هي مخرجات إدارة التنفيذ:

| الملف المضاف | الغرض |
| --- | --- |
| `scripts/verify_kernel_baseline.sh` | أوامر تشغيل موحدة وقابلة لإعادة الاستخدام لإنتاج هذا الخط الأساس |
| `docs/KERNEL_REMEDIATION_BASELINE.md` | هذا التقرير |
| `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` | سجل الفجوات الموحد |

> **تنبيه منهجي:** نتائج التقارير القديمة (`KERNEL_COMPLETION_REPORT.md`، `KERNEL_MUTATION_REPORT.md`، `architecture_purity_report.md`، `parity_report.md`) تُعامل هنا كـ "ادعاءات محفوظة" يتم مقارنتها بما أُعيد تشغيله، لا كدليل نجاح لهذه المرحلة.

---

## 2. بصمة المستودع وبيئة التنفيذ

| العنصر | القيمة المعاد قياسها |
| --- | --- |
| المستودع / الوحدة | `github.com/osmai4603-collab/accounting_engine` |
| الفرع | `main` |
| Commit SHA | `94054d9c9a5f8d8419c9871b2d61b17b44d5b556` |
| Commit القصير | `94054d9` |
| تاريخ الـ Commit | `2026-11-21T06:25:41+03:00` — `feat(kernel): finalize kernel v12 stabilization and acceptance 10/10` |
| Remote | `origin https://github.com/osmai4603-collab/accountant_engine.git` |
| حالة Git عند بدء الفحص | نظيفة من التعديلات؛ ملف واحد غير متتبع: `docs/KERNEL_REMEDIATION_EXECUTION_PLAN.md` |
| `go.mod` | `go 1.26.0` |
| `toolchain` | `go1.26.3` |
| `go version` الفعلي | `go version go1.26.3 linux/amd64` ✅ مطابق |
| `go env` | `GOVERSION=go1.26.3`, `GOOS=linux`, `GOARCH=amd64`, `GOTOOLCHAIN=auto`, `GOROOT=/usr/local/go` |
| وقت الفحص | 2026-11-21 (UTC)، كما في أعلى الوثيقة |
| قاعدة بيانات الاختبار | PostgreSQL متاح عبر المقبس المحلي `/var/run/postgresql/.s.PGSQL.5432`؛ متغير `POSTGRES_URL` غير مضبوط |

**نتيجة مطابقة الإصدار:** PASS — لا يوجد مانع بيئي.

---

## 3. الجرد

### 3.1 ملفات النواة

`internal/kernal/**` يحتوي على **78 ملفاً**:

| التصنيف | العدد |
| --- | ---: |
| وثائق `*.md` | 7 |
| شيفرة مصدر `*.go` (غير اختبار) | 40 |
| اختبارات `*_test.go` | 30 |
| بيانات اختبار `testdata/` | 1 |
| **الإجمالي** | **78** |

هذا يطابق جرد `KERNEL_RELEASE_BASELINE.md` (78 ملفاً: 7 وثائق + 71 برمجي/اختبار/بيانات).

### 3.2 الحزم التسع + حزمة الجذر

| الحزمة | ملفات | اختبارات `Test` | أهداف `Fuzz` |
| --- | ---: | ---: | ---: |
| `internal/kernal` (الجذر، فحوص معمارية/وثائقية) | 9 | 2 | 0 |
| `internal/kernal/apperrors` | 2 | 1 | 0 |
| `internal/kernal/governance` | 7 | 17 | 1 |
| `internal/kernal/journal` | 3 | 1 | 0 |
| `internal/kernal/ledger` | 4 | 1 | 0 |
| `internal/kernal/masterdata` | 15 | 29 | 1 |
| `internal/kernal/policy` | 5 | 7 | 0 |
| `internal/kernal/posting` | 16 | 51 | 5 |
| `internal/kernal/types` | 10 | 33 | 2 |
| `internal/kernal/valuation` | 6 | 9 | 0 |
| **الإجمالي** | **78** | **151** | **9** |

مجموع دوال الاختبار الفريدة: **160** (`151 Test` + `9 Fuzz`)، وهو مطابق لما ورد في `KERNEL_COMPLETION_REPORT.md`.

### 3.3 مخطط الاعتماد الفعلي (من `go list`، لا من الوثائق)

```text
internal/kernal        -> (none)
internal/kernal/apperrors  -> errors fmt
internal/kernal/governance -> context crypto/sha256 encoding/hex errors fmt types time
internal/kernal/journal    -> types decimal time
internal/kernal/ledger     -> types time
internal/kernal/masterdata -> context errors fmt types strings time
internal/kernal/policy     -> context errors fmt journal masterdata types strings sync time
internal/kernal/posting    -> context errors fmt uuid journal masterdata policy types decimal sort strings time
internal/kernal/types      -> bytes encoding/json errors fmt uuid decimal sort strings
internal/kernal/valuation  -> context errors fmt journal posting types decimal sort strings sync time
```

- إجمالي حزم التبعية (`go list -deps ./...`): **312** (التقرير المحفوظ: 309).
- الحزم التي تحتوي `/internal/`: **81** (مطابق للتقرير المحفوظ).
- حواف `go mod graph`: **43** (مطابق للتقرير المحفوظ).
- دورات استيراد: **0** (`verify_architecture.sh` / R6).

### 3.4 السطح العام للنواة (جرد وظيفي مختصر)

| المجموعة | أبرز العناصر المصدّرة |
| --- | --- |
| القيم والمعرفات | `Money`, `NewMoney`, `NewMoneyFromString`, `Currency`, `LookupCurrency`, `CurrencyMinorUnitChecked`, `RoundBanker`, `Zero`, `*ID` الصلبة |
| القيد | `journal.JournalEntry`, `EntryLine`, `EntryStatus`, `NewJournalEntry(At)`, `EnsureImmutable`, `BuildReversalEntry(At)` |
| الترحيل | `PostingPipeline`, `NewPostingPipeline`, `PostEntry`, `PostOptions`, `PostingExecutor`, `PostingReversalExecutor`, `Clock`, `ExtractAndSortAccountIDs` |
| البيانات الرئيسية | `masterdata.Reader`, `Company`, `Account`, `Journal`, `ValidateCompanyScope`, `ValidateAccountScope`, `GenerateFiscalYear`, `ValidateFiscalYear` |
| السياسات | `PolicyEngine`, `PostingPolicy`, `CurrencyPolicy`, `CheckAccountEligibility` |
| التقييم | `RateRegistry`, `RateProvider`, `Convert`, `ConvertForDate`, `RevaluationEngine`, `GenerateRevaluationEntry(At)` |
| التدقيق | `AuditLog`, `AuditChain`, `AtomicAuditLogRepository`, `AuditVerificationSnapshot`, `VerifyAuditIntegrity(WithCheckpoint)` |
| الأخطاء | `CodedError`, `IsCode`, `Err*` المصنفة |

> هذه مجرد خريطة جرد؛ لا يُستنتج منها اكتمال العقود. فجوات العقود (idempotency، الذرية، أقفال Adapter) موثقة في سجل الفجوات.

### 3.5 جرد الاختبارات حسب النوع

| النوع | الدليل المادي | الحالة |
| --- | --- | --- |
| Unit | 151 دالة `Test` داخل `internal/kernal/**` | أُعيد تشغيلها: PASS |
| Property / Adversarial | `posting/property_adversarial_test.go`, `posting/entry_invariants_test.go`, `posting/pipeline_invariants_test.go`, `masterdata/validation_test.go`, `governance/verification_test.go` | أُعيد تشغيلها ضمن Unit: PASS |
| Fuzz | 9 أهداف مذكورة في القسم 4.6 | أُعيد تشغيلها 30s/هدف: PASS |
| Race | `go test -race ./internal/kernal/...` | PASS — بلا سباقات |
| Architecture | `internal/kernal/doc_test.go` (2 اختبارات) + `scripts/verify_architecture.sh` (11 قاعدة) | PASS |
| Mutation | `KERNEL_MUTATION_REPORT.md` — 8 طفرات "موجهة"، دون أداة آلية أو قصاصات مصدر محفوظة | **لم يُعاد إنتاجها**؛ تُسجل فجوة G-005 |
| Contract | `test/contract_parity` — 6 سيناريوهات × محولي Memory وPostgreSQL = 12 حالة | أُعيد تشغيلها: PASS (تقرير مولّد أُعيد إلى حالته الأصلية بعد الإضافة) |
| Integration | `test/acid`, `test/atomic`, `test/concurrency`, `test/dr`, `test/external_consumer` (وحدة منفصلة) | PASS |

---

## 4. أوامر التحقق ونتائجها الفعلية

جميع الأوامر نُفذت من جذر المستودع ما لم يُذكر غير ذلك، باستثناء اختبار المستهلك الخارجي. السجلات الخام محفوظة في `/tmp/opencode/phase0/logs/`، وأهم المخرجات مُضمّنة أدناه حتى لا تعتمد الأدلة على ملفات مؤقتة فقط.

### 4.1 التنسيق — PASS

```text
$ gofmt -l internal/kernal/
(لا مخرجات)
EXIT=0
```

### 4.2 اختبارات النواة — PASS (10/10 حزم)

```text
$ go test ./internal/kernal/... -count=1
ok  .../internal/kernal              0.066s
ok  .../internal/kernal/apperrors    0.018s
ok  .../internal/kernal/governance   0.047s
ok  .../internal/kernal/journal      0.009s
ok  .../internal/kernal/ledger       0.020s
ok  .../internal/kernal/masterdata   0.010s
ok  .../internal/kernal/policy       0.010s
ok  .../internal/kernal/posting      0.016s
ok  .../internal/kernal/types        0.006s
ok  .../internal/kernal/valuation    0.005s
EXIT=0
```

### 4.3 كشف السباقات — PASS

```text
$ go test -race ./internal/kernal/... -count=1
ok  .../internal/kernal             1.179s
ok  .../internal/kernal/apperrors   1.033s
ok  .../internal/kernal/governance  1.138s
ok  .../internal/kernal/journal     1.021s
ok  .../internal/kernal/ledger      1.021s
ok  .../internal/kernal/masterdata  1.027s
ok  .../internal/kernal/policy      1.023s
ok  .../internal/kernal/posting     1.077s
ok  .../internal/kernal/types       1.038s
ok  .../internal/kernal/valuation   1.018s
EXIT=0
```

### 4.4 اختبارات المستودع الكاملة — PASS

```text
$ go test ./... -count=1
EXIT=0
```

- جميع الحزم التي تحتوي اختبارات نجحت، بما فيها `storage/postgres`، `test/contract_parity`، `test/atomic`، `test/concurrency`، `test/dr`.
- تخطٍ واحد فقط على مستوى اختبار فردي: `test/benchmark :: TestPostingLatencySLO` — سببه البيئة (`POSTGRES_URL` غير مضبوط)، وليس فشل كود.
- لا توجد حالات `FAIL`.
- اختبارات الأداء (Benchmarks) لم تُشغّل افتراضياً؛ اثنان منها يحملان `b.Skip` صريحاً لغياب host-provided stores.

### 4.5 وحدة المستهلك الخارجي — PASS

```text
$ cd test/external_consumer && go test ./... -count=1
ok  .../test/external_consumer  0.033s
EXIT=0
```

### 4.6 البوابة المعمارية — PASS (11/11 قاعدة)

```text
$ ./scripts/verify_architecture.sh
Architecture verification (module: github.com/osmai4603-collab/accounting_engine)
-------------------------------------------------------------
  [OK]   R1: internal/* free of net/http, grpc and protobuf
  [OK]   R2a: pkg/spi/* free of internal/* imports
  [OK]   R2b: pkg/api/* internal imports limited to internal/kernal/*
  [OK]   R2c: exported API free of unbridged internal/* type exposure
  [OK]   R3a: internal/kernal/* free of internal/orbitals/* imports
  [OK]   R3b: internal/kernal/* free of pkg/* imports
  [OK]   R3c: internal/kernal/* free of concrete storage/* imports
  [OK]   R3d: internal/kernal/* free of database/sql and pgx
  [OK]   R4: no cmd/ or proto/ directories
  [OK]   R5: go.mod free of grpc, protobuf, aws-sdk and net/http
  [OK]   R6: package graph resolves without import cycles
-------------------------------------------------------------
RESULT: PASS - clean architectural base
EXIT=0
```

لا تخفيض في عدد القواعد أو شدّتها مقابل `docs/reports/architecture_purity_report.md`.

### 4.7 Fuzz smoke — PASS (9/9، 30 ثانية لكل هدف)

```text
go test ./internal/kernal/<pkg> -run='^$' -fuzz='^<target>$' -fuzztime=30s -count=1
```

| الهدف | الحزمة | التنفيذات | النتيجة |
| --- | --- | ---: | :---: |
| `FuzzMoneyFromStringNeverPanics` | `types` | 757,085 | PASS |
| `FuzzMoneyJSONNeverPanics` | `types` | 769,390 | PASS |
| `FuzzGenerateFiscalYearMaintainsContinuityAndContainment` | `masterdata` | 765,191 | PASS |
| `FuzzValidateEntryDoubleEntryNeverPanics` | `posting` | 671,975 | PASS |
| `FuzzEntryLineRejectsCurrencyAndExchangeRateMismatch` | `posting` | 608,092 | PASS |
| `FuzzBalancedEntryProperties` | `posting` | 723,179 | PASS |
| `FuzzRoundingResidualRequiresExplicitSettlement` | `posting` | 341,672 | PASS |
| `FuzzReversalNetImpactIsZeroPerAccount` | `posting` | 592,594 | PASS |
| `FuzzAuditSnapshotDetectsPayloadTampering` | `governance` | 709,901 | PASS |
| **الإجمالي** | | **5,939,079** | **0 panics / 0 failures** |

لم يُكتب أي corpus جديد إلى المستودع؛ ملف الـ corpus الوحيد الموجود (`posting/testdata/fuzz/FuzzReversalNetImpactIsZeroPerAccount/68e9ff99a56b7025`) بقي كما هو.

### 4.8 التغطية — PASS (مطابقة رقمياً للتقرير المحفوظ)

```text
$ go test ./internal/kernal/... -count=1 -coverprofile=<artifacts>/kernel.cover
$ go tool cover -func=<artifacts>/kernel.cover
total: (statements) 93.1%
$ COVERAGE_MIN=90 ./scripts/check_coverage.sh <artifacts>/kernel.cover
total coverage: 93.1% (minimum 90%)   EXIT=0
```

| الحزمة | المقاس الآن | التقرير المحفوظ |
| --- | ---: | ---: |
| `apperrors` | 100.0% | 100.0% |
| `ledger` | 100.0% | 100.0% |
| `types` | 98.3% | 98.3% |
| `governance` | 98.4% | 98.4% |
| `policy` | 97.1% | 97.1% |
| `journal` | 94.1% | 94.1% |
| `valuation` | 92.3% | 92.3% |
| `masterdata` | 91.8% | 91.8% |
| `posting` | 89.3% | 89.3% |
| **الإجمالي** | **93.1%** | **93.1%** |

### 4.9 التتبع الاسمي — PASS (اسمياً فقط)

```text
مراجع Test/Fuzz الفريدة في KERNEL_TRACEABILITY_MATRIX.md : 66
أسماء Test/Fuzz الفريدة في *_test.go                     : 160
مراجع المصفوفة بلا تعريف مطابق                           : 0
```

هذا يثبت المطابقة الاسمية التي ادّعاها التقرير المحفوظ، ولا يثبت كفاية الـ assertions دلالياً (انظر G-012).

### 4.10 فحوص إضافية سجلت كدليل

| الأمر | النتيجة | ملاحظة |
| --- | --- | --- |
| `./scripts/check_api_compat.sh` | PASS | لا إزالة أو تغيير توقيع في السطح العام |
| `./scripts/verify_integrity.sh` | **FAIL** (فحصان) | 27 ملف Go غير منسق **خارج** النواة؛ فحص `doc.go` يُعلّم 10 مجلدات من ضمنها `internal/kernal/apperrors` ومجلدات `testdata` (سلبيات خاطئة محتملة). التفاصيل في G-007/G-008 |
| أدوات Mutation (`go-mutesting`, `mutesting`, `go-mutation`, `gremlins`, ...) | **غير مثبتة** | `go tool` لا يعرض أداة طفرات؛ لا يمكن إعادة إنتاج نسبة 100% آلياً (G-005) |
| `go tool cover -func=coverage.out` (الملف الموجود في الجذر) | **52.7%** | ملف تغطية عام قديم/مختلف النطاق؛ ليس دليل تغطية النواة (G-006) |

---

## 5. مقارنة والتفاوتات المسجلة

| الادعاء المحفوظ | ما أُعيد تشغيله | الحكم |
| --- | --- | --- |
| Go 1.26.3 و`toolchain go1.26.3` | `go version go1.26.3` | مطابق |
| 78 ملفاً في النواة | 78 ملفاً | مطابق |
| 160 اختباراً فريداً | 160 | مطابق |
| `go test` و`-race` يمران | PASS | مطابق (بشكل مستقل) |
| 11 قاعدة معمارية بصفر مخالفات | PASS | مطابق (بشكل مستقل) |
| تغطية 93.1% و Per-package | مطابق رقمياً | مطابق |
| 9 أهداف Fuzz / ~5.99M تنفيذ | 9/9 PASS / ~5.94M تنفيذ | **مطابق في النتيجة، مختلف في العدد** — عدد التنفيذات غير حتمي ويعتمد على الـ corpus والآلة؛ لا يُعدّ انحرافاً |
| 8/8 طفرات مقتولة (100%) | لا أداة آلية، لا قصاصات، لا سجل تشغيل | **غير معاد إنتاجها** (G-005) |
| 66 مرجعاً في مصفوفة التتبع، صفر مفقود | 66 / 0 | مطابق اسمياً فقط (G-012) |
| "12 حالة تكافؤ محولات PASS" | PASS، لكن تشغيل الاختبار أعاد كتابة تقريرين متتبعين | مطابق في النتيجة؛ فجوة نظافة Working Tree (G-010) |
| إجمالي التبعيات 309 | 312 | **تفاوت غير مفسر**؛ internal=81 وmodule edges=43 مطابقان (G-009) |
| `coverage.out` في الجذر | 52.7% (تغطية كل المستودع) | **مضلل** إن استُخدم كدليل للنواة (G-006) |
| بوابة `verify_integrity.sh` غير مذكورة في تقارير v12 | FAIL | فجوة خارج نطاق النواة (G-007) |

---

## 6. ملخص سجل الفجوات

سُجّلت **15 فجوة** في [`KERNEL_REMEDIATION_GAP_REGISTER.md`](./KERNEL_REMEDIATION_GAP_REGISTER.md):

- **Critical:** 0
- **High:** 4 — G-001 (عقد idempotency)، G-002 (ترتيب الأقفال مقابل القفل الفعلي)، G-003 (خريطة الحزم في الوثيقة المعمارية)، G-004 (مصدر حقيقة GL).
- **Medium:** 7 — G-005، G-006، G-007، G-011، G-012، G-013، G-014.
- **Low:** 4 — G-008، G-009، G-010، G-015.

لا فجوة Critical تمنع اعتماد خط الأساس نفسه؛ كل الفجوات مرتبطة بمراحل لاحقة أو بمالكي مكونات خارج النواة.

---

## 7. الأوامر الموحدة وإعادة الإنتاج

```bash
# خط الأساس الكامل (الافتراضي: fuzztime=30s لكل هدف)
./scripts/verify_kernel_baseline.sh /tmp/opencode/phase0

# تشغيل سريع للتحقق من السكربت فقط
FUZZTIME=1s ./scripts/verify_kernel_baseline.sh /tmp/opencode/phase0/script_smoke
```

السكربت:

1. يسجل بصمة Git وGo ووقت الفحص في `logs/environment.txt`.
2. يشغّل: gofmt، unit، race، المستودع الكامل، البوابة المعمارية، وحدة المستهلك الخارجي، 9 أهداف Fuzz، والتغطية.
3. يكتب كل مخرجاته خارج المستودع ولا يعدّل ملفات متتبعة؛ ينسخ تقريرَي التكافؤ احتياطياً ثم يعيدهما بعد تشغيل اختبارات المستودع.
4. مُختبَر من البداية إلى النهاية بنجاح (`RESULT: PASS`) باستخدام `FUZZTIME=1s`، وبالمدة الكاملة 30s للـ fuzz أُنتجت نتائج القسم 4.

السجلات الخام: `/tmp/opencode/phase0/logs/*`، ونسخة نظيفة من تقريرَي التكافؤ بعد التشغيل: `/tmp/opencode/phase0/logs/*.fresh.md`. هذه المسارات مؤقتة؛ الأرقام الجوهرية مُضمّنة في هذه الوثيقة.

---

## 8. قرار بوابة المرحلة 0

| معيار القبول | الحكم |
| --- | :---: |
| إصدار Go مطابق للمطلوب | PASS |
| جميع أوامر التحقق موثقة بنتائج فعلية | PASS |
| جرد الحزم والملفات والاختبارات موجود | PASS |
| كل تفاوت بين الوثائق والتشغيل مسجل | PASS |
| لا تعديل منطق مجال قبل اعتماد الخط الأساس | PASS |

**القرار:** **PASS** — يُعتمد خط الأساس هذا كنقطة مقارنة إلزامية للمراحل 1–12. لا يُعتبر أي ادعاء وارد في تقارير v12 المُعادة تحققُه إلا إذا أُعيد تشغيله ووُثّق في مرحلته المالكة. لا تبدأ المرحلة 1 إلا بعد اعتماد هذا التقرير وتحديث مالكي الفجوات.
