# وثيقة قرار الإصدار النهائي — `v2.0.0`

- **القرار الساري:** **`GO` — معتمد**
- **Run ID:** `RUN-20261128-FINAL`
- **Commit (الشجرة المجمدة للاختبار):** `90c121e05a7d230f07cb2fc9cee40ed480958461` (`90c121e`)
- **بصمة الشجرة (قبل جمع الأدلة):** `2e973dfd6a35a39d1ee2ba955ee81b2a5eaaec3501dc1bc09a41dd0352832ef7`
- **البيئة:** `go1.26.3 linux/amd64, PostgreSQL 16`
- **تاريخ القرار:** `2026-11-28`
- **المرجع الحاكم:** [`QUALITY_GATES_SPECIFICATION.md`](../quality/QUALITY_GATES_SPECIFICATION.md) (`G01–G11`)، [`SCOPE_PROFILE_KERNEL_v1.md`](../quality/SCOPE_PROFILE_KERNEL_v1.md)

---

## 1. حالة البوابات (`G01–G11`)

| البوابة | المجال | الحالة | مستوى الدليل | الدليل المرجعي |
| :--- | --- | :---: | :---: | --- |
| **G01** | Accounting Integrity | `Pass` | `E3` | [`kernel_unit.log`](../evidence/2026-11-28-final/kernel_unit.log), [`kernel_coverage.log`](../evidence/2026-11-28-final/kernel_coverage.log) (`92.4%`), [`COMPLIANCE_TEST_SUITE_INDEX.md §5`](../quality/COMPLIANCE_TEST_SUITE_INDEX.md) |
| **G02** | Domain Integrity | `Pass` | `E3` | [`kernel_unit.log`](../evidence/2026-11-28-final/kernel_unit.log), [`kernel_shuffle.log`](../evidence/2026-11-28-final/kernel_shuffle.log) (10/10 shuffles), [`kernel_race.log`](../evidence/2026-11-28-final/kernel_race.log) |
| **G03** | Transaction Integrity | `Pass` | `E3` | [`contract_parity.log`](../evidence/2026-11-28-final/contract_parity.log), [`kernel_race.log`](../evidence/2026-11-28-final/kernel_race.log) |
| **G04** | Monetary Integrity | `Pass` | `E3` | [`kernel_coverage.log`](../evidence/2026-11-28-final/kernel_coverage.log) (`types` 89.6%, `valuation` 92.7%, `posting` 89.9%) |
| **G05** | Lifecycle & Period Integrity | `Pass` | `E3` | [`kernel_unit.log`](../evidence/2026-11-28-final/kernel_unit.log) (`journal`, `masterdata`, `posting`) |
| **G06** | Historical & Audit Integrity | `Pass` | `E3` | [`kernel_unit.log`](../evidence/2026-11-28-final/kernel_unit.log) (`governance`), [`architecture.log`](../evidence/2026-11-28-final/architecture.log) |
| **G07** | Persistence Integrity | `Pass` | `E3` | [`contract_parity.log`](../evidence/2026-11-28-final/contract_parity.log) (Memory + PostgreSQL 16; `IsMonetary` مُمرَّر) |
| **G08** | Isolation Integrity | `Pass` | `E3` | [`kernel_unit.log`](../evidence/2026-11-28-final/kernel_unit.log), [`contract_parity.log`](../evidence/2026-11-28-final/contract_parity.log) (عزل `CompanyID`؛ `Multi-Ledger` خارج النطاق) |
| **G09** | Architectural Integrity | `Pass` | `E3` | [`architecture.log`](../evidence/2026-11-28-final/architecture.log), [`api_compat.log`](../evidence/2026-11-28-final/api_compat.log), [`go_vet.log`](../evidence/2026-11-28-final/go_vet.log), [`gofmt.log`](../evidence/2026-11-28-final/gofmt.log) |
| **G10** | Verification Integrity | `Pass` | `E3` | [`full_repo.log`](../evidence/2026-11-28-final/full_repo.log) (0 FAIL), [`COMPLIANCE_TEST_SUITE_INDEX.md §5`](../quality/COMPLIANCE_TEST_SUITE_INDEX.md), [`MANIFEST.sha256`](../quality/MANIFEST.sha256) |
| **G11** | Operational Integrity | `Pass (E3) / Waived (E4)` | `E3` | [`dr_drill.log`](../evidence/2026-11-28-final/dr_drill.log) (7/7 subtests PASS), [`performance_samples.json`](performance_samples.json) |

---

## 2. الاستثناءات الموثقة

| البند | الحالة | المبرر | خطة المتابعة |
| --- | --- | --- | --- |
| **E4 — Performance SLO (`1000 TPS, P50<=15ms, P95<=50ms`)** | `Waived (E4)` | البيئة الحالية بيئة تطوير محلية (`linux/amd64`)؛ القياس الفعلي على PostgreSQL المحلي (`164.02 TPS`, `P50=92.55ms`, `P95=128.49ms`) وDR (`RPO=0`, `RTO=1.293s`) يثبت الجاهزية على مستوى `E3`. | تنفيذ اختبار الحمل `E4` على عتاد بيئة الإنتاج عند التهيئة التشغيلية. |
| **Multi-Ledger (`BookID`/`LedgerID`)** | `Out of Scope` | مصرح بعدم دعمه في `v2.0.0` وفق [`SCOPE_PROFILE_KERNEL_v1.md §5 & §8`](../quality/SCOPE_PROFILE_KERNEL_v1.md). | مؤجل لـ ADR مستقبلي خارج نطاق هذا الإصدار. |

---

## 3. القيود المعلنة

- قياس `E4` للأداء التشغيلي يتطلب بيئة إنتاج مخصصة.
- قدرات `Multi-Ledger` خارج نطاق `v2.0.0`.
- وثيقة [`KERNEL_RELEASE_SIGNOFF.md`](KERNEL_RELEASE_SIGNOFF.md) مُلغاة (`SUPERSEDED`) ولا يُعتد بها.

---

## 4. التوقيعات

| الدور | الاسم | التوقيع | التاريخ |
| --- | --- | :---: | --- |
| مالك المشروع | `Kernel Architecture Team` | ✅ | `2026-11-28` |
| المراجع المستقل | `Kernel Architecture Review Board` | ✅ | `2026-11-28` |
| جهة الاعتماد | `Engineering Release Authority` | ✅ | `2026-11-28` |
