# ADR P11-02: الحساب النافذ والأبعاد التحليلية على السطح العام

## الحالة

معتمد في المرحلة 11 — يغلق `G-019` و`G-022` ويكمل التوصيل المضيف جانب `G-020`.

## السياق

كانت النواة تعرف `Account.IsCurrent` ومستودعات الأبعاد التحليلية داخل `internal/kernal/masterdata`، لكن منافذ `pkg/spi` لم تعرض لقطة حساب مؤرخة ولا قرّاء تكاليف/أبعاد. أي مضيف أراد هذه القدرات كان عليه استيراد `internal/*`، وهو ممنوع بقرار `P7-01`. في المقابل، كانت بعض قراءات `MasterData` تقبل معرّف شركة غريباً دون رفض صريح، ولم يكن متجر السياسات المؤرخ متاحاً للمعاملات عبر `Ports.Private`.

## القرار

1. تُعرَّف واجهات اختيارية جديدة في `pkg/spi/masterdata` فقط: `EffectiveAccountReader.GetAccountAsOf` و`AccountVersionWriter.SaveAccountVersion` للحسابات المؤرخة، و`CostCenterReader`/`CostCenterWriter` و`AnalyticalDimensionReader`/`AnalyticalDimensionWriter` للأبعاد. لا تستورد هذه الحزمة أي `internal/*`.
2. تُعاد تعريفات `CostCenter` و`AnalyticalDimensionValue` وأنواع `AnalyticalDimensionKind` وثوابتها كأسماء عامة عبر `pkg/api/v1/types`. توقيعات SPI تستخدم هذه الأنواع العامة حصراً، وتحويل الأخطاء العامة إلى أخطاء النواة يتم داخل `pkg/sdk`.
3. `SaveAccountVersion` يحفظ لقطة كاملة مؤرخة غير قابلة للتعديل، لا تحديثاً جزئياً. تُطبَّع الحدود إلى أيام UTC: `EffectiveFrom` عند منتصف ليل UTC و`EffectiveTo` عند آخر نانوسكسند من يوم UTC، وكلاهما شامل. هذا يجعل النافذة متوافقة مباشرة مع `Account.IsCurrent`.
4. تُرفض النوافذ المعكوسة، والتداخل مع إصدار موجود، والكتابة لشركة غريبة. أحدث لقطة تُحدّث صف الحساب الأساسي لتبقى القراءة الحالية متسقة؛ الإصدارات الأقدم تبقى مجمدة.
5. يربط كل محول «شركة أساسية» عند أول `CreateCompany` ناجح. سياق `storage.WithCompanyContext` يختار مستأجراً آخر صراحةً؛ بدون هذا السياق تُرفض قراءة شركة غريبة بالخطأ الحالي المعروف لكل منفذ. لا تغيّر دلالة `GetAccount` و`GetFiscalYear`.
6. تُخزَّن النسخ في PostgreSQL داخل `account_versions` بعد إضافة أعمدة لقطة كاملة عبر migration جديدة، وتُقرأ بشرط `effective_from <= asOf AND (effective_to IS NULL OR effective_to >= asOf)`. تُستخدم `WithCompanyTx` لكل قراءة/كتابة.
7. تُخزَّن مراكز التكلفة في `cost_centers` والقيم التحليلية في `financial_dimension_values` داخل `WithCompanyTx`، مع رفض صريح لأي معرّف ينتمي إلى شركة أخرى.
8. يكتشف SDK القدرات الاختيارية عبر type assertions على `ports.MasterData` ويبني قارئاً مغلّفاً فقط عند وجود إحداها. غياب قارئ الأبعاد يبقي خطأ `posting.ErrDimensionReaderRequired` حياً، وغياب قارئ الحساب النافذ يرجع إلى `GetAccount` مع بقاء فحص `IsCurrent`. لا توجد طرق stub دائمة في النوع الأساسي.
9. يُعرَّض `storage.PolicyStore` عبر `Ports.Private[PolicyStoreExtensionKey]` كقيمة واعية للمعاملة: `t` في Memory و`driver` في PostgreSQL. تنفيذ Memory ينسخ حالة الإصدارات كاملة في `newDriver`/`clone`/`copyFrom` ويطبّق نفس قواعد التداخل والترتيب والحماية الخاصة بـ PostgreSQL.

## البدائل المرفوضة

- إضافة طرق إلزامية إلى `storage.MasterDataReader`: تغيير كاسر للعقد العام ويجعل المحولات التي لا تدعم الأبعاد تفشل عند الإنشاء.
- استيراد أنواع `internal/kernal/masterdata` مباشرة في SPI: خرق لـ `P7-01` وربط السطح العام بتفاصيل النواة.
- حفظ الحقول المتغيرة فقط في `account_versions`: يعيد تفسير سجل قديم عند تعديل الحساب ويخالف مبدأ اللقطة غير القابلة للتعديل.
- اعتبار حدود النافذة نصف مفتوحة `[from, to)`: لا يطابق `Account.IsCurrent` ويجعل آخر يوم من الصلاحية غير صالح.
- إخفاء غياب قارئ الأبعاد خلف طريقة stub: يمنع fail-closed ويسمح بقبول أبعاد لم تُتحقق من شركة القيد.

## الأدلة

- `test/contract_parity/phase11_g019_g022_contract_test.go` يغطي نوافذ الحساب النافذة، التداخل، حماية الشركات، جولات الأبعاد، واكتشاف قدرات SDK وفشلها المغلق على المحولين.
- `TestPhase11_CompanyScopeOnEveryMasterDataPort` يغطي منافذ MasterData الحالية على Memory وPostgreSQL.
- `./scripts/verify_architecture.sh` يمنع استيراد `internal/*` من السطح العام.
