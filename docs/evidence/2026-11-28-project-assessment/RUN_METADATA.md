# سجل تشغيل تقييم المشروع ككل

**وقت التسجيل:** 2026-11-28 04:15 UTC  
**Git HEAD:** `0e0f81ac79d0f6f2f04bd92ee4707be93d531ef5`  
**بيئة الاختبار:** Linux/amd64؛ Go `1.26.3`; PostgreSQL server `16.15`; `pg_dump`/`pg_restore` `16.15`; 4 CPUs متاحة و15.5 GiB ذاكرة بحسب البيئة الظاهرة.  
**نطاق الأوامر:** مصدر Go عند HEAD أعلاه. تغييرات شجرة العمل أثناء التشغيل محصورة في تقارير/أدلة التقييم و`docs/reports/performance_samples.json` الذي يولده benchmark.

## النتائج

| المجموعة | الأمر | النتيجة | السجل |
| --- | --- | --- | --- |
| المستودع الكامل عدا فحص SLO | `POSTGRES_URL=<isolated PG16> go test ./... -count=1 -skip='^TestPostingLatencySLO$'` مع `pgcrypto` مثبتاً في `public` وأدوات PG16 في `PATH` | Pass؛ جميع الحزم المدرجة في وحدة الجذر اجتازت | [full_repo_without_slo.log](full_repo_without_slo.log) |
| SLO مستقل | `POSTGRES_URL=<isolated PG16> POSTING_SLO_WORKERS=16 POSTING_SLO_OPS=1000 go test -v ./test/benchmark -run '^TestPostingLatencySLO$' -count=1` | **Fail**: 87.60 TPS؛ P50 157.63 ms؛ P95 257.91 ms؛ P99 1084.58 ms. الحدود: 1000 TPS، P50≤15 ms، P95≤50 ms | [performance_slo.log](performance_slo.log)، [performance_samples.json](../../reports/performance_samples.json) |
| المستهلك الخارجي | من `test/external_consumer`: `GOCACHE=/tmp/accountant_engine_assessment_gocache go test ./... -count=1` | Pass | [external_consumer.log](external_consumer.log) |
| race | `go test -race ./internal/kernel/... ./test/concurrency/... -count=1` | Pass | [kernel_and_concurrency_race.log](kernel_and_concurrency_race.log) |
| ثبات ترتيب اختبارات النواة | `go test ./internal/kernel/... -shuffle=on -count=10` | Pass | [kernel_shuffle.log](kernel_shuffle.log) |
| المعمارية | `./scripts/verify_architecture.sh` | Pass؛ جميع قواعد الفحص المعلنة اجتازت | [architecture.log](architecture.log) |
| توافق API | `./scripts/check_api_compat.sh` | Pass؛ لا إزالة أو كسر في اللقطة الحالية | [api_compat.log](api_compat.log) |
| التحليل الساكن | `go vet ./...` | Pass | [go_vet.log](go_vet.log) |
| التنسيق | `gofmt -l internal modules pkg storage test` ثم `gofmt -l .` ضمن فحص السلامة | Pass؛ لا ملفات غير منسقة | [gofmt.log](gofmt.log)، [source_integrity.log](source_integrity.log) |
| سلامة المصدر | `GOCACHE=/tmp/accountant_engine_assessment_gocache ./scripts/verify_integrity.sh` | Pass؛ `go mod verify`, tidy، مواثيق الحزم، والاعتماديات اجتازت | [source_integrity.log](source_integrity.log) |
| بصمات وثائق الجودة | `sha256sum -c docs/quality/MANIFEST.sha256` | Pass؛ 12/12 ملفاً مطابق | [quality_manifest.log](quality_manifest.log) |
| DR والاستعادة | `go test ./...` ضمن مجموعة المستودع مع أدوات PG16 | Pass ضمن [full_repo_without_slo.log](full_repo_without_slo.log) |
| ترحيل backfill للتدقيق منفرداً | `go test ./storage/postgres -run '^TestPostgresPhase10BackfillRechainsLegacyRows$' -count=1 -v` | Pass عند التشغيل المنفرد | [postgres_migration_targeted.log](postgres_migration_targeted.log) |

## المحاولات التشخيصية الأولى

1. التشغيل الأول `POSTGRES_URL=<isolated PG16> go test ./... -count=1` انتهى بفشل ثلاث نقاط:
   - اختبار backfill واجه `function digest(bytea, unknown) does not exist` عند التشغيل المتوازي على مخططات مؤقتة. الاختبار نفسه اجتاز منفرداً؛ وبعد تثبيت `pgcrypto` في schema مشتركة `public` اجتازت حزمة PostgreSQL في إعادة التشغيل.
   - اختبار الأداء فشل حدود SLO؛ أعيد قياسه منفرداً لتقليل التنافس، وظل فاشلاً بالقياسات المسجلة أعلاه.
   - DR استخدم `pg_restore 18.6` مع خادم `16.15`، وفشل عند `transaction_timeout`. أعيد التشغيل بأدوات PG16 المطابقة واجتاز DR.
   تفاصيل التشغيل الأول في [full_repo.log](full_repo.log).

2. التشغيل المصحح استثنى **اختبار الأداء وحده** من مجموعة التحقق العامة، بعد تشغيله منفرداً. لذلك نتيجة المجموعة `Pass` لا تعني أن SLO اجتاز؛ نتيجة SLO هي `Fail` كما هو ظاهر أعلاه.

3. أظهر فحص checksum الأول ثلاث بصمات قديمة في manifest لملفات كانت مطابقة للنسخة المثبتة في HEAD ولم تتغير في شجرة العمل. حُدّثت قيم البصمات إلى محتوى الملفات الحالي وأضيفت ملفات التقييم إلى القائمة؛ نتيجة الفحص النهائي `12/12 OK`.

## حدود الاستنتاج

- هذه الأدلة تثبت نتائج الاختبارات على بيئة التطوير المسجلة، ولا تثبت الأداء على عتاد الإنتاج أو كل نسخة PostgreSQL مدعومة.
- `pgcrypto` في `public` شرط إعداد استُخدم لمنع تعارض تثبيت الامتداد في مخططات الاختبار المتوازية. يجب توثيق إعداد قاعدة الاختبار هذا في CI.
- نجاح الوحدات واختباراتها لا يعد مراجعة مستقلة لمطابقة IFRS/IAS، ولا يغطي سيناريوهات لم تذكرها مجموعة الاختبارات.
- `RUN-20261128-FINAL` في فهرس الجودة يخص `90c121e`؛ لا تخلط نتائجه بنتائج هذه الحزمة.
