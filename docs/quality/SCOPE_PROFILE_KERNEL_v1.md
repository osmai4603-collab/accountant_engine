# ملف النطاق للنواة المحاسبية — Scope Profile (`internal/kernal`)

> **المرجع المعياري:** [ACCOUNTING_KERNEL_QUALITY_STANDARD.md](ACCOUNTING_KERNEL_QUALITY_STANDARD.md) — الإصدار `2.0.0`

## 1. بيانات التقييم

| الحقل | القيمة |
| --- | --- |
| اسم المنتج / المشروع | `Accountant Engine — Accounting Kernel (internal/kernal)` |
| إصدار المستودع | `v2.0.0` (مرشح للمراجعة) |
| Commit | `e3318e4` |
| تاريخ التقييم | `2026-11-27` |
| نوع التقييم | `Product Capability Assessment` |
| المعيار | `2.0.0` |
| Compliance Test Suite | [COMPLIANCE_TEST_SUITE_INDEX.md](COMPLIANCE_TEST_SUITE_INDEX.md) |
| Lead Assessor | `Antigravity AI Assessor` |
| Independent Reviewer | `Pending Independent Review` |
| Approval Authority | `Pending Approval Authority` |

## 2. Target Environment

| البعد | القيمة / الإصدار / الإعداد |
| --- | --- |
| نظام التشغيل والمعمارية | `linux/amd64` (بيئة التطوير والتحقق المحلية) |
| Runtime وإصداره | `go1.26.3` |
| قاعدة البيانات وإصدارها | `In-Memory (E3)؛ PostgreSQL SPI عبر test/contract_parity وtest/atomic` |
| مستوى العزل وإعدادات المعاملات | يُفوض إلى Host Adapter عبر `pkg/spi/storage` (`UnitOfWorkRunner`, `OrderedAccountKeys`) |
| إعدادات التخزين والمخطط والفهارس | خارج النواة (`storage/postgres`, `storage/memory`) |
| إعدادات Tenant وLedger | العزل عبر `CompanyID` (`RULE-KERNEL-016`) و`LedgerID/BookID` عبر `storage.WithLedgerContext` (`ADR P12-01`) |
| حجم البيانات والحمل والتزامن | مقاس على PostgreSQL المحلي (16 متزامن، 1000 عملية): `164.02 TPS` (`P50=92.55ms`, `P95=128.49ms`) |
| العتاد / حدود الموارد | بيئة التحقق المحلية `linux/amd64` (يتطلب إعادة القياس على عتاد الإنتاج المستهدف لبلوغ `1000 TPS`) |
| Backup / Restore | مُتحقق عبر `test/dr/dr_drill_test.go` (`pg_dump` / `pg_restore` + التحقق من سلسة التدقيق والأرصدة) |
| RPO / RTO / SLO عند انطباقها | `RPO = 0`، `RTO = 1.293s` (الهدف التشغيلي `<= 5s` والحد الأقصى `<= 30s` — مُتحقق فعلياً) |

## 3. نموذج الكيانات والعلاقات

**النموذج الفعلي في النواة (`internal/kernal`):**

```text
Company (CompanyID)
  +-- Chart of Accounts (Account, effective-dated)
  +-- FiscalYear --> FiscalPeriod (1..12 + 13 Year-End + 14 Audit)
  +-- Journal (JournalID, allowed currencies & accounts)
  +-- PolicyEngine (PostingPolicy, CurrencyPolicy, versioned windows)
  +-- JournalEntry (Aggregate Root) --> EntryLine (1..N, functional & transaction money)
  +-- AuditChain (Company-scoped SHA-256 hash chain + external checkpoint)
```

## 4. أساس المحاسبة والإطار

| الموضوع | الاختيار / الحدود / المرجع |
| --- | --- |
| Accounting Basis | `Accrual` |
| Accounting Framework | النواة محايدة للأطر (قيد مزدوج، توازن، فترات، عملات)؛ `IAS 21` في `internal/kernal/valuation`؛ بقية معايير `IFRS 9/15/16` في `modules/*` |
| Framework Version | `IFRS / IAS 21` (للعملات وإعادة التقييم) |
| Effective Date | محكوم بنوافذ نفاذ `PolicyEngine` و`EffectiveAccountReader` |
| نطاق الاعتراف والقياس | النواة مسؤولة عن صحة القيد والترحيل والعملة الوظيفية؛ سياسات الاعتراف المتخصصة في `modules/*` |
| مسؤولية Kernel / Policy Module / Host | المعيار يعرّف `ACC-001`–`ACC-013` (13 ثابتاً حرجاً) بينما `docs/rules` تعرّف `RULE-ACC-001..043` ( namespaces منفصلة )؛ النواة: الثوابت المحاسبية؛ المضيف: ACID وLocks وRLS وOutbox وAuth/RBAC وMaker-Checker |

## 5. تصنيف القدرات

| القدرة | التصنيف | معلنة كمدعومة؟ | مفعلة في النشر؟ | مالك القدرة | عقد التكامل / الدليل | سبب الاستبعاد إن وجد |
| --- | --- | ---: | ---: | --- | --- | --- |
| General Ledger Posting | `Core Supported` | نعم | نعم | `internal/kernal/posting` | `PostingPipeline.PostEntry` | — |
| Chart of Accounts | `Core Supported` | نعم | نعم | `internal/kernal/masterdata` | `ResolvePostableAccountAsOf` | — |
| Multi-Currency & FX Revaluation | `Core Supported` | نعم | نعم | `internal/kernal/types`, `valuation` | `Money`, `ConvertForDate`, `RevaluationEngine` | — |
| Tax Posting | `Core Supported` | نعم | نعم | `internal/kernal/posting` | أسطر قيد متوازنة على حسابات الضرائب | — |
| Multi-Ledger (`BookID`/`LedgerID`) | `Out of Scope` | لا | لا | مؤجل لـ ADR مستقبلي | [ledger/doc.go](../../internal/kernal/ledger/doc.go#L23-L25) | مصرح بعدم دعمه في هذه النسخة |
| Consolidation | `Orbital Module` | لا | — | خارج النواة | `RebuildAccountBalances` | مسؤولية طبقة التجميع |
| Intercompany Processing | `Orbital Module` | لا | — | خارج النواة | `PostingPipeline` | مسؤولية طبقة المضيف |
| Budgeting & Forecasting | `Out of Scope` | لا | لا | خارج النواة | — | خارج مسؤولية النواة |
| Bank Reconciliation | `Orbital Module` | لا | — | خارج النواة | `EntryLine.MatchingNumber` | مسؤولية المدارات |
| Fixed Assets / AR / AP | `Orbital Module` | لا | — | `modules/*` / المضيف | `PostingPipeline` | مسؤولية المدارات |
| Tax Calculation & Compliance | `Orbital Module` | لا | — | المضيف | — | تحسب خارج النواة |

## 6. العملات والفترات

- العملات المدعومة: سجل `ISO-4217` الكامل في `internal/kernal/types/currency.go` (مع رفض `XXX`).
- العملة الوظيفية: محددة لكل شركة (`Company.FunctionalCurrency` و`CurrencyPolicy.FunctionalCurrency`).
- عملة المعاملة: محددة على مستوى القيد والسطر مع التحقق من `CurrencyPolicy.AllowsCurrency`.
- FX rate source / date / type: `SPOT`, `CLOSING`, `AVERAGE`, `HISTORICAL` عبر `RateProvider` و`RateRegistry`.
- Currency scale / internal precision / rounding / allocation: `InternalScale = 12`، سقف 28 رقماً، تقريب Half to Even افتراضياً (`bankers`) مع السماح بـ `half_up` عبر `CurrencyPolicy`، وتسوية البواقي على `RoundingDifferenceAccountID`.
- Fiscal calendar: 12 فترة عادية + فترة 13 (إقفال/تسويات نهاية السنة) + فترة 14 (تسويات التدقيق).
- Close / reopen policy: `Open -> SoftClosed -> HardClosed`؛ إعادة الفتح تتطلب `ActorID` و`Reason`.

## 7. حدود الحجم والأداء

- حجم القيود المتوقع: مقاس عند عينة `1000` قيد متزامن (`16` عامل) في `test/benchmark/latency_slo_test.go`.
- متوسط / أقصى أسطر القيد: `2` أسطر في عينة الـ SLO القياسية.
- Throughput: `164.02 TPS` على PostgreSQL محلي أحادي العقدة (`docs/reports/performance_samples.json`).
- p50 / p95 / p99: `P50 = 92.55 ms`، `P95 = 128.49 ms`، `P99 = 172.55 ms` على بيئة PostgreSQL المحلية.
- Memory / CPU limits: مقاس على بيئة `linux/amd64` المحلية؛ هدف الإنتاج (`1000 TPS`, `P50 <= 15ms`, `P95 <= 50ms`) يتطلب عتاد إنتاج مخصص.
- Dataset and workload profile: `In-Memory` و`PostgreSQL 16` عبر `test/contract_parity` و`test/benchmark` و`test/dr`.

## 8. القدرات المستبعدة والافتراضات

| العنصر | التصنيف | المبرر | أثره على التقييم | موافق النطاق |
| --- | --- | --- | --- | --- |
| Multi-Ledger (`BookID`/`LedgerID`) | `Out of Scope` | مصرّح بعدم دعمه في هذه النسخة؛ مؤجل لـ ADR مستقبلي | لا يُحتسب كفجوة عزل في هذا التقييم | `Pending Approval Authority` |
| Consolidation / Intercompany / Budgeting | `Out of Scope` / `Orbital Module` | مسؤولية طبقة التجميع أو المضيف | خارج نطاق النواة | `Pending Approval Authority` |
| Bank Reconciliation / Fixed Assets / AR / AP / Tax Calculation | `Orbital Module` | مسؤولية المدارات أو المضيف | خارج نطاق النواة | `Pending Approval Authority` |

**قاعدة:** Optional Capability المعلنة كمدعومة تدخل التقييم ولو لم تكن مفعلة في نشر بعينه؛ أما Undeclared فتُسجَّل فجوة نطاق. لا تُعتبر قدرة مستبعدة N/A دون موافقة Scope Profile معتمدة.

## 9. اعتماد ملف النطاق

| الدور | الاسم | القرار | التاريخ |
| --- | --- | --- | --- |
| Project / Kernel Owner | `Pending Nomination` | `Pending` | `—` |
| Lead Assessor | `Antigravity AI Assessor` | `Pending` | `—` |
| Independent Reviewer | `Pending Independent Review` | `Pending` | `—` |
| Approval Authority | `Pending Approval Authority` | `Pending` | `—` |

**الحالة:** `Pending` — ملف النطاق هذا لم يُعتمد بعد؛ لا يُفسَّر كشهادة اجتياز أو موافقة نطاق نهائية.

## 10. سجل التغييرات

| الإصدار | التاريخ | التغيير | الموافق |
| --- | --- | --- | --- |
| 0.1.0 | 2026-11-27 | إنشاء ملف النطاق للمراجعة الحالية | `Pending Approval Authority` |
| 0.1.1 | 2026-11-27 | توحيد إصدار المستودع مع `2.0.0` وتوضيح namespaces الثوابت؛ إضافة حدود الحجم والأداء والقدرات المستبعدة وحالة الاعتماد | `Pending Approval Authority` |
