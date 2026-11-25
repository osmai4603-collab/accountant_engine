# 02 — التقييم التفصيلي للمحاور الاثني عشر (Axes 5.1–5.13)

> المنهجية: تطبيق الخطوات الخمس في [دليل الأدلة والتقييم](../ACCOUNTING_KERNEL_QUALITY_DOCUMENTATION_v2.0.0/docs/quality/EVIDENCE_AND_SCORING_GUIDE.md) بفحص مسار التنفيذ الفعلي والاختبارات دون الاكتفاء بالأسماء أو الوثائق.

---

## 5.1 الصحة المحاسبية (`Accounting Correctness`) — الوزن: 20% | الدرجة: `8.50 / 10` | الدليل: `E3`

### نقاط القوة المثبتة

- **القيد المزدوج والتوازن (`ACC-001`, `ACC-013`):** دالة `ValidateEntryDoubleEntry` في [entry_validation.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/entry_validation.go#L123-L180) تفرض وجود سطرين على الأقل، ومدين ودائن موجبين، وتوحيد العملة الوظيفية، وتطابق مجموع المدين والدائن الوظيفي صفرياً (`net.IsZero()`).
- **سلامة السطر وإشارة المبلغ (`ACC-002`, `ACC-003`):** `ValidateEntryLine` في [entry_validation.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/entry_validation.go#L52-L120) ترفض المبالغ السالبة (`ErrNegativeAmountProhibited`)، وتمنع وجود مدين ودائن في نفس السطر (`ErrLineMustHaveOneSide`)، وترفض الأسطر الصفرية (`ErrZeroAmountLine`).
- **قابلية الترحيل (`ACC-004`):** `ResolvePostableAccountAsOf` في [coa.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/coa.go#L136-L173) تمنع الترحيل إلى الحسابات التجميعية (`HasAccountChildren`), غير النشطة، أو خارج نطاق الصلاحية الزمنية، وبقراءة واحدة تمنع TOCTOU.
- **إعادة بناء الأرصدة (`ACC-012`):** `RebuildAccountBalances` في [projection.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/ledger/projection.go#L191-L237) تعيد حساب الأرصدة حتمياً من الحركات المرحلة دون الاعتماد على رصيد مخزن قابل للعبث.

### الفجوات الحقيقية

1. **عدم ربط `NormalBalance` بنوع الحساب و`IsContraAccount`:** رغم وجود `AccountType` و`NormalBalance` و`IsContraAccount` في [types.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/types.go#L81-L105)، لا يوجد تحقق في النواة يمنع إنشاء حساب `asset` برصيد طبيعي `credit` دون تعليم `IsContraAccount = true`.
2. **غياب التحقق من المعادلة المحاسبية وإقفال الحسابات المؤقتة:** لا تحتوي النواة على خدمة مجال تتحقق من `Assets = Liabilities + Equity` عند الإقفال السنوي أو تولد قيود إقفال الإيرادات والمصروفات تلقائياً.

---

## 5.2 النموذج المجالي (`Domain Model / DDD`) — الوزن: 12% | الدرجة: `7.50 / 10` | الدليل: `E3`

### نقاط القوة المثبتة

- **Value Objects حقيقية:** `Money` في [money.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/types/money.go#L32-L35) غير قابل للتغيير (`amount`, `currency` حقول خاصة)، ومعرفات قوية النوع في [identifiers.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/types/identifiers.go).
- **خدمات المجال والمنافذ:** `PostingPipeline` في [pipeline.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/pipeline.go) يمثل Domain/Application orchestrator نظيفاً يعتمد على واجهات `masterdata.Reader` و`PostingExecutor`.
- **النسخ الدفاعي وعدم التغيير:** `JournalEntry.Clone()` و`ValidateImmutability` في [entry.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/journal/entry.go#L173-L225).

### الفجوة الحقيقية (حاسمة في `G02`)

- **نقص تغليف `JournalEntry` كـ Aggregate Root:** جميع حقول `JournalEntry` و`EntryLine` في [entry.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/journal/entry.go#L120-L170) مكشوفة (`Exported`)، ولا توجد طرق مجال مثل `entry.AddLine()`, `entry.RemoveLine()`, `entry.Post()`. أي كود داخل التطبيق يمكنه تعديل `entry.Status = journal.EntryPosted` أو تعديل `entry.Lines` مباشرة في الذاكرة متجاوزاً `NewJournalEntryAt` و`PostingPipeline`. هذا يخالف مبدأ المعيار (القسم 5.2 و14): *"Make invalid accounting states unrepresentable"*.

---

## 5.3 سلامة المعاملات (`Transaction Integrity`) — الوزن: 12% | الدرجة: `8.00 / 10` | الدليل: `E3` (E4 غير متحقق)

### نقاط القوة المثبتة

- **عقد `Idempotency` محكم (`ACC-007`):** `BuildIdempotencyScope` يفرض ثلاثي `(CompanyID, Operation, Key)` مع استبعاد تلاعب المستدعي، و`FingerprintRequest` / `FingerprintCallerPayload` في [transaction.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/transaction.go#L127-L250) تحسب بصمة SHA-256 حتمية لكافة الحقول والأسطر المرتبة.
- **تصنيف الأخطاء لإعادة المحاولة:** `RetryableError` و`IsRetryable` تميّز بوضوح بين أخطاء التزامن القابلة للإعادة وتعارضات الحمولة غير القابلة للإعادة.
- **حدود المعاملة:** `PostingPipeline.PostEntry` ينفذ جميع التحققات قبل استدعاء `PostingExecutor` أو `PostingReversalExecutor` مرة واحدة ذرية.
- **الاختبارات:** `test/atomic`, `test/acid`, `test/concurrency` ناجحة بالكامل ضد محول الذاكرة.

### الفجوات الحقيقية

1. **عدم التحقق في Target Environment (`E4`):** غياب `POSTGRES_URL` حال دون تشغيل اختبارات الذرية والتزامن والأقفال على PostgreSQL الفعلي في هذا التقييم.
2. **غياب عقد `Transactional Outbox` داخل النواة:** نشر الأحداث يقع في `internal/orbitals/events` و`pkg/spi/events` ولا يوجد عقد Outbox صريح مرتبط بـ `PostingExecutor` داخل `internal/kernal`.

---

## 5.4 دقة القياس المالي (`Monetary Precision`) — الوزن: 10% | الدرجة: `9.20 / 10` | الدليل: `E3`

### نقاط القوة المثبتة

- **تمثيل آمن وحظر `float`:** استخدام `shopspring/decimal` مع `InternalScale = 12`، وحد أقصى `28` خانة معنوية وصحيحة، وفحص AST آلي (`TestKernelFinancialCodeHasNoFloatConversions`) يمنع أي `float32`/`float64`/`NewFromFloat` في النواة واختباراتها.
- **كشف الفاقد الصامت:** `MinorUnitsExact()` يرفض الكسور الأقل من الوحدة الصغرى (`ErrSubMinorUnitAmount`)، و`RoundingDifference()` يحسب الفارق بدقة.
- **تسوية فروق التقريب:** `RoundEntryWithRoundingPolicy` في [rounding.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/rounding.go#L18-L159) يولّد سطر تسوية حتمياً على حساب التقريب المعتمد عند وجود بقايا.
- **دعم العملات و`ACC-013`:** جدول ISO-4217 كامل في [currency.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/types/currency.go)، وتحويل `ConvertForDate` وإعادة تقييم `revaluation.go` مع ربط `ExchangeRate` والعملة الوظيفية.

### الفجوات الطفيفة

- لا تزال دوال التوافق المتساهلة (`RoundForPosting`, `CurrencyMinorUnit`) موجودة ومصدرة في `types/money.go` (رغم حظر استخدامها داخل إنتاج النواة).
- لا توجد دالة توزيع نسبية (`Allocate(ratios)`) جاهزة في `types/money.go` لتوزيع المبالغ على عدة بنود مع حفظ الباقي.

---

## 5.5 دورة حياة العمليات (`Accounting Lifecycle`) — الوزن: 7% | الدرجة: `8.50 / 10` | الدليل: `E3`

### نقاط القوة المثبتة

- آلة حالات صريحة في [entry.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/journal/entry.go#L14-L97): `draft -> posted -> reversed`، تمنع تعديل المرحل أو إعادة ترحيله.
- بناء القيد العكسي في [reversal.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/posting/reversal.go#L25-L103) يفرض: القيد الأصلي مرحل، لم يُعكس سابقاً، وجود `ActorID` و`Reason`، تاريخ العكس ≥ تاريخ القيد الأصلي، قلب المدين والدائن، والحفاظ على الأبعاد.
- إدارة الفترات في [fiscal_calendar.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/masterdata/fiscal_calendar.go): حالات `Open -> SoftClosed -> HardClosed`، اشتراط `actorID` و`reason` لإعادة الفتح، ودعم الفترتين `13` و`14` للتسويات والتدقيق.

### الفجوات الحقيقية

- غياب حالة `Void` لإلغاء المسودات (`Draft`) بشكل موثق، وغياب نموذج `Partial Reversal` الصريح، وعدم الفصل بين `HardClosed` و`Locked`.

---

## 5.6 التدقيق وسلامة التاريخ (`Auditability & Historical Integrity`) — الوزن: 8% | الدرجة: `8.50 / 10` | الدليل: `E3`

### نقاط القوة المثبتة

- سلسلة تدقيق مشفرة بـ SHA-256 (`CalculateHash` v2) تغطي التسلسل والشركة والنوع والمعرف والعملية والمنفذ والسبب والتوقيت والحمولة والهاش السابق في [audit_chain.go](file:///home/osm/StudioProjects/cashflow_system/accountant_engine/internal/kernal/governance/audit_chain.go#L51-L69).
- التحقق مقابل نقطة تفتيش خارجية (`VerifyAuditIntegrityWithCheckpoint`) يفشل بإغلاق (`fail-closed`) عند غياب نقطة التفتيش أو تعارض اللقطة أو كسر السلسلة.
- حفظ `PolicyFingerprint` غير قابل للتغيير داخل القيد المرحل لضمان التتبع التاريخي للسياسات.

### الفجوة الحقيقية (حاسمة في `G06`)

- **عدم فرض `ACC-011` (سلامة مستند المصدر):** يمتلك `JournalEntry` حقلَي `SourceModule` و`ReferenceID`، لكن `PostingPipeline.PostEntry` لا يتحقق من وجودهما ولا يفرض سياسة صريحة للسبب البديل عند خلوهما (باستثناء `AdjustmentReason` للتسويات و`ReversalReason` للعكس).

---

## 5.7 الاستقلال المعماري (`Architectural Independence`) — الوزن: 10% | الدرجة: `9.80 / 10` | الدليل: `E3`

- **تحقق 100% عملياً:** فحص `go list` وسكربت `./scripts/verify_architecture.sh` (11/11 PASS) و`TestKernelArchitectureUsesPureInterfaces` تثبت خلو `internal/kernal/*` التام من أي استيراد لـ `database/sql`, `pgx`, `net/http`, `grpc`, `pkg/*`, `storage/*`, `internal/orbitals/*`, `modules/*`.
- النواة قابلة للتضمين بالكامل ولا تعتمد إلا على المكتبة القياسية و`shopspring/decimal` و`google/uuid`.

---

## 5.8 تعدد الدفاتر والعزل (`Multi-Ledger & Multi-Tenancy`) — الوزن: 5% | الدرجة: `4.50 / 10` | الدليل: `E3` (Company) / `E0` (Tenant & Ledger)

- **العزل الموجود:** عزل صارم على مستوى `CompanyID` في الحسابات واليوميات والفترات والتدقيق ومفتاح Idempotency.
- **الفجوة الجوهرية:**
  1. لا يوجد `TenantID` منفصل عن `CompanyID` داخل النواة.
  2. لا يوجد `LedgerID` لدعم تعدد الدفاتر (مثل General / Tax / Management Ledgers) بفترات ودليل حسابات وعملات وظيفية مستقلة لكل دفتر؛ `JournalID` يمثل اليومية فقط وليس دفتراً محاسبياً مستقلاً.

---

## 5.9 قابلية التوسع وحدود المجال (`Extensibility & Domain Boundaries`) — الوزن: 4% | الدرجة: `8.00 / 10` | الدليل: `E3`

- حدود النواة موثقة بوضوح (استبعاد RBAC والمصادقة وسير العمل الإداري).
- `PolicyEngine` يدعم إصدارات السياسات المؤرخة، و`EffectiveAccountReader` و`RateProviderSPI` تسمح بتوسيع المزودين.
- **ملاحظة:** الأبعاد التحليلية الخمسة (`Partner`, `CostCenter`, `Project`, `Segment`, `BusinessUnit`) ثابتة في بنية `EntryLine` وليست قابلة للتوسيع الديناميكي دون تعديل النواة.

---

## 5.10 الاختبارات والتحقق (`Testing & Verification`) — الوزن: 7% | الدرجة: `8.20 / 10` | الدليل: `E3`

- اختبارات النواة كاملة (`10` حزم) تعمل وتنجح تحت `-race` بتغطية ~`92.4%`، وتشمل اختبارات Fuzz (مثال `FuzzFXConversionNeverPanics`, `FuzzReversalNetImpactIsZeroPerAccount`) واختبارات خصائص وتزامن وتكافؤ عقود الذاكرة.
- **الفجوات:** فشل `test/dr` (`TestDRDrill_FullRecovery`: غياب `/tmp/opencode`)، وتخطي اختبارات PostgreSQL لغياب `POSTGRES_URL`.

---

## 5.11 الأداء وقابلية التوسع (`Performance & Scalability`) — الوزن: 3% | الدرجة: `5.00 / 10` | الدليل: `Not Verified (E4)`

- توجد قياسات أداء في `test/benchmark` وقد تم تشغيلها بنجاح (5 benchmarks).
- لكن اختبار `TestPostingLatencySLO` تم **تخطيه (`SKIP`)** لعدم توفر `POSTGRES_URL`، مما يجعل التحقق من SLO الفعلي غير مكتمل (`Not Verified`).

---

## 5.12 جودة التنفيذ والصيانة (`Maintainability & Engineering Quality`) — الوزن: 2% | الدرجة: `9.00 / 10` | الدليل: `E3`

- كود Go نظيف، حزمة `apperrors.CodedError` واضحة، وثائق داخلية متكاملة (`KERNEL_ACCOUNTING_INVARIANTS.md`, `KERNEL_TRACEABILITY_MATRIX.md`, `KERNEL_COVERAGE_EXCEPTIONS.md`, `KERNEL_MUTATION_REPORT.md`) مع اختبارات آلية تتحقق من تطابقها.
