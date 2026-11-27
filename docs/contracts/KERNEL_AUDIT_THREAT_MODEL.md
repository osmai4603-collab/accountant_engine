# نموذج تهديد التدقيق — Audit Threat Model

- **رمز الوثيقة:** `AE-REMED-P10-THREAT`
- **المرحلة:** 10 — التدقيق وسلامة الأدلة
- **التاريخ:** 2026-11-22
- **المرجع:** [الخطة](../plans/KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [ADR P10-01](../ard/P10_01_external_audit_checkpoint_custody.md)، [عقد المستودع](./KERNEL_AUDIT_REPOSITORY_CONTRACT.md)
- **القواعد:** `RULE-KERNEL-025` و`RULE-KERNEL-026`

## الأصول والحدود

| الأصل | الحماية | الحدود |
| --- | --- | --- |
| `AuditLog` (الحقول الدلالية والهاش) | تجزئة canonical length-prefix + SHA-256 في `CalculateHash` | لا يمنع تعديلاً بصلاحية كتابة على قاعدة البيانات؛ يكشفه عند التحقق |
| التسلسل والهاش السابق | تعيين ذري داخل معاملة/قفل شركة واحدة، وقيد تفرد `(company_id, sequence_number)` | يعتمد على سلامة المحول؛ النواة لا تملك قفل SQL |
| اللقطة | قراءة واحدة متسقة (Repeatable Read في PostgreSQL، قفل قراءة في Memory) | لا تحمي ضد مزود checkpoint مُخترق |
| النقطة الخارجية | علاقة مستقلة، تحقق شكل موحد، منع تراجع، RLS/سياسة مستأجر | «خارج السجل» لا «خارج قاعدة البيانات»؛ الاستقلال الحقيقي مسؤولية المضيف |

## التهديدات والضوابط

| ID | التهديد | الضابط | الاختبار القابل للتحقق | المخاطرة المتبقية |
| --- | --- | --- | --- | --- |
| T-01 | تغيير حقل دلالي (المبلغ في الحمولة، الشركة، الكيان، الفعل، الممثل، السبب، الارتباط، الطابع الزمني) دون تغيير الهاش | كل الحقول تدخل `CalculateHash` بتمثيل length-prefix و payload digest وtimestamp مقيّد للميكروثانية | `TestCalculateHashProtectsEverySemanticField`، `TestCalculateHashGoldenVector` | none داخل النطاق |
| T-02 | إعادة ترتيب السجلات | كل سجل يرتبط بالهاش السابق ويتحقق من تسلسل متصل من 1 | `TestAuditSnapshotVerificationDetectsTampering`، `TestPostgresAuditPhase10TamperingIsDetected` | none |
| T-03 | إدخال سجل في الوسط | رابط الهاش والقيد الفريد للتسلسل | `TestPostgresAuditPhase10TamperingIsDetected` | من يملك `session_replication_role=replica` يستطيع التعديل، لكن التحقق يكشفه |
| T-04 | حذف سجل من الوسط | انكسار الرابط + فحص حد اللقطة مقابل النقطة | `TestAuditSnapshotVerificationDetectsMiddleDeletion`، `TestAuditSnapshotVerificationDetectsReorderedLogs`، `TestPostgresAuditPhase10TamperingIsDetected` (تلاعب السجل الوسط) | none |
| T-05 | حذف الذيل | النقطة الخارجية تقارن الطول والهاش النهائي؛ غيابها فشل مغلق | `TestAuditCheckpointDetectsDeletedTail`، `TestPostgresAuditPhase10TailDeletionFailsClosedWithCheckpoint` | بدون نقطة موثوقة لا يُكتشف؛ مُصرَّح به صراحةً |
| T-06 | تزوير نقطة بقيمة فارغة أو شكل غير قانوني | `ValidateCheckpoint`: هاش 64 سداسياً، شركة غير صفرية، إصدار موجب عند تسلسل موجب، هاش التكوين عند الصفر | `TestValidateCheckpointRejectsMalformedAnchors`، `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint` | النقطة نفسها مُوثوق بها تعاقدياً؛ التوقيع خارج النواة |
| T-07 | تراجع النقطة إلى إصدار/موضع أقدم | رفض في المحولين + trigger في PostgreSQL | `TestPostgresAuditPhase10CheckpointCannotRegress` | none داخل قاعدة البيانات |
| T-08 | إضافة متزامنة تكرر التسلسل أو تكسر الرابط | قفل الشركة/معاملة + قيد تفرد + تسلسل محسوب داخل القفل | `TestMemoryPhase10ConcurrentAppendBuildsOneChain`، `TestPostgresAuditPhase10ConcurrentAppendSequenceAndChain` | none |
| T-09 | لقطة غير متماسكة أثناء الإضافات | لقطة واحدة Repeatable Read / قفل قراءة، ورفض أي انحراف في الحد أو الهاش | `TestPostgresAuditPhase10SnapshotIsStableDuringConcurrentAppends`، `TestAuditSnapshotVerificationIsSafeDuringConcurrentAppends` | none |
| T-10 | قيد مرحّل دون أثر تدقيقي | أثر v2 داخل نفس وحدة العمل، rollback عند فشل التدقيق | `TestMemoryPhase10PostingWritesVerifiableAuditEvidence`، `TestMemoryPhase10AuditFailureRollsBackPostingEffect` | PostgreSQL يضمن ذلك عبر معاملة `uow.AppendAudit` |
| T-11 | فقدان السجل/النقطة عند الكارثة | DR drill ينشئ نقطة قبل النسخ ويجعل المقارنة شرط نجاح | `TestDRDrill_FullRecovery/05_audit_chain` | الاستقلال الحقيقي للنسخة الخارجية مسؤولية المضيف |
| T-12 | تسريب بيانات شركة أخرى عبر النقطة | RLS + `FORCE ROW LEVEL SECURITY` + سياسة المستأجر + Company Scope في النواة | `TestEveryFinancialTableHasForcedRLS` (43 جدولاً) | none داخل المحول |
| T-13 | ترحيل قديم بسلسلة v1 غير صالحة بعد الترقية | backfill يعيد حساب التسلسل والروابط والهاشات | `TestPostgresPhase10BackfillRechainsLegacyRows` | الهاش القديم لا يمكن إثباته؛ الترحيل يعلن سلسلة v2 جديدة |

## الافتراضات غير المغطاة

- لا تمنع النواة ولا المحولات كاتباً يملك صلاحيات قاعدة بيانات كاملة من تعديل السجل؛ الضمان هو **الاكتشاف عند التحقق** لا **المنع المطلق**.
- لا يدخل في النطاق: SIEM، إدارة المفاتيح، توقيع النقطة، دوران المفاتيح، أو تخزين النقطة في مزود خارجي محدد.
- إذا لم ينشئ المضيف نقطة موثوقة خارج السجل، تبقى الحماية عند مستوى «السلسلة الداخلية» فقط، ويُرفض أي ادعاء باكتشاف حذف الذيل.
