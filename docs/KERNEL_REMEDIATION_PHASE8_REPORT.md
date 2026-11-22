# تقرير المرحلة 8 — التزامن والعزل متعدد الشركات

- **رمز الوثيقة:** `AE-REMED-P8-REPORT`
- **المرحلة:** 8 — التزامن والعزل متعدد الشركات
- **التاريخ:** 2026-11-22
- **خط الأساس المرجعي:** الالتزام `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **الخطة:** [`KERNEL_REMEDIATION_EXECUTION_PLAN.md`](./KERNEL_REMEDIATION_EXECUTION_PLAN.md) (قسم المرحلة 8، المهام 1–15)
- **القرار:** [`ADR P8-01`](./ard/P8_01_locking_and_isolation.md)
- **المخرجات:** [نموذج التزامن](./KERNEL_CONCURRENCY_MODEL.md) · [عقد القفل](./KERNEL_LOCKING_CONTRACT.md) · [تقرير العزل متعدد الشركات](./reports/multi_company_isolation_report.md)
- **النتيجة:** `PHASE 8 GATE: PASS`

> يُقرأ هذا التقرير مع العقود الثلاثة أعلاه. وجود دالة ترتيب في النواة لا يُعدّ قفلاً؛ العبرة بحجز فعلي مُرصَد في المحولين.

## 1. الهدف والنطاق

حوّلت المرحلة 8 عبارة «النواة ترتب مفاتيح القفل» من تعليق مضلل إلى عقد محايد قابل للرصد، وأثبتت أن `CommitPosting` يحجز فعلياً بالترتيب نفسه في `storage/memory` و`storage/postgres`، وأن طلبات الشركتين المتداخلة لا تتسرب عبر أي منفذ.

النطاق المنفّذ:

- `internal/kernal/posting/lock_manager.go` واختباراته.
- `pkg/spi/storage/locks.go` و`errors.go` و`transaction.go` واختباراتها.
- `storage/memory/posting.go` و`storage/postgres/uow.go` و`storage/postgres/posting.go`.
- `test/contract_parity/phase8_concurrency_isolation_test.go` و`storage/postgres/phase8_isolation_test.go`.
- `pkg/sdk/application.go` لتصنيف `ErrScopeViolation`.
- لا Auth/RBAC ولا RLS داخل `internal/kernal`؛ لا SQL ولا `pgx` في النواة.

## 2. تغطية المهام 1–15

| # | المهمة من الخطة | التنفيذ والدليل | الحالة |
| ---: | --- | --- | --- |
| 1 | توثيق مفاتيح الحسابات والقيود والأصول والمفاتيح | [عقد القفل](./KERNEL_LOCKING_CONTRACT.md)§2؛ `LockKind` أربع فئات؛ `PostingLockTargets` | PASS |
| 2 | تدقيق الترتيب وتجنب الحتمية الزائفة | `OrderLockTargets` يفرز سلسلة `Canonical()` ويزيل التكرار؛ `OrderedAccountKeys` يفرز مفاتيح الشركة | PASS |
| 3 | توثيق أن Lock Manager لا يحجز | تعليقات `lock_manager.go` وcharter `posting/doc.go` و[عقد القفل](./KERNEL_LOCKING_CONTRACT.md)§1 | PASS |
| 4 | فرض الترتيب في Adapter بعقد واختبار | `WithOrderedLocks` + `LockObserver`؛ `TestPhase8_LockOrderEnforced` على المحولين | PASS |
| 5 | ترحيل متزامن على نفس الحساب | `TestPhase8_SameAccountConcurrentPosting`: قيدان بمفتاحين مختلفين، كلاهما يُرحَّل | PASS |
| 6 | حسابات متقاطعة بترتيب عكسي | `TestPhase8_CrossAccountReverseOrderNoDeadlock`: أسطر معكوسة، مراقب مستقل لكل طلب، لا Deadlock | PASS |
| 7 | عشرات الطلبات لنفس القيد والمفتاح | `TestPhase8_ConcurrentReplayAtScale`: 16 منافساً، `EntryID` واحد، أثر 100 فقط | PASS |
| 8 | العكس المتزامن لنفس القيد | `TestPhase8_ConcurrentReversalSingleWinner`: 12 منافساً، عكس واحد، الأصل مرتبط مرة واحدة | PASS |
| 9 | تغيير الفترة/السياسة مع الترحيل | [سياسة اللقطة](./KERNEL_CONCURRENCY_MODEL.md)§4؛ `TestPhase8_FrozenSnapshotRemainsStableUnderConcurrentStateChange` | PASS |
| 10 | شركة A وB بمعرّفات متشابهة وتداخل | `TestPhase8_InterleavedCompanyIsolation`: قيدان متداخلان، سجلات وأرصدة منفصلة | PASS |
| 11 | Company Scope في كل Query/Command | `TestPhase8_CompanyScopeOnEveryPort` يغطيEntries/Idempotency/Balance/MasterData؛ لا fallback | PASS |
| 12 | RLS أو عزل فعلي في Adapter | `TestPhase8PostgresRLSRuntimeIsolation` بدور غير superuser؛ [تقرير العزل](./reports/multi_company_isolation_report.md)§4 | PASS |
| 13 | `go test -race` على الحزم والمحولات | `go test -race ./... -count=1` → PASS لكل الحزم | PASS |
| 14 | تصنيف Deadlock/Serialization/Unique/Scope | `storage.IsRetryable` وSQLSTATE translation و`ErrScopeViolation`؛ SDK يترجمه إلى `ErrForbidden` | PASS |
| 15 | سياسة retry محدودة في المضيف | `MaxAllowedRetries=5`، `RetryPolicy.Validate`، `ExecuteWithRetry`، لا retry داخل النواة أو المحول | PASS |

## 3. التغييرات الإنتاجية

| الملف | التغيير |
| --- | --- |
| `internal/kernal/posting/lock_manager.go` | تصحيح الادعاء الخاطئ؛ إضافة `AccountLockKey` و`OrderedAccountKeys` مع الإبقاء على `ExtractAndSortAccountIDs` للتوافق |
| `pkg/spi/storage/locks.go` *(جديد)* | `LockKind` و`LockTarget` و`Canonical()` و`OrderLockTargets` و`PostingLockTargets` و`LockObserver` و`WithOrderedLocks` و`ValidateIdempotencyScope` |
| `pkg/spi/storage/errors.go` | `ErrScopeViolation` و`ErrInvalidLockTarget` كأخطاء نهائية محايدة |
| `pkg/spi/storage/transaction.go` | `MaxAllowedRetries` و`RetryPolicy.Validate` وتحقق الحدود داخل `ExecuteWithRetry` |
| `pkg/spi/storage/doc.go` | charter المرحلة 8: عقد قفل محايد بلا محرك تخزين |
| `storage/memory/posting.go` | `acquireNamedLock` باستخدام `postingLocks` وقنوات حساسة للـ context؛ `CommitPosting` يلفّ `commitPostingLocked` بـ `WithOrderedLocks` ويتحقق من النطاق |
| `storage/postgres/uow.go` | `acquireOrderedAdvisoryLock` بمفتاح قانوني وsalt مستقل وتحقق من شركة المعاملة؛ `CommitPosting` يحجز قبل أي كتابة |
| `storage/postgres/posting.go` | تمرير `txCtx` الناتج عن `ExecuteWithOptions` إلى `CommitPosting` حتى يُنفَّذ القفل داخل المعاملة |
| `pkg/sdk/application.go` | تصنيف `storage.ErrScopeViolation` إلى `publicapplication.ErrForbidden` |
| `internal/kernal/posting/doc.go`, `pkg/spi/storage/doc.go`, traceability/invariants/charters | تحديث الحدود والمسؤوليات بإضافة المرحلة 8 |

لم تُضف هجرة قاعدة بيانات: الأقفال الاستشارية لا تغيّر المخطط. لم يُضف أي استيراد `database/sql` أو `pgx` أو `storage/*` إلى `internal/kernal`.

## 4. منهجية Test-First والأدلة الحمراء

1. كُتبت اختبارات النواة والـ SPI أولاً؛ ظهر الفشل المتوقع `undefined: posting.OrderedAccountKeys` و`undefined: storage.LockTarget` قبل التنفيذ، ثم تحوّلت إلى PASS.
2. كُتب `test/contract_parity/phase8_concurrency_isolation_test.go` قبل تعديل المحولات. الحالات الحمراء المرصودة:

| الحالة الحمراء قبل الإصلاح | السبب |
| --- | --- |
| `adapter did not report actual lock acquisitions to the observer` | المحولات لم تستدعِ `WithOrderedLocks`، فلم يوجد قفل فعلي يُرصَد |
| `cross-company scope error = <nil>, want ErrScopeViolation` | لم يكن نطاق idempotency مُتحقَّقاً منه في `CommitPosting` |
| `ordered lock requires an active transaction` | `Driver.CommitPosting` كان يمرّر `ctx` الأصلي لا `txCtx` إلى `uow.CommitPosting` |
| ترتيب acquisitions غير تصاعدي في اختبارات التزامن | مراقب واحد كان يتلقى آثار عدة طلبات؛ فُصل المراقب لكل طلب |
| `duplicate key ... uq_move_journal_number` | أرقام تسلسل متطابقة في بيانات الاختبار نفسها، وليست عيب قفل؛ خُصّت بأرقام distinct |

3. بعد الإصلاح: `go test ./test/contract_parity -run TestPhase8 -count=1` → `ok`، ثم `go test -race ./... -count=1` → PASS.

## 5. بوابات القبول

| البوابة | الدليل | النتيجة |
| --- | --- | --- |
| لا Deadlock تحت ترتيب عدائي | `TestPhase8_CrossAccountReverseOrderNoDeadlock` مع سياق محدد بـ 15s + مراقب ترتيب | PASS |
| لا ترحيل مكرر تحت التزامن | `TestPhase8_ConcurrentReplayAtScale` (16)، `TestPhase8_ConcurrentReversalSingleWinner` (12) | PASS |
| لا تسريب بين الشركات | `TestPhase8_InterleavedCompanyIsolation`، `TestPhase8_CompanyScopeOnEveryPort`، RLS runtime test | PASS |
| لا TOCTOU بلا سياسة لقطة | لقطة مجمدة + `TestPhase8_FrozenSnapshotRemainsStable...` + [النموذج](./KERNEL_CONCURRENCY_MODEL.md)§4 | PASS |
| Race detector نظيف | `go test -race ./... -count=1` | PASS |
| المحول يثبت القفل الفعلي والعزل | `LockObserver` على المحولين + RLS بدور غير superuser | PASS |

## 6. أوامر التحقق النهائية

| الأمر | النتيجة |
| --- | --- |
| `gofmt -w` على ملفات المرحلة 8 | لا ملفات غير منسقة |
| `go test ./internal/kernal/... -count=1` | PASS لجميع حزم النواة |
| `go test ./pkg/spi/storage/... -count=1` | PASS |
| `go test ./test/contract_parity -run TestPhase8 -count=1` | PASS |
| `go test ./... -count=1` | PASS لكل الحزم |
| `go test -race ./... -count=1` | PASS لكل الحزم |
| `./scripts/verify_architecture.sh` | `RESULT: PASS - clean architectural base` (11/11) |
| `./scripts/check_api_compat.sh` | `API compatibility check: passed` |
| `git diff --check` | نظيف |
| `docs/reports/contract_parity_report.md` | 42 passed, 0 failed, 0 skipped |

## 7. حالة الفجوات

| الفجوة | الحالة السابقة | الحالة بعد المرحلة 8 |
| --- | --- | --- |
| `G-002` — lock manager يرتب بلا قفل ووصفه مضلل | Open | **Closed — Phase 8**: عقد SPI + حجز فعلي في المحولين + مراقب + `go test -race` |
| `G-007`, `G-019`–`G-022` | Open | بدون تغيير؛ مالكوها خارج نطاق المرحلة 8 |

## 8. الحكم

`PHASE 8 GATE: PASS`. النواة تنتج ترتيباً حتمياً بلا قفل، والمحولات تحجز فعلياً داخل معاملاتها، واللقطة المجمدة تمنع TOCTOU، والعزل متعدد الشركات مُثبت على المحولين مع RLS وقت التشغيل في PostgreSQL. لا توجد بوابة معلّقة للمرحلة 8 داخل `internal/kernal`.
