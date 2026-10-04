# procreepy

أداة صغيرة متعددة المنصات (Linux وWindows وmacOS): تستخرج تسجيل التايم-لابس الجاهز الموجود داخل أرشيف `.procreate` وتجمع مقاطعه في ملف MP4 واحد. لا يُعاد ترميز أي شيء (stream copy)، ولا يتم أي رندر.

`.procreate` هو أرشيف ZIP. إذا كان تسجيل التايم-لابس مفعّلًا، فإنه يحتوي على:

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
الأداة تأخذ هذه الملفات تحديدًا: ترتبها **رقميًا** (`segment-9` قبل `segment-10`)، وتحلل بنية MP4 لكل مقطع، ثم تعيد بناءها في ملف MP4 واحد من نوع moov-first مع نسخ الإطارات كما هي. وفي وضع الدفعة تكتب أيضًا، لكل عمل مُعالَج، نسخة من المشروع يمكن إعادة استيرادها من دون التايم-لابس، ومع الخيار `--psd` تكتب مستند PSD بطبقات Photoshop. ومسار التايم-لابس لا يفتح مطلقًا `Document.archive` أو الطبقات أو كتل البيانات النقطية (`*.lz4`) — فقط `--psd` يفتحها.

## المتطلبات

لا توجد تبعيات خارجية: لا حاجة إلى `ffmpeg` أو `ffprobe`. يتطلب البناء Go فقط (الإصدار موجود في `go.mod`) و`make` (متوفر على جميع المنصات المدعومة؛ وفي الأنظمة الدنيا يمكن تثبيته عبر مدير الحزم).
Makefile هو مدخل البناء الأساسي المعتمد: فهو يثبت نفس البيئة المعزولة التي تستخدمها CI (وضع الوحدات غير المتصل بالشبكة، وtoolchain محلية، ومن دون cgo)، ويحدد موقع Go toolchain تلقائيًا.

```bash
make build      # تجميع كل شيء وإنشاء ./procreepy قابل للتشغيل
make check      # gofmt + build + vet + مجموعة الاختبارات كاملة
```

يقوم `make build` بوسم الملف التنفيذي بـ `dev-<short sha>` (خارج checkout من Git: `dev-nogit`)، بحيث يعرض `procreepy --version` مصدره (بينما تحمل حزم إصدار release الوسم بدلًا من ذلك). وبدون `make` يكون المكافئ المباشر هو `go build ./... && go build -o procreepy ./cmd/procreepy` (يعرض ذلك الملف التنفيذي `dev`، أو `dev-<commit>` حين تكون ختمات VCS متاحة).

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
جميع عمليات البناء ثابتة (من دون cgo): يعمل ملف Linux التنفيذي على أي توزيعة بغض النظر عن إصدار glibc. وتبني خطوط CI (GitLab وGitHub) هذه الأهداف نفسها مع كل commit، وكما أن Windows هو المنصة الرئيسية فتشغّل أيضًا مجموعة الاختبارات كاملة أصلًا على Windows؛ وتنشر مهمة `dist` حزم tarball مع بيان `SHA256SUMS`، بينما تثبت مهام `repro:*` قابلية إعادة إنتاج الملفات التنفيذية بتطابق تام على مستوى البتات.

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
```
تدعم الأداة جميع التركيبات الأربع من `INPUT`/`OUTPUT`: `FILE OUTPUT` و`FILE -` و`- OUTPUT` و`- -`. إذا لم يُحدّد `OUTPUT`، يكون الناتج على stdout. وإذا كان `OUTPUT` مجلدًا موجودًا، يوضع الفيديو داخله بالاسم الأصلي (`procreepy art.procreate videos/` → `videos/art.mp4`).

### وضع الدفعة: مجلد من ملفات `.procreate` → مجلدات من الفيديوهات والمشاريع

سيناريو «لدي `input/` مليء بملفات `.procreate` وأريد النتائج في `output/`»:

```bash
procreepy input/
```

كل عمل يُحوَّل يعطي زوجًا — التايم-لابس ومشروعًا يمكن إعادة استيراده من دونه:

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              (تم تخطيه، مع تحذير — لم يُكتب شيء)
```
- **الأسماء**: `<الاسم الأصلي من دون .procreate>`, مع إضافة `.mp4` أو
  `.procreepy.procreate`. تُحفظ المسافات والأحرف السيريلية والمحارف الخاصة
  كما هي؛ وعلى نظام ملفات لا يميز بين الأحرف الكبيرة والصغيرة، تتلقى الأسماء
  التي تختلف فقط في درجة الأحرف لاحقات `-2`, `-3`, …
- **مجلد الإخراج**: افتراضيًا `output/` بالنسبة إلى المجلد الحالي، ويُنشأ
  تلقائيًا. ويمكن تحديد مجلد آخر كوسيط ثانٍ: `procreepy input/ ~/Videos/procreate`.
- **المشروع المُخفَّف**: `projects/NAME.procreepy.procreate` هو الأرشيف نفسه
  من دون أعضاء `video/segments/segment-N.mp4`؛ وتُنقل جميع الأعضاء الأخرى
  بايتًا ببايت (الترتيب، طرق الضغط، الطوابع الزمنية). يحتفظ المشروع بحظة
  تعديل ملفه الأصلي، كي لا يعيد استيراده في Procreate ترتيب معرضك من جديد.
- **إعادة التشغيل آمنة**: يُتخطى الملف الذي وُجدتا فيه النتيجة الاثنتان
  مسبقًا؛ والزوج الناقص يُبنى من جديد بالكامل. لإعادة بناء كل شيء استخدم
  `--force` (`-f`).
- **`-r`** يتنقل أيضًا داخل المجلدات الفرعية؛ وتُعكس بنيتها في كلا الشجرتين،
  لذلك لا تتصادم الأسماء المتطابقة في مجلدات مختلفة.
- **الملف التالف لا يوقف بقية الملفات.** الملف الذي لا يحتوي على تايم-لابس
  (أي إن التسجيل كان متوقفًا) ينتج تحذيرًا لا خطأً: لا يُكتب له شيء. أما
  الملف التالف فهو خطأ: يظهر في الملخص النهائي ويصبح رمز الخروج `1`.
- **مجموعات ذرية**: تُكتب نتائج كل إدخال (التايم-لابس + المشروع، وPSD مع
  `--psd`) إلى ملفات مؤقتة ثم تُنشر معًا كمجموعة واحدة — كلها أو لا شيء.
  التشغيل الفاشل لا يترك أبدًا فيديو يتيم بجانب مشروع مفقود.
- تُتجاهل الملفات المخفية (`._Foo.procreate`، التي يتركها macOS عند النسخ).
- لا يتم تعديل الملفات الأصلية أبدًا.

مثال على الخرج (كله إلى stderr؛ وينتهي التنفيذ برمز الخروج `1` بسبب الملف التالف):

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` هو عدد ملفات المقاطع التي حُذفت من المشروع المُخفَّف،
و`video_size` هو حجمها الإجمالي (بعد الضغط) داخل الأرشيف. تشغيل نفس الأمر
مرة أخرى يبلغ عن كل زوج موجود:

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

كما تقبل `--list` و`--verify` مجلدًا، وتتنقلان عبر جميع الملفات داخله.

### تصدير PSD (`--psd`)

```bash
procreepy --psd input/ out/
```

إدخال مجلدي فقط. إلى جانب كل زوج محوَّل يُكتب `out/psd/NAME.psd`، ويُصدَّر
بشكل ذري مع بقية المجموعة:

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

ما يحمله PSD:

- شجرة الطبقات (المجموعات، الترتيب)، أسماء الطبقات (Unicode)، الظهور،
  الشفافية، أوضاع المزج، الحدود وحالة القفل؛
- بكسلات RGBA ثمانية البت لكل طبقة، مضغوطة بطريقة PackBits;
- قيم DPI وملف ICC المدمج;
- التركيبة المدمجة كما هي من رندر الالتواء الخاص بـ Procreate نفسه (وعند
  غياب هذا الرندر أو تلفه تُركَّب الطبقات الظاهرة في الوضع Normal كتقريب).

ما لا يحمله — PSD هو تصدير وليس دورة عودة بلا فقد:

- أقنعة الطبقات والدقة الكاملة لدلالات القص إلى الطبقة الأدنى لا تنجو؛
- طبقات النص تحتفظ ببكسلاتها لكن لا تحتفظ ببيانات النص القابلة للتحرير؛
- ألفا المباشر (غير المضاعف مسبقًا) لا يمكن استعادته بدقة: Procreate يخزن
  بلاطات ثمانية البت مضاعفة مسبقًا، لذا قد تختلف ألوان الحواف عند أطراف
  الطبقات اختلافًا طفيفًا؛
- تُرفض اللوحات الأوسع أو الأطول من 30000 بكسل رفضًا قاطعًا (حد من صيغة
  PSD، بعكس PSB).

ملف `.procreate` يبقى النسخة الأصلية؛ اعتبر PSD لقطة سريعة لـ Photoshop
وباقي المستوردين.

### التشخيص

```bash
procreepy --list artwork.procreate
```

```text
input: input/Portrait of a Cat.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
3  video/segments/segment-3.mp4
...
17 video/segments/segment-17.mp4
18 video/segments/segment-18.mp4
```

```bash
procreepy --verify artwork.procreate
```
يحلل كل مقطع مباشرة من الأرشيف (بما في ذلك فحص CRC داخل ZIP)، ويطبع تقريرًا سطرًا بسطر، ويتحقق من إمكانية ضم المقاطع من دون إعادة ترميز. **لا يتم إنشاء أي فيديو إخراج.** أما `--list` فيقرأ فهرس ZIP فقط.

## الخيارات

| الخيار | الوظيفة |
|---|---|
| `-h`, `--help` | عرض المساعدة والخروج |
| `--list` | سرد المقاطع بترتيب التشغيل والخروج |
| `--verify` | التحقق من كل مقطع؛ لا يُنشأ أي فيديو إخراج |
| `-r`, `--recursive` | عند إدخال مجلد: النزول أيضًا إلى المجلدات الفرعية |
| `-f`, `--force` | عند إدخال مجلد: استبدال النتائج الموجودة مسبقًا |
| `--strict` | اعتبار أرقام المقاطع المفقودة خطأً (تحذير افتراضيًا) |
| `--psd` | عند إدخال مجلد: تصدير PSD بطبقات لكل عمل إضافيًا |
| `--tmpdir DIR` | مكان وضع الملفات المؤقتة (افتراضيًا: `$TMPDIR`، ثم `/var/tmp`، ثم مجلد النظام المؤقت) |
| `-q`, `--quiet` | طباعة التحذيرات والأخطاء فقط |
| `--version` | عرض رقم الإصدار والخروج |
| `--` | وقف تحليل الخيارات؛ ومعاملة ما تبقى كوسيطة موضعية |

## كيف تعمل الأداة

1. `INPUT` ملف أو stdin. تتم كتابة stdin (وأي إدخال غير قابل للبحث) أولًا إلى ملف مؤقت، لأن ZIP يتطلب وصولًا عشوائيًا.
2. يُتحقق من صحة ZIP (للقراءة فقط)، ثم تُحدد إدخالات `video/segments/segment-N.mp4`.
3. ترتيب رقمي. الفجوات في الترقيم تحذير؛ والأسماء التي لا تحتوي على رقم تُتجاهل مع تحذير.
4. يُحلل كل مقطع مباشرة من ZIP (من دون استخراج كامل): صناديق MP4، وأحجام المسارات، ومعلمات الترميز. عند أول تلف — تتوقف العملية.
5. فحص التوافق (الدقة، وبرنامج الترميز، ومجموعات SPS/PPS، والصوت). وإلا فإن `-c copy` قد ينتج ملفًا تالفًا بصمت؛ لذلك يكون عدم التوافق خطأً برسالة واضحة، لا مفاجأة في الفيديو النهائي.
6. يُجمع MP4 من نوع moov-first: `ftyp`، ثم `moov` (جميع المسارات، مقتطعة من المقاطع)، ثم `mdat` بعد `mdat` بترتيب التشغيل.
7. يُجهز كل ما وعد به التنفيذ (MP4، والمشروع المُخفَّف، وPSD) في ملفات
   مؤقتة، ولا يُنشر كمجموعة واحدة إلا بعد نجاح الأخيرة؛ وفي وضع الدفعة
   يُؤخذ الإدخال التالي مهما حدث.
8. يُحذف الملف المؤقت، إن وُجد، دائمًا — عند النجاح، وعند الخطأ، وعند Ctrl+C، وعند SIGTERM.

### الكتابة إلى ملف وإلى stdout

يبني المساران ملف MP4 واحدًا من نوع moov-first: تُكتب ذرّة moov أولًا لأن الإطارات تُنسخ مباشرة من المقاطع المصدرية وتكون البيانات الوصفية معروفة قبل بدء الكتابة. بالنسبة إلى ملف، فهذا MP4 «كلاسيكي» مناسب لكل من مشغلات الفيديو والمحررات؛ والملف نفسه تمامًا يمر عبر الأنبوب — تنتج `> artwork.mp4` النتيجة نفسها التي تنتجها صراحةً `procreepy artwork.procreate artwork.mp4`.
- **الإخراج إلى ملف** ذري: يُنشأ ملف `.partial` بجانب الهدف، ولا يُعاد تسميته إلا بعد النجاح. التشغيل الفاشل لا يترك بقايا ولا يفسد ملفًا موجودًا.
- خاصية خاصة بويندوز: تتم إعادة التسمية النهائية عبر `MoveFileEx` مع استبدال الملف الموجود، لذا فإن `--force` وإعادة توليد مجموعة غير مكتملة يستبدلان المخرجات الموجودة في مكانها تمامًا كما في يونكس. العملية ليست ذريةً بذات صرامة أمر rename في POSIX، والفرق العملي يظهر في حالة واحدة فقط — إذا كان الملف الهدف لا يزال مفتوحًا في برنامج آخر (مشغل وسائط ما مثلًا ما زال محمَّلًا على MP4 السابق)، تُرفض إعادة التسمية بخلل قابل للقراءة باسم `Access is denied`، ويبقى الملف القديم سليمًا دون مساس، ثم ينجح تشغيل جديد بعد إغلاق البرنامج.
- لا يختلط stdout بالنص أبدًا. تذهب جميع أسطر السجل (`level=INFO`/`WARN`/`ERROR`، سجل structured واحد من نوع key=value في كل سطر) إلى stderr. والاستثناء الوحيد هو تقرير `--list`/`--verify` حيث يكون stdout *هو* النتيجة. وإذا كان stdout طرفية، ترفض الأداة سكب MP4 ثنائي إليها.

### ألوان الطرفية

إذا كان stderr طرفية تفاعلية ولم تُعرَّف متغير البيئة `NO_COLOR`، تُلوَّن
رموز المستوى `WARN` و`ERROR` (بالأصفر وبالاحمر الغامق)؛ ويبقى `INFO` دون
لون. والأنابيب وإعادة التوجيه وCI والاختبارات تحافظ على التنسيق البسيط
بايتًا ببايت، فلا يتغير شيء في سير العمل المكتوب. ولا يوجد خيار `--color`
عن قصد.

### الملفات المؤقتة وFedora

على Fedora يكون `/tmp` عبارة عن tmpfs في الذاكرة. وقد يبلغ حجم مقاطع التايم-لابس مئات الميغابايت، كما أن القراءة من stdin تعني كتابة ملف `.procreate` كاملًا مؤقتًا. لذلك يُختار مجلد الملفات المؤقتة بهذا الترتيب: `--tmpdir` → `$TMPDIR` → `/var/tmp` (على القرص) → الإعداد الافتراضي للنظام. إذا امتلأ القرص أثناء الكتابة المؤقتة، تحصل على خطأ واضح مع إرشاد (استخدم `--tmpdir` مع مجلد أكبر مدعوم بالقرص) بدل رسالة مقتضبة من نوع "No space left".

## رموز الخروج

| الرمز | المعنى |
|---|---|
| 0 | نجاح |
| 1 | خطأ غير متوقع؛ في وضع الدفعة — فشل ملف واحد على الأقل |
| 2 | وسيطات غير صحيحة (منها `--psd` مع ملف واحد، أو توجيه مجلد النتائج كاملًا إلى stdout)؛ OUTPUT هو نفس ملف INPUT؛ stdout طرفية |
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

مسار التايم-لابس لا يحلل `Document.archive` (NSKeyedArchive)، ولا يمس
`*.lz4`، ولا يستعيد الطبقات، ولا يعمل على رندر الصورة؛ أما `--psd` فيحلل
المستند — من أجل التصدير المذكور أعلاه، ومع تحفظات الدقة المذكورة في ذلك
القسم. وإذا لم يتم تسجيل تايم-لابس في الملف، فلا يمكن لهذه الأداة
استعادته من سجل الرسم. ملاحظة: تشغيل `lz4 -t` على ملف `.lz4` مأخوذ من
`.procreate` ليس فحصًا لسلامة البيانات — فهذه ليست إطارات LZ4 مستقلة.

## مراجع الصيغة

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## الترخيص

Apache License 2.0، انظر `LICENSE`.

---

## اللغات

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दী](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
