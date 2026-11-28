# مقارنة النواة الفعلية مع التوثيق (`KERNAL_VS_DOCS_COMPARISON`)

> **الحالة:** وثيقة تحليلية مرجعية
> **التاريخ:** `2026-11-28`
> **نطاق الفحص:** `internal/kernal/` مقابل `docs/specifications/kernel/` و`docs/rules/` و`docs/contracts/`

---

## 1. نتيجة الفحص المباشر للكود

أثبت الفحص المباشر للشيفرة المصدرية في `internal/kernal/` أن جميع القواعد المحاسبية الحتمية (**27/27** في `RULE-KERNEL-001..027` و**13/13** في `AC-MAND-01..13`) منفّذة ومختبرة فعلياً:

| القاعدة / الميزة | موقع التنفيذ في `internal/kernal/` | الحالة في الكود | الحالة في التوثيق قبل التحديث |
| --- | --- | :---: | --- |
| القيد المزدوج وتوازن العملة الوظيفية | `posting/entry_validation.go` (`ValidateEntryDoubleEntry`) | ✅ | ✅ متطابق |
| حظر `float32`/`float64` | `shopspring/decimal` + فحص AST | ✅ | ✅ متطابق |
| تسوية التقريب بتسامح `0.5` وحدة صغرى | `posting/rounding.go` (`RoundEntryWithRoundingPolicy`) | ✅ | ⚠️ كانت غير محددة القيمة في `PST-004` |
| آلة حالة القيد وإلغاء المسودة | `journal/entry.go` (`Draft→Posted→Reversed`, `Draft→Cancelled`) | ✅ | ⚠️ بعض الإحالات القديمة كانت تذكر `Standard` بدل `Regular` |
| سمة `IsMonetary *bool` وإعادة التقييم `CLOSING` | `masterdata/`, `valuation/revaluation.go` | ✅ | ⚠️ كانت غائبة من `MD-004` في `MASTERDATA` |
| سلسلة التدقيق v2 (12 حقلاً، ميكروثانية، length-prefixed) | `governance/audit_chain.go` (`CalculateHash`) | ✅ | ⚠️ `RULE-ACC-041/042` كانت تصف v1 و`UnixNano()` |
| عزل الشركات | `masterdata/`, `posting/pipeline.go` | ✅ | ✅ متطابق |

---

## 2. الخلاصة

- **لا توجد فجوات تنفيذية مفتوحة في الكود المحاسبي** (`internal/kernal/`).
- جميع الفجوات الـ27 المسجلة في [`KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md`](../reports/KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md) هي فجوات وثائقية/مواصفاتية تُعالج بتحديث المواصفات والوثائق في المرحلتين 1 و2 من [`APPROVAL_EXECUTION_PLAN.md`](../APPROVAL_EXECUTION_PLAN.md).

