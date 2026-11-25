# 03 — مصفوفة الثوابت المحاسبية وبوابات الجودة (`ACC-001`–`ACC-013` & `G01`–`G11`)

## 1. تقييم الثوابت المحاسبية ومتطلبات المرجع

| Requirement ID | المتطلب | حرج؟ | الدليل الكودي الفعلي | مستوى الدليل | الحالة |
| --- | --- | :---: | --- | :---: | --- |
| **ACC-001** | توازن القيد | نعم | `ValidateEntryDoubleEntry` ([entry_validation.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/entry_validation.go)) | E3 | **Pass** |
| **ACC-002** | سلامة سطر القيد | نعم | `ValidateEntryLine` (منع مدين+دائن معاً ومنع الصفر) | E3 | **Pass** |
| **ACC-003** | إشارة المبلغ | نعم | `ValidateEntryLine` (`ErrNegativeAmountProhibited`) | E3 | **Pass** |
| **ACC-004** | قابلية الترحيل | نعم | `ResolvePostableAccountAsOf` ([coa.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/coa.go)) | E3 | **Pass** |
| **ACC-005** | صلاحية الفترة | نعم | `ValidatePeriodForPostingWithSoftClose` ([fiscal_calendar.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/fiscal_calendar.go)) | E3 | **Pass** |
| **ACC-006** | عدم تعديل التاريخ | نعم | `EntryStatus.IsMutable`, `ValidateImmutability`, `BuildReversalEntryAt` | E3 | **Pass** |
| **ACC-007** | Idempotency | نعم | `BuildIdempotencyScope`, `FingerprintRequest`, `FingerprintCallerPayload` ([transaction.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/transaction.go)) | E3 (ذاكرة) / E4 (غير متحقق) | **Pass (E3)** |
| **ACC-008** | اتساق العملة | نعم | `Money.ensureSameCurrency`, `ValidateEntryLine`, `entryJournal.AllowsCurrency` | E3 | **Pass** |
| **ACC-009** | عزل الدفتر (`Ledger`) | نعم | يوجد `JournalID` فقط ولا يوجد `LedgerID` مستقل | E0 | **Fail (نقص نموذج الدفتر)** |
| **ACC-010** | عزل المستأجر (`Tenant`) | نعم | عزل `CompanyID` محقق؛ لا يوجد `TenantID` مستقل | E3 (`Company`) / E0 (`Tenant`) | **Partial / Fail لـ TenantID** |
| **ACC-011** | تتبع المصدر | نعم | حقلا `SourceModule` و`ReferenceID` غير مفروضين في `PostEntry` | E2 (حقول فقط) | **Fail (جزئي)** |
| **ACC-012** | اتساق الرصيد | نعم | `ProjectPostedMovements` و`RebuildAccountBalances` ([projection.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/ledger/projection.go)) | E3 | **Pass** |
| **ACC-013** | التوازن الوظيفي | نعم | `ValidateEntryDoubleEntry` + `RoundEntryWithRoundingPolicy` + `valuation` | E3 | **Pass** |
| **DOM-001** | حماية Aggregate Invariants | نعم | `JournalEntry` مكشوف الحقول دون تغليف Aggregate Root | E2 | **Fail** |
| **TXN-001/002** | Atomicity / Concurrency | نعم | `PostingPipeline` + `lock_manager.go` + `test/atomic`, `test/concurrency` | E3 (ذاكرة) | **Not Verified (E4 DB)** |
| **MON-001** | Representation & Rounding | نعم | `Money`, `InternalScale=12`, `MinorUnitsExact`, AST float ban | E3 | **Pass** |
| **LIFE-001** | Lifecycle & Period Rules | نعم | `EntryStatus` + `FiscalPeriod` (ينقصه `Void` و`Partial Reversal`) | E3 | **Pass (مع ملاحظات)** |
| **AUD-001** | Audit & Tamper Evidence | نعم | `AuditChain` SHA-256 + `VerifyAuditIntegrityWithCheckpoint` | E3 | **Pass** |
| **ARC-001** | Dependency Direction | نعم | `verify_architecture.sh` (11/11) + `doc_test.go` | E3 | **Pass** |
| **PERF-001** | Performance SLO | حسب النطاق | `TestPostingLatencySLO` متخطى لغياب `POSTGRES_URL` | E0/Skipped | **Not Verified** |
| **OPS-001** | Recovery / Migration | حسب النطاق | `TestDRDrill_FullRecovery` فشل (`/tmp/opencode` غير موجود) | Failed | **Fail / Not Verified** |

---

## 2. قرارات بوابات الجودة (`G01–G11`)

| Gate | الحد الأدنى المطلوب | القرار | التبرير المبني على الأدلة |
| --- | --- | :---: | --- |
| **G01** Accounting Integrity | E3 | **Pass** | جميع اختبارات `posting`, `types`, `valuation`, `masterdata` الخاصة بالتوازن وصحة القيد نجحت |
| **G02** Domain Integrity | E3 | **Fail** | يمكن إنشاء وتعديل `journal.JournalEntry` و`EntryLine` مباشرة في Go خارج `PostingPipeline` بسبب تصدير جميع الحقول |
| **G03** Transaction Integrity | E3 / E4 | **Not Verified (E4)** | نجحت اختبارات الذاكرة (`test/atomic`, `test/acid`, `test/concurrency`) لكن بيئة Postgres (`E4`) لم تُختبر |
| **G04** Monetary Integrity | E3 | **Pass** | اجتياز اختبارات الدقة، الفازينج، حظر `float`، تسوية التقريب، والتوازن الوظيفي `ACC-013` |
| **G05** Lifecycle & Period Integrity | E3 | **Pass** | آلة حالات القيد والفترات (بما فيها 13 و14 وإعادة الفتح المدققة) مثبتة بالاختبارات |
| **G06** Historical & Audit Integrity | E3 | **Fail (جزئي)** | سلسلة التدقيق ونقاط التفتيش ممتازة، لكن `ACC-011` لا يفرض سياسة مصدر/سبب بديل للقيود اليدوية في `PostEntry` |
| **G07** Persistence Integrity | E3 / E4 | **Not Verified (E4)** | `test/contract_parity` نجحت على محول الذاكرة؛ لم يُختبر محول PostgreSQL لغياب `POSTGRES_URL` |
| **G08** Isolation Integrity | E3 | **Fail (نطاقي)** | عزل `CompanyID` مثبت بالكامل، لكن `TenantID` و`LedgerID` غير موجودين في نموذج النواة |
| **G09** Architectural Integrity | E2/E3 | **Pass** | صفر مخالفات اعتمادية؛ `verify_architecture.sh` و`go list` تثبت نقاء `internal/kernal` |
| **G10** Verification Integrity | E3 | **Pass (E3)** | 10/10 حزم نواة نجحت مع `-race`؛ تغطية ~92.4%؛ قتل 8/8 طفرات موثق |
| **G11** Operational Integrity | E4 | **Not Verified / Fail** | فشل `TestDRDrill_FullRecovery` وتخطي `TestPostingLatencySLO` |
