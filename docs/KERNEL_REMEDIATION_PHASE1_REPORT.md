# تقرير المرحلة 1 — حسم المعمارية ومصدر الحقيقة

- **رمز التقرير:** `AE-REMED-PHASE1-REPORT`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)
- **خط الأساس:** [`KERNEL_REMEDIATION_BASELINE.md`](./KERNEL_REMEDIATION_BASELINE.md)
- **سجل الفجوات:** [`KERNEL_REMEDIATION_GAP_REGISTER.md`](./KERNEL_REMEDIATION_GAP_REGISTER.md)
- **تاريخ التنفيذ (UTC):** 2026-11-21T21:12:59Z
- **Commit المرجعي:** `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **إصدار Go الفعلي:** `go version go1.26.3 linux/amd64`
- **قرار البوابة:** **PASS**

---

## 1. الهدف والنطاق

نفّذت هذه المرحلة المهام 1–14 من المرحلة 1 حصراً: حسم خريطة الحزم وحدود المسؤولية، ومصدر الحقيقة المحاسبي، وعلاقة `journal`/`ledger`/`masterdata`، وتحديث الوثائق والـ charters، ووسم الخطط القديمة المتعارضة بـ Superseded.

**لم يُنفّذ:** Multi-Book، ولا تقارير أو أرصدة مادية داخل `internal/kernal`، ولا تغيير في منطق المجال أو السطح البرمجي العام، ولا حذف نوع أو حقل أو alias، ولا حزمة تنفيذية جديدة.

---

## 2. تنفيذ مهام المرحلة 1

| # | المهمة | النتيجة | الدليل |
| ---: | --- | --- | --- |
| 1 | خريطة فعلية للحزم التسع | أُنجزت | [`KERNEL_ARCHITECTURE_MAP.md`](./KERNEL_ARCHITECTURE_MAP.md) القسم 1: ملفات ومسؤوليات تسع حزم + جذر `doc-only` |
| 2 | Dependency Graph من `go list` | أُنجز | الخريطة القسم 2 + إخراج `go list` أدناه؛ DAG بلا دورات |
| 3 | مسؤولية `journal` | حُسمت | تعريف aggregate فقط؛ لا تحقق توازن ولا انتقال حالة ولا حفظ. `journal/doc.go`، `TARGET_ACCOUNTING_KERNAL.md:204-209`، ADR P1-01 |
| 4 | مسؤولية `ledger` | حُسمت | DTO مرجعي لـ `pkg/api/v1/ledger` فقط؛ ليس GL ولا مخزن حركة. `ledger/doc.go`، ADR P1-03 |
| 5 | مصدر حقيقة GL وCanonical | حُسم | Posted `journal.JournalEntry` + `EntryLine`؛ `account_moves` + `account_move_lines` في PostgreSQL أو `JournalEntry` واحدة في الذاكرة. [ADR P1-02](./ard/P1_02_gl_source_of_truth.md) |
| 6 | الأرصدة والتقارير | حُسم | مشتقات مضيفة قابلة لإعادة البناء؛ لا `Materialized Balance Engine` في النواة. `TARGET_ACCOUNTING_KERNAL.md:46,230,274` |
| 7 | العلاقة الرسمية | وُثّقت | مخطط `JournalEntry → posted facts → host executor → derived balances → reports` في `TARGET_ACCOUNTING_KERNAL.md:219-227` وADR P1-02 |
| 8 | مفاتيح النطاق | حُسمت | `CompanyID` هو مفتاح النطاق الحالي؛ لا `BookID`/`LedgerID` في عقد الترحيل؛ Multi-Book يحتاج ADR منفصل. ADR P1-02 القسم 4 |
| 9 | `masterdata.Account` مقابل `ledger.Account` | وُثّق | `masterdata` مصدر الحقيقة؛ `ledger` DTO توافق؛ قواعد التحويل في [ADR P1-03](./ard/P1_03_reference_model_duality.md) |
| 10 | `masterdata.FiscalPeriod` مقابل `ledger.FiscalPeriod` | وُثّق | `masterdata` مصدر التقويم والفترات 13/14؛ `ledger.Contains` ليس تحققاً للترحيل. ADR P1-03 |
| 11 | مراجعة P7-01 | أُنجزت | الخريطة القسم 4 تثبت: `pkg/api/v1` هو الجسر الوحيد، `pkg/spi` ممنوع من `internal`، `pkg/sdk` تفصيل تنفيذي. لم تُضعف القواعد `R2a–R2c` |
| 12 | تحديث Target Kernel | أُنجز | [`TARGET_ACCOUNTING_KERNAL.md`](./TARGET_ACCOUNTING_KERNAL.md) الإصدار 1.1.0: تسع حزم، مخططات Mermaid، مسارات فعلية، وتصحيح ادعاء `SELECT FOR UPDATE` إلى "المحول يطبق الأقفال" |
| 13 | ADRs | أُنشئت | [P1-01](./ard/P1_01_kernel_package_map.md)، [P1-02](./ard/P1_02_gl_source_of_truth.md)، [P1-03](./ard/P1_03_reference_model_duality.md) |
| 14 | الوثائق القديمة | وُسمت | Superseded على 6 وثائق: 3 خطط تحت `docs/` و3 تقارير تحت `internal/kernal/` |

---

## 3. القرارات المعمارية المُثبّتة

1. **خريطة الحزم:** تسع حزم تنفيذية + حزمة جذر `internal/kernal` للتوثيق والفحوص؛ لا حزمة عشرية في هذه المرحلة.
2. **Posting:** `posting.PostEntry` هو المسار الوحيد للتحقق والترحيل؛ يفوض الحفظ الذري إلى `PostingExecutor`/`PostingReversalExecutor` المقدمين من المضيف.
3. **Journal:** يملك تعريف `JournalEntry` و`EntryLine` والحالات والروابط و`Clone` الدفاعي، ولا يملك تحققاً أو حفظاً.
4. **Ledger:** جسر DTO للتوافق العام فقط؛ لا يملك سجل GL ولا رصيداً ولا تحقق أهلية.
5. **Masterdata:** المصدر المخول لـ COA والشركة واليومية والتقويم والأبعاد.
6. **Canonical GL:** القيود المرحلة (`posted`/`reversed`) مع أسطرها؛ الأرصدة والتقارير إسقاطات مشتقة خارج النواة.
7. **إعادة البناء:** ترتيب كلي `(EffectiveDate UTC, SequenceNumber, JournalEntryID.String())` ثم `(EntryLine.ID.String())` داخل نطاق `CompanyID`؛ لا اعتماد على ترتيب Map. المواصفة موثقة، والإثبات التنفيذي مؤجل إلى المرحلتين 4 و12.
8. **النطاق:** `CompanyID` اليوم؛ لا Multi-Book في هذه المرحلة.
9. **P7-01:** حدود السطح العام باقية كما هي، بدون أي استثناء جديد.

---

## 4. الملفات المتأثرة

### 4.1 الوثائق المعمارية وقرارات ADR

- `docs/KERNEL_ARCHITECTURE_MAP.md` (جديد).
- `docs/TARGET_ACCOUNTING_KERNAL.md` (تحديث إلى 1.1.0).
- `docs/ard/P1_01_kernel_package_map.md` (جديد).
- `docs/ard/P1_02_gl_source_of_truth.md` (جديد).
- `docs/ard/P1_03_reference_model_duality.md` (جديد).
- `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` (تحديث المرحلة 1؛ الفجوات `G-003`، `G-004`، `G-008`، `G-016`، `G-017`).

### 4.2 Go doc comments / charters (بلا منطق)

- `internal/kernal/doc.go`.
- `internal/kernal/apperrors/doc.go` (جديد — إغلاق `G-008`).
- `internal/kernal/journal/doc.go`.
- `internal/kernal/ledger/doc.go`.
- `internal/kernal/posting/doc.go`.
- `internal/kernal/governance/doc.go`.
- `internal/kernal/masterdata/doc.go`.
- `internal/kernal/policy/doc.go`.
- `internal/kernal/valuation/doc.go`.

### 4.3 وسم الوثائق المتعارضة كـ Superseded

- `docs/KERNAL_V12_FINAL_STABILIZATION_EXECUTION_PLAN.md`.
- `docs/KERNEL_FINAL_EXECUTION_PLAN_V10.md`.
- `docs/KERNEL_AUDIT_AND_COMPLETION_PLAN.md`.
- `internal/kernal/KERNEL_COMPLETION_REPORT.md`.
- `internal/kernal/KERNEL_RELEASE_BASELINE.md`.
- `internal/kernal/KERNEL_V12_CHANGELOG.md`.

> مخرجات المرحلة 0 (`docs/KERNEL_REMEDIATION_BASELINE.md`، `docs/KERNEL_REMEDIATION_EXECUTION_PLAN.md`، سجل الفجوات، و`scripts/verify_kernel_baseline.sh`) ما زالت غير ملتزمة في شجرة العمل وستظهر ضمن فحص الفرق النهائي؛ لم يُنشأ commit أو push في هذه المرحلة.

---

## 5. الأدلة والاختبارات المعاد تشغيلها

| الأمر | النتيجة | ملاحظات |
| --- | --- | --- |
| `gofmt -l internal/kernal/` | no output / exit 0 | لا ملفات غير منسقة داخل النواة |
| `go test ./internal/kernal/... -count=1` | `ok` — 10/10 حزم | root, apperrors, governance, journal, ledger, masterdata, policy, posting, types, valuation |
| `go test -race ./internal/kernal/... -count=1` | `ok` — 10/10 حزم | لا بيانات سباق |
| `./scripts/verify_architecture.sh` | `RESULT: PASS - clean architectural base` | 11/11: R1، R2a، R2b، R2c، R3a–R3d، R4، R5، R6 |
| `go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./internal/kernal/...` | DAG بلا دورات | `journal/ledger/masterdata/governance → types`؛ `policy → journal+masterdata+types`؛ `posting → journal+masterdata+policy+types`؛ `valuation → journal+posting+types`؛ `apperrors` معزول |
| `git diff --check` | no output / exit 0 | لا أخطاء مساحات أو نهاية سطر في التعديلات المتتبعة |
| `git status --short` | تعديلات جديدة وغير ملتزمة | لا commit/push |

### إخراج البوابة المعمارية (11/11)

```text
[OK] R1  internal/* free of net/http, grpc and protobuf
[OK] R2a pkg/spi/* free of internal/* imports
[OK] R2b pkg/api/* internal imports limited to internal/kernal/*
[OK] R2c exported API free of unbridged internal/* type exposure
[OK] R3a internal/kernal/* free of internal/orbitals/* imports
[OK] R3b internal/kernal/* free of pkg/* imports
[OK] R3c internal/kernal/* free of concrete storage/* imports
[OK] R3d internal/kernal/* free of database/sql and pgx
[OK] R4  no cmd/ or proto/ directories
[OK] R5  go.mod free of grpc, protobuf, aws-sdk and net/http
[OK] R6  package graph resolves without import cycles
RESULT: PASS - clean architectural base
```

---

## 6. حالة الفجوات بعد المرحلة 1

| الفجوة | الحالة | الملخص |
| --- | --- | --- |
| `G-003` | **Closed** | خريطة الحزم والمسارات صُححت؛ 9 حزم + جذر؛ بوابة معمارية 11/11 |
| `G-008` | **Closed** | أُضيف `apperrors/doc.go` مع charter؛ الاختبارات والتنسيق نظيفة |
| `G-004` | **Partially Closed** | القرار والتوثيق مغلقان؛ إثبات Rebuild التنفيذي مُؤجل إلى 4/12 |
| `G-001` | Open | `IdempotencyKey` يُتحقق منه في `pipeline.go:135-138` لكنه غير موجود في `PostingCommit`؛ المعالجة في المرحلة 7 |
| `G-002` | Open | ترتيب المفاتيح لا يحجز أقفالاً؛ المرحلة 8 |
| `G-016` | Open (جديدة) | انحراف `masterdata` DTOs مقابل `ledger`؛ المرحلة 3/11 |
| `G-017` | Open (جديدة) | تعارض أسماء روابط العكس بين kernel والوثائق/trigger؛ المرحلة 4/7/11 |
| بقية الفجوات | Open | محالة إلى مراحلها المالكة في السجل |

---

## 7. المخاطر المتبقية

1. **إثبات Rebuild:** الترتيب الحتمي موثق كمواصفة فقط؛ لا يوجد اختبار تنفيذي بعد، ولذلك لم تُغلق `G-004` بالكامل.
2. **Idempotency:** وجود `IdempotencyKey` في خيار النواة لا يثبت منع التكرار؛ `PostingCommit` لا يحمل المفتاح، والضمان في المضيف/المحول (المرحلة 7).
3. **الأقفال:** `ExtractAndSortAccountIDs` ينتج ترتيباً فقط ولا يحجز قفلاً (المرحلة 8).
4. **انحراف النماذج المرجعية:** لا أتمتة تحقق بين `masterdata` و`ledger` (المرحلة 3/11).
5. **تعارض تسمية العكس:** قيود PostgreSQL تبحث عن `ReversedEntryID`/`OriginalEntryID` بينما JSON القادم من kernel يحمل `ReversedByID`/`ReversalOfID`؛ لم يُختبر تنفيذياً في هذه المرحلة (المرحلة 4/7/11).
6. **أدلة المرحلة 0 غير الملتزمة:** خط الأساس والخطة والسجل والسكربت ما زالت ملفات غير متتبعة في شجرة العمل؛ يجب إدراجها مع المرحلة 1 في مراجعة التغيير النهائية.

---

## 8. قرار البوابة

بناءً على:

- تطابق الوثائق مع التنفيذ الفعلي؛
- وجود ثلاثة ADRs مقبولة؛
- وسم ست وثائق قديمة متعارضة بـ Superseded؛
- عدم تغيير منطق المجال أو السطح العام؛
- نجاح `go test` و`go test -race` لكامل النواة؛
- نجاح `gofmt`؛
- نجاح `verify_architecture.sh` بـ11 قاعدة دون تخفيض؛
- خلو `git diff --check` من الأخطاء؛

**قرار بوابة المرحلة 1: PASS.**

لا تُبدأ المرحلة 2 قبل إدراج هذه المخرجات في مراجعة التغيير واعتمادها. تبقى الفجوات `G-004` (جزئياً) و`G-016` و`G-017` مفتوحة في سجل الفجوات بمالكيها ومراحلها المحددة.
