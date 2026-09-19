# procreepy

Ein kleines Unix-Tool für Linux: Entnimmt einer `.procreate`-Datei den
fertigen Archiv-Timelapse und fügt seine Segmente zu einer einzigen MP4
zusammen. Nichts wird neu kodiert (Stream Copy), nichts wird gerendert.

`.procreate` ist ein ZIP. Wenn die Timelapse-Aufnahme aktiviert war,
befindet sich darin

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

Das Tool nimmt genau diese Dateien: sortiert sie **numerisch**
(`segment-9` vor `segment-10`), parst die MP4-Struktur jedes Segments und
baut daraus einen moov-first MP4, wobei die Frames unverändert kopiert
werden. `Document.archive`, Ebenen und Raster-Chunks (`*.lz4`) werden gar
nicht geöffnet.

## Anforderungen

Keine externen Abhängigkeiten: weder `ffmpeg` noch `ffprobe` noch Python.
Zum Bauen wird nur Go benötigt (die Version steht in `go.mod`).

```bash
go build -o procreepy ./cmd/procreepy
```

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

Alle vier `INPUT`/`OUTPUT`-Kombinationen werden unterstützt: `FILE OUTPUT`,
`FILE -`, `- OUTPUT`, `- -`. Wenn `OUTPUT` weggelassen wird, ist es stdout.
Wenn `OUTPUT` ein vorhandenes Verzeichnis ist, liegt das Video dort unter
dem Originalnamen (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Batch-Modus: Ordner mit `.procreate` → Ordner mit Videos

Das Szenario „`input/` ist voller `.procreate`, und die Videos sollen nach
`output/timelaps/`":

```bash
procreepy input/
```

```text
input/                            output/timelaps/
├── Portrait of a Cat.procreate →  ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →  ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (übersprungen, mit Warnung)
```

- **Namen**: `<Originalname ohne .procreate>.mp4`. Leerzeichen, Kyrillisch
  und Sonderzeichen bleiben unverändert.
- **Zielordner**: standardmäßig `output/timelaps/` relativ zum aktuellen
  Verzeichnis, wird automatisch angelegt. Ein anderer kann als zweites
  Argument angegeben werden: `procreepy input/ ~/Videos/procreate`.
- **Neu ausführen ist sicher**: vorhandene Videos werden übersprungen. Zum
  vollständigen Neu-Bauen: `--force` (`-f`).
- **`-r`**: geht auch in Unterverzeichnisse; die Struktur wird im Ergebnis
  gespiegelt (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`),
  dadurch stoßen gleiche Namen in verschiedenen Ordnern nicht zusammen.
- **Eine kaputte Datei stoppt die anderen nicht.** Eine Datei ohne Timelapse
  (Aufnahme war aus) ist eine Warnung, kein Fehler. Eine defekte Datei ist
  ein Fehler: Sie landet in der Endauswertung, und der Exit-Code wird `1`.
- Versteckte Dateien (`._Foo.procreate`, die macOS beim Kopieren
  zurücklässt) werden ignoriert.
- Originale werden nie verändert.

Beispiel-Ausgabe (alles geht nach stderr):

```text
info: 4 .procreate file(s) in input -> output/timelaps/
info: [1/4] input/Landscape v2.procreate -> output/timelaps/Landscape v2.mp4
warning: [2/4] input/No Timelapse.procreate: no timelapse video inside, skipped
error: [3/4] input/Corrupt file.procreate: input is not a valid ZIP archive ...
info: [4/4] input/Portrait of a Cat.procreate -> output/timelaps/Portrait of a Cat.mp4
info: summary: 2 converted, 1 without timelapse, 1 FAILED
```

`--list` und `--verify` akzeptieren ebenfalls ein Verzeichnis und laufen
über alle Dateien.

### Aufteilen: Video + verschlanktes Projekt (`--split`)

Der Gedanke: Timelapses belegen mehr Platz als das Bild selbst — z. B.
möchte man sie bei einem iPad-Backup getrennt halten. `--split` schreibt
neben jede fertige `MP4` eine verschlankte Kopie des Projekts **ohne**
alles unter `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (der Timelapse, lossless)
                     →  artwork.procreepy.procreate  (gleiches Projekt, ohne video/)
```

Im Batch-Modus gleiches Spiel: neben jedem `X.mp4` erscheint ein
`X.procreepy.procreate`. Alle weiteren Archivmitglieder (Ebenen,
`Info.plist`, Vorschauen) werden Byte für Byte übernommen: Reihenfolge,
Kompressionsmethoden und Zeitstempel bleiben erhalten. Die originale
`.procreate` wird nicht verändert; die verschlankte Kopie kann nicht nach
stdout geschrieben werden, daher verlangt `--split` eine Datei-`OUTPUT`.

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

Parst jedes Segment direkt aus dem Archiv (inklusive CRC-Prüfung im ZIP),
druckt einen zeilenweisen Bericht und prüft, dass die Segmente ohne
Neukodierung verbunden werden können. **Es wird kein Ausgangs-Video
erstellt.** `--list` liest nur das ZIP-Verzeichnis.

## Optionen

| Option | Wirkung |
|---|---|
| `-r`, `--recursive` | Verzeichnis-Eingang: auch Unterverzeichnisse durchlaufen |
| `-f`, `--force` | Verzeichnis-Eingang: vorhandene Videos überschreiben |
| `--strict` | fehlende Segmentnummern als Fehler behandeln (standardmäßig Warnung) |
| `--reencode` | aus Kompatibilitätsgründen angenommen; es gibt keine Neukodierung, immer Stream Copy |
| `--split` | neben jeder `MP4` ein `X.procreepy.procreate` schreiben — das Projekt ohne `video/` |
| `--tmpdir DIR` | wohin die Segmente ausgepackt werden |
| `-q`, `--quiet` | nur Warnungen und Fehler ausgeben |

## So funktioniert es

1. `INPUT` ist eine Datei oder stdin. Stdin (und jeder nicht suchbare
   Eingang) wird zuerst in eine temporäre Datei gepumpt, weil ZIP
   Zufallszugriff erfordert.
2. Der ZIP wird validiert (nur lesen), und die Einträge
   `video/segments/segment-N.mp4` werden gesucht.
3. Numerische Sortierung. Lücken in der Nummerierung sind eine Warnung;
   Namen ohne Nummer werden mit Warnung ignoriert.
4. Jedes Segment wird direkt aus dem ZIP geparst (ohne vollständiges
   Auspacken): MP4-Boxen, Stromgrößen, Codec-Parameter. Bei der ersten
   Beschädigung — Stopp.
5. Kompatibilitätsprüfung (Auflösung, Codec, SPS/PPS-Sätze, Audio). Sonst
   würde `-c copy` still und leise Müll liefern — deshalb ist
   Inkompatibilität ein Fehler mit klarer Meldung, keine Überraschung im
   fertigen Video.
6. Der moov-first MP4 wird aufgebaut: `ftyp`, `moov` (alle Spuren, aus den
   Segmenten herausgeschnitten), dann `mdat` nach `mdat` in
   Wiedergabeordnung.
7. Die temporäre Datei (falls vorhanden) wird immer entfernt — bei Erfolg,
   bei Fehler, bei Strg+C und bei SIGTERM.

### Schreiben in Datei und stdout

Beide Wege bauen denselben moov-first MP4: Das moov-Atom wird zuerst
geschrieben, weil die Frames direkt aus den Quellsegmenten kopiert werden
und die Metadaten vor dem Schreiben bekannt sind. Für eine Datei ist das
ein „klassischer" MP4, geeignet für Player wie Editoren; dieselbe Datei
geht auch in die Pipe — `> artwork.mp4` liefert exakt dasselbe Ergebnis
wie ein ausdrückliches `procreepy artwork.procreate artwork.mp4`.

- **Datei**-Ausgabe ist atomar: eine `.partial`-Datei neben dem Ziel,
  Umbenennung erst nach Erfolg. Ein fehlgeschlagener Lauf hinterlässt keine
  Reste und beschädigt nie eine bestehende Datei.
- stdout wird nie mit Text verschmutzt. Alle `info:`/`warning:`/`error:`-
  Zeilen gehen nach stderr. Die einzige Ausnahme ist der
  `--list`/`--verify`-Bericht, wo stdout *das* Ergebnis ist. Wenn stdout
  ein Terminal ist, verweigert das Tool, dort einen binären MP4 abzulegen.

### Temporäre Dateien und Fedora

Unter Fedora ist `/tmp` ein tmpfs im RAM. Timelapse-Segmente können
hunderte Megabyte groß sein, und beim Lesen von stdin wird die gesamte
`.procreate` gepumpt. Deshalb wird das Temp-Verzeichnis so gewählt:
`--tmpdir` → `$TMPDIR` → `/var/tmp` (auf der Platte). Vor dem Auspacken
wird der freie Speicherplatz geprüft; wenn nicht genug da ist, gibt es
einen klaren Fehler mit Hinweis statt eines „No space left" mitten im
Lauf.

## Exit-Codes

| Code | Bedeutung |
|---|---|
| 0 | Erfolg |
| 1 | unerwarteter Fehler; im Batch-Modus — mindestens eine Datei fehlgeschlagen |
| 2 | falsche Argumente; Ausgabe würde Eingabe überschreiben; stdout ist ein Terminal |
| 3 | Eingang nicht gefunden, leer oder kein ZIP |
| 4 | keine `video/segments` im Archiv (kein Timelapse aufgenommen) |
| 5 | beschädigtes Segment; mehrdeutige oder fehlende Nummerierung (`--strict`) |
| 6 | reserviert (nicht genutzt: keine externen Abhängigkeiten) |
| 7 | Segmente sind für Stream Copy inkompatibel |
| 8 | reserviert (nicht genutzt: keine externen Abhängigkeiten) |
| 9 | Schreiben des Ergebnisses oder temporärer Dateien fehlgeschlagen |
| 130 | unterbrochen (Strg+C / SIGTERM) |

## Tests

```bash
go test ./...
```

Echte `.procreate`-Dateien braucht es nicht: Die Tests bauen ZIPs aus
erzeugten MP4-Segmenten (siehe `internal/testkit`). Die Prüfungen sind
strukturell: Parsen des entstandenen MP4, Box-Reihenfolge,
Stichprobenanzahl, Inhalt von `mdat`. `-race` ist nicht nötig, funktioniert
aber, wenn ein C-Compiler installiert ist.

Abgedeckt: normale Datei, fehlendes `video/segments`, ein einzelnes
Segment, Segmente in verrutschter Reihenfolge (`segment-9`/`segment-10`),
stdin, stdout, Leerzeichen und Sonderzeichen in Namen, defekte und
geschnittene ZIPs, defekte und geschnittene MP4s, CRC-Beschädigung,
Schreibfehler (`/dev/full`), inkompatible Segmente und der gesamte
Batch-Modus.

## Was das Tool bewusst nicht tut

Es parst `Document.archive` (NSKeyedArchive) nicht, rührt `*.lz4` nicht
an, stellt Ebenen nicht wieder her und rendert kein Bild. Wenn in der Datei
kein Timelapse aufgenommen wurde, kann dieses Tool ihn nicht aus der
Zeichnungshistorie zurückholen. Hinweis: `lz4 -t` auf einem aus der
`.procreate` entnommenen `.lz4` ist keine Integritätsprüfung — das sind
keine eigenständigen LZ4-Frames.

## Format-Referenzen

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Lizenz

Apache License 2.0, siehe `LICENSE`.

---

## Sprachen

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
