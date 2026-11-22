# عقد مستودع التدقيق — Audit Repository Contract

- **رمز الوثيقة:** `AE-REMED-P10-AUDIT-CONTRACT`
- **المرحلة:** 10 — التدقيق وسلامة الأدلة
- **التاريخ:** 2026-11-22
- **المرجع:** [الخطة](../KERNEL_REMEDIATION_EXECUTION_PLAN.md)، [ADR P10-01](./ard/P10_01_external_audit_checkpoint_custody.md)، [نموذج التهديد](./KERNEL_AUDIT_THREAT_MODEL.md)
- **القواعد:** `RULE-KERNEL-025`، `RULE-KERNEL-026`، `RULE-KERNEL-027`

## 1. المنافذ

| المنفذ | المعنى | الضمان |
| --- | --- | --- |
| `AppendAuditLogAtomic(ctx, log) -> log` | يعيّن `ID` و`SequenceNumber` و`PreviousLogHash` ويحسب `CurrentLogHash` ويحفظ السجل | ذري لكل شركة؛ لا يظهر سجل ناقص؛ التسلسل يبدأ من 1 ويزيد بمقدار 1 |
| `AppendAuditLog(ctx, log) -> error` | نسخة مطابقة عند عدم الحاجة للسجل المُخزَّن | نفس ضمان الذرية |
| `GetLastAuditLogForCompany(ctx, companyID)` | آخر سجل أو `nil` | يعيد نسخة defensive؛ لا مرجعاً قابلاً للتعديل |
| `IterateAuditLogs(ctx, companyID, fn)` | تكرار بترتيب التسلسل | Company Scope إلزامي؛ أي خطأ من `fn` يُنهي التكرار |
| `GetAuditVerificationSnapshot(ctx, companyID)` | السجلات + النقطة + حد اللقطة في قراءة واحدة | `len(Logs) == SnapshotSequence`؛ `CheckpointPresent` يفرّق الغياب عن نقطة التكوين |
| `SetExternalAuditCheckpoint(ctx, companyID, checkpoint)` | يحفظ موضعاً موثوقاً خارج السجل | `ValidateCheckpoint` + منع التراجع؛ لا ينشئ دليلاً على ذيل لم يُثبت |

يُعاد التصدير عبر `pkg/api/v1/governance` كأسماء مستعارة لأنواع النواة وفق ADR `P7-01`؛ لا يُكشف نوع جديد من `internal`.

## 2. قواعد الحالة

1. `CalculateHash` هو المرجع الوحيد: أي تغيير في الحقول الدلالية يغير الهاش.
2. الهاش السابق لأول سجل هو `GenesisHash`؛ لكل سجل لاحق هو `CurrentLogHash` للسجل السابق.
3. `ValidateCheckpoint` تُطبَّق في النواة عند التحقق وفي كل محول قبل الحفظ؛ لا توجد قواعد محلية متباينة.
4. عند غياب النقطة: `CheckpointPresent=false` وفحص النقطة يُعيد `ErrAuditCheckpointMismatch`؛ لا تُقبل نقطة افتراضية.
5. عند وجودها: يُشترط `SnapshotSequence == checkpoint.SequenceNumber` و`len(Logs)==SnapshotSequence` وآخر هاش == `checkpoint.LogHash`.
6. لا تُقبل نقطة بنفس الإصدار أو بإصدار/تسلسل أقل من المحفوظ؛ التصحيح نقطة أعلى.

## 3. مصفوفة السلوك

| الحالة | Memory | PostgreSQL |
| --- | --- | --- |
| نجاح الإضافة | قفل `d.mu`، نسخة defensive من الحمولة، إرجاع نسخة مستقلة | معاملة شركة + قفل استشاري + قيد تفرد |
| إضافات متزامنة | تسلسل فريد 1..N داخل القفل | تسلسل فريد 1..N داخل القفل/المعاملة |
| لقطة أثناء كتابة | copy تحت `RLock` | `RepeatableRead` + `ReadOnly` في معاملة واحدة |
| Company Scope | مفاتيح `map[CompanyID]` | RLS + `set_config('app.current_company_id', ...)` |
|Immutable | قفل/نسخ داخلي | WORM triggers + `FORCE ROW LEVEL SECURITY` للنقطة |
| rollback عند فشل التدقيق | `postingSnapshot` يستعيد السجلات v2 والقديمة | معاملة `uow.AppendAudit` تُلغى بالكامل |
| الترقية | لا ترحيل (ذاكرة) | backfill يعيد تسلسل/هاش v2 ويحذف الأعمدة عند down |
| غياب نقطة | فشل مغلق | فشل مغلق |

## 4. الأدلة

`TestCalculateHashProtectsEverySemanticField`، `TestValidateCheckpointRejectsMalformedAnchors`، `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint`، `TestMemoryPhase10AuditLogPayloadIsDefensivelyCopied`، `TestMemoryPhase10ConcurrentAppendBuildsOneChain`، `TestMemoryPhase10PostingWritesVerifiableAuditEvidence`، `TestMemoryPhase10AuditFailureRollsBackPostingEffect`، `TestPostgresAuditPhase10ConcurrentAppendSequenceAndChain`، `TestPostgresAuditPhase10SnapshotIsStableDuringConcurrentAppends`، `TestPostgresAuditPhase10TamperingIsDetected`، `TestPostgresAuditPhase10TailDeletionFailsClosedWithCheckpoint`، `TestPostgresAuditPhase10MissingCheckpointFailsClosed`، `TestPostgresAuditPhase10CheckpointCannotRegress`، `TestPostgresPhase10BackfillRechainsLegacyRows`، `TestDRDrill_FullRecovery/05_audit_chain`.

> ملاحظة تحريرية: كل اسم اختبار أعلاه مذكور كدليل تشغيل؛ عند إضافته إلى مصفوفة التتبع النووية يُذكر اسم اختبار النواة فقط داخل backticks لأن بوابة المصفوفة تحلل الاختبارات داخل `internal/kernal` فقط.
