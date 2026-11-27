# سجل تغييرات النواة المحاسبية — Kernel v12 Changelog

> **Superseded (متجاوز):** هذا السجل تاريخي للإصدار v12 فقط؛ لا يُستخدم لتحديد الحالة المعمارية الحالية أو مخرجات المرحلة 1. المرجع الحالي: [سجل فجوات المعالجة](../../docs/plans/KERNEL_REMEDIATION_GAP_REGISTER.md) و[تقرير المرحلة 1](../../docs/reports/KERNEL_FINAL_VERIFICATION_REPORT.md).

- **رمز الوثيقة:** `AE-LOG-KERNEL-V12-CHANGELOG`
- **خط الأساس:** Kernel v11 (`accountant_engine11.zip` / Commit `48ad0be1d4964da7c9b2eaa065ba1248a52b16fd`)
- **الإصدار الناتج:** Kernel v12 Final Stabilization & Acceptance
- **تاريخ الإصدار:** 2026-11-21
- **النطاق البرمجي:** `internal/kernal/` حصراً

---

## 1. التغييرات والتحسينات في الإصدار v12

### 1.1 الحوكمة وسلسلة التدقيق (`governance`)

- إضافة اختبارات موجهة دلالية `TestCalculateHashGoldenVector` للتأكد من حتمية تشفير الهاش الكنسي وتغيره الحتمي عند تغيير أي بايت في الحمولة أو الترويسة.
- إضافة اختبار `TestVerifyAuditSnapshotSequenceMismatchKillsMutant` لإثبات كشف أي انحراف في تسلسل لقطة التدقيق بالنسبة لنقطة التحقق (Checkpoint).

### 1.2 البيانات المرجعية والتقويم المالي (`masterdata`)

- إضافة اختبارات حدودية موسعة لـ `ValidateFiscalYear` تمنع قبول فترات يكون تاريخ نهايتها قبل تاريخ بدايتها.
- إضافة اختبار `TestValidatePeriodForPostingRejectsDateOutsideYear` للتحقق من رفض أي ترحيل يقع تاريخه خارج حدود السنة المالية المعتمدة.

### 1.3 التوثيق ومصفوفة التتبع

- ترقية جميع وثائق النواة إلى إصدار **v12**:
  - `KERNEL_COMPLETION_REPORT.md` (تحديث الإصدار والنتائج الفعلية والبيانات المعتمدة).
  - `KERNEL_TRACEABILITY_MATRIX.md` (تحديث الرمز والإصدار، إضافة 4 اختبارات جديدة لتصل المراجع إلى **66**).
  - `KERNEL_RELEASE_BASELINE.md` (توثيق خط أساس v11 ومخرجات v12).
  - إنشاء `KERNEL_COVERAGE_EXCEPTIONS.md` لتوثيق تحليل الفروع المستثناة تقنياً.
  - إنشاء `KERNEL_MUTATION_REPORT.md` لتوثيق نتائج اختبارات الطفرات الموجهة (100% Killed).
  - إنشاء `KERNEL_V12_CHANGELOG.md` لتوثيق تاريخ التغييرات.

### 1.4 حدود النطاق المعماري

- تأكيد عدم تعديل أي ملف خارج `internal/kernal/`.
- خلو التغييرات من أي انتهاك للاستقلال المعماري أو استخدام `float` أو استيراد حزم المضيف.
