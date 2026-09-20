# procreepy

أداة Unix صغيرة متعددة المنصات (Linux وWindows وmacOS): تستخرج مَجموع فيديو
التايم-لابس الجاهزة من ملف `.procreate` وتُجمّع مقاطعه في ملف MP4 واحد. لا
يُعاد ترميز أي شيء (stream copy)، ولا يتم أي رندر.

`.procreate` هو ZIP. إذا كان تسجيل التايم-لابس مفعلاً، ففيه

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

الأداة تأخذ هذه الملفات تحديداً: تُرتّبها **رقمياً** (`segment-9` قبل
`segment-10`)، وتحلّل بنية MP4 لكل مقطع، ثم تعيد بناءها في MP4 من النوع
moov-first مع نسخ الإطارات كما هي. لا تفتح `Document.archive` أو الطبقات
أو كتل الـ raster (`*.lz4`).

## المتطلبات

لا تبعيات خارجية: لا حاجة إلى `ffmpeg` ولا `ffprobe`. للتجميع
يُكفي Go (النسخة في `go.mod`).

```bash
go build -o procreepy ./cmd/procreepy
```

### البناء لأنظمة تشغيل أخرى

المشروع Go خالص ويُجمَّع عبر التجميع المتقاطع (cross-compile) بشكل سليم
لجميع المنصات المدعومة. من أي منصة، يعمل أيٌّ من هذه الأوامر:

| المنصة | الأمر |
|---|---|
| Linux x86-64 | `GOOS=linux GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `GOOS=linux GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 32-bit | `GOOS=linux GOARCH=arm GOARM=7 go build -o procreepy ./cmd/procreepy` |
| Windows x86-64 (10/11) | `GOOS=windows GOARCH=amd64 go build -o procreepy.exe ./cmd/procreepy` |
| Windows ARM 64-bit | `GOOS=windows GOARCH=arm64 go build -o procreepy.exe ./cmd/procreepy` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| macOS Apple Silicon (M1–M5) | `GOOS=darwin GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |

جميع التجميعات ثابتة (بدون cgo): ملف التنفيذ الخاص بـ Linux يعمل على أي
توزيعة مهما كان إصدار glibc فيها. خط GitLab CI يجمِّع تمامًا هذه
المنصات مع كل commit؛ مهمة `dist` تنشر حزم tarball مع قائمة `SHA256SUMS`،
ومهام `repro:*` تثبت أن الملفات التنفيذية قابلة لإعادة الإنتاج بتطابق
تام عند مستوى البت.

- Linux/macOS: لا توجد خطوة تثبيت، شغِّل ملف التنفيذ مباشرة.
- Windows: الملف غير موقَّع، لذلك قد يعرض SmartScreen "تم حماية جهازك" —
  اختر **المزيد من المعلومات → تنفيذ على أي حال**.

## الاستعمال

ملف واحد:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

تُقبل كل تركيبات `INPUT`/`OUTPUT` الأربع: `FILE OUTPUT`، `FILE -`،
`- OUTPUT`، `- -`. عند إهمال `OUTPUT` يكون الهدف stdout. وإذا كان `OUTPUT`
مجلداً موجوداً يوضع الفيديو فيه باسمه الأصلي
(`procreepy art.procreate videos/` → `videos/art.mp4`).

### وضع الدفعة: مجلد من `.procreate` → مجلد من الفيديوهات

سيناريو «عندي `input/` مليء بملفات `.procreate` وأريد الفيديوهات في
`output/timelaps/`»:

```bash
procreepy input/
```

```text
input/                              output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate              └── (تُتجاهل مع تحذير)
```

- **الأسماء**: `<الاسم الأصلي دون .procreate>.mp4`. المسافات والأحرف
  السيريلية والرموز الخاصة محفوظة كما هي.
- **مجلد الخروج**: الافتراضي `output/timelaps/` نسبة إلى المجلد الحالي،
  ويُنشأ تلقائياً. ويمكن إعطاء غيره كالحجة الثانية:
  `procreepy input/ ~/Videos/procreate`.
- **إعادة التشغيل آمنة**: الفيديوهات الموجودة تُتجاوز. لإعادة بناء كل شيء:
  `--force` (`-f`).
- **`-r`**: ينزل أيضاً إلى المجلدات الفرعية؛ تُعكس بنية المجلدات الفرعية في
  النتيجة (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`)،
  فلا تتعارض الأسماء المتماثلة في مجلدات مختلفة.
- **ملف معطوب لا يوقف البقية.** الملف بدون تايم-لابس (كان التسجيل
  غير مفعّل) تحذير وليس خطأً. الملف التالف خطأ: يظهر في الملخص النهائي
  ويصبح رمز الخروج `1`.
- تُتجاهل الملفات المخفية (`._Foo.procreate`، ما يتركه macOS عند النسخ).
- ملفات المصدر لا تُعدَّل أبداً.

مثال على الإخراج (كله إلى stderr):

```text
info: 4 .procreate file(s) in input -> output/timelaps/
info: [1/4] input/Landscape v2.procreate -> output/timelaps/Landscape v2.mp4
warning: [2/4] input/No Timelapse.procreate: no timelapse video inside, skipped
error: [3/4] input/Corrupt file.procreate: input is not a valid ZIP archive ...
info: [4/4] input/Portrait of a Cat.procreate -> output/timelaps/Portrait of a Cat.mp4
info: summary: 2 converted, 1 without timelapse, 1 FAILED
```

`--list` و`--verify` يقبلان مجلداً أيضاً ويتصفحان كل ملفاتِه.

### التقسيم: فيديو + مشروع مُخفَّف (`--split`)

الفكرة: التايم-لابس تشغل مساحة أكبر من الرسمة نفسها — مثلاً عند النسخ
الإحتياطي على iPad قد تريد الاحتفاظ بها منفصلة. `--split` يكتب بجانب
كل `MP4` جاهزة نسخة مخففة من المشروع **بدون** أي شيء تحت `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (التايم-لابس، بلا فقد)
                     →  artwork.procreepy.procreate  (نفس المشروع، بلا video/)
```

في وضع الدفعة الأمر ذاته: بجانب كل `X.mp4` يظهر `X.procreepy.procreate`.
جميع أعضاء الأرشيف الأخرى (الطبقات، `Info.plist`, المعاينات) تُنقل حرفياً
بايتاً بايت: الترتيب وطرائق الضغط والطوابع الزمنية محفوظة. الملف الأصلي لا
يُعدَّل؛ والنسخة المخففة لا يمكن كتابتها على stdout، لذا `--split` يشترط
`OUTPUT` ملف.

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

يحلل كل مقطع مباشرة من الأرشيف (بما في ذلك فحص CRC داخل ZIP)، ويعرض
تقرير سطراً سطراً، ويتأكد أن المقاطع قابلة للدمج دون إعادة ترميز. **لا
يُنشأ أي فيديو خروج.** `--list` يقرأ فهرس ZIP فقط.

## الخيارات

| الخيار | الدور |
|---|---|
| `-r`, `--recursive` | إدخال مجلد: تصفح المجلدات الفرعية أيضاً |
| `-f`, `--force` | إدخال مجلد: استبدال الفيديوهات الموجودة |
| `--strict` | اعتبار أرقام المقاطع الناقصة خطاً (الافتراضي تحذير) |
| `--reencode` | مقبول من أجل توافق السكربتات القديمة؛ لا إعادة ترميز، دائماً stream copy |
| `--split` | يكتب `X.procreepy.procreate` بجانب كل `MP4` — المشروع بلا `video/` |
| `--tmpdir DIR` | أين تُفكّك المقاطع |
| `-q`, `--quiet` | طباعة التحذيرات والأخطاء فقط |

## كيف تعمل

1. `INPUT` ملف أو stdin. يُسكب stdin (وأي إدخال غير قابل للإرشاد) أولاً في
   ملف مؤقت، لأن ZIP يتطلب وصولاً عشوائياً.
2. تُتحقق صحة ZIP (قراءة فقط) ثم تُحدد مدخلات
   `video/segments/segment-N.mp4`.
3. ترتيب رقمي. فجوات الترقيم تحذير؛ الأسماء بلا رقم تُتجاهل مع تحذير.
4. يحلل كل مقطع مباشرة من ZIP (دون فك كامل): صناديق MP4، أحجام المسارات،
   معاملات المترجم. عند أول عطب — توقف.
5. فحص التوافق (الدقة، مترجم الفيديو، مجموعات SPS/PPS، الصوت). وإلا
   لكان `-c copy` يُنتج هراءً بصمت — لذا عدم التوافق خطأ برسالة واضحة،
   لا مفاجأة في الفيديو النهائي.
6. يُبنى MP4 من نوع moov-first: `ftyp`، ثم `moov` (كل المسارات، مقتطعة
   من المقاطع)، ثم `mdat` بعد `mdat` بترتيب العرض.
7. يُحذف الملف المؤقت (إن وُجد) دائماً — عند النجاح، وعند الخطأ، وعند
   Ctrl+C وعند SIGTERM.

### الكتابة إلى ملف وإلى stdout

مساران يبنيان نفس MP4 من نوع moov-first: تُكتب ذرّة moov أولاً لأن
الإطارات تُنسخ مباشرة من المقاطع المصدرية والميتاداتا معروفة قبل بدء
الكتابة. للملف يكون هذا MP4 «كلاسيكياً» يصلح للاعشين والمحررات على حد
سواء؛ والملف نفسه تماماً يمر عبر الأنبوب — `> artwork.mp4` يعطي النتيجة
نفسها تماماً كـ`procreepy artwork.procreate artwork.mp4` الصريح.

- الخروج إلى **ملف** ذرّي: ملف `.partial` بجانب الهدف، يُعاد تسميته فقط
  بعد النجاح. التشغيل الفاشل لا يترك بقايا ولا يفسد ملفاً موجوداً أبداً.
- لا يتلوث stdout بالنص أبداً. كل أسطر `info:`/`warning:`/`error:` تذهب
  إلى stderr. الاستثناء الوحيد تقرير `--list`/`--verify` حيث stdout *هو*
  النتيجة. إذا كان stdout طرفية ترفض الأداة سكب MP4 ثنائي فيه.

### الملفات المؤقتة وFedora

على Fedora يكون `/tmp` tmpfs في الذاكرة. مقاطع التايم-لابس قد تبلغ
مئات الميغابايت، وقراءة stdin تعني إسكاب كامل `.procreate`. لذلك يُختار
مجلد المؤقتات هكذا: `--tmpdir` ← `$TMPDIR` ← `/var/tmp` (على القرص).
تُفحص المساحة الحرة قبل الفك؛ وإن نقصت فالحصيلة خطأ واضح بدليل بدل «No
space left» في منتصف العمل.

## رموز الخروج

| الرمز | المعنى |
|---|---|
| 0 | نجاح |
| 1 | خطأ غير متوقع؛ في وضع الدفعة — فشل ملف واحد على الأقل |
| 2 | حجج خاطئة؛ الخروج سيستبدل الإدخال؛ stdout طرفية |
| 3 | الإدخال غير موجود أو فارغ أو ليس ZIP |
| 4 | لا `video/segments` في الأرشيف (لم يسجل تايم-لابس) |
| 5 | مقطع معطوب؛ ترقيم غامض أو ناقص (`--strict`) |
| 6 | محجوز (غير مستخدم: لا تبعيات خارجية) |
| 7 | مقاطع غير متوافقة للـ stream copy |
| 8 | محجوز (غير مستخدم: لا تبعيات خارجية) |
| 9 | فشل كتابة الناتج أو الملفات المؤقتة |
| 130 | تم الإيقاف (Ctrl+C / SIGTERM) |

## الاختبارات

```bash
go test ./...
```

لا حاجة إلى ملفات `.procreate` حقيقية: تُبني الاختبارات ZIPs من مقاطع
MP4 مولدة (انظر `internal/testkit`). الفحوص بنيوية: تحليل MP4 الناتج،
ترتيب الصناديق، عدد العينات، محتوى `mdat`. `-race` ليس مطلوباً لكنه يعمل
إذا وُجد مُجمِّع C.

المغطى: الملف العادي، غياب `video/segments`، مقطع واحد، مقاطع غير مرتبة
(`segment-9`/`segment-10`)، stdin، stdout، مسافات ورموز خاصة في الأسماء،
ZIPs معطوبة ومقطوعة، MP4s معطوبة ومقطوعة، تلف CRC، أخطاء كتابة
(`/dev/full`)، مقاطع غير متوافقة، ووضع الدفعة كاملاً.

## ما لا تفعله الأداة عن قصد

لا تحلل `Document.archive` (NSKeyedArchive)، ولا تمسّ `*.lz4`، ولا
استعيد الطبقات ولا ترندر الصورة. إذا لم يُسجل تايم-لابس في الملف فلا يمكن
لهذه الأداة استرجاعه من سجل الرسم. ملاحظة: `lz4 -t` على `.lz4` أُخرج من
`.procreate` ليس فحص سلامة — فهي ليست إطارات LZ4 مستقلة.

## مراجع الصيغة

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## الرخصة

Apache License 2.0، انظر `LICENSE`.

---

## اللغات

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
