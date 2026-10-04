# procreepy

एक छोटा क्रॉस-प्लैटफ़ॉरम् यूटिलिटี (Linux, Windows, macOS): यह `.procreate`
फ़ाइल सें पहले सें तैयार archive timelapse निकालती है और उसके segments को एक
MP4 में जोड़ देती है। कुछ भी re-encode नहीं किया जाता (stream copy), और कुछ
भी render नहीं किया जाता।

`.procreate` एक ZIP archive है। अगर timelapse recording चालू थी, तो इसमें
ये फ़ाइलें होती हैं

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
यूटिलिटी ठीक इनहीं फ़ाइलों को लेती है: इन्हें **संख्यاتمक रूप से** sort करती
है (`segment-9`, `segment-10` से पहले), हर segment की MP4 संरचना parse करती है,
और frames को जस का तस कॉपी करके एक moov-first MP4 बनाती है। Batch mode में
यह हर process हुई काम के लिए timelapse के बिना एक दोबारा import हो सकने वाला
project की copy लिखती है, और `--psd` फ्लैग से एक layers वाला Photoshop PSD
document भी। Timelapse का रास्ता कभी भी `Document.archive`, layers या raster
chunks (`*.lz4`) नहीं खोलता — सिर्फ `--psd` खोलता है।

## आवश्यकताएँ

कोई बाहरी dependency नहीं है: `ffmpeg` या `ffprobe` की ज़रूरत नहीं। Build के लिए केवल Go (version `go.mod` में है) और `make` चाहिए (यह सभी supported platforms पर उपलब्ध है; minimal systems पर package manager से आसानी से install किया जा सकता है)।
Makefile build का canonical entry point है: यह वह ही hermetic environment तय करता है जिससे CI इस्तेमाल करता है (offline module mode, local toolchain, no cgo) और Go toolchain को अपने-आप खोजता है।

```bash
make build      # सब कुछ compile करके runnable ./procreepy बनाना
make check      # gofmt + build + vet + पूरी test suite
```

`make build` binary में `dev-<short sha>` दर्ज करता है (git checkout के बाहर:
`dev-nogit`), इसलिए `procreepy --version` बताता है कि binary कहाँ से बन गई
है (release tarballs में इसकी जगह tag होता है)। `make` के बिना raw
equivalent है `go build ./... && go build -o procreepy ./cmd/procreepy` (वह
binary `dev` रिपोर्ट करती है, या `dev-<commit>` अगर VCS stamping उपलब्ध हो)।

### दूसरे operating systems के लिए build करना

Project pure Go है और सभी supported targets के लिए साफ़ तरीके से cross-compile होता है। किसी भी platform से:

| Target | Command |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32-bit | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64-bit | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
हर target एक normalized `dist/procreepy-<version>-<os>-<arch>.tar.gz` बनाता है और उसका SHA-256 दिखाता है; `make cross` पूरी matrix को एक साथ build करता है, और `make repro` साबित करता है कि build bit-for-bit reproducible है। एक target के लिए raw equivalent है `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`।
सभी builds static हैं (no cgo): Linux binary किसी भी distribution पर चलेगी, glibc का version कुछ भी हो। CI pipelines (GitLab और GitHub) हर commit पर इन्हीं targets को build करती हैं और, क्योंकि Windows मुख्य platform है, पूरी test suite को Windows पर भी natively चलाती हैं; `dist` job tarballs और `SHA256SUMS` manifest publish करती है, और `repro:*` jobs binaries की bit-for-bit reproducibility साबित करती हैं।

- Linux/macOS: किसी installation step की ज़रूरत नहीं; binary सीधे चलाएँ।
- Windows: binary unsigned है, इसलिए SmartScreen “Protected your PC” दिखा सकता है — **More info → Run anyway** चुनें।

## उपयोग

एक फ़ाइल:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
```
`INPUT`/`OUTPUT` की चारों combinations supported हैं:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`। अगर `OUTPUT` नहीं दिया गया है, तो
output stdout है। अगर `OUTPUT` पहले से मौजूद directory है, तो video वहाँ मूल
नामे से रखा जाता है (`procreepy art.procreate videos/` → `videos/art.mp4`)।

### Batch mode: `.procreate` फ़ाइलों वाला folder → videos और projects वाले folders

Scenario: `input/` में बहुत-सी `.procreate` फ़ाइलें हैं और results को
`output/` में चाहिए:

```bash
procreepy input/
```

हर converted काम एक जोड़ी देता है — timelapse और एक दोबारा import हो सकने
वाला project जो उससे बन्या है:

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              (छोड़ा गया, warning के साथ — कुछ भी लिखा नहीं)
```
- **नाम**: `<मूल नाम बिना .procreate>`, साथ में `.mp4` या
  `.procreepy.procreate`। spaces, Cyrillic और special characters जस के तस
  रहते हैं; case-insensitive file system पर केवल case में अलग नामों को
  `-2`, `-3`, … जैसे suffixes मिलते हैं।
- **Output folder**: डिफ़ॉल्ट रूप से current directory के सापेक्ष `output/`,
  जो अपने-आप बनता है। दूसरा folder दूसरे argument से दिया जा सकता है:
  `procreepy input/ ~/Videos/procreate`।
- **Slim project**: `projects/NAME.procreepy.procreate` वही archive है
  बिना members `video/segments/segment-N.mp4`; बाकी सभी members byte-for-byte
  carried होते हैं (order, compression methods, timestamps)। project अपने
  source का modification time रखता है, ताकि Procreate में दोबारा import
  करने से gallery न उलझें।
- **दोबारा चलाना सुरक्षित है**: जिस input का timelapse और project पहले से
  मौजूद हैं वह skip हो जाता है; अधूरा pair पूरी तरह से दोबारा बनाया जाता
  है। सब कुछ फिर से बनाने के लिए: `--force` (`-f`)।
- **`-r`** sub-folders में भी जाता है; उनका structure दोनों trees में
  mirrored होता है, इसलिये अलग folders में एक जैसे नाम आपस में नहीं
  टकराते।
- **एक ख़राब फ़ाइल बाकी फ़ाइलों को नहीं रोकती।** बिना timelapse वाली
  फ़ाइल (recording बंद थी) warning है, error नहीं: उसके लिए कुछ भी लिखा
  नहीं जाता। Corrupt फ़ाइल error है: वह final summary में आती है और
  exit code `1` हो जाता है।
- **Atomic sets**: एक input की सभी outputs (timelapse + project, और `--psd`
  के साथ PSD) temporary files में लिखी जाती हैं और पूरे set के रूप में
  published होती हैं — या सब कुछ या कुछ नहीं। असफल run कभी भी orphaned
  video और missing project का मिश्रण नहीं छोड़ता।
- Hidden files (`._Foo.procreate`, जिन्हें macOS copy करते समय छोड़ता है) ignore होती हैं।
- Original files कभी बदली नहीं जातीं।

Example output (सारा output stderr पर जाता है; corrupt फ़ाइल की वजह से run
code `1` के साथ खत्म होता है):

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` उन segment files की संख्या है जो slim project से हटाई गईं,
और `video_size` उनका कुल (compressed) size archive के अंदर है। वही command
दोबारा चलाने पर हर मौजूदा pair के लिए बताया जाता है:

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

`--list` और `--verify` directory को भी input के रूप में लेते हैं और उसमें सभी
files पर चलते हैं।

### PSD export (`--psd`)

```bash
procreepy --psd input/ out/
```

सिर्फ directory input। हर converted pair के पास `out/psd/NAME.psd` लिखा जाता
है, पूरे set के बाकी हिस्से के साथ atomically published होता है:

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

PSD में क्या है:

- layers का tree (groups, order), layer के नाम (Unicode), visibility,
  opacity, blend modes, bounds और lock स्थिति;
- हर layer के लिए 8-bit RGBA pixels (PackBits से compressed);
- DPI और built-in ICC profile;
- merged composite, जो Procreate के अपने flatten render से ऐसे ही लिया
  जाता है (अगर वह missing या damaged हो, तो visible layers को Normal mode
  में approximate करके combine किया जाता है)।

जो नहीं है — PSD एक export है, lossless round-trip नहीं:

- layer masks और exact clip-to-below semantics survive नहीं करतीं;
- text layers अपने pixels रखती हैं लेकिन editable text data नहीं;
- straight (non-premultiplied) alpha exactly recover नहीं हो सकता:
  Procreate premultiplied 8-bit tiles store करता है, इसलिये layer के
  किनारों पर fringe के रंग हल्का सा अलग हो सकते हैं;
- 30000 pixels से ज़्यादा चौड़ा या लंबा canvas outright reject किया
  जाता है (PSD format की सीमा, PSB की नहीं)।

`.procreate` master copy बना रहता है; PSD को Photoshop और दूसरे importers
के लिए एक snapshot मानिए।

### Diagnostics

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
हर segment को archive से सीधे parse किया जाता है (ZIP के अंदर CRC check सहित),
line-by-line report दिखाई जाती है, और जाँचा जाता है कि segments को बिना
re-encoding जोड़ा जा सकता है। **कोई output video नहीं बनाया जाता।** `--list`
सिर्फ ZIP directory पढ़ता है।

## विकल्प

| Option | क्या करता है |
|---|---|
| `-h`, `--help` | help दिखाएँ और बाहर निकलें |
| `--list` | segments को playback order में list करके बाहर निकलें |
| `--verify` | हर segment check करें; output video ना बनाएँ |
| `-r`, `--recursive` | directory input: sub-folders को भी traverse करें |
| `-f`, `--force` | directory input: पहले से मौजूद results को overwrite करें |
| `--strict` | missing segment numbers को error मानें (default: warning) |
| `--psd` | directory input: हर काम के लिए अतिरिक्त layers वाला PSD export करें |
| `--tmpdir DIR` | temporary files कहाँ रखें (default: `$TMPDIR`, फिर `/var/tmp`, फिर system का temp directory) |
| `-q`, `--quiet` | केवल warnings और errors दिखाएँ |
| `--version` | version number दिखाकर बाहर निकलें |
| `--` | option parsing रोकेँ; बाकी सब positional मानिए |

## यह कैसे काम करता है

1. `INPUT` एक file या stdin है। Stdin (और कोई भी non-seekable input) पहले
   temporary file में spool किया जाता है, क्योंकि ZIP को random access चाहिए।
2. ZIP को validate किया जाता है (read-only), और
   `video/segments/segment-N.mp4` entries खोजी जाती हैं।
3. Numeric sort। Numbering में gaps warning हैं; बिना number वाले names warning
   के साथ ignore किए जाते हैं।
4. हर segment को ZIP से सीधे parse किया जाता है (पूरी extraction के बिना): MP4
   boxes, track sizes और codec parameters। पहली corruption पर प्रक्रिया रुक जाती है।
5. Compatibility check (resolution, codec, SPS/PPS sets, audio)। वरना `-c copy`
   चुपचाप ख़राब data बना सकता है — इसलिये incompatibility स्पष्ट message के साथ
   error है, finished video में छिपी हुई समस्या नहीं।
6. moov-first MP4 assemble किया जाता है: `ftyp`, `moov` (सभी tracks, segments
   से काटे गए), फिर playback order में `mdat` के बाद `mdat`।
7. जो भी run promise करता है (MP4, slim project, PSD) — सब temporary files
   में stage होता है और आखिरी के successful होने के बाद ही एक single set
   के रूप में published होता है; batch mode में चाहे कुछ भी हो अगला
   input लिया जाता है।
8. Temporary file (अगर बनी हो) हमेशा हटाई जाती है — success, error, Ctrl+C
   और SIGTERM, सभी स्थितियों में।

### File और stdout में लिखना

दोनों रास्ते एक ही moov-first MP4 बनाते हैं: moov atom पहले लिखा जाता है,
क्योंकि frames source segments से सीधे copy होते हैं और metadata लिखना शुरू
करने से पहले ज्ञात होता है। File के लिए यह "classic" MP4 है, जो players और
editors दोनों के लिए उपयुक्त है; pipe में भी ठीक वही file जाती है —
`> artwork.mp4` का परिणाम explicit `procreepy artwork.procreate artwork.mp4`
के समान है।
  * **File output** atomic है: target के पास `.partial` file बनाई जाती है और
    केवल सफलता के बाद rename होती है। असफल run कोई अधूरा file नहीं छोड़ता और
    मौजूदा file को कभी ख़राब नहीं करता।
  * Windows क खास बात: आखिरी rename `MoveFileEx` से होता है, जो मौजूदा
    target file को जगह-पर ही replace कर देता है, इसलिये `--force` aur अधूरे
    set के दोबारा generation भी Unix की तरह ही मौजूदा outputs को जगह-पर
    बदल देता है। यह POSIX rename जितना strict atomic नहीं है; व्यवहारिक
    फर्क सिर्फ एक हालत में दिखता है — अगर target file किसी दूसरे program
    में अभी खुली हुई है (जैसे media player पिछला MP4 open करके पकड़े
    हुआ हो), तो rename एक पढ़ने योग्य `Access is denied` error के साथ
    reject हो जाता है, पुरानी file छुए-बिना वैसी ही रह जाती है, aur program
    बंद करके दोबारा run करने पर काम सफल हो जाता है।
  * stdout में कभी text नहीं मिलाया जाता। सभी log lines
    (`level=INFO`/`WARN`/`ERROR`, प्रति line एक structured key=value record)
    stderr पर जाती हैं। केवल `--list`/`--verify` report का अपवाद है, जिसमें
    stdout ही result है। अगर stdout terminal है, तो utility उसमें binary MP4
    लिखने से मना कर देती है।

### Console colors

अगर stderr interactive terminal है और environment variable `NO_COLOR` set
नहीं है, तो level tokens `WARN` और `ERROR` highlight होते हैं (पीला और
bold red); `INFO` plain रहता है। Pipes, redirects, CI और tests byte-for-byte
plain format ही रखते हैं, इसलिये किसी भी script पर असर नहीं। `--color`
flag जानबूझकर नहीं है।

### Temporary files और Fedora

Fedora पर `/tmp` RAM में tmpfs होता है। Timelapse segments सैकड़ों megabytes
 तक हो सकते हैं, और stdin से पढ़ते समय पूरा `.procreate` spool किया जाता है।
इसलिये temporary directory का क्रम है: `--tmpdir` → `$TMPDIR` → `/var/tmp`
(disk पर) → system default। Spooling के दौरान disk भर जाने पर केवल "No space
left" के बजाय एक स्पष्ट error और संकेत मिलता है (`--tmpdir` से किसी बड़ी disk-backed
directory का उपयोग करें)।

## Exit codes

| Code | अर्थ |
|---|---|
| 0 | सफलता |
| 1 | अप्रत्याशित error; batch mode में — कम से कम एक file विफल |
| 2 | ग़لط arguments (एक single file के साथ `--psd`, या पूरी results directory को stdout में भेजना, शामिल); OUTPUT INPUT जैसी ही file है; stdout terminal है |
| 3 | input नहीं मिला, खाली है या ZIP नहीं है |
| 4 | archive में `video/segments` नहीं है (timelapse रिकॉर्ड नहीं हुआ) |
| 5 | corrupt segment; ambiguous या missing numbering (`--strict`) |
| 6 | reserved (उपयोग नहीं होता: कोई external dependency नहीं) |
| 7 | segments stream copy के लिए incompatible हैं |
| 8 | reserved (उपयोग नहीं होता: कोई external dependency नहीं) |
| 9 | result या temporary files लिखने में failure |
| 130 | interrupted (Ctrl+C / SIGTERM) |

## Tests

```bash
make test          # or: go test ./...
```
असली `.procreate` files की ज़रूरत नहीं: tests generated MP4 segments से ZIP
बनाते हैं (देखें `internal/testkit`)। Checks structural हैं: resulting MP4 का
analysis, box order, sample counts और `mdat` content। `-race` आवश्यक नहीं है,
लेकिन C compiler installed होने पर चलता है।
Covered: ordinary file, missing `video/segments`, single segment, out-of-order
segments (`segment-9`/`segment-10`), stdin, stdout, names में spaces और special
characters, corrupt और truncated ZIPs, corrupt और truncated MP4s, CRC damage,
write errors (`/dev/full`), incompatible segments और पूरा batch mode।

## Utility जानबूझकर क्या नहीं करती

Timelapse का रास्ता `Document.archive` (NSKeyedArchive) को parse नहीं करता,
`*.lz4` को नहीं छूता, layers restore नहीं करता और image render नहीं
करता; `--psd` document parse करता है — ऊपर बताई गई export के लिए, उसी
section में बताई गई fidelity की सावधानियों के साथ। अगर file में timelapse
रिकॉर्ड नहीं हुआ था, तो यह utility drawing history से उसे recover नहीं
कर सकती। ध्यान दें: `.procreate` से निकाले गए `.lz4` पर `lz4 -t` चलाना
integrity check नहीं है — वे standalone LZ4 frames नहीं हैं।

## Format references

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## License

Apache License 2.0, `LICENSE` देखें।

---

## भाषाएँ

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
