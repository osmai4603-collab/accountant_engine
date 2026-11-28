# تحليل التضاربات داخل مجلد الوثائق (`DOCS_CONFLICT_ANALYSIS`)

> **الحالة:** وثيقة تحليلية مرجعية (لا تمنح قرار اعتماد)
> **التاريخ:** `2026-11-28`
> **النطاق:** `docs/` (المواصفات، العقود، التقارير، الجودة، قواعد العمل، المعمارية)

---

## 1. الملخص التنفيذي

رصدت المراجعة الشاملة لوثائق المشروع التضاربات التالية بين ملفات الاعتماد والمواصفات والعقود، وتم توثيق حالة إغلاقها ضمن خطة التنفيذ [`APPROVAL_EXECUTION_PLAN.md`](../APPROVAL_EXECUTION_PLAN.md):

| المعرف | نوع التضارب | الموضع الأول | الموضع الثاني | الحكم والحل | الحالة |
| --- | --- | --- | --- | --- | :---: |
| **C-01** | تضارب قرار الاعتماد | `reports/KERNEL_RELEASE_SIGNOFF.md` (`GO`) | `reports/KERNEL_RELEASE_DECISION_2026-11-27.md` (`NO-GO`) | إلغاء `KERNEL_RELEASE_SIGNOFF.md` وتوسيمها `SUPERSEDED`؛ ادعاءات Multi-Ledger وIFRS غير مُثبتة وخارج نطاق `SCOPE_PROFILE_KERNEL_v1.md` §8 | ✅ مغلق |
| **C-02** | تكرار مواصفة النقود | `specifications/kernel/MONEY_DOMAIN_SPECIFICATION_v2.6.1.md` | `MONEY_DOMAIN_SPECIFICATION_v2.6.1.ar.md` | حذف النسخة المكررة `.ar.md` والإبقاء على ملف واحد مرجعي | ✅ مغلق |
| **C-03** | عدم تطابق أسماء ملفات المواصفات | أسماء ملفات `_v1.0.0.md` | الإصدار الداخلي `1.1.0`/`1.2.0` | جميع المواصفات الثماني مسماة حالياً بـ `_v1.2.0.md` | ✅ مغلق |
| **C-04** | تضارب دقة الطابع الزمني للتدقيق | `GOVERNANCE_AND_AUDIT` (`GOV-003`: ميكروثانية + v2 length-prefixed) | `rules/ACCOUNTING_BUSINESS_RULES.md` (`RULE-ACC-041/042`: `UnixNano()` + `v1\|...`) | تحديث `RULE-ACC-041/042` لتطابق `GOV-003` (ميكروثانية + v2 length-prefixed) | 🔄 المرحلة 2 |
| **C-05** | تضارب الأخطاء الحارسة | `ErrAccountNotEffective`, `ErrIdempotencyKeyRequired`, `Regular`, `Revenue` | `ErrAccountNotCurrent`, `ErrIdempotencyScopeRequired`, `Standard`, `Income` | توحيد الأخطاء الحارسة والمصطلحات عبر المواصفات لتطابق الكود الفعلي | 🔄 المرحلة 2 |
| **C-06** | فجوات `IAS 21` و`IsMonetary` | `MASTERDATA` (`MD-004`), `FX_REVALUATION` (`VAL-006`) | `TARGET_ACCOUNTING_KERNEL.md`, `RULE-ACC-013` | توثيق `IsMonetary *bool`، قصر النواة على الفروق غير المحققة، واستبعاد `IAS 21.23(c)` خارج النواة | 🔄 المرحلة 2 |
| **C-07** | تسرب مصطلحات Go في المتن المعياري | `IDENTIFIERS_AND_ERRORS` (`int64`), `FX_REVALUATION` (`storage.OperationRevaluation`), `POSTING/MASTERDATA` (`nil`) | منهجية `NSM-001 §2(1), §13` | استبدالها بتعبيرات محايدة لغوياً | 🔄 المرحلة 2 |
| **C-08** | سجل الأدلة وبصمة الشجرة | `COMPLIANCE_TEST_SUITE_INDEX.md` §5 | `KERNEL_RELEASE_DECISION_2026-11-27.md` | تجميد الشجرة، تنفيذ الأوامر العشرة، وملء §5 وتحديث `MANIFEST.sha256` | 🔄 المرحلة 3–4 |

---

## 2. مراجع ذات صلة

- [`DOCS_COMPLETE_ANALYSIS.md`](DOCS_COMPLETE_ANALYSIS.md) — الجرد الوظيفي الكامل لملفات `docs/`
- [`KERNAL_VS_DOCS_COMPARISON.md`](KERNAL_VS_DOCS_COMPARISON.md) — مقارنة الكود الفعلي في `internal/kernal/` مع التوثيق
- [`../reports/KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md`](../reports/KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md) — تفاصيل الملاحظات الـ27
- [`../APPROVAL_EXECUTION_PLAN.md`](../APPROVAL_EXECUTION_PLAN.md) — خطة التنفيذ الشاملة
