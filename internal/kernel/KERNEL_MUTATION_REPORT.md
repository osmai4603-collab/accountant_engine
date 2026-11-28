# تقرير اختبار الطفرات الموجه — Targeted Mutation Testing Report

> **حالة التقرير:** سجل اختبار الطفرات الدلالية الموجهة (`scripts/run_mutation_tests.sh`) — تم توسيعه في 2026-11-27 من 8 إلى **11 طفرة حرجة (11/11 Killed — 100.0%)** بإضافة `MUT-09` (`PST-008`) و`MUT-10` (`MONEY-025`) و`MUT-11` (`MD-010`). المرجع التاريخي السابق كان (8/8 Killed). راجع [مقارنة kernel بـ docs](../../docs/analysis/KERNEL_VS_DOCS_COMPARISON.md).

- **رمز التقرير:** `AE-REP-KERNEL-MUTATION-V12`
- **الإصدار المستهدف:** Kernel Remediation Verification (Phase 12+)
- **تاريخ التحديث:** 2026-11-27
- **المنهجية:** اختبار الطفرات الدلالية الموجهة (Targeted Semantic Business Rule Mutation Testing) عبر سكربت تشغيل آلي قابل لإعادة الإنتاج (`scripts/run_mutation_tests.sh`).
- **الحالة:** PASS (نسبة قتل الطفرات الحرجة: 100.0% — 11/11 Killed، 0 Survived)
- **أمر إعادة التشغيل المعتمد:** `./scripts/run_mutation_tests.sh /tmp/mutation_run.log`

---

## 1. ملخص النتائج

تم إخضاع القواعد المحاسبية الحرجة في حزم `types`, `posting`, `masterdata`, `governance`, `valuation` لإحدى عشرة طفرة كودية دلالية (Mutants) تعطل عمداً صمامات الأمان والتحقق، والتأكد آلياً من سقوطها وفشلها في اختبارات النواة المقابلة.

| رقم الطفرة | القاعدة المحاسبية المستهدفة | الملف المستهدف | السلوك المغيّر عمداً | نتيجة الاختبار | آلية التحقق وسجل الفشل |
| --- | --- | --- | --- | :---: | --- |
| **MUT-01** | `RULE-KERNEL-004` (Currency Uniformity) | `types/money.go` | تعطيل فحص تطابق العملات في الجمع والطرح | **KILLED** | `TestMoneyOperationsPreserveCurrency` (exit code 1) |
| **MUT-02** | `RULE-KERNEL-002` (Precision Boundaries) | `types/money.go` | السماح بأكثر من 28 رقماً صحيحاً في `Money` | **KILLED** | `TestMoneyRejectsIntegerDigitOverflow` (exit code 1) |
| **MUT-03** | `RULE-KERNEL-008` (Double-Entry Balance) | `posting/entry_validation.go` | تعطيل فحص توازن المدين والدائن في القيد | **KILLED** | `TestDoubleEntryMathematicalValidation` (exit code 1) |
| **MUT-04** | `RULE-KERNEL-017` (Calendar SoftClose) | `masterdata/fiscal_calendar.go` | السماح بالترحيل في فترة SoftClosed بدون إذن السياسة | **KILLED** | `TestPostingPeriodRequiresFiscalYearAndExplicitSoftClosePolicy` (exit code 1) |
| **MUT-05** | `RULE-KERNEL-025` (Audit Snapshot Sequence) | `governance/audit_chain.go` | تعطيل مطابقة تسلسل اللقطة مع Checkpoint | **KILLED** | `TestVerifyAuditSnapshotSequenceMismatchKillsMutant` (exit code 1) |
| **MUT-06** | `RULE-KERNEL-012` (Entry Reversal Identity) | `posting/entry_validation.go` | السماح للقيد العكسي بعكس نفسه (Self-Reversal) | **KILLED** | `TestReversalClassificationAndReferenceMustMatch` (exit code 1) |
| **MUT-07** | `RULE-KERNEL-024` (Audit Tamper Detection) | `governance/audit_chain.go` | تعطيل إعادة حساب الهاش أثناء التحقق من اللقطة | **KILLED** | `TestAuditSnapshotVerificationDetectsTampering` (exit code 1) |
| **MUT-08** | `RULE-KERNEL-016` (Period Date Containment) | `masterdata/fiscal_calendar.go` | تعطيل فحص احتواء تاريخ العملية داخل الفترة | **KILLED** | `TestMasterDataEdgeCases` (exit code 1) |
| **MUT-09** | `PST-008` / `RULE-ACC-026` (No Re-Reversal) | `posting/reversal.go` | السماح لـ `BuildReversalEntryAt` بعكس قيد عكسي | **KILLED** | `TestReversalLifecycle` (exit code 1) |
| **MUT-10** | `MONEY-025` (FX Rate Precision Ceiling) | `types/money.go` | تجاوز حد الـ 12 خانة عشرية في أسعار الصرف | **KILLED** | `TestExchangeRateValidationEnforcesMoney025Money026AndSpecialCurrencies` (exit code 1) |
| **MUT-11** | `MD-010` (Ancestor Cost Center Active) | `posting/pipeline.go` | تعطيل فحص نشاط عقد آباء مركز التكلفة | **KILLED** | `TestPostingRejectsInactiveAncestorCostCenter` (exit code 1) |

---

## 2. النتيجة الإجمالية والأدلة القابلة لإعادة الإنتاج

- **إجمالي الطفرات الحرجة:** 11
- **الطفرات التي تم قتلها (Killed):** 11
- **الطفرات الناجية (Survived):** 0
- **نسبة قتل الطفرات (Mutation Score):** **100.0%** (متجاوزة المستهدف المحدد في الخطة ≥85%).
- **الدليل التنفيذي:** تم توفير السكربت `scripts/run_mutation_tests.sh` لضمان استقلالية الفحص وخلوه من الاعتماد على النتائج المحفوظة يدوياً؛ يتم حفظ السجلات التفصيلية لكل عملية حقن طفرة واستعادتها في `/tmp/mutation_run.log`.
- **إغلاق الفجوة المرتبطة:** تم إغلاق الفجوة `G-005` بالكامل وتوسيعها في موجات التحصين `1–5` لتغطي `PST-008` و`MONEY-025` و`MD-010`.
