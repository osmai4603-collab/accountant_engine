# 01 — ملف النطاق وبيئة التحقق (Scope Profile)

> مبني وفق [قالب ملف النطاق v2.0.0](../ACCOUNTING_KERNEL_QUALITY_DOCUMENTATION_v2.0.0/docs/quality/SCOPE_PROFILE_TEMPLATE.md).

## 1. بيانات التقييم

| الحقل | القيمة الفعلية |
| --- | --- |
| اسم المنتج / المشروع | `Accountant Engine — Accounting Kernel (internal/kernal)` |
| إصدار المستودع / Commit | `f637cafd5309b363f928e810fce1d69c3d4bfef5` |
| تاريخ التقييم | `2026-11-25` |
| نوع التقييم | `Product Capability Assessment` (لنواة `internal/kernal` فقط) |
| المعيار الحاكم | `ACCOUNTING_KERNEL_QUALITY_STANDARD v2.0.0` |
| الحزم الداخلة في التقييم | `apperrors`, `governance`, `journal`, `ledger`, `masterdata`, `policy`, `posting`, `types`, `valuation` |
| الحزم المستبعدة من التقييم | `internal/orbitals/*`, `modules/*`, `pkg/api/*`, `pkg/sdk/*`, `storage/*` (تُذكر فقط كعقود تكامل عند الحاجة) |

---

## 2. Target Environment (بيئة التحقق الفعلية في هذه الجلسة)

| البعد | القيمة / الحالة المرصودة |
| --- | --- |
| نظام التشغيل والمعمارية | `Linux pop-os 7.2.6-76070206-generic x86_64` |
| Runtime وإصداره | `go version go1.26.3 linux/amd64` |
| قاعدة البيانات وإصدارها | `غير متصلة في هذه الجلسة` (`POSTGRES_URL` غير مضبوط) — تم التحقق عبر محاكي الذاكرة فقط |
| مستوى العزل وإعدادات المعاملات | تحقق E3 في الذاكرة عبر `test/atomic`, `test/concurrency`, `test/acid`؛ عزل Postgres E4 غير متحقق |
| Backup / Restore & SLO | `test/dr` فشل لغياب `/tmp/opencode`؛ `TestPostingLatencySLO` تم تخطيه لغياب `POSTGRES_URL` |

---

## 3. نموذج الكيانات الفعلي في النواة

بخلاف القالب التوضيحي في المعيار (`Tenant -> Company -> Ledger -> CoA/Periods/Entries`)، النموذج الفعلي المنفذ داخل [identifiers.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/types/identifiers.go) و[types.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/types.go) هو:

```text
Company (CompanyID, FunctionalCurrency, PresentationCurrency)
  ├── Chart of Accounts (AccountID, ParentID, EffectiveFrom/To)
  ├── FiscalYears & FiscalPeriods (Periods 1..12 + Adjustment 13, 14)
  ├── Journals (JournalID, AllowedCurrencies, AllowedAccountIDs)
  ├── Policies (PostingPolicy, CurrencyPolicy — versioned by EffectiveDate)
  └── JournalEntries (EntryID, JournalID, FiscalPeriodID)
        └── EntryLines (LineID, AccountID, Dimensions[5], TransactionAmount, FunctionalDebit/Credit)
```

**ملاحظة نطاقية مهمة:**

- لا يوجد كيان `TenantID` منفصل عن `CompanyID`.
- لا يوجد كيان `LedgerID` منفصل؛ يُستخدم `JournalID` لتصنيف اليوميات داخل الشركة الواحدة (`CompanyID`)، ولا توجد دفاتر متعددة مستقلة (General vs Tax vs Management) بفترات وعملات وظيفية مستقلة لكل دفتر.

---

## 4. أساس المحاسبة والإطار

| الموضوع | التحديد الفعلي في النواة |
| --- | --- |
| Accounting Basis | `Accrual` (الترحيل يعتمد على `EffectiveDate` و`FiscalPeriod` وحسابات الاستحقاق/التأجيل في `AdjustingEntryType`) |
| Accounting Framework | النواة تنفذ آليات القيد والترحيل والتوازن وتدعم `IFRSCategory` كبيانات وصفية؛ بينما وحدات `modules/ias12`, `ifrs9`, `ifrs15`, `ifrs16` تقع خارج النواة |
| مسؤولية Kernel مقابل Host | موثقة في [KERNEL_ACCOUNTING_INVARIANTS.md](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md): المصادقة وRBAC واعتمادات Maker-Checker والتخزين الفعلي خارج النواة |

---

## 5. تصنيف القدرات في النواة

| القدرة | التصنيف الفعلي | مفعلة في النواة؟ | الدليل الكودي | الملاحظة |
| --- | --- | :---: | --- | --- |
| General Ledger Posting | `Core Supported` | نعم | [pipeline.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/pipeline.go) | خط ترحيل كامل مع تفويض `PostingExecutor` |
| Chart of Accounts & Fiscal Calendar | `Core Supported` | نعم | [coa.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/coa.go), [fiscal_calendar.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/fiscal_calendar.go) | دورة حياة الحسابات والفترات 1..14 |
| Multi-Currency & FX Revaluation | `Core Supported` | نعم | [conversion.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/valuation/conversion.go), [revaluation.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/valuation/revaluation.go) | يدعم `ACC-008` و`ACC-013` |
| Audit Trail & Tamper Evidence | `Core Supported` | نعم | [audit_chain.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/governance/audit_chain.go) | SHA-256 chain + Checkpoint verification |
| Multi-Company Isolation | `Core Supported` | نعم | [company_scope.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/company_scope.go) | عزل صارم عبر `CompanyID` |
| Multi-Tenancy (`TenantID`) | `Undeclared` | لا | غير موجود في `types/identifiers.go` | فجوة نطاقية إذا طُلب عزل مستأجرين أعلى من الشركة |
| Multi-Ledger (`LedgerID`) | `Undeclared` | لا | غير موجود في `types/identifiers.go` | فجوة نطاقية مقارنة بمتطلب 5.8 |
| Consolidation / Bank Rec / Subledgers / IFRS Modules | `Orbital Module` / `Out of Scope` | خارج النواة | `internal/orbitals/*`, `modules/*` | لا تُخصم نقاط النواة لوجودها خارجها |
