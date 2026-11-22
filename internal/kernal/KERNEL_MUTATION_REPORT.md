# تقرير اختبار الطفرات الموجه — Targeted Mutation Testing Report

- **رمز التقرير:** `AE-REP-KERNEL-MUTATION-V12`
- **الإصدار المستهدف:** Kernel v12 Final Acceptance
- **التاريخ:** 2026-11-21
- **المنهجية:** اختبار الطفرات الدلالية الموجهة (Targeted Semantic Business Rule Mutation Testing) على القواعد المحاسبية الحرجة.
- **الحالة:** PASS (نسبة قتل الطفرات الحرجة: 100.0%)

---

## 1. ملخص النتائج

تم إخضاع القواعد المحاسبية الحرجة في حزم `types`, `posting`, `masterdata`, `governance` لثماني طفرات كودية دلالية (Mutants) تعطل عمداً صمامات الأمان والتحقق، للتأكد من قدرة جناح الاختبارات على كشفها وإسقاطها فوراً.

| رقم الطفرة | القاعدة المحاسبية المستهدفة | الملف المستهدف | السلوك المغيّر عمداً | نتيجة الاختبار |
| --- | --- | --- | --- | :---: |
| **MUT-01** | `INV-004` (Currency Uniformity) | `types/money.go` | تعطيل فحص تطابق العملات في الجمع | **KILLED** |
| **MUT-02** | `INV-002` (Precision Boundaries) | `types/money.go` | السماح بأكثر من 28 رقماً صحيحاً في `Money` | **KILLED** |
| **MUT-03** | `INV-008` (Double-Entry Balance) | `posting/entry_validation.go` | تعطيل فحص توازن المدين والدائن في القيد | **KILLED** |
| **MUT-04** | `INV-017` (Calendar SoftClose) | `masterdata/fiscal_calendar.go` | السماح بالترحيل في فترة SoftClosed بدون إذن السياسة | **KILLED** |
| **MUT-05** | `INV-025` (Audit Snapshot Sequence) | `governance/audit_chain.go` | تعطيل مطابقة تسلسل اللقطة مع Checkpoint | **KILLED** |
| **MUT-06** | `INV-012` (Entry Reversal Identity) | `posting/entry_validation.go` | السماح للقيد العكسي بعكس نفسه (Self-Reversal) | **KILLED** |
| **MUT-07** | `INV-024` (Audit Tamper Detection) | `governance/audit_chain.go` | تعطيل إعادة حساب الهاش أثناء التحقق من اللقطة | **KILLED** |
| **MUT-08** | `INV-016` (Period Date Containment) | `masterdata/fiscal_calendar.go` | تعطيل فحص احتواء تاريخ العملية داخل الفترة | **KILLED** |

---

## 2. النتيجة الإجمالية

- **إجمالي الطفرات الحرجة:** 8
- **الطفرات التي تم قتلها (Killed):** 8
- **الطفرات الناجية (Survived):** 0
- **نسبة قتل الطفرات (Mutation Score):** **100.0%** (متجاوزة المستهدف المحدد في الخطة ≥85%).
