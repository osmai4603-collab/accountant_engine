# خطة التنفيذ النهائية P13 — سلامة البصمات، إثبات العقود، واعتماد Kernel

- **رمز الوثيقة:** `AE-KERNEL-P13-FINAL-ASSURANCE`
- **الإصدار المرجعي محل المراجعة:** الأرشيف `accountant_engine(1).zip`
- **بصمة الأرشيف SHA-256:** `95237b6a5dc02ebde99cc990d74d5f2853cdc15dd53d4540abe641f188e8b10e`
- **النطاق البرمجي الملزم:** `internal/kernal/**` فقط
- **نوع الخطة:** خطة إغلاق نهائي تكميلية بعد مراحل المعالجة P0–P12؛ ليست إعادة تنفيذ للمراحل السابقة
- **الهدف:** الوصول إلى اعتماد `Kernel GO` مبني على تنفيذ صحيح وأدلة قابلة لإعادة الإنتاج ومراجعة مستقلة، لا على عبارة نجاح في تقرير فقط
- **حالة الوثيقة:** خطة تنفيذية مقترحة مبنية على مراجعة الأرشيف والوثائق والشيفرة الفعلية

---

## 0. القرار التنفيذي والملخص

وصل المشروع إلى مرحلة متقدمة. يحتوي الأرشيف على تسع حزم داخل `internal/kernal`، ووثائق ثوابت محاسبية ومصفوفة تتبع وتقارير إنجاز واثني عشر تقرير مرحلة سابقة. ويذكر سجل الفجوات أن الفجوات داخل النواة أُغلقت، فيما بقيت مسائل خارج النطاق.

لا توصي هذه الخطة بإعادة تنفيذ مراحل P0–P12 أو بإعادة بناء Kernel. المطلوب الآن هو **P13 — Final Assurance & Integrity Closeout**: مراجعة خط الأساس، إصلاح مخاطر محددة في بصمات منع التكرار، حسم عقد نطاق إعادة التقييم، تدقيق موثوقية أدلة الاختبارات، تسوية التناقضات التوثيقية، ثم تنفيذ اعتماد نهائي مستقل.

### 0.1 نتائج المراجعة التي بُنيت عليها الخطة

| المرجع الذي تمت مراجعته | النتيجة المستخلصة | أثرها على خطة P13 |
| --- | --- | --- |
| `internal/kernal/posting/transaction.go` | `FingerprintRequest` و`FingerprintCallerPayload` يستخدمان `field()` الذي يكتب نص الحقول مباشرة مع فواصل ` | `؛ لا يوجد ترميز يحسم حدود القيمة النصية | عيب حرج في التمثيل canonical قبل SHA-256؛ يعالج أولاً |
| `internal/kernal/posting/revaluation_scope_hint_test.go` | اختبار `valid hint takes precedence over reversal derivation` يثبت أن تلميح Revaluation يستطيع اختيار العملية الثالثة حتى عندما يحتوي القيد على `ReversalOfID` | يلزم حسم دلالة هذا السلوك مقابل ADR P11-01، وعدم تغييره بلا قرار معماري واختبارات |
| `docs/ard/P11_01_revaluation_idempotency_spi.md` | القرار يسمح بتمرير `RevaluationIdempotencyScope` في `PostOptions.Scope`، ويمنع تغيير توقيع `PostEntry` أو `PostingExecutor` | يجب المحافظة على العقد المقبول أو تحديث القرار بإجراء معتمد؛ لا تضف use-case عامّاً خارج النطاق |
| `internal/kernal/KERNEL_ACCOUNTING_INVARIANTS.md` | توجد 27 قاعدة محاسبية محددة | يجب إعادة إثبات كل قاعدة وعدم اختراع قواعد أو تغييرها ضمنياً |
| `internal/kernal/KERNEL_TRACEABILITY_MATRIX.md` | المصفوفة تربط القواعد بالدوال والاختبارات، لكن يجب التحقق من الارتباط الدلالي لا الاسم فقط | تدقيق assertions وربط كل قاعدة بأدلة فعلية |
| `internal/kernal/KERNEL_COMPLETION_REPORT.md` | تقرير إنجاز يعلن نجاح بوابات واختبارات | يعامل كدليل محفوظ يحتاج مطابقة مع المخرجات الخام الحالية، لا كبديل عن إعادة التشغيل |
| `internal/kernal/KERNEL_COVERAGE_EXCEPTIONS.md` | توجد فروع غير مغطاة مصنفة كاستثناءات | يعاد فحص كل استثناء؛ لا تقبل تسمية «دفاعي» وحدها سبباً للاستثناء |
| `internal/kernal/KERNEL_MUTATION_REPORT.md` | توجد نتائج Mutation موجهة | يجب التفريق بين الطفرات المستهدفة وبين Mutation Score شامل، وفحص صحة تطبيق كل طفرة |
| `docs/KERNEL_REMEDIATION_EXECUTION_PLAN.md` وتقارير P1–P12 | توجد خطة سابقة واسعة وتقارير مراحل مكتملة | لا تكرر هذه المراحل؛ افحصها كمصدر تاريخي وإثبات سابق |
| `docs/KERNEL_REMEDIATION_GAP_REGISTER.md` | ملخص السجل يصف G-007 وG-018 كفجوتين خارج النطاق، بينما صف G-018 يذكر أنها Open ولا تُغلق بقرار ADR وحده | يجب الفصل بين Kernel GO وSystem/Product GO، وعدم وصف G-018 بأنها مغلقة |
| `docs/KERNEL_REMEDIATION_PLAN_REVIEW.md` | وثيقة المراجعة رصدت C1/C2 حول المراجع المستقل وتعريف PASS/CONDITIONAL/FAIL، إضافة إلى قضايا الأدلة والاعتماديات | تثبيت سلطة القرار وحالات البوابات والأدلة في سجل الاعتماد الجديد |
| `scripts/verify_kernel_baseline.sh` | حلقة Fuzz تسجل `EXIT=$?` لكنها لا ترفع متغير الفشل العام عند فشل هدف Fuzz | لا تعتمد النتيجة النهائية لهذا السكربت وحدها كإثبات نجاح Fuzz |
| `scripts/run_fuzz_all.sh` | يشغل 16 هدف Fuzz ويلتقط رمز الخروج لكل هدف | يمكن استخدامه قراءةً وتشغيلاً دون تعديله، مع توثيق المدة والنتيجة |
| `scripts/run_mutation_tests.sh` | أي خروج غير صفري من `go test` يصنف الطفرة KILLED، دون فصل فشل assertion عن فشل build/setup؛ كما لا يتحقق صراحة من تطبيق الطفرة | يلزم تحقق مستقل من صحة كل طفرة؛ لا تعتبر 8/8 برهاناً شاملاً دون فحص آلية التشغيل |
| `scripts/check_coverage.sh` | القيمة الافتراضية 80%، وتعليقه يقيّد override للاستخدام التشخيصي | احسب Coverage المستهدف مباشرة من profile، ولا تعتمد الافتراضي دليلاً على عتبة 90% |
| `go.mod` | يتطلب `go 1.26.0` و`toolchain go1.26.3` | يلزم تنفيذ الاعتماد النهائي على Go 1.26.3 بالضبط |

### 0.2 العيب المؤكد الذي يبدأ به P13

في `posting/transaction.go` تُسلسل الحقول النصية بهذه الصورة تقريباً:

```go
func field(builder *strings.Builder, format string, args ...any) {
    builder.WriteString(fmt.Sprintf(format, args...))
}
```

وتستخدم الحقول فواصل خاماً مثل `|` دون escaping أو length-prefix. لذلك يمكن أن تنتج قيم مختلفة تمثيلاً وسيطاً واحداً قبل حساب SHA-256.

مثال تصادمي:

**الطلب A**

```text
Description = "x|entry.source_module=y"
SourceModule = "z"
```

**الطلب B**

```text
Description = "x"
SourceModule = "y|entry.source_module=z"
```

ينتج عن الحالتين التسلسل النصي نفسه:

```text
entry.description=x|entry.source_module=y|entry.source_module=z|
```

إذن الخلل ليس تصادماً في SHA-256؛ بل غموض في ترميز البيانات قبل الهاش. يجب إصلاح هذه النقطة واختبارها في كل من `FingerprintRequest` و`FingerprintCallerPayload`.

### 0.3 حالة نقطة إعادة التقييم

`BuildIdempotencyScope` يسمح لتلميح `IdempotencyRevaluation` باختيار العملية الثالثة بعد مطابقة الشركة والمفتاح، والاختبار الحالي يثبت أن هذا التلميح يتقدم على اشتقاق `reverse` من `ReversalOfID`. هذه ليست ملاحظة يجوز تغييرها تلقائياً؛ لأن ADR `P11-01` اعتمد حامل `PostOptions.Scope` لتجنب كسر توقيع `PostEntry` و`PostingExecutor`.

يجب في P13 حسم ما إذا كان مصدر التلميح موثوقاً ومحصوراً في مسار Revaluation الذي ينتج `RevaluationIdempotencyScope`، وهل يجوز له تجاوز تصنيف العكس. لا يُجرى تعديل على هذا السلوك قبل قرار موثق واختبار يغطي القرار.

### 0.4 حدود ما أثبتته المراجعة

- الأرشيف المرفوع لا يتضمن مجلد `.git`؛ لذلك لا يمكن إثبات Commit HEAD أو حالة working tree أو diff الحقيقي انطلاقاً من ZIP وحده.
- مستندات المشروع تذكر Commit مرجعياً، لكن لا يجوز افتراض أن محتوى الأرشيف مطابق لذلك Commit دون فحص Git في بيئة الوكيل.
- أداة المراجعة الحالية تستخدم Go 1.23.2 بينما `go.mod` يتطلب Go 1.26.3؛ لذلك لم تُعد هذه الخطة تشغيل اختبارات Go بنفسها.
- تقارير P0–P12 تُعامل كأدلة تاريخية. يجب إعادة تشغيل أوامر الاعتماد اللازمة على المصدر الدقيق قبل إصدار الحكم النهائي.

---

## 1. النتيجة المستهدفة ونطاق العمل

### 1.1 الهدف النهائي

إصدار `Kernel Final Assurance` الذي يحقق جميع الآتي:

1. كل قاعدة من `RULE-KERNEL-001` إلى `RULE-KERNEL-027` لها تعريف وتنفيذ واختبار قبول ورفض وأدلة تشغيل.
2. لا توجد بصمات غير ملتبسة أو عمليات Idempotency ذات تصنيف غير محسوم.
3. لا يوجد عيب Critical أو High مفتوح داخل Kernel.
4. جميع أدلة الاختبارات حديثة ومربوطة بـ Commit وبيئة وأمر ورمز خروج.
5. مصفوفة التتبع تشير إلى أسماء اختبار فعلية وتؤكد دلالة assertions.
6. أي استثناء تغطية مبرر ومراجع مستقلاً، ولا يخفي مساراً قابلاً للوصول.
7. مراجعة مستقلة منفصلة عن التنفيذ، بقرار وتاريخ ومراجع محدد.
8. يميز التقرير بين `Kernel GO` و`System/Product GO`؛ لا تُغلق فجوات المضيف أو الوحدات الخارجية بتقرير Kernel.

### 1.2 النطاق المسموح بالتعديل

جميع تغييرات الكود والاختبارات والوثائق والأدلة الجديدة أو المعدلة في هذه المهمة يجب أن تكون داخل:

```text
internal/kernal/**
```

يشمل ذلك:

- `types/`
- `journal/`
- `ledger/`
- `masterdata/`
- `policy/`
- `posting/`
- `valuation/`
- `governance/`
- `apperrors/`
- ملفات الجذر داخل `internal/kernal/` مثل الوثائق والتقارير ومصفوفة التتبع.

يجوز إنشاء أدلة الاختبارات تحت:

```text
internal/kernal/evidence/P13/<commit-sha>/
```

إذا وافق مالك المستودع على حفظ سجلات التشغيل داخل المصدر. إن كانت سياسة المستودع تمنع إدراج ملفات السجلات، تُحفظ كـ CI artifacts دائمة مع وضع manifest وبصمة SHA-256 للملفات داخل `internal/kernal/`.

### 1.3 ما يبقى خارج النطاق وغير قابل للتعديل

المجلدات التالية للقراءة والتشغيل والاختبار فقط، ولا يجوز تعديلها في P13:

- `pkg/**`
- `storage/**`
- `modules/**`
- `internal/orbitals/**`
- `test/**`
- `scripts/**`
- `docs/**` خارج `internal/kernal/**`
- `go.mod` و`go.sum`
- CI وأي ملفات جذرية أخرى.

يجوز تشغيل اختبارات المستودع أو contract tests بوصفها أدلة توافق قراءة فقط. إذا كشف الاختبار خللاً يتطلب تعديل مكون خارج Kernel، يُسجل كاعتماد/فجوة خارجية ويُرفع لمالك ذلك المكون؛ لا يُنفذ تعديل خارج النطاق دون موافقة جديدة صريحة.

### 1.4 ما لا يدخل النواة

لا تضف أو تعِد إدخال:

- RBAC أو Roles أو Permissions.
- Authentication أو Authorization أو Delegation.
- Maker-Checker أو موافقات المستخدمين وسير العمل.
- HTTP/REST/gRPC أو واجهات المستخدم.
- SQL أو Database Drivers أو محولات تخزين ملموسة.
- تقارير القوائم المالية أو BI أو واجهات Orbitals.
- وحدات IFRS قطاعية أو Use Cases خارج مسؤولية النواة.
- أرصدة مادية مخزنة أو طبقة تخزين Ledger داخل Kernel.

`ActorID` و`CreatedBy` و`PostedBy` تبقى فقط بيانات نسبة وتدقيق، ولا تُحوّل إلى نظام صلاحيات.

---

## 2. قواعد التنفيذ الصارمة

هذه القواعد إلزامية في كل مرحلة. خرق أي قاعدة يوقف المرحلة حتى التصحيح.

### 2.1 مراجعة قبل أي تعديل

1. افتح الملف كاملاً قبل تعديله؛ لا تعتمد على مقتطف أو نتيجة `grep` وحدها.
2. ارسم مسار الاستدعاء من نقطة الدخول إلى كل دالة متأثرة، وافحص جميع المستهلكين داخل Kernel.
3. افحص الاختبارات الحالية والوثائق/ADR ذات الصلة قبل كتابة التعديل.
4. ميّز بين خلل مثبت، ومخاطرة محتملة، وقرار معماري. لا تعرض الاستنتاج على أنه حقيقة إذا لم يثبته المصدر.
5. عند تعارض الكود مع ADR معتمد، أوقف التعديل واطلب قراراً؛ لا تتجاوز ADR ضمنياً.
6. قبل كل مرحلة، سجّل ملفاتها المتوقعة والملفات التي ستبقى للقراءة فقط.

### 2.2 حدود النطاق والتغييرات

1. يمنع أي تعديل خارج `internal/kernal/**`.
2. يمنع تعديل `go.mod` أو `go.sum` أو scripts أو CI في هذه المهمة.
3. لا تُعدّل أو تحذف اختباراً ناجحاً لإخفاء فشل؛ إذا أصبح الاختبار قديماً، أضف اختباراً بديلاً ثم اشرح سبب إزالته ضمن مراجعة مستقلة.
4. لا تغيّر توقيعاً عاماً أو قيمة enum أو صيغة fingerprint أو error contract دون تحليل التوافق وقرار موثق.
5. لا تضف Dependency جديدة.
6. لا تعِد تسمية/نقل الحزم لمجرد التجميل.
7. لا تستخدم `float32` أو `float64` في تمثيل أو حساب الأموال.

### 2.3 منهج الاختبار والتعديل

كل إصلاح يتبع دورة:

1. **RED:** أضف اختباراً يثبت المشكلة قبل الإصلاح، وتأكد أنه يفشل للسبب المتوقع.
2. **GREEN:** نفذ أصغر تعديل يعالج السبب دون توسيع السلوك غير المطلوب.
3. **REGRESSION:** شغّل اختبارات الحزمة كاملة، ثم كل Kernel.
4. **RACE/PROPERTY:** شغّل اختبارات التزامن والخصائص المتأثرة.
5. **REVIEW:** راجع الفرق النهائي وتأكد أن الاختبار لا يمر بسبب compile/setup error أو skip غير مقصود.

لا تقبل اختباراً يثبت فقط عدم حدوث Panic إذا كانت القاعدة تتطلب إثبات قبول/رفض أو نتيجة مالية محددة.

### 2.4 لا تخفِ الأخطاء

- لا تستخدم fallback يحوّل خطأ العملة أو الدقة إلى قيمة تقريبية افتراضية.
- لا تتجاهل أي error عبر `_ =` أو `continue` دون تعليل موثق.
- لا تحوّل فشل build أو فشل البيئة إلى Mutation KILLED.
- لا تعتبر فشل اتصال أو غياب Toolchain دليلاً على فشل الكود؛ سجله كـ `ENVIRONMENT_BLOCKED`، ولا تمنحه PASS.
- لا تعتبر اختباراً `PASS` إذا تم `Skip` بسبب غياب متطلب أساسي من دون بوابة بيئية معتمدة.

### 2.5 إدارة الأدلة

كل نتيجة تعتمد في بوابة يجب أن تحتوي:

- Commit SHA الفعلي.
- SHA-256 للملف/الأرشيف عند اختبار ZIP.
- تاريخ/وقت UTC.
- إصدار Go وToolchain وOS/ARCH.
- الأمر كاملاً.
- رمز الخروج الفعلي.
- ملف السجل الخام أو رابط artifact دائم وبصمته.
- ملخصاً لا يحل محل السجل.

لا تعتمد على `/tmp` وحده. لا تعتمد على سطر `RESULT: PASS` إذا لم يكن script يمرّر رمز خروج كل خطوة إلى النتيجة الكلية.

### 2.6 المراجعة المستقلة

- المنفذ لا يكون المراجع الوحيد.
- يجب أن يسجل المراجع اسمه/معرفه، التاريخ، Commit الذي راجعه، checklist والقرار.
- أي بوابة تنتظر المراجعة لا تعد PASS.
- لا يجوز للمراجع أن يراجع نسخة مختلفة عن النسخة التي شُغّلت عليها الاختبارات.

### 2.7 حالات البوابات

| الحالة | التعريف الملزم | أثرها |
| --- | --- | --- |
| `PASS` | كل معايير البوابة محققة بأدلة حديثة قابلة لإعادة الإنتاج؛ لا Critical/High داخل النطاق؛ المراجع المستقل وافق | يسمح بالانتقال للمرحلة التالية إذا استوفيت الاعتماديات |
| `CONDITIONAL` | يوجد Medium/Low أو اعتماد خارجي موثق، بمالك وموعد ومسار إغلاق، ولا يؤثر على سلامة Kernel ضمن النطاق | لا يعني Release GO، ولا يسمح بفتح مرحلة تعتمد على العنصر المؤجل |
| `FAIL` | Critical/High داخل النطاق، أو اختبار إلزامي فاشل، أو دليل غير موثوق، أو Scope violation، أو غياب مراجعة مستقلة في بوابة الاعتماد | توقف؛ أصلح ثم أعد كامل بوابة المرحلة |

لا تستخدم `PASS-SCOPED` كحالة رابعة. بدلاً من ذلك اكتب `PASS` مع حقل Scope واضح، وميّز `Kernel GO` عن `System/Product GO`.

---

## 3. معرفات نتائج P13 الجديدة

لا يُعاد استخدام معرفات فجوات G-001 إلى G-025. تُسجل النتائج الجديدة في وثيقة داخل Kernel باسم:

`KERNEL_P13_FINDINGS_REGISTER.md`

وتستخدم المعرفات التالية، مع السماح بإضافة KFR جديدة عند ظهور دليل:

| ID | الأولوية المبدئية | الموضوع | الحالة عند إعداد الخطة |
| --- | --- | --- | --- |
| `KFR-001` | Critical | غموض canonical encoding في `FingerprintRequest` و`FingerprintCallerPayload` | Open — مثبت بالمراجعة الساكنة |
| `KFR-002` | High / Decision | حدود الثقة ودلالة تلميح `IdempotencyRevaluation`، خصوصاً مع `ReversalOfID` | Pending Decision — السلوك الحالي متعمد في اختبار ومرتبط بـ ADR |
| `KFR-003` | High | توافق بصمات v1 المخزنة مع أي إصدار v2 جديد | Pending Compatibility Assessment |
| `KFR-004` | Medium | أدلة مستقلة على سلامة مشغلات Fuzz/Mutation وموثوقية رموز الخروج | Open — أداة خارجية للقراءة فقط |
| `KFR-005` | Medium | تعريف سلطة اعتماد PASS/CONDITIONAL/FAIL والمراجع المستقل | Open — حوكمة وإثبات |
| `KFR-006` | Medium | اتساق حالات G-007 وG-018 وفرق Kernel GO عن System GO | Open — توثيق حدود النطاق |

أي نتيجة إضافية يجب أن تحتوي: دليل/موضع، الأثر، السبب الجذري، الأولوية، المالك، القرار، الاختبار، المرحلة، وحالة الإغلاق.

---

# 4. المراحل التنفيذية المتسلسلة

## المرحلة P13-0 — مراجعة المصدر وتثبيت خط الأساس

### المشكلة التي تحلها

الأرشيف المضغوط لا يتضمن `.git`، لذلك لا يمكن منه وحده إثبات Commit أو نظافة working tree أو أن تقارير P12 تطابق المصدر الفعلي. كما أن بعض تقارير المراحل القديمة تشير إلى Commit مختلف أو لا تحمل بصمة المصدر الحالي.

### الهدف

تحديد نسخة واحدة غير ملتبسة ستكون موضوع كل الإصلاحات والاختبارات والمراجعة. لا يسمح بالتعديل قبل إتمام هذه المرحلة.

### النطاق

- قراءة فقط للمستودع كله لأغراض الهوية والاعتماديات.
- التعديلات المسموحة في هذه المرحلة: لا توجد.
- لا تعديل خارج `internal/kernal` أو داخله.

### المهام المتسلسلة

1. افتح المستودع الحقيقي في working tree يحتوي `.git`. إذا كان المتاح ZIP فقط، لا تدّعِ وجود Commit؛ سجّل ذلك كقيد وأطلب من الوكيل تشغيل الخطة داخل Git checkout.
2. سجّل `git rev-parse HEAD`، اسم الفرع، `git status --short`، وهل يوجد uncommitted diff.
3. سجل بصمة SHA-256 للأرشيف الأصلي ولنسخة المصدر التي ستُختبر.
4. سجل `go version` و`go env GOVERSION GOOS GOARCH GOTOOLCHAIN GOPATH GOMOD`.
5. تحقق من مطابقة Toolchain إلى `go1.26.3` كما يطلب `go.mod`؛ لا تسمح بتنزيل صامت لإصدار غير مثبت في سجل التشغيل.
6. اجرد ملفات `internal/kernal`، حزم Go، ملفات الإنتاج، ملفات الاختبار، أهداف Fuzz، وثائق Kernel.
7. افحص ملفات الوثائق المرجعية فقط دون تعديلها: خطة P0–P12، تقارير المراحل، Gap Register، Plan Review، Sign-off، ADR P11-01، invariants، traceability، coverage exceptions، mutation report.
8. قارن ملخص Gap Register مع صفوف G-007 وG-018، وسجل أي تناقض دون تعديل `docs/**`.
9. افحص نسخة `transaction.go` ومواضع استدعاء fingerprint من SDK/Storage بوصفها اعتماديات قراءة فقط.
10. أنشئ داخل `internal/kernal/` ملف `KERNEL_P13_BASELINE.md` يسجل المعلومات السابقة ونتيجة المراجعة.

### معايير القبول والنجاح

- يوجد Commit SHA معلوم أو تم التصريح بوضوح أن المصدر ZIP-only ولا يمكن إثبات Commit.
- يوجد SHA-256 للأرشيف والمصدر.
- Toolchain المطلوب محدد ومتحقق قبل الاختبارات.
- جرد Kernel مكتمل ولا توجد حزم/ملفات اختبار غير مدرجة.
- قائمة الملفات المتوقع تعديلها في P13 محددة.
- لا توجد تغييرات غير مصرح بها قبل بداية التنفيذ.

### أدلة الإغلاق

- `internal/kernal/KERNEL_P13_BASELINE.md`
- سجل أوامر الهوية والبيئة مع exit codes.
- manifest لجرد الملفات وبصماتها.

### قرار البوابة

`PASS` فقط إذا أمكن تحديد المصدر الدقيق. إذا لم يتوفر Git، يمكن الانتقال إلى تحليل ZIP لكن لا يتم منح Release GO نهائي حتى يعاد التحقق داخل Git checkout.

---

## المرحلة P13-1 — تصميم ترميز Canonical آمن للبصمات

### المشكلة التي تحلها

التمثيل النصي الحالي في `posting/transaction.go` يضم قيم الحقول مباشرةً مع `|`؛ لذلك يمكن لقيم نصية مختلفة أن تنتج تسلسلاً نصياً واحداً قبل الهاش. ينتج عن ذلك احتمال تطابق `FingerprintRequest` أو `FingerprintCallerPayload` لحمولتين مختلفتين دون أي كسر لـ SHA-256.

### الهدف

تعريف ترميز canonical غير ملتبس، محدد الإصدار، حتمي، وموحد لكل حقول طلب الترحيل وحمولة المتصل.

### النطاق

- داخل Kernel: `posting/transaction.go` واختباراته وملفات توثيق Fingerprint داخل `internal/kernal/`.
- ملفات للقراءة فقط: `pkg/sdk/**`, `pkg/spi/**`, `storage/**`, ADR P11-01 وعقود idempotency الخارجية.
- لا تعديل على Adapter أو SDK في هذه المرحلة.

### المهام المتسلسلة

1. جرد جميع الحقول التي تدخل `FingerprintRequest`.
2. جرد جميع الحقول التي تدخل `FingerprintCallerPayload`.
3. تصنيف كل حقل: string، integer، decimal، timestamp، typed ID، optional pointer، enum، list.
4. تحديد سياسة canonical لكل نوع قبل التنفيذ:
   - string: UTF-8 bytes مع tag ونوع وطول بايتات محدد، ثم المحتوى.
   - optional: علامة وجود منفصلة؛ nil لا يساوي empty string أو zero value.
   - decimal: تمثيل exact canonical لا يمر بتقريب display.
   - timestamp: UTC، مع دقة ثابتة متوافقة مع عقد الحفظ.
   - enum: القيمة القانونية الموثقة؛ القيمة غير المعروفة تُرفض إذا كان المجال مغلقاً.
   - list: عدد العناصر + ترتيب حتمي + تمثيل كل عنصر بترميز مستقل.
   - IDs: نوع المعرف + canonical string؛ لا تدمج أنواع ID متشابهة النص.
5. تحديد ما إذا كانت أسطر القيد تُرتب حسب `LineID`; تحقق من أن معرفات الأسطر الفارغة/المكررة ترفض قبل حساب بصمة حتمية، أو وثق لماذا يمكن قبولها.
6. اختيار encoder واحد داخل `posting` يدعم `WriteTag`, `WriteString`, `WriteBytes`, `WriteOptional`, `WriteInt`, `WriteDecimal`, `WriteTimestamp` و`WriteCount`، مع اختبارات مستقلة له.
7. استخدم length-prefix بالبايتات أو ترميزاً ثنائياً مكافئاً لا يقبل غموض الفواصل. لا تستخدم `fmt.Sprintf` مع فاصل نصي خام لحقول غير موثوقة.
8. أضف Domain Separation واضحاً، مثل `posting-request-fingerprint-v2` و`posting-caller-payload-v2`، حتى لا تتشارك عمليات مختلفة نفس مجال الهاش.
9. قرر شكل قيمة الإرجاع: يجب أن يكون versioned أو أن يكون الإصدار مثبتاً بطريقة موثقة في preimage والعقد؛ لا تغيّر الصيغة عشوائياً.
10. جهّز تقييم توافق v1/v2 قبل أي تغيير إنتاجي. راجع ما إذا كان هناك idempotency record تاريخي يحمل v1، وما هي مدة احتفاظه، وكيف يتصرف Replay بعد نشر v2.
11. إذا تعذر ضمان replay للطلبات التاريخية دون تعديل Adapter/SDK، أوقف cutover واطلب قراراً من المالك. لا توسع نطاق التعديل خارج Kernel تلقائياً.
12. أنشئ ملفاً داخل Kernel يشرح Canonical Fingerprint Contract ونسخة الترميز والحقول الداخلة والمستثناة.

### معايير القبول والنجاح

- لا توجد طريقة لتكوين preimage متطابق من قيم حقول مختلفة ضمن النوع نفسه.
- empty string وnil وzero و`"-"` ليست متساوية إلا إذا قرر العقد ذلك صراحة.
- ترتيب أسطر مختلف مع نفس المعنى ينتج البصمة نفسها إذا كان الترتيب غير دلالي.
- تغيير أي حقل دلالي ينتج بصمة مختلفة.
- الحقول غير الدلالية المستثناة محددة ومبررة.
- التمثيل لا يعتمد على map iteration غير الحتمي.
- اختبار توافق v1/v2 يبين سلوك Replay في حالات المفتاح القديم والجديد؛ لا يتم تجاوز هذا الاختبار بعبارة «نادر الحدوث».

### أدلة الإغلاق

- جدول جميع الحقول في كل Fingerprint API.
- اختبارات collision regressions.
- Canonical encoding specification.
- قرار v1/v2 compatibility موثق.

### قرار البوابة

أي احتمال لتطابق preimage لحمولتين مختلفتين = `FAIL`. أي غموض غير محسوم في replay التاريخي = `CONDITIONAL` ويمنع cutover إلى v2.

---

## المرحلة P13-2 — إصلاح بصمات Posting واختبارات التصادم

### المشكلة التي تحلها

المشكلة المثبتة في المرحلة السابقة لا تغلق بوثيقة encoder فقط؛ يجب ربط `FingerprintRequest` و`FingerprintCallerPayload` به وتوفير اختبارات تمنع إعادة ظهور الغموض.

### الهدف

إزالة `field()` ذي الصيغة النصية غير الملتبسة من المسارات الحساسة، واعتماد encoder واحد لكل البصمات.

### النطاق

- `internal/kernal/posting/transaction.go`
- `internal/kernal/posting/transaction_contract_test.go`
- `internal/kernal/posting/revaluation_scope_hint_test.go` عند الحاجة لاختبارات التوافق فقط
- وثائق البصمات داخل Kernel.
- ممنوع تعديل SDK/Storage/SPI.

### المهام المتسلسلة

1. اكتب اختبار RED للحالتين التصادميتين في `FingerprintRequest`.
2. اكتب الاختبارين نفسيهما لـ `FingerprintCallerPayload`.
3. أضف حالات لحقول `Description`, `SourceModule`, `ReferenceID`, `MatchingNumber`, `AdjustmentReason`, `ReversalReason`, `Reason`, `ActorID` وكل string يدخل بصمة.
4. أضف حالات قيمة تحتوي على `|`, `=`, `:`, newline, tab, NUL إن كان العقد يسمح، Unicode عربي، combining characters، وUTF-8 متعددة البايتات.
5. أضف اختبارات nil مقابل empty للحقول الاختيارية.
6. أضف اختبار تغير كل حقل دلالي واحداً تلو الآخر.
7. أضف اختبار ثبات الترتيب: تبديل أسطر القيد لا يغير البصمة إذا كانت الأسطر مرتبة canonical حسب ID.
8. أضف اختبار فشل معرف سطر مكرر/فارغ إذا كانت هذه من شروط هوية القيد.
9. نفذ canonical encoder الجديد ومرر جميع حقول الطلب والحمولة عبره.
10. احذف أو احصر استخدام `field()` القديم؛ يجب ألا يبقى أي مسار إنتاجي للبصمة يستخدم format-string separator.
11. أنشئ Golden Vector حقيقياً: مدخل ثابت وبصمة متوقعة hard-coded محسوبة خارج الدالة أو بمرجع مستقل، لا استدعاء الدالة مرتين ومقارنة نتيجتيها.
12. أضف اختبار Domain Separation يثبت اختلاف بصمة Request عن Caller Payload وعن Revaluation Payload.
13. شغّل اختبارات `posting` كاملة و`go test -race` للحزمة.
14. راجع compatibility مع `CallerFingerprint` و`RequestFingerprint`؛ إذا كان هناك حقل يصل للمضيف بتنسيق مختلف، وثق أثره ولا تعدله خارج Kernel.

### معايير القبول والنجاح

- جميع collision regression tests تفشل على V1 وتنجح على V2.
- جميع حقول النصوص مغطاة بحالات تحتوي فواصل.
- كل حقل دلالي يغير البصمة عند تغييره.
- الترتيب غير الدلالي للأسطر لا يغير البصمة.
- Golden Vector يقارن قيمة ثابتة مستقلة.
- لا توجد استدعاءات للـ legacy `field()` داخل مسار Fingerprint.
- جميع اختبارات الحزمة تنجح دون Skip غير مبرر.

### أدلة الإغلاق

- `transaction_fingerprint_v2_test.go` أو دمج الاختبارات في ملف العقد الحالي.
- `KERNEL_FINGERPRINT_CONTRACT.md` داخل Kernel.
- نتائج Unit/Race وملفات السجل.

### قرار البوابة

أي collision regression يمر أو أي حقل دلالي لا يدخل البصمة = `FAIL`.

---

## المرحلة P13-3 — حسم حدود Revaluation Idempotency Scope

### المشكلة التي تحلها

`BuildIdempotencyScope` يقبل `opts.Scope.Operation == IdempotencyRevaluation` إذا تطابقت الشركة والمفتاح، حتى إذا كان `entry.ReversalOfID` غير nil. والاختبار `valid hint takes precedence over reversal derivation` يثبت هذا السلوك. ينص ADR P11-01 على أن التلميح هو حامل محايد للعملية الثالثة، لكنه لا يوضح بما يكفي حدود الثقة الدلالية عندما يكون القيد عكسياً.

### الهدف

منع التصنيف الخاطئ للعملية أو namespace idempotency، مع المحافظة على قرار ADR أو تحديثه بقرار صريح قبل تغيير السلوك.

### النطاق

- Kernel: `posting/transaction.go`، `posting/revaluation_scope_hint_test.go`، اختبارات `valuation` ذات الصلة، وثيقة قرار داخل `internal/kernal/`.
- قراءة فقط: `docs/ard/P11_01_revaluation_idempotency_spi.md`، `pkg/sdk/application.go`، `pkg/spi/storage/**`، `storage/**`.
- لا تعديل للعقد الخارجي أو SDK.

### المهام المتسلسلة

1. ارسم مصدر إنشاء `RevaluationIdempotencyScope` من `valuation/revaluation.go` حتى وصوله إلى `PostOptions.Scope`.
2. راجع استعمال `BuildIdempotencyScope` في `PostEntry` وفي فحص SDK السابق للترحيل.
3. راجع ADR P11-01 كاملاً، خاصة سبب عدم توسيع توقيع `PostEntry`، والحاجة إلى استخدام `RevaluationPayloadFingerprint` بدلاً من FingerprintRequest العادي.
4. صِف نموذج التهديد: من يستطيع إنشاء `PostOptions`؟ هل هو Host داخلي موثوق فقط أم يمكن لمتصل عام تمرير Scope arbitrary؟ لا تفترض الإجابة.
5. احسم القرار كتابةً داخل `internal/kernal/KERNEL_P13_DECISIONS.md`:
   - الخيار A: الحفاظ على السلوك الحالي لأن المضيف وحده ينشئ التلميح، مع إثبات هذا الحد بعقد واضح واختبارات.
   - الخيار B: جعل `reverse` ذا أولوية مطلقة ورفض Revaluation hint عندما يحتوي القيد على `ReversalOfID`، مع تحديث اختبار precedence وقرار ADR في نطاق Kernel بوصفه قراراً مقترحاً يحتاج مالك ADR الخارجي إلى اعتماده.
   - الخيار C: إدخال proof/capability typed داخل Kernel إذا كان ذلك ممكناً دون توسيع غير مبرر للعقد العام.
6. لا تختَر A فقط لأنه السلوك الحالي، ولا B فقط لأنه يبدو أكثر تحفظاً؛ وثّق trade-off والأثر على storage operation وaudit action وreplay.
7. أضف اختبارات: hint صحيح مع قيد Revaluation حقيقي، hint مزيف مع شركة أخرى، مفتاح آخر، مفتاح فارغ، قيد reversal مع hint، قيد regular مع hint، ومحاولة إعادة استخدام key عبر العمليات post/reverse/revaluation.
8. أثبت أن Revaluation يستخدم `RevaluationPayloadFingerprint` v2 ولا يستخدم `FingerprintRequest` أو `FingerprintCallerPayload` كبصمة نتيجة التقييم.
9. اختبر تصنيف `ErrRevaluationAlreadyApplied` و`ErrIdempotencyConflict` و`ErrReversalAlreadyApplied` من حيث retryability.
10. وثق حدود التخزين الدائم: `RevaluationRegistry` وحده ليس ضماناً بعد restart؛ الضمان الدائم يعتمد على Host/Adapter وفق ADR، ولا تنسبه إلى Kernel وحدها.

### معايير القبول والنجاح

- يوجد قرار معماري صريح ومراجع من مالك النواة.
- لا يوجد مسار يخلط بين post/reverse/revaluation دون اختبار وقرار.
- الشركة والمفتاح يستمدان من المصادر المعتمدة؛ لا يسمح للتلميح باختراع شركة أو مفتاح.
- بصمة نتيجة Revaluation مستقرة ولا تتضمن معرّفات توليد عشوائية إذا كان ADR يستثنيها.
- أخطاء العملية مصنفة retryable/non-retryable بصورة متسقة.
- لا تغيير في توقيع `PostEntry` أو `PostingExecutor` ما لم يصدر ADR جديد صريح.

### أدلة الإغلاق

- `KERNEL_P13_DECISIONS.md`.
- جدول مصدر التلميح وحدود الثقة.
- اختبارات scope وreplay وoperation isolation.

### قرار البوابة

لا تنفذ تغييراً سلوكياً قبل قرار المرحلة. غياب القرار = `CONDITIONAL`، واختبار يثبت خلطاً غير مقصود للعملية = `FAIL`.

---

## المرحلة P13-4 — تدقيق Audit Snapshot وموثوقية عقد Repository

### المشكلة التي تحلها

لدى المشروع عقد متقدم للقراءة المتسقة ونقطة التحقق، لكن يجب إثبات أن التحقق داخل Kernel يفهم اللقطة وحدودها بصورة صحيحة، وأن التقدم المتزامن لا يُصنّف فساداً، وأن الفساد الحقيقي لا يُخفى كتعذر إعادة المحاولة.

### الهدف

تثبيت دلالة Snapshot وCheckpoint والتسلسل والهاش في Kernel، دون إدخال قاعدة بيانات أو قفل فعلي إلى النواة.

### النطاق

- Kernel: `governance/audit_chain.go`, `governance/verifier.go` واختبارات governance.
- العقود الخارجية تقرأ فقط؛ لا تعديل في `storage/**` أو `pkg/spi/**`.

### المهام المتسلسلة

1. اقرأ عقد `AuditVerificationSnapshotRepository` و`AtomicAuditLogRepository` كاملاً.
2. وثق invariants اللقطة: company, checkpoint sequence, snapshot sequence, checkpoint hash, first/last log, ordering.
3. تحقق من أن كل سجل داخل اللقطة يحمل CompanyID المطلوب.
4. تحقق من أن تسلسل السجلات متصل من نقطة البداية حتى حد اللقطة.
5. تحقق من `PreviousLogHash` و`CurrentLogHash` لكل سجل.
6. تحقق من مطابقة hash عند التسلسل الذي تمثل عليه نقطة التحقق.
7. لا تعتبر إضافة سجل بعد snapshot فساداً؛ يجب استبعاده من اللقطة الحالية.
8. إذا تقدمت checkpoint بعد الالتقاط، يجب أن يعاد الالتقاط أو يعاد التحقق وفق السياسة الموثقة؛ لا تصنف الحالة تلقائياً tampering.
9. استمر في اعتبار عدم تطابق hash داخل لقطة ثابتة موثوقة فساداً فعلياً.
10. تحقق من حد محاولات Retry وتأكد أن exhaustion يرجع خطأ اتساق قابلاً لإعادة المحاولة لا `IntegrityViolation`.
11. أضف اختبارات deletion-middle, deletion-tail, reorder, duplicate sequence, wrong CompanyID, altered reason/payload, stale checkpoint, concurrent append, concurrent checkpoint advance.
12. تحقق أن `CalculateHash` يحمي كل حقل دلالي بقيمة Golden ثابتة؛ لا تكتفِ بمقارنة نتيجتين من نفس implementation.
13. افحص النسخ الدفاعي لكل slices/maps/pointers التي تعيدها واجهات Governance.
14. حافظ على المسؤولية الخارجية للنسخة المستقلة من checkpoint والتخزين الفعلي؛ Kernel يتحقق من العقد ولا يدعي ضمان الاحتفاظ الخارجي.

### معايير القبول والنجاح

- لا قبول لسجل معدل ضمن snapshot ثابتة.
- لا false positive على append طبيعي خارج حد اللقطة.
- لا الخلط بين retryable consistency conflict وnon-retryable integrity violation.
- فشل مغلق عند غياب checkpoint إذا كان العقد يشترطه لكشف tail deletion.
- جميع اختبارات التزامن تمر عبر `-race`.
- لا تعديلات على Adapter أو SQL أو SPI في هذه المرحلة.

### أدلة الإغلاق

- `governance` unit/race logs.
- جدول حالات snapshot/checkpoint ونتائجها.
- تحديث تقرير التدقيق داخل `internal/kernal/`.

### قرار البوابة

أي false negative في tampering أو false positive غير مبرر في concurrent append = `FAIL`.

---

## المرحلة P13-5 — تدقيق موثوقية Fuzz وMutation وCoverage evidence

### المشكلة التي تحلها

بعض مشغلات المستودع خارج Kernel لا تقدم دليلاً كافياً بمفردها: `verify_kernel_baseline.sh` لا يرفع فشل Fuzz smoke إلى متغير النتيجة العامة، و`run_mutation_tests.sh` يعتبر كل خروج غير صفري من go test قتلاً للطفرات حتى لو كان build/setup failure، و`check_coverage.sh` يبدأ بحد 80% لا حد الاعتماد 90%.

### الهدف

إنتاج أدلة يمكن تكرارها والتحقق من رمز خروجها، من دون تعديل scripts خارج Kernel.

### النطاق

- الاختبارات والوثائق/manifest داخل Kernel.
- قراءة وتشغيل scripts خارج Kernel فقط.
- ممنوع تعديل `scripts/**` أو CI.

### المهام المتسلسلة

1. جرد جميع أهداف Fuzz داخل `internal/kernal` ومطابقتها بالاختبارات في الملفات الحالية.
2. تثبيت مدة موحدة لكل هدف (التوصية: 30 ثانية في مرحلة release، ويمكن استخدام smoke أقصر في التطوير).
3. استخدم `scripts/run_fuzz_all.sh` الذي يلتقط رمز خروج كل هدف، ولا تعتمد على `verify_kernel_baseline.sh` منفرداً لإثبات Fuzz بسبب قصور تجميع return code في حلقة smoke.
4. شغل كل Fuzz target مع حفظ stdout/stderr ورمز الخروج لكل هدف.
5. إذا ظهر interesting corpus جديد، قلّصه واحفظه في `testdata/fuzz` داخل الحزمة المسؤولة، ثم أعد تشغيل الهدف.
6. افصل fuzz timeout عن fuzz failure وعن environment/toolchain failure.
7. راجع `coverage.out` من المصدر الحالي بعد إعادة توليده، لا تستخدم ملفاً قديماً في الأرشيف.
8. احسب تغطية `internal/kernal/...` حصراً، مع ذكر statement coverage وعدم تسميتها branch coverage.
9. احسب تغطية الحزم الحرجة على حدة وفق العتبات المعتمدة. أي استثناء يجب أن يحدد الدالة والفرع وسبب عدم الوصول وأثره وقرار المراجع.
10. لا تعتمد على `check_coverage.sh` بقيمته الافتراضية 80% لإثبات حد 90%. نفذ مقارنة صريحة مع 90% على profile الحديث، واحتفظ بالحساب والرمز.
11. أعد فحص `scripts/run_mutation_tests.sh` باعتباره أداة خارج النطاق. لكل طفرة تحقق من أن التعديل طُبق فعلاً، وأن package/build نجح قبل التوقع بقتل الطفرة، وأن الفشل من assertion المستهدف لا من syntax/compile/setup.
12. نفذ Mutation في disposable worktree أو نسخة مؤقتة نظيفة، واحسب SHA-256 للملف قبل الطفرة وبعد restoration، وأثبت أن working tree عاد كما كان.
13. صف نتيجة 8/8 بأنها «8 طفرات مستهدفة قُتلت» لا «Mutation Score شامل 100%» ما لم تستخدم أداة mutation عامة وتحدد denominator.
14. احفظ جميع logs وsummaries وtool versions في دليل evidence خاص بالـ commit.

### معايير القبول والنجاح

- كل Fuzz target في inventory نُفذ وخرج برمز 0.
- لا interesting corpus فاشل أو غير محفوظ.
- نسبة Kernel الإجمالية تحقق الحد المعتمد؛ الهدف في الخطط السابقة ≥90%.
- الحزم الحرجة تحقق عتباتها أو لها استثناءات مراجعة مستقلة ومحددة.
- كل mutation KILLED بسبب assertion الذي يستهدفه، وليس لأن build لم ينجح.
- كل الملفات المعدلة أثناء Mutation restored إلى byte-identical baseline.
- جميع أدلة التشغيل قابلة لإعادة الإنتاج ومرتبطة بـ commit.

### أدلة الإغلاق

- `KERNEL_P13_TEST_EVIDENCE.md`.
- `coverage.out` حديث أو artifact دائم مع SHA-256.
- سجل Fuzz لكل هدف.
- تقرير Mutation دقيق يذكر الأداة/الطفرات/denominator/آلية التحقق.
- `KERNEL_COVERAGE_EXCEPTIONS.md` محدث عند الحاجة.

### قرار البوابة

لا تمنح PASS على أساس script summary إذا ثبت أنه لا ينقل exit code؛ لا تمنح PASS على Mutation إذا كان سبب فشل test غير assertion؛ لا تمنح PASS على coverage من profile قديم.

---

## المرحلة P13-6 — تدقيق Traceability وسجل الفجوات والقرارات

### المشكلة التي تحلها

رغم أن أسماء الاختبارات قابلة للمطابقة آلياً، فإن المراجعة السابقة لخطة P0–P12 حددت مسائل في سلطة القرار المستقل وتعريف PASS/CONDITIONAL/FAIL، وتوجد تناقضات في حالة الفجوات الخارجية G-007 وG-018. كما أن المطابقة الاسمية لا تثبت أن assertion يختبر القاعدة المقصودة.

### الهدف

جعل كل قاعدة محاسبية قابلة للتتبع من المتطلب إلى الدالة إلى الاختبار إلى الدليل، وإصدار حالة صادقة تفصل Kernel عن النظام الكامل.

### النطاق

- ملفات التوثيق داخل `internal/kernal/**` فقط.
- `docs/**` وADRs خارج Kernel للقراءة فقط.

### المهام المتسلسلة

1. اقرأ قائمة القواعد الـ27 من `KERNEL_ACCOUNTING_INVARIANTS.md`.
2. لكل قاعدة، حدد: Rule ID، نص القاعدة، الحزمة، الملف، الدالة، الاختبار الإيجابي، الاختبار السلبي، الحدود، fuzz/property عند انطباقها، ورقم evidence.
3. لا تكتفِ بوجود اسم Test: افتح assertion وتحقق أنه يفحص النتيجة المطلوبة فعلاً.
4. تحقق من أن الاختبارات لا تعتمد على mocks تسمح بالمسار غير الصحيح دون كشفه.
5. ضع كل مرجع لا يطابق اختباراً فعلياً في قائمة Missing، ولا تُخفِه بذكر اختبار قريب دون شرح العلاقة.
6. افحص أن كل error جديد له test لتصنيفه (retryable/non-retryable/validation/conflict/integrity).
7. افحص قيم enums والـ serialized strings التي لها بيانات محفوظة؛ اربطها بعقد التوافق.
8. أعد مراجعة `KERNEL_COVERAGE_EXCEPTIONS.md` مقابل profile حديث؛ احذف الاستثناءات التي غطيت، وأعد تصنيف الفروع القابلة للوصول.
9. راجع `KERNEL_MUTATION_REPORT.md` وميّز الطفرات المستهدفة عن Mutation score العام.
10. راجع `KERNEL_REMEDIATION_GAP_REGISTER.md` للقراءة فقط:
    - G-007: مخاطر خارجية تخص repo/CI، تبقى out-of-scope مع مالك واضح.
    - G-018: الصف التفصيلي يقول Open في وحدات/Orbitals؛ لا تعرضها كـ Closed لمجرد أنها خارج Kernel.
    - صغ الحكم: `Kernel GO` لا يعني `System/Product GO`.
11. أضف داخل Kernel ملحقاً يبين الفجوات الخارجية المفتوحة وتأثيرها على نطاق المنتج دون تعديل سجل الفجوات الخارجي.
12. راجع نتيجة `KERNEL_REMEDIATION_PLAN_REVIEW.md`: C1/C2 حول المراجع المستقل وتعريف القرار يجب أن تكون مغلقة داخل وثيقة اعتماد P13 حتى لو بقيت الخطة التاريخية كما هي.
13. عرّف حالات PASS/CONDITIONAL/FAIL وفق قسم القواعد الصارمة من هذه الخطة.
14. اجعل كل تقرير مرحلة يستخدم معرّفات المهام الأصلية أو معرفات P13 المستقرة، ويعرض PASS/PARTIAL/N/A مع السبب.

### معايير القبول والنجاح

- 27/27 قاعدة لها دالة مسؤولة واختبارات دلالية.
- لا توجد أسماء اختبار مفقودة أو مراجع غير دقيقة.
- كل صف يوضح دليل القبول والرفض، أو سبب N/A.
- الاستثناءات تتطابق مع coverage profile الحالي.
- G-018 لا توصف بأنها مغلقة إذا بقي صفها Open خارجياً.
- التقرير يميز صراحةً Kernel GO عن System/Product GO.
- لكل حالة PASS مراجع مستقل وتاريخ وcommit.

### أدلة الإغلاق

- `KERNEL_P13_TRACEABILITY_REVIEW.md`.
- `KERNEL_P13_EXTERNAL_DEPENDENCIES.md`.
- تحديث `KERNEL_TRACEABILITY_MATRIX.md` داخل Kernel.
- سجل قرار C1/C2 داخل وثيقة P13.

### قرار البوابة

أي قاعدة بلا تنفيذ أو اختبار فعلي = `FAIL`. أي اختلاف توثيقي غير محسوم يؤثر على Release scope = `CONDITIONAL` على الأقل.

---

## المرحلة P13-7 — تدقيق دلالي شامل لحزم Kernel

### المشكلة التي تحلها

النجاح في الاختبارات المحددة لا يثبت أن كل عقد عام أو مسار خطأ أو نتيجة projection آمن. يلزم تدقيق نهائي عابر للحزم للتأكد من عدم وجود ثغرات إضافية غير مسجلة، مع تجنب إعادة كتابة ما ثبتت صحته.

### الهدف

إجراء مراجعة محددة بنطاقات واضحة لكل حزمة، وفتح إصلاح فقط عند وجود دليل قابل لإعادة الإنتاج.

### النطاق

`internal/kernal/**` فقط. أي اختبار تكامل خارجي يُشغّل للقراءة/الإثبات فقط ولا يعدل.

### المهام المتسلسلة حسب الحزم

#### 7.1 `types`

1. راجع `Money`, `Currency`, IDs, rounding, errors.
2. تحقق من الحدود: integer digits, significant digits, representational scale, exponent extremes.
3. اختبر JSON round-trip، exact decimal string، minor units، banker’s rounding.
4. تحقق أن unknown currency لا يتحول إلى FinancialScale افتراضي في مسار posting.
5. تحقق من رفض XXX والعملات الفارغة وغير الصالحة.
6. افحص defensive copies في أي API يعيد بيانات قابلة للتعديل.

**القبول:** لا فقدان صامت ولا overflow غير مكتشف، ولا fallback يخفي خطأ العملة/الدقة.

#### 7.2 `journal`

1. راجع كامل `JournalEntry` و`EntryLine`.
2. لكل حقل: حدد هل يدخل fingerprint أو audit أو posting snapshot، وما سبب استبعاده إن لم يدخل.
3. تحقق من Clone وdeep-copy لكل pointers/slices.
4. اختبر lifecycle: Draft → Posted، منع إعادة التعديل، reversal relationships.
5. تحقق من تصنيف Regular/Adjusting/Closing/Audit/Reversal وقيم serialization القديمة.

**القبول:** لا حقل مالي/دلالي بلا قاعدة؛ لا تعديل غير مقصود بعد clone؛ لا انتقال حالة غير مسموح.

#### 7.3 `masterdata`

1. راجع Company Scope وCOA وAccount effective dating.
2. تحقق من parent cycles، duplicate IDs، missing parent، cross-company references.
3. راجع journal currency/account allowlists وdefensive copying.
4. راجع cost center/dimension hierarchy والـ effective dates.
5. راجع Fiscal Year/Period 1–14، start day غير الأول، نهاية الشهر/السنة الكبيسة، continuity، overlap، containment.
6. تحقق أن فشل Reader لا يتحول إلى قبول.

**القبول:** لا كيان خارج الشركة/المدة/الحالة المسموحة؛ لا فجوة أو تداخل في الفترات؛ الفشل مغلق عند نقص البيانات الأساسية المطلوبة.

#### 7.4 `policy`

1. راجع selection by company/date/effective window.
2. تحقق من overlap detection ومنع سياسة غير فعالة.
3. تحقق من canonical policy fingerprint ومن أن كل حقل يغير السلوك يدخل البصمة.
4. اختبر snapshot pinning: نفس القرار يستخدم نسخة السياسة التي تحققت منها العملية.
5. اختبر أن تعديل المصفوفة المسترجعة لا يعدل السياسة المسجلة.

**القبول:** اختيار سياسة واحدة حتمية لكل شركة/تاريخ/نوع، أو فشل واضح عند overlap/missing coverage؛ لا إعادة تفسير قيد قديم بسبب سياسة جديدة.

#### 7.5 `posting`

1. راجع ترتيب `PostEntry` كاملاً.
2. تحقق من الهوية والحالة والشركة واليومية والعملة والفترة والسياسة والحسابات والأبعاد قبل Executor.
3. تحقق من التطبيع والتقريب وحساب residual.
4. تحقق من توازن المدين والدائن بوحدات العملة الوظيفية الصحيحة.
5. تحقق من request fingerprint وcaller fingerprint v2 بعد P13-1/P13-2.
6. تحقق من idempotency scope وoperation precedence بعد قرار P13-3.
7. تحقق من reversal واحدة فقط، وسبب عكس غير فارغ، وفترة الوجهة الصحيحة.
8. تحقق من خطأ Executor: لا تعلن Posted ولا تُصدر snapshot ناجحاً عند فشل المنفذ.
9. تحقق من تصنيف الأخطاء retryable/non-retryable.
10. تحقق من أن `PostOptions` لا تسمح بتغيير قيم مختومة بعد حساب fingerprint.

**القبول:** لا يصل Executor قيداً غير صحيح؛ كل فشل قبل الالتزام لا يُعرض كعملية ناجحة؛ التكرار يظل ضمن العقد المحدد.

#### 7.6 `valuation`

1. راجع rate type/effective date/source/currency pair.
2. تحقق من same-currency rate = 1.
3. تحقق من inverse-rate precision وzero/negative/missing/future rates.
4. راجع monetary vs non-monetary positions.
5. اختبر اتجاه FX gain/loss للأصول والالتزامات.
6. تحقق من أن rate date غير valuation date.
7. تحقق من fingerprint وإصدار Revaluation payload.
8. تحقق من ارتباط Revaluation scope بقرار P13-3 وعدم الخلط مع reversal.

**القبول:** نتائج قابلة لإعادة الحساب، ودقة ثابتة، ورفض سعر غير صالح، واتجاه محاسبي مطابق للحالة.

#### 7.7 `governance`

1. راجع canonical audit encoding وتغطية جميع الحقول.
2. تحقق من snapshot consistency وcheckpoint generation/version.
3. اختبر tampering/deletion/reordering/wrong company.
4. اختبر append concurrent مع verify concurrent.
5. تحقق من error classification وretry policy.
6. وثق ما لا يمكن للنواة إثباته عن استقلال النسخة الخارجية أو الاحتفاظ خارج النظام.

**القبول:** كل فساد ضمن حدود نموذج التهديد يكتشف؛ append طبيعي خارج اللقطة لا يصنف فساداً؛ الاعتماد الخارجي موثق بوضوح.

#### 7.8 `ledger`

1. راجع `ProjectPostedMovements` و`RebuildAccountBalances` وعقد المصدر المرجعي.
2. تحقق من استبعاد Drafts والقيود الملغاة أو المعكوسة وفق ADR.
3. تحقق من حتمية projection.
4. لأن Go map غير مرتبة، لا تستخدم ترتيب map iteration كتمثيل canonical؛ إذا استُخدم output في hash/comparison، حوله إلى sorted slice أو ترتيب مفاتيح صريح.
5. تحقق من عدم إدخال تخزين أرصدة مادية أو تقارير داخل Kernel خلافاً للحدود المعتمدة.

**القبول:** إعادة البناء تعطي نفس القيم لنفس الحركات، والمقارنة/الهاش يستخدمان تمثيلاً مرتباً حتمياً، ولا تتوسع المسؤولية إلى تقارير أو تخزين خارجي.

#### 7.9 `apperrors` والجذر

1. تحقق من errors.Is/errors.As للمغلفات.
2. تأكد من أن الأخطاء العامة لا تكشف أسراراً أو تتجاهل السبب.
3. راجع doc.go والعقود المعمارية.
4. تحقق من AST architecture tests ومن أنها لا تحظر استيرادات مشروعة أو تسمح باستيرادات ممنوعة عبر alias.

**القبول:** تصنيف ثابت وقابل للاختبار، واختبارات معمارية تغطي الاستيرادات الممنوعة فعلاً.

### معايير القبول العامة للمرحلة

- لا عيب Critical/High جديد غير معالج.
- جميع الإصلاحات الجديدة test-first ومربوطة بمعرف KFR.
- لا إعادة هيكلة غير لازمة.
- لا تغيير عقد عام دون ADR.
- كل حزمة لديها نتيجة مراجعة مستقلة.

---

## المرحلة P13-8 — إعادة تنفيذ الاختبارات النهائية وحفظ الأدلة

### المشكلة التي تحلها

تقارير الإنجاز السابقة تصف نجاح الاختبارات، لكن لا يجوز إعادة استخدام سجلات قديمة لإصدار جديد. يلزم إنتاج سجل حديث على Commit واحد بعد انتهاء كل الإصلاحات.

### الهدف

إثبات أن النسخة النهائية المحددة تجتاز الاختبارات على Go 1.26.3، وإتاحة إعادة تشغيل النتائج.

### النطاق

- اختبارات Kernel في نطاق مباشر.
- اختبارات المحولات وSDK والتكافؤ وRLS وDR يمكن تشغيلها قراءةً فقط لإثبات التوافق الخارجي؛ لا تعديل خارج Kernel.
- الأدلة تضاف تحت `internal/kernal/evidence/P13/<commit>/` أو CI artifact دائم مع manifest داخل Kernel.

### المهام المتسلسلة

1. ثبت Commit النهائي الذي يحتوي جميع إصلاحات P13.
2. تأكد من `git status --short` نظيف باستثناء ملفات evidence المسموح بها والتي تدخل تحت `internal/kernal`.
3. سجل `go version` ويجب أن يظهر `go1.26.3`.
4. سجل OS/ARCH و`go env` ونسخة dependencies من `go list -m all`.
5. نفذ `gofmt -l internal/kernal`; النتيجة المطلوبة فارغة.
6. نفذ `git diff --check`.
7. نفذ `go vet ./internal/kernal/...` برمز خروج 0.
8. نفذ `go test ./internal/kernal/... -count=1`.
9. نفذ `go test -race ./internal/kernal/... -count=1`.
10. نفذ `go test ./internal/kernal/... -shuffle=on -count=5`.
11. أنشئ Coverage Profile جديداً من نفس Commit:

```bash
mkdir -p internal/kernal/evidence/P13/$(git rev-parse --short HEAD)
go test ./internal/kernal/... -count=1 \
  -coverprofile=internal/kernal/evidence/P13/$(git rev-parse --short HEAD)/kernel.cover

go tool cover -func=internal/kernal/evidence/P13/$(git rev-parse --short HEAD)/kernel.cover
```

 1. احسب Statement Coverage الإجمالية والحزم الحرجة على حدة؛ عتبة الإجمالي المقترحة 90%، والحرجة 95% إذا اعتمد مالك المشروع هذا الهدف. أي استثناء يجب أن يتبع نموذج `Coverage Exception` ولا يُقبل لمجرد أنه defensive.
 2. نفذ كل Fuzz target المسجل حالياً في `internal/kernal` بمدة موحدة 30 ثانية أو المدة التي يثبتها خط الأساس؛ استخدم مشغل يلتقط exit code لكل هدف ولا يعتمد على `verify_kernel_baseline.sh` وحده.
 3. نفذ Mutation Testing الموجه في بيئة مؤقتة نظيفة؛ تحقق من تطبيق الطفرة، نجاح compilation قبل mutation test، فشل assertion المقصود، ثم restoration byte-identical.
 4. نفذ اختبارات Kernel ذات العلاقة من `go test ./...` كتحقق تكامل قراءة فقط، إذا سمح الوقت والبيئة. لا تعتبر فشل مكون خارجي عيباً داخل Kernel تلقائياً؛ صنفه وحدد مالكه.
 5. نفذ `go test ./test/contract_parity -count=1` وDR/RLS/API compatibility إن كانت موجودة، مع تسجيلها كأدلة مضيف لا كدليل Kernel صرف.
 6. نفذ architecture verification، لكن لا تعتمد `verify_kernel_baseline.sh` كدليل Fuzz وحيد حتى تُعالج مشكلة propagation في script خارج النطاق؛ شغل Fuzz منفصلاً.
 7. احفظ كل أمر مع output كامل ورمز الخروج والمدة وhash للـ log.
 8. احسب SHA-256 لكل artifact، واكتب manifest JSON أو Markdown داخل evidence folder.
 9. أعد تشغيل الاختبارات بعد أي تغيير في الكود أو test أو docs التي تولد مدخلات اختبار؛ لا تنقل PASS من Commit سابق.

### معايير القبول والنجاح

- Go 1.26.3 مثبت في سجل البيئة.
- جميع أوامر Kernel الإلزامية exit 0.
- Race detector بلا data races.
- Shuffle خمس مرات ناجح.
- Fuzz جميع الأهداف ناجحة؛ لا corpus failure غير محفوظ.
- Coverage حديث، ومحقق للعتبة المعتمدة أو استثناءات مراجعة مستقلة.
- Mutation evidence موثوق وليس false positive.
- كل الملفات المولدة مرتبطة بنفس Commit.
- لا تغير غير مصرح به خارج `internal/kernal`.

### أدلة الإغلاق

- `KERNEL_P13_TEST_EVIDENCE.md`.
- `evidence/P13/<commit>/kernel.cover`.
- سجلات Unit/Race/Shuffle/Fuzz/Vet/Architecture/Mutation.
- `manifest.sha256` أو ملف manifest مكافئ.

### قرار البوابة

أي فشل في أمر إلزامي = `FAIL` حتى يثبت أنه فشل بيئي مستقل. فشل بيئي يمنع إعلان PASS إلى أن يعاد التشغيل على بيئة مطابقة.

---

## المرحلة P13-9 — مراجعة مستقلة وقرار الاعتماد النهائي

### المشكلة التي تحلها

المراجعة السابقة لخطة P0–P12 سجلت غياب تعريف حاسم لحالات القرار، وعدم كفاية تسجيل المراجع المستقل في بعض التقارير. لا يكفي أن يكتب المنفذ أن جميع البوابات PASS.

### الهدف

إصدار قرار اعتماد مستقل يحدد بدقة هل النواة جاهزة ضمن حدود مسؤولياتها، وما هي المسائل الخارجية غير المشمولة.

### النطاق

- وثائق الاعتماد داخل `internal/kernal/` فقط.
- يمكن للمراجع فحص المستودع كله قراءةً فقط للتحقق من التكامل، لكن لا يُعدل أي ملف خارجه.

### المهام المتسلسلة

1. جمّع نتائج P13-0 إلى P13-8 في جدول مهام برقم ثابت لكل مهمة.
2. تأكد أن كل مهمة PASS لها أمر وoutput وexit code وcommit.
3. تأكد أن كل استثناء له معرف ومالك وموعد أو شرط إغلاق.
4. راجع كل KFR جديد: closed أو conditional أو open مع السبب.
5. راجع جميع RULE-KERNEL-001..027 من المصفوفة.
6. راجع جميع Gates G1–G15 أو أي Gate set معتمد، مع توثيق N/A وسببه.
7. افصل `Kernel GO` عن `System/Product GO`.
8. سجل G-007 كمخاطر خارجية مقبولة فقط إذا كان قرار المالك موثقاً.
9. سجل G-018 كمشكلة خارجية Open ما دام صفها في Gap Register يذكر أنها لا تُغلق بقرار ADR وحده. لا تعلن أنها مغلقة ضمن Kernel ولا تُعدل وحدات خارج النطاق.
10. عين Reviewer مختلفاً عن Implementer.
11. يراجع الشخص المستقل diff على Commit النهائي، وليس ملخص الوكيل فقط.
12. يوقع المراجع على checklist تشمل: correctness, fingerprint collision, scope semantics, precision, audit integrity, tests, evidence, traceability, scope compliance.
13. يحدد Release Authority قرار `PASS` أو `CONDITIONAL` أو `FAIL` وفق التعاريف الملزمة.
14. أنشئ `KERNEL_P13_FINAL_SIGNOFF.md` داخل Kernel؛ لا تستبدل تقارير P0–P12 التاريخية، بل اربطها كأدلة سابقة.
15. إذا كانت النواة PASS بينما G-018 خارج النطاق Open، اكتب صراحة `Kernel GO / System GO not assessed`، ولا تستخدم عبارة أن المنتج كله GO.

### معايير القبول والنجاح

- لا توجد Critical/High داخل Kernel.
- كل القواعد الـ27 موثقة ومنفذة ومختبرة.
- كل الأدلة حديثة ومرتبطة بنفس Commit.
- المراجع مستقل ومحدد بالاسم/المعرف والتاريخ.
- لا يوجد تناقض بين حكم النواة وحالة فجوات المضيف.
- نطاق التغيير مطابق حصرياً لـ `internal/kernal/**`.

### أدلة الإغلاق

- `KERNEL_P13_FINAL_SIGNOFF.md`.
- جدول المراجع والقرار.
- سجل KFR النهائي.
- مصفوفة تتبع نهائية.
- manifest بصمات الأدلة.

### قرار البوابة

- `PASS`: يمكن إصدار Kernel GO.
- `CONDITIONAL`: النواة لا تحتوي عيباً حرجاً، لكن يوجد دليل/مراجعة/اعتماد متوسط ناقص؛ لا تعلن 10/10.
- `FAIL`: أي عيب Critical/High أو فشل اختبار إلزامي أو scope violation أو غياب مراجع مستقل.

---

# 5. قائمة أوامر التحقق المعيارية

يتم تشغيل الأوامر التالية من جذر Git checkout باستخدام Go 1.26.3. قد تختلف مسارات حفظ logs بحسب Commit؛ لا تغير الأمر الأساسي دون تسجيل التغيير.

## 5.1 تحديد البيئة

```bash
go version
go env GOVERSION GOOS GOARCH GOTOOLCHAIN GOMOD

git rev-parse HEAD
git branch --show-current
git status --short
sha256sum accountant_engine.zip
```

## 5.2 مراجعة تنسيق ونقاء Kernel

```bash
gofmt -l internal/kernal
git diff --check
go vet ./internal/kernal/...
go test ./internal/kernal/... -count=1
```

## 5.3 Race وShuffle

```bash
go test -race ./internal/kernal/... -count=1
go test ./internal/kernal/... -shuffle=on -count=5
```

## 5.4 Coverage Profile حديث

```bash
COVER_DIR="internal/kernal/evidence/P13/$(git rev-parse --short HEAD)"
mkdir -p "$COVER_DIR"

go test ./internal/kernal/... -count=1 -coverprofile="$COVER_DIR/kernel.cover"
go tool cover -func="$COVER_DIR/kernel.cover"
```

قارن النسبة المحسوبة مع عتبة 90% الإجمالية وعتبة 95% للحزم الحرجة إذا كانتا معتمدتين في Release Gate. لا تعتمد على default 80% في `scripts/check_coverage.sh` لإثبات عتبة 90%.

## 5.5 Fuzz

```bash
FUZZTIME=30s ./scripts/run_fuzz_all.sh \
  "internal/kernal/evidence/P13/$(git rev-parse --short HEAD)/fuzz.log"
```

هذا script خارج نطاق التعديل لكنه يستخدم قراءة/تشغيلاً فقط. تحقق من أن كل target خرج 0 وأن `Targets Run` يطابق inventory الحالي. إذا تغير عدد targets، حدث inventory داخل Kernel مع السبب.

## 5.6 اختبار المستودع والتوافق — قراءة فقط

```bash
go test ./... -count=1
go test -race ./... -count=1
go test ./test/contract_parity -count=1
./scripts/verify_architecture.sh
./scripts/check_api_compat.sh
```

أي فشل خارج Kernel يصنف باسم الحزمة والمالك؛ لا تعدل تلك الحزمة ضمن P13.

---

# 6. مصفوفة المخرجات المطلوبة

| المخرج | مكانه الملزم | محتواه |
| --- | --- | --- |
| Baseline P13 | `internal/kernal/KERNEL_P13_BASELINE.md` | Commit، hash، toolchain، الجرد، الحالة قبل التعديل |
| Findings Register | `internal/kernal/KERNEL_P13_FINDINGS_REGISTER.md` | KFR-001..n، الأولوية، الأدلة، المالك، الحالة |
| Decisions | `internal/kernal/KERNEL_P13_DECISIONS.md` | قرار Canonical v2، v1 compatibility، Revaluation scope precedence |
| Fingerprint Contract | `internal/kernal/KERNEL_FINGERPRINT_CONTRACT.md` | الحقول، canonical encoding، versioning، golden vectors |
| Test Evidence | `internal/kernal/KERNEL_P13_TEST_EVIDENCE.md` | الأوامر، البيئة، exit codes، coverage، Fuzz، Race، Mutation |
| Traceability Review | `internal/kernal/KERNEL_P13_TRACEABILITY_REVIEW.md` | 27 قاعدة مع تنفيذ واختبارات وassertions |
| External Dependencies | `internal/kernal/KERNEL_P13_EXTERNAL_DEPENDENCIES.md` | G-007 وG-018 واعتماديات المضيف وحدود Kernel GO |
| Final Signoff | `internal/kernal/KERNEL_P13_FINAL_SIGNOFF.md` | قرار المراجع المستقل وRelease Authority |
| Evidence manifest | `internal/kernal/evidence/P13/<commit>/manifest.sha256` | بصمات logs وcoverage وملفات الأدلة |

لا تنشئ نسخاً متكررة من الوثائق القديمة دون سبب. حدّث وثائق Kernel الموجودة إذا كانت هي المصدر المعتمد، مع سجل تغيير واضح.

---

# 7. قواعد الإيقاف والتراجع

## 7.1 حالات التوقف الفوري

أوقف المرحلة فوراً إذا:

- ظهر أي تغيير خارج `internal/kernal/**`.
- تعذر التحقق من مصدر الكود أو Commit.
- فشل اختبار RED للسبب غير المتوقع.
- ظهر تغيير في API/enum/serialization دون قرار توافق.
- اكتشف تصادم preimage إضافياً.
- تعذر تحديد مصدر Revaluation Scope أو سلطته.
- ظهر اختلاف مالي أو loss of precision.
- ظهر data race أو nondeterminism.
- لم يمكن إعادة الاختبار على Go 1.26.3.

## 7.2 سياسة التراجع

1. لا تستخدم `git reset --hard` على شجرة عمل المستخدم دون موافقة.
2. استخدم branch أو worktree منفصل لتنفيذ P13.
3. احتفظ ببصمة الملفات قبل وبعد Mutation.
4. عند فشل patch، أعد الملف المتأثر فقط إلى حالته السابقة في branch الخاص بالمهمة؛ لا تمس أعمالاً أخرى.
5. لا تحذف corpus أو logs قبل توثيقها.
6. لا تُسقط test لمجرد أنه يكشف bug؛ إما إصلاح السبب أو توثيق قرار إزالة مدعوم بمراجعة مستقلة.

---

# 8. توزيع المسؤوليات

| الدور | المسؤولية | لا يجوز له |
| --- | --- | --- |
| Implementer | تنفيذ الإصلاحات والاختبارات وإعداد الأدلة | اعتماد عمله وحده |
| Kernel Reviewer | مراجعة الشيفرة والثوابت والاختبارات | تعديل نتيجة الاختبار لإجبار PASS |
| Verification Owner | التحقق من البيئة والأوامر والـ artifacts | الاكتفاء بملخص الوكيل |
| Release Authority | إصدار قرار Kernel GO/CONDITIONAL/NO-GO | دمج فجوة خارجية في قرار Kernel دون توضيح |
| Host/Adapter Owner | معالجة الاعتماديات الخارجية عند الحاجة | اعتبار ذلك مسؤولية Kernel دون عقد واضح |

يمكن أن تكون الأدوار لأشخاص مختلفين أو جهات مختلفة؛ لكن Implementer لا يكون Reviewer الوحيد ولا Release Authority الوحيد.

---

# 9. معايير 10/10 النهائية

لا تمنح النواة 10/10 إلا إذا تحققت جميع الشروط التالية:

1. `KFR-001` مغلق باختبارات تصادم canonical مستقلة.
2. `KFR-002` مغلق بقرار معماري يحدد سلطة Revaluation hint وعلاقته بالـ reversal.
3. `KFR-003` مغلق بتوافق موثق مع fingerprints v1 أو خطة cutover آمنة لا تؤدي إلى تكرار أثر مالي.
4. Audit snapshot/checkpoint مثبتة باختبارات التلاعب والتزامن.
5. كل قاعدة من القواعد الـ27 لها تنفيذ ودليل قبول ورفض.
6. جميع الأخطاء الدلالية مصنفة retryable/non-retryable بصورة واضحة.
7. Statement coverage تحقق العتبات المعتمدة بأدلة حديثة واستثناءات سليمة؛ لا تُسمى Branch Coverage ما لم تُقَس فعلياً.
8. جميع Fuzz targets تنجح بمدة ثابتة وlogs محفوظة.
9. Mutation tests لا تصنف build/setup failures على أنها KILLED، والنتيجة موصوفة بدقة كطفرات مستهدفة إذا لم يكن هناك أداة شاملة.
10. Race detector وshuffle tests تنجح.
11. `go vet` و`gofmt` و`git diff --check` تنجح.
12. مصفوفة traceability لا تحتوي اسماً وهمياً أو اختباراً لا يثبت assertion المطلوبة.
13. لا توجد Critical/High داخل Kernel.
14. لا توجد تغييرات خارج `internal/kernal/**` في P13.
15. كل بوابة لها Reviewer مستقل وRelease decision واضح.
16. G-007 وG-018 لا تُعرضان كمسألتين مغلقتين على مستوى المنتج ما لم يثبت ذلك في مصدرهما؛ يذكر نطاق Kernel منفصلاً.
17. كل النتائج مرتبطة بـ Commit واحد وGo 1.26.3 وEvidence manifest.

**إذا نقص أي شرط إلزامي، تكون النتيجة أقل من 10/10 حتى لو كان مجموع التغطية مرتفعاً.**

---

# 10. قالب تقرير كل مرحلة

يجب أن يحتوي تقرير كل مرحلة على الأقسام التالية:

1. رقم المرحلة ورمزها.
2. المشكلة التي تعالجها.
3. نطاق العمل داخل Kernel وخارجه.
4. الملفات التي تمت مراجعتها قبل التنفيذ.
5. المهام المتسلسلة وحالة كل مهمة.
6. معرفات KFR التي عولجت.
7. الملفات التي تغيرت وسبب كل تغيير.
8. الاختبارات المضافة وسبب كل assertion.
9. أوامر التشغيل والبيئة ورموز الخروج.
10. Coverage/Mutation/Fuzz/Race حسب صلة المرحلة.
11. الأدلة ومساراتها وبصماتها.
12. المخاطر أو الاعتماديات الخارجية.
13. اسم المنفذ.
14. اسم المراجع المستقل وتاريخه.
15. قرار البوابة PASS/CONDITIONAL/FAIL وتعليله.

---

# 11. التكليف التنفيذي المختصر للوكيل

نفّذ خطة P13 بوصفها إغلاقاً نهائياً تكملياً بعد P0–P12، ولا تعِد تنفيذ المراحل السابقة بالكامل. ابدأ بمراجعة Git/source baseline ثم عالج `KFR-001` في ترميز بصمات Posting؛ احسم توافق v1/v2 قبل cutover؛ راجع حدود تلميح `IdempotencyRevaluation` مقابل reversal وفق ADR P11-01 ولا تغيره بلا قرار؛ أعد إثبات Audit Snapshot/Checkpoint؛ دقق موثوقية Fuzz/Mutation/Coverage evidence؛ أعد تدقيق مصفوفة القواعد الـ27؛ ثم نفذ Kernel validation على Go 1.26.3 ومراجعة مستقلة وتوقيع نهائي.

**قواعد إلزامية:**

- جميع تعديلات الكود والاختبارات والوثائق والأدلة الجديدة/المعدلة داخل `internal/kernal/**` فقط.
- جميع الملفات خارج Kernel قراءة/تشغيل فقط؛ لا تعديل `pkg`, `storage`, `modules`, `orbitals`, `test`, `scripts`, `docs` الخارجية، `go.mod`, `go.sum`, CI.
- لا RBAC أو صلاحيات أو تفويض أو Maker-Checker.
- لا ميزات جديدة ولا إعادة تصميم معماري إلا بقرار ADR.
- لا حذف اختبار أو إضعاف assertion لإجبار PASS.
- لا ادعاء نجاح دون أمر وبيئة ورمز خروج ودليل محفوظ.
- لا إعلان `Kernel GO` قبل مراجع مستقل مختلف عن المنفذ.
- لا تخلط `Kernel GO` مع `System/Product GO`؛ تبقى G-007 وG-018 خارج النطاق وفق حالتهم الموثقة.

**المخرج النهائي المطلوب:** `KERNEL_P13_FINAL_SIGNOFF.md` داخل `internal/kernal/` مع سجل النتائج، والمراجع، وCommit، وmanifest الأدلة، وحالة كل قاعدة من 27 قاعدة وكل KFR، وقرار نهائي صريح.

---

## 12. سجل تغيير هذه الخطة

| الإصدار | التغيير |
|---|---|
| P13-Draft-1 | مراجعة الأرشيف، تحديد collision في Canonical Fingerprint، إضافة قرار Revaluation Scope، تضمين مراجعة الحوكمة والأدلة وحدود النطاق |

**ملاحظة نهائية:** هذه الخطة لا تدعي أن الإصدار الحالي فشل في جميع البوابات، ولا تعيد فتح فجوات P0–P12 المغلقة دون دليل جديد. هي تستهدف العيب المحدد في ترميز البصمة، والقرارات غير المحسومة، وتوثيق وإعادة إثبات صلاحية الاعتماد النهائي. لا تمنح 10/10 بمجرد اكتمال المهام الورقية؛ تمنحها فقط بعد ثبوت السلوك في الشيفرة والاختبارات المستقلة.
