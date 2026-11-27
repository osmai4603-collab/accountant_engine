# تقرير اختبار العزل متعدد الشركات — Multi-Company Isolation Test Report

- **رمز الوثيقة:** `AE-P8-ISOLATION-REPORT`
- **المرحلة:** 8 — التزامن والعزل متعدد الشركات
- **التاريخ:** 2026-11-22
- **المحولات المختبرة:** `storage/memory` و`storage/postgres`
- **القرار:** [ADR P8-01](../architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md#adr-p8-01)
- **العقود:** [قفل](../contracts/KERNEL_LOCKING_CONTRACT.md) · [تزامن](../contracts/KERNEL_CONCURRENCY_MODEL.md) · [معاملة](../contracts/KERNEL_POSTING_TRANSACTION_CONTRACT.md)

---

## 1. الغرض

إثبات أن شركة لا ترى قيود شركة أخرى، أو سجل idempotency خاص بها، أو رصيداً، أو بيانات رئيسية، سواء كانت الطلبات متسلسلة أو متداخلة، وسواء جاءت القراءة عبر منفذ `storage.Ports` أو عبر SQL مباشر في PostgreSQL.

## 2. منهجية الاختبار

- كل حالة تستخدم `ForEachStorageAdapter` مع شركة أو شركتين مُهيَّأتين في مخطط/ذاكرة معزولين.
- الشركة B تُنشأ داخل نفس المحول عبر `ProvisionFixture` أو `phase8ProvisionCompany`؛ لا تُعاد تهيئة المحول بين الشركتين.
- الطلبات تُطلق في goroutines منفصلة مع `context.WithTimeout` للكشف عن الجمود.
- مراقب قفل مستقل لكل طلب يمنع خلط آثار الطلبات المتزامنة في سجل واحد.

## 3. مصفوفة السيناريوهات

| السيناريو | المحولات | ما تم التحقق منه | النتيجة |
|---|---|---|---|
| `InterleavedCompanyIsolation` | memory + postgres | قيدان لشركتين متداخلان؛ كل شركة ترى سجلها فقط؛ الأرصدة منفصلة؛ أقفال كل طلب داخل شركة الطلب | PASS |
| `CompanyScopeOnEveryPort` | memory + postgres | `EntryReader` و`IdempotencyLookup` و`BalanceReader` و`MasterDataReader` ترفض/لا تجد بيانات الشركة الأخرى | PASS |
| `ScopeViolationRejected` | memory + postgres | نطاق idempotency بشركة أخرى يُرفض بـ `ErrScopeViolation`، ولا يُكتب قيد ولا سجل مفتاح | PASS |
| `FrozenSnapshotRemainsStable` | memory + postgres | إضافة فترة مالية جديدة بعد بدء الطلب لا تُغيّر `FiscalPeriodID` أو التواريخ المختومة في القيد | PASS |
| `CanonicalCompanyLockOrder` | SPI | هوية القفل تشمل الشركة، والتوحيد والترتيب لا يعدّلان الإدخال | PASS |
| `TestPhase8PostgresRLSRuntimeIsolation` | postgres فقط | RLS يخفي صفوف الشركة A عن استعلام SQL مباشر تحت سياق B، والعكس، بمستخدم غير superuser | PASS |

## 4. تفاصيل عزل PostgreSQL

`storage/postgres/phase8_isolation_test.go`:

1. ينشئ شركتين في مخطط اختبار معزول، لكل منهما يومية وفترة وحسابان.
2. يرحّل قيداً لكل شركة عبر `storage.WithCompanyContext` و`UnitOfWork.CommitPosting` مع نطاق idempotency يعتمد على الشركة.
3. يتحقق أن `driver.GetEntry(ctx, companyB, entryA.ID)` لا يعيد قيد الشركة A.
4. ينشئ دوراً غير `superuser` يملك `SELECT` فقط على مخطط الاختبار؛ المستخدم الافتراضي في بيئة الاختبار قد يتجاوز RLS حتى مع `FORCE ROW LEVEL SECURITY`.
5. داخل معاملة SQL مباشرة:
   - `SET LOCAL ROLE <test-role>`
   - `set_config('app.current_company_id', <viewer>, true)`
   - `SELECT COUNT(*) FROM account_moves WHERE company_id = <target>`
6. النتائج:

| سياق العرض (`viewer`) | الهدف (`target`) | الصفوف المرئية | التوقع |
|---|---|---:|---|
| Company A | Company A | 1 | يرى قيده فقط |
| Company A | Company B | 0 | لا تسريب |
| Company B | Company B | 1 | يرى قيده فقط |
| Company B | Company A | 0 | لا تسريب |

7. استعلام بصلاحيات superuser يؤكد وجود صفين فعلياً؛ الصفر ناتج عن RLS لا عن غياب البيانات.

## 5. ما لا يعتبر دليلاً كافياً

- وجود سياسة RLS في ملف SQL وحده لا يثبت سلوك وقت التشغيل؛ لذلك أُضيف اختبار الدور غير المتميز.
- تحقق النواة من `CompanyID` وحده لا يثبت أن منافذ القراءة في المحول آمنة؛ لذلك اختُبرت المنافذ الأربعة صراحةً.
- تطابق نص المفتاح في شركتين لا يثبت العزل؛ لذلك استُخدم مفتاح idempotency مختلف لكل شركة، واختُبر رفض النطاق المزوّر.
- `ActorID` ليس تفويضاً؛ لا يُستخدم لاشتقاق الشركة في أي مسار.

## 6. أثر `ErrScopeViolation`

| الطبقة | السلوك |
|---|---|
| SPI | `storage.ErrScopeViolation` خطأ محايد، `IsRetryable = false` |
| Adapter | يُرفض قبل أي كتابة أو حجز قفل؛ لا يترك سجل idempotency ولا قيداً |
| SDK | `classifyError` يترجمه إلى `publicapplication.ErrForbidden` |
| Host | لا يعيد المحاولة؛ يُسجّل الحدث كخرق نطاق ويرفض الطلب |

## 7. الأدلة المولّدة

- `docs/reports/contract_parity_report.md`: تشغيل كامل للمستودع، **90 نتيجة PASS (45 سيناريو × محولين)** بعد إضافة سيناريوهات المرحلة 8؛ رقم 42 (21 × 2) كان رقماً قديماً قبل التوسيع.
- `go test -race ./... -count=1`: PASS لكل الحزم بما فيها `test/contract_parity` و`storage/postgres`.
- `./scripts/verify_architecture.sh`: PASS 11/11؛ لا SQL ولا `pgx` داخل `internal/kernal`.

## 8. الحكم

**PASS — لا تسريب بيانات بين الشركات في المحولين المختبرين.** تحقق النواة يبقى الخط الأول، وعزل المحول/RLS هو خط الدفاع الثاني؛ لا يُستبدل أحدهما بالآخر.
