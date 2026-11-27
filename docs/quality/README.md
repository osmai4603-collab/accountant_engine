# Accounting Kernel Quality Documentation — v2.0.0

**Status:** Review candidate; not yet institutionally approved.

## Documents

1. [Accounting Kernel Quality Standard](ACCOUNTING_KERNEL_QUALITY_STANDARD.md) — normative accounting, domain and architecture criteria.
2. [Quality Gates Specification](QUALITY_GATES_SPECIFICATION.md) — G01–G11 pass/fail criteria.
3. [Evidence & Scoring Guide](EVIDENCE_AND_SCORING_GUIDE.md) — E0–E4, scoring, N/A, Not Verified, Waived and governance.
4. [Scope Profile Template](SCOPE_PROFILE_TEMPLATE.md) — fillable scope and target-environment record for each assessment.
5. [Compliance Test Suite Index](COMPLIANCE_TEST_SUITE_INDEX.md) — requirement/test traceability and run register.
6. [Documentation Architecture Specification](DOCUMENTATION_ARCHITECTURE_SPECIFICATION.md) — ownership, dependencies, versioning and validation rules.

## Source and disposition

Generated from `ACCOUNTING_KERNEL_QUALITY_STANDARD_v2.0.0.md` (23 November 2026). The source is retained separately and unchanged. The standard sections were distributed by responsibility; the Scope Profile is a fillable template and the Test Suite Index is an index, not proof that tests exist or have passed.

## Release checks

- Version: 2.0.0
- Normative status: review candidate
- Project compliance status: not assessed by this documentation build

## سجل التغييرات وقرار الإصدار من المصدر

## سجل التغييرات (Change Log)

| الإصدار | التاريخ | التغييرات | الحالة |
| --- | --- | --- | --- |
| 1.0.0 | 23 نوفمبر 2026 | النسخة الأساسية ذات المحاور الاثني عشر والأوزان والبوابات الأولية | سابقة |
| 1.1.0 | 23 نوفمبر 2026 | إضافة النطاق، كتالوج الثوابت، دليل الحسابات، الفترات، ضمانات التخزين، نموذج المال والعملات، العزل، الاختبارات، الأداء، سياسة الدرجات والأدلة والحوكمة | مرجع أساس |
| 2.0.0 | 23 نوفمبر 2026 | إضافة ACC-013؛ التجميع الهرمي للدرجات؛ قواعد Optional وN/A؛ تعريف Target Environment لـ E4؛ تشديد Source Traceability؛ جدول القدرات المجاورة؛ Quick وFull Checklists؛ ترقيم 5.13؛ RACI؛ عتبات G01–G11؛ Anti-Patterns؛ عقود Outbox وIdempotency؛ ضوابط Materialized Balances؛ Requirement Traceability Matrix | مسودة مرشحة للمراجعة والاعتماد |

---

## سجل قرار الإصدار 2.0.0

تم الحفاظ على المحاور الاثني عشر وأوزانها الأصلية (100%)، مع إدراج المتطلبات الجديدة داخل المحاور المناسبة منعاً لازدواج الاحتساب. تم تحويل خطة التغييرات CHG-001 إلى CHG-013 إلى نص معياري في مواضعها، مع استكمال مصفوفة التتبع وشروط البوابات.

**حالة الاعتماد:** هذه النسخة مكتملة الدمج تحريرياً، لكنها لا تصبح معياراً مؤسسياً نافذاً إلا بعد المراجعة المحاسبية والمعمارية والمنهجية المستقلة، وموافقة Standard Owner وApproval Authority وفق RACI. ولا تمثل هذه الوثيقة تقييماً أو شهادة اجتياز لمشروع Accountant Engine.

## سجل تغييرات الإصدار 2.0.0

| Change ID | التغيير المدمج |
| --- | --- |
| CHG-001 | ACC-013 Functional Currency Balance |
| CHG-002 | Hierarchical Scoring وWeakest-Link للبنود الحرجة |
| CHG-003 | Product Capability مقابل Deployment Configuration وقواعد Optional/N/A |
| CHG-004 | Source Traceability وسياسة السبب البديل |
| CHG-005 | Target Environment ومتطلبات E4 |
| CHG-006 | جدول القدرات المحاسبية المجاورة وحدود النواة |
| CHG-007 | Quick Review Checklist وFull Compliance Checklist |
| CHG-008 | إعادة ترقيم قسم سلامة الزمن إلى 5.13 وتوضيح المعادلة المحاسبية |
| CHG-009 | RACI للاعتماد |
| CHG-010 | شروط Pass القابلة للقياس للبوابات G01–G11 |
| CHG-011 | Anti-Patterns Catalog وتصحيح التصنيفات الخاطئة |
| CHG-012 | عقود Idempotency وTransactional Outbox |
| CHG-013 | Materialized Balances ومصدر الحقيقة والمصالحة |

## فحوص اتساق التحرير

- مجموع أوزان المحاور الأصلية ما زال 100%.
- أضيف ACC-013 بعد ACC-012 دون إعادة استخدام معرف.
- لا يشترط المعيار ConnectionID لمفتاح Idempotency.
- يصف المعيار Transactional Outbox بوصفه كتابة ذرية في المعاملة المحلية نفسها.
- لا يصنف Materialized Balance غير المتزامن خطأً بذاته.
- حالات N/A وNot Verified وFailed وWaived منفصلة.
- لا يحول Waiver إلى Pass.
- لا يعني تحديث المعيار اجتياز المشروع له.
