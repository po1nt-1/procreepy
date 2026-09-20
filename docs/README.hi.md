# procreepy

हर OS के लिए एक छोटी Unix उपयोगिता (Linux, Windows, macOS): यह `.procreate` फ़ाइल से तैयार
आर्काइव टाइमलाप्स निकालती है और उसके खंडों को एक ही MP4 में जोड़ देती है।
कोई री-एन्कोडिंग नहीं (स्ट्रीम कॉपी), कोई रेंडरिंग नहीं।

`.procreate` एक ZIP है। अगर टाइमलाप्स रिकॉर्डिंग चालू थी, तो अंदर ये
होते हैं

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

यूटिलिटी बिल्कुल इन ही फ़ाइलों को लेती है: इन्हें **संख्यात्मक** रूप से
sort करती है (`segment-9`, `segment-10` से पहले), हर खंड की MP4 संरचना
parse करती है, और इन्हें एक moov-first MP4 में पुनर्निर्मित करती है —
फ्रेम जैसे-तैसे कॉपी किए जाते हैं। `Document.archive`, लेयर्स या
रस्टर चंक (`*.lz4`) उसकी कभी नहीं खुलतीं।

## आवश्यकताएँ

कोई बाहरी निर्भरता नहीं: न `ffmpeg`, न `ffprobe`। बिल्ड के लिए
केवल Go चाहिए (version `go.mod` में है)।

```bash
go build -o procreepy ./cmd/procreepy
```

### दुरसरी OS केलिए बिल्ड करन

ये प्रोजेक्ट शुद्ध Go हαι और सभाइ समर्थित target केलिए साफ-साफ़
cross-compile होतै है। किसी भी platform से, इनमें से कोई भी command
चलेगी:

| Target | Command |
|---|---|
| Linux x86-64 | `GOOS=linux GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `GOOS=linux GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |
| Linux ARM 32-bit | `GOOS=linux GOARCH=arm GOARM=7 go build -o procreepy ./cmd/procreepy` |
| Windows x86-64 (10/11) | `GOOS=windows GOARCH=amd64 go build -o procreepy.exe ./cmd/procreepy` |
| Windows ARM 64-bit | `GOOS=windows GOARCH=arm64 go build -o procreepy.exe ./cmd/procreepy` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o procreepy ./cmd/procreepy` |
| macOS Apple Silicon (M1–M5) | `GOOS=darwin GOARCH=arm64 go build -o procreepy ./cmd/procreepy` |

सभी builds static हैं (cgo नहीं): Linux binary किसी भी distribution पर
चलती है, चाहे उसकी glibc की version कुछ भी हो। GitLab CI pipeline हर
commit पर बिल्कुल ये target बिल्ड करती है; `dist` job tarball और
`SHA256SUMS` manifest publish करता है, और `repro:*` jobs साबित करते हैं
कि binaries bit-by-bit reproduce होती हैं।

- Linux/macOS: installation step नहीं है, binary सीधे चलाइए।
- Windows: binary sign नहीं है, इसलिये SmartScreen "आपके PC की सुरक्षा हो
  गई" दिखा सकता है — **और जानकारी → फिर भी चलाइए** चुनिए।

## उपयोग

एक फ़ाइल:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

चारों `INPUT`/`OUTPUT` संयोजन सपोर्टेड हैं: `FILE OUTPUT`, `FILE -`,
`- OUTPUT`, `- -`। अगर `OUTPUT` छोड़ा जाए, तो वह stdout है। अगर `OUTPUT`
एक मौजूदा फ़ोल्डर है, तो वीडियो मूल नाम पर उसमें जाता है
(`procreepy art.procreate videos/` → `videos/art.mp4`)।

### बैच मोड: `.procreate` वाला फ़ोल्डर → वीडियो वाला फ़ोल्डर

"मेरे पास `input/` भरई `.procreate` से है, और वीडियो चाहिए `output/timelaps/`
में" का सीनारियो:

```bash
procreepy input/
```

```text
input/                           output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (छोड़ा गया, चेतावनी के साथ)
```

- **नाम**: `<मूल नाम बिना .procreate>.mp4`। स्पेस, सिरिलिक और विशेष वर्ण
  जैसे-तैसे रहते हैं।
- **आउटपुट फ़ोल्डर**: डिफ़ॉल्ट रूप से वर्तमान फ़ोल्डर से relatively
  `output/timelaps/`, स्वयं बनता है। दूसरा दूसरे आर्ग्यूमेंट से दिया जा
  सकता है: `procreepy input/ ~/Videos/procreate`।
- **फिर चलाना सुरक्षित है**: मौजूदा वीडियो skip हो जाते हैं। सब कुछ दोबारा
  बनाने के लिए: `--force` (`-f`)।
- **`-r`** सबफ़ोल्डरों में भी जाता है; सबफ़ोल्डर संरचना रिज़ल्ट में mirror
  होती है (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`),
  इसलिए अलग-अलग फ़ोल्डरों में समान नाम टकराते नहीं हैं।
- **एक बुरी फ़ाइल बाकियों को रोकती नहीं है।** बिना टाइमलाप्स वाली फ़ाइल
  (रिकॉर्डिंग बंद थी) चेतावनी है, ग़लती नहीं। टूटी फ़ाइल ग़लती है: वह
  अंतिम सारांश में आएगी, और exit code `1` होगा।
- छिपी फ़ाइलें (`._Foo.procreate`, जिन्हें macOS कॉपी करते समय पीछे छोड़ता
  है) ignore हो जाती हैं।
- मूल फ़ाइलें कभी नहीं बदलतीं।

उदाहरण आउटपुट (सब stderr पर जाता है):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`--list` और `--verify` भी फ़ोल्डर स्वीकार करती हैं और सभी फ़ाइलों पर
घूमती हैं।

### विभाजन: वीडियो + हल्का प्रोजेक्ट (`--split`)

माकसद: टाइमलाप्स स्वयं चित्र से ज़्यादा जगह घेरते हैं — जैसे iPad पर बैकअप
लेते समय इन्हें अलग रखना अच्छा लगता है। `--split` हर तैयार `MP4` के बगल
में प्रोजेक्ट की हल्की कॉपी लिखती है, जिसमें `video/` का **कुछ भी नहीं**:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (टाइमलाप्स, lossless)
                     →  artwork.procreepy.procreate  (वही प्रोजेक्ट, video/ के बिना)
```

बैच मोड में वही: हर `X.mp4` के बगल में `X.procreepy.procreate` दिखता है।
आर्काइव के बाकी सभी members (लेयर्स, `Info.plist`, प्रीव्यू) बित-दर-बित
ले जाते हैं: क्रम, कंप्रेशन विधियाँ और timestamps बरक़रार रहते हैं। मूल
`.procreate` नहीं बदलता; हल्की कॉपी को stdout में नहीं लिखा जा सकता,
इसलिए `--split` को फ़ाइल `OUTPUT` चाहिए।

### निदान

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

हर खंड को सीधे आर्काइव से parse करती है (ZIP के अंदर CRC जाँच सहित),
लाइन-दर-लाइन रिपोर्ट दिखाती है, और जाँचती है कि खंड बिना री-एन्कोडिंग के
जोड़े जा सकेंगे। **कोई आउटपुट वीडियो बनाया नहीं जाता।** `--list` सिर्फ
ZIP का directory पढ़ती है।

## विकल्प

| विकल्प | काम |
|---|---|
| `-r`, `--recursive` | डायरेक्टरी इनपुट: सबफ़ोल्डर भी घूमे |
| `-f`, `--force` | डायरेक्टरी इनपुट: मौजूदा वीडियो पर लिखना |
| `--strict` | अनुपस्थित segment numbers को error मानें (डिफ़ॉल्ट में warning) |
| `--reencode` | पुराने स्क्रिप्टों के साथ संगतता के लिए स्वीकार किया जाता है; री-एन्कोडिंग नहीं, हमेशा stream copy |
| `--split` | हर `MP4` के बगल में `X.procreepy.procreate` लिखे — `video/` के बिना प्रोजेक्ट |
| `--tmpdir DIR` | segments कहाँ उतारे जाएँ |
| `-q`, `--quiet` | सिर्फ warning/error दिखाएँ |

## यह कैसे काम करती है

1. `INPUT` फ़ाइल या stdin है। Stdin (और कोई भी non-seekable इनपुट) पहले
   एक अस्थायी फ़ाइल में spool किया जाता है, क्योंकि ZIP को random access
   चाहिए।
2. ZIP validate होता है (केवल पठन), फिर `video/segments/segment-N.mp4`
   entries ढूँधी जाती हैं।
3. संख्यात्मक क्रम। Numbering में गैप — warning; बिना number के नाम
   warning के साथ ignore होते हैं।
4. हर खंड ZIP से सीधे parse होता है (पूरी extraction के बिना): MP4 boxes,
   track आकार, codec पैरामीटर। पहली टूट पर — रुकना।
5. संगतता जाँच (resolution, codec, SPS/PPS sets, ऑडियो)। वरना `-c copy`
   चुपचाप कूड़ा दे देता — इसलिए असंगतता एक स्पष्ट message के साथ error है,
   तैयार वीडियो में आश्चर्य नहीं।
6. moov-first MP4 बँधा जाता है: `ftyp`, `moov` (सभी tracks, segments से
   काटी हुई), फिर playback order में `mdat` के बाद `mdat`।
7. अस्थायी फ़ाइल (अगर बनी हो) हमेशा हटाई जाती है — सफलता, त्रुटि, Ctrl+C
   और SIGTERM — तीनों में।

### फ़ाइल और stdout में लिखना

दोनों रास्तों पर वही moov-first MP4 बनता है: moov atom पहले लिखा जाता
है, क्योंकि फ्रेम source segments से सीधे कॉपी होते हैं और metadata
लिखने से पहले ही ज्ञात होता है। फ़ाइल के लिए यह "क्लासिक" MP4 है —
players और editors दोनों के लिए उपयुक्त; वही ठीक-ठीक फ़ाइल pipe में भी
जाती है — `> artwork.mp4` साफ़ `procreepy artwork.procreate artwork.mp4`
जैसा ही निकलता है।

- **फ़ाइल** आउटपुट atomic है: target के बगल में `.partial`, सिर्फ सफलता
  के बाद rename। असफल चाली अवशेष नहीं छोड़ती और मौजूदा फ़ाइल कभी ख़राब
  नहीं करती।
- stdout कभी text से नहीं पसता। सभी संरचित `level=INFO`/`WARN`/`ERROR`
  लाइनें (हर event के लिए एक key=value लाइन) stderr पर जाती हैं।
  अकेला अपवाद `--list`/`--verify` की रिपोर्ट है, जहाँ
  stdout *ही* परिणाम है। अगर stdout टर्मिनल है, तो यूटिलिटी वहाँ binary
  MP4 डालने से इनकार दे देती है।

### अस्थायी फ़ाइलें और Fedora

Fedora पर `/tmp` RAM में tmpfs है। टाइमलाप्स के segments सैकड़ों
मेगाबाइट के हो सकते हैं, और stdin से पढ़ते समय पूरी `.procreate` spool
होती है। इसलिए temp directory इस तरह चुनी जाती है: `--tmpdir` →
`$TMPDIR` → `/var/tmp` (डिस्क पर)। extraction से पहले खाली जगह की जाँच
होती है; कम पड़ने पर काम के बीच "No space left" की बजाय एक स्पष्ट error
सुझाव के साथ मिलता है।

## Exit codes

| कोड | अर्थ |
|---|---|
| 0 | सफलता |
| 1 | अप्रत्याशित त्रुटि; बैच मोड में — कम से कम एक फ़ाइल विफल |
| 2 | ग़لط आर्ग्यूमेंट; आउटपुट इनपुट को overwrite कर देता; stdout टर्मिनल है |
| 3 | इनपुट नहीं मिला, खाली है या ZIP नहीं है |
| 4 | आर्काइव में `video/segments` नहीं (टाइमलाप्स रिकॉर्ड नहीं हुआ था) |
| 5 | segment ख़राब; अस्पष्ट या अनुपस्थित numbering (`--strict`) |
| 6 | रखा गया (व्यवहारिक: बाहरी निर्भरताएँ नहीं हैं) |
| 7 | segments stream copy के लिए असंगत |
| 8 | रखा गया (व्यवहारिक: बाहरी निर्भरताएँ नहीं हैं) |
| 9 | परिणाम या अस्थायी फ़ाइलों की लेखन त्रुटि |
| 130 | बीच में रूका (Ctrl+C / SIGTERM) |

## टेस्ट

```bash
go test ./...
```

असली `.procreate` की ज़रूरत नहीं: टेस्ट generated MP4 segments से ZIP
बनाते हैं (देखें `internal/testkit`)। जाँचें संरचनात्मक हैं: निकले MP4 का
विश्लेषण, box क्रम, sample संख्याएँ, `mdat` सामग्री। `-race` ज़रूरी नहीं,
पर C कम्पाइलर स्थापित होने पर चलता है।

कवर: सामान्य फ़ाइल, `video/segments` की अनुपस्थिति, एक segment, गड़बड़
क्रम के segments (`segment-9`/`segment-10`), stdin, stdout, नामों में
स्पेस और विशेष वर्ण, ख़राब और कटे हुए ZIP, ख़राब और कटे हुए MP4, CRC क्षति,
लेखन त्रुटियाँ (`/dev/full`), असंगत segments, और पूरा बैच मोड।

## जो यूटिलिटी जानबूझकर नहीं करती

`Document.archive` (NSKeyedArchive) parse नहीं करती, `*.lz4` को नहीं छूती,
लेयर्स पुनर्स्थापित नहीं करती और छवि render नहीं करती। अगर फ़ाइल में
टाइमलाप्स रिकॉर्ड नहीं हुआ था, तो यह यूटिलिटी उसे चित्रण इतिहास से वापस
नहीं ला सकती। ध्यान दें: `.procreate` से निकाले गए `.lz4` पर `lz4 -t`
संपूर्णता जाँच नहीं है — वे स्वतंत्र LZ4 फ्रेम नहीं हैं।

## फ़ॉर्मेट संदर्भ

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## लाइसेंस

Apache License 2.0, देखें `LICENSE`।

---

## भाषाएँ

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
