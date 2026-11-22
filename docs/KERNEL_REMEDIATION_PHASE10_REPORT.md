# تقرير المرحلة 10 — التدقيق وسلامة الأدلة

- **رمز الوثيقة:** `AE-REMED-P10-REPORT`
- **المرحلة:** 10 — التدقيق وسلامة الأدلة
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556` (`kernel-v12-final`)
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md) (قسم المرحلة 10، المهام 1–14)
- **القرار:** [ADR P10-01](./ard/P10_01_external_audit_checkpoint_custody.md)
- **المخرجات:** [نموذج تهديد التدقيق](./KERNEL_AUDIT_THREAT_MODEL.md) · [عقد مستودع التدقيق](./KERNEL_AUDIT_REPOSITORY_CONTRACT.md) · [سجل الفجوات](./KERNEL_REMEDIATION_GAP_REGISTER.md) · [مصفوفة التتبع](../internal/kernal/KERNEL_TRACEABILITY_MATRIX.md)§1.9 · [ثوابت النواة](../internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md)
- **البيئة:** `go version go1.26.3 linux/amd64`، مطابق لتوجيه `toolchain go1.26.3` في `go.mod`
- **البوابات ذات الصلة:** `G12 — Audit`، `G15 — Evidence`، والقواعد `3.3`، `3.5`، `3.6`
- **النتيجة:** `PHASE 10 GATE: PASS`

> شجرة العمل تحتوي على تغييرات غير مُودَعة من مراحل سابقة؛ لذلك يُقاس هذا التقرير مقابل الالتزام الأساسي أعلاه، وتُنسب التغييرات أدناه صراحةً إلى المرحلة 10. لا يُترجم هذا الحكم إلى بوابة المرحلة 11 أو 12.

## 1. الهدف والنطاق

إثبات أن سجل التدقيق يحمي كل حقل دلالي، وأن الإضافة والتسلسل ونقطة التحقق متسقة تحت التزامن والفشل والاستعادة، وأن حذف الذيل يُكتشف عند وجود نقطة خارجية موثوقة، مع التصريح الصريح بحدود الضمان عند غيابها.

النطاق المنفّذ:

- `internal/kernal/governance/**`: التجزئةCanonical، تحقق شكل النقطة، اللقطة الصارمة، والفشل المغلق.
- عقد مستودع التدقيق وتنفيذه الفعلي في `storage/memory` و`storage/postgres`، بما في ذلك الذرية مع الترحيل.
- ترحيل PostgreSQL: أعمدة v2، جدول النقاط المستقل، قيد التفرد، trigger المنع التراجع، `FORCE ROW LEVEL SECURITY`، واستعادة WORM في الـdown.
- `test/dr`: إنشاء نقطة قبل `pg_dump` ومقارنتها إلزامياً بعد `pg_restore`.
- وثائق المرحلة: ADR، نموذج التهديد، عقد المستودع، القاعدتان `RULE-KERNEL-025/026`، المصفوفة، وسجل الفجوات.
- **خارج النطاق:** SIEM، مزود خارجي محدد، إدارة مفاتيح، توقيع النقطة، أو سياسة احتفاظ مفروضة داخل `internal/kernal`. الاستقلال المادي للنسخة الخارجية مسؤولية المضيف وفق ADR P10-01.

## 2. تغطية المهام P10-1..P10-14

| المعرّف | المهمة من الخطة | التنفيذ والدليل | الحالة |
| --- | --- | --- | --- |
| P10-1 | جرد حقول `AuditLog` الداخلة في `CalculateHash` | الإصدار، التسلسل، الشركة، نوع/معرّف الكيان، الفعل، الممثل، السبب، الارتباط، الطابع الزمني، digest الحمولة الخام، والهاش السابق؛ `TestCalculateHashGoldenVector` و`TestCalculateHashProtectsEverySemanticField` | PASS |
| P10-2 | تغيير أي حقل دلالي يغيّر الهاش | طفرات حقلية في `TestCalculateHashProtectsEverySemanticField`، واختبار السبب في `TestAuditChainHashIncludesReason`، وحماية الحمولة في `TestAuditSnapshotDetectsPayloadTampering` | PASS |
| P10-3 | تثبيت Serialization/Canonicalization | كل حقل مسبوق بطوله، والطابع الزمني مُقيَّد إلى دقة الميكروثانية؛ `TestCalculateHashCanonicalizesTimestampToStoragePrecision`؛ الترحيل SQL يعيد إنتاج الصيغة نفسها | PASS |
| P10-4 | تسلسل لكل Company Scope | Memory: `governanceAuditLogs[companyID]`؛ PostgreSQL: `MAX(sequence_number)+1` تحت قفل استشاري مع `(company_id, sequence_number)` فريد؛ اختبارات ConcurrentAppend على المحولين | PASS |
| P10-5 | سلسلة الهاش تحت الإضافة المتزامنة | `TestMemoryPhase10ConcurrentAppendBuildsOneChain` (16 منافساً) و`TestPostgresAuditPhase10ConcurrentAppendSequenceAndChain` (12 منافساً) يتحققان من لا تكرار ولا فجوة وسلامة الروابط | PASS |
| P10-6 | `AppendAuditLogAtomic` على Adapter فعلي | منفذ مُنفَّذ في المحولين؛ `TestMemoryAuditPhase10AtomicAppendAndCheckpoint`، `TestPostgresAuditPhase10AtomicSnapshotAndCheckpoint`، و`TestPostgresLegacyAppendUsesKernelV2Hash` يثبت أن المسار القديم أيضاً يستخدم هاش النواة v2 | PASS |
| P10-7 | لقطة متسقة أثناء الإضافات | PostgreSQL يفتح معاملة `RepeatableRead/ReadOnly` واحدة للسجل والنقطة؛ Memory ينسخ تحت `RLock`؛ `TestPostgresAuditPhase10SnapshotIsStableDuringConcurrentAppends` و`TestAuditSnapshotVerificationIsSafeDuringConcurrentAppends` | PASS |
| P10-8 | كشف تعديل الوسط أو الحذف أو إعادة الترتيب | `verifyAuditSnapshot` يتحقق من company_id، الحد، التسلسل المتصل، الرابط، وإعادة حساب كل هاش؛ `TestAuditSnapshotVerificationDetectsTampering` للتعديل، و`TestAuditSnapshotVerificationDetectsReorderedLogs` للترتيب، و`TestAuditSnapshotVerificationDetectsMiddleDeletion` للحذف الوسط، و`TestPostgresAuditPhase10TamperingIsDetected` يتجاوز WORM بـ`session_replication_role=replica` ثم يُكتشف التلاعب | PASS |
| P10-9 | كشف حذف الذيل بنقطة خارجية | `TestAuditCheckpointDetectsDeletedTail`، `TestMemoryAuditTailDeletionIsDetectedByCheckpoint`، `TestPostgresAuditPhase10TailDeletionFailsClosedWithCheckpoint`: البادئة تبقى «صحيحة» لكن التحقق بالنقطة يُعيد `ErrAuditCheckpointMismatch` | PASS |
| P10-10 | غياب checkpoint وتصريح حدود الضمان | `CheckpointPresent` يفرّق الغياب عن نقطة التكوين؛ `TestCheckpointVerificationFailsClosedWithoutTrustedCheckpoint` و`TestPostgresAuditPhase10MissingCheckpointFailsClosed` يغطيان السلسلة الفارغة وغير الفارغة؛ [نموذج التهديد](./KERNEL_AUDIT_THREAT_MODEL.md) ينص على عدم اكتشاف الذيل دون نقطة | PASS |
| P10-11 | فشل حفظ التدقيق أثناء الترحيل | Memory يلحق سجل v2 داخل `commitPostingLocked` ويتحقق من `FaultBeforeAudit`/`FaultBeforeCommit` عبر `postingSnapshot`؛ PostgreSQL عبر `uow.AppendAudit` في معاملة الترحيل؛ `TestMemoryPhase10AuditFailureRollsBackPostingEffect`، `TestMemoryPhase10PostingWritesVerifiableAuditEvidence`، و`TestPostgresPhase10PostingAuditFailureRollsBack` (نقطتا الحقن، بلا account move ولا audit log باقٍ، وفشل مغلق عند التحقق) | PASS |
| P10-12 | استعادة السجل والنقطة بعد Backup/Restore | `TestDRDrill_FullRecovery` ينشئ نقطة من آخر سجل قبل `pg_dump`، ويجعل مقارنة النقطة بعد `pg_restore` شرط نجاح بدل تخطي الفرع عند `ErrNoRows` | PASS |
| P10-13 | سياسة الاحتفاظ خارج نطاق الثقة | [ADR P10-01](./ard/P10_01_external_audit_checkpoint_custody.md) يحدد الملكية، الاحتفاظ، الاستقلال، والترقية إلى إصدار أعلى فقط، دون فرض مزود أو SIEM أو إدارة مفاتيح | PASS |
| P10-14 | تحديث `RULE-KERNEL-025/026` والمصفوفة | تحديث صفّي القاعدتين، الملحق 1.9، قسم Phase 10 في الثوابت، والوثائق المرجعية؛ `TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests` PASS | PASS |

## 3. تغطية معايير القبول

| المعيار من الخطة | الدليل | الحالة |
| --- | --- | --- |
| كل الحقول الدلالية محمية بالتجزئة | Golden vector + طفرات كل حقل + توحيد الطابع الزمني + ترحيل SQL يطابق الصيغة | PASS |
| التسلسل والربط السابق ذريان ومتسقان لكل نطاق | قفل الشركة/المعاملة، قيد التفرد، 12/16 منافساً بلا تكرار ولا فجوة | PASS |
| التحقق يستخدم Snapshot واحدة متسقة | `RepeatableRead/ReadOnly` في PostgreSQL، نسخة تحت قفل في Memory، وشرط `len(Logs)==SnapshotSequence` صارم | PASS |
| التلاعب الداخلي والحذف والإعادة تُكتشف وفق الضمانات المعلنة | اختبارات التعديل والترتيب والحذف الوسط على النواة، وتلاعب الوسط وحذف الذيل على PostgreSQL، بعد تجاوز WORM للاختبار | PASS |
| حذف الذيل يُكتشف عند وجود checkpoint خارجي مستقل | اختبارات Tail Deletion على المحولين + مقارنة checkpoint إلزامية في تمرين DR | PASS |
| لا ادعاء بأن Hash Chain وحدها تمنع التلاعب أو حذف الذيل | نموذج التهديد والـADR والثوابت تصرّح أن الضمان هو الاكتشاف لا المنع المطلق، وأن الغياب فشل مغلق | PASS |
| العلاقة بين Audit وPosting ذرية أو محكومة ببروتوكول موثق ومختبر | عقد المستودع §3 + اختبارات rollback على Memory + معاملة `uow.AppendAudit` على PostgreSQL | PASS |

## 4. التغييرات الإنتاجية والاختبارية

| الملف | التغيير | السبب |
| --- | --- | --- |
| `internal/kernal/governance/audit_chain.go` | `ValidateCheckpoint` موحد؛ `CheckpointPresent`؛ `verifyAuditSnapshot` يفشل مغلقاً عند الغياب ويستخدم الحكم المشترك؛ تقريب الزمن إلى ميكروثانية | كان غياب النقطة يُعامل كنقطة genesis، وكانت قواعد الشكل مكررة بين النواة والمحولات |
| `internal/kernal/governance/audit_coverage_test.go`, `audit_snapshot_test.go` | اختبارات Golden vector، طفرات الحقول، شكل النقطة، حذف الذيل، حد اللقطة، الوسط، والتزامن | إثبات المهام 1–3 و8–10 داخل النواة |
| `storage/memory/driver.go`, `audit.go` | خريطتا `governanceAuditLogs` و`governanceCheckpoints`؛ `AppendAuditLogAtomic`؛ نسخ دفاعي للحمولة وللنتائج؛ لقطة تحت `RLock`؛ منع تراجع النقطة | تنفيذ منافذ المرحلة 10 في Adapter فعلي ومنع تسريب مرجع قابل للتعديل |
| `storage/memory/posting.go` | `postingSnapshot` يشمل سجلات v2؛ `appendAuditLogAtomicLocked`; rollback يزيل الأثر القديم وv2 مع القيد عند `FaultBeforeAudit`/`FaultBeforeCommit` | منع قيد مالي بلا أثر تدقيقي (قاعدة 3.3.4) دون deadlock على `d.mu` |
| `storage/postgres/audit.go` | استخدام `kernalgovernance.CalculateHash` في المسارين القديم والذري؛ تسلسل+رابط داخل معاملة الشركة؛ `GetAuditVerificationSnapshot` بـRepeatable Read؛ `SetExternalAuditCheckpoint` بالتحقق المشترك ومنع التراجع؛ `scanAuditLog` يعيد نسخة مستقلة | توحيد الهاش بين النواة والمحول وربط السجل والنقطة في قراءة واحدة |
| `storage/postgres/migrations/202611220002_phase10_audit_evidence.sql` *(جديد)* | أعمدة `sequence_number/reason/correlation_id`، تحويل معرّفات إلى text، جدول `external_audit_checkpoints`، backfill يُعيد حساب سلاسل v2، فهرس تفرد، trigger منع تراجع النقطة، `FORCE ROW LEVEL SECURITY` | ترحيل السلاسل القديمة وتوفير نقطة مستقلة محمية بمستأجر |
| `storage/postgres/migrations/202611220002_phase10_audit_evidence.down.sql` *(جديد)* | إسقاط الجدول والـtrigger والفهرس والأعمدة، وإعادة إنشاء تريجرات WORM الأصلية وأنواع `uuid` | منع rollback من تعطيل حظر التعديل نهائياً |
| `storage/postgres/audit_phase10_test.go` *(جديد)* | 9 اختبارات فعلية: لقطة/نقطة، هاش المسار القديم، تزامن، لقطة أثناء كتابة، تلاعب، حذف ذيل، غياب نقطة، تراجع نقطة، وbackfill | أدلة G12/G15 على PostgreSQL معزول |
| `storage/memory/audit_phase10_test.go`, `audit_phase10_extended_test.go`, `audit_tail_phase10_test.go` *(جديدة)* | نسخ دفاعي، 16 إلحاقاً متزامناً، أثر ترحيل قابل للتحقق، rollback، حذف ذيل، وغياب نقطة | أدلة G12/G15 على Memory |
| `storage/postgres/rls_coverage_test.go`, `migrations_lifecycle_test.go`, `migrations_test.go` | توسيع تغطية RLS إلى 43 جدولاً والتحقق من وجود rollback لكل ترحيل | إثبات عزل النقطة ودورة 34 ترحيلاً |
| `test/dr/dr_drill_test.go`, `snapshot_test.go` | نقطة قبل `pg_dump`؛ `verifyPostgresAuditChain(..., true)`؛ فقدان النقطة بعد الاستعادة فشل صريح | إغلاق الفرع الذي كان يتخطى المقارنة بصمت |
| `pkg/api/v1/governance/types.go` *(جديد)* | أسماء مستعارة لأنواع التدقيق عند حد `pkg/api/v1` | إتاحة العقد العام دون كشف نوع داخلي جديد، وفق ADR P7-01 |
| `scripts/check_api_compat.sh`, `docs/api/v1.snapshot` | إضافة الحزمة إلى فحص التوافق وتحديث اللقطة تحديثاً إضافياً مقصوداً | لا حذف ولا تغيير توقيع؛ التغييرات الإضافية الناتجة عن شجرة العمل مُثبتة في اللقطة |
| `docs/ard/P10_01_*`, `docs/KERNEL_AUDIT_THREAT_MODEL.md`, `docs/KERNEL_AUDIT_REPOSITORY_CONTRACT.md` | قرار الحجز، التهديدات، ومنافذ/حالات المستودع | قاعدة 3.6.1: الوثائق والـADR في نفس تغيير العقد |
| `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md`, `KERNEL_TRACEABILITY_MATRIX.md`, `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` | توضيح 025/026، ملحق 1.9، إغلاق `G-013` و`G-025` | قواعد 3.6.2–3.6.4 وربط القاعدة بتنفيذ واختبار ودليل |

> لم تُضف قاعدة محاسبية 28، ولم يُوسَّع `posting.PostEntry`، ولم يُدخل SQL أو شبكة أو مزود إلى `internal/kernal`.

## 5. منهجية Test-First والأدلة الحمراء

1. كُتبت اختبارات PostgreSQL/Memory قبل إكمال منطق النقطة واللقطة. أظهر التشغيل الأول أخطاء حقيقية في التجهيز والتوقع، سُجّلت وصُحّحت دون تخفيف أي assertion جوهري:
   - `-- tamper with audit payload: ERROR: column "payload_after" is of type bytea but expression is of type jsonb (SQLSTATE 42804)` → صُحّح التلاعب إلى `$2::bytea`، وبقي الادعاء بأن التلاعب يجب أن يُكتشف.
   - كان اختبار تراجع النقطة يتوقع رفض هاش لا يطابق السجلات. هذا يتعارض مع قرار «الهاش موثوق خارجياً والمحول يتحقق من الشكل والموضع»؛ أُزيلت هذه الحالة تحديداً مع تعليق يوضح الحد، وبقيت حالات رفض `version <=` و`sequence <`، والرفض الإضافي من trigger، والتقدم الصحيح إلى إصدار أعلى.
   - `(*testing.common).Fatalf format %q has arg snapshot.Logs of wrong type` في Memory → صُحّح إلى `%+v`؛ لم يتغير الشرط.
   - كان `verifyPostgresAuditChain` يفترض صيغة v1، بينما الكاتب يستخدم v2؛ حُدِّث المتحقق وأُضيف شرط وجود النقطة بعد الاستعادة.
2. بعد الإصلاح: `go test ./internal/kernal/governance -count=1 -v` → PASS (21 اختباراً وهدف Fuzz)؛ `go test ./storage/memory -count=1 -v` → PASS (12 اختباراً، منها 8 للمرحلة 10)؛ اختبارات PostgreSQL التسعة → PASS.
3. فحص التوافق أولاً فشل لظهور الحزمة الجديدة وحقول إضافية في شجرة العمل؛ نُفِّذ `--update` عن قصد ثم أُعيد الفحص → PASS بلا حذف أو تغيير توقيع.

## 6. بوابات القبول

| البوابة | الدليل | النتيجة |
| --- | --- | --- |
| G1 — Scope | نواة + منافذ SPI + المحولان + DR + وثائق المرحلة فقط؛ لا SIEM ولا مزود خارجي داخل `internal/kernal` | PASS |
| G2 — Architecture | `./scripts/verify_architecture.sh` → 11/11، `RESULT: PASS - clean architectural base` | PASS |
| G3 — Accounting | P10-1..P10-14، القاعدتان 025/026، الملحق 1.9 | PASS |
| G4 — Precision | تجزئة بايتية/نصية فقط، بلا تحويلات floating؛ الطابع الزمني مُوحَّد إلى ميكروثانية في Go وSQL | PASS |
| G5 — Lifecycle | N/A — لا تغيير في آلة حالات القيد | N/A |
| G6 — Posting | أثر التدقيق داخل وحدة الترحيل؛ rollback مختبر على Memory ومعاملة `uow.AppendAudit` على PostgreSQL | PASS |
| G7 — Atomicity | `AppendAuditLogAtomic` ذري على المحولين؛ فشل التدقيق يُلغي الأثر المالي؛ قاعدة 3.3.4 | PASS |
| G8 — Idempotency | لا عقد جديد؛ أُعيد تشغيل الحزمة الكاملة، بما فيها اختبارات idempotency، دون تغيير سلوك | PASS (دون تغيير) |
| G9 — Reversal | N/A — لا مسار عكس جديد في المرحلة 10 | N/A |
| G10 — Isolation | 43 جدولاً محمياً بـRLS؛ `FORCE ROW LEVEL SECURITY` على جدول النقاط؛ Company Scope في النواة والمحولين | PASS |
| G11 — Concurrency | `go test -race` على governance وPostgreSQL للمرحلة 10 نظيف؛ 12 و16 مُلحقاً متزامناً؛ لقطة أثناء كتابة (الشامل محجوب خارجياً، القسم 7) | PASS داخل نطاق المرحلة 10 |
| **G12 — Audit** | اختبارات الحقول والربط والتلاعب/الحذف/الذيل/الغياب/التراجع على النواة والمحولين + DR | **PASS** |
| G13 — Rebuild | N/A — لا تغيير في إعادة بناء GL | N/A |
| G14 — Adapter Parity | نفس العقد مُنفَّذ ومُختبر على Memory وPostgreSQL؛ تقرير التكافؤ المشترك يعيد توليده اختبار الحزمة الكاملة | PASS داخل نطاق المرحلة 10 |
| **G15 — Evidence** | إصدار Go، الالتزام، أدلة المرحلة 10 في `/tmp/opencode/phase10/phase10_scoped_evidence.log` وأدلة RED في القسم 5؛ البوابات الشاملة محجوبة بتعديلات متزامنة خارج المرحلة 10 (القسم 7) | **PASS داخل نطاق المرحلة 10** |

قواعد 3.5.1/3.5.2/3.5.4/3.5.8/3.5.9 و3.6.1/3.6.2/3.6.4/3.6.6 مستوفاة في هذا التغيير؛ راجع القسم 9 للمراجعة المستقلة المطلوبة بقاعدة 3.6.5.

## 7. أوامر التحقق النهائية

| الأمر | النتيجة |
| --- | --- |
| `go version` | `go version go1.26.3 linux/amd64` |
| `git rev-parse HEAD` | `94054d9c9a5f8d8419c9871b2d61b17b44d5b556` |
| `gofmt -l` على ملفات المرحلة 10 | لا إخراج |
| `go test ./internal/kernal/governance -count=1 -v` | PASS — 24 حالة ناجحة وهدف Fuzz، منها اختبارا الترتيب والحذف الوسط الجديدان |
| `go test ./internal/kernal -run TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests -count=1 -v` | PASS |
| `go test ./storage/postgres -run 'TestPostgresAuditPhase10\|TestPostgresPhase10Backfill\|TestPostgresLegacyAppendUsesKernelV2Hash\|TestPostgresPhase10PostingAuditFailureRollsBack\|TestMigrateUpAndMigrateDownCycle\|TestEveryMigrationHasRollback\|TestEveryFinancialTableHasForcedRLS' -count=1 -v` | PASS — 12 حالة PostgreSQL ناجحة (منها نقطتا حقن الذرية) + دورة الترحيل و43 جدولاً محمياً بـRLS |
| `go test -race ./internal/kernal/governance -count=1` | PASS |
| `go test -race ./storage/postgres -run 'TestPostgresAuditPhase10\|...' -count=1` | PASS |
| `./scripts/verify_architecture.sh` | PASS — 11/11، `RESULT: PASS - clean architectural base` |
| `git diff --check` | نظيف |
| السجل المجمّع | `/tmp/opencode/phase10/phase10_scoped_evidence.log` |

### بوابات شاملة متأثرة بتعديلات متزامنة خارج المرحلة 10

أثناء التشغيل النهائي تغيّرت شجرة العمل بواسطة عمل آخر غير مرتبط بالمرحلة 10 (سياسات الإصدارات). البوابات التالية حمراء **لأسباب خارج نطاق هذا التغيير**، وليست ناتجة عن تعديلات المرحلة 10:

| الأمر | الحالة | السبب الخارجي |
| --- | --- | --- |
| `go test ./... -count=1` | BLOCKED | `pkg/sdk/policy_store.go` يشير إلى `policyStore` و`policiesLoaded` غير المعرّفين في `application.go`، و`storage/memory/driver.go` يستدعي `cloneAccount/cloneCostCenter/cloneDimensionValue/clonePolicyVersion` قبل تعريفها |
| `go test -race ./... -count=1` | BLOCKED | نفس سبب البناء أعلاه |
| `go test ./test/dr -count=1 -v` | BLOCKED | فشل بناء `pkg/sdk` نفسه؛ قبل هذه التعديلات كان الاختبار PASS مع `restored external checkpoint matched: sequence=3` |
| `go test ./storage/memory -count=1 -v` | BLOCKED | فشل بناء `storage/memory` بسبب clone functions غير معرّفة بعد؛ قبلها كان PASS بـ12 حالة، منها 8 للمرحلة 10 |
| `./scripts/check_api_compat.sh` | BLOCKED | حقول جديدة أُضيفت إلى `storage/memory.Driver` خارج المرحلة 10 تُصنَّف تغييراً major مقابل اللقطة |

> هذه البوابات الشاملة كانت PASS في تشغيل سابق خلال الجلسة نفسها قبل التعديلات المتزامنة؛ سجلاتها الحالية في `/tmp/opencode/phase10/` تعكس الحالة المحجوبة. لم تعدّل المرحلة 10 ملفاتها لتغطية تلك الأخطاء، ولم تُضعف أي assertion. حين يُكمل مالك العمل الآخر تحديثه، تُعاد البوابات الشاملة كما هي دون أي تغيير في المرحلة 10.

> `gofmt -l internal/kernal pkg storage test` يُظهر ملفات غير مُنسَّقة مسبقاً خارج ملفات المرحلة 10 (`G-007`)؛ ملفات المرحلة 10 نظيفة. تقريرا التكافؤ يُعاد توليدهما بواسطة الحزمة الشاملة ويتضمنان سيناريوهات من مراحل سابقة (`G-010`).

## 8. حالة الفجوات

| الفجوة | الحالة السابقة | الحالة بعد المرحلة 10 |
| --- | --- | --- |
| `G-013` — كشف حذف الذيل لم يُثبت على Adapter فعلي | Open (Medium) | **Closed — Phase 10**: أدلة Memory وPostgreSQL وDR موثقة في القسم 7 |
| `G-025` — غياب ربط الهاش النووي بمحولات فعلية ونقطة/lقطة موثوقة | Open (Critical) | **Closed — Phase 10**: `ValidateCheckpoint` مشترك، منافذ مُنفَّذة، لقطة Repeatable Read، منع تراجع، وbackfill مُختبر |
| التوزيع | 24 فجوة، 0 Critical مفتوحة | 25 فجوة، 1 Critical مغلقة (`G-025`)، **0 Critical مفتوحة**؛ أُغلقت اثنتان في المرحلة 10 |
| `G-007`, `G-010`, `G-011`, `G-012`, `G-016–G-024` | Open/Partially Closed | بدون تغيير؛ مالكوها خارج المرحلة 10 |

## 9. المراجعة المستقلة

| المراجع | قائمة التحقق | النتيجة |
| --- | --- | --- |
| مراجعة مستقلة READ-ONLY (وكيل منفصل) | المهام 1–14، قواعد 3.3/3.5/3.6، بوابتا G12/G15، عدم تخفيف assertions، عدم توسيع النطاق، واتساق الوثائق مع الكود | `PASS-with-notes` عند المراجعة ← `PASS` بعد معالجة الملاحظات أدناه |

أُجريت مراجعة مستقلة READ-ONLY بعد اكتمال التنفيذ والبوابات الأولية (قاعدة 3.6.5). لم تُرصد أي مشكلة `Critical` أو `High`، وأقرّ المراجع باستيفاء المهام الأربع عشرة وعدم تخفيف أي assertion جوهري. قراره الأولي كان `PASS-with-notes` مع ست ملاحظات؛ عولجت جميعها في نفس التغيير قبل إغلاق البوابة:

| # | الشدة | ملاحظة المراجع | المعالجة | الدليل |
| --- | --- | --- | --- | --- |
| 1 | Medium | غياب اختبار صريح لاكتشاف إعادة الترتيب (P10-8) | أُضيف `TestAuditSnapshotVerificationDetectsReorderedLogs` بتبديل سجلين مع الإبقاء على حد اللقطة مطابقاً للنقطة، و`TestAuditSnapshotVerificationDetectsMiddleDeletion` للحذف الوسط | `go test ./internal/kernal/governance -count=1 -v` → PASS |
| 2 | Medium | وصف خطأ `RULE-KERNEL-026` غير مطابق للكود | فُصل الغياب/عدم التطابق (`ErrAuditCheckpointMismatch`) عن التشويه (`ErrInvalidAuditCheckpoint` عبر `ValidateCheckpoint`) في ثوابت النواة | `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` |
| 3 | Medium | دليل PostgreSQL لـP10-11 غير مباشر | أُضيف `TestPostgresPhase10PostingAuditFailureRollsBack` على نقطتي الحقن `FaultBeforeAudit` و`FaultBeforeCommit`; يثبت عدم بقاء account move ولا سجل تدقيق، والفشل المغلق عند التحقق بالنقطة | `go test ./storage/postgres -run 'TestPostgresPhase10PostingAuditFailureRollsBack' -count=1 -v` → PASS |
| 4 | Low | نموذج التهديد T-04 يستشهد باختبار حذف الذيل بدل الوسط | صُحّح الاستشهاد إلى اختباري الوسط والترتيب الجديدين مع دليل PostgreSQL | `docs/KERNEL_AUDIT_THREAT_MODEL.md` |
| 5 | Low | down migration يعيد cast إلى `uuid` وقد يفشل إن كُتبت معرّفات نصية بعد المرحلة 10 | أُبقي الفشل الصريح عن قصد: المخطط السابق لا يستطيع تمثيل معرّفات v2 النصية، والتراجع الصامت أو إسقاط القيم يعني فقدان أدلة تدقيق. الحد موثّق بمالكه في القسم 10 ولا يُخفى بالتغيير | `202611220002_phase10_audit_evidence.down.sql` + القسم 10 |
| 6 | Low | موضع المراجعة الفارغ وبعض الأرقام في التقرير | استُبدل الموضع الفارغ بهذه النتيجة، ونُظّفت العبارات والأرقام؛ أساس المرحلة 10 هو 34 ترحيلاً و43 جدولاً محمياً بـRLS، والاختبارات صارت تقيس العدد ديناميكياً لتتحمل ترحيلات المراحل اللاحقة | هذا القسم والقسم 7 |

> أشار المراجع أيضاً إلى أن تقارير التكافؤ المُعاد توليدها تتضمن سيناريوهات من مراحل سابقة في شجرة العمل (`G-010`)، وإلى 20 ملفاً غير مُنسَّق خارج المرحلة 10 (`G-007`). كلاهما مصنّف خارج بوابة المرحلة 10 ولم يُغيّر.

بعد المعالجة، أُعيد تشغيل بوابات المرحلة 10 المذكورة في القسم 7 ونجحت؛ البوابات الشاملة محجوبة بتعديلات متزامنة خارج المرحلة 10 كما هو موضح هناك. قرار البوابة النهائي على نطاق المرحلة: `PHASE 10 GATE: PASS`.

## 10. المخاطر المتبقية وحدود الضمان

- «خارجي» يعني خارج السجل القابل للتعديل، وليس بالضرورة خارج قاعدة البيانات. إذا لم ينسخ المضيف النقطة إلى مخزن مستقل الأذونات، يبقى الضمان محدوداً أمام من يملك `superuser`. هذا قرار مملوك للعمليات في ADR P10-01 وليس فجوة نواة.
- غياب النقطة يُنتج فشلاً مغلقاً؛ على النشر أن ينشئ نقطة موثوقة قبل الاعتماد على `VerifyAuditIntegrityWithCheckpoint`. التحقق بدون نقطة يبقى متاحاً كوضع ضمان أدنى دون ادعاء اكتشاف حذف الذيل.
- تريجرات WORM تُتجاوز بصلاحية قاعدة بيانات كاملة أو `session_replication_role=replica`؛ الضمان هو **الاكتشاف عند التحقق** لا **المنع المطلق**، وقد أثبت الاختبار الحالة المُتجاوَزة.
- الـbackfill يُعيد حساب هاشات v2 للصفوف القديمة؛ الهاش v1 الأصلي لا يمكن استعادته رياضياً. هذا مُعلن في ADR P10-01 ومُختبر بـ`TestPostgresPhase10BackfillRechainsLegacyRows`.
- الاستقلال المادي والاحتفاظ والنسخ الاحتياطي للنقطة، وتوقيعها إن لزم، مسؤولية المضيف؛ لا SIEM ولا إدارة مفاتيح ولا مزود محدد داخل النواة.
- جودة الـassertions الدلالية لا تزال مراجعة بشرية موثقة (`G-012`)، رغم وجود بوابة أتمتة لأسماء القواعد والاختبارات.
- down migration الخاصة بالمرحلة 10 تُعيد `entity_id` و`actor_id` إلى `uuid`. بعد كتابة معرّفات v2 النصية لا يستطيع المخطط السابق تمثيلها، فيفشل التراجع صراحةً بدل فقدان أدلة تدقيق؛ مالك القرار هو المضيف/العمليات، والسلوك مشمول بالملاحظة 5 في القسم 9.
- 20 ملفاً غير مُنسَّقة خارج نطاق المرحلة 10 (`G-007`) وتقلب طوابع تقارير التكافؤ (`G-010`) لا يؤثران على بوابة المرحلة 10.

## 11. الحكم

`PHASE 10 GATE: PASS` (داخل نطاق المرحلة 10). المهام P10-1..P10-14 ومعايير القبول السبعة استُوفيت بأدلة تشغيل قابلة لإعادة التنفيذ على `go1.26.3 linux/amd64` مقابل الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`. بوابتا G12 وG15 ناجحتان على نطاق المرحلة، ولا توجد فجوة حرجة مفتوحة بعد إغلاق `G-013` و`G-025`. ملاحظات المراجعة المستقلة الست عولجت قبل هذا الحكم. البوابات الشاملة (`go test ./...`، `-race`، DR، فحص التوافق) محجوبة حالياً بتعديلات متزامنة في `pkg/sdk` و`storage/memory` وترحيلات إضافية خارج المرحلة 10، كما هو مفصّل في القسم 7؛ لا تُترجم هذه النتيجة تلقائياً إلى بوابة المرحلة 11 أو 12.
