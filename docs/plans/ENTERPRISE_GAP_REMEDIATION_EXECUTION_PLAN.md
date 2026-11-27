# الخطة التنفيذية لسد الفجوات والوصول إلى الجاهزية المؤسسية (`accountant_engine`)

- **تاريخ الإعداد:** 2026-11-27
- **الحالة:** مقترحة للتنفيذ (تستهدف معالجة الفجوات المحاسبية، والتكاملية، والمعيارية، والتشغيلية، والاعتماد)
- **المبدأ الحاكم:** عدم التوسع في إضافة وظائف جديدة قبل إكمال عقود **Public SPI** و**Application Layer** وإثبات التكافؤ المحاسبي بين محولات التخزين (`Memory` و`PostgreSQL`).

---

## ملخص المراحل الخمس

| المرحلة | الهدف الرئيسي | المخرجات الحرجة |
| --- | --- | --- |
| **المرحلة 1** | إكمال العقود العامة (`Public API / SPI`) | عقود موحدة لفترات التسوية، وإلغاء المسودة، وإقفال السنة |
| **المرحلة 2** | إكمال `Application Layer` وربط المحولات | تنفيذ `CancelDraft`، `GetPeriodByNumber`، و`YearEndClosingService` في `pkg/sdk` و`storage/` |
| **المرحلة 3** | حسم نطاق المحاسبة المؤسسية والمعايير | قرار معماري (`ADR`) لـ `Multi-Ledger` + خطة استكمال `IFRS 9/15/16` و`IAS 12` + خريطة نشر المدارات |
| **المرحلة 4** | إثبات الاستقلال والتكافؤ والتوثيق | تكافؤ `Memory` / `PostgreSQL` / `External Consumer` + مصفوفة التتبع وتصحيح الوثائق |
| **المرحلة 5** | الاعتماد الإنتاجي والحوكمة | اختبارات الحمل والتعافي (`RPO/RTO`) + مراجعة مستقلة + اعتماد إصدار جديد |

---

## المرحلة 1: إكمال العقود العامة (`Public API / SPI`)

**الهدف:** توحيد العقود العامة بحيث لا تبقى قدرات النواة محجوبة أو معتمدة على اكتشاف اختياري غير منفذ.

### المهام

1. **توحيد عقد فترات التسوية (13 و14):**
   - مراجعة `pkg/spi/masterdata/masterdata.go` وإضافة عقد صريح للبحث برقم الفترة (`GetPeriodByNumber(ctx, companyID, fiscalYearID, number)`) أو واجهة متخصصة واضحة في `pkg/spi/masterdata` مع التحقق من التوافق الرجعي عبر `scripts/check_api_compat.sh`.
   - تحديث `pkg/sdk/application.go` لإزالة المسار الذي يعيد `"adjustment periods not supported"` عند توفر العقد الموحد.
2. **إضافة عقد إلغاء المسودة (`CancelDraft`):**
   - إضافة `ActionCancel` و`CancelDraftCommand` في `pkg/spi/application/application.go`.
   - تحديد واجهة `CancelDraft` (إما ضمن `Service` أو عبر واجهة متوافقة إذا تطلبت سياسة `check_api_compat.sh` عدم كسر التوقيع الحالي مع ترقيتها في الإصدار الرئيسي).
3. **مراجعة عقد إقفال السنة المالية (`YearEndClosingService`):**
   - التحقق من اكتمال `ExecuteYearEndClosingCommand` و`ReopenYearEndClosingCommand` ونتائجهما في `pkg/spi/application/application.go` وتغطية متطلبات التدقيق والموافقة وإعادة الفتح.
4. **حصر واجهات المدارات (`internal/orbitals/*`):**
   - جرد قدرات `consolidation` و`reconciliation` و`reporting` و`subledgers` و`events` وتحديد ما يمتلك `SPI`/`SDK` عاماً مقابل ما يحتاج واجهة عامة جديدة.

### معايير القبول (Exit Criteria)

- نجاح `scripts/check_api_compat.sh` و`scripts/verify_architecture.sh`.
- توثيق العقود المضافة في `pkg/spi/`.

---

## المرحلة 2: إكمال `Application Layer` ومحولات التخزين

**الهدف:** تمرير جميع العمليات المحاسبية الأساسية عبر مسار تطبيق متكامل ومحمي بالصلاحيات والتدقيق والذرية.

### المهام

1. **تنفيذ `GetPeriodByNumber` في المحولات:**
   - تنفيذ `GetPeriodByNumber` في `storage/memory/` و`storage/postgres/` (مع أي استعلام/فهرس SQL لازم في `storage/postgres/migrations/`).
   - تحديث `storage/masterdatacontract/` لاختبار الفترتين `13` و`14` عبر جميع المحولات.
2. **تنفيذ `CancelDraft` عبر طبقة التطبيق:**
   - استدعاء `JournalEntry.Cancel(actorID, reason, cancelledAt)` في `internal/kernal/journal/entry.go` عبر طبقة التطبيق و`pkg/sdk/application.go`.
   - فرض التحقق من `Principal` و`Authorizer`، والتحقق من عدم خلو `ActorID` والسبب، وحفظ الحالة `Cancelled` في `storage/memory/` و`storage/postgres/`، وتسجيل حدث التدقيق.
3. **تنفيذ `YearEndClosingService` المتكامل:**
   - ربط `internal/orbitals/reporting/closing_engine.go` بخدمة التطبيق في `pkg/sdk/` بحيث تصبح `sdk.YearEndClosing(engine)` في `pkg/sdk/engine.go` فعّالة وليست معيدة لـ `ErrCapabilityNotImplemented`.
   - تغطية التسلسل الكامل: فحوص الجاهزية، إنشاء قيود الإقفال، الترحيل الذري عبر `WithinTransaction`، إقفال السنة المالية، إنشاء الأرصدة الافتتاحية عند تمرير `NextFiscalYearID`، ودعم `ReopenYearEndClosing` وفق سياسة التدقيق والموافقات.

### معايير القبول (Exit Criteria)

- مرور اختبارات الوحدة والتكامل لجميع الحالات في `pkg/sdk/` و`storage/memory/` و`storage/postgres/`.
- عمل `sdk.YearEndClosing(engine)` و`CancelDraft` وفترات التسوية 13 و14 فعلياً عبر الـ SDK العام.

---

## المرحلة 3: حسم نطاق المحاسبة المؤسسية والمعايير

**الهدف:** اتخاذ قرارات معمارية واضحة حول النطاق المؤسسي وسد فجوات المعايير والمدارات.

### المهام

1. **قرار معماري لتعدد الدفاتر (`Multi-Ledger`):**
   - إعداد `ADR` جديد في `docs/architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md` يحدد نموذج `BookID` / `LedgerID` في النواة وعقود التخزين ومفاتيح Idempotency والتقارير والتدقيق (رداً على القيد الموثق حالياً في `internal/kernal/ledger/doc.go`).
   - تنفيذ العزل على مستوى النواة والمحولات واختبارات العزل بناءً على الـ `ADR` المعتمد.
2. **خطة استكمال معايير `IFRS / IAS`:**
   - **`modules/ifrs9/module.go`:** جدولة تنفيذ `Derecognition` و`Hedge Accounting` و`Independent Validation` وتوثيق حدود مسؤولية النظام المضيف (مثل تصنيف الأدوات وتقييم `SPPI`).
   - **`modules/ifrs15/module.go`:** استكمال `Principal vs Agent` و`Complex IP Licensing` وتحديد عقود مدخلات النظام المضيف لنقل السيطرة ونسبة الإنجاز.
   - **`modules/ifrs16/module.go`:** استكمال `Sale-and-Leaseback` و`Subleases` و`Variable Lease Payments` و`Index/Rate Reassessments` و`Impairment` و`Termination/Transition` مع استمرارية الحفظ والإفصاحات.
   - **`modules/ias12/module.go`:** استكمال المعالجة الكاملة للضرائب الحالية والخسائر والائتمانات الضريبية والعرض والإفصاح الضريبي.
   - توحيد آلية تمرير إصدار المعيار وتاريخ النفاذ والولاية القضائية من النظام المضيف.
3. **نشر قدرات المدارات (`internal/orbitals/*`):**
   - توفير واجهات `SPI`/`SDK` عامة للقدرات المدارية المعتمدة (خصوصاً `consolidation` و`reconciliation`) مع اختبارات عقد للمستهلك الخارجي.

### معايير القبول (Exit Criteria)

- اعتماد `ADR` تعدد الدفاتر ووضوح حدود كل معيار (`IFRS 9/15/16`, `IAS 12`) في الكود والوثائق.
- وجود واجهات عامة مختبرة للمدارات المستهدفة.

---

## المرحلة 4: إثبات الاستقلال والتكافؤ واتساق الوثائق

**الهدف:** إثبات تطابق السلوك بين محولات التخزين ومنع تضارب المواصفات والوثائق.

### المهام

1. **اختبارات التكافؤ (`Contract Parity`):**
   - توسيع `test/contract_parity/` و`test/external_consumer/` لتغطي `CancelDraft`، فترات التسوية 13 و14، `YearEndClosing`، وحالات الفشل والتزامن عبر `Memory` و`PostgreSQL` ومستهلك خارجي.
2. **مصفوفة التتبع الموحدة وتصحيح الوثائق:**
   - إنشاء مصفوفة تتبع واحدة تربط:
     `Capability → Specification → Public API/SPI → Implementation → Test → Evidence → Status`
   - تحديث الوثائق المتضاربة (مثل ملاحظة غياب `Cancelled` في `docs/specifications/kernel/JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.2.0.md`، وجرد القدرات، ومسارات `internal/orbitals/*`) بحيث لا تُعلن أي قدرة "مكتملة" دون دليل اختبار وعقد عام فعلي.

### معايير القبول (Exit Criteria)

- نجاح 100% لاختبارات `test/contract_parity/` و`test/external_consumer/` و`scripts/verify_kernel_baseline.sh`.
- خلو الوثائق من التضاربات المرجعية.

---

## المرحلة 5: الجاهزية التشغيلية واعتماد الإنتاج

**الهدف:** استبدال حالة `SUPERSEDED` في `docs/reports/KERNEL_RELEASE_SIGNOFF.md` باعتماد جديد مبني على أدلة حية وقابلة لإعادة الإنتاج.

### المهام

1. **التحقق من الأداء تحت الحمل الفعلي (`Load Testing`):**
   - تنفيذ اختبارات حمل منهجية في `test/benchmark/` على `PostgreSQL` بحجم بيانات وتزامن واقعيين (بدلاً من العينة المحدودة في `docs/reports/performance_samples.json` عند ~63.3 TPS) وقياس `P50` و`P95` و`TPS` عند الحمل المستهدف وتوثيق النتائج.
2. **إثبات النسخ الاحتياطي والتعافي (`DR / RPO / RTO`):**
   - تنفيذ تمارين استعادة فعلية في `test/dr/` على بيئة مماثلة للإنتاج، وحسم التناقض بين هدف `< 5s` ومعيار `30s` في دليل التعافي، وتحديث حالة `RPO/RTO` من `Not Verified` في `docs/quality/SCOPE_PROFILE_KERNEL_v1.md`.
3. **تفعيل سجل الاختبارات وإعادة الاعتماد:**
   - استكمال `docs/quality/COMPLIANCE_TEST_SUITE_INDEX.md` بحيث يرتبط كل تشغيل بـ `Run ID` و`Commit` و`Environment` و`Evidence URI` و`Reviewer`.
   - استكمال المراجعة المستقلة واعتماد `Scope Profile`، ثم إصدار وثيقة اعتماد جديدة تحل محل `docs/reports/KERNEL_RELEASE_SIGNOFF.md` (`SUPERSEDED`).

### معايير القبول (Exit Criteria)

- توثيق نتائج الحمل والتعافي في بيئة مماثلة للإنتاج.
- توقيع الاعتماد الجديد وسريان قرار الإصدار (`GO`) المبني على أدلة قابلة للتحقق.
