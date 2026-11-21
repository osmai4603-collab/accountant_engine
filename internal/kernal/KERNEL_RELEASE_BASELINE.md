# خط الأساس المعتمد للنواة المحاسبية — Kernel Release Baseline v10

- **تاريخ الاعتماد:** 2026-11-21
- **رمز الوثيقة:** `AE-BASELINE-KERNEL-V10`
- **النطاق:** `internal/kernal/` حصراً

---

## 1. جرد ملفات النواة وتصنيفها (خط أساس v9 قبل تعديلات v10)

### أ. ملفات المصدر (Source Code) — 24 ملفاً

1. `internal/kernal/apperrors/errors.go`
2. `internal/kernal/governance/audit_chain.go`
3. `internal/kernal/governance/verifier.go`
4. `internal/kernal/journal/entry.go`
5. `internal/kernal/ledger/account.go`
6. `internal/kernal/ledger/period.go`
7. `internal/kernal/masterdata/account.go`
8. `internal/kernal/masterdata/coa.go`
9. `internal/kernal/masterdata/company_scope.go`
10. `internal/kernal/masterdata/cost_center.go`
11. `internal/kernal/masterdata/dimension.go`
12. `internal/kernal/masterdata/fiscal_calendar.go`
13. `internal/kernal/masterdata/journal.go`
14. `internal/kernal/masterdata/ports.go`
15. `internal/kernal/masterdata/types.go`
16. `internal/kernal/policy/eligibility.go`
17. `internal/kernal/policy/posting_rules.go`
18. `internal/kernal/posting/adjusting.go`
19. `internal/kernal/posting/entry_validation.go`
20. `internal/kernal/posting/lock_manager.go`
21. `internal/kernal/posting/pipeline.go`
22. `internal/kernal/posting/reversal.go`
23. `internal/kernal/posting/rounding.go`
24. `internal/kernal/types/currency.go`
25. `internal/kernal/types/errors.go`
26. `internal/kernal/types/identifiers.go`
27. `internal/kernal/types/money.go`
28. `internal/kernal/types/rounding.go`
29. `internal/kernal/valuation/conversion.go`
30. `internal/kernal/valuation/exchange_rate.go`
31. `internal/kernal/valuation/revaluation.go`

### ب. ملفات التوثيق (Documentation) — 9 ملفات

1. `internal/kernal/doc.go`
2. `internal/kernal/governance/doc.go`
3. `internal/kernal/journal/doc.go`
4. `internal/kernal/ledger/doc.go`
5. `internal/kernal/masterdata/doc.go`
6. `internal/kernal/policy/doc.go`
7. `internal/kernal/posting/doc.go`
8. `internal/kernal/types/doc.go`
9. `internal/kernal/valuation/doc.go`

### ج. ملفات الاختبار (Tests) — 13 ملفاً

1. `internal/kernal/governance/audit_snapshot_test.go`
2. `internal/kernal/governance/audit_test.go`
3. `internal/kernal/journal/entry_test.go`
4. `internal/kernal/masterdata/account_validity_test.go`
5. `internal/kernal/masterdata/fiscal_calendar_test.go`
6. `internal/kernal/masterdata/journal_test.go`
7. `internal/kernal/policy/policy_test.go`
8. `internal/kernal/posting/entry_invariants_test.go`
9. `internal/kernal/posting/lock_test.go`
10. `internal/kernal/posting/pipeline_invariants_test.go`
11. `internal/kernal/posting/rounding_precision_test.go`
12. `internal/kernal/posting/rounding_test.go`
13. `internal/kernal/types/money_fuzz_test.go`
14. `internal/kernal/types/money_test.go`
15. `internal/kernal/types/rounding_test.go`
16. `internal/kernal/valuation/ias21_test.go`

### د. ملفات بيانات الاختبار (Testdata)

- `internal/kernal/posting/testdata/fuzz/FuzzReversalNetImpactIsZeroPerAccount/68e9ff99a56b7025`

---

## 2. مقارنة v8 مقابل v9 — الملفات المفقودة المطلوب مراجعتها واستعادتها

بمقارنة ملفات النواة في `accountant_engine8.zip` مع خط الأساس الحالي، تم حصر 10 ملفات غابت عن v9:

1. `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` — وثيقة القواعد الـ 27 الحاكمة للنواة (P0).
2. `internal/kernal/doc_test.go` — اختبار أمثلة التوثيق.
3. `internal/kernal/ledger/ledger_test.go` — اختبار تكامل نماذج الأستاذ العام.
4. `internal/kernal/governance/verification_test.go` — اختبارات التحقق من سجل التدقيق.
5. `internal/kernal/apperrors/errors_test.go` — اختبارات تصنيف الأخطاء ونوعيتها.
6. `internal/kernal/types/identifiers_test.go` — اختبارات صحة المعرفات (UUIDs) والمقارنة.
7. `internal/kernal/masterdata/fiscal_calendar_fuzz_test.go` — اختبار Fuzz لاتصال وعدم تداخل الفترات المالية.
8. `internal/kernal/masterdata/validation_test.go` — اختبارات شاملة للتحقق من بيانات الدليل ودورات الحسابات.
9. `internal/kernal/posting/property_adversarial_test.go` — اختبارات عدائية لخصائص القيود والترحيل.
10. `internal/kernal/posting/posting_fuzz_test.go` — اختبار Fuzz لتوازن القيود.

**سياسة المعالجة:**

- لا استعادة عمياء أو آلية فورية.
- استعادة وثيقة الثوابت `KERNEL_ACCOUNTING_INVARIANTS.md` في المرحلة 1 بعد تدقيق نصوصها مقابل الكود.
- استعادة ملفات الاختبار تباعاً في المراحل 2، 7، 9، 10، 11 مع التأكد من مطابقتها للعقود وتحديث ما يلزم وفق القرارات المعتمدة.

---

## 3. تدقيق الاعتماديات والاستيراد (Import Audit)

تم فحص جميع ملفات النواة، والتأكد من التالي:

- صفر استيراد خارجي من حزم `pkg/`، `storage/`، `orbitals/`، `modules/`.
- صفر استيراد لطبقة قواعد البيانات `database/sql`.
- صفر استيراد لبروتوكولات النقل والشبكة `net/http`، `net`، `google.golang.org/grpc`.
- عدم تعديل ملفات `go.mod` أو `go.sum` مطلقاً.
- النواة معزولة تماماً وتعتمد فقط على المكتبة القياسية ومكتبتي `shopspring/decimal` و`google/uuid`.
