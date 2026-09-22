# procreepy

एक छोटा क्रॉस-प्लैटफ़ॉर्म यूटिलिटी (Linux, Windows, macOS): यह `.procreate`
फ़ाइल से पहले से तैयार archive timelapse निकालती है और उसके segments को एक
MP4 में जोड़ देती है। कुछ भी re-encode नहीं किया जाता (stream copy), और कुछ
भी render नहीं किया जाता।

`.procreate` एक ZIP archive है। अगर timelapse recording चालू थी, तो इसमें
ये फ़ाइलें होती हैं

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
यूटिलिटी ठीक इन्हीं फ़ाइलों को लेती है: इन्हें **संख्यात्मक रूप से** sort करती
है (`segment-9`, `segment-10` से पहले), हर segment की MP4 संरचना parse करती है,
और frames को जस का तस कॉपी करके एक moov-first MP4 बनाती है। यह कभी भी
`Document.archive`, layers या raster chunks (`*.lz4`) नहीं खोलती।

## आवश्यकताएँ

कोई बाहरी dependency नहीं है: `ffmpeg` या `ffprobe` की ज़रूरत नहीं। Build के लिए केवल Go (version `go.mod` में है) और `make` चाहिए (यह सभी supported platforms पर उपलब्ध है; minimal systems पर package manager से आसानी से install किया जा सकता है)।
Makefile build का canonical entry point है: यह वही hermetic environment तय करता है जिसे CI इस्तेमाल करता है (offline module mode, local toolchain, no cgo) और Go toolchain को अपने-आप खोजता है।

```bash
make build      # सब कुछ compile करके runnable ./procreepy बनाना
make check      # gofmt + build + vet + पूरी test suite
```

`make build` binary में `dev-<commit>` दर्ज करता है, इसलिए `procreepy --version` बताता है कि binary किस commit से बनी है (release tarballs में इसकी जगह tag होता है)। `make` के बिना raw equivalent है `go build ./... && go build -o procreepy ./cmd/procreepy` (Git checkout के अंदर build करने पर binary `dev` या `dev-<commit>` रिपोर्ट करती है)।

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
सभी builds static हैं (no cgo): Linux binary किसी भी distribution पर चलेगी, glibc का version कुछ भी हो। CI pipelines (GitLab और GitHub) हर commit पर इन्हीं targets को build करती हैं; `dist` job tarballs और `SHA256SUMS` manifest publish करती है, और `repro:*` jobs binaries की bit-for-bit reproducibility साबित करती हैं।

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
procreepy --split artwork.procreate artwork.mp4
```
`INPUT`/`OUTPUT` की चारों combinations supported हैं:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`। अगर `OUTPUT` नहीं दिया गया है, तो
output stdout है। अगर `OUTPUT` पहले से मौजूद directory है, तो video वहाँ मूल
नाम से रखा जाता है (`procreepy art.procreate videos/` → `videos/art.mp4`)।

### Batch mode: `.procreate` फ़ाइलों वाला folder → videos वाला folder

Scenario: `input/` में बहुत-सी `.procreate` फ़ाइलें हैं और videos को
`output/timelaps/` में रखना है:

```bash
procreepy input/
```

```text
input/                           output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (छोड़ दिया गया, warning के साथ)
```
- **नाम**: `<मूल नाम बिना .procreate>.mp4`। spaces, Cyrillic और special
  characters जस के तस रहते हैं।
- **Output folder**: डिफ़ॉल्ट रूप से current directory के सापेक्ष
  `output/timelaps/`, जो अपने-आप बनता है। दूसरा folder दूसरे argument से दिया
  जा सकता है: `procreepy input/ ~/Videos/procreate`।
- **दोबारा चलाना सुरक्षित है**: जो videos पहले से मौजूद हैं वे skip हो जाते हैं।
  सब कुछ फिर से बनाने के लिए `--force` (`-f`) इस्तेमाल करें।
- **`-r`** sub-folders में भी जाता है; परिणाम में उनका structure mirror होता है
  (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`), इसलिए अलग
  folders में एक जैसे नाम आपस में नहीं टकराते।
- **एक खराब फ़ाइल बाकी फ़ाइलों को नहीं रोकती।** बिना timelapse वाली फ़ाइल
  (recording बंद थी) warning है, error नहीं। Corrupt फ़ाइल error है: वह final
  summary में आती है और exit code `1` हो जाता है।
- Hidden files (`._Foo.procreate`, जिन्हें macOS copy करते समय छोड़ता है) ignore होती हैं।
- Original files कभी बदली नहीं जातीं।
Example output (सारा output stderr पर जाता है):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` और `--verify` directory को भी input के रूप में लेते हैं और उसमें सभी
files पर चलते हैं।

### अलग करना: video + slim project (`--split`)

उद्देश्य: timelapses खुद drawing से ज़्यादा जगह लेते हैं — उदाहरण के लिए,
iPad पर backup करते समय इन्हें अलग रखना उपयोगी हो सकता है। `--split` हर तैयार
`MP4` के पास project की एक slim copy लिखता है, जिसमें `video/` के नीचे कुछ भी
नहीं होता:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (timelapse, lossless)
                     →  artwork.procreepy.procreate  (वही project, video/ के बिना)
```
Batch mode में भी यही होता है: हर `X.mp4` के पास `X.procreepy.procreate` आता है।
Archive के बाकी सभी members (layers, `Info.plist`, previews) byte for byte
कॉपी होते हैं: order, compression methods और timestamps सुरक्षित रहते हैं।
मूल `.procreate` नहीं बदली जाती; slim copy को stdout पर नहीं लिखा जा सकता,
इसलिए `--split` के लिए file `OUTPUT` चाहिए।

### Diagnostics

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
हर segment को archive से सीधे parse किया जाता है (ZIP के अंदर CRC check सहित),
line-by-line report दिखाई जाती है, और जाँचा जाता है कि segments को बिना
re-encoding जोड़ा जा सकता है। **कोई output video नहीं बनाया जाता।** `--list`
सिर्फ ZIP directory पढ़ता है।

## विकल्प

| Option | क्या करता है |
|---|---|
| `-r`, `--recursive` | directory input: sub-folders को भी traverse करें |
| `-f`, `--force` | directory input: पहले से मौजूद videos को overwrite करें |
| `--strict` | missing segment numbers को error मानें (default: warning) |
| `--reencode` | पुराने scripts के साथ compatibility के लिए स्वीकार किया जाता है; re-encoding नहीं होती, हमेशा stream copy |
| `--split` | हर `MP4` के पास `X.procreepy.procreate` लिखें — `video/` के बिना project |
| `--tmpdir DIR` | temporary files कहाँ रखें |
| `-q`, `--quiet` | केवल warnings और errors दिखाएँ |

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
   चुपचाप खराब data बना सकता है — इसलिए incompatibility स्पष्ट message के साथ
   error है, finished video में छिपी हुई समस्या नहीं।
6. moov-first MP4 assemble किया जाता है: `ftyp`, `moov` (सभी tracks, segments
   से काटे गए), फिर playback order में `mdat` के बाद `mdat`।
7. Temporary file (अगर बनी हो) हमेशा हटाई जाती है — success, error, Ctrl+C और
   SIGTERM, सभी स्थितियों में।

### File और stdout में लिखना

दोनों रास्ते एक ही moov-first MP4 बनाते हैं: moov atom पहले लिखा जाता है,
क्योंकि frames source segments से सीधे copy होते हैं और metadata लिखना शुरू
करने से पहले ज्ञात होता है। File के लिए यह "classic" MP4 है, जो players और
editors दोनों के लिए उपयुक्त है; pipe में भी ठीक वही file जाती है —
`> artwork.mp4` का परिणाम explicit `procreepy artwork.procreate artwork.mp4`
के समान है।
  * **File output** atomic है: target के पास `.partial` file बनाई जाती है और
    केवल सफलता के बाद rename होती है। असफल run कोई अधूरा file नहीं छोड़ता और
    मौजूदा file को कभी खराब नहीं करता।
  * stdout में कभी text नहीं मिलाया जाता। सभी log lines
    (`level=INFO`/`WARN`/`ERROR`, प्रति line एक structured key=value record)
    stderr पर जाती हैं। केवल `--list`/`--verify` report का अपवाद है, जिसमें
    stdout ही result है। अगर stdout terminal है, तो utility उसमें binary MP4
    लिखने से मना कर देती है।

### Temporary files और Fedora

Fedora पर `/tmp` RAM में tmpfs होता है। Timelapse segments सैकड़ों megabytes
tक हो सकते हैं, और stdin से पढ़ते समय पूरा `.procreate` spool किया जाता है।
इसलिए temporary directory का क्रम है: `--tmpdir` → `$TMPDIR` → `/var/tmp`
(disk पर) → system default। Spooling के दौरान disk भर जाने पर केवल "No space
left" के बजाय एक स्पष्ट error और संकेत मिलता है (`--tmpdir` से किसी बड़ी disk-backed
directory का उपयोग करें)।

## Exit codes

| Code | अर्थ |
|---|---|
| 0 | सफलता |
| 1 | अप्रत्याशित error; batch mode में — कम से कम एक file विफल |
| 2 | गलत arguments; output input को overwrite करेगा; stdout terminal है |
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

यह `Document.archive` (NSKeyedArchive) को parse नहीं करती, `*.lz4` को नहीं
छूती, layers restore नहीं करती और image render नहीं करती। अगर file में
timelapse रिकॉर्ड नहीं हुआ था, तो यह utility drawing history से उसे recover
नहीं कर सकती। ध्यान दें: `.procreate` से निकाले गए `.lz4` पर `lz4 -t` चलाना
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
