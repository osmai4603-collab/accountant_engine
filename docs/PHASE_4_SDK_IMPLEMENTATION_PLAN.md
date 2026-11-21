---
title: "الخطة التنفيذية للمرحلة P4: السطح البرمجي العام للمكتبة — SDK Public Surface Implementation Plan"
subtitle: "خطة تفصيلية لبناء سطح SDK والمحرك الموحد واستيعاب الأحداث والمستهلك الخارجي المستقل"
document_code: "AE-PLAN-EXEC-P4"
phase_id: "P4"
plan_type: "reengineering_embeddable_core"
version: "1.0.0"
date: "2026-11-21"
status: "معتمدة للتنفيذ"
project: "accountant_engine"
reference_repo: "accounting_engine"
language: "ar"
technical_terms: "en"
---

# الخطة التنفيذية للمرحلة P4: السطح البرمجي العام للمكتبة (SDK Public Surface)

## 1. ملخص المرحلة والهدف المحوري

- **الهدف المحوري للمرحلة:**  
  بناء وتقديم السطح البرمجي العام النهائي للنواة المحاسبية عبر حزمة `pkg/sdk`، وتوفير المنشئ الموحد الصارم `sdk.NewBuilder` بمسار وحيد حصراً يعتمد على موانئ التخزين المجردة (`WithStoragePorts`)، وتوفير كائن المحرك `sdk.Engine` وخدماته، وبناء خدمة استيعاب الأحداث الاقتصادية ومنع التكرار (`EventIngestionService`) وقوالب القطاعات، وإثبات قابلية التضمين الكاملة عبر مستهلك خارجي مستقل في `test/external_consumer` لا يستورد أي كود داخلي إطلاقاً.
- **الاعتماديات العامة:** اعتماد واكتمال المرحلة `P3` (محولات التخزين ووحدة العمل).
- **بوابة اعتماد المرحلة:** نجاح بناء حزمة `pkg/sdk` وقوالبها، واجتياز 100% من اختبارات SDK واختبارات المستهلك الخارجي المستقل في `test/external_consumer` دون استيراد أي حزمة داخلية، مع بقاء كود النواة `internal/kernal` دون أي تغيير.

---

## 2. القيود والضوابط الصارمة (Strict Constraints)

1. **حظر تعديل النواة نهائياً (Zero-Change to `internal/kernal`):**
   - يُحظر إجراء أي تعديل أو إضافة على أي ملف داخل `internal/kernal/*`.
   - النواة المحاسبية مكتملة ومعتمدة في المرحلة P2 ومغلقة أمام أي تعديل.
2. **حظر اختبار النواة (No Testing for Kernal):**
   - يُمنع إنشاء أو تعديل أي اختبارات تستهدف داخليات `internal/kernal`.
   - كافة الاختبارات في هذه المرحلة محصورة حصراً في سطح المكتبة العام (`pkg/sdk/*`) واختبارات المستهلك الخارجي (`test/external_consumer/*`).
3. **التطهير الكامل من المسارات القديمة (Zero Legacy Paths):**
   - استبعاد مسار `WithCoreStorage` نهائياً؛ المنشئ يدعم حصراً `WithStoragePorts(ports storage.Ports)`.
   - خلو كامل من حزم `legacyengine` أو `serviceauth` أو أي تعريفات gRPC/HTTP أو بروتوكولات شبكية داخل المحرك.

---

## 3. تحليل النواقص والفجوات مقارنة بالمشروع المرجعي (Gaps Analysis)

بناءً على فحص كود المشروع المرجعي `/home/osm/StudioProjects/cashflow_system/accounting_engine/pkg/sdk`، تم رصد المكونات التالية التي لم تذكر أو لم تفصل بالكامل في وثيقة المرحلة الأصلية، وتم اعتماد إضافتها ضمن هذه الخطة:

| المكون في المشروع المرجعي | الحالة في وثيقة P4 الأصلية | الإجراء المعتمد في الخطة | التبرير التقني والمعماري |
| :--- | :--- | :--- | :--- |
| **`pkg/sdk/posting_config.go`** | ذُكر كإشارة سريعة | **مهمة تفصيلية** | هيكل `PostingConfig` وضوابط تفعيل شروط الترحيل الإلزامية (الموافقة، الأبعاد، السياسات). |
| **`pkg/sdk/bootstrap_capabilities.go`** | ذُكر كإشارة سريعة | **مهمة تفصيلية** | مصفوفة التحقق الإقلاعي للقدرات المطلوبة (`WithRequiredCapabilities`) للفشل السريع ومنع التشغيل الجزئي. |
| **`pkg/sdk/module_ports.go`** | غير مذكور | **إضافة كاملة** | موانئ `standards.ModulePorts` لتغذية المعايير المحاسبية عبر `storage.Ports` دون كشف التخزين. |
| **`pkg/sdk/cash_flow.go`** | غير مذكور | **إضافة كاملة** | تكامل استعلامات قائمة التدفقات النقدية (`GetCashFlow`) ضمن `ReportingService`. |
| **`pkg/sdk/templates/adapters.go`** | غير مذكور | **إضافة كاملة** | قوالب أحداث القطاعات الاقتصادية (التجزئة Retail، الزراعة Agriculture، البرمجيات SaaS). |
| **`pkg/sdk/errors.go`** | منصوص في ميثاق الهيكل الجديد | **إضافة كاملة** | تصدير أخطاء النطاق المعيارية (`Domain Sentinel Errors`) الموحدة لمستهلكي المكتبة. |
| **`pkg/sdk/event_recovery_test.go`** | غير مذكور | **إضافة كاملة** | اختبارات استئناف وتعافي الأحداث المتوقفة أو المعلقة. |
| **`pkg/sdk/consumer_contract_test.go`** | غير مذكور | **إضافة كاملة** | اختبارات عقود المستهلك المباشر مع SDK. |
| **`test/external_consumer/ports_adapter_test.go`** | غير مذكور | **إضافة كاملة** | إثبات عمل المحرك مع محول موانئ تخزين خارجي مستقل كلياً (`externalPortsAdapter`). |
| **`test/external_consumer/p7_industry_consumers_test.go`** | غير مذكور | **إضافة كاملة** | سيناريوهات استهلاك قطاعية واقعية (نقاط بيع، مزارع، مصانع). |

---

## 4. شجرة الملفات المستهدفة للمرحلة P4

```text
accountant_engine/
├── pkg/
│   └── sdk/
│       ├── doc.go                           # وثيقة الحزمة وقواعد الاستيراد (موجود)
│       ├── errors.go                        # أخطاء النطاق العامة المصدرة (New)
│       ├── engine.go                        # واجهة Engine الرئيسية والخدمات العامة (Copy & Adapt)
│       ├── builder.go                       # منشئ المحرك الموحد sdk.NewBuilder (Build New & Clean)
│       ├── options.go                       # خيارات التكوين العامة (New)
│       ├── posting_config.go                # متطلبات وضوابط الترحيل (Copy & Adapt)
│       ├── bootstrap_capabilities.go        # مصفوفة التحقق الإقلاعي (Copy & Adapt)
│       ├── module_ports.go                  # موانئ المعايير الدولية المجردة (Copy & Adapt)
│       ├── application.go                   # طبقة التطبيق والتفويض المباشر للنواة (Build New)
│       ├── event_ingestion.go               # خدمة استيعاب وتوجيه الأحداث ومنع التكرار (Copy & Adapt)
│       ├── cash_flow.go                     # محولات ونماذج خدمة التدفقات النقدية (Copy & Adapt)
│       ├── templates/                       # حزمة قوالب القطاعات الاقتصادية
│       │   ├── doc.go                       # وثيقة القوالب
│       │   ├── adapters.go                  # محولات التجزئة والزراعة والـ SaaS (Copy & Adapt)
│       │   └── adapters_test.go             # اختبارات قوالب القطاعات (Copy & Adapt)
│       ├── builder_test.go                  # اختبارات المنشئ وفحص الفشل السريع
│       ├── application_test.go              # اختبارات تفويض التطبيق والصلاحيات
│       ├── event_ingestion_test.go          # اختبارات استيعاب الأحداث
│       ├── event_idempotency_test.go        # اختبارات منع تكرار الأحداث والتعارض
│       ├── event_recovery_test.go           # اختبارات تعافي استيعاب الأحداث
│       └── consumer_contract_test.go        # اختبارات عقود المستهلك للـ SDK
└── test/
    └── external_consumer/
        ├── doc.go                           # وثيقة المستهلك الخارجي (موجود)
        ├── go.mod                           # موديول Go مستقل يثبت التضمين النظيف
        ├── go.sum
        ├── consumer_test.go                 # سيناريو المستهلك الكامل + فحص حظر استيراد internal
        ├── ports_adapter_test.go            # إثبات العمل بمحول موانئ مستقل بالكامل
        └── p7_industry_consumers_test.go    # سيناريوهات المستهلكين بالقطاعات المختلفة
```

---

## 5. خطة المهام التفصيلية (Detailed Execution Tasks)

### المهمة `P4-01`: المنشئ الصارم الموحد وخيارات التكوين ومصفوفة الإقلاع

- **الأهداف:**
  - بناء المنشئ الموحد `sdk.NewBuilder` بحيث يقبل حصراً موانئ التخزين المجردة `sdk.WithStoragePorts(ports)`.
  - التحقق المسبق الصارم (`preflight`) من سلامة الموانئ عبر `ports.Validate()`.
  - دعم مصفوفة القدرات الإقلاعية `WithRequiredCapabilities` للتحقق الفوري ومنع بدء تشغيل ناقص.
  - إزالة أي أثر لمسار `WithCoreStorage` أو حزمة `legacyengine`.
- **الملفات المستهدفة:**
  - `pkg/sdk/errors.go`
  - `pkg/sdk/posting_config.go`
  - `pkg/sdk/bootstrap_capabilities.go`
  - `pkg/sdk/options.go`
  - `pkg/sdk/builder.go`
  - `pkg/sdk/builder_test.go`
- **معايير القبول:**
  - [ ] البناء ينجح فقط عند تمرير موانئ كاملة عبر `WithStoragePorts`.
  - [ ] الفشل الفوري في مرحلة البناء مع توضيح المنفذ الناقص إذا كانت الموانئ غير مكتملة.
  - [ ] خلو الملفات تماماً من أي استيراد لـ `internal/legacyengine` أو `pkg/spi/legacy`.

---

### المهمة `P4-02`: واجهة المحرك وخدماته العامة والتفويض للتطبيق

- **الأهداف:**
  - تعريف كائن `sdk.Engine` الذي يجمع الخدمات العامة:
    - `Ledger() LedgerService`
    - `Reporting() ReportingService`
    - `Governance() GovernanceService`
    - `Events() EventIngestionService`
    - `Standards() StandardsService`
    - `Scheduler() standards.SchedulerPort`
    - `Close() error` و `CloseWithContext(ctx context.Context) error`
  - تطبيق نمط التفويض المباشر (Direct Delegation) في `pkg/sdk/application.go` للربط بين موانئ التخزين ونواة الترحيل المحاسبية `internal/kernal/posting.PostingPipeline` ومحركات السياسات والبيانات الرئيسية دون أي تعديل في كود النواة.
  - بناء موانئ المعايير `module_ports.go` واستعلامات التدفق النقدي `cash_flow.go`.
- **الملفات المستهدفة:**
  - `pkg/sdk/engine.go`
  - `pkg/sdk/module_ports.go`
  - `pkg/sdk/application.go`
  - `pkg/sdk/cash_flow.go`
  - `pkg/sdk/application_test.go`
  - `pkg/sdk/consumer_contract_test.go`
- **معايير القبول:**
  - [ ] كافة دوال الخدمات تستقبل `ctx context.Context` كمعامل أول وتعتمد على كائنات `pkg/api/v1/*`.
  - [ ] ترحيل واسترجاع وعكس القيود يتم بنجاح عبر `LedgerService`.
  - [ ] اختبارات إغلاق المحرك الآمن (`Graceful Shutdown`) تمر بنجاح دون تسريب للموارد.

---

### المهمة `P4-03`: خدمة استيعاب الأحداث وقوالب القطاعات الاقتصادية

- **الأهداف:**
  - بناء `EventIngestionService` لاستقبال الأحداث الاقتصادية (`AccountingEvent`)، والتحقق من سلامتها، وحساب البصمة المشفرة للحدث (`publicEventFingerprint`).
  - تطبيق التحقق الصارم من مفاتيح عدم التكرار (`IdempotencyKey`) بالاعتماد على `ports.Events.Idempotency`.
  - إعادة نتيجة القيد السابق فوراً عند تكرار نفس المفتاح والحمولة (Idempotent Replay).
  - إرجاع خطأ تعارض صريح `ErrIdempotencyKeyConflict` عند استخدام نفس المفتاح لبيانات مختلفة.
  - بناء قوالب القطاعات الاقتصادية في `pkg/sdk/templates/adapters.go` لدعم قطاعات التجزئة، الزراعة، و SaaS.
- **الملفات المستهدفة:**
  - `pkg/sdk/event_ingestion.go`
  - `pkg/sdk/templates/doc.go`
  - `pkg/sdk/templates/adapters.go`
  - `pkg/sdk/templates/adapters_test.go`
  - `pkg/sdk/event_ingestion_test.go`
  - `pkg/sdk/event_idempotency_test.go`
  - `pkg/sdk/event_recovery_test.go`
- **معايير القبول:**
  - [ ] استيعاب الحدث الاقتصادي وتوليد قيد مرحل متوازن بنجاح.
  - [ ] إعادة إرسال نفس الحدث يعيد نفس القيد دون إنشاء قيد مكرر.
  - [ ] إرسال حمولة مختلفة بنفس المفتاح يفشل فوراً بخطأ تعارض صريح.
  - [ ] قوالب القطاعات تولد أحداثاً مطابقة لقواعد الترحيل المعتمدة.

---

### المهمة `P4-04`: بناء موديول المستهلك الخارجي المستقل وإثبات التضمين

- **الأهداف:**
  - بناء موديول اختباري خارجي مستقل بالكامل داخل `test/external_consumer/` بملف `go.mod` خاص به يثبت قابلية التضمين المباشر للنواة كمكتبة Go مستقلة.
  - كتابة اختبار رحلة المستهلك الكاملة (`consumer_test.go`).
  - تضمين فحص آلي صارم `TestNoInternalImports` يتأكد من أن ملفات المستهلك لا تحتوي على أي استيراد لكلمة `internal`.
  - بناء اختبار `ports_adapter_test.go` لإثبات عمل المحرك مع محول موانئ تخزين خارجي بالكامل.
  - بناء اختبار `p7_industry_consumers_test.go` لمحاكاة سيناريوهات أعمال حقيقية (نقاط بيع، زراعة، مصانع).
- **الملفات المستهدفة:**
  - `test/external_consumer/go.mod`
  - `test/external_consumer/consumer_test.go`
  - `test/external_consumer/ports_adapter_test.go`
  - `test/external_consumer/p7_industry_consumers_test.go`
- **معايير القبول:**
  - [ ] اجتياز 100% من اختبارات المستهلك الخارجي.
  - [ ] نجاح فحص `TestNoInternalImports` وخلو كود المستهلك من أي استيراد لـ `internal/*`.
  - [ ] إثبات التضمين النقي دون الحاجة لأي خادم شبكي خارجي.

---

## 6. مصفوفة التحقق وبوابة اعتماد المرحلة (Stage Gate Verification Matrix)

| الاختبار / الإجراء | الأمر التنفيذي | النتيجة المستهدفة |
| :--- | :--- | :---: |
| **بناء سطح المكتبة SDK** | `go build ./pkg/sdk/...` | PASS (بدون أي أخطاء) |
| **بناء قوالب القطاعات** | `go build ./pkg/sdk/templates/...` | PASS |
| **فحص عدم استيراد التخزين المباشر** | فحص استيرادات ملفات `pkg/sdk/*.go` | عدم وجود استيراد مباشر لـ `storage/postgres` أو `storage/memory` |
| **اختبارات وحدة SDK** | `go test -v -race ./pkg/sdk/...` | PASS (100% نجاح) |
| **اختبارات قوالب القطاعات** | `go test -v -race ./pkg/sdk/templates/...` | PASS (100% نجاح) |
| **اختبارات المستهلك الخارجي** | `cd test/external_consumer && go test -v -race ./...` | PASS (100% نجاح) |
| **فحص منع استيراد الداخليات** | `cd test/external_consumer && go test -run TestNoInternalImports` | PASS |
| **فحص حرمة النواة المحاسبية** | `git status internal/kernal/` | No changes (صفر تعديلات) |
