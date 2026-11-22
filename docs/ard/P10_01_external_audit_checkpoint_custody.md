# ADR P10-01 — حجز سلسلة التدقيق ونقطة التفتيش الخارجية

- **الحالة:** مقبول (Accepted)
- **التاريخ (UTC):** 2026-11-22
- **المالك:** مالك `internal/kernal/governance` + مالكو `storage/memory` و`storage/postgres` + مالك العمليات
- **المرجع:** [الخطة، المرحلة 10](../KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [الفجوة G-013](../KERNEL_REMEDIATION_GAP_REGISTER.md)، [الفجوة G-025](../KERNEL_REMEDIATION_GAP_REGISTER.md)
- **ذو صلة:** [نموذج تهديد التدقيق](../KERNEL_AUDIT_THREAT_MODEL.md)، [عقد مستودع التدقيق](../KERNEL_AUDIT_REPOSITORY_CONTRACT.md)، [تقرير المرحلة 10](../KERNEL_REMEDIATION_PHASE10_REPORT.md)، [ADR P7-01](./P7_01_public_surface_decision.md)

## السياق

1. `CalculateHash` يربط كل سجل بسابقه، لكن حذف الذيل (أحدث السجلات) يترك الباقي سلسلة صحيحة؛ لا يمكن لسلسلة التجزئة وحدها أن تثبت أن ذيلاً كان موجوداً.
2. كان التحقق يعتمد على مستودعات وهمية في اختبارات النواة، ولم يثبت سلوك الحذف والفشل والاستعادة مع Adapter فعلي؛ هذه الفجوة `G-013`.
3. لم يكن الهاش النووي موصولاً بمحولات تخزين فعلية، ولا توجد نقطة تفتيش منفصلة عن السجل ولا لقطة واحدة تربطهما؛ هذه الفجوة `G-025` ذات الأولوية Critical.
4. النواة لا تملك قاعدة بيانات ولا مزوداً خارجياً؛ أي ادعاء بحماية الذيل بدون دليل مادي خارج السجل هو ادعاء كاذب.

## القرار

1. **شكل النقطة الموحد:** `governance.ValidateCheckpoint` هو الحكم الوحيد لصحة `AuditCheckpoint`: شركة غير صفرية، تجزئة SHA-256 سداسية عشرية بطول 64، وتسلسل صفر مع هاش التكوين فقط، أو تسلسل موجب مع إصدار موجب. المحولان يستدعيان الدالة نفسها قبل الحفظ، ولا يعيد كل منهما تعريف القواعد.
2. **اللقطة الصارمة:** `AuditVerificationSnapshot` يربط السجلات والنقطة في قراءة واحدة. الشرط `len(Logs) == SnapshotSequence == checkpoint.SequenceNumber` إلزامي، والتجزئة النهائية يجب أن تساوي `checkpoint.LogHash`. أي انحراف يُرفض بـ `ErrAuditCheckpointMismatch`؛ لا يُقبل تحقق «بادئة صحيحة» كبديل عن الذيل المفقود.
3. **الفشل المغلق عند غياب النقطة:** الحقل `CheckpointPresent` يميّز «لا توجد نقطة موثوقة» عن «نقطة التكوين». غياب النقطة يعيد `ErrAuditCheckpointMismatch` حتى للسلسلة الفارغة. التحقق بدون نقطة (`VerifyAuditIntegrity`) يبقى متاحاً لكنه لا يدّعي حماية الذيل.
4. **الإضافة الذرية:** `AppendAuditLogAtomic` يعيّن التسلسل والهاش السابق داخل معاملة/قفل شركة واحدة. Memory يستخدم قفل `d.mu` ويربط أثر الترحيل بالسلسلة v2 في نفس وحدة العمل مع rollback عند أي فشل؛ PostgreSQL يستخدم قفل استشاري داخل معاملة الشركة وقيود تفرد `(company_id, sequence_number)`.
5. **حجز النقطة في المحولات:** `SetExternalAuditCheckpoint` يحفظ نقطة مُرقّمة في علاقة مستقلة عن سجل التدقيق (`external_audit_checkpoints` في PostgreSQL، وخريطة منفصلة في Memory). التراجع إلى إصدار أو تسلسل أقدم ممنوع في المحولين، وفي PostgreSQL إضافياً عبر trigger؛ الجدول يخضع لـ `FORCE ROW LEVEL SECURITY` وسياسة المستأجر.
6. **فصل النقطة عن السجل:** «خارجي» هنا يعني «خارج السجل القابل للتعديل» لا «خارج قاعدة البيانات بالضرورة». المحول يوفّر منفذاً محايداً؛ على المضيف أن ينسخ النقطة إلى مخزن بأذونات وسياسة احتفاظ مستقلتين إن أراد حماية الذيل ضد من يملك صلاحية تعديل قاعدة البيانات نفسها. لا توجد خدمة SIEM ولا إدارة مفاتيح ولا مزود محدد داخل النواة.
7. **الاستعادة:** النسخ الاحتياطي/الاستعادة يجب أن يشمل السجل والنقطة معاً. تمرين DR ينشئ نقطة قبل `pg_dump` ويجعل مقارنة النقطة بعد `pg_restore` شرطاً للنجاح.

## البدائل المرفوضة

| البديل | سبب الرفض |
| --- | --- |
| اعتبار نقطة غائبة تساوي `(sequence=0, hash=genesis)` والنجاح | يحوّل غياب الدليل إلى ادعاء حماية؛ السلسلة الفارغة أو المقطوعة تبدو «مُتحققاً منها». |
|قبول تحقق البادئة إذا كان الذيل مختلفاً | يخفي بالضبط الهجوم الذي تهدف النقطة إلى كشفه (حذف الذيل). |
| تخزين النقطة داخل `financial_audit_logs` | النقطة تصبح جزءاً من السجل القابل للتعديل فتفقد استقلالها. |
| تعريف مزود checkpoint أو سياسة احتفاظ داخل `internal/kernal` | يخالف Embedded Core ويجعل النواة تعتمد على SQL أو شبكة أو مزود خارجي. |
| السماح بتحديث النقطة إلى إصدار أقل «لتصحيح خطأ» | يفتح باب إعادة كتابة التاريخ الموثوق؛ التصحيح نقطة جديدة أعلى. |
| الاعتماد على WORM وحده لمنع حذف الذيل | حساب superuser أو `session_replication_role=replica` يتجاوز التريجرات؛ النقطة المستقلة هي الضابط. |

## التبعات والمسؤوليات

- **النواة:** تجزئة canonical لكل الحقول الدلالية، تحقق شكل النقطة، لقطة صارمة، فشل مغلق عند الغياب، واختبارات الحقول/Tampering/الحذف/التزامن داخل `internal/kernal/governance`.
- **SPI/المحولات:** `AtomicAuditLogRepository` و`AuditVerificationSnapshotRepository` و`SetExternalAuditCheckpoint` مع Company Scope وACID وRLS وWORM وrollback. الذرية بين الترحيل والتدقيق مُثبتة على المحولين.
- **المضيف/العمليات:** ملكية النقطة وسياسة الاحتفاظ ونسخها إلى مخزن مستقل الأذونات؛ إدراجها في نسخ احتياطي منفصل؛ تحديد مدة الاحتفاظ ووالي الحفظ وفق متطلبات الامتثال. لا تُعرَّف هذه السياسة في النواة.
- **الوثائق:** [نموذج التهديد](../KERNEL_AUDIT_THREAT_MODEL.md)، [عقد المستودع](../KERNEL_AUDIT_REPOSITORY_CONTRACT.md)، [تقرير المرحلة 10](../KERNEL_REMEDIATION_PHASE10_REPORT.md)، وتحديث القاعدتين `RULE-KERNEL-025/026` في [الثوابت](../../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md) و[المصفوفة](../../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md).

## سياسة الاحتفاظ (قرار مضيف، وليس قاعدة نواة)

1. **`retention window`:** يحتفظ المضيف بنقاط على الأقل لمدة الاحتفاظ القانوني للسجلات المالية؛ النواة لا تفرق بين سنة وخمس سنوات.
2. **الاستقلال:** تُحفظ نسخة من النقطة خارج قاعدة بيانات السجل وبصلاحيات لا يملكها كاتب السجل؛ otherwise يبقى الضمان محدوداً بمن يملك `superuser`.
3. **الترقيات:** التحديث يُنشئ إصداراً أعلى فقط؛ لا حذف ولا تخفيض. أي تصحيح يُسجَّل كنقطة جديدة مع سبب خارج النواة.
4. **الاستعادة:** تُستعاد النقطة مع السجل، ويُفشل DR إذا فقدت النقطة أو لم تطابق آخر سجل.
5. **السرية:** النقطة تجزئة وليست سراً؛ لا إدارة مفاتيح داخل النواة، وتوقيع النقطة إن لزم مسؤولية المضيف.

## الأدلة التنفيذية

`TestCalculateHashProtectsEverySemanticField`، `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint`، `TestMemoryAuditTailDeletionIsDetectedByCheckpoint`، `TestMemoryPhase10AuditFailureRollsBackPostingEffect`، `TestPostgresAuditPhase10ConcurrentAppendSequenceAndChain`، `TestPostgresAuditPhase10TamperingIsDetected`، `TestPostgresAuditPhase10TailDeletionFailsClosedWithCheckpoint`، `TestPostgresAuditPhase10CheckpointCannotRegress`، `TestPostgresPhase10BackfillRechainsLegacyRows`، و`TestDRDrill_FullRecovery/05_audit_chain`. تفاصيل الأوامر والمخرجات في [تقرير المرحلة 10](../KERNEL_REMEDIATION_PHASE10_REPORT.md).
