---
title: "هيكل المشروع الجديد للنواة المحاسبية — Target Project Directory Structure"
subtitle: "المواصفة المعمارية الكاملة لتنظيم الحزم وشجرة المجلدات لمشروع Accounting Engine المعاد هيكلته"
document_code: "AE-STRUC-NEW-001"
version: "1.0.0"
date: "2026-11-20"
status: "وثيقة معمارية معتمدة"
project: "Accounting Engine"
classification: "Software Architecture & Packaging Specification"
language: "ar"
technical_terms: "en"
---

# هيكل المشروع الجديد للنواة المحاسبية (Target Project Structure)

> **بيان الإلزام والهدف:**  
> تُحدد هذه الوثيقة **الهيكل المعماري والبرمجي الجديد (Target Packaging & Directory Structure)** لمشروع `accounting_engine`.  
> يعكس هذا الهيكل الرؤية المعتمدة لتحويل المشروع إلى **نواة محاسبية نقية، عالية الأداء، وقابلة للتضمين (Embeddable Kernal Engine)** عبر Go SDK، مع الفصل الصارم بين:
>
> 1. السطح البرمجي العام المفتوح للمستهلك (`pkg/`).
> 2. النواة المحاسبية الحتمية الصرفة (`internal/kernal/`).
> 3. المكونات المدارية الاختيارية (`internal/orbitals/`).
> 4. محولات التخزين الملموسة (`storage/`).
> 5. وحدات المعايير التخصصية (`modules/`).
> مع الاستبعاد التام والنهائي لكافة خوادم وبروتوكولات النقل الشبكي والمصادقة وإدارة الهويات.

---

## فهرس المحتويات

1. [شجرة المجلدات الهندسية الكاملة (Complete Directory Tree)](#1-شجرة-المجلدات-الهندسية-الكاملة-complete-directory-tree)
2. [تحليل الطبقات المعمارية والحزم (Layer-by-Layer Breakdown)](#2-تحليل-الطبقات-المعمارية-والحزم-layer-by-layer-breakdown)
3. [المكونات المستبعدة وسجل التبرير المعماري (Pruned Components)](#3-المكونات-المستبعدة-وسجل-التبرير-المعماري-pruned-components)
4. [تنظيف وترشيد ملف الاعتماديات (`go.mod`)](#4-تنظيف-وترشيد-ملف-الاعتماديات-gomod)
5. [قواعد اتجاه الاستيراد بين الحزم (Import Rules & Boundaries)](#5-قواعد-اتجاه-الاستيراد-بين-الحزم-import-rules--boundaries)
6. [دليل تضمين واستخدام الهيكل الجديد للمطورين (Integration Walkthrough)](#6-دليل-تضمين-واستخدام-الهيكل-الجديد-للمطورين-integration-walkthrough)

---

## 1. شجرة المجلدات الهندسية الكاملة (Complete Directory Tree)

```text
accounting_engine/
│
├── go.mod                                # خالٍ تماماً من gRPC و Protobuf و AWS SDK و HTTP Server
├── go.sum
├── README.md                             # دليل الاستخدام والتضمين المباشر للنواة
├── LICENSE
│
├── pkg/                                  # 🟢 1. السطح البرمجي العام للمستهلك الخارجي (Public Surface)
│   ├── sdk/                              # نقطة الدخول البرمجية الموحدة (Entry Point)
│   │   ├── engine.go                     # واجهة Engine الرئيسية والخدمات العامة
│   │   ├── builder.go                    # منشئ المحرك الموحد sdk.NewBuilder
│   │   ├── options.go                    # خيارات التكوين (WithStoragePorts, WithReporting, WithStandard)
│   │   ├── event_ingestion.go            # خدمة استيعاب الأحداث الاقتصادية العامة
│   │   ├── cash_flow.go                  # محولات ونماذج التدفق النقدي
│   │   └── errors.go                     # أخطاء المجال العامة المصدرة (Domain Sentinel Errors)
│   │
│   ├── api/                              # نماذج البيانات والعقود المعيارية العامة
│   │   └── v1/
│   │       ├── types/                    # المعرفات والأنواع العامة (CompanyID, AccountID, Money)
│   │       ├── ledger/                   # نماذج القيد العام، أسطر القيد، الحسابات، والفترات
│   │       ├── events/                   # كائن الحدث الاقتصادي المعياري (AccountingEvent) وقواعده
│   │       ├── reporting/                # نماذج التقارير المالية (TrialBalance, BalanceSheet, P&L)
│   │       └── subledger/                # نماذج الدفاتر المساعدة (AR, AP, Cash, Assets)
│   │
│   └── spi/                              # 🔌 منافذ مزودي الخدمة (Service Provider Interfaces - Ports)
│       ├── storage/                      # عقود التخزين المجردة (UnitOfWork, EntryStore, AccountStore...)
│       ├── standards/                    # عقود الموديلات المعيارية الدولية (StandardModule, Evaluator)
│       ├── masterdata/                   # عقود استرجاع بيانات الحسابات والشركات
│       └── application/                  # عقود سياق الأمان الموثوق (Principal, Context)
│
├── internal/                             # 🔒 2. المنطق الداخلي المحمي للنواة ومكوناتها (Internal Domain)
│   │
│   ├── kernal/                           # 🟢 النواة المحاسبية الصرفة (Kernal Accounting Kernel)
│   │   ├── posting/                      # محرك الترحيل الذري، القيد المزدوج، العكس، والتقريب
│   │   │   ├── pipeline.go               # خط أنابيب الترحيل المالي الذري الثماني
│   │   │   ├── entry_validation.go       # التحقق الرياضي والهيكلي الصارم للقيد المزدوج والعملة
│   │   │   ├── rounding.go               # تسوية فروق التقريب المصرفي وحساب الفروق الصريح
│   │   │   ├── reversal.go               # محرك القيود العكسية والربط الدلالي
│   │   │   ├── lock_manager.go           # إدارة أقفال الحسابات المرتبة تصاعدياً لمنع Deadlocks
│   │   │   ├── content_hash.go           # التجزئة الثابتة لمحتوى القيد وبصمة الاعتماد
│   │   │   ├── dimensions.go             # محرك التحقق من أبعاد التكلفة والمشاريع
│   │   │   └── error_classification.go   # تصنيف الأخطاء المحاسبية وفق التصنيف المعياري
│   │   │
│   │   ├── masterdata/                   # إدارة شجرة الحسابات والتقويم المالي
│   │   │   ├── coa.go                    # شجرة الحسابات، التدرج، وفحص الحسابات التجميعية
│   │   │   ├── account.go                # الكيان الكامل للحساب والتصنيفات ودوال الأهلية
│   │   │   ├── account_version.go        # التتبع التاريخي لتغيرات الحسابات (Effective-Dating)
│   │   │   ├── fiscal_calendar.go        # تقويم الفترات والسنوات وفصل فترات التسوية 13 و 14
│   │   │   ├── journal.go                # تعريف اليوميات المحاسبية وضوابط العملات والحسابات
│   │   │   ├── cost_center.go            # هيكلية مراكز التكلفة والتحقق منها
│   │   │   ├── dimension.go              # الأبعاد المالية التحليلية وقيمها
│   │   │   └── company_scope.go          # عزل وضبط نطاق الشركة المستأجرة
│   │   │
│   │   ├── types/                        # كائنات القيمة والحساب العشري الدقيق
│   │   │   ├── money.go                  # تمثيل المبالغ عبر الحساب العشري أو Int64 الصغرى
│   │   │   ├── currency.go               # معيار ISO 4217 وضوابط العملات
│   │   │   ├── rounding.go               # خوارزمية التقريب المصرفي (Banker's Rounding)
│   │   │   └── identifiers.go            # المعرفات الصلبة (AccountID, CompanyID, ...)
│   │   │
│   │   ├── valuation/                    # أسعار الصرف وإعادة التقييم
│   │   │   ├── exchange_rate.go          # سجل أسعار الصرف التاريخية والتحويل المالي
│   │   │   ├── conversion.go             # محرك التحويل المالي الصريح بين العملات
│   │   │   ├── rates.go                  # إدارة مستويات أسعار الصرف ومزوداتها
│   │   │   └── revaluation.go            # تسوية وإعادة تقييم فروق العملات الأجنبية (IAS 21)
│   │   │
│   │   ├── policy/                       # السياسات وضوابط الاعتماد المالي
│   │   │   ├── policy.go                 # نموذج السياسات الشامل (Posting & Currency Policies)
│   │   │   ├── posting_rules.go          # قواعد الترحيل والاعتماد والرقابة المزدوجة
│   │   │   └── eligibility.go            # سياسات أهلية الحسابات للعمليات
│   │   │
│   │   ├── governance/                   # الحوكمة وسجل التدقيق الجنائي
│   │   │   ├── audit_chain.go            # سجل التدقيق الدائم (Append-Only) المشفر بالفواصل الصريحة
│   │   │   ├── verifier.go               # محرك الفحص الجنائي واكتشاف التلاعب والحذف
│   │   │   ├── approvals.go              # بصمات الاعتماد والتحقق الرقابي
│   │   │   └── sod_policy.go             # فصل المهام والصلاحيات (Segregation of Duties)
│   │   │
│   │   └── apperrors/                    # الأخطاء المحاسبية المعيارية المكودة
│   │       └── errors.go                 # هيكل CodedError والتصنيفات المحاسبية الموحدة
│   │
│   └── orbitals/                         # 🔵 المكونات المدارية حول النواة (Orbital Components)
│       ├── subledgers/                   # دفاتر الأستاذ المساعدة وتكامل حسابات الرقابة
│       │   ├── ar.go                     # حسابات المدينين والعملاء وأعمار الديون
│       │   ├── ap.go                     # حسابات الدائنين والموردين والاستحقاقات
│       │   ├── cash_treasury.go          # الخزينة والبنوك وحركات النقدية
│       │   └── fixed_assets.go           # الأصول الثابتة وجداول الإهلاك الدوري
│       │
│       ├── reporting/                    # محرك التقارير المالية وميزان المراجعة
│       │   ├── trial_balance.go          # ميزان المراجعة اللحظي بالمجاميع والأرصدة
│       │   ├── balance_sheet.go          # قائمة المركز المالي وإثبات التوازن
│       │   ├── income_statement.go       # قائمة الأرباح والخسائر المقارنة
│       │   └── cash_flow_statement.go    # قائمة التدفقات النقدية وفق IAS 7
│       │
│       ├── events/                       # محرك استيعاب ومطابقة الأحداث
│       │   ├── event_processor.go        # تحويل الحدث الاقتصادي لقيد محاسبي
│       │   └── idempotency_store.go      # إدارة مفاتيح عدم التكرار وكشف التعارضات
│       │
│       ├── reconciliation/               # المطابقات والتسويات المصرفية
│       │   ├── matcher.go                # محرك مقارنة السجلات الداخلية والخارجية
│       │   └── bank_parsers/             # محللات كشوف البنك (CAMT.053 و MT940)
│       │
│       ├── consolidation/                # توحيد القوائم للشركات التابعة
│       │   └── intercompany.go           # استبعاد المعاملات البينية وترجمة العملات
│       │
│       └── sdkapplication/               # محولات حالات الاستخدام الداخلية للمكتبة
│           └── year_end_closing.go       # سير عمل إقفال نهاية السنة الذري
│
├── storage/                              # ⚙️ 3. محولات التخزين الملموسة (Concrete Storage Adapters)
│   ├── postgres/                         # محول PostgreSQL الإنتاجي الكامل
│   │   ├── adapter.go                    # تنفيذ واجهات pkg/spi/storage
│   │   ├── uow.go                        # إدارة المعاملات الذرية (Unit of Work)
│   │   ├── migrations/                   # ملفات الترحيل SQL (32 Up / 32 Down)
│   │   ├── rls_policies.sql              # سياسات عزل المستأجرين على مستوى الصفوف (RLS)
│   │   └── triggers_worm.sql             # Triggers منع التعديل والحذف (Append-Only WORM)
│   │
│   └── memory/                           # محول الذاكرة المؤقتة للاختبارات (In-Memory)
│       ├── adapter.go                    # تنفيذ واجهات التخزين في الذاكرة
│       └── uow.go                        # محاكاة المعاملات الذرية والأقفال
│
├── modules/                              # 🟣 4. وحدات المعايير الدولية المتخصصة (Standards Modules)
│   ├── ifrs15/                           # معيار الإيراد من العقود مع العملاء (5-step model)
│   │   ├── module.go                     # تعريف الوحدة وتسجيلها عبر SPI
│   │   └── revenue_schedule.go           # جداول الاعتراف بالإيراد والتزامات الأداء
│   ├── ifrs16/                           # معيار عقود الإيجار وحق الاستخدام (ROU)
│   │   ├── module.go
│   │   └── lease_amortization.go         # جداول إهلاك حق الاستخدام والفائدة التمويلية
│   ├── ifrs9/                            # معيار الأدوات المالية ومخصص الخسائر (ECL)
│   │   ├── module.go
│   │   └── ecl_calculator.go             # احتساب مخصص التعثر الائتماني
│   └── ias12/                            # معيار ضرائب الدخل المؤجلة
│       ├── module.go
│       └── deferred_tax.go               # احتساب الفروق المؤقتة والضرائب المؤجلة
│
├── test/                                 # 🧪 5. حزم الاختبارات وضمان الجودة (Quality Assurance)
│   ├── unit/                             # اختبارات الوحدة السريعة بالذاكرة للنواة
│   ├── contract_parity/                  # اختبارات التكافؤ التام بين الذاكرة و PostgreSQL (22 اختبار)
│   ├── atomic/                           # اختبارات الذرية وحقن الأعطال (All-or-Nothing)
│   ├── acid/                             # اختبارات التزامن والأقفال وسياسات RLS
│   ├── external_consumer/                # اختبار المستهلك الخارجي (استيراد pkg/sdk فقط دون internal)
│   └── benchmark/                        # قياس زمن استجابة الترحيل (P95 < 50ms) والضغط المتزامن
│
└── docs/                                 # 📖 6. الوثائق والمعايير المعمارية الرسمية المعتمدة
    ├── PID_RESTRUCTURING_EMBEDDABLE_KERNAL.md
    ├── TARGET_ACCOUNTING_KERNAL.md
    ├── ARCHITECTURAL_PRINCIPLES_CHARTER.md
    ├── ACCOUNTING_CAPABILITIES_INVENTORY.md
    ├── ACCOUNTING_BUSINESS_RULES.md
    ├── ENGINEERING_BUSINESS_RULES.md
    ├── ACCOUNTING_ENGINE_SUCCESS_CRITERIA.md
    ├── ACCOUNTING_ENGINE_ACCEPTANCE_CRITERIA.md
    └── NEW_PROJECT_STRUCTURE.md
```

---

## 2. تحليل الطبقات المعمارية والحزم (Layer-by-Layer Breakdown)

### 1. طبقة السطح العام (`pkg/`)

* **`pkg/sdk/`:** هذه الحزمة هي **الواجهة الوحيدة** التي يراها مطور النظام المضيف عند استدعاء المحرك داخل تطبيقه. توفر المنشئ `sdk.NewBuilder`، وكائن المحرك الرئيسي `sdk.Engine`، والخدمات المجردة (`LedgerService`, `ReportingService`, `EventIngestionService`).
* **`pkg/api/v1/`:** نماذج بيانات ثابتة غير قابلة للتغيير (`Immutable DTOs`)، لا تحتوي على أي منطق تخزين أو استعلام.
* **`pkg/spi/`:** عقود الموانئ (Ports) التي تضمن استقلالية المحرك؛ حيث تتحدث النواة مع هذه العقود ولا تعلم من ينفذها.

### 2. طبقة النواة المحاسبية الصرفة (`internal/kernal/`)

* تمثل **القلب الحسابي المصمت** للمحرك. لا تعتمد على أي حزم مدارية، ولا تعتمد على أي بروتوكول خارجي.
* مسؤولة حصرياً عن: التحقق من التوازن، الترحيل الذري، حظر الحسابات التجميعية، الفترات المالية، الحساب العشري الدقيق، والقيود العكسية.

### 3. طبقة المكونات المدارية (`internal/orbitals/`)

* مكونات وظيفية متقدمة تدور في فلك النواة وتتغذى من مخرجاتها.
* يتم تفعيلها بصورة اختيارية (`Opt-in`) عبر خيارات المنشئ `sdk.WithReporting()` أو `sdk.WithSubledgers()`.

### 4. طبقة موائمات التخزين (`storage/`)

* تقع خارج النواة تماماً وتطبق عقود `pkg/spi/storage`.
* **الفصل التشغيلي الصارم بين التخزين الإنتاجي وتخزين الاختبارات:**
  * **`storage/postgres` (المحول الإنتاجي الوحيد المعتمد):** يوفر الحفظ الدائم على القرص، المعاملات المتوافقة مع ACID، عزل المستأجرين (RLS)، وقواعد الحرمة الجنائية (WORM Triggers).
  * **`storage/memory` (محول مخصص للتطوير والاختبارات فقط):** محول خفيف وسريع في الذاكرة لتشغيل اختبارات الوحدة وبوابات CI في أجزاء من الميلي ثانية. **يُحظر منعاً باتاً استخدامه في بيئات الإنتاج**؛ لكونه تخزيناً مؤقتاً متطايراً (Volatile) يفقد كافة البيانات المحاسبية وأثر التدقيق عند إعادة التشغيل ولا يدعم سياسات RLS.

---

## 3. المكونات المستبعدة وسجل التبرير المعماري (Pruned Components)

| المكون المحذوف / المستبعد | المسار القديم | التبرير المعماري للاستبعاد | أين يتواجد البديل؟ |
| :--- | :--- | :--- | :--- |
| **خادم gRPC و HTTP REST** | `cmd/accounting-service/` | المحرك مكتبة مضمنة؛ تضمين خوادم شبكية يفرض منافذ شبكية وتبعية معقدة غير مرغوبة تعيق التضمين السلس. | إذا رغبت بنشر المحرك كخدمة مصغرة، يتم إنشاء مستودع مستقل كـ Gateway يستورد `pkg/sdk`. |
| **تعريفات Protobuf و gRPC** | `proto/` | لا حاجة لأي عقود شبكية أو تسلسل ثنائي للشبكة داخل النواة المحاسبية. | ينقل إلى مستودع خدمة النقل الشبكي الخارجية. |
| **أداة الأوامر القديمة** | `cmd/agy/` | كانت تعتمد على `legacyengine` المتجاوز للعقود وتكسر حدود الحزم. | استبدالها بأمثلة استخدام نقية داخل `examples/` أو عبر SDK مباشرة. |
| **المصادقة وتوكنات JWT** | `internal/serviceauth/` | إدارة المستخدمين وكلمات المرور والتوكنات مسؤولية مزود الهوية المركزي (IAM) للنظام المضيف. | يمرر النظام المضيف سياق الهوية الموثوق (`Principal`) عبر `context.Context`. |
| **تحديد معدل الطلبات** | `internal/ratelimit/` | وظيفة تخص حافة الشبكة وبوابات المرور (API Gateways)، وليس النواة المالية الحسابية. | بوابات النقل وحافة الشبكة في النظام المضيف. |
| **الواجهات المزدوجة القديمة** | `facade.go`, `internal/legacyengine/` | وجود مسارين متوازيين للبناء (`WithCoreStorage` مقابل `WithStoragePorts`) كان يسبب تشويشاً معمارياً. | مسار بناء واحد موحد حصراً: `sdk.NewBuilder(sdk.WithStoragePorts(...))`. |

---

## 4. تنظيف وترشيد ملف الاعتماديات (`go.mod`)

في الهيكل الجديد، تم التخلص من حزم النقل الشبكي وحزم التخزين السحابي البعيدة، مما جعل ملف `go.mod` خفيفاً ونقياً:

```text
module github.com/osmai4603-collab/accounting_engine

go 1.26.0

require (
    github.com/google/uuid v1.6.0              // لمعالجة المعرفات الفريدة
    github.com/shopspring/decimal v1.6.0       // للحساب العشري الدقيق ومنع أخطاء التقريب
    github.com/jackc/pgx/v5 v5.12.0            // لمحول تخزين PostgreSQL (محصور في storage/postgres)
    golang.org/x/text v0.43.0                  // لدعم النصوص والرموز الدولية
)
```

> **النتيجة الهندسية:**  
> تم حذف: `google.golang.org/grpc`, `google.golang.org/protobuf`, و `github.com/aws/aws-sdk-go-v2` وكافة ملحقاتها، مما خفض حجم الشيفرة وسرّع زمن البناء وعزل النواة عن أي ثغرات شبكية خارجية بنسبة 100%.

---

## 5. قواعد اتجاه الاستيراد بين الحزم (Import Rules & Boundaries)

```mermaid
flowchart TD
    subgraph PublicSurface ["السطح العام (pkg/)"]
        SDK["pkg/sdk"]
        API["pkg/api/v1"]
        SPI["pkg/spi"]
    end

    subgraph InternalCore ["النواة المحاسبية (internal/kernal/)"]
        CORE_PKG["posting, masterdata, types, valuation, policy, governance"]
    end

    subgraph InternalOrbitals ["المكونات المدارية (internal/orbitals/)"]
        ORBIT_PKG["subledgers, reporting, events, reconciliation, consolidation"]
    end

    subgraph StorageAdapters ["محولات التخزين (storage/)"]
        STORAGE_PKG["storage/postgres, storage/memory"]
    end

    %% اتجاه الاستيراد المسموح به
    SDK --> API
    SDK --> SPI
    SDK --> CORE_PKG
    SDK --> ORBIT_PKG
    
    ORBIT_PKG --> API
    ORBIT_PKG --> SPI
    ORBIT_PKG --> CORE_PKG
    
    CORE_PKG --> API
    CORE_PKG --> SPI
    
    STORAGE_PKG --> SPI
    STORAGE_PKG --> API

    %% قواعد الحظر الصارمة
    CORE_PKG -.->|محظور قطعياً| ORBIT_PKG
    CORE_PKG -.->|محظور قطعياً| STORAGE_PKG
    API -.->|محظور قطعياً| InternalCore
    SPI -.->|محظور قطعياً| InternalCore

    style PublicSurface fill:#023e8a,stroke:#0077b6,stroke-width:2px,color:#fff;
    style InternalCore fill:#2d6a4f,stroke:#1b4332,stroke-width:3px,color:#fff;
    style InternalOrbitals fill:#1d3557,stroke:#457b9d,stroke-width:2px,color:#fff;
    style StorageAdapters fill:#6a0572,stroke:#8b008b,stroke-width:2px,color:#fff;
```

---

## 6. دليل تضمين واستخدام الهيكل الجديد للمطورين (Integration Walkthrough)

يوضح المثال التالي كيف يقوم أي نظام خارجي (نظام مبيعات POS، تطبيق تجارة إلكترونية، أو نظام مصنع) بتضمين النواة واستخدامها بسلاسة:

```go
package main

import (
 "context"
 "log"

 "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/events"
 "github.com/osmai4603-collab/accounting_engine/pkg/api/v1/types"
 "github.com/osmai4603-collab/accounting_engine/pkg/sdk"
 "github.com/osmai4603-collab/accounting_engine/storage/postgres"
)

func main() {
 ctx := context.Background()

 // 1. تهيئة محول التخزين المطلوب (PostgreSQL)
 pgAdapter, err := postgres.NewAdapter(postgres.Config{
  DatabaseURL: "postgres://app_user:secret@localhost:5432/finance_db",
 })
 if err != nil {
  log.Fatalf("فشل الاتصال بقاعدة البيانات: %v", err)
 }

 // 2. بناء المحرك بنمط المنشئ الموحد عبر Go SDK فقط
 engine, err := sdk.NewBuilder(
  sdk.WithStoragePorts(pgAdapter.Ports()),
  sdk.WithReporting(pgAdapter.ReportingStore()), // تفعيل القدرات المدارية اختيارياً
 ).Build(ctx)
 if err != nil {
  log.Fatalf("فشل بناء المحرك المحاسبي: %v", err)
 }
 defer engine.Close()

 // 3. التخاطب حصراً عبر أحداث اقتصادية مجردة (Economic Events)
 invoiceEvent := events.AccountingEvent{
  EventID:        "evt-inv-2026-9901",
  EventType:      "INVOICE_ISSUED",
  IdempotencyKey: "idem-key-inv-9901-v1",
  CompanyID:      types.MustParseCompanyID("cmp-retail-01"),
  // تمرير التفاصيل المالية والحسابات ومفتاح منع التكرار
 }

 // 4. معالجة الحدث وترحيله ذرياً داخل النواة
 entry, err := engine.Events().ProcessEvent(ctx, invoiceEvent)
 if err != nil {
  log.Fatalf("فشل ترحيل الحدث المالي: %v", err)
 }

 log.Printf("تم ترحيل القيد بنجاح! رقم القيد: %s", entry.ID)
}
```

---

> **خلاصة وتأكيد:**  
> يمثل هذا الهيكل الجديد تجسيداً عملياً لمعمارية برمجية نقية وعالية الأداء، تحمي النواة المحاسبية من أي شوائب تقنية، وتجعلها قابلة للتضمين الفوري في أي تطبيق ضمن منظومة `cashflow_system`.
