# procreepy

Ein kleines plattformübergreifendes Dienstprogramm (Linux, Windows, macOS): Es
extrahiert den fertigen Timelapse aus einer `.procreate`-Datei und fügt dessen
Segmente zu einer einzigen MP4-Datei zusammen. Es wird nichts neu kodiert
(Stream Copy), und es wird nichts gerendert.

`.procreate` ist ein ZIP-Archiv. Wenn die Timelapse-Aufnahme aktiviert war,
enthält es

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
Das Tool nimmt genau diese Dateien: Es sortiert sie **numerisch**
(`segment-9` vor `segment-10`), analysiert die MP4-Struktur jedes Segments und
baut daraus eine einzige moov-first-MP4-Datei, wobei die Frames unverändert
kopiert werden. `Document.archive`, Ebenen und Raster-Chunks (`*.lz4`) werden
niemals geöffnet.

## Anforderungen

Keine externen Abhängigkeiten: weder `ffmpeg` noch `ffprobe` werden benötigt. Für den Build werden nur Go (Version in `go.mod`) und `make` benötigt (auf allen unterstützten Plattformen vorhanden; auf Minimal-Systemen über den Paketmanager installierbar).
Die Makefile ist der kanonische Einstiegspunkt für den Build: Sie fixiert dieselbe hermetische Umgebung wie CI (Offline-Modus für Module, lokale Toolchain, kein cgo) und findet die Go-Toolchain selbstständig.

```bash
make build      # alles kompilieren und ein ausführbares ./procreepy erzeugen
make check      # gofmt + Build + vet + vollständige Testsuite
```

`make build` versieht die Binärdatei mit `dev-<commit>`, sodass `procreepy --version` anzeigt, aus welchem Commit sie stammt (Release-Tarballs tragen stattdessen das Tag). Ohne `make` entspricht dies direkt `go build ./... && go build -o procreepy ./cmd/procreepy` (die Binärdatei meldet `dev` bzw. `dev-<commit>`, wenn sie innerhalb eines Git-Checkouts gebaut wurde).

### Für andere Betriebssysteme bauen

Das Projekt besteht aus reinem Go und lässt sich für alle unterstützten Ziele problemlos cross-kompilieren. Von jeder Plattform aus:

| Ziel | Befehl |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64-Bit (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32-Bit | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64-Bit | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
Jedes Ziel erzeugt ein normalisiertes `dist/procreepy-<version>-<os>-<arch>.tar.gz` und gibt dessen SHA-256 aus; `make cross` baut die gesamte Matrix auf einmal, und `make repro` weist die bitweise Reproduzierbarkeit eines Builds nach. Das direkte Äquivalent für ein Ziel ist `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.
Alle Builds sind statisch (ohne cgo): Ein Linux-Binary läuft auf jeder Distribution unabhängig von deren glibc-Version. Die CI-Pipelines (GitLab und GitHub) bauen bei jedem Commit genau diese Ziele; der Job `dist` veröffentlicht die Tarballs samt `SHA256SUMS`-Manifest, und die `repro:*`-Jobs weisen die bitweise Reproduzierbarkeit der Binärdateien nach.

- Linux/macOS: keine Installation erforderlich, Binärdatei direkt ausführen.
- Windows: Die Binärdatei ist nicht signiert, daher kann SmartScreen „Ihr PC wurde geschützt“ anzeigen — wählen Sie **Weitere Informationen → Trotzdem ausführen**.

## Verwendung

Einzelne Datei:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```
Alle vier Kombinationen `INPUT`/`OUTPUT` werden unterstützt:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Wird `OUTPUT` weggelassen, ist die
Ausgabe stdout. Ist `OUTPUT` ein vorhandenes Verzeichnis, wird das Video dort
unter seinem ursprünglichen Namen abgelegt (`procreepy art.procreate videos/`
→ `videos/art.mp4`).

### Batch-Modus: Ein Ordner mit `.procreate`-Dateien → ein Videoordner

Das Szenario „`input/` ist voller `.procreate`-Dateien und die Videos sollen in
`output/timelaps/`“:

```bash
procreepy input/
```

```text
input/                            output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate           └── (übersprungen, mit Warnung)
```
- **Namen**: `<ursprünglicher Name ohne .procreate>.mp4`. Leerzeichen,
  kyrillische Zeichen und Sonderzeichen bleiben unverändert.
- **Ausgabeordner**: standardmäßig `output/timelaps/` relativ zum aktuellen
  Verzeichnis, automatisch erstellt. Ein anderer kann als zweites Argument
  angegeben werden: `procreepy input/ ~/Videos/procreate`.
- **Erneutes Ausführen ist sicher**: bereits vorhandene Videos werden
  übersprungen. Alles neu aufbauen: `--force` (`-f`).
- **`-r`** durchläuft auch Unterverzeichnisse; deren Struktur wird im Ergebnis
  gespiegelt (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`),
  sodass identische Namen in verschiedenen Verzeichnissen nicht kollidieren.
- **Eine fehlerhafte Datei stoppt den Rest nicht.** Eine Datei ohne Timelapse
  (Aufnahme war deaktiviert) ist eine Warnung, kein Fehler. Eine beschädigte
  Datei ist ein Fehler: Sie erscheint in der abschließenden Zusammenfassung,
  und der Exit-Code wird `1`.
- Versteckte Dateien (`._Foo.procreate`, die macOS beim Kopieren hinterlässt)
  werden ignoriert.
- Originaldateien werden nie verändert.
Beispielausgabe (alles geht nach stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` und `--verify` akzeptieren ebenfalls ein Verzeichnis und durchlaufen
alle darin enthaltenen Dateien.

### Aufteilen: Video + schlankes Projekt (`--split`)

Die Idee: Timelapses benötigen mehr Speicherplatz als die Zeichnung selbst —
beispielsweise kann es beim Backup auf einem iPad sinnvoll sein, beides getrennt
zu halten. `--split` schreibt neben jede fertige `MP4` eine schlanke Projektkopie
**ohne** alles unter `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (der Timelapse, lossless)
                     →  artwork.procreepy.procreate  (dasselbe Projekt, ohne video/)
```
Im Batch-Modus gilt dasselbe: Neben jeder `X.mp4` erscheint eine
`X.procreepy.procreate`. Alle anderen Archivmitglieder (Ebenen, `Info.plist`,
Vorschaubilder) werden Byte für Byte übernommen: Reihenfolge,
Kompressionsmethoden und Zeitstempel bleiben erhalten. Die ursprüngliche
`.procreate`-Datei wird nicht verändert; die schlanke Kopie kann nicht nach
stdout geschrieben werden, daher benötigt `--split` ein Datei-`OUTPUT`.

### Diagnose

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
Jedes Segment wird direkt aus dem Archiv analysiert (einschließlich der
CRC-Prüfung innerhalb des ZIP), ein Bericht wird Zeile für Zeile ausgegeben,
und es wird geprüft, ob sich die Segmente ohne Neukodierung zusammenfügen
lassen. **Es wird kein Ausgabevideo erzeugt.** `--list` liest nur das ZIP-
Verzeichnis.

## Optionen

| Option | Funktion |
|---|---|
| `-r`, `--recursive` | Verzeichniseingabe: auch Unterverzeichnisse durchlaufen |
| `-f`, `--force` | Verzeichniseingabe: bereits vorhandene Videos überschreiben |
| `--strict` | fehlende Segmentnummern als Fehler behandeln (standardmäßig Warnung) |
| `--reencode` | aus Kompatibilitätsgründen für ältere Skripte akzeptiert; keine Neukodierung, immer Stream Copy |
| `--split` | neben jedem `MP4` eine `X.procreepy.procreate` schreiben — das Projekt ohne `video/` |
| `--tmpdir DIR` | Ort für temporäre Dateien |
| `-q`, `--quiet` | nur Warnungen und Fehler ausgeben |

## Funktionsweise

1. `INPUT` ist eine Datei oder stdin. Stdin (und jede nicht seekbare Eingabe)
   wird zunächst in eine temporäre Datei gespult, weil ZIP einen wahlfreien
   Zugriff benötigt.
2. Das ZIP wird validiert (nur lesend) und Einträge
   `video/segments/segment-N.mp4` werden gesucht.
3. Numerische Sortierung. Lücken in der Nummerierung sind eine Warnung; Namen
   ohne Nummer werden mit Warnung ignoriert.
4. Jedes Segment wird direkt aus dem ZIP analysiert (ohne vollständige
   Extraktion): MP4-Boxen, Track-Größen und Codec-Parameter. Beim ersten
   Fehler wird abgebrochen.
5. Kompatibilitätsprüfung (Auflösung, Codec, SPS/PPS-Sätze, Audio). Andernfalls
   würde `-c copy` stillschweigend unbrauchbare Daten erzeugen — deshalb ist
   eine Inkompatibilität ein Fehler mit klarer Meldung und keine Überraschung
   im fertigen Video.
6. Die moov-first-MP4-Datei wird zusammengesetzt: `ftyp`, `moov` (alle Tracks,
   aus den Segmenten ausgeschnitten), danach `mdat` hinter `mdat` in
   Wiedergabereihenfolge.
7. Die temporäre Datei (falls vorhanden) wird immer entfernt — bei Erfolg,
   Fehler, Ctrl+C und SIGTERM.

### Schreiben in eine Datei und nach stdout

Beide Wege erzeugen dieselbe moov-first-MP4-Datei: Das moov-Atom wird zuerst
geschrieben, weil die Frames direkt aus den Quellsegmenten kopiert werden und
die Metadaten bereits vor Beginn des Schreibens bekannt sind. Für eine Datei
ist das eine „klassische“ MP4, die sich für Player und Editoren gleichermaßen
eignet; genau dieselbe Datei geht in eine Pipe — `> artwork.mp4` liefert dasselbe
Ergebnis wie ein explizites `procreepy artwork.procreate artwork.mp4`.
  * **Dateiausgabe** ist atomar: Eine `.partial`-Datei wird neben dem Ziel angelegt
    und erst nach Erfolg umbenannt. Ein fehlgeschlagener Lauf hinterlässt keine
    Reste und beschädigt niemals eine vorhandene Datei.
  * stdout wird niemals mit Text vermischt. Alle Log-Zeilen
    (`level=INFO`/`WARN`/`ERROR`, ein strukturiertes key=value-Record pro Zeile)
    gehen nach stderr. Die einzige Ausnahme ist der `--list`/`--verify`-Bericht,
    bei dem stdout das Ergebnis ist. Ist stdout ein Terminal, verweigert das
    Tool die Ausgabe einer binären MP4-Datei dorthin.

### Temporäre Dateien und Fedora

Auf Fedora ist `/tmp` ein tmpfs im RAM. Timelapse-Segmente können Hunderte
Megabyte groß sein, und beim Lesen von stdin wird die gesamte `.procreate`
zwischengespeichert. Deshalb wird das temporäre Verzeichnis so ausgewählt:
`--tmpdir` → `$TMPDIR` → `/var/tmp` (auf der Platte) → das System-Standardverzeichnis.
Wenn der Datenträger beim Spoolen voll läuft, erhältst du einen klaren Fehler
mit einem Hinweis (verwende `--tmpdir` in einem größeren, datenträgerbasierten
Verzeichnis) statt eines bloßen „No space left“.

## Exit-Codes

| Code | Bedeutung |
|---|---|
| 0 | Erfolg |
| 1 | unerwarteter Fehler; im Batch-Modus — mindestens eine Datei ist fehlgeschlagen |
| 2 | falsche Argumente; Ausgabe würde Eingabe überschreiben; stdout ist ein Terminal |
| 3 | Eingabe nicht gefunden, leer oder kein ZIP |
| 4 | kein `video/segments` im Archiv (kein Timelapse wurde aufgezeichnet) |
| 5 | beschädigtes Segment; mehrdeutige oder fehlende Nummerierung (`--strict`) |
| 6 | reserviert (ungenutzt: keine externen Abhängigkeiten) |
| 7 | Segmente sind für Stream Copy inkompatibel |
| 8 | reserviert (ungenutzt: keine externen Abhängigkeiten) |
| 9 | Ergebnis oder temporäre Dateien konnten nicht geschrieben werden |
| 130 | unterbrochen (Ctrl+C / SIGTERM) |

## Tests

```bash
make test          # or: go test ./...
```
Echte `.procreate`-Dateien werden nicht benötigt: Die Tests erstellen ZIPs aus
erzeugten MP4-Segmenten (siehe `internal/testkit`). Die Prüfungen sind
strukturell: Analyse der resultierenden MP4, Box-Reihenfolge, Sample-Anzahl,
`mdat`-Inhalt. `-race` ist nicht erforderlich, funktioniert aber, wenn ein
C-Compiler installiert ist.
Abgedeckt sind: normale Datei, fehlendes `video/segments`, ein einzelnes Segment,
Segmente in falscher Reihenfolge (`segment-9`/`segment-10`), stdin, stdout,
Leerzeichen und Sonderzeichen in Dateinamen, beschädigte und abgeschnittene ZIPs,
beschädigte und abgeschnittene MP4s, CRC-Schäden, Schreibfehler (`/dev/full`),
inkompatible Segmente und der gesamte Batch-Modus.

## Was das Tool absichtlich nicht tut

Es analysiert `Document.archive` (NSKeyedArchive) nicht, berührt `*.lz4` nicht,
stellt keine Ebenen wieder her und rendert das Bild nicht. Wenn in der Datei
kein Timelapse aufgezeichnet wurde, kann dieses Tool ihn nicht aus der
Zeichenhistorie wiederherstellen. Hinweis: `lz4 -t` für eine aus einem
`.procreate` entnommene `.lz4`-Datei ist keine Integritätsprüfung — es handelt
sich nicht um eigenständige LZ4-Frames.

## Format-Referenzen

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Lizenz

Apache License 2.0, siehe `LICENSE`.

---

## Sprachen

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
