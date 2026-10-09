# procreepy

जो timelapse Procreate ने आपकी `.procreate` फ़ाइल के अंदर पहले से रिकॉर्ड कर रखा
है, उसे एक सामान्य MP4 में बदल देता है।

- कोई re-encoding नहीं: frames कॉपी होते हैं, इसलिए वीडियो में Procreate की ही गुणवत्ता रहती है।
- आपकी मूल `.procreate` फ़ाइलें कभी नहीं बदली जातीं।
- एक बार में एक फ़ाइल, या पूरा folder।
- बस एक प्रोग्राम फ़ाइल। न ffmpeg, न Python, न कोई account, न कोई सेटअप।
- Windows, macOS और Linux पर offline चलता है।

## क्या यह आपके काम का है?

procreepy इस्तेमाल करें, अगर:

- आपके पास `.procreate` फ़ाइलें हैं;
- ड्रॉइंग करते समय **Timelapse Recording चालू थी** (Procreate में यह डिफ़ॉल्ट रूप से
  चालू रहती है);
- आप वह timelapse एक MP4 के रूप में चाहते हैं — अपलोड करने, एडिट करने या सहेजने के लिए;
- या आप अपने projects से timelapse डेटा हटाकर उन्हें छोटा करना चाहते हैं।

procreepy **नहीं कर सकता**:

- वह timelapse बनाना जो कभी रिकॉर्ड ही नहीं हुआ — यह सिर्फ़ मौजूदा को निकालता है;
- आपकी layers या undo history से timelapse फिर से बनाना;
- क्षतिग्रस्त `.procreate` फ़ाइल को ठीक करना;
- आपकी कलाकृति की तैयार तस्वीर export करना (layered PSD export कर सकता है, देखें
  [PSD में export](#psd-में-export))।

पता नहीं आपकी फ़ाइल में timelapse है या नहीं?
[पहले जाँच लें](#बदलने-से-पहले-फ़ाइल-जाँचें) — एक ही command, और कुछ भी नहीं बनता।

## आपको क्या मिलेगा

एक फ़ाइल अंदर, एक वीडियो बाहर:

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

एक folder अंदर; बाहर एक वीडियो ट्री और एक project archive:

```text
input/                          input_procreepy/
├── Cat.procreate         →     ├── mp4/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── procreate.zip   (हल्के किए गए project)
```

- `mp4/` में वीडियो हैं।
- `procreate.zip` में हर कलाकृति की एक कॉपी है **जिसमें से timelapse हटा दिया गया है** —
  आकार में काफ़ी छोटी, और आप इसे Procreate में वापस import कर सकते हैं। project का
  बाकी सब कुछ byte-to-byte सुरक्षित रहता है। यह एक सामान्य zip है जिसके अंदर एक
  `procreate/` folder होता है, iPad पर ले जाने के लिए तैयार; `--no-zip` देने पर
  archive के बजाय वही `procreate/` folder डिस्क पर रहता है।
- output folder का नाम input के नाम पर रखा जाता है (`input/` → `input_procreepy/`)
  और जिस directory से आप command चलाते हैं उसी में बनता है। इसे खुद चुनने के लिए दूसरा
  path दें।
- हर output अपने स्रोत फ़ाइल की तारीख रखता है, इसलिए project को Procreate में फिर से
  import करने पर आपकी gallery का क्रम नहीं बदलता।
- `input/` ठीक वैसा ही रहता है जैसा था।

## इंस्टॉल

releases पेज से अपने सिस्टम के लिए तैयार प्रोग्राम डाउनलोड करें — Go या किसी build
tool की ज़रूरत नहीं।

- GitLab: https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub: https://github.com/po1nt-1/procreepy/releases

अपने कंप्यूटर के हिसाब से फ़ाइल चुनें:

| आपका सिस्टम | डाउनलोड |
|---|---|
| Windows (अधिकतर PC) | `procreepy_<version>_windows_amd64.zip` |
| ARM पर Windows | `procreepy_<version>_windows_arm64.zip` |
| Apple Silicon वाला Mac (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Intel वाला Mac | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (अधिकतर PC) | `procreepy_<version>_linux_amd64.tar.gz` |
| ARM पर Linux (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| 32-bit ARM पर Linux | `procreepy_<version>_linux_arm.tar.gz` |

Mac पर Apple मेनू → "About This Mac" से पता चलता है कि आपके पास Apple Silicon है
या Intel।

### Windows

1. `procreepy_<version>_windows_amd64.zip` डाउनलोड करें।
2. डाउनलोड की गई फ़ाइल पर राइट-क्लिक → **Extract All** → ऐसा folder चुनें जो आपको
   बाद में मिल जाए, जैसे `Downloads\procreepy`।
3. उस folder को खोलें, ऊपर address bar पर क्लिक करें, `cmd` टाइप करें और Enter
   दबाएँ। उसी folder में एक काली Command Prompt विंडो खुल जाएगी।
4. उसी folder में एक `.procreate` फ़ाइल रखें और चलाएँ:

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   Quotes की ज़रूरत सिर्फ़ तब है जब नाम में spaces हों।

**SmartScreen चेतावनी के बारे में।** यह प्रोग्राम Microsoft के किसी भुगतान वाले
certificate से signed नहीं है, इसलिए पहली बार चलाने पर Windows एक नीली विंडो दिखा
सकता है — "Windows protected your PC" — और उसे "unrecognized app" कह सकता है। यह
वायरस की रिपोर्ट नहीं है; Windows हर उस प्रोग्राम के लिए यह दिखाता है जिसे उसने
अभी पर्याप्त बार नहीं देखा। आगे बढ़ने के लिए **More info** पर क्लिक करें, फिर जो
**Run anyway** बटन दिखे उस पर। अगर आप यह नहीं करना चाहते, तो
[container image](#docker--podman) इस्तेमाल करें।

### macOS

1. अपने chip के हिसाब से `.tar.gz` डाउनलोड करें (Apple Silicon के लिए
   `darwin_arm64`, Intel के लिए `darwin_amd64`)।
2. Terminal खोलें (Applications → Utilities → Terminal) और Downloads folder में
   जाएँ:

   ```bash
   cd ~/Downloads
   ```

3. Extract करें और चलने की अनुमति दें:

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   `xattr` वाली पंक्ति download quarantine flag हटाती है। इसके बिना macOS प्रोग्राम
   शुरू करने से मना कर देता है, क्योंकि यह Apple से notarized नहीं है।

4. एक फ़ाइल बदलें:

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

कहीं से भी `procreepy` टाइप कर पाने के लिए इसे PATH में ले जाएँ:
`sudo mv ./procreepy /usr/local/bin/`।

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

प्रोग्राम statically linked है, इसलिए किसी भी distribution पर चलता है, glibc के
version से स्वतंत्र। सभी users के लिए install करने के लिए:
`sudo install -m 755 procreepy /usr/local/bin/`।

### Docker / Podman

हर release के लिए `linux/amd64` और `linux/arm64` का container image प्रकाशित होता
है। Apple Silicon वाले Mac पर arm64 variant अपने आप चुना जाता है।

```bash
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

`docker` की जगह `podman` बिना बदलाव चलेगा। version pin करने के लिए `:latest` की
जगह `:0.3.0` लिखें (image tags में `v` prefix नहीं होता)। फ़ाइलों के ownership और
बाकी विवरण [usage.md](usage.md#container-usage) में हैं।

### Source से build

ज़रूरत सिर्फ़ तब जब आप कोड बदलना चाहें। देखें [development.md](development.md)।

### डाउनलोड की जाँच (वैकल्पिक)

हर release के साथ `CHECKSUMS.txt` भी प्रकाशित होता है। डाउनलोड पूरा आया है या नहीं,
यह पक्का करने के लिए अपनी फ़ाइल का hash निकालें और उसी नाम वाली पंक्ति से मिलाएँ:

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # अपेक्षित मान
```

Windows पर: `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`।

दोनों मान एक जैसे होने चाहिए। यह कदम वैकल्पिक है — यह अधूरा या बदला हुआ download
पकड़ता है, इससे ज़्यादा कुछ नहीं।

## एक फ़ाइल बदलना

```bash
procreepy artwork.procreate artwork.mp4
```

परिणाम:

```text
artwork.mp4
```

मूल `artwork.procreate` नहीं बदलता। अगर `artwork.mp4` पहले से है तो वह बदल दिया
जाएगा, लेकिन सिर्फ़ तब जब नया वीडियो पूरा लिखा जा चुका हो।

आप एक folder को भी destination दे सकते हैं और नाम procreepy पर छोड़ सकते हैं:

```bash
procreepy artwork.procreate videos/
```

परिणाम: `videos/artwork.mp4`। वह folder पहले से मौजूद होना चाहिए।

## एक folder बदलना

```bash
procreepy input/
```

`input/` की हर `.procreate` फ़ाइल पढ़ता है और `input_procreepy/` में लिखता है, जैसा
[आपको क्या मिलेगा](#आपको-क्या-मिलेगा) में दिखाया गया है। destination खुद चुनने के
लिए:

```bash
procreepy input/ ~/Videos/timelapses
```

sub-folders शामिल करने के लिए (उनकी संरचना output में भी बनी रहती है):

```bash
procreepy -r input/ output/
```

चलते समय क्या होता है:

- प्रगति हर फ़ाइल के लिए एक पंक्ति में बताई जाती है।
- जिस फ़ाइल का timelapse कभी रिकॉर्ड नहीं हुआ, उसका हल्का project (और `--psd` के साथ
  उसका PSD) फिर भी बनता है — केवल वीडियो छोड़ा जाता है, एक सूचना के साथ, और काम चलता
  रहता है।
- क्षतिग्रस्त फ़ाइल error के रूप में दर्ज होती है, बाकी फ़ाइलों पर काम जारी रहता है,
  और command exit code `1` के साथ ख़त्म होती है ताकि scripts इसे पकड़ सकें।
- वही command दोबारा चलाने से पूरा हो चुका काम दोहराया नहीं जाता: जिन कलाकृतियों के
  परिणाम पहले से हैं, वे छोड़ दी जाती हैं। उन्हें फिर भी बनाना हो तो `-f` जोड़ें।
- बिना किसी विफलता वाले run के बाद हल्के project `procreate.zip` में pack हो जाते हैं
  और `procreate/` folder हटा दिया जाता है। जिस run में कोई भी विफलता हो, वह folder को
  बिना pack किए छोड़ देता है ताकि आप उसे जाँच सकें और काम आगे बढ़ा सकें। `--no-zip`
  हमेशा folder रखता है।

## बदलने से पहले फ़ाइल जाँचें

ये दोनों commands कोई वीडियो नहीं बनाते और कुछ नहीं बदलते।

**इस फ़ाइल में timelapse है क्या, और कितना लंबा?**

```bash
procreepy --list artwork.procreate
```

```text
input: artwork.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
```

`--list` फ़ाइल की सूची (table of contents) पढ़ता है। तुरंत चलता है और बताता है कि
timelapse है ही या नहीं।

**बदलना असल में काम करेगा या नहीं?**

```bash
procreepy --verify artwork.procreate
```

`--verify` आगे जाता है: timelapse के हर segment को पढ़ता है, क्षति जाँचता है, और
पक्का करता है कि segments को re-encode किए बिना जोड़ा जा सकता है। `--list` से धीमा,
और "यह साफ़-साफ़ convert होगा?" का ईमानदार जवाब।

दोनों folder भी स्वीकार करते हैं, और तब उसमें की हर फ़ाइल पर रिपोर्ट देते हैं।

## PSD में export

```bash
procreepy --psd input/ output/
```

हर वीडियो और हल्के project के साथ `output/psd/NAME.psd` भी लिखा जाता है: एक layered
Photoshop फ़ाइल, जो Photoshop, Affinity Photo, GIMP वगैरह में खुलती है।

`--psd` **सिर्फ़ folder input के साथ** काम करता है। एक अकेली फ़ाइल देने पर यह रुक
जाता है: `--psd needs a directory INPUT; it writes into OUTPUT/psd/`।

PSD एक export है, सटीक प्रतिलिपि नहीं। यह layer tree, group की संरचना और क्रम, नाम,
visibility, opacity, blend modes और ख़ुद छवि को रखता है; यह layer masks, clipping
संबंध और editable text को **नहीं** रखता। अंतिम काम के लिए इस्तेमाल करने से पहले
[PSD क्या रखता है और क्या खोता है](usage.md#export-a-psd) पढ़ें। master copy के रूप
में `.procreate` फ़ाइल ही रखें।

PSD में एक छोटी preview छवि भी embed रहती है, इसलिए इसे पढ़ने वाले apps (Photoshop,
Affinity, GIMP) एक thumbnail दिखाते हैं। पर यह अपने आप Windows Explorer से thumbnail
नहीं बनवाता: Explorer को `.psd` के लिए एक registered thumbnail handler चाहिए, जो
Windows साथ नहीं देता (Photoshop या SageThumbs जैसा pack उसे देता है), और
`.procreate` के लिए तो कोई है ही नहीं।

## आपकी फ़ाइलों का क्या होता है

- **आपकी मूल फ़ाइलें कभी नहीं बदलतीं।** procreepy `.procreate` फ़ाइलें read-only
  खोलता है। जो कुछ वह बनाता है, कहीं और लिखा जाता है।
- **कुछ भी अधूरा लिखा नहीं रहता।** हर परिणाम पहले एक अस्थायी फ़ाइल में बनता है और
  पूरा होने पर ही अपनी जगह रखा जाता है। बीच में रुका या असफल run कभी टूटा वीडियो
  नहीं छोड़ता और पहले से मौजूद फ़ाइल को नुकसान नहीं पहुँचाता।
- **Folder वाले run हर कलाकृति के लिए एक सेट के रूप में प्रकाशित होते हैं।** किसी एक
  कलाकृति का वीडियो, हल्का project और PSD साथ-साथ आते हैं या आते ही नहीं — आपको
  project के बिना वीडियो कभी नहीं मिलेगा।
- **तारीख़ें साथ चलती हैं।** हर output — वीडियो, हल्का project और PSD — पर उस
  `.procreate` की modification तारीख़ पड़ती है जिससे वह बना (और Windows पर creation
  तारीख़ भी), इसलिए Procreate में वापस import किया गया project gallery में अपनी जगह
  बनाए रखता है।
- **दोबारा चलाना सुरक्षित है।** पूरी हो चुकी कलाकृतियाँ छोड़ दी जाती हैं — चाहे project
  अब भी एक folder हों या `procreate.zip` में pack हो चुके हों। किसी रुके हुए run से
  अधूरा छूटा सेट पूरा दोबारा बनाया जाता है। `-f` सब कुछ दोबारा बनाता है।
- **एक फ़ाइल बदलने पर destination बदल दिया जाता है** (अगर मौजूद हो), नया वीडियो पूरा
  लिखने के बाद।
- **बड़ी फ़ाइलों को अस्थायी जगह चाहिए।** बड़े timelapse एक अस्थायी फ़ाइल के ज़रिये
  जोड़े जाते हैं। जगह कम पड़े तो `--tmpdir` को बड़ी disk पर इंगित करें।

## कुछ गड़बड़ हो जाए तो

| आप जो देखते हैं | इसका मतलब |
|---|---|
| `procreepy: command not found` | आप उस folder में नहीं हैं जहाँ extract किया था; macOS/Linux पर `./procreepy` चलाएँ। |
| `no video/segments in the archive` | उस फ़ाइल में timelapse रिकॉर्ड ही नहीं हुआ। इसे वापस नहीं लाया जा सकता। |
| `input is not a valid ZIP archive` | यह `.procreate` फ़ाइल नहीं है, या download/कॉपी अधूरी है। |
| `segment ... is corrupted inside the archive` | timelapse का डेटा क्षतिग्रस्त है। |
| `segments are incompatible` | timelapse canvas या quality बदलने के बीच रिकॉर्ड हुआ, इसलिए re-encode किए बिना जोड़ा नहीं जा सकता। |
| `refusing to write video data to a terminal` | destination फ़ाइल का नाम दें, या `> out.mp4` से redirect करें। |
| `Windows protected your PC` | देखें [SmartScreen टिप्पणी](#windows)। |
| `no space left` / लिखने की errors | `--tmpdir` को ऐसी disk पर इंगित करें जहाँ ज़्यादा जगह हो। |

इनमें से हर स्थिति, सटीक लक्षण और उपाय के साथ,
[troubleshooting.md](troubleshooting.md) में है।

## Command संदर्भ

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` एक `.procreate` फ़ाइल, उनका folder, या standard input के लिए `-` है।
`OUTPUT` एक फ़ाइल नाम, folder, या standard output के लिए `-` है। अकेली फ़ाइल के लिए
`OUTPUT` छोड़ देने का मतलब है वीडियो standard output पर जाएगा; folder के लिए
डिफ़ॉल्ट मौजूदा directory में `<INPUT>_procreepy/` है।

| विकल्प | क्या करता है | कहाँ लागू |
|---|---|---|
| `-h`, `--help` | मदद दिखाकर बाहर निकलें | हमेशा |
| `--version` | version दिखाकर बाहर निकलें | हमेशा |
| `--list` | timelapse segments सूचीबद्ध करें; वीडियो न लिखें | फ़ाइल या folder |
| `--verify` | हर segment जाँचें; वीडियो न लिखें | फ़ाइल या folder |
| `-r`, `--recursive` | sub-folders भी process करें | सिर्फ़ folder input |
| `-f`, `--force` | पहले से मौजूद परिणाम overwrite करें | सिर्फ़ folder input |
| `--psd` | हर कलाकृति का layered PSD भी export करें | सिर्फ़ folder input |
| `--no-zip` | project को `procreate.zip` में pack करने के बजाय `procreate/` folder रहने दें | सिर्फ़ folder input |
| `--strict` | segment numbering के अंतराल को चेतावनी नहीं, error मानें | फ़ाइल या folder |
| `--tmpdir DIR` | अस्थायी फ़ाइलें कहाँ रखें | हमेशा |
| `-q`, `--quiet` | सिर्फ़ चेतावनियाँ और errors छापें | हमेशा |
| `--` | options पढ़ना बंद करें; बाकी को फ़ाइल नाम मानें | हमेशा |

उदाहरणों, output formats और exit codes के साथ पूरा संदर्भ:
[usage.md](usage.md)।

## यह कैसे काम करता है

`.procreate` फ़ाइल एक ZIP archive है। जब timelapse recording चालू होती है, Procreate
तैयार वीडियो उसी के अंदर रखता है, क्रमांकित टुकड़ों में बँटा हुआ
(`video/segments/segment-1.mp4`, `segment-2.mp4`, …)। procreepy इन टुकड़ों को सीधे
archive से पढ़ता है, संख्या के क्रम में लगाता है, जाँचता है कि उनका codec और canvas
एक ही है, और frames को बिना छुए कॉपी करते हुए उन्हें एक MP4 में सी देता है। कुछ भी
render नहीं होता और कुछ भी re-encode नहीं होता — इसीलिए यह तेज़ और lossless है।

timelapse का रास्ता आपकी layers को कभी नहीं देखता। कलाकृति ख़ुद को सिर्फ़ `--psd`
पढ़ता है।

विवरण — MP4 assembly, atomic writes, अस्थायी फ़ाइलों की रणनीति, PSD की fidelity:
[how-it-works.md](how-it-works.md)।

## विकास

Build, tests, coverage, cross-compilation, release और CI:
[development.md](development.md)।

ज़रूरतें हैं Go (version [`go.mod`](../go.mod) में है) और `make`। project में कोई
third-party dependency नहीं है।

```bash
make check   # format जाँच + build + vet + पूरा test suite
```

## लाइसेंस

Apache License 2.0 — देखें [LICENSE](../LICENSE)।

## भाषाएँ

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
