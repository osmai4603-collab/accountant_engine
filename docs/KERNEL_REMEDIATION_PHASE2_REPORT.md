# تقرير المرحلة 2 — سلامة الأنواع والقيم المالية

- **رمز التقرير:** `AE-REMED-PHASE2-REPORT`
- **الخطة:** [KERNEL_REMEDIATION_EXECUTION_PLAN.md](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)
- **خط الأساس:** [KERNEL_REMEDIATION_BASELINE.md](./KERNEL_REMEDIATION_BASELINE.md)
- **تقرير المرحلة 1:** [KERNEL_REMEDIATION_PHASE1_REPORT.md](./KERNEL_REMEDIATION_PHASE1_REPORT.md)
- **سجل الفجوات:** [KERNEL_REMEDIATION_GAP_REGISTER.md](./KERNEL_REMEDIATION_GAP_REGISTER.md)
- **القرار المرجعي:** [ADR P2-01](./ard/P2_01_monetary_precision_contract.md)
- **المصفوفة التنفيذية:** [KERNEL_MONETARY_PRECISION_MATRIX.md](./KERNEL_MONETARY_PRECISION_MATRIX.md)
- **تاريخ التنفيذ (UTC):** 2026-11-22
- **Commit المرجعي:** `94054d9c9a5f8d8419c9871b2d61b17b44d5b556`
- **إصدار Go الفعلي:** `go version go1.26.3 linux/amd64` — مطابق لـ`go.mod` و`toolchain`
- **قرار البوابة:** **PASS**

---

## 1. الهدف والنطاق

نفّذت هذه المرحلة المهام 1–14 من المرحلة 2 حصراً: إثبات أن `Money` و`Currency` وDecimal والتقريب تعمل بسياسة واحدة في كل مسارات الحساب والتحويل والتطبيع، وإضافة أدلة خصائص وفَزّ وحدود قابلة لإعادة الإنتاج.

**لم يُنفّذ:** استبدال مكتبة Decimal، ولا تغيير `InternalScale` أو أي وضع تقريب، ولا تغيير قاعدة محاسبية (`RULE-KERNEL-001..008` بقيت كما هي)، ولا تغيير توقيع أو حذف أي API عام، ولا نقل مسؤولية خارجية إلى النواة، ولا تعديل على `journal` أو `ledger` أو `posting` الإنتاجية.

**القاعدة الحاكمة المطبقة:** كل ما كان صحيحاً تم تثبيته بأدلة، وما كان ناقصاً أُضيف كاختبار أو بوابة آلية؛ ولم يُغيَّر أي سلوك مالي قائم.

## 2. تنفيذ مهام المرحلة 2

| # | المهمة | النتيجة | الدليل |
|---:|---|---|---|
| 1 | جدول الأنواع المالية ودقتها ومجالها وقواعد إنشائها | أُنجز | [KERNEL_MONETARY_PRECISION_MATRIX.md](./KERNEL_MONETARY_PRECISION_MATRIX.md) الأقسام 1–2 |
| 2 | تتبع العمليات المالية وتأكيد خلوها من Float | أُنجز + سُدّت ثغرة اختبارية | `TestKernelFinancialCodeHasNoFloatConversions` يفحص AST لكل ملفات النواة بما فيها الاختبارات؛ استُبدلت 3 إنشاءات `decimal.NewFromFloat` بـ`decimal.RequireFromString` في `posting/rounding_more_test.go` و`valuation/valuation_coverage_test.go` بنفس القيم العشرية |
| 3 | تدقيق `NewMoney` و`NewMoneyFromString` والدلالة والأسس | أُنجز | `TestMonetaryPrecisionContractConstants`, `TestNewMoneyNormalizesCurrencyAndRejectsXXX`, `TestNewMoneyPrecisionBoundaryMatrix`, `FuzzMoneyConstructorPrecisionAndRoundTrip` |
| 4 | تدقيق `InternalScale` وتطبيع `MinorUnit` | أُنجز | `TestDivideKeepsInternalPrecisionUntilPosting` (موجود)، `TestMinorUnitsContractAcrossRegistry`, `TestSequentialOperationsAreDeterministicAndBounded`؛ المصفوفة القسم 4 |
| 5 | الفرق بين `MinorUnits` و`MinorUnitsExact` | أُنجز | `TestMinorUnitsContractAcrossRegistry` يمر على كل العملات، و`FuzzMinorUnitsExactNeverDropsFraction` يثبت أن المسار الصارم يفشل مغلقاً والصريح حتمي ومحدود |
| 6 | Banker's Rounding وجميع الأوضاع المدعومة | أُنجز | `TestBankersRoundingTieToEvenProperty` يولّد نقاط المنتصف لـ`k=-20..20` و`scale=0..4`؛ `TestBankersRoundingEdgeCases` (موجود)؛ ADR P2-01 يثبت أن هذا هو الوضع الوحيد |
| 7 | عملات 0 و2 و3 وحدات، والمجهولة و`XXX` | أُنجز | `TestCurrencyMinorUnitClasses`, `TestCurrencyRegistryIsDeterministicAndIsolated`, `TestNewMoneyNormalizesCurrencyAndRejectsXXX`؛ السجل يحتوي فعلياً على فئة 4 وحدات أيضاً (CLF/UYW) |

| # | المهمة | النتيجة | الدليل |
|---:|---|---|---|
| 8 | منع خلط العملات في الجمع/الطرح/المقارنة | أُنجز | `TestMoneyOperationsPreserveCurrency`، `FuzzMoneyOperationsPreserveCurrencyAndDeterminism`؛ المقارنة تُعيد `false` والعملية الحسابية تُعيد `ErrCurrencyMismatch` |
| 9 | سعر العملة لنفسها = 1 بالضبط | أُنجز | `TestSameCurrencyConversionKeepsUnitRateExact`: `GetRate` يُعيد `1` دون سجل، ويظل يتحقق من الرمز والتاريخ، و`Convert` يرفض أي سعر آخر |
| 10 | التراكم وفروق التقريب على مجموعات كبيرة | أُنجز | `TestSequentialOperationsAreDeterministicAndBounded`: 360 قسطاً + 64 قيمة بثلاثة ترتيبات، وتصالح الفروق مع الفرق بين المجموع الدقيق والمطبّع |
| 11 | overflow والحدود والمدخلات النصية الشاذة | أُنجز | `TestNewMoneyPrecisionBoundaryMatrix`, `TestMoneyMinorUnitsInt64Overflow` (موجود)، `TestMoneyRegistry…`، و4 أهداف Fuzz |
| 12 | Property/Fuzz إضافية | أُنجز | 4 أهداف جديدة في `types/money_property_fuzz_test.go`؛ العدد الإجمالي 13 بدلاً من 9 |
| 13 | النسخ وعدم تسريب مراجع داخلية | أُنجز | `TestMoneyValueImmutability` يثبت أن `Coefficient()` و`BigInt()` نسخ، و`TestCurrencyRegistryIsDeterministicAndIsolated` يثبت أن `AllCurrencies()` مقطع جديد مرتب |
| 14 | تحديث القواعد 001–008 ومصفوفة التتبع | أُنجز | لم تتغير نصوص القواعد الـ27 (`RULE-KERNEL-001..027`)؛ أُضيف ملحق توضيحي غير مُنشئ لقاعدة في `KERNEL_ACCOUNTING_INVARIANTS.md`، ورُبطت الأدلة الجديدة بالصفوف 001–008 والقسم 1.1 في `KERNEL_TRACEABILITY_MATRIX.md` |

## 3. قرار بعدم تغيير السلوك المالي

| وجه المقارنة | قبل المرحلة 2 | بعد المرحلة 2 | الحكم |
|---|---|---|---|
| نصوص `RULE-KERNEL-001..008` | 8 قواعد | 8 قواعد + ملحق توضيحي غير مُلزم | بلا تغيير |
| قيم `FinancialScale` / `InternalScale` / السقوف | 4 / 12 / 28 | 4 / 12 / 28 | بلا تغيير |
| `RoundBank` كقاعدة تقريب | الوضع الوحيد | الوضع الوحيد | بلا تغيير |
| `MinorUnitsExact` عند كسر تحت الوحدة الصغرى | `ErrSubMinorUnitAmount` | `ErrSubMinorUnitAmount` | بلا تغيير |
| السطح العام | 0 إزالة/تغيير توقيع | 0 إزالة/تغيير توقيع (`check_api_compat.sh` PASS) | بلا تغيير |
| تغطية النواة | 93.1% | 93.1% | بلا انحدار |
| دوال الاختبار / أهداف Fuzz | 151 / 9 | 165 / 13 | +14 اختبار، +4 أهداف |

استُبدلت ثلاثة ثوابت اختبارية من `decimal.NewFromFloat(x)` إلى `decimal.RequireFromString("x")` بنفس القيمة العشرية بالضبط؛ لذلك لا يوجد فرق سلوكي، والاختبات المعنية (`TestRoundEntryEdgeCasesAndBranches`, `TestValuationCoverageBranches`) نجحت قبل التغيير وبعده.

## 4. الملفات المتأثرة

### 4.1 اختبارات جديدة

- `internal/kernal/types/money_integrity_test.go` (جديد — 8 اختبارات).
- `internal/kernal/types/currency_integrity_test.go` (جديد — اختباران).
- `internal/kernal/types/money_property_fuzz_test.go` (جديد — 4 أهداف Fuzz).
- `internal/kernal/valuation/conversion_precision_test.go` (جديد — اختباران).

### 4.2 بوابات واختبارات معدّلة

- `internal/kernal/doc_test.go` — بوابتان آليتان جديدتان (حظر Float في كل الملفات، وحظر الواجهات المتساهلة في إنتاج النواة).
- `internal/kernal/types/doc.go` — تحديث charter المالي (doc-only).
- `internal/kernal/posting/rounding_more_test.go` — إزالة `NewFromFloat` من اختبار.
- `internal/kernal/valuation/valuation_coverage_test.go` — إزالة `NewFromFloat` من اختبار.

### 4.3 وثائق وقرارات

- `docs/ard/P2_01_monetary_precision_contract.md` (جديد).
- `docs/KERNEL_MONETARY_PRECISION_MATRIX.md` (جديد).
- `docs/KERNEL_REMEDIATION_PHASE2_REPORT.md` (هذا التقرير).
- `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` — ملحق توضيحي غير مُنشئ لقواعد.
- `internal/kernal/KERNEL_TRACEABILITY_MATRIX.md` — ربط الأدلة بالقواعد 001–008 + ملحق المرحلة 2.
- `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` — إغلاق `G-015` (نطاق النواة)، وإنشاء `G-018`، وتحديث التوزيع.

**لم يُعدّل أي ملف مصدر إنتاجي في `internal/kernal`**؛ التغييرات البرمجية كانت اختبارات وبوابات فقط.

## 5. الأدلة وأوامر التحقق المعاد تشغيلها

| الأمر | النتيجة | ملاحظات |
|---|---|---|
| `gofmt -l internal/kernal/` | لا إخراج / exit 0 | تنسيق نظيف |
| `go test ./internal/kernal/... -count=1` | `ok` — 10/10 حزم | 165 Test + 13 Fuzz |
| `go test -race ./internal/kernal/... -count=1` | `ok` — 10/10 حزم | لا سباقات |
| `go test ./... -count=1` | `ok` لكل الحزم | تخطٍ بيئي واحد معروف في `test/benchmark` لغياب `POSTGRES_URL`؛ أُعيد تقريرا التكافؤ إلى حالتهما المتتبعة بعد التشغيل (G-010) |
| `./scripts/verify_architecture.sh` | `RESULT: PASS - clean architectural base` | 11/11 قاعدة دون تخفيض |
| `./scripts/check_api_compat.sh` | `API compatibility check: passed` | لا إزالة أو تغيير توقيع |
| `git diff --check` | لا إخراج / exit 0 | لا أخطاء مساحات |
| `go test ./internal/kernal/... -coverprofile=… && go tool cover -func=…` | `total: 93.1%` | مطابق لخط الأساس |
| 4 أهداف Fuzz جديدة، `-fuzztime=5s` لكل هدف | PASS — 0 فشل / 0 panic | 243,518 تنفيذ تقريبياً؛ corpus الفشل لم يُكتب إلى المستودع |

### ملخص Fuzz الجديد

| الهدف | التنفيذات (تقريباً) |
|---|---:|
| `FuzzMinorUnitsExactNeverDropsFraction` | 26,998 |
| `FuzzMoneyOperationsPreserveCurrencyAndDeterminism` | 44,274 |
| `FuzzMoneyConstructorPrecisionAndRoundTrip` | 45,591 |
| `FuzzBankersRoundingIsDeterministicAndBounded` | 126,655 |

## 6. حالة الفجوات بعد المرحلة 2

| الفجوة | الحالة | الملخص |
|---|---|---|
| `G-015` | **Closed — Phase 2 (kernel scope)** | أتمتة AST تمنع `RoundForPosting` و`CurrencyMinorUnit` في إنتاج النواة؛ سلوك التوافق مثبت بالاختبارات |
| `G-018` | Open (جديدة) | استخدام الواجهتين نفسهما خارج النواة في `modules/` و`internal/orbitals/`؛ مالكها خارج النطاق، لا تُغلق بـADR وحده |
| `G-001` | Open | idempotency — المرحلة 7 |
| `G-002` | Open | ترتيب الأقفال مقابل القفل الفعلي — المرحلة 8 |
| `G-004` | Partially Closed | إثبات Rebuild التنفيذي — المرحلة 4/12 |
| `G-011` | Open | الـ corpus المحفوظ ما زال محدوداً؛ الأهداف الجديدة أضافت seeds في الشيفرة لا ملفات corpus |
| `G-012` | Open | دلالة assertions في المصفوفة ما زالت بلا أتمتة عامة؛ المرحلة 6 |
| بقية الفجوات | Open | بمالكيها ومراحلها في السجل |

## 7. المخاطر المتبقية

1. **الاستخدام الخارجي للواجهات المتساهلة (`G-018`):** النواة نظيفة آلياً، لكن `modules/` و`internal/orbitals/` ما زالت تستدعيها؛ أي حذف لاحق يتطلب ترحيلاً منسقاً.
2. **الفئة رباعية الوحدات الصغرى:** CLF/UYW مغطاة بالاختبار العام، لكنها ليست个工作ة محاسبية شائعة؛ أي سياسة حفظ خاصة بها يجب أن تُقرر في المرحلة 7/11.
3. **الفَزّ غير حتمي العدد:** أرقام التنفيذات تعتمد على الآلة والـcache؛ المعتمد هو PASS وصفر panic لا العدد.
4. **إثبات Rebuild (`G-004`):** خارج هذه المرحلة؛ لا يزال مرتبطاً بالمرحلتين 4 و12.

## 8. قرار البوابة

بناءً على:

- تنفيذ المهام 1–14 كاملة؛
- عدم تغيير أي قاعدة محاسبية أو توقيع عام أو ثابت مالي؛
- وجود ADR مقبول ومصفوفة دقة قابلة للتنفيذ؛
- نجاح `go test` و`go test -race` لكامل النواة؛
- نجاح 4 أهداف Fuzz جديدة و11/11 بوابة معمارية؛
- ثبات التغطية عند 93.1% وخلو `git diff --check` و`check_api_compat.sh` من المشاكل؛
- تحديث مصفوفة التتبع وسجل الفجوات في نفس التغيير؛

**قرار بوابة المرحلة 2: PASS.**

لا تُبدأ المرحلة 3 قبل إدراج مخرجات هذه المرحلة في مراجعة التغيير؛ تبقى `G-018` و`G-001` و`G-002` و`G-004` و`G-011` و`G-012` مفتوحة بمالكيها ومراحلها المحددة.
