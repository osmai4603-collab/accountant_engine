---
title: "قواعد العمل الهندسية والتقنية الشاملة — Engineering & Technical Rules"
subtitle: "الحصر الكامل للضوابط والمعايير الهندسية والمعمارية لمحرك المحاسبة (Accounting Engine)"
document_code: "AE-RULE-ENG-001"
version: "1.0.0"
date: "2026-11-20"
status: "وثيقة معيارية معتمدة"
project: "Accounting Engine"
classification: "Software Engineering & Architecture Standards"
language: "ar"
technical_terms: "en"
---

# قواعد العمل الهندسية والتقنية الشاملة (Engineering & Technical Rules)

> **بيان الإلزام والهدف:**  
> تجمع هذه الوثيقة وتحصر بدقة **كافة قواعد العمل الهندسية والتقنية والمعمارية** المعتمدة لمشروع `accounting_engine` (استناداً إلى وثائق المعمارية `project_specification_and_architecture.md`, `ENGINEERING_AND_ACCOUNTING_STANDARD.md`, ومعايير ISO/IEC 25010 و NIST SSDF).  
> تهدف هذه القواعد لضمان سلامة النواة البرمجية، متانتها، كفاءتها التشغيلية، وعزلها التام كـ Embeddable Core Engine خالية من الشوائب والاقتران التقني الضار.

---

## فهرس المحتويات

1. [المعمارية واستقلالية النواة (Architecture & Core Isolation)](#1-المعمارية-واستقلالية-النواة-architecture--core-isolation)
2. [المعاملات والذرية التامة (Transactions & ACID Invariants)](#2-المعاملات-والذرية-التامة-transactions--acid-invariants)
3. [منع التكرار والحتمية (Idempotency & Determinism)](#3-منع-التكرار-والحتمية-idempotency--determinism)
4. [الأمان وتعدد الشركات والعزل (Multi-Tenancy & Tenant Isolation)](#4-الأمان-وتعدد-الشركات-والعزل-multi-tenancy--tenant-isolation)
5. [التزامن وسلامة البيانات (Concurrency & Locking Rules)](#5-التزامن-وسلامة-البيانات-concurrency--locking-rules)
6. [الربط بالبنية التحتية والتخزين (Storage & Persistence SPI)](#6-الربط-بالبنية-التحتية-والتخزين-storage--persistence-spi)
7. [التدقيق الجنائي والرقابة (Auditability & Append-Only Logs)](#7-التدقيق-الجنائي-والرقابة-auditability--append-only-logs)
8. [معايير الشيفرة ولغة Go (Go Programming & Design Standards)](#8-معايير-الشيفرة-ولغة-go-go-programming--design-standards)
9. [معايير الاختبارات وضمان الجودة (Testing & Verification Standards)](#9-معايير-الاختبارات-وضمان-الجودة-testing--verification-standards)
10. [الأداء وقابلية التوسع (Performance & Scalability Rules)](#10-الأداء-وقابلية-التوسع-performance--scalability-rules)

---

## 1. المعمارية واستقلالية النواة (Architecture & Core Isolation)

* **[RULE-ENG-001] التبعية المعمارية الصارمة (Clean / Hexagonal Architecture):**
  * يجب أن تتبع كافة الحزم اتجاه التبعية إلى الداخل (Dependency Inversion):
    $$\text{Infrastructure / Adapters} \longrightarrow \text{Public SDK / SPI} \longrightarrow \text{Core Domain}$$
  * يُمنع منعاً باتاً استيراد أي حزمة بنية تحتية (`storage`, `postgres`, `http`, `grpc`, `sql`) داخل طبقة النواة (`internal/posting`, `internal/masterdata`, `internal/types`).

* **[RULE-ENG-002] حظر استيراد `internal/` للمستهلك الخارجي (Package Encapsulation):**
  * يُحظر على أي كود خارجي أو مستهلك للنواة استيراد أي حزمة داخل مسار `internal/*`.
  * السطح البرمجي العام المتاح للمستهلك هو حصراً: `pkg/sdk`, `pkg/api/v1`, و `pkg/spi`.

* **[RULE-ENG-003] حيادية بروتوكول النقل (Transport Agnosticism):**
  * النواة المحاسبية هي مكتبة Go برمجية (Embedded Go Library) فقط.
  * يُمنع تضمين أي خادم شبكي (HTTP REST, gRPC, WebSocket) أو أي موائم شبكي داخل النواة المحاسبية.
  * طبقات النقل الشبكي تعيش خارج مستودع النواة في خدمات مستقلة.

* **[RULE-ENG-004] تجريد عمليات المصادقة والتفويض (Auth-Free Core):**
  * يُمنع تضمين جداول المستخدمين، كلمات المرور، جلسات الدخول، التحقق من JWT/OAuth، أو Rate Limiting داخل النواة.
  * تتلقى النواة سياق أمان موثوقاً ومجرداً (`Security Context / Principal`) ممرراً عبر `context.Context` وتثبته فقط في سجلات التدقيق دون إدارة هويات.

* **[RULE-ENG-005] مسار البناء والتركيب الموحد (Single Builder Path):**
  * توفير واجهة بناء موحدة حصراً عبر نمط المنشئ:

    ```go
    engine, err := sdk.NewBuilder(sdk.WithStoragePorts(ports)).Build(ctx)
    ```

  * حظر مسارات البناء القديمة أو المزدوجة المتجاوزة للعقود المجردة.

---

## 2. المعاملات والذرية التامة (Transactions & ACID Invariants)

* **[RULE-ENG-006] وحدة العمل الذرية الشاملة (All-or-Nothing Unit of Work):**
  * ترحيل القيد المحاسبي هو معاملة ذرية متكاملة تتضمن:
    1. حفظ رأس القيد.
    2. حفظ كافة أسطر القيد.
    3. تحديث أرصدة الحسابات المتأثرة.
    4. تسجيل حركات الدفاتر الفرعية (إن وجدت).
    5. حفظ سجل التدقيق المرتبط.
    6. حفظ قيد منع التكرار (`Idempotency Record`).
  * في حال فشل أي خطوة من هذه الخطوات الست، يجب أن تتراجع المعاملة بالكامل (`Full Transaction Rollback`) فوراً، ويُحظر حدوث أي حفظ جزئي نهائياً.

* **[RULE-ENG-007] مستويات عزل المعاملات في قاعدة البيانات (Transaction Isolation):**
  * يجب ألا يقل مستوى عزل قاعدة البيانات عن `READ COMMITTED` مع أقفال صريحة أو `SERIALIZABLE` لمنع القراءة المتسخة (Dirty Reads) وتداخل المعاملات المتزامنة.

---

## 3. منع التكرار والحتمية (Idempotency & Determinism)

* **[RULE-ENG-008] إلزامية مفتاح عدم التكرار (Mandatory Idempotency Key):**
  * كل طلب ترحيل قيد أو معالجة حدث اقتصادي يجب أن يحمل مفتاح عدم تكرار فريد (`Idempotency-Key != empty`).
  * يرفض المحرك فوراً أي طلب يفتقر إلى هذا المفتاح.

* **[RULE-ENG-009] معالجة تكرار الطلب المتطابق (Safe Idempotent Replay):**
  * إذا استقبل المحرك طلباً يحمل مفتاحاً تم تنفيذه مسبقاً بنفس الحمولة (`Payload Hash`)، يجب عدم إعادة تنفيذ المعاملة المالية، بل إرجاع نفس النتيجة الأصلية السابقة مع رمز نجاح حتمي.

* **[RULE-ENG-010] كشف التعارض الحتمي في مفاتيح التكرار (Idempotency Conflict):**
  * إذا استقبل المحرك طلباً يحمل مفتاح تكرار مسجل مسبقاً لحمولة أو بيانات مختلفة، يجب إرجاع خطأ تعارض صريح (`ErrIdempotencyKeyConflict`) دون إجراء أي تعديل مالي.

* **[RULE-ENG-011] الحتمية البرمجية (Execution Determinism):**
  * يجب أن ينتج عن نفس المدخلات (قيد، أسعار صرف، تاريخ) نفس الأثر المالي والقيود والأرصدة في كل مرة بصرف النظر عن بيئة التشغيل أو التوقيت.

---

## 4. الأمان وتعدد الشركات والعزل (Multi-Tenancy & Tenant Isolation)

* **[RULE-ENG-012] العزل الصارم بين الشركات (Strict Multi-Tenancy Isolation):**
  * بيانات كل شركة (`CompanyID / TenantID`) معزولة بالكامل منطقياً وأمنياً.
  * لا يجوز لأي استعلام أو عملية ترحيل خاصة بشركة A أن تقرأ أو تؤثر على حسابات أو قيود أو أرصدة شركة B.

* **[RULE-ENG-013] عدم الثقة بمعرف الشركة في متن الطلب وحده (Tenant Context Verification):**
  * يُحظر الاعتماد الأمني الحصري على معرف الشركة الممرر داخل كائن القيد (`Payload CompanyID`).
  * يجب التحقق الإلزامي من تطابق معرف الشركة في الحمولة مع سياق الأمان الموثوق (`Context Company Claims`) قبل الشروع في الترحيل.

* **[RULE-ENG-014] تطبيق أمان مستوى الصفوف (PostgreSQL Row-Level Security - RLS):**
  * في محولات قواعد البيانات المعتمدة (مثل PostgreSQL)، يجب تطبيق سياسات RLS على مستوى جداول المحرك لحماية البيانات حتى في حال تسرب الاستعلامات البرمجية.

---

## 5. التزامن وسلامة البيانات (Concurrency & Locking Rules)

* **[RULE-ENG-015] حظر تعارض الأرصدة والسباق (Race Conditions & Deadlock Prevention):**
  * يجب ترتيب تحديث الحسابات المتزامنة بتسلسل تصاعدي ثابت وفق معرف الحساب (`Ordered Locking by AccountID ASC`) عند حجز أقفال الصفوف (`SELECT FOR UPDATE`) لمنع حدوث Deadlocks بين القيود المتقاطعة.

* **[RULE-ENG-016] قفل الأرصدة اللحظي المتزامن:**
  * يتم حجز أقفال الصفوف الخاصة بأرصدة الحسابات المتأثرة بالقيد فقط طوال مدة المعاملة الذرية، مع ضمان سرعة الإفراج عنها عند انتهاء الترحيل.

---

## 6. الربط بالبنية التحتية والتخزين (Storage & Persistence SPI)

* **[RULE-ENG-017] التجريد التام لطبقة التخزين (Persistence SPI):**
  * تتعامل النواة حصراً مع واجهات مجردة (`pkg/spi/storage`):
    * `UnitOfWork`
    * `AccountStore`
    * `EntryStore`
    * `BalanceStore`
    * `AuditStore`
  * تغيير قاعدة البيانات من PostgreSQL إلى أي محرك آخر يتم بتحقيق واجهات SPI دون لمس سطر واحد في كود النواة المحاسبية.

* **[RULE-ENG-018] حظر منطق الأعمال داخل استعلامات SQL:**
  * تُحظر الإجراءات المخزنة (Stored Procedures) أو Triggers المعقدة التي تحتوي على منطق محاسبي أو تقييمات ائتمانية داخل قاعدة البيانات.
  * تقتصر استعلامات SQL على عمليات الإدخال والاسترجاع والأقفال وحفظ النزاهة الهيكلية فقط؛ والمنطق المحاسبي يقطن حصراً في Go Domain.

---

## 7. التدقيق الجنائي والرقابة (Auditability & Append-Only Logs)

* **[RULE-ENG-019] نمط الإضافة فقط لسجلات التدقيق (Append-Only Audit Logs):**
  * كافة سجلات التدقيق والمتابعة غير قابلة للتعديل أو الحذف، ويتم إدراجها كسطور تاريخية متتالية.
  * تسجيل هوية المنفذ الحقيقي (`Principal/Actor`)، الطابع الزمني بالنانوثانية، معرف الحدث، وسبب المعاملة مع كل عملية ترحيل أو إلغاء.

* **[RULE-ENG-020] سلامة سلسلة التجزئة (Audit Hash Chaining):**
  * دعم التحقق الجنائي من سلامة سجلات التدقيق عبر ربط السجلات المتعاقبة بسلسلة تجزئة رياضية (Cryptographic Hash Chaining)، بحيث يُكتشف فوراً أي تلاعب مباشر في سجلات قاعدة البيانات.

---

## 8. معايير الشيفرة ولغة Go (Go Programming & Design Standards)

* **[RULE-ENG-021] إلزامية `context.Context` في كافة العمليات:**
  * كل دالة تؤدي عملاً معالجياً أو تتواصل مع موانئ التخزين يجب أن تستقبل `ctx context.Context` كمعامل أول، وتلتزم بمراقبة الإلغاء والمهل الزمنية (`Timeouts & Cancellation`).

* **[RULE-ENG-022] معالجة الأخطاء الصريحة وتفادي الذعر (No Panics):**
  * يُمنع استخدام `panic()` داخل مسارات الإنتاج لمعالجة الأخطاء التشغيلية أو المحاسبية.
  * يجب إرجاع أخطاء صريحة موصوفة، واستخدام فئات أخطاء المجال المعرفة (`Domain Sentinel Errors` مثل `ErrUnbalancedEntry`, `ErrClosedPeriod`).

* **[RULE-ENG-023] التحرر من التأثيرات الجانبية عند التهيئة:**
  * بناء كائن المحرك عبر `NewBuilder` يجب ألا يبدأ اتصالات شبكية مخفية أو خيوط معالجة دورية ما لم يطلب المستخدم ذلك صراحة عبر خيارات التكوين.

---

## 9. معايير الاختبارات وضمان الجودة (Testing & Verification Standards)

* **[RULE-ENG-024] تغطية الاختبارات الآلية (Coverage Floor):**
  * يجب ألا تقل نسبة التغطية الاختبارية الإجمالية للمشروع عن 80% في بوابة الإنتاج، مع استهداف تغطية تفوق 95% لطبقة النواة المحاسبية (`internal/posting`).

* **[RULE-ENG-025] هرم الاختبارات المتكامل (Testing Pyramid):**
  * يجب أن يخضع المحرك لأربع طبقات اختبارية إلزامية:
    1. **اختبارات الوحدة (Unit Tests):** تعمل بالكامل على موائم الذاكرة وتغطي القواعد المحاسبية والحالات الحدية.
    2. **اختبارات تكافؤ العقود (Contract Parity Tests):** تضمن مطابقة سلوك موائم الذاكرة مع موائم PostgreSQL بنسبة 100%.
    3. **اختبارات التكامل والذرية (ACID & Integration Tests):** تختبر التزامن، الأقفال، RLS، وحالات حقن الأعطال (Fault Injection).
    4. **اختبارات المستهلك الخارجي (External Consumer Tests):** تضمن إمكانية بناء واستدعاء المحرك من حزمة مستقلة كلياً تستورد `pkg/sdk` فقط.

---

## 10. الأداء وقابلية التوسع (Performance & Scalability Rules)

* **[RULE-ENG-026] الحد الأعلى لزمن الاستجابة (Latency Invariant):**
  * يجب ألا يتجاوز زمن ترحيل القيد المحاسبي الاعتيادي حاجز $50\,\text{ms}$ عند الشريحة المئوية 95 ($P95$) تحت ملف الحمل القياسي المعتمد.

* **[RULE-ENG-027] ترشيد استهلاك الذاكرة وتفادي التسريبات (Zero Leakage):**
  * تفادي التخصيص غير الضروري للذاكرة (Allocations) في مسارات الترحيل الحساسة، والتأكد من انعدام تسريبات الذاكرة (Memory Leaks) في موائمات التخزين أو خيوط معالجة الأحداث.

---

> **خلاصة وتأكيد:**  
> تشكل هذه القواعد الـ 27 الحصن الهندسي والمعماري لمشروع `accounting_engine`. وتعد ملزمة لجميع المهندسين ومراجعي الكود لضمان الحفاظ على نقاء وموثوقية النواة المحاسبية.
