# تقرير المرحلة 11: تكافؤ محولات التخزين والعقود

- **رمز المرحلة:** P11
- **التاريخ:** 2026-11-22
- **المنفذون المستهدفون:** Memory، PostgreSQL (تشغيل فعلي بـ DSN من الـ harness، بلا تخطٍ)
- **حالة البوابة:** **PASS** — كل بوابات المرحلة نُفّذت وأعطت نتائج قابلة لإعادة التشغيل

## النطاق المنفذ

عقود `pkg/spi/storage` و`pkg/spi/masterdata`، اختبارات التكافؤ المشتركة على المحولين، مسار SDK السياسات/idempotency، وإغلاق الفجوات `G-016` و`G-019`–`G-022` و`G-024`. لم يُنشأ محول Oracle، ولم تتوسّع النواة المحاسبية بمنفذ جديد.

## مهام المرحلة

| المهمة | التنفيذ | النتيجة |
|---|---|---:|
| 1-2. جرد العقود وسلوك النجاح/الفشل/التكرار/التزامن/الإلغاء | `storage.PortCatalog()` قائمة مرجعية آلية تغطي كل حقول `Ports`؛ `docs/KERNEL_CONTRACT_TEST_SUITE.md` يوثّق السلوك | PASS |
| 3-4. Suite مستقلة وتشغيلها على المحولين | `test/contract_parity` عبر `ForEachStorageAdapter`، تقرير مولّد آلياً من التشغيل الكامل فقط | PASS |
| 5. Company Scope على كل Repository | `TestPhase11_CompanyScopeOnEveryMasterDataPort` + اختبارات عزل G-019/G-022 | PASS |
| 6. Posting Atomicity وIdempotency وReversal Atomicity | سيناريوهات Phase 7 المشتركة + `TestPhase11_Revaluation*` | PASS |
| 7. Audit Append وSnapshot وCheckpoint | سيناريوهات Phase 10 المشتركة + `RevaluationAuditAction` | PASS |
| 8. الأقفال والتعارضات وإعادة المحاولة | `BoundedRetryOnUnitOfWork` و`TestPhase8_*` مع `go test -race` | PASS |
| 9. rollback وunknown outcome وانقطاع الاتصال | `ConnectionInterruptionBeforeCommit` و`RevaluationFaultAfterCommitReplay` | PASS |
| 10. توافق إصدار العقد وإصدار Adapter | `StorageContractVersion` + `ValidateAdapterCapabilities` كلاهما PASS | PASS |
| 11-12. منع تسرب `internal` ومنع اعتماد Kernel على SQL | `verify_architecture.sh`: R1–R6 PASS | PASS |
| 13-14. توثيق الفروقات وإصدار تقرير التكافؤ | تقرير مولّد من 45 سيناريو فريد × محولين = 90 صف نتيجة PASS، بلا فروق دلالية غير معلَنة | PASS |
| 15. `G-019`/`G-022` | منافذ عامة اختيارية + تنفيذ في المحولين + ربط SDK انتقائي | PASS |
| 16. `G-020`/`G-021` | مخزن سياسات مؤرخ + تحميل كل الإصدارات + عدم ابتلاع أخطاء التسجيل | PASS |
| 17. `G-024` | عملية `revaluation` مستقلة داخل نطاق idempotency الحالي | PASS |

## التغييرات المنفذة

### عقود ومحولات

- `storage.PortCatalog()` و`ValidateContractVersion()` و`ValidateAdapterCapabilities()`.
- `storage.PolicyStore` و`PolicyVersion` و`PostingPolicyPayload`/`CurrencyPolicyPayload` و`ErrPolicyVersionConflict`/`ErrPolicyVersionNotFound` عبر `Ports.Private`؛ لا توسيع لـ `storage.Ports` المستقرة.
- `storage.OperationRevaluation` كعملية ثالثة؛ `PostEntry` و`PostingCommit` لم يتوسّعا.
- منافذ `pkg/spi/masterdata` للنسخة النافذة ومراكز التكلفة والأبعاد، مع aliases عامة في `pkg/api/v1/types`؛ أنواع `internal/kernal` لا تظهر في التوقيعات العامة.
- `BalanceReader` يطبّق `asOf` على `posting_date` مع رجوع إلى `entry_date`، ويحل العملة الفارغة إلى العملة الوظيفية في المحولين.
- هجرتان جديدتان فقط: `202611220003_phase11_account_versions_full.sql` و`202611220004_policy_version_windows.sql`؛ لم تعدَّل أي هجرة موقّعة سابقة.

### SDK

- `ensurePolicies`: متجر السياسات هو المصدر عندما يحتوي نسخاً؛ تُحمَّل كل النسخ مرة واحدة لكل شركة، وتفشل النوافذ غير المغطاة مغلقاً، ولا أخطاء مُبتلعة في مسار التوافق القدامى. عند ظهور أول نسخة تُمسَح السياسات الاصطناعية، وأي فشل أثناء التسجيل يُعاد مع مسح ما سُجِّل قبله حتى تبقى إعادة المحاولة ممكنة. متجر بصفر نسخة لشركة يعني أنها لم تشترك بعد؛ يُسمح حينها بالتوافق القدامى، وبمجرد أول نسخة يصبح المتجر مصدر الحقيقة.
- `commitIdempotency`: يحفظ نطاق النواة الخاتم بما فيه `revaluation`، مع مسار توافق للمتصلين القدامى. لا يوجد use-case عام لإعادة التقييم في SDK؛ إثبات `G-024` على مستوى SPI والمحولات فقط، كما وُثِّق في ADR P11-01.
- `classifyError`: تعارض النسخ/السياسات/إعادة التقييم ← `ErrConflict`؛ غياب النسخة أو عدم نفاذها ← `ErrNotFound`.
- ربط انتقائي لقدرات master data؛ غيابها يُبقي الفشل المغلق.

### أدلة ووثائق

- ADRs: `P11_01_revaluation_idempotency_spi.md`، `P11_02_effective_account_and_dimensions.md`، `P11_03_balance_reader_semantics.md`، `P11_04_versioned_policy_store.md`.
- تحديث `docs/api/v1.snapshot` بوابة `--update` صريحة بعد استقرار السطح العام.
- تحديث `KERNEL_CONTRACT_TEST_SUITE.md` و`KERNEL_IDEMPOTENCY_CONTRACT.md` و`SDK_ARCHITECTURE_AND_INTERNALS.md` و`KERNEL_REMEDIATION_GAP_REGISTER.md`.

## الأدلة (قابلة لإعادة التشغيل)

```bash
go test ./... -count=1
go test -race ./... -count=1
./scripts/verify_architecture.sh
./scripts/check_api_compat.sh --update   # تحديث مقصود للسطح العام
./scripts/check_api_compat.sh
gofmt -l $(git diff --name-only -- '*.go'; git ls-files --others --exclude-standard -- '*.go')
git diff --check
```

النتائج المسجّلة:

- `go test ./... -count=1`: **PASS** (exit 0، صفر أسطر `FAIL`) على كل الحزم، بما فيها `storage/postgres` و`test/contract_parity` بتشغيل PostgreSQL فعلي.
- `go test -race ./... -count=1`: **PASS** (exit 0، صفر أسطر `FAIL`).
- `./scripts/verify_architecture.sh`: **PASS** — R1–R6 خضراء، لا اعتماد من `internal/kernal` على SQL/Driver أو `pkg/*`.
- `./scripts/check_api_compat.sh`: **PASS** بعد تحديث مقصود للقاعدة؛ لا removals غير معلَنة.
- `gofmt -l` لكل ملفات Go المعدَّلة/الجديدة: لا إخراج. `git diff --check`: لا إخراج.
- [تقرير التكافؤ المولّد](./reports/contract_parity_report.md): **45 سيناريو فريد × 2 محول = 90 صف PASS، 0 FAIL، 0 SKIP**؛ كل سيناريو PASS على Memory وPostgreSQL. لا فروق دلالية معلَنة ولا فروق غير متوقعة.

> تقرير التكافؤ لا يُحدَّث يدوياً؛ `TestMain` يمنع الكتابة من تشغيلات `-test.run` الفرعية، ويُعاد توليده بالكامل من `go test ./... -count=1`.

## فروق القدرات المقبولة

| المحول | Durability | Foreign Keys | Savepoints | Serializable |
|---|---:|---:|---:|---:|
| Memory | false | false | false | true |
| PostgreSQL | true | true | false | true |

هذه فروق قدرات موثّقة، وليست اختلافاً دلالياً في السيناريوهات المشتركة.

## الفجوات المغلقة

`G-016` (جسر ledger العام)، `G-019` (النسخة النافذة)، `G-020` (سجل السياسات المؤرخ)، `G-021` (أخطاء التسجيل)، `G-022` (مراكز التكلفة والأبعاد)، `G-024` (idempotency إعادة التقييم). التفاصيل في [سجل الفجوات](./KERNEL_REMEDIATION_GAP_REGISTER.md).

## المخاطر المتبقية

- Savepoints غير مدعوم من المحولين؛ العقود الحالية لا تفرضه.
- `RevaluationRegistry` لا يزال ذاكرياً في النواة؛ الديمومة على مستوى التخزين مثبتة عبر `G-024`، ولا يوجد مسار عام جديد لإنشاء قيد إعادة تقييم في SDK.
- `G-004` و`G-006` و`G-007` و`G-012` خارج بوابة المرحلة 11 (مرحلة 12 أو مالك المستودع أو مراجعة بشرية).

## بوابة القبول

**القرار: PASS.** جميع بوابات المرحلة 11 نُفّذت بنتائج فعلية قابلة لإعادة التشغيل؛ لا `PASS-SCOPED`، ولا تخطي PostgreSQL، ولا أدلة مستمدة من تشغيلات فرعية.
