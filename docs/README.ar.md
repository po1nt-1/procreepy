# procreepy

أداة صغيرة متعددة المنصات (Linux وWindows وmacOS): تستخرج تسجيل التايم-لابس الجاهز الموجود داخل أرشيف `.procreate` وتجمع مقاطعه في ملف MP4 واحد. لا يُعاد ترميز أي شيء (stream copy)، ولا يتم أي رندر.

`.procreate` هو أرشيف ZIP. إذا كان تسجيل التايم-لابس مفعّلًا، فإنه يحتوي على:

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
الأداة تأخذ هذه الملفات تحديدًا: ترتبها **رقميًا** (`segment-9` قبل `segment-10`)، وتحلل بنية MP4 لكل مقطع، ثم تعيد بناءها في ملف MP4 واحد من نوع moov-first مع نسخ الإطارات كما هي. ولا تفتح مطلقًا `Document.archive` أو الطبقات أو كتل البيانات النقطية (`*.lz4`).

## المتطلبات

لا توجد تبعيات خارجية: لا حاجة إلى `ffmpeg` أو `ffprobe`. يتطلب البناء Go فقط (الإصدار موجود في `go.mod`) و`make` (متوفر على جميع المنصات المدعومة؛ وفي الأنظمة الدنيا يمكن تثبيته عبر مدير الحزم).
Makefile هو مدخل البناء الأساسي المعتمد: فهو يثبت نفس البيئة المعزولة التي تستخدمها CI (وضع الوحدات غير المتصل بالشبكة، وtoolchain محلية، ومن دون cgo)، ويحدد موقع Go toolchain تلقائيًا.

```bash
make build      # تجميع كل شيء وإنشاء ./procreepy قابل للتشغيل
make check      # gofmt + build + vet + مجموعة الاختبارات كاملة
```

يقوم `make build` بوسم الملف التنفيذي بـ `dev-<commit>`، بحيث يعرض `procreepy --version` مصدره (بينما تحمل حزم الإصدار tarball الوسم بدلًا من ذلك). وبدون `make` يكون المكافئ المباشر هو `go build ./... && go build -o procreepy ./cmd/procreepy` (يعرض ذلك الملف التنفيذي `dev`، أو `dev-<commit>` عند بنائه داخل checkout من Git).

### البناء لأنظمة تشغيل أخرى

المشروع مكتوب بلغة Go الخالصة ويمكن cross-compile له بسهولة لجميع الأهداف المدعومة. من أي منصة:

| الهدف | الأمر |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32-bit | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64-bit | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
ينتج كل هدف ملفًا موحدًا باسم `dist/procreepy-<version>-<os>-<arch>.tar.gz` ويطبع قيمة SHA-256 الخاصة به؛ ويبني `make cross` المصفوفة كاملة دفعة واحدة، بينما يثبت `make repro` أن البناء قابل لإعادة الإنتاج على مستوى كل بت. والمكافئ المباشر لهدف واحد هو `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.
جميع عمليات البناء ثابتة (من دون cgo): يعمل ملف Linux التنفيذي على أي توزيعة بغض النظر عن إصدار glibc. وتبني خطوط CI (GitLab وGitHub) هذه الأهداف نفسها مع كل commit؛ وتنشر مهمة `dist` حزم tarball مع بيان `SHA256SUMS`، بينما تثبت مهام `repro:*` قابلية إعادة إنتاج الملفات التنفيذية بتطابق تام على مستوى البتات.

- Linux/macOS: لا توجد خطوة تثبيت؛ شغّل الملف التنفيذي مباشرة.
- Windows: الملف التنفيذي غير موقّع، لذا قد يعرض SmartScreen رسالة "Protected your PC" — اختر **More info → Run anyway**.

## الاستخدام

ملف واحد:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```
تدعم الأداة جميع التركيبات الأربع من `INPUT`/`OUTPUT`: `FILE OUTPUT` و`FILE -` و`- OUTPUT` و`- -`. إذا لم يُحدّد `OUTPUT`، يكون الناتج على stdout. وإذا كان `OUTPUT` مجلدًا موجودًا، يوضع الفيديو داخله بالاسم الأصلي (`procreepy art.procreate videos/` → `videos/art.mp4`).

### وضع الدفعة: مجلد من ملفات `.procreate` → مجلد من الفيديوهات

سيناريو «لدي `input/` مليء بملفات `.procreate` وأريد الفيديوهات في `output/timelaps/`»:

```bash
procreepy input/
```

```text
input/                              output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate              └── (تم تخطيه، مع تحذير)
```
- **الأسماء**: `<الاسم الأصلي من دون .procreate>.mp4`. تُحفظ المسافات والأحرف السيريلية والمحارف الخاصة كما هي.
- **مجلد الإخراج**: افتراضيًا `output/timelaps/` بالنسبة إلى المجلد الحالي، ويُنشأ تلقائيًا. ويمكن تحديد مجلد آخر كوسيط ثانٍ: `procreepy input/ ~/Videos/procreate`.
- **إعادة التشغيل آمنة**: يتم تخطي الفيديوهات الموجودة. لإعادة بناء كل شيء استخدم `--force` (`-f`).
- **`-r`** يتنقل أيضًا داخل المجلدات الفرعية؛ وتُعكس بنيتها في النتيجة (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`)، لذلك لا تتصادم الأسماء المتطابقة في مجلدات مختلفة.
- **الملف التالف لا يوقف بقية الملفات.** الملف الذي لا يحتوي على تايم-لابس (أي إن التسجيل كان متوقفًا) ينتج تحذيرًا لا خطأً. أما الملف التالف فهو خطأ: يظهر في الملخص النهائي ويصبح رمز الخروج `1`.
- تُتجاهل الملفات المخفية (`._Foo.procreate`، التي يتركها macOS عند النسخ).
- لا يتم تعديل الملفات الأصلية أبدًا.
مثال على الخرج (كله إلى stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
كما تقبل `--list` و`--verify` مجلدًا أيضًا، وتتنقلان عبر جميع الملفات داخله.

### التقسيم: فيديو + مشروع مُخفَّف (`--split`)

الفكرة: ملفات التايم-لابس تستهلك مساحة أكبر من الرسم نفسه؛ مثلًا عند أخذ نسخة احتياطية على iPad قد ترغب في إبقائها منفصلة. يكتب `--split` إلى جانب كل `MP4` مكتمل نسخة مخففة من المشروع **من دون** أي شيء تحت `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (التايم-لابس، بلا فقد)
                     →  artwork.procreepy.procreate  (المشروع نفسه، من دون video/)
```
وفي وضع الدفعة يعمل الأمر بالطريقة نفسها: يظهر `X.procreepy.procreate` بجانب كل `X.mp4`. وتُنقل جميع أعضاء الأرشيف الأخرى (الطبقات، `Info.plist`، المعاينات) بايتًا ببايت: مع الحفاظ على الترتيب وطرق الضغط والطوابع الزمنية. لا يتم تعديل ملف `.procreate` الأصلي؛ ولا يمكن كتابة النسخة المخففة إلى stdout، لذلك يتطلب `--split` قيمة `OUTPUT` تشير إلى ملف.

### التشخيص

```bash
procreepy --list artwork.procreate
```

```text
input: artwork.procreate
segments: 12

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
12 video/segments/segment-12.mp4
```

```bash
procreepy --verify artwork.procreate
```
يحلل كل مقطع مباشرة من الأرشيف (بما في ذلك فحص CRC داخل ZIP)، ويطبع تقريرًا سطرًا بسطر، ويتحقق من إمكانية ضم المقاطع من دون إعادة ترميز. **لا يتم إنشاء أي فيديو إخراج.** أما `--list` فيقرأ فهرس ZIP فقط.

## الخيارات

| الخيار | الوظيفة |
|---|---|
| `-r`, `--recursive` | عند إدخال مجلد: النزول أيضًا إلى المجلدات الفرعية |
| `-f`, `--force` | عند إدخال مجلد: استبدال الفيديوهات الموجودة |
| `--strict` | اعتبار أرقام المقاطع المفقودة خطأً (تحذير افتراضيًا) |
| `--reencode` | مقبول للتوافق مع السكربتات القديمة؛ لا توجد إعادة ترميز، ودائمًا يتم استخدام stream copy |
| `--split` | كتابة `X.procreepy.procreate` بجانب كل `MP4` — المشروع من دون `video/` |
| `--tmpdir DIR` | مكان وضع الملفات المؤقتة |
| `-q`, `--quiet` | طباعة التحذيرات والأخطاء فقط |

## كيف تعمل الأداة

1. `INPUT` ملف أو stdin. تتم كتابة stdin (وأي إدخال غير قابل للبحث) أولًا إلى ملف مؤقت، لأن ZIP يتطلب وصولًا عشوائيًا.
2. يُتحقق من صحة ZIP (للقراءة فقط)، ثم تُحدد إدخالات `video/segments/segment-N.mp4`.
3. ترتيب رقمي. الفجوات في الترقيم تحذير؛ والأسماء التي لا تحتوي على رقم تُتجاهل مع تحذير.
4. يُحلل كل مقطع مباشرة من ZIP (من دون استخراج كامل): صناديق MP4، وأحجام المسارات، ومعلمات الترميز. عند أول تلف — تتوقف العملية.
5. فحص التوافق (الدقة، وبرنامج الترميز، ومجموعات SPS/PPS، والصوت). وإلا فإن `-c copy` قد ينتج ملفًا تالفًا بصمت؛ لذلك يكون عدم التوافق خطأً برسالة واضحة، لا مفاجأة في الفيديو النهائي.
6. يُجمع MP4 من نوع moov-first: `ftyp`، ثم `moov` (جميع المسارات، مقتطعة من المقاطع)، ثم `mdat` بعد `mdat` بترتيب التشغيل.
7. يُحذف الملف المؤقت، إن وُجد، دائمًا — عند النجاح، وعند الخطأ، وعند Ctrl+C، وعند SIGTERM.

### الكتابة إلى ملف وإلى stdout

يبني المساران ملف MP4 واحدًا من نوع moov-first: تُكتب ذرّة moov أولًا لأن الإطارات تُنسخ مباشرة من المقاطع المصدرية وتكون البيانات الوصفية معروفة قبل بدء الكتابة. بالنسبة إلى ملف، فهذا MP4 «كلاسيكي» مناسب لكل من مشغلات الفيديو والمحررات؛ والملف نفسه تمامًا يمر عبر الأنبوب — تنتج `> artwork.mp4` النتيجة نفسها التي تنتجها صراحةً `procreepy artwork.procreate artwork.mp4`.
- **الإخراج إلى ملف** ذري: يُنشأ ملف `.partial` بجانب الهدف، ولا يُعاد تسميته إلا بعد النجاح. التشغيل الفاشل لا يترك بقايا ولا يفسد ملفًا موجودًا.
- لا يختلط stdout بالنص أبدًا. تذهب جميع أسطر السجل (`level=INFO`/`WARN`/`ERROR`، سجل structured واحد من نوع key=value في كل سطر) إلى stderr. والاستثناء الوحيد هو تقرير `--list`/`--verify` حيث يكون stdout *هو* النتيجة. وإذا كان stdout طرفية، ترفض الأداة سكب MP4 ثنائي إليها.

### الملفات المؤقتة وFedora

على Fedora يكون `/tmp` عبارة عن tmpfs في الذاكرة. وقد يبلغ حجم مقاطع التايم-لابس مئات الميغابايت، كما أن القراءة من stdin تعني كتابة ملف `.procreate` كاملًا مؤقتًا. لذلك يُختار مجلد الملفات المؤقتة بهذا الترتيب: `--tmpdir` → `$TMPDIR` → `/var/tmp` (على القرص) → الإعداد الافتراضي للنظام. إذا امتلأ القرص أثناء الكتابة المؤقتة، تحصل على خطأ واضح مع إرشاد (استخدم `--tmpdir` مع مجلد أكبر مدعوم بالقرص) بدل رسالة مقتضبة من نوع "No space left".

## رموز الخروج

| الرمز | المعنى |
|---|---|
| 0 | نجاح |
| 1 | خطأ غير متوقع؛ في وضع الدفعة — فشل ملف واحد على الأقل |
| 2 | وسيطات غير صحيحة؛ الإخراج سيستبدل الإدخال؛ stdout طرفية |
| 3 | الإدخال غير موجود أو فارغ أو ليس ZIP |
| 4 | لا يوجد `video/segments` في الأرشيف (لم يُسجّل تايم-لابس) |
| 5 | مقطع تالف؛ ترقيم ملتبس أو مفقود (`--strict`) |
| 6 | محجوز (غير مستخدم: لا تبعيات خارجية) |
| 7 | المقاطع غير متوافقة مع stream copy |
| 8 | محجوز (غير مستخدم: لا تبعيات خارجية) |
| 9 | فشل في كتابة النتيجة أو الملفات المؤقتة |
| 130 | تمت المقاطعة (Ctrl+C / SIGTERM) |

## الاختبارات

```bash
make test          # or: go test ./...
```

لا حاجة إلى ملفات `.procreate` حقيقية: تبني الاختبارات أرشيفات ZIP من مقاطع MP4 مولدة (انظر `internal/testkit`). والفحوص بنيوية: تحليل MP4 الناتج، وترتيب الصناديق، وعدد العينات، ومحتوى `mdat`. الخيار `-race` غير مطلوب، لكنه يعمل عند توفر مترجم C.
وتشمل الاختبارات: الملف العادي، وغياب `video/segments`، ومقطعًا واحدًا، ومقاطع بترتيب غير صحيح (`segment-9`/`segment-10`)، وstdin، وstdout، والمسافات والمحارف الخاصة في الأسماء، وأرشيفات ZIP تالفة ومبتورة، وملفات MP4 تالفة ومبتورة، وتلف CRC، وأخطاء الكتابة (`/dev/full`)، والمقاطع غير المتوافقة، ووضع الدفعة بالكامل.

## ما لا تفعله الأداة عن قصد

لا تحلل `Document.archive` (NSKeyedArchive)، ولا تمس `*.lz4`، ولا تستعيد الطبقات، ولا تعمل على رندر الصورة. وإذا لم يتم تسجيل تايم-لابس في الملف، فلا يمكن لهذه الأداة استعادته من سجل الرسم. ملاحظة: تشغيل `lz4 -t` على ملف `.lz4` مأخوذ من `.procreate` ليس فحصًا لسلامة البيانات — فهذه ليست إطارات LZ4 مستقلة.

## مراجع الصيغة

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## الترخيص

Apache License 2.0، انظر `LICENSE`.

---

## اللغات

[English](README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
