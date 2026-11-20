---
title: "تفاصيل المرحلة P0: التهيئة والتأسيس — Bootstrap & Setup"
subtitle: "إعداد بيئة المشروع المستهدف accountant_engine وملف go.mod الرشيق وهيكل المجلدات"
document_code: "AE-PLAN-STEP-P0"
phase_id: "P0"
plan_type: "reengineering_embeddable_core"
version: "1.0.0"
date: "2026-11-20"
status: "معتمدة للتنفيذ"
language: "ar"
technical_terms: "en"
---

# المرحلة P0: التهيئة والتأسيس (Bootstrap & Setup)

- **الهدف المحوري للمرحلة:** تهيئة مساحة العمل للمشروع الجديد `accountant_engine`، وتوليد ملف `go.mod` رشيق وخالٍ تماماً من توابع الشبكات والمصادقة، وبناء شجرة المجلدات المتوافقة مع الرؤية المستهدفة، وإعداد أدوات السلامة والفحص الآلي.
- **الاعتماديات العامة للمرحلة:** لا شيء (نقطة انطلاق المشروع).
- **بوابة اعتماد المرحلة:** نجاح تنفيذ `go mod tidy` و `go vet ./...` وإنشاء شجرة المجلدات كاملة بنجاح دون أي خطأ.

---

## تفاصيل مهام المرحلة P0

### `P0-01` — تهيئة بيئة Go للمشروع وإنشاء ملف `go.mod` الرشيق

- **المشكلة التي تحلها:**  
  مشروع المرجع الحالي `accounting_engine` يعاني من تضخم حاد في ملف `go.mod`؛ حيث يشتمل على حزم `google.golang.org/grpc`, `google.golang.org/protobuf`, وحزم `aws-sdk-go-v2` المعقدة، ومكتبات شبكية لا علاقة لها بالنواة المحاسبية، مما يجعل تضمين المحرك كمكتبة خفيفة أمراً مستحيلاً ويفتح ثغرات أمنية غير مبررة.

- **الهدف الذي تحققه:**  
  تأسيس موديول Go نظيف ورشيق باسم الحزمة المستهدفة لمشروعنا، يحتوي حصراً على الاعتماديات الجوهرية (الحساب العشري الدقيق، محرك uuid، ومحرك اتصال pgx لـ Postgres)، وخالٍ بنسبة 100% من أي مكتبات سيرفرات ويب أو gRPC.

- **نطاق المهمة:**  
  - يشمل: إنشاء `go.mod` الجديد، تثبيت إصدار Go المستهدف (`go 1.26.0` أو المعتمد بالبيئة)، وتكوين حزم `decimal`, `uuid`, و `pgx`.
  - لا يشمل: كتابة كود النواة أو نقل ملفات التخزين في هذه المهمة.

- **خطوات التنفيذ التفصيلية:**
  - **ما يجب بناؤه من جديد (Build New):**
    1. إنشاء ملف `go.mod` جديد في جذر `/home/osm/StudioProjects/cashflow_system/accountant_engine/go.mod`.
    2. تسمية الموديول باسم مسار الاستيراد المعتمد للنواة المضمنة.
    3. إضافة الاعتماديات الحتمية الصرفة فقط:

       ```go
       module github.com/osmai4603-collab/accounting_engine // أو مسار الحزمة المعتمد

       go 1.26.0

       require (
           github.com/google/uuid v1.6.0
           github.com/shopspring/decimal v1.6.0
           github.com/jackc/pgx/v5 v5.12.0
           golang.org/x/text v0.43.0
       )
       ```

    4. تشغيل `go mod download` لتحميل الحزم النظيفة في بيئة العمل.
  - **ما يجب نسخه (Copy):**
    - أرقام الإصدارات المستقرة المتوافقة لحزم `uuid`, `decimal`, و `pgx` من `accounting_engine/go.mod` لضمان التوافق مع الكود المنقول.
  - **ما يجب تعديله أو استبعاده (Modify/Exclude):**
    - استبعاد وحذف كل أسطر `require` الخاصة بـ: `google.golang.org/grpc`, `google.golang.org/protobuf`, و `github.com/aws/aws-sdk-go-v2`.

- **الاعتماديات:** لا شيء.

- **معايير القبول:**
  - [x] ملف `go.mod` موجود في جذر المشروع وخالٍ تماماً من أي مرجع لـ `grpc` أو `aws-sdk`.
  - [x] تنفيذ `go mod verify` يمر بنجاح دون أي خطأ تعارض.
  - [x] عدد الاعتماديات المباشرة أقل من 6 حزم مقارنة بـ 15 حزمة في المشروع المرجعي.

---

### `P0-02` — إنشاء شجرة المجلدات المعمارية المستهدفة

- **المشكلة التي تحلها:**  
  الهيكل القديم يخلط بين النواة والخدمة، ويحتوي على مجلدات غير مرغوبة في النواة (`cmd/accounting-service`, `cmd/agy`, `proto/`, `internal/serviceauth`, `internal/ratelimit`). غياب هيكل مجلدات نظيف في بداية العمل يؤدي إلى فوضى الاستيراد وتداخل التبعيات.

- **الهدف الذي تحققه:**  
  تأسيس الشجرة الهيكلية المعتمدة في [NEW_PROJECT_STRUCTURE.md](../NEW_PROJECT_STRUCTURE.md) داخل مستودع المشروع، وتجهيز المسارات لتقسيم العمل إلى (سطح عام `pkg/`، نواة `internal/kernal/`، مدارات `internal/orbitals/`، محولات `storage/`، ووحدات `modules/`).

- **نطاق المهمة:**  
  - يشمل: إنشاء المجلدات الفرعية وملفات `doc.go` التأسيسية التي تشرح مسؤولية كل حزمة.
  - لا يشمل: نقل ملفات التنفيذ الحقيقية (يتم في المراحل اللاحقة).

- **خطوات التنفيذ التفصيلية:**
  - **ما يجب بناؤه من جديد (Build New):**
    1. إنشاء المسارات التالية:
       - `pkg/sdk`, `pkg/api/v1/types`, `pkg/api/v1/ledger`, `pkg/api/v1/events`, `pkg/api/v1/reporting`, `pkg/api/v1/subledger`.
       - `pkg/spi/storage`, `pkg/spi/standards`, `pkg/spi/masterdata`, `pkg/spi/application`.
       - `internal/kernal/posting`, `internal/kernal/masterdata`, `internal/kernal/types`, `internal/kernal/valuation`, `internal/kernal/policy`, `internal/kernal/governance`.
       - `internal/orbitals/subledgers`, `internal/orbitals/reporting`, `internal/orbitals/events`, `internal/orbitals/reconciliation`, `internal/orbitals/consolidation`.
       - `storage/postgres/migrations`, `storage/memory`.
       - `modules/ifrs15`, `modules/ifrs16`, `modules/ifrs9`, `modules/ias12`.
       - `test/unit`, `test/contract_parity`, `test/atomic`, `test/acid`, `test/external_consumer`, `test/benchmark`.
    2. وضع ملف `doc.go` داخل كل حزمة يحدد مسؤوليتها الصريحة والحدود المسموح باستيرادها وفق الميثاق المعماري.
  - **ما يجب نسخه (Copy):** لا شيء.
  - **ما يجب تعديله أو استبعاده (Modify/Exclude):**
    - حظر إنشاء مجلدات `cmd/` أو `proto/` نهائياً.

- **الاعتماديات:** `P0-01`.

- **معايير القبول:**
  - [x] كافة المجلدات المذكورة في الهيكل المستهدف تم إنشاؤها بنجاح.
  - [x] كل مجلد يحتوي على `doc.go` بتعريف الحزمة وميثاق استيرادها.
  - [x] غياب تام لأي مجلد باسم `cmd` أو `proto`.

---

### `P0-03` — إعداد سكربتات فحص السلامة المعمارية وحوكمة التبعيات

- **المشكلة التي تحلها:**  
  خلال عمليات النقل والتعديل المتتابعة، قد يقوم المطور أو الوكيل باستيراد حزمة غير مصرح بها عن طريق الخطأ (مثل استيراد `internal` داخل `pkg/api`، أو استيراد `storage` داخل `internal/kernal/posting`) دون أن يلاحظ، مما يفسد المعمارية النظيفة.

- **الهدف الذي تحققه:**  
  توفير سكربت فحص آلي محلي (`scripts/verify_architecture.sh`) يتحقق من عدم وجود أي استيراد ممنوع، ويمنع وجود تبعيات دائرية، ويضمن عدم تسرب بروتوكولات الشبكة إلى كود النواة.

- **نطاق المهمة:**  
  - يشمل: إنشاء مجلد `scripts/`، كتابة سكربت الفحص المعماري، وسكربت التحقق من تكامل الشيفرة.
  - لا يشمل: تشغيل الفحص على كود كامل (لأن الكود لم ينقل بعد).

- **خطوات التنفيذ التفصيلية:**
  - **ما يجب بناؤه من جديد (Build New):**
    1. إنشاء سكربت `scripts/verify_architecture.sh` يقوم بالآتي:
       - فحص عدم احتواء المشروع على أي استيراد لـ `google.golang.org/grpc` أو `net/http` داخل `internal/*`.
       - فحص عدم احتواء `pkg/api/*` و `pkg/spi/*` على أي استيراد لـ `internal/*`.
       - فحص عدم احتواء `internal/kernal/*` على استيراد لـ `internal/orbitals/*` أو `storage/*`.
    2. إعطاء صلاحية التنفيذ للسكربت `chmod +x scripts/verify_architecture.sh`.
  - **ما يجب نسخه (Copy):**
    - استلهام منطق فحص التوافق من `accounting_engine/scripts/check_api_compat.sh` وتكييفه.
  - **ما يجب تعديله (Modify):**
    - تكييف قواعد الفحص لتطابق التقسيم الجديد (`internal/kernal` مقابل `internal/orbitals`).

- **الاعتماديات:** `P0-02`.

- **معايير القبول:**
  - [x] السكربت `scripts/verify_architecture.sh` موجود وقابل للتنفيذ.
  - [x] تشغيل السكربت على الهيكل الحالي يمر بنجاح ويعطي نتيجة خضراء (Clean Base).

---

## سجل بوابة اعتماد المرحلة P0

- **تاريخ الفحص المستهدف:** فور اكتمال P0-03.
- **الأدلة المطلوبة:** مخرجات `go mod verify`، شجرة المجلدات، ومخرجات فحص `scripts/verify_architecture.sh`.
- **نتيجة التنفيذ الفعلية (2026-11-20):** بوابة P0 **معتمدة** — جميع مهام `P0-01`..`P0-03` مكتملة ومعايير القبول الثمانية مُنجزة.
- **الأدلة المسجلة:**
  - `go mod download` + `go mod tidy`: ناجحان؛ `go.mod` يحتوي على 4 تبعيات مباشرة فقط (`uuid`, `decimal`, `pgx/v5`, `x/text`) وصفر مراجع لـ `grpc`/`protobuf`/`aws-sdk`.
  - `go mod verify`: `all modules verified`.
  - `go vet ./...`: ناجح دون أخطاء؛ `gofmt -l .`: فارغ.
  - شجرة المجلدات: 45 حزمة/مجلد كلها تحتوي على `doc.go` بميثاق استيراد، وصفر مجلدات `cmd/` أو `proto/`.
  - `./scripts/verify_architecture.sh`: `RESULT: PASS - clean architectural base` (8 قواعد، إضافة إلى اختبار سلبي أكد كشف الانتهاكات R1/R2/R3a/R3b).
  - `./scripts/verify_integrity.sh`: `RESULT: PASS - source integrity verified` (6 فحوصات، بما فيها تكرارية `go mod tidy`).
  - `tools.go` (وسم البناء `tools`) يثبّت التبعيات الأربع حتى استيرادها الفعلي في P1، ويمنع `go mod tidy` من حذفها في هذه المرحلة.
- **ملاحظة حوكمة:** مسار الموديول أُبقي على `github.com/osmai4603-collab/accounting_engine` كما في `NEW_PROJECT_STRUCTURE.md`، بينما مجلد المشروع المستهدف هو `accountant_engine`؛ ذلك يضمن نسخ ملفات المصدر في المراحل القادمة دون تعديل مسارات الاستيراد.
