# تحليل تضاربات وثائق مجلد `docs/`

**التاريخ:** 2026-11-27  
**النطاق:** جميع ملفات `docs/` (51 ملفاً باستثناء لقطة API الكبيرة التي فُحصت انتقائياً).  
**المنهجية:** قراءة مقارنة لكل مجموعة وثائق، مع تحقق آلي من الروابط والم hashes ومطابقة الادعاءات مقابل شجرة الكود والتقارير.  
**الحالة:** وثيقة تحليلية — ليست قرار اعتماد.  
**تحديث الحالة (2026-11-27):** طُبقت الإصلاحات الموصى بها أدناه على الوثائق؛ هذا الملف يبقى سجلاً تحليلياً للحالة قبل الإصلاح، مع [قسم المعالجة](#6-حالة-المعالجة-بعد-التطبيق) في نهايته.  
**ملاحظة:** أُجري التحليل أولاً بالقراءة فقط، ثم طُبقت الإصلاحات لاحقاً بناءً على طلب المستخدم.

---

## 1. الملخص التنفيذي

نعم، توجد تعارضات فعلية، بعضها **حرج**. الملفات تحمل تعديلات غير ملتزمة (`git status` يُظهر `M` على 23 ملفاً) تبدو كمحاولة إصلاح لتعارضات سابقة. هذه المحاولة أصلحت قسماً كبيراً، لكنها تركت تعارضات جديدة، أبرزها إحالة إلى ملف `analysis/` كان غير موجود، واعتماد يُلغي نفسه، وManifest مكسور.

| # | الشدة | التعارض | الموضع |
| --- | --- | --- | --- |
| 1 | 🔴 حرجة | وثيقة الاعتماد تقول `SUPERSEDED` ثم تُصدر `GO` في نفس الملف، والـ runbooks تعتمد عليها | `reports/KERNEL_RELEASE_SIGNOFF.md:3` ↔ `:55`؛ `operations/runbooks/release_rollback_runbook.md:60` |
| 2 | 🔴 حرجة | `GO` نهائي مقابل «مواصفات مرشحة للاعتماد» ومراجعة/اعتماد معلّقان | `KERNEL_RELEASE_SIGNOFF.md:55` ↔ `specifications/kernel/*:5` و`quality/SCOPE_PROFILE_KERNEL_v1.md:17-18` |
| 3 | 🔴 حرجة | جدول ربط البوابات يعيد تعريف `G01` الرسمي ويخلط `G1`/`G5` بين 3–4 معانٍ | `quality/QUALITY_GATES_CROSSWALK.md:19-33` ↔ `QUALITY_GATES_SPECIFICATION.md:25-35` |
| 4 | 🔴 حرجة | `ACC-013` معرّف واحد بمعنيين: «التوازن بالعملة الوظيفية» مقابل «فروق IAS 21» | `quality/ACCOUNTING_KERNEL_QUALITY_STANDARD.md:357` ↔ `rules/ACCOUNTING_BUSINESS_RULES.md:119` |
| 5 | 🔴 حرجة | اتجاه استيراد النواة: `CORE_PKG → API/SPI` مسموح في رسم، وممنوع بـ R1/R3b في ADR والسكربت | `architecture/NEW_PROJECT_STRUCTURE.md:291-292` ↔ `architecture/KERNEL_ARCHITECTURE_DECISION_RECORDS.md:193` و`architecture/KERNEL_ARCHITECTURE_MAP.md:76` |
| 6 | 🟠 عالية | لقطة API تالفة فعلياً: اسم حزمة يحتوي نصاً عربياً | `api/v1.snapshot:1064` |
| 7 | 🟠 عالية | إشارة `TransactionAmount` متناقضة (موجب إلزامياً مقابل موجب/سالب) | `specifications/kernel/JOURNAL_ENTRY_DOMAIN_SPECIFICATION_v1.2.0.md:95` ↔ `LEDGER_PROJECTION_DOMAIN_SPECIFICATION_v1.2.0.md:94` |
| 8 | 🟠 عالية | حالة `Cancelled` إلزامية لكنها غير مسموحة في أفعال التدقيق، وغائبة من لقطة API والكود | `JOURNAL_ENTRY_...:157` ↔ `GOVERNANCE_AND_AUDIT_...:120`؛ `api/v1.snapshot` |
| 9 | 🟠 عالية | `PASS 15/15` مقابل `E4 Not Verified` وسجل تشغيل اختبارات فارغ | `reports/KERNEL_FINAL_VERIFICATION_REPORT.md:6,91` ↔ `quality/COMPLIANCE_TEST_SUITE_INDEX.md:66-73` |
| 10 | 🟠 عالية | Manifest يفشل لملف واحد: `QUALITY_GATES_CROSSWALK.md` (غير متتبع في git) | `quality/MANIFEST.sha256:6` |

---

## 2. التعارضات تفصيلاً

### أ. حالة الاعتماد والحوكمة

- **اعتماد مُلغٍ لنفسه:** `KERNEL_RELEASE_SIGNOFF.md:3` ينص على `SUPERSEDED — أُلغي هذا الاعتماد ... يلزم إعداد وثيقة اعتماد جديدة`، بينما السطر `55` يقول `GO — معتمد رسميّاً للنشر`. و`:5` لا يزال «الإصدار المعتمد v1.0.0»، و`:57` يختم بتوقيع `2026-11-22T06:14:00Z`. كذلك `release_rollback_runbook.md:60` يشترط «توقيع البوابات في اعتماد الإصدار بحالة `GO`» لإغلاق الحادث، أي أن حالة التشغيل مبنية على مستند مُلغى.
- **المرجعية غير معتمدة:** كل المواصفات العشر تحمل `حالة النضج: Review Candidate` (مثال `GOVERNANCE_AND_AUDIT_...:5`)، و`quality/README.md:3,22` تنص على `not yet institutionally approved / not assessed`، و`SCOPE_PROFILE_KERNEL_v1.md:17-18` فيه `Pending Independent Review / Pending Approval Authority` — ومع ذلك تُمنح `RELEASE GO` في `KERNEL_FINAL_VERIFICATION_REPORT.md:93`.
- **حالة جودة متناقضة داخلياً:** `COMPLIANCE_TEST_SUITE_INDEX.md:57-67` يسجّل `Pass (2026-11-27, e3318e4)` لعشر مجموعات اختبار، بينما `:73` يترك سجل التشغيل كعناصر نائبة `Pass/Fail/Not Run` بلا Run ID/Commit/Evidence، في خرق لـ `QUALITY_GATES_SPECIFICATION.md:56-57`.
- **قاعدة Waiver:** `QUALITY_GATES_SPECIFICATION.md:40` تجيز اعتماد اختبار حرج فاشل «كاستثناء رسمي وفق سياسة Waiver»، بينما `EVIDENCE_AND_SCORING_GUIDE.md:268-270` يمنع Waiver «لثابت محاسبي أساسي أو لتجاوز عزل مطلوب».
- **مصدر الحقيقة:** `docs/README.md:3` يدّعي أنه «مصدر الحقيقة لكل فئة»، بينما `quality/DOCUMENTATION_ARCHITECTURE_SPECIFICATION.md:33-41` يوزّع الملكية على كل وثيقة على حدة؛ و`quality/README.md:16` يقول إن المصدر `ACCOUNTING_KERNEL_QUALITY_STANDARD_v2.0.0.md` «محتفظ به منفصلاً» وهو غير موجود.

### ب. المعرّفات والبوابات

- **`ACC-013`**: في المعيار = «التوازن بالعملة الوظيفية» (`STANDARD.md:357,551`)، وفي قواعد العمل = «معالجة فروق أسعار الصرف IAS 21» (`BUSINESS_RULES.md:119`) — مع أن `quality/README.md:63` و`STANDARD.md:1235` يؤكدان «أُضيف ACC-013 دون إعادة استخدام معرف». كذلك تصادمات `ACC-003/005/007/008` مع `RULE-ACC-003/005/007/008` بمعانٍ مختلفة تماماً.
- **عدد الثوابت**: `SCOPE_PROFILE_KERNEL_v1.md:58` «الثوابت الـ27» ↔ `BUSINESS_RULES.md:310` «هذه القواعد الـ43» (الواقع 47 معرّفاً بعد `016A/022A/022B/022C`) ↔ المعيار 13 ثابتاً فقط (`ACC-001..013`).
- **Crosswalk مقلوب**: `CROSSWALK.md:21` يجعل `G01` = «نظافة البناء» بينما الرسمي `G01 Accounting Integrity`؛ `:24` `G04` = تكافؤ المحولات مقابل `G04 Monetary Integrity`؛ `:27` `G07` = API مقابل `G07 Persistence Integrity`؛ `:28-29` `G08/G09` أيضاً معكوسة. و`:32` يترك «السلامة المحاسبية» بلا مقابل رسمي رغم أنها `G01` في المواصفة و`G2` في معايير القبول.
- **ثلاثة أنظمة ترقيم** (`G01–G11` رسمي، `G1–G15` تشغيلي، `G1–G6` قبول) بلا ربط سليم: `G1` يعني «المعمارية» في `ACCEPTANCE_CRITERIA.md:139` و«Scope» في `KERNEL_FINAL_VERIFICATION_REPORT.md:57` و«نظافة البناء/API» في Crosswalk؛ `G5` يُربط بأربع بوابات رسمية في نفس الجدول.
- **6 بوابات قبول مقابل 11 إلزامية**: `ACCEPTANCE_CRITERIA.md:135-151` يشترط «كافة البوابات التالية» (G1–G6)، بينما `QUALITY_GATES_SPECIFICATION.md:10` يُلزم بـ G01–G11. و`:162` يجعل تغطية 80% شرط GO بينما `SUCCESS_CRITERIA.md:68` و`ENGINEERING_BUSINESS_RULES.md:175` يشترطان >95% للنواة، والمعيار `:21` ينفي أن تكون التغطية وحدها شرطاً.
- **Manifest**: `sha256sum -c docs/quality/MANIFEST.sha256` من جذر المستودع → 8 OK و`QUALITY_GATES_CROSSWALK.md: FAILED` (المتوقع `838029c8…`، المحسوب `04f03e5e…`)؛ الملف `??` في git، أي أُضيف بعد الـ Manifest وهو نفسه حامل تعارضات البوابات.

### ج. المعمارية

- **اتجاه الاعتماد:** `NEW_PROJECT_STRUCTURE.md:291-292` يرسم `CORE_PKG --> API` و`CORE_PKG --> SPI`، بينما `ADR P7-01/4` في `KERNEL_ARCHITECTURE_DECISION_RECORDS.md:193` و`KERNEL_ARCHITECTURE_MAP.md:76` يحظران «أي اعتماد من `internal/kernal/*` على `pkg/*` أو `storage/*`» (R1/R3a–R3d)، والسكربت `verify_architecture.sh` يفرض المنع.
- **جسر `pkg/api/v1`:** ADR `:190` وMap `:100` يجيزان الجسر الوحيد `type X = kernal...X`؛ لكن `NEW_PROJECT_STRUCTURE.md:300` ينص على `API -.->|محظور قطعياً| InternalCore`، و`ARCHITECTURAL_PRINCIPLES_CHARTER.md:207-208` (ENF-02) يمنع أي نوع `internal/*` في السطح العام بلا استثناء.
- **من هو الدستور:** `docs/README.md:3,11` مقابل `ARCHITECTURAL_PRINCIPLES_CHARTER.md:229` («الدستور الأعلى») مقابل `DECISION_RECORDS.md:3` («المرجع الموحّد») مقابل `KERNEL_ARCHITECTURE_MAP.md:14` («تُستمد الحقائق من `go list` لا من الادعاءات»). لاهرم سلطة عند التعارض.
- **7 مقابل 9 حزم:** `CHARTER.md:170-179` و`NEW_PROJECT_STRUCTURE.md:75-121` يعرضان 7 حزم (بلا `journal`/`ledger`) ويضعان `pkg/spi/storage` داخل النواة (`:178`)، بينما `ADR P1-01` (`DECISION_RECORDS.md:34,42-52`) ينص صراحة «يُحل محل: القائمة المكوّنة من سبع حزم» ويعتمد «تسع حزم + جذر» (`MAP.md:16-29`). و`NEW...:270` يسرد ست حزم فقط.
- **الأرصدة داخل وحدة العمل؟** `CHARTER.md:106` يُدرج «تحديث أرصدة الأستاذ العام، تسجيل حركات الدفاتر الفرعية» في الـ Unit of Work؛ بينما `ADR P1-02:81` و`MAP.md:86` و`TARGET_ACCOUNTING_KERNEL.md:46,230` تجعل الأرصدة «مشتقة، مملوكة للمضيف، خارج النواة»، بل `:111` من الميثاق نفسه يمنع `SetBalance`.
- **طول الأنبوب:** `TARGET_ACCOUNTING_KERNEL.md:73` «17-Step (0–16)» مقابل `NEW_PROJECT_STRUCTURE.md:77` «الأنبوب الثماني»؛ و`POSTING_AND_REVERSAL_...:99` لا يزال يقول «المراحل 1–7» رغم إصلاحه جزئياً.
- **النواة تُقفل أم ترتّب؟** `NEW...:81` يصف `lock_manager.go` بأنه «مدير أقفال ... لمنع Deadlocks» مقابل `ADR P8-01:210` «النواة ترتب ولا تُقفل ... القفل مسؤولية Adapter/Host».
- **RBAC/اعتماد داخل النواة؟** `NEW...:110,116-117` يسرد `posting_rules.go` «الرقابة المزدوجة» و`approvals.go`/`sod_policy.go`، بينما `TARGET:152` و`CHARTER:82-84` و`MAP.md:26` تنفي وجود أدوار/موافقات في النواة؛ والملفات غير موجودة فعلياً في `internal/kernal`.
- **مسارات غير موجودة:** `CHARTER.md:164-168` و`TARGET:85-89` يستخدمان `internal/subledgers|reporting|events|reconciliation|consolidation`، بينما الواقع `internal/orbitals/*` (`NEW...:122-147`)؛ `SDK_OVERVIEW.md:373-374` يشير إلى `docs/ARCHITECTURAL_PRINCIPLES_CHARTER.md` والصحيح `docs/architecture/...`؛ `NEW...:82-84,89,105,109,116-117,136-137,144,147` يسرد ملفات غير موجودة (`content_hash.go`, `error_classification.go`, `account_version.go`, `rates.go`, `policy.go`, `approvals.go`, `sod_policy.go`, `event_processor.go`, `intercompany.go`, `year_end_closing.go`).
- **fail-closed لليومية:** `SDK_INTERNALS_AND_LIFECYCLE.md:34` يُنشئ مجلة تركيبية `GENERAL` بتاريخ `2000-01-01` «دون خطأ» مقابل `TARGET:162` «يرفض الترحيل اليوميات غير الموجودة ...» و`MAP.md:38`.
- **حالة `cancelled`:** `TARGET:207` يذكرها مقابل `ADR P1-02:82` «الانتقالات المسموحة: Draft → Posted → Reversed» وغيابها من `internal/kernal/journal` و`api/v1.snapshot`.
- **سطح SDK:** `NEW...:213` يذكر `sdk.WithSubledgers()` (غير موجود؛ الصحيح `WithSubledgerARAP` في `SDK_INTERNALS...:207`)؛ `TARGET:257-258` 4 خدمات مقابل 6 في `SDK_OVERVIEW.md:12,209-218`.

### د. المواصفات والقواعد المالية

- **ترتيب التقريب (أثر مالي مباشر):** `MONEY_DOMAIN_SPECIFICATION_v2.6.1.md:433` «يحسب التحويل بالسعر الدقيق ... ويقرّب الناتج النهائي فقط» مقابل `POSTING_AND_REVERSAL_...:117` «تطبيع `TransactionAmount` أولاً ... ثم الضرب ... ثم التطبيق». النص يقول «لا يتجاوزها» لكنه عملياً يغيّر المدخل → تقريب مزدوج قد يختلف بوحدة صغرى. كذلك `MONEY:317` «يجب ألّا يُقرّب لمجرد كسرية حاصل الضرب».
- **تسامح 0.5 وحدة مقابل مساواة صارمة:** `POSTING_...:117` يسمح بفرق `0.5` وحدة صغرى، بينما `JOURNAL_ENTRY_...:107` يشترط `TransactionAmount = المبلغ الوظيفي` عند تطابق العملة، و`POSTING_...:107` يشترط التوازن «حتى الوحدة الصغرى الدقيقة». لا قاعدة تحسم الأولوية.
- **سقف التمثيل:** `IDENTIFIERS_AND_ERRORS_...:121,124` يفرض `int64` (`[-2^63, 2^63-1]`) بينما `MONEY:140` يجيز حتى 28 خانة صحيحة و`:168` مقياساً كسرياً 55 خانة؛ القيم الصالحة في Money تصبح غير قابلة للترحيل دون قاعدة أولوية. وسجل تغييرات `IDENTIFIERS...:254` يزعم «تجريد int64» بينما `:9,119,241` ما زالت تذكره.
- **ISO 4217:** `ISO_4217_SNAPSHOT_AND_OVERRIDES.md:1` يوسم اللقطة `2015` لكن `:18` يُدرج `XCG` (إشعار صيانة لاحق) دون رقم إشعار/تاريخ سريان، في خرق لـ `MONEY:208` (MONEY-010). و`:19` يعطي `ANG/SLL/CUC/BGN` وحدة فرعية `—` بينما `MONEY:220,232` تمنع اختلاقها. و`:25,29` تحظر `XPT/XPD` بشكل مطلق مقابل `MONEY:259` التي تجيزها «ما لم يُنمذج المجال صراحةً». والملحق نفسه بلا إصدار/مالك/حالة نضج (`NSM-001 §14`).
- **إشارة `TransactionAmount`:** `LEDGER_PROJECTION_...:94` «الموجب/السالب حسب اتجاه المدين والدائن» مقابل `JOURNAL_ENTRY_...:95` «يجب أن يكون موجباً تماماً (>0) ... لا تُستخدم السالبة».
- **`Cancelled` بلا فعل تدقيق:** `JOURNAL_ENTRY_...:157` يلزم `ActorID` وسبباً وختم `CancelledAt/By/Reason`، لكن `GOVERNANCE_AND_AUDIT_...:120` يحصر الأفعال بـ `post, reverse, revaluation, reopen_period, close_period, policy_update` — لا `cancel`.
- **ذرية الإلحاق بالتدقيق:** `GOVERNANCE_AND_AUDIT_...:29` تشترط «إلحاق التدقيق داخل وحدة عمل الترحيل»، بينما `POSTING_...:89,199` يذكر «الختم التدقيقي» ثم «تفويض الحفظ مرة واحدة في الخطوة الأخيرة» دون قاعدة قابلة للقياس للإلحاق الذري.
- **بتر الذيل:** `BUSINESS_RULES.md:203-204` (RULE-ACC-022C) ينص صراحة «لا يكتشف وحده حذف ذيل السلسلة ... نقطة تحقق خارج النواة»، بينما نفس الملف `:302` (RULE-ACC-042) و`CAPABILITIES_INVENTORY.md:88` و`ACCEPTANCE_CRITERIA.md:73` و`SUCCESS_CRITERIA.md:74` يدّعون كشف «بما فيها حذف أواخر السجلات فوراً». حذف الذيل لا يترك فجوة تسلسلية، فالتعارض مستحيل التوفيق منطقياً.
- **Maker-Checker:** `CAPABILITIES_INVENTORY.md:49` و`ACCEPTANCE_CRITERIA.md:51` (AC-MAND-11) و`:70` (AC-POST-10) تجعل الرقابة المزدوجة شرطاً داخل النواة، مقابل `RULE-ACC-022B` في `BUSINESS_RULES.md:198` «لا تفرض النواة Maker-Checker ...» و`SCOPE_PROFILE...:58` «المضيف: Maker-Checker». الملاحظة `:70` تُصلح AC-POST-10 فقط لا AC-MAND-11.
- **حالات الفترات:** `CAPABILITIES_INVENTORY.md:66` `SOFT_LOCKED/CLOSED` مقابل `BUSINESS_RULES.md:167-169` `SOFT_CLOSED/HARD_CLOSED` و`SCOPE_PROFILE...:84` `SoftClosed/HardClosed`. و`:69` يشترط «رقم اعتماد» لإعادة الفتح مقابل `RULE-ACC-020:173` «أي موافقة خارج النواة».
- **أسماء أخطاء متعددة لنفس الحالة:** `ErrIdempotencyPayloadConflict` / `ErrIdempotencyConflict` / `ErrIdempotencyKeyConflict` في `IDENTIFIERS...:228`، `KERNEL_IDEMPOTENCY_CONTRACT.md:108`، `ENGINEERING_BUSINESS_RULES.md:97`، `ACCEPTANCE_CRITERIA.md:117`، بينما اللقطة تعرّف اثنين كرمزين مختلفين في حزمتين (`api/v1.snapshot:728,1506`). وتوثيق «ترادف» `ErrIdempotencyKeyRequired`/`ErrIdempotencyScopeRequired` في `IDENTIFIERS...:179,228` يُلغي التمييز الذي يعتمده أنبوب الترحيل (`:187,216`).
- **الاستعلام بعد Commit:** `KERNEL_IDEMPOTENCY_CONTRACT.md:133` يعترف أن `Lookup(companyID, key)` بلا عملية قد يُرجع سجل `post` عند طلب `reverse`، بينما `:137` يشترط «استرجاع دون تخمين»؛ الحجز ثلاثي (`:29`) لكن الاستعلام ثنائي.
- **تصنيف `ErrUniqueConstraint` (23505):** `KERNEL_LOCKING_CONTRACT.md:113` «نهائي / لا يُعاد المحاولة» مقابل `KERNEL_IDEMPOTENCY_CONTRACT.md:83` ترجمة إلى `ErrConcurrentModification` (قابل لإعادة المحاولة في `KERNEL_CONCURRENCY_MODEL.md:96`).
- **من يمرّر `OriginalEntryID`:** `KERNEL_REVERSAL_ATOMICITY_CONTRACT.md:57` يجعل الإضافة من عمل المحول، بينما `:70` يقول «النواة تمرر `OriginalEntryID` ... فقط»؛ ولا يوجد `OriginalEntryID` داخل `internal/kernal/**/*.go`.
- **صيغة البصمات:** `KERNEL_IDEMPOTENCY_CONTRACT.md:36` يتحدث عن «بصمتين» بصيغة `sha256:<hex>`، بينما `KERNEL_VALUATION_RULES_MATRIX.md:20` يُضيف بصمة v2 ببادئة `fx-revaluation-payload-v2|` دون تحديث القاعدة العامة.

### هـ. التقارير والروابط

- **تقرير الفجوات قديم/متناقض مع الحالة الراهنة:** `KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md:31-43` يصف ملفات `*_v1.0.0.md` بالإصدار الداخلي 1.1.0 وازدواج `MONEY_...ar.md`، والواقع `*_v1.2.0.md` وملف MONEY واحد. و`:84,99,122,116-117` تصنّف «حرجة/عالية» مسائل تم إصلاحها في المواصفات الحالية (`MASTERDATA:117` IsMonetary، `GOV:94` تأطير v2، `BUSINESS_RULES:296` ميكروثانية، `POSTING:117` تسامح 0.5، `LEDGER:94` رصيد العملة الأجنبية). المسارات `docs/specifications/kernal` و`TARGET_ACCOUNTING_KERNAL.md` غير موجودة.
- **أرقام التكافؤ:** `multi_company_isolation_report.md:75` «42 حالة (21×2)» مقابل `contract_parity_report.md:100` «90 passed (45×2)» و`KERNEL_RELEASE_SIGNOFF.md:49` «90».
- **الأداء:** `performance_samples.json` → `tps 63.3`، `p50 238.7ms`، `p95 337.7ms` مقابل `KERNEL_RELEASE_SIGNOFF.md:34` «12.55 ms» و`SUCCESS_CRITERIA.md:84` «1000 قيد/ثانية» و`release_rollback_runbook.md:11` «P50 ≤ 15ms / P95 ≤ 50ms». الفرق بين `ms/op` وقياس الحمل غير مفسّر.
- **Commit/الإصدار:** `KERNEL_RELEASE_SIGNOFF.md:7` `c28151d` مقابل `KERNEL_CONTRACT_TEST_SUITE.md:6` `94054d9` مقابل `SCOPE_PROFILE...:10-11` `e3318e4 / v2.0.0-candidate` مقابل `contract_parity_report.md:4` `(devel)`.
- **إعادة كتابة تقرير التكافؤ:** `fault_injection_report.md:62` ينص على أن تشغيلاً فرعياً يعيد كتابة التقرير مقابل `KERNEL_CONTRACT_TEST_SUITE.md:142` «منع `TestMain` من إعادة الكتابة حتى لا تُمحى الأدلة».
- **روابط مكسورة حية:** `docs/README.md:15` → `./analysis/` و`KERNEL_RELEASE_SIGNOFF.md:3` → `../analysis/DOCS_CONFLICT_ANALYSIS.md` (كانا غير موجودين؛ أُصلح الثاني بإنشاء هذا الملف). روابط `plans/` كانت مشطوبة لكنها تظل روابط markdown مكسورة تقنياً (19 موضعاً) — أُزيلت لاحقاً في مرحلة المعالجة. `disaster_recovery_runbook.md:49` → `docs/reports/p7_gate_evidence.md` مفقود. مراجع `docs/KERNEL_POSTING_FLOW_SPECIFICATION.md` و`docs/KERNEL_ARCHITECTURE_MAP.md` و`docs/KERNEL_AUDIT_REPOSITORY_CONTRACT.md` داخل المواصفات مساراتها الصحيحة تحت `docs/contracts/` و`docs/architecture/`.
- **لقطة API:** `:1064` تلف صريح: `pkg/spi/evالتحقق النهائي من الاختبارات والمعماريents`؛ و`MAP.md:100` تتعامل مع اللقطة كدليل لحزم `pkg/api/v1` فقط بينما اللقطة تشمل `pkg/sdk/**` و`pkg/spi/**` و`storage/**` (178 مدخلاً)؛ و`SDK_OVERVIEW.md:50` ينص «never storage/* directly» رغم عرضها في السطح.

---

## 3. تعارضات تم إصلاحها فعلياً في التعديلات الحالية

تأكدت أن هذه لم تعد مفتوحة رغم أن تقرير الفجوات القديم يسجّلها:

- دقة الطابع الزمني والتأطير: `RULE-ACC-041/042` الآن `v2 length-prefixed` + ميكروثانية (`BUSINESS_RULES.md:296-300`)، متوافقة مع `GOV-003`.
- أسماء الأخطاء، `Regular/Standard`، حقول `Cancelled`، سقف `0.5`، `IsMonetary`، رصيد العملة الأجنبية، أسماء الملفات `v1.2.0`، حذف نسخة `MONEY.ar.md`، لقطة ISO جديدة، تشطيب روابط `plans/`.
- **لكن** بقي أثر غير مكتمل: `CAPABILITIES_INVENTORY.md:87` لا يزال ينص على «ترميز بنيوي ذي فواصل صريحة» (v1) رغم انتقال `ENGINEERING_BUSINESS_RULES.md:153` و`GOV:94` إلى length-prefixed v2.

---

## 4. التوصيات مرتبة بالأولوية

1. **حسم حالة الاعتماد:** إمّا إزالة `SUPERSEDED` واستعادة `GO` مع تحديث المرجع، أو إبقاء الإلغاء وحذف/تعديل قرار `:55` وكل استشهادات الـ runbooks به، وإنشاء وثيقة اعتماد بديلة.
2. **استعادة أو حذف `analysis/`:** أُنشئ هذا الملف ليُصلح الرابط في `SIGNOFF:3`؛ يبقى أن تُحدَّث `docs/README.md:15` إن لزم، ويُصنَّف هذا الملف كتحليل لا كبديل لسجل الفجوات.
3. **إعادة توليد الـ Manifest** بعد تعديل Crosswalk، وتصحيح خريطة `G01/G1/G5` مقابل `QUALITY_GATES_SPECIFICATION.md`، وتوحيد بوابات القبول الـ6 مع الـ11.
4. **إصلاح `ACC-013`**: إما تغيير معرف ثابت «التوازن بالعملة الوظيفية» أو إعادة تسمية `RULE-ACC-013`، وتوثيق القرار في `DECISION_RECORDS`.
5. **تحديث `ARCHITECTURAL_PRINCIPLES_CHARTER.md` و`NEW_PROJECT_STRUCTURE.md`** ليتبعا ADR P1-01/P7-01/P8-01: تسع حزم، منع `kernal → pkg`، الجسر الوحيد لـ `api/v1`، النواة ترتّب لا تُقفل، لا RBAC/أرصدة مادية داخل النواة، ومسارات `internal/orbitals/*`.
6. **حسم التعارضات المالية:** ترتيب التقريب في `PST-004` مقابل `MONEY-028`، أولوية `0.5` مقابل المساواة الصارمة، سقف `int64` مقابل 28 خانة، إشارة `TransactionAmount`، وإضافة فعل `cancel` إلى `GOV-004`.
7. **إصلاح سطر 1064 في `api/v1.snapshot`** وإعادة تشغيل فحص توافق API؛ وإضافة `EntryCancelled` أو إزالة حالة `Cancelled` من المواصفة.
8. **أرشفة/تحديث تقرير الفجوات** ليُصنّف الملاحظات المُصلَحة كـ `Closed` بموضع الإثبات، وتوحيد أرقام التكافؤ (45 سيناريو/90 نتيجة) والأداء.
9. **تحسين الـ runbooks**: عتبة تغطية صحيحة للنواة (لا `coverage.out` الكامل 52.5%)، وتبرير RTO 5ث مقابل 30ث، وعدم الاعتماد على `pg_dump` وحده لادعاء RPO=0.

---

## 5. ملحق — أوامر التحقق المستخدمة

```bash
# حالة التعديلات والسياق التاريخي
git status --short
git log --oneline -12 -- docs
git log --all --oneline --diff-filter=D --name-only -- docs/analysis docs/plans

# سلامة حزمة الجودة
sha256sum -c docs/quality/MANIFEST.sha256

# التلف في لقطة الواجهة
sed -n '1064p' docs/api/v1.snapshot | cat -A

# الروابط المعلّقة
grep -rn "analysis/DOCS_CONFLICT_ANALYSIS\|\.\./analysis/\|docs/analysis" docs

# التعارضات المالية/التدقيقية
grep -rn "IsMonetary\|ErrIdempotencyKeyRequired\|ErrIdempotencyScopeRequired" docs
grep -rn "UnixNano()\|الميكروثانية\|النانو" docs/specifications/kernel/GOVERNANCE_AND_AUDIT_DOMAIN_SPECIFICATION_v1.2.0.md docs/rules/ACCOUNTING_BUSINESS_RULES.md
```

---

## 6. حالة المعالجة بعد التطبيق

قرارات المستخدم: **تطبيق كل الإصلاحات**، **إبقاء `SUPERSEDED`**، و**فصل namespace المعرّفات `ACC-xxx` عن `RULE-ACC-xxx` دون إعادة ترقيم**.

| المحور | ما طُبّق | موضع التحقق |
| --- | --- | --- |
| الاعتماد | إلغاء قرار `GO` صراحةً، وتحويل التقارير وأدلة التشغيل إلى سجلات تاريخية لا تمنح إذن نشر | `reports/KERNEL_RELEASE_SIGNOFF.md:53`، `reports/KERNEL_FINAL_VERIFICATION_REPORT.md:1`، `operations/runbooks/*.md` |
| الروابط | إنشاء `analysis/DOCS_CONFLICT_ANALYSIS.md` (هذا الملف) وإصلاح مسارات `kernal`/مراجع محذوفة؛ روابط `plans/` المشطوبة حُوّلت إلى نصوص دون روابط حية (19 موضعاً) | `README.md:3,14,15`، `reports/KERNEL_SPECIFICATIONS_GAP_AND_DEFECT_ANALYSIS.md:1` |
| الجودة والبوابات | تصحيح خريطة `G01–G11`، منع Waiver للثوابت/العزل، تنبيه أن `Pass` مرشح لا شهادة، استكمال ملف النطاق، فصل `ACC-013` | `quality/QUALITY_GATES_CROSSWALK.md`، `QUALITY_GATES_SPECIFICATION.md:40`، `COMPLIANCE_TEST_SUITE_INDEX.md:13`، `SCOPE_PROFILE_KERNEL_v1.md` |
| Manifest | إعادة توليد قيم الـ hash التسعة | `sha256sum -c docs/quality/MANIFEST.sha256` → 9/9 OK |
| المعمارية | تحديث الميثاق/الهيكل/الخريطة/ADR لتتبع القرارات: 9 حزم، منع `kernal→pkg`، جسر `api/v1` فقط، النواة ترتّب لا تُقفل، مسارات `internal/orbitals/*` | `architecture/*.md` |
| القواعد المالية | ترتيب التقريب، أولوية التسامح، سقف int64، إشارة `TransactionAmount`، فعل `cancel`، كشف بتر الذيل، Maker-Checker، حالات الفترات، أسماء الأخطاء | `specifications/kernel/*.md`، `rules/*.md`، `contracts/*.md` |
| اللقطة والتقارير | إصلاح السطر 1064 في `api/v1.snapshot`، توحيد 45 سيناريو/90 نتيجة، تحفّظ الأداء، إصلاح قاعدة إعادة كتابة تقرير التكافؤ | `api/v1.snapshot:1064`، `reports/contract_parity_report.md`، `reports/fault_injection_report.md:62` |

**لم يُطبَّق:** أي تغيير على شيفرة Go أو إنشاء وثيقة اعتماد جديدة؛ وُصفت الإجراءات المطلوبة لذلك داخل `KERNEL_RELEASE_SIGNOFF.md:53`.
