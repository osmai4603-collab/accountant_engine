# الخطة التنفيذية الموحدة لاستكمال النواة المحاسبية `internal/kernal` — Kernel v10

- **اسم الخطة:** Unified Kernel Final Hardening & Scope-Complete Execution Plan
- **رمز الوثيقة:** `AE-PLAN-KERNEL-V10-UNIFIED`
- **الإصدار المستهدف:** Kernel v10 حتى بوابة الاعتماد النهائي
- **خط الأساس:** `accountant_engine9.zip` (الإصدار التاسع)، مع مراجعة الإصدار الثامن كمرجع تكافؤ سلوكي
- **تاريخ الإعداد:** 2026-11-21
- **الحالة:** تم التنفيذ والاعتماد النهائي بنجاح (IMPLEMENTED & VERIFIED) — تم اجتياز جميع الاختبارات (Unit, Race, Shuffle, Fuzz) داخل `internal/kernal/` بنسبة 100% وفق البوابات A–H ومستند الإنجاز `internal/kernal/KERNEL_COMPLETION_REPORT.md`
- **النطاق البرمجي الملزم:** `internal/kernal/` فقط
- **المرجع الحاكم:** هذه الوثيقة هي المرجع الوحيد للتنفيذ. أي تعارض مع وثيقة أخرى يُحسم وفق القسم 3 أدناه.

---

## 0. لماذا هذه الوثيقة وكيف تُستخدم

وُجدت خطتان سابقتان في `docs/`:

1. `KERNEL_FINAL_EXECUTION_PLAN_V10.md` — الخطة (أ): «مقترحة»، شاملة، صارمة الإثبات، متسقة مع معمارية الهدف.
2. `KERNEL_V10_MASTER_EXECUTION_PLAN.md` — الخطة (ب): «معتمدة للتطبيق الفوري»، مختصرة، عملية، لكنها تحمل قرار GL مخالفاً لوثائق المشروع ومنهجية إثبات أضعف.

سُجّلت المقارنة والتقييم في `docs/KERNEL_V10_PLANS_COMPARISON_AND_EVALUATION.md`. الخلاصة: **(أ) هي الأفضل كمرجع حاكم**، و**(ب) تُستخدم كملحق تقني مُنقَّح لا كمرجع معماري**.

هذه الوثيقة تجمع الاثنين في مرجع واحد:

- **من (أ):** البنية، القرارات، حدود المسؤولية، مصفوفة التتبع، مصفوفات الاختبارات، بوابات الإصدار، متطلبات الإثبات.
- **من (ب):** مقتطفات الكود العملية، ملاحظة أداة Go، تنبيه `AccountTypeRevenue`، فكرة التوافق القرائي لقيمة `audit` — بعد تقييدها بالشروط الواردة في القسم 3.

**قاعدة استخدام:** لا يقرأ المنفّذ الخطتين السابقتين بوصفهما أمرين متنافسين؛ يقرأ هذه الوثيقة، ويرجع إلى (أ) عند الحاجة إلى تفصيل لم يُختصر هنا، وإلى (ب) فقط عند الحاجة إلى مقتطف كود مُعتمد في الملحق ب.

---

## 1. الغرض ومعنى الاكتمال

الهدف: نواة محاسبية مكتملة **ضمن مسؤولياتها المعتمدة**، بحيث تكون كل قاعدة:

1. محددة بعبارة غير ملتبسة.
2. مرتبطة بموضع تنفيذ داخل `internal/kernal`.
3. مرتبطة باختبار يثبت القبول والرفض والحالات الحدية.
4. متوافقة مع عقود Kernel والمضيف.
5. موثّقة بنتيجة تشغيل فعلية، لا بمجرد وجود ملف أو دالة أو اسم اختبار.

### 1.1 تعريف 100%

تعني 100% اكتمال المتطلبات المعتمدة **داخل نطاق Kernel**، ولا تعني:

- تغطية أسطر 100% بالضرورة.
- اعتماداً محاسبياً أو قانونياً خارجياً.
- إثبات ذرية قاعدة بيانات لا تنفذها النواة.
- اكتمال الوحدات المدارية (Orbitals) أو وحدات المعايير.

لا يُعلن 10/10 قبل اجتياز بوابة القبول (القسم 10)، مع الإفصاح المنفصل عن اعتمادات المضيف.

---

## 2. القيود غير القابلة للتفاوض

### 2.1 حصر النطاق

يُسمح بالتعديل داخل `internal/kernal/` فقط. **يُمنع** تعديل أي ملف خارجها، بما في ذلك:

```text
pkg/  storage/  internal/orbitals/  internal/events/  internal/reconciliation/
modules/  test/  scripts/  docs/ (جذر المشروع)  go.mod  go.sum  CI/workflows
```

يُسمح بقراءة الملفات الخارجية وتشغيل اختباراتها كأدلة توافق فقط. إذا تطلّب إثبات تكامل تغييراً خارجياً:

1. وثّق الاعتماد الخارجي.
2. اشرح لماذا لا يمكن إثباته من Kernel وحده.
3. أضف اختبار عقد داخلي أو fake يثبت جانب Kernel.
4. سجّله كاعتماد خارجي في التقرير النهائي.
5. **لا تُنفّذ التعديل الخارجي.**

> هذه الوثيقة نفسها كُتبت في `docs/` بأمر صريح من مالك المشروع، وهي استثناء توثيقي واحد لا يُعمّم على مهام التنفيذ.

### 2.2 ما هو مستبعد صراحة

لا تُضف ولا تُعاد تسمية أي مما يلي داخل Kernel:

- RBAC، الأدوار، الصلاحيات، التفويض، المصادقة، الجلسات، Maker-Checker، موافقات Workflow.
- HTTP/REST/gRPC أو أي طبقة نقل.
- SQL أو PostgreSQL أو محولات التخزين.
- SDK أو API أو خدمات التطبيق.
- تقارير القوائم، الدفاتر الفرعية، الخزينة، المطابقة، التوحيد، وحدات IFRS/IAS.

تظل `ActorID` و`CreatedBy` و`PostedBy` و`Reason` بيانات إسناد وتدقيق فقط، ولا تُستخدم لإثبات صلاحية.

### 2.3 حدود المسؤولية Kernel / المضيف

| المسؤولية | Kernel | المضيف / Executor |
|---|---|---|
| صحة القيد المزدوج | يفرض التحقق | لا يعيد تفسير القاعدة |
| تطبيع المبالغ والتقريب | ينفذ ويصدر قيداً مطبعاً | يحفظ المخرجات كما استلمها |
| أهلية الحساب واليومية والفترة | يتحقق من البيانات والسياسات المقدمة | يوفر بيانات متسقة وحديثة |
| إنشاء القيد العكسي | ينشئ Draft ويحدد بياناته | يربط الأصل بالعكس ذرياً ويمنع التكرار |
| قرار الترحيل | يمر عبر `PostingPipeline` | لا يتجاوز المسار المعتمد |
| ذرية الحفظ والقفل | يحدد عقد المنفذ | ينفذ المعاملة والقفل والتراجع |
| Idempotency | يفرض وجود المفتاح وشكل العقد | ينفذ replay/conflict ذرياً ويحفظ النتيجة |
| سجل التدقيق | يبني الهاش ويتحقق من السلسلة واللقطة | يوفر append ذرياً ونقطة تحقق خارجية |
| المصادقة والتفويض | خارج النواة | مسؤولية المضيف بالكامل |

**تنبيه اعتماد:** نجاح Kernel لا يثبت وحده ACID أو RLS أو منع التكرار أو حفظ نقطة التحقق خارجياً. تُذكر هذه كاعتمادات مضيف منفصلة.

---

## 3. حسم التعارضات بين الخطتين — قسم ملزم

هذا هو جوهر الدمج. البنود أدناه محسومة ولا يجوز للمنفذ إعادة اجتهادها.

| # | موضع التعارض | الخطة (أ) | الخطة (ب) | **القرار الموحّد** |
|---|---|---|---|---|
| 1 | الأستاذ العام داخل Kernel | `DEC-05`: لا GL ولا Trial Balance؛ توثيق الحدود في `ledger/doc.go` | الاحتمال (أ): `AccountBalance` + `BalanceCalculator` + `TrialBalanceVerification` داخل Kernel | **يُرفض احتمال (أ) من الخطة (ب) قطعياً.** `TARGET_ACCOUNTING_KERNAL.md` يحصر النواة في سبع حزم ويضع ميزان المراجعة والقوائم في `internal/reporting`. يُعتمد DEC-05. أي توسع لاحق يحتاج قرار معماري منفصل وتعديل وثيقة الهدف أولاً، وهو خارج هذه المهمة. |
| 2 | حذف حزمة `ledger` | تُبقى الأنواع وتُوثّق | الاحتمال (ب) يقترح الحذف | **يُرفض الحذف:** `pkg/api/v1/ledger` يستخدم أنواعها مرجعياً. تُبقى الحقول العامة كما هي. |
| 3 | سقف المنازل العشرية | 28 رقماً ذا دلالة + `InternalScale=12`، وإظهار تعارض 12/13 واشتراط قرار DEC-03 | `MaximumRepresentationalScale = 18` | **يُرفض الرقم 18.** يُعتمد DEC-03: تُكتب الأسماء `MaximumRepresentationalScale` و`MaximumIntegerDigits` وتُحدَّد قيمتها بقرار مالك القاعدة **قبل كتابة الكود**. |
| 4 | حماية `MarshalJSON` | رفض صريح في المنشئ، ولا فقدان قيمة صامت | `if scale > max { scale = max }` | **يُرفض القصّ في الإخراج:** تغيير القيمة أثناء التمثيل فقدان صامت. التحقق يحدث في `NewMoney` و`NewMoneyFromString` و`UnmarshalJSON` عبر مسار واحد. |
| 5 | قيمة `AdjustingEntryAudit` | DEC-04: المحافظة على `audit_adjustment`، وخطة انتقال عند التغيير | إعادة القيمة + قبول `audit` وإعادة تسميته | **يُعتمد مزيج مقيد:** الكتابة الموحّدة `audit_adjustment`؛ القراءة تقبل `audit` لأغراض التوافق فقط، مع توثيق صريح لخطة انتقال. لا تُخزَّن القيمة القديمة كبديل صامت. |
| 6 | تأشيرات القبول | يُمنع إعلان الاكتمال بلا نتائج تشغيل | جدول الـ 12 شرطاً مؤشَّر ✅ «مستوفى 100%» قبل التنفيذ | **تُحوَّل جميع التأشيرات إلى «هدف مستهدف»** ولا تُرفع إلى «مستوفى» إلا بنتيجة تشغيل محفوظة. |
| 7 | عدد التراجعات | 15 فجوة `BASE-01..15` | «7 تراجعات» لكنها تسرد 8 عناصر | يُعتمد جرد (أ)؛ ويُوحَّد العدد في أي تقرير لاحق. |
| 8 | IdempotencyKey | BASE-09 / المرحلة 5: إلزامي داخل Kernel | غائب | **يُضاف صراحةً** كبند P0 (`ErrIdempotencyKeyRequired`). |

---

## 4. القرارات المعتمدة (تعديلاً على DEC-01..06 من الخطة أ)

- **DEC-01 — حدود التغيير:** كما في (أ). كل التعديلات داخل `internal/kernal/`.
- **DEC-02 — Audit Snapshot:** كما في (أ) كاملاً: لقطة منطقية واحدة، `ErrAuditSnapshotConflict`، إعادة محاولة bounded (القيمة الحالية 3)، `ErrAuditSnapshotConsistencyRetryable` عند الاستنفاد، وعدم إعادة المحاولة لإخفاء hash mismatch، وعدم قبول السلسلة المقتطعة دون Anchor موثق.
- **DEC-03 — دقة Money:** كما في (أ) مع الحسم المذكور في القسم 3 (بند 3 و4). `FinancialScale = 4` و`InternalScale = 12` و`MaximumSignificantDigits = 28` قيم مبدئية موثقة؛ أما `MaximumRepresentationalScale` و`MaximumIntegerDigits` فيُقرَّران قبل التنفيذ. يجب وضع حد للأس الموجب أيضاً (`1e999999`) لا للأس السالب فقط.
- **DEC-04 — قيمة Audit:** وفق القسم 3 (بند 5) أعلاه.
- **DEC-05 — حدود Ledger:** كما في (أ)، مع إلحاق تنبيه `AccountTypeRevenue = "income"` في `ledger/account.go` أثناء التوثيق: يُعامل كمرادف موثق لـ `AccountTypeIncome` أو يُفصل بقرار توافق؛ لا يُغيَّر بصمت.
- **DEC-06 — نوع إعادة التقييم:** لا يُضاف `AdjustingEntryRevaluation` قبل قرار محاسبي مستقل يبيّن سبب عدم كفاية `AdjustingEntryAccrual`.
- **DEC-07 — المرجعية عند التعارض:** هذه الوثيقة هي المرجع الوحيد للتنفيذ؛ والقسم 3 هو الفاصل لأي خلاف مع (أ) أو (ب).

### 4.1 حالات المهام

استخدم: `NOT_STARTED`، `IN_PROGRESS`، `IMPLEMENTED`، `TESTED`، `VERIFIED`، `BLOCKED_EXTERNAL`، `DEFERRED_BY_DECISION`، `REJECTED`. **لا تستخدم `DONE` وحدها.**

---

## 5. خط الأساس والفجوات المعتمدة

| المعرف | الموضع | الفجوة | المرحلة |
|---|---|---|---|
| BASE-01 | `masterdata/fiscal_calendar.go` | يوم البداية يُقسر إلى 1 | 2 |
| BASE-02 | `posting/pipeline.go` | `time.Now()` أكثر من مرة | 3 |
| BASE-03 | `posting/entry_validation.go` | `NewJournalEntry` يقرأ الوقت مباشرة | 3 |
| BASE-04 | `posting/reversal.go` | `BuildReversalEntry` كذلك | 3 |
| BASE-05 | `valuation/revaluation.go` | قيد إعادة التقييم بوقت مباشر مرتين | 3 |
| BASE-06 | `governance/audit_chain.go` | `Append` بلا مسار وقت قابل للحقن | 3 و9 |
| BASE-07 | `types/money.go` | لا حد للمقياس/الأس/حجم التمثيل | 4 |
| BASE-08 | `journal/entry.go` | `audit` بدل `audit_adjustment` | 6 |
| BASE-09 | `posting/pipeline.go` | IdempotencyKey غير مفروض | 5 |
| BASE-10 | `internal/kernal/` | غياب وثيقة الثوابت | 1 |
| BASE-11 | اختبارات التقويم | فقدان يوم البداية وFuzz | 2 و11 |
| BASE-12 | حزم متعددة | فقدان اختبارات هوية/أنواع/ledger/Adversarial | 11 |
| BASE-13 | `ledger/` | نماذج فقط، حدود غير محسومة | 10 |
| BASE-14 | `valuation/revaluation.go` | نوع `AdjustingEntryAccrual` يحتاج حسم دلالة | 6 و8 |
| BASE-15 | `governance/audit_chain.go` | عقد snapshot repository يحتاج توثيقاً | 9 |

**قاعدة منع الانحدار:** لا يُحذف أي اختبار قائم إلا ببديل يثبت السلوك نفسه، ويُسجَّل في مصفوفة التتبع. لا تُستعاد الاختبارات آلياً دون مراجعة.

---

## 6. مراحل التنفيذ

### المرحلة 0 — تجميد خط الأساس وحماية النطاق (P0)

1. جرد كل ملفات `internal/kernal` وتصنيفها: source / tests / docs / testdata.
2. إنشاء `KERNEL_RELEASE_BASELINE.md` داخل Kernel بمقارنة v8 مقابل v9، وتسجيل الملفات المفقودة دون استعادة فورية.
3. فحص كل imports داخل Kernel: لا `pkg/` ولا `storage/` ولا `orbitals/` ولا `modules/` ولا `database/sql` ولا `net/http` ولا `net` ولا `grpc`.
4. فحص الحقول العامة والأنواع النصية ذات الطابع التعاقدي.
5. قاعدة مراجعة: لا حذف اختبار بلا بديل موثق.
6. لا تُعدَّل `go.mod` أو `go.sum`.

**الإغلاق:** جرد مطابق للواقع، صفر import محظور، صفر تعديل خارج Kernel.

---

### المرحلة 1 — الثوابت ومصفوفة التتبع (P0)

1. استعادة `KERNEL_ACCOUNTING_INVARIANTS.md` من v8 كأساس لا كنسخة عمياء، مع مراجعة كل نص مقابل الكود.
2. إنشاء `KERNEL_TRACEABILITY_MATRIX.md`: لكل قاعدة `Rule ID`، نص المتطلب، الحزمة/الملف، الدالة، اختبار قبول، اختبار رفض، الحالات الحدية، اعتماد المضيف، حالة التنفيذ الفعلية.
3. الحالات المسموحة: `Implemented`، `Partially Implemented`، `Missing`، `Host Responsibility`، `Out of Kernel`. لا توجد حالة `Unknown`.
4. تمييز قواعد Kernel عن `ACC-028..040` (Orbitals/Subledgers/Standards) وربط `ACC-041..043` بحزمة `governance` بالقدر المحدد.
5. لا تُكتب «مكتمل» قبل اختبار ناجح.

**الإغلاق:** كل قاعدة لها مالك برمجي واختبار، ومسؤوليات المضيف مميزة.

---

### المرحلة 2 — التقويم المالي (P0)

**الملفات:** `masterdata/fiscal_calendar.go`، `fiscal_calendar_test.go`، `fiscal_calendar_fuzz_test.go`، واختبار اتساق فقط على `ledger/period.go`.

1. إزالة `start = time.Date(..., 1, ...)`.
2. طبّع إلى UTC مع الحفاظ على اليوم التقويمي المقصود وتصفير الساعات.
3. استعد `addFiscalMonths(start, n)` بحيث تُحسب كل حد من تاريخ البداية الأصلي، مع clamp إلى آخر يوم صالح في الشهر القصير.

**مقتطف معتمد من الخطة (ب) — يُراجع وفق اختبارات (أ):**

```go
func addFiscalMonths(start time.Time, months int) time.Time {
    start = start.UTC()
    targetYear, targetMonth := start.Year(), int(start.Month())-1+months
    targetYear += targetMonth / 12
    targetMonth %= 12
    if targetMonth < 0 {
        targetYear--
        targetMonth += 12
    }
    month := time.Month(targetMonth + 1)
    lastDay := time.Date(targetYear, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
    day := start.Day()
    if day > lastDay {
        day = lastDay
    }
    return time.Date(targetYear, month, day, 0, 0, 0, 0, time.UTC)
}
```

4. ثبّت اصطلاح الحدود: نهاية الفترة = `addFiscalMonths(start, i).Add(-time.Nanosecond)` ونهاية السنة = `addFiscalMonths(start, 12).Add(-time.Nanosecond)`. **لا تخلط مع تمثيل نصف مفتوح `[start,end)`** إلا بقرار عام جديد موثق.
5. الفترتان 13 و14 إجرائيتان: نفسيهما `FiscalYearID` و`CompanyID`، ولا تختارهما `Contains(date)` أبداً. 13 = `YearEndAdjustment`، 14 = `AuditAdjustment`.
6. أضف Validator للسنة المولّدة: 12 فترة عادية بالترتيب، داخل حدود السنة، متصلة بلا فجوة ولا تداخل.
7. افصل التحقق من صلاحية بيانات الفترة المسترجعة عن التحقق من التاريخ؛ لا تُثق بـ Repository دون تحقق دفاعي.
8. احتفظ بسلوك `SoftClosed` وفق `PostingPolicy.AllowSoftClosedPosting` فقط.

**مصفوفة الاختبارات الإلزامية:** 1 يناير؛ 15 مارس؛ 31 يناير (بلا انجراف تراكمي)؛ فبراير كبيس وغير كبيس؛ بداية 30/31 مع clamp؛ تاريخ قبل/بعد السنة؛ نهاية فترة؛ `Contains` للفترتين 13/14 = false؛ فجوة/تداخل/انتماء خاطئ يرفضه Validator.

**Fuzz:** `FuzzGenerateFiscalYearHasContiguousPeriods` (أو الاسم الفعلي بعد الإنشاء) بمدخلات bounded تثبت الاتصال والاحتواء no-gap/no-overlap.

---

### المرحلة 3 — الحتمية الزمنية (P0)

**الملفات:** `posting/pipeline.go`، `entry_validation.go`، `reversal.go`، `valuation/revaluation.go`، `governance/audit_chain.go`.

1. استعد واجهة `Clock` و`WithClock`. **مقتطف معتمد من (ب):**

```go
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
```

2. عالج `Clock` nil بسلوك موثق (افتراضي `systemClock{}`) ولا تدع `PostEntry` causes panic.
3. التقط لحظة واحدة: `now := p.clock.Now().UTC()`، ومرّرها إلى `ValidatePostingPolicy` و`PostingDate` و`PostedAt` وكل حقل مشتق.
4. استعد `NewJournalEntryAt` و`BuildReversalEntryAt` و`GenerateRevaluationEntryAt`؛ دوال الراحة تُفوّض إليها بساعة النظام.
5. أضف `AppendAt` أو clock option محلي في `governance` **دون استيراد `posting`** لتجنب دورة التبعيات.
6. يستعمل مسار الترحيل `ValidateCanPostToAccountAsOf` بالوقت المحقون، لا wrapper `time.Now()`.
7. `time.Now()` مسموح فقط داخل `systemClock` وwrappers العامة.
8. وثّق: `EffectiveDate` = تاريخ الحدث، `PostingDate` = وقت الترحيل؛ لا يُستبدل أحدهما بالآخر.

**الاختبارات:** ساعة ثابتة ⇒ `PostingDate == PostedAt`؛ السياسة والترحيل يستخدمان اللحظة ذاتها؛ backdating قبل/بعد الحد؛ ثبات timestamps عند البناء بوقت صريح؛ صفر فروق nanoseconds؛ UTC وحدود اليوم.

---

### المرحلة 4 — Money & Precision (P0)

**الثوابت:** `FinancialScale = 4`, `InternalScale = 12`, `MaximumSignificantDigits = 28`، plus `MaximumRepresentationalScale` و`MaximumIntegerDigits` وفق DEC-03 (قرار قبل الكود).

1. مسار تحقق واحد في `NewMoney` و`NewMoneyFromString` و`UnmarshalJSON`.
2. ارفض `NaN`/`Infinity` و`XXX` والعملات غير المعروفة، وأضف حالات exponent موجبة وسالبة متطرفة.
3. أضف `ExactString()`؛ ووثّق أن `String()` للعرض فقط ولا تُستخدم في مقارنة أو hash أو persistence.
4. لا تغيّر توقيع `RoundForPosting()` دون بديل متوافق؛ أضف دالة strict جديدة للاستخدام الداخلي إن لزم.
5. راجع `CurrencyMinorUnit` وحدود `int64` عند تحويل minor units.
6. تحقق JSON round-trip لا يُغيّر القيمة ولا يُحوّل غير الصالح إلى صالح.
7. صفر استخدام لـ `float32`/`float64` في المال.

**جدول الاختبارات:** sign؛ significant digits عند الحد/+1؛ scale 0/2/3/12/13/ضخم؛ integer magnitude؛ USD/JPY/KWD/YER/XXX/ZZZ؛ half-even؛ minor units exact/sub/overflow؛ JSON round-trip/null/invalid/exponent attack؛ Add/Subtract/Multiply/Divide وoverflow.

**Properties/Fuzz:** لا panic في `String` و`ExactString` و`Rounded` و`MinorUnits` و`RoundingDifference` و`MarshalJSON` لأي قيمة يقبلها المنشئ؛ round-trip = Equal؛ `MinorUnitsExact` يرفض الكسر الأصغر من الوحدة؛ Add/Subtract ترفض اختلاف العملة.

---

### المرحلة 5 — Posting Pipeline وIdempotency (P0)

**ترتيب ثابت موثق داخل `PostEntry`:**

1. هوية الرأس والأسطر. 2. التصنيف والبيانات التابعة. 3. حالة Draft. 4. `JournalID` و`CompanyID` و`ActorID` و`EffectiveDate`. 5. `Clone()` قبل التطبيع. 6. تحميل الشركة والتحقق. 7. تحميلPolicy النافذة بتاريخ القيد. 8. العملات وسعر الصرف. 9. اليومية والصلاحيات. 10. حل الفترة وقواعد 13/14/Opening/Closing. 11. التطبيع وإعادة الحساب والتسوية. 12. تحقق مزدوج نهائي. 13. الحسابات والأبعاد. 14. سياسة الترحيل. 15. `PostingDate`/`PostedAt` من Clock واحد. 16. تسليم snapshot إلى Executor.

**Idempotency (BASE-09، P0):**

- أضف `ErrIdempotencyKeyRequired` وافرض `strings.TrimSpace(opts.IdempotencyKey) != ""` وفق `ENG-008`.
- Kernel يفرض الوجود والشكل فقط؛ replay/conflict وحجز المفتاح ذرياً مسؤولية Executor.
- وثّق صراحةً أن Kernel لا يملك مخزن حالة ولا يُرجع نتيجة سابقة.

**قواعد إضافية:** احتفظ بفصل `PostingExecutor` و`PostingReversalExecutor`؛ خطأ واضح عند غياب PolicyEngine/Reader/Executor بلا panic؛ لا تُعدَّل المسودة الأصلية؛ حدّد معنى `TargetPeriodNumber` (خصوصاً 0 و1 و13 و14) بمصفوفة `Class × TargetPeriodNumber`؛ تحقق تطابق الفترة المحلولة مع القيمة الصريحة؛ تحقق أهلية سطر التسوية المضاف؛ نفس العملة ⇒ `ExchangeRate = 1`؛ احسم relationship بين رأس `TransactionCurrency`/`ExchangeRate` والأسطر؛ لا تدّع Kernel تنفيذ ACID.

**اختبارات الرفض:** ID صفر/مكرر، أقل من سطرين، طرف واحد، سطر ذو وجهين، مبالغ صفر/سالبة، عملات غير مطابقة، يومية/حساب/بعد غير صالح، فترات مغلقة، TargetPeriod غير مطابق، IdempotencyKey فارغ/مسافات، غياب التبعيات، وعدم تغيير المسودة عند أي فشل.

**عقود Executor بـ fakes:** لا يُستدعى عند الفشل؛ يُستدعى مرة عند النجاح؛ يستقبل قيداً مطبعاً متوازناً بمفتاح غير فارغ؛ reversal لا يذهب للـ executor العادي؛ الخطأ يُعاد كما هو.

---

### المرحلة 6 — القيود العكسية والتسويات (P0)

1. ثبّت القيم النصية للتصنيفات والأنواع كعقد بيانات؛ `AdjustingEntryAudit = "audit_adjustment"` للكتابة، وقراءة `audit` للتوافق وفق DEC-04 المقيّد.
2. تصنيف فارغ أو غير معروف مرفوض. Adjusting ⇒ نوع وسبب بعد Trim؛ Audit فقط في 14؛ الأنواع العادية مرفوضة في 14؛ 13 لا يقبل Audit.
3. مصفوفة كاملة للفترات 1–12 و13 و14 × كل التصنيفات والأنواع.
4. `SummarizeAdjustingEntriesForPreClose`: يفحص Drafts والبيانات الوصفية ونوع التسوية والفترة المستهدفة، ولا يصف ملخصاً جاهزاً مع مخالفة؛ يوثّق أن الدالة لا تثبت `FiscalPeriodID` دون بيانات مرجعية (أو يضيف دالة pure تأخذ بيانات الفترات صراحةً).
5. `BuildReversalEntry`: الأصل Posted وغير معكوس؛ actor وreason غير فارغين؛ تاريخ العكس لا يسبق الأصل؛ العكس Draft؛ لا يرث `FiscalPeriodID`؛ تبديل Debit/Credit مع حفظ الأبعاد والعملات والمبالغ؛ `ReversalOfID` صحيح وغير ذاتي.
6. عقد `PostingReversalExecutor` يشترط ذرية الربط ومنع العكس المزدوج المتزامن؛ النواة لا تدّعي تنفيذ القفل وحدها.
7. لا تُضاف `AdjustingEntryRevaluation` قبل DEC-06.

---

### المرحلة 7 — Master Data وPolicy (P1)

- **Account/COA:** تحقق IDs والاسم والكود والعملة والطبيعة؛ فصل التطبيع عن `Validate()`؛ EffectiveFrom/To بكل التركيبات؛ NormalBalance للعادي وContra؛ منع الترحيل على Header/Control؛ `RequiresCostCenter` على مستوى الحساب؛ SubClassification حسب النوع؛ كشف الدورات والأب المفقود/الخارج عن الشركة/غير الصالح وتكرار ID؛ لا تحويل صامت لأب مفقود إلى Root إلا بعقد صريح للمدخلات الجزئية.
- **Company/Journal:** تطابق ID المسترجع مع المطلوب؛ FunctionalCurrency معروفة وغير `XXX`؛ `PresentationCurrency` ليست عملة ترحيل؛ IDs والكود والاسم والتواريخ؛ حسم دلالة القوائم الفارغة في `AllowedCurrencies`/`AllowedAccountIDs` بشكل متطابق في `Validate` و`Allows...` وPipeline؛ نسخ مستقل للشرائح.
- **Cost Center/Dimensions:** الملكية والنشاط والتاريخ وParent/Child بلا دورات؛ رفض UUID صفري مؤشر؛ منع التكرار إن منعت القواعد ذلك.
- **Policies:** غياب Policy نافذة يمنع الترحيل؛ لا policy لشركة أخرى؛ لا EffectiveDate صفر؛ أحدث سياسة نافذة؛ منع تكرار تاريخ النفاذ؛ نسخ `AllowedCurrencies`؛ تطابق FunctionalCurrency مع Company؛ أهلية `RoundingDifferenceAccountID` مع AutoSettle؛ حدود Backdating والتوقيت؛ SoftClosed فقط بسماح السياسة وHardClosed مرفوض مطلقاً؛ صفر حقول صلاحية/موافقة.

---

### المرحلة 8 — Valuation وFX داخل حدود النواة (P1)

- تحقق العملات والزوج والنوع والتاريخ والسعر الموجب؛ رفض `XXX`؛ توثيق دقة مفتاح التكرار (اليوم UTC حالياً).
- بحث مباشر وعكسي؛ سعر العملة إلى نفسها = 1 حتى مع Provider خارجي؛ `Convert` يرفض سعراً غير 1 عند التساوي؛ أحدث سعر لا يتجاوز تاريخ الطلب؛ `FXResult.RateDate` = تاريخ نفاذ السعر لا تاريخ التقييم.
- Revaluation: لا تعيد تقييم NonMonetary بـ Closing Rate؛ تحقق IDs والعملات والرصيد والقيمة الدفترية؛ **قرار محاسبي مطلوب** بخصوص الأرصدة السالبة/المراكز الدائنة؛ الحالات الأربع Asset/Liability × Gain/Loss؛ مرجع السعر بعد inversion؛ فرق صفر ⇒ لا قيد؛ القيد Draft ومتوازن؛ `GenerateRevaluationEntryAt`؛ لا ادعاء بتطبيق IAS 21 كامل.

---

### المرحلة 9 — Governance وAudit Snapshot (P1)

**Append Contract:** المستودع يخصص التسلسل والهاش السابق ذرياً، يثبّت الطابع والبيانات، يحسب `CurrentLogHash` بعد تثبيت كل الحقول، يضيف دون تعديل السابق، يحدّث checkpoint، ويمنع سباق appendين. حدّد ما إذا كان `AuditLog.ID` مستبعداً من الهاش ووثّقه؛ كل الحقول الدلالية (Reason وPayload وTimestamp وCompanyID وActorID وCorrelationID وAction وEntity metadata وSequenceNumber وPreviousLogHash) تدخل الهاش.

**Snapshot Contract:** companyID مطابقة؛ logs وcheckpoint من لقطة واحدة؛ ترتيب تصاعدي بلا تكرار أو فجوة؛ `SnapshotSequence` = حد اللقطة؛ تحقق version/sequence/hash؛ تحقق كل الهاشات حتى الحد؛ تطابق الهاش النهائي مع checkpoint؛ `ErrAuditSnapshotConflict` عند تغيّر الحدود أثناء الالتقاط؛ إعادة المحاولة bounded للالتقاط فقط؛ `ctx.Err()` يوقفها؛ استنفاد المحاولات ⇒ `ErrAuditSnapshotConsistencyRetryable` لا `ErrAuditChainBroken`؛ لا تقبل سلسلة مقتطعة دون Anchor؛ لا تدّع الكشف عن حذف الذيل بلا checkpoint مستقل؛ `NewAuditChain(nil)` لا يسبب panic؛ `AppendAt` متاح.

**اختبارات التلاعب:** تغيير Payload/Reason/ActorID/CompanyID/SequenceNumber/PreviousLogHash/Timestamp/CurrentLogHash؛ حذف من الوسط/الذيل؛ إعادة ترتيب؛ checkpoint لشركة أخرى أو قديم/متقدم؛ Append أثناء وبعد اللقطة؛ conflict مستمر؛ context cancelled؛ تزامن Appenders؛ لا اختلاط بين شركتين.

---

### المرحلة 10 — حدود Ledger (P1)

1. وثّق الغرض في `ledger/doc.go` والفرق بين `ledger.Account` و`masterdata.Account`، و`ledger.FiscalPeriod` و`masterdata.FiscalPeriod` مع بيان authority وmapping.
2. حافظ على الحقول العامة؛ لا `SetBalance` ولا تخزين أرصدة ولا TrialBalance ولا تقارير.
3. عالج تنبيه `AccountTypeRevenue` وفق DEC-05.
4. أي pure calculation محاسبي يحتاج متطلباً محدداً واختبارات نقية، وإلا يُرفض نقله إلى Kernel.

**الإغلاق:** لا ازدواجية غير موثقة ولا توسع إلى Orbitals أو Storage.

---

### المرحلة 11 — استعادة الاختبارات والتحقق الكامل (P0/P1)

**ملفات v8 المرجعية للمراجعة:** `doc_test.go`، `apperrors/errors_test.go`، `governance/verification_test.go`، `ledger/ledger_test.go`، `masterdata/fiscal_calendar_fuzz_test.go`، `masterdata/validation_test.go`، `posting/posting_fuzz_test.go`، `posting/property_adversarial_test.go`، `types/identifiers_test.go`، ووثيقة الثوابت.

**قواعد الاستعادة:** لا استعادة آلية؛ لا اختبارات لرفع coverage فقط؛ الاسم يعكس السلوك؛ `errors.Is` لا مقارنة نص؛ Fuzz bounded وبseed corpus داخل Kernel؛ لا يُحذف اختبار بلا بديل.

**خصائص مطلوبة:** Money (constructor/JSON/minor/arithmetic)؛ Entry (balance/uniqueness/identity/sides)؛ Rounding (conservation + residual)؛ Reversal (net zero + dimensions)؛ Fiscal (contiguity + custom day)؛ FX (inverse + same-currency + no future)؛ Audit (hash determinism + snapshot sequence)؛ Master data (no cycles + no cross-company).

**Coverage مقترح:** 95% لـ `types`, `posting`, `masterdata`, `governance`؛ 90% لباقي Kernel — مع بند صريح: النسبة لا تعوّض اختبار قاعدة حرجة.

---

### المرحلة 12 — بوابة الاعتماد (انظر القسم 10)

لا يُعلن الاكتمال قبل نتائج تشغيل فعلية وتقرير تسليم مكتمل.

---

## 7. مصفوفة تتبع القواعد (مختصرة)

| Rule | المتطلب | Rule | المتطلب |
|---|---|---|---|
| 001 | توازن بعد التقريب والتسوية | 014 | تصنيف معتمد |
| 002 | حد أدنى سطرين وطرفي القيد | 015 | حظر Header Account |
| 003 | تطابق العملة الوظيفية والمعاملة | 016 | أهلية الحساب وتاريخه |
| 004 | مدين أو دائن فقط | 017 | منع دورات COA |
| 005 | موجبية المبالغ حسب العقد | 018 | اتساق الفترة والسنة |
| 006 | IDs وأبعاد كاملة | 019 | OPEN/SOFT/HARD CLOSED |
| 007 | حظر float في المال | 020 | تدقيق إعادة الفتح |
| 008 | Banker's rounding | 021 | 13/14 ويوم البداية |
| 009 | تسوية صريحة لفروق التقريب | 022 | عدم تعديل المرحّل |
| 010 | عملات معروفة | 022A | Draft-only وActor وIDs |
| 011 | فصل Transaction/Functional/Presentation | 022B | Auth/RBAC خارج Kernel |
| 012 | صحة سعر الصرف | 022C | Checkpoint خارجي |
| 013 | FX وفق نطاق IAS 21 | 023 | لا تعديل مباشر للأرصدة |
| 024 | تصحيح بقيد عكسي | 026 | منع العكس المكرر |
| 025 | ربط العكس بالأصل | 027 | فترة العكس بتاريخ العكس |

تُستكمل المصفوفة التفصيلية (الملف/الدالة/الاختبار/الحالة/اعتماد المضيف) في `internal/kernal/KERNEL_TRACEABILITY_MATRIX.md` في المرحلة 1. قواعد `ACC-028..040` لا تُنقل إلى Kernel تلقائياً؛ `ACC-041..043` تُرتبط بـ `governance` بالقدر المحدد.

---

## 8. ترتيب التنفيذ والاعتماديات

| الترتيب | المرحلة | تعتمد على |
|---:|---|---|
| 0 | Baseline / Scope Lock | — |
| 1 | Invariants & Traceability | 0 |
| 2 | Fiscal Calendar | 0–1 |
| 3 | Clock / Determinism | 0–1 |
| 4 | Money / Precision | 0–1، وDEC-03 |
| 5 | Posting Pipeline + Idempotency | 2–4 |
| 6 | Reversal / Adjustments | 5 |
| 7 | MasterData / Policy | 1–5 |
| 8 | Valuation / FX | 3–4 و7 |
| 9 | Governance / Snapshot | 1 و3 |
| 10 | Ledger Boundary | 0–1 |
| 11 | Test Restoration & Verification | 2–10 |
| 12 | Release Gate | الجميع |

لا تنفذ المهام بالتوازي إذا كانت تُغيّر عقداً تعتمد عليه أخرى.

---

## 9. التحقق الفني

المتطلب: Go 1.26.3 كما في `go.mod` (`go 1.26.0`، `toolchain go1.26.3`). **لا تُغيَّر إعدادات الإصدار لتجاوز بيئة اختبار.**

**ملاحظة عملية مأخوذة من الخطة (ب):** إذا حاول Go تنزيل الـ toolchain، استخدم الأداة المحلية المثبتة:

```bash
export GOTOOLCHAIN=local
go version
gofmt -w $(find internal/kernal -type f -name '*.go')
go vet ./internal/kernal/...
go test -count=1 ./internal/kernal/...
go test -race -count=1 ./internal/kernal/...
go test -shuffle=on -count=20 ./internal/kernal/...
go test -coverprofile=/tmp/kernel.cover ./internal/kernal/...
go tool cover -func=/tmp/kernel.cover
```

**Fuzz** — استخدم الأسماء الفعلية الموجودة بعد الإنشاء، ولا تنسخ اسماً غير موجود:

```bash
go test ./internal/kernal/types -run=^$ -fuzz=FuzzMoneyFromStringNeverPanics -fuzztime=2m
go test ./internal/kernal/types -run=^$ -fuzz=FuzzMoneyJSONNeverPanics -fuzztime=2m
go test ./internal/kernal/masterdata -run=^$ -fuzz=FuzzGenerateFiscalYearHasContiguousPeriods -fuzztime=2m
go test ./internal/kernal/posting -run=^$ -fuzz=FuzzBalancedEntryProperties -fuzztime=2m
```

**Architecture Verification** — اختبار داخلي يثبت غياب imports المحظورة، وغياب float في المال، وغياب واجهات Auth/RBAC/Role/Permission/MakerChecker، وغياب `SetBalance`.

**Mutation Testing** — إن كانت الأداة متاحة دون تعديل `go.mod`؛ وثّق الأعداد والطفرات الباقية، ولا تدّع النتيجة إن لم تُشغَّل.

مخرجات التغطية تُوضع في `/tmp` ولا تُضاف إلى Git.

---

## 10. بوابة الإصدار

| البوابة | الشرط |
|---|---|
| **A — النطاق** | تغييرات داخل `internal/kernal` فقط؛ لا `go.mod`/CI/Storage/SDK/Orbitals؛ لا RBAC؛ صفر import محظور |
| **B — الثوابت** | قواعد 001..027 موثقة ومرتبطة بملف/دالة/اختبار؛ لا حالة مجهولة؛ مسؤولية المضيف مميزة؛ 13/14 مثبتتان بالاتجاهين |
| **C — الدقة** | عقد precision واحد؛ صفر float؛ لا فقدان صامت؛ لا JSON غير محدود؛ أسعار موجبة؛ نفس العملة = 1 |
| **D — الترحيل** | `PostEntry` يرفض كل نقص؛ لا Executor عند الفشل؛ IdempotencyKey إلزامي؛ reversal بـ executor خاص؛ atomicity موثقة كاعتماد |
| **E — Master/Fiscal** | IDs متسقة؛ صلاحيات مؤرخة؛ COA بلا دورات؛ أبعاد مملوكة؛ يوم البداية محفوظ؛ فترات متصلة؛ حالات الإقفال صحيحة |
| **F — Audit** | الهاش يغطي الحقول الدلالية؛ append ذري حسب العقد؛ snapshot متسق؛ التزامن ليس فساداً؛ retryable عند الاستنفاد؛ التلاعب مكتشف؛ checkpoint موثوق؛ لا خلط شركات |
| **G — الاختبارات** | اختبارات وvet وrace وshuffle وfuzz ناجحة فعلياً؛ تغطية أو استثناءات مبررة؛ mutation موثق إن شُغّل؛ لا حذف بلا بديل |
| **H — العيوب** | صفر P0 وصفر P1 مفتوح؛ P2 مغلق أو مقبول صراحة؛ لا TODO في سلوك إلزامي؛ لا fallback يخفي خطأ دقة/عملة/فترة |

**معيار 10/10:** صحة القواعد، عدم فجوات المتطلب→التنفيذ→الاختبار، عدم تراجع غير موثق، حتمية الوقت والتقريب والتقييم، سلامة الهوية والملكية، اكتمال دورة القيد والعكس، Audit Snapshot بعقد تزامن، استقلال معماري، اختبارات ناجحة فعلياً، وإفصاح عن اعتمادات المضيف. **لا تمنح 10/10 بسبب عدد الملفات أو الدوال أو التغطية وحدها.**

---

## 11. تقرير التسليم

يُكتب `internal/kernal/KERNEL_COMPLETION_REPORT.md` ويحتوي:

1. ملخص التغيير. 2. قائمة الملفات الجديدة/المعدلة/المحذوفة وسببها. 3. حالة كل Rule من 001 إلى 027. 4. تقرير Regression لكل ملف/اختبار كان في v8 وغاب عن v9. 5. دليل الاختبارات: الأمر، إصدار Go، النتيجة، العدد، الزمن. 6. التغطية الكلية وللحزم الحرجة. 7. نتائج Race/Fuzz/Mutation الفعلية. 8. الاعتمادات الخارجية. 9. القيود المعروفة. 10. قرار Pass/Fail مع الأسباب.

**يُمنع** استخدام «100% مكتمل» أو «جميع الاختبارات ناجحة» دون نتائج تشغيل محفوظة قابلة للمراجعة.

---

## الملحق أ — ما أُخذ من الخطة (ب) وما رُفض

| مأخوذ ومُعتمد | مرفوض ومُحسوم في القسم 3 |
|---|---|
| مقتطف `addFiscalMonths` (المرحلة 2) | بناء GL/Trial Balance داخل Kernel |
| مقتطف `Clock`/`systemClock` (المرحلة 3) | حذف حزمة `ledger` |
| `GOTOOLCHAIN=local` (القسم 9) | `MaximumRepresentationalScale = 18` |
| تنبيه `AccountTypeRevenue` (DEC-05) | قصّ `scale` في `MarshalJSON` |
| قراءة `audit` للتوافق — مقيدة DEC-04 | تأشيرات ✅ قبل التشغيل |
| مخططات/ملخص تنفيذي (للعرض فقط) | الاكتفاء بـ 7 تراجعات وإغفال Idempotency |

## الملحق ب — ملاحظات لا تتجاوزها

- لا `time.Now()` مرتين داخل قرار ترحيل واحد.
- لا تختار `Contains()` الفترتين 13 أو 14.
- لا تصنيف فارغ، ولا تسوية عادية في 14، ولا Audit خارج 14.
- لا rounding اختياري منفصل عن `PostEntry`.
- لا سعر غير 1 عند تطابق العملتين، ولا سياسة عملة فارغة/غير نافذة.
- لا ثقة بنتيجة Reader دون تحقق CompanyID وطلب ID.
- لا اعتبار التزامن الطبيعي أثناء Snapshot فساداً، ولا إعادة محاولة لإخفاء hash mismatch.
- لا exponent غير محدود، ولا حذف اختبار بلا بديل، ولا نقل تخزين ذري أو تقارير إلى Kernel، ولا عودة RBAC.

---

## الملحق ج — حالات القرار المعلقة

| القرار | الحالة المطلوبة قبل التنفيذ |
|---|---|
| DEC-03: قيمة `MaximumRepresentationalScale` و`MaximumIntegerDigits`، وحسم تعارض 12/13 | `DEFERRED_BY_DECISION` حتى قرار مالك القاعدة |
| DEC-06: دلالة نوع `AdjustingEntryAccrual` وعدم إضافة `AdjustingEntryRevaluation` | `DEFERRED_BY_DECISION` حتى قرار محاسبي |
| DEC-05 التوسع إلى GL داخل Kernel | `REJECTED` في هذه المهمة؛ يحتاج قرار معماري منفصل |
