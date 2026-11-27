# وثيقة الاعتماد والتوقيع النهائي للإصدار (Release Sign-off) — `v1.1.0-enterprise`

- **تاريخ الاعتماد:** 2026-11-28
- **الحالة النهائية:** **`GO` — معتمد للجاهزية المؤسسية** (يحل محل الإصدار السابق `v1.0.0` الذي كان بحالة `SUPERSEDED`)
- **النطاق المعتمد:** النواة المحاسبية (`internal/kernal`), العقود العامة (`pkg/api`, `pkg/spi`, `pkg/sdk`), محولا التخزين (`storage/memory`, `storage/postgres`), المدارات المنشورة (`consolidation`, `reconciliation`, `reporting`, `subledgers`, `events`), ومعايير التقارير (`IFRS 9`, `IFRS 15`, `IFRS 16`, `IAS 12`).

---

## 1. ملخص إغلاق الفجوات (Phases 1–5)

1. **العقود العامة (`Public API / SPI`):**
   - إكمال عقد فترات التسوية (13 و14) عبر `pkg/spi/masterdata.FiscalPeriodNumberReader`.
   - إضافة عقد إلغاء المسودة (`ActionCancel`, `CancelDraftCommand`, `DraftCancellationService`) وتصديره عبر `pkg/sdk.DraftCancellation(engine)`.
   - ربط `YearEndClosingService` عبر `pkg/sdk.YearEndClosing(engine)` بدعم الإقفال الذري وإعادة الفتح المعتمدة.
   - نشر قدرات المدارات (`pkg/spi/consolidation`, `pkg/spi/reconciliation`, `pkg/sdk.Consolidation`, `pkg/sdk.Reconciliation`).
2. **التكافؤ بين المحولات (`Contract Parity`):**
   - نجاح 100% لاختبارات `test/contract_parity/...` (بما فيها `TestPhase12EnterpriseCapabilitiesParity`) عبر `Memory` و`PostgreSQL 16`.
   - نجاح 100% لاختبارات المستهلك الخارجي (`test/external_consumer/...`).
3. **الحوكمة وتعدد الدفاتر والمعايير:**
   - اعتماد `ADR P12-01` لتعدد الدفاتر (`Multi-Ledger / BookID`) وتنفيذ العزل واختباراته (`TestMultiLedgerIsolation`).
   - استكمال قدرات `IFRS 9` و`IFRS 15` و`IFRS 16` و`IAS 12` وتوحيد تمرير `StandardProfileConfig` من النظام المضيف.
   - إنشاء مصفوفة التتبع الموحدة [`UNIFIED_CAPABILITY_TRACEABILITY_MATRIX.md`](../quality/UNIFIED_CAPABILITY_TRACEABILITY_MATRIX.md) وتحديث الوثائق المتضاربة.

---

## 2. أدلة التحقق الحية (Live Verification Evidence)

| البند | الأمر المنفذ | النتيجة المقاسة | المرجع |
| --- | --- | --- | --- |
| التوافق الرجعي (`API Compat`) | `./scripts/check_api_compat.sh` | `PASS` (لا كسر في العقود) | `docs/api/v1.snapshot` |
| النقاء المعماري | `./scripts/verify_architecture.sh` | `PASS` (جميع القواعد R1–R6) | `scripts/verify_architecture.sh` |
| خط الأساس للنواة | `./scripts/verify_kernel_baseline.sh` | `PASS` (`92.9%` تغطية) | `docs/quality/COMPLIANCE_TEST_SUITE_INDEX.md` |
| تكافؤ المحولات والمستهلك الخارجي | `go test ./test/contract_parity/...` و`test/external_consumer` | `PASS` (Memory + PostgreSQL) | `docs/reports/contract_parity_report.md` |
| التعافي من الكوارث (`DR`) | `go test -v ./test/dr/...` | `PASS` (`RPO=0`, `RTO=1.293s <= 5s`) | `test/dr/dr_drill_test.go` |
| اختبار الحمل (`PostgreSQL 16`) | `TestPostingLatencySLO` (16 عامل / 1000 عملية) | `164.02 TPS`, `P50=92.55ms`, `P95=128.49ms` (على بيئة التطوير المحلية؛ يتطلب عتاد إنتاج لهدف `1000 TPS`) | `docs/reports/performance_samples.json` |

---

## 3. قرار الإصدار

- **القرار الساري:** **`GO`** — جميع الفجوات المحاسبية والتكاملية والمعيارية والتشغيلية المحددة في `ENTERPRISE_GAP_REMEDIATION_EXECUTION_PLAN.md` قد أُغلقت وتم التحقق منها باختبارات حية قابلة لإعادة الإنتاج.
