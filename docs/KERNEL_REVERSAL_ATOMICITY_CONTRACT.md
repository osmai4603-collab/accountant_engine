# عقد ذرية العكس — Reversal Atomicity Contract

- **رمز الوثيقة:** `AE-P7-REVERSAL-CONTRACT`
- **المرحلة:** 7 — المعاملات والذرية ومنع التكرار
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)، المهام 10–11 و16
- **التنفيذ المرجعي:** `storage/postgres/uow.go`، `storage/memory/posting.go`، `storage/postgres/posting.go`
- **الوثائق المرتبطة:** [عقد معاملة الترحيل](./KERNEL_POSTING_TRANSACTION_CONTRACT.md)، [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md)، [مواصفة التدفق](./KERNEL_POSTING_FLOW_SPECIFICATION.md)

---

## 1. الغرض

يمنع هذا العقد الحالات الوسيطة المرئية في العكس المحاسبي: لا يجوز أن يوجد قيد عكسي دون ربط بالأصل، ولا أصل بحالة `reversed` دون قيد عكسي، ولا مفتاح idempotency محفوظ دون أثر العكس، ولا أثر مالي دون تدقيق. النواة تبني العكس وتتحقق منه؛ المحول يضمن أن مجموعة التغييرات وحدة واحدة.

## 2. وحدة عمل العكس

تُنجز العناصر الآتية معاً أو لا يُنجز شيء منها:

| العنصر | القيمة المطلوبة |
|---|---|
| قيد العكس | `Status = posted`، `Classification = reversing`، `ReversalOfID = original.ID`، أسطر بمبالغ معكوسة |
| الأصل | `Status = reversed` و`ReversedByID = reversal.ID` |
| idempotency | سجل بالنطاق `operation=reverse` والبصمة المختومة و`EntryID` للعكس |
| الأثر المالي | تحديث الرصيد/الحركة بما يعكس أثر العكس دون أثر مكرر |
| التدقيق | سليل `AuditRecord` داخل نفس الالتزام |

لا تُكتب هذه العناصر في مراحل منفصلة. أي فشل قبل Commit يُلغيها جميعاً.

## 3. فائز واحد

1. يُقرأ الأصل ويُقفل داخل وحدة العمل (`SELECT ... FOR UPDATE` في PostgreSQL؛ قفل مكافئ في الذاكرة).
2. يُرفض الأصل إن لم يكن `posted` أو كان يحمل `ReversedByID` بالفعل.
3. يُكتب العكس ويُحدَّث الأصل قبل إكمال مفتاح idempotency.
4. أي محاولة ثانية بعكس جديد — حتى بمفتاح مختلف — تُرفض بـ `storage.ErrEntryAlreadyReversed`.
5. إعادة الطلب بالمفتاح والبصمة نفسيهما تُعيد نتيجة العكس الأول؛ لا تُنشئ عكساً ثانياً.
6. المفتاح نفسه بحمولة مختلفة يُرفض بـ `storage.ErrIdempotencyPayloadConflict`.

القيد النهائي: لكل أصل مرحّل عكس واحد على الأكثر، وللعكس أصل واحد، وللحالة والرابط نفس القيد المكتمل.

## 4. السلوك عند الفشل

| لحظة الفشل | الحالة المرصودة بعد الفشل |
|---|---|
| قبل حجز المفتاح | لا قيد عكسي، لا تغيير في الأصل، لا مفتاح، لا رصيد، لا تدقيق |
| قبل التدقيق | كل ما سبق ملغى؛ لا حركة مالية بلا أثر تدقيقي |
| قبل Commit | كل العناصر ملغية؛ الأصل يبقى `posted` و`ReversedByID == nil` |
| بعد Commit | العكس والأصل والرابط والمفتاح والتدقيق دائمة؛lost response لا يلغيها |

الدليل: `TestPhase7_ReversalAtomicity` و`TestPhase7_FaultInjectionAtomicity` في `test/contract_parity/phase7_transaction_contract_test.go`.

## 5. روابط WORM وتسميات JSON

نموذج النواة يستخدم `ReversalOfID` على العكس و`ReversedByID` على الأصل. قواعد WORM/الـ trigger في قاعدة البيانات تتحقق من الأسماء `OriginalEntryID` و`ReversedEntryID` داخل JSON. يحل محول PostgreSQL ذلك صراحةً:

- عند الحفظ: يُضاف `OriginalEntryID` للعكس و`ReversedEntryID` للأصل المعكوس، ويُحذف `ReversedByID` من JSON لتفادي تضارب القواعد.
- عند القراءة: تُترجم الأسماء القديمة إلى حقول نموذج النواة إذا كانت فارغة.
- `reversed_move_id` في الجدول يظل مرآة علائقية للرابط نفسه.

بهذا لا توجد نسختان متعارضتان من الحقيقة؛ الاسم في JSON مجرد تمثيل لنفس المعرف.

## 6. القيود المطلوبة من التخزين

- فهرس/قيد فريد يمنع أكثر من عكس للأصل الواحد.
- قفل الأصل أثناء العكس لمنع سباق فائزين.
- إمكانية إلغاء كل التغييرات عند أي خطأ قبل Commit، بما فيها تحديث الحالة والرابط.
- نفس قواعد idempotency الموضحة في [عقد idempotency](./KERNEL_IDEMPOTENCY_CONTRACT.md)، مع `operation=reverse`.

لا شيء من هذا يفرض SQL داخل `internal/kernal`؛ النواة تمرر `OriginalEntryID` و`ReversalOfID` و`PostingIdempotency` فقط.

## 7. الحكم

يُعد العقد مستوفى عندما يُثبت المحولان أن:

- الفشل قبل Commit لا يغيّر الأصل ولا يترك عكساً.
- العكس المكتمل يحمل رابطاً صحيحاً، والأصل معكوس بالرابط نفسه.
- المحاولة الثانية تُرفض صراحةً بـ `ErrEntryAlreadyReversed`.
- لا توجد حالة وسيطة مرئية بين كتابة العكس وتحديث الأصل.

هذا الحكم محايد عن نوع قاعدة البيانات؛ التفاصيل التنفيذية في [تقرير حقن الأعطال](./reports/fault_injection_report.md) وتقرير المرحلة 7.
