# procreepy

Verwandelt die Zeitraffer-Aufnahme, die Procreate bereits in der
`.procreate`-Datei gespeichert hat, in ein normales MP4.

- Keine Neukodierung: die Bilder werden kopiert, die Qualität bleibt die von Procreate.
- Ihre `.procreate`-Originale werden nie verändert.
- Eine Datei oder ein ganzer Ordner auf einmal.
- Ein einzelnes Programm. Kein ffmpeg, kein Python, kein Konto, nichts einzurichten.
- Läuft offline unter Windows, macOS und Linux.

## Ist das etwas für Sie?

Nutzen Sie procreepy, wenn:

- Sie `.procreate`-Dateien haben;
- die **Zeitraffer-Aufnahme beim Zeichnen aktiv war** (in Procreate ist sie
  standardmäßig an);
- Sie diesen Zeitraffer als MP4 wollen — zum Hochladen, Schneiden oder Aufbewahren;
- oder Sie die Zeitraffer-Daten aus Ihren Projekten entfernen wollen, damit sie
  kleiner werden.

procreepy **kann nicht**:

- einen Zeitraffer erzeugen, der nie aufgenommen wurde — es extrahiert nur einen
  vorhandenen;
- einen Zeitraffer aus Ihren Ebenen oder dem Rückgängig-Verlauf rekonstruieren;
- eine beschädigte `.procreate`-Datei reparieren;
- ein fertiges Bild exportieren (ein Ebenen-PSD kann es exportieren, siehe
  [PSD exportieren](#psd-exportieren)).

Unsicher, ob Ihre Datei einen Zeitraffer enthält?
[Prüfen Sie es zuerst](#datei-vorab-prüfen) — ein Befehl, der nichts anlegt.

## Was dabei herauskommt

Eine Datei hinein, ein Video heraus:

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

Ein Ordner hinein, zwei Ordner heraus:

```text
input/                          output/
├── Cat.procreate         →     ├── timelapses/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── projects/
                                    ├── Cat.procreepy.procreate
                                    ├── Landscape.procreepy.procreate
                                    └── Sketch.procreepy.procreate
```

- `timelapses/` enthält die Videos.
- `projects/` enthält eine Kopie jeder Arbeit **ohne den Zeitraffer** — deutlich
  kleiner, und Sie können sie wieder in Procreate importieren. Alles andere im
  Projekt bleibt Byte für Byte erhalten.
- `input/` bleibt genau so, wie es war.

## Installation

Laden Sie ein fertiges Programm für Ihr System von der Releases-Seite — Go oder
Build-Werkzeuge brauchen Sie nicht.

- GitLab: https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub: https://github.com/po1nt-1/procreepy/releases

Wählen Sie die Datei, die zu Ihrem Rechner passt:

| Ihr System | Download |
|---|---|
| Windows (die meisten PCs) | `procreepy_<version>_windows_amd64.zip` |
| Windows auf ARM | `procreepy_<version>_windows_arm64.zip` |
| Mac mit Apple Silicon (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Mac mit Intel | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (die meisten PCs) | `procreepy_<version>_linux_amd64.tar.gz` |
| Linux auf ARM (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| Linux auf 32-Bit-ARM | `procreepy_<version>_linux_arm.tar.gz` |

Auf einem Mac sehen Sie unter Apple-Menü → „Über diesen Mac“, ob Sie Apple
Silicon oder Intel haben.

### Windows

1. Laden Sie `procreepy_<version>_windows_amd64.zip` herunter.
2. Rechtsklick auf die Datei → **Alle extrahieren** → einen Ordner wählen, den
   Sie wiederfinden, zum Beispiel `Downloads\procreepy`.
3. Diesen Ordner öffnen, oben in die Adressleiste klicken, `cmd` eingeben und
   Enter drücken. Es öffnet sich ein schwarzes Eingabeaufforderungs-Fenster in
   diesem Ordner.
4. Legen Sie eine `.procreate`-Datei in denselben Ordner und führen Sie aus:

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   Anführungszeichen sind nur bei Leerzeichen im Namen nötig.

**Zur SmartScreen-Warnung.** Das Programm ist nicht mit einem kostenpflichtigen
Microsoft-Zertifikat signiert, daher zeigt Windows beim ersten Start
möglicherweise ein blaues Fenster: „Der Computer wurde durch Windows geschützt“
mit einer „unbekannten App“. Das ist keine Virenmeldung — Windows zeigt sie bei
jedem Programm, das es noch nicht oft genug gesehen hat. Klicken Sie auf
**Weitere Informationen** und dann auf **Trotzdem ausführen**. Wenn Sie das nicht
möchten, nutzen Sie das [Container-Image](#docker--podman).

### macOS

1. Laden Sie das `.tar.gz` für Ihren Chip (`darwin_arm64` für Apple Silicon,
   `darwin_amd64` für Intel).
2. Terminal öffnen (Programme → Dienstprogramme → Terminal) und in den
   Downloads-Ordner wechseln:

   ```bash
   cd ~/Downloads
   ```

3. Entpacken und die Ausführung erlauben:

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   Die `xattr`-Zeile entfernt die Download-Quarantäne. Ohne sie verweigert macOS
   den Start, weil das Programm nicht von Apple notarisiert ist.

4. Eine Datei umwandeln:

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

Damit `procreepy` von überall funktioniert, verschieben Sie es in den PATH:
`sudo mv ./procreepy /usr/local/bin/`.

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

Das Programm ist statisch gelinkt und läuft daher auf jeder Distribution
unabhängig von deren glibc-Version. Für alle Benutzer installieren:
`sudo install -m 755 procreepy /usr/local/bin/`.

### Docker / Podman

Zu jedem Release wird ein Container-Image für `linux/amd64` und `linux/arm64`
veröffentlicht. Auf einem Mac mit Apple Silicon wird die arm64-Variante
automatisch gewählt.

```bash
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

`podman` ersetzt `docker` wortgleich. Eine Version pinnen: `:0.3.0` statt
`:latest` (Image-Tags tragen kein `v`-Präfix). Details zu Dateieigentum und
weiterem in [usage.md](usage.md#container-usage).

### Aus dem Quellcode bauen

Nur nötig, wenn Sie den Code ändern wollen. Siehe
[development.md](development.md).

### Download prüfen (optional)

Jedes Release veröffentlicht auch `CHECKSUMS.txt`. Um zu bestätigen, dass der
Download vollständig ist, geben Sie den Hash Ihrer Datei aus und vergleichen ihn
mit der passenden Zeile:

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # der erwartete Wert
```

Unter Windows: `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`.

Die beiden Werte müssen identisch sein. Der Schritt ist optional — er erkennt
einen abgebrochenen oder manipulierten Download, mehr nicht.

## Eine Datei umwandeln

```bash
procreepy artwork.procreate artwork.mp4
```

Ergebnis:

```text
artwork.mp4
```

Das Original `artwork.procreate` wird nicht verändert. Existiert `artwork.mp4`
schon, wird es ersetzt — aber erst, nachdem das neue Video vollständig
geschrieben wurde.

Sie können auch einen Ordner als Ziel angeben und procreepy den Namen wählen
lassen:

```bash
procreepy artwork.procreate videos/
```

Ergebnis: `videos/artwork.mp4`. Der Ordner muss bereits existieren.

## Einen Ordner umwandeln

```bash
procreepy input/
```

Liest jede `.procreate`-Datei in `input/` und schreibt nach `output/`, wie unter
[Was dabei herauskommt](#was-dabei-herauskommt) gezeigt. Um das Ziel selbst zu
wählen:

```bash
procreepy input/ ~/Videos/timelapses
```

Um Unterordner einzubeziehen (ihre Struktur wird in der Ausgabe gespiegelt):

```bash
procreepy -r input/ output/
```

Was während des Laufs passiert:

- Der Fortschritt wird pro Datei mit einer Zeile gemeldet.
- Eine Datei, deren Zeitraffer nie aufgenommen wurde, wird **mit einer Warnung
  übersprungen**. Für sie wird nichts geschrieben, der Lauf geht weiter.
- Eine beschädigte Datei wird als Fehler gemeldet, der Lauf macht mit den
  übrigen weiter, und der Befehl endet mit Exit-Code `1`, damit Skripte es
  merken.
- Denselben Befehl zweimal auszuführen wiederholt keine fertige Arbeit: Werke,
  deren Ergebnisse schon vorliegen, werden übersprungen. Mit `-f` werden sie
  trotzdem neu gebaut.

## Datei vorab prüfen

Beide Befehle erzeugen kein Video und ändern nichts.

**Ist ein Zeitraffer in dieser Datei, und wie lang ist er?**

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

`--list` liest das Inhaltsverzeichnis der Datei. Das geht sofort und sagt Ihnen,
ob überhaupt ein Zeitraffer vorhanden ist.

**Wird die Umwandlung wirklich funktionieren?**

```bash
procreepy --verify artwork.procreate
```

`--verify` geht weiter: es liest jedes Zeitraffer-Segment, prüft es auf Schäden
und bestätigt, dass die Segmente ohne Neukodierung verbunden werden können.
Langsamer als `--list` und die ehrliche Antwort auf „wird das sauber
umgewandelt?“.

Beide akzeptieren auch einen Ordner und berichten dann über jede Datei darin.

## PSD exportieren

```bash
procreepy --psd input/ output/
```

Neben jedem Video und schlanken Projekt wird `output/psd/NAME.psd` geschrieben:
eine Photoshop-Datei mit Ebenen, die sich in Photoshop, Affinity Photo, GIMP und
ähnlichen öffnen lässt.

`--psd` funktioniert **nur mit einem Ordner als Eingabe**. Bei einer einzelnen
Datei bricht es ab mit `--psd needs a directory INPUT; it writes into
OUTPUT/psd/`.

Das PSD ist ein Export, keine perfekte Kopie. Es behält den Ebenenbaum und die
Namen, Sichtbarkeit, Deckkraft, Füllmethoden und das Bild selbst; es behält
**nicht** Ebenenmasken, Schnittmasken-Beziehungen oder editierbaren Text. Lesen
Sie [was das PSD bewahrt und was es verliert](usage.md#export-a-psd), bevor Sie
es für fertige Arbeiten verwenden. Die `.procreate`-Datei bleibt Ihr Original.

## Was mit Ihren Dateien passiert

- **Ihre Originale werden nie verändert.** procreepy öffnet `.procreate`-Dateien
  nur lesend. Alles, was es erzeugt, wird woanders geschrieben.
- **Nichts bleibt halb geschrieben.** Jedes Ergebnis entsteht zuerst in einer
  temporären Datei und wird erst vollständig an seinen Platz gesetzt. Ein
  abgebrochener oder fehlgeschlagener Lauf hinterlässt kein defektes Video und
  beschädigt keine vorhandene Datei.
- **Ordner-Läufe veröffentlichen pro Werk als Satz.** Video, schlankes Projekt
  und PSD eines Werks erscheinen gemeinsam oder gar nicht — ein Video ohne sein
  Projekt bekommen Sie nie.
- **Erneutes Ausführen ist sicher.** Fertige Werke werden übersprungen. Ein Satz,
  der durch einen Abbruch unvollständig blieb, wird komplett neu gebaut. `-f`
  baut alles neu.
- **Eine einzelne Datei umzuwandeln ersetzt das Ziel**, falls es existiert —
  nachdem das neue Video vollständig geschrieben ist.
- **Große Dateien brauchen temporären Platz.** Große Zeitraffer werden über eine
  temporäre Datei zusammengesetzt. Wenn der Platz ausgeht, zeigen Sie mit
  `--tmpdir` auf ein größeres Laufwerk.

## Wenn etwas schiefgeht

| Was Sie sehen | Was es bedeutet |
|---|---|
| `procreepy: command not found` | Sie sind nicht im entpackten Ordner; nutzen Sie unter macOS/Linux `./procreepy`. |
| `no video/segments in the archive` | In dieser Datei wurde kein Zeitraffer aufgenommen. Er ist nicht wiederherstellbar. |
| `input is not a valid ZIP archive` | Keine `.procreate`-Datei, oder der Download bzw. die Kopie ist abgeschnitten. |
| `segment ... is corrupted inside the archive` | Die Zeitraffer-Daten sind beschädigt. |
| `segments are incompatible` | Der Zeitraffer entstand über einen Leinwand- oder Qualitätswechsel hinweg und lässt sich ohne Neukodierung nicht verbinden. |
| `refusing to write video data to a terminal` | Geben Sie einen Zieldateinamen an oder leiten Sie mit `> out.mp4` um. |
| `Der Computer wurde durch Windows geschützt` | Siehe [den SmartScreen-Hinweis](#windows). |
| `no space left` / Schreibfehler | Nutzen Sie `--tmpdir` auf einem Laufwerk mit mehr freiem Platz. |

Jeder dieser Fälle steht mit genauem Symptom und Vorgehen in
[troubleshooting.md](troubleshooting.md).

## Befehlsreferenz

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` ist eine `.procreate`-Datei, ein Ordner davon oder `-` für die
Standardeingabe. `OUTPUT` ist ein Dateiname, ein Ordner oder `-` für die
Standardausgabe. Bei einer einzelnen Datei bedeutet ein fehlendes `OUTPUT`, dass
das Video in die Standardausgabe geht; bei einem Ordner ist der Standard
`output/`.

| Option | Wirkung | Gilt für |
|---|---|---|
| `-h`, `--help` | Hilfe anzeigen und beenden | immer |
| `--version` | Version anzeigen und beenden | immer |
| `--list` | die Zeitraffer-Segmente auflisten; kein Video schreiben | Datei oder Ordner |
| `--verify` | jedes Segment prüfen; kein Video schreiben | Datei oder Ordner |
| `-r`, `--recursive` | auch Unterordner verarbeiten | nur Ordner-Eingabe |
| `-f`, `--force` | bereits vorhandene Ergebnisse überschreiben | nur Ordner-Eingabe |
| `--psd` | zusätzlich ein Ebenen-PSD pro Werk exportieren | nur Ordner-Eingabe |
| `--strict` | Lücken in der Segmentnummerierung als Fehler behandeln, nicht als Warnung | Datei oder Ordner |
| `--tmpdir DIR` | wohin temporäre Dateien gelegt werden | immer |
| `-q`, `--quiet` | nur Warnungen und Fehler ausgeben | immer |
| `--` | Optionsauswertung beenden; der Rest sind Dateinamen | immer |

Vollständige Referenz mit Beispielen, Ausgabeformaten und Exit-Codes:
[usage.md](usage.md).

## Wie es funktioniert

Eine `.procreate`-Datei ist ein ZIP-Archiv. Ist die Zeitraffer-Aufnahme aktiv,
speichert Procreate das fertige Video darin, aufgeteilt in nummerierte Teile
(`video/segments/segment-1.mp4`, `segment-2.mp4`, …). procreepy liest diese Teile
direkt aus dem Archiv, sortiert sie numerisch, prüft, dass sie Codec und
Leinwandgröße teilen, und fügt sie zu einem MP4 zusammen, indem es die Bilder
unverändert kopiert. Es wird nichts gerendert und nichts neu kodiert — darum ist
es schnell und verlustfrei.

Der Zeitraffer-Pfad sieht Ihre Ebenen nie an. Nur `--psd` liest das Werk selbst.

Details — MP4-Zusammenbau, atomares Schreiben, Strategie für temporäre Dateien,
PSD-Treue: [how-it-works.md](how-it-works.md).

## Entwicklung

Bauen, Tests, Coverage, Cross-Kompilierung, Release und CI:
[development.md](development.md).

Voraussetzungen sind Go (die Version steht in [`go.mod`](../go.mod)) und `make`.
Das Projekt hat keine Fremdabhängigkeiten.

```bash
make check   # Formatprüfung + Build + vet + die komplette Testsuite
```

## Lizenz

Apache License 2.0 — siehe [LICENSE](../LICENSE).

## Sprachen

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
