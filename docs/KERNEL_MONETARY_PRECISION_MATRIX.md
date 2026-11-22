# مصفوفة الدقة المالية — Kernel Monetary Precision Matrix

- **رمز الوثيقة:** `AE-REMED-P2-MATRIX`
- **المرحلة:** 2 — سلامة الأنواع والقيم المالية
- **القرار المرجعي:** [ADR P2-01](./ard/P2_01_monetary_precision_contract.md)
- **الخطة:** [KERNEL_REMEDIATION_EXECUTION_PLAN.md](./KERNEL_REMEDIATION_EXECUTION_PLAN.md)، المرحلة 2 مهمة 1
- **النطاق:** `internal/kernal/types` ونقاط استخدام `Money` في `journal` و`posting` و`valuation`
- **الحالة:** مصفوفة تنفيذية مُثبتة بالاختبارات؛ لا تغيير في السلوك المالي القائم

---

## 1. الثوابت والأنواع

| النوع/الثابت | القيمة | المعنى | مجال القيم | قاعدة الإنشاء | دليل التنفيذ |
|---|---|---|---|---|---|
| `Money` | `decimal.Decimal` + `string` | قيمة مالية دقيقة مرتبطة بعملة | أي Decimal يقبل قواعد `newMoney` أدناه | `NewMoney`, `NewMoneyFromDecimal`, `NewMoneyFromString`, `Zero` | `types/money.go:32-81` |
| `FinancialScale` | `4` | مقياس عرض فقط؛ ليس وحدة صغرى ولا مقياس حفظ | ثابت | لا يُستخدم في قرار مالي؛ يُستخدم في `String` و`MarshalJSON` وfallback التوافق | `money.go:22`, `money.go:258-260`, `money.go:301-310` |
| `InternalScale` | `12` | مقياس القسمة والتجميع الوسيط | ثابت | `Divide` → `DivRound(scalar, 12)` | `money.go:23`, `money.go:205-210` |
| `MaximumSignificantDigits` | `28` | عدد الخانات ذات الدلالة بعد إزالة الأصفار الزائدة | `1..28` | `precisionDigits(amount)` | `money.go:24`, `money.go:74-76`, `money.go:99-106` |
| `MaximumIntegerDigits` | `28` | عدد الخانات يسار الفاصلة | `0..28` | `integerDigits(amount)` | `money.go:25`, `money.go:77-79`, `money.go:108-118` |
| `MaximumRepresentationalScale` | `28` | عدد الخانات يمين الفاصلة في التمثيل الداخلي | `0..28` | `amount.Exponent() >= -28` | `money.go:26`, `money.go:71-73` |
| `Currency` | `Code`, `NumericCode`, `Symbol`, `Name`, `MinorUnit` | سجل ISO 4217 ثابت | `MinorUnit` من `0` إلى `4` في السجل الحالي | `LookupCurrency`, `LookupCurrencyByNumericCode`, `AllCurrencies` | `types/currency.go:182-308` |
| `CurrencyCode` | `string` | رمز عملة مكوّن من 3 أحرف | رموز السجل + `XXX` كعلامة منع | `normalizeCurrency` (upper + trim) | `currency.go:180`, `currency.go:302-308` |
| `XXX` | `"999"` sentinel | عملة محظورة مالياً | `MinorUnit = 0`, رمز فارغ | `LookupCurrency` succeeds, لكن بناء `Money` يفشل بـ`ErrInvalidCurrency` | `currency.go:275`, `money.go:67-70` |

> لا يوجد `float32` أو `float64` في أي مسار مالي داخل النواة، بما في ذلك الاختبارات؛ يُفرض ذلك بفحص AST في `internal/kernal/doc_test.go` (`TestKernelFinancialCodeHasNoFloatConversions`).

## 2. قواعد البناء والرفض

| العملية | القاعدة | الخطأ عند المخالفة | اختبار الإثبات |
|---|---|---|---|
| `NewMoney(amount, currency)` | تطبيع الرمز؛ رفض `XXX`/المجهول؛ فحص الأسس والدلالة والخانات الصحيحة | `ErrInvalidCurrency`, `ErrAmountExceedsPrecision` | `TestNewMoneyNormalizesCurrencyAndRejectsXXX`, `TestNewMoneyPrecisionBoundaryMatrix`, `FuzzMoneyConstructorPrecisionAndRoundTrip` |
| `NewMoneyFromString(value, currency)` | `TrimSpace` ثم `decimal.NewFromString` ثم نفس قواعد `newMoney` | `ErrInvalidAmount`, `ErrInvalidCurrency`, `ErrAmountExceedsPrecision` | `TestInvalidAmounts`, `TestNewMoneyPrecisionBoundaryMatrix` |
| `Zero(currency)` | `NewMoney(decimal.Zero, currency)` | كقواعد `newMoney` | `TestZeroValueMoneyJSONRoundTrip` |
| `UnmarshalJSON` | يفك الحقلين النصيين ثم يعيد التحقق عبر `NewMoneyFromString` | خطأ المُفك | `TestMoneyJSONRoundTripAtPrecisionBoundaries`, `FuzzMoneyJSONNeverPanics` |

## 3. مصفوفة المداخل والمخارج حسب الوحدة الصغرى

| فئة العملة | ممثلون | `MinorUnit` | مثال كمية | `MinorUnitsExact` | `MinorUnits` | `Rounded()` | الفرق المسموح |
|---|---|---:|---|---|---|---|---|
| بلا كسور | JPY, KRW, CLP, BIF | 0 | `1.5` | `ErrSubMinorUnitAmount` | `2` (half-to-even) | `2` | `≤ 0.5` وحدة |
| وحدتان عشريتان | USD, EUR, SAR, GBP | 2 | `1.005` | `ErrSubMinorUnitAmount` | `100` | `1.00` | `≤ 0.005` وحدة |
| ثلاث وحدات | KWD, BHD, OMR, IQD | 3 | `1.0005` | `ErrSubMinorUnitAmount` | `1000` | `1.000` | `≤ 0.0005` وحدة |
| أربع وحدات | CLF, UYW | 4 | `1.00005` | `ErrSubMinorUnitAmount` | `10000` | `1.0000` | `≤ 0.00005` وحدة |

الدليل: `TestMinorUnitsContractAcrossRegistry` يمر على **كل** عملات السجل (باستثناء `XXX`) ويتحقق من تطابق المسارين الصارم والصريح، ومن أن residual لا يتجاوز نصف الوحدة الصغرى. الدليل الإضافي: `FuzzMinorUnitsExactNeverDropsFraction`.

## 4. العمليات وسياساتها

| العملية | السياسة | حتمية/حدود | اختبار الإثبات |
|---|---|---|---|
| `Add`, `Subtract` | فحص تطابق العملة ثم `newMoney` على النتيجة | تجمّع عشري exact؛ لا يعتمد على ترتيب Map | `TestMoneyOperationsPreserveCurrency`, `TestSequentialOperationsAreDeterministicAndBounded`, `FuzzMoneyOperationsPreserveCurrencyAndDeterminism` |
| `Multiply` | ضرب عشري ثم إعادة التحقق من السقوف | تجاوز 28 خانة → `ErrAmountExceedsPrecision` | `TestMoneyOperationsPreserveCurrency` |
| `Divide` | `DivRound(scalar, InternalScale)` | مقسوم عليه صفر → `ErrInvalidAmount`؛ كسور داخلية حتى 12 | `TestDivideKeepsInternalPrecisionUntilPosting`, `TestSequentialOperationsAreDeterministicAndBounded` |
| `Rounded`, `RoundForPostingChecked` | `RoundBank(MinorUnit)` | وضع التقريب الوحيد؛ نتيجة قابلة للتمثيل | `TestBankersRoundingTieToEvenProperty`, `TestRoundForPostingCheckedRejectsInvalidCurrency` |
| `MinorUnits`, `MinorUnitsExact` | صريح مع تقريب / صارم بلا تقريب | تجاوز `int64` → `ErrAmountExceedsPrecision` | `TestMoneyMinorUnitsInt64Overflow`, `FuzzMinorUnitsExactNeverDropsFraction` |
| `RoundingDifference` | `amount - Rounded(amount)` | يعيد الفرق المُسجَّل، ولا يُسقطه | `TestMinorUnitsExactRejectsSubMinorFractions` |
| `Equal`, `GreaterThan`, `LessThan` | تطابق العملة شرط ضمني | عملتان مختلفتان لا تُقارنان أبداً | `TestMoneyOperationsPreserveCurrency` |
| `Convert`, `ConvertForDate` | ضرب في السعر ثم `Rounded()` | سعر نفس العملة = `1` بالضبط مع بقاء التحقق سارياً | `TestSameCurrencyConversionKeepsUnitRateExact`, `TestCurrencyConversionIsDeterministic` |
| `MarshalJSON`, `UnmarshalJSON` | نص دقيق بمقياس `max(-exponent, 4)` | لا فقد عند الحدود | `TestMoneyJSONPreservesInternalPrecision`, `FuzzMoneyConstructorPrecisionAndRoundTrip` |
| `AllCurrencies` | نسخة جديدة مرتبة حسب `Code` | لا تسريب لمرجع داخلي | `TestCurrencyRegistryIsDeterministicAndIsolated` |

## 5. التقريب نصف إلى زوجي (Banker's Rounding)

| المقياس | midpoint | نتيجة زوجية | نتيجة فردية | الدليل |
|---:|---|---|---|---|
| 0 | `2.5` / `3.5` | `2` / `4` | حسب زوجية خانة الآحاد | `TestBankersRoundingEdgeCases` |
| 2 | `10.005` / `10.015` | `10.00` / `10.02` | `10.02` عند فردية الخانة | `TestBankersRoundingEdgeCases`, `TestBankersRoundingTieToEvenProperty` |
| 3 | `1.0005` / `1.0015` | `1.000` / `1.002` | حسب زوجية الخانة الثالثة | `TestBankersRoundingEdgeCases`, `TestBankersRoundingTieToEvenProperty` |
| 4 | `0.00085` / `0.00095` | `0.0008` / `0.0010` | حسب زوجية الخانة الرابعة | `TestBankersRoundingTieToEvenProperty` |

خاصية التوليد في `TestBankersRoundingTieToEvenProperty`: المعامل `10k+5` بالأس `-(scale+1)` هو منتصف تام بين `k` و`k+1` بوحدات المقياس؛ النتيجة `k` إذا كان زوجياً وإلا `k+1`. تغطي الحلقة `k` من `-20` إلى `20` لكل مقياس، مع التحقق من التكرار (idempotence).

## 6. حدود التراكم والحتمية

- 360 قسطاً من `1000 / 360` عند `InternalScale = 12` تُجمَع إلى `1000.00000000008` بالضبط؛ الفرق بعد التطبيع إلى وحدات USD الصغرى أقل من نصف سنت.
- مجموع الفروق الفردية (`RoundingDifference`) يساوي الفرق بين المجموع الدقيق ومجموع القيم المطبّعة، دون افتراض ترتيب.
- جمع 64 قيمة بترتيب أصلي وترتيب معكوس وترتيب أبجدي حسب `ExactString` يعطي النتيجة نفسها.
- حساب السعر المعكوس في `RateRegistry` يستخدم `DivRound(..., 12)`، ونتائج التحويل المتكررة متطابقة.

الدليل: `TestSequentialOperationsAreDeterministicAndBounded`، `TestCurrencyConversionIsDeterministic`.

## 7. التتبع إلى القواعد المحاسبية

| القاعدة | ما تثبته هذه المصفوفة |
|---|---|
| `RULE-KERNEL-001` | الفروق الناتجة عن التقريب معلنة وقابلة للتسوية الصريحة |
| `RULE-KERNEL-002` | وحدة العملة إلزامية في الجمع/الطرح والمقارنة |
| `RULE-KERNEL-003` | رفض المدخلات النصية والأسس غير الصالحة |
| `RULE-KERNEL-004` | `XXX` والمجهول مرفوضان؛ التطبيع حتمي |
| `RULE-KERNEL-005` | ثلاثة سقوف `28` واختبارات حدية لكل واحد |
| `RULE-KERNEL-006` | `InternalScale` وسيط؛ `MinorUnit` مقياس الحفظ |
| `RULE-KERNEL-007` | `MinorUnitsExact` لا يُسقط كسراً بصمت |
| `RULE-KERNEL-008` | سعر العملة لنفسها `1` بالضبط وبعد التحقق |
