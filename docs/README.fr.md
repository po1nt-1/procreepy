# procreepy

Un petit utilitaire multiplateforme (Linux, Windows, macOS) : extrait le
time-lapse déjà présent dans un fichier `.procreate` et assemble ses segments
en un seul MP4. Rien n'est réencodé (stream copy), rien n'est rendu.

`.procreate` est une archive ZIP. Si l'enregistrement du time-lapse était
activé, elle contient

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```
L'utilitaire prend exactement ces fichiers : il les trie **numériquement**
(`segment-9` avant `segment-10`), analyse la structure MP4 de chaque segment
et les reconstruit en un seul MP4 moov-first en copiant les images telles quelles.
Il n'ouvre jamais `Document.archive`, les calques ni les blocs raster (`*.lz4`).

## Exigences

Aucune dépendance externe : ni `ffmpeg` ni `ffprobe` ne sont nécessaires. La compilation requiert seulement Go (version indiquée dans `go.mod`) et `make` (présent sur toutes les plateformes prises en charge ; sur les systèmes minimaux, il s'installe via le gestionnaire de paquets).
Le Makefile est le point d'entrée canonique de la compilation : il verrouille le même environnement hermétique que celui utilisé par la CI (mode hors ligne pour les modules, toolchain locale, sans cgo) et localise automatiquement la toolchain Go.

```bash
make build      # tout compiler et produire un ./procreepy exécutable
make check      # gofmt + compilation + vet + suite complète de tests
```

`make build` marque le binaire avec `dev-<commit>` afin que `procreepy --version` indique son origine (les tarballs de release portent plutôt le tag). Sans `make`, l'équivalent brut est `go build ./... && go build -o procreepy ./cmd/procreepy` (ce binaire indique `dev`, ou `dev-<commit>` lorsqu'il est compilé dans un checkout Git).

### Compiler pour d'autres systèmes d'exploitation

Le projet est écrit en Go pur et se compile proprement pour toutes les cibles prises en charge. Depuis n'importe quelle plateforme :

| Cible | Commande |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64 bits (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32 bits | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64 bits | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |
Chaque cible produit un `dist/procreepy-<version>-<os>-<arch>.tar.gz` normalisé et affiche son SHA-256 ; `make cross` construit toute la matrice en une fois et `make repro` prouve qu'une compilation est reproductible bit à bit. L'équivalent brut pour une cible est `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.
Toutes les compilations sont statiques (sans cgo) : un binaire Linux fonctionne sur n'importe quelle distribution, quelle que soit sa version de glibc. Les pipelines CI (GitLab et GitHub) construisent exactement ces cibles à chaque commit ; le job `dist` publie les tarballs avec un manifeste `SHA256SUMS`, et les jobs `repro:*` prouvent la reproductibilité bit à bit des binaires.

- Linux/macOS : aucune installation, exécutez directement le binaire.
- Windows : le binaire n'est pas signé ; SmartScreen peut donc afficher « Protection de votre ordinateur » — choisissez **Plus d'informations → Exécuter quand même**.

## Utilisation

Fichier unique :

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```
Les quatre combinaisons `INPUT`/`OUTPUT` sont prises en charge :
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Si `OUTPUT` est omis, la sortie est
stdout. Si `OUTPUT` est un répertoire existant, la vidéo y est placée sous son
nom d'origine (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Mode par lots : un dossier de `.procreate` → un dossier de vidéos

Le scénario « j'ai `input/` rempli de fichiers `.procreate` et je veux les vidéos
dans `output/timelaps/` » :

```bash
procreepy input/
```

```text
input/                               output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (ignoré, avec un avertissement)
```
- **Noms** : `<nom d'origine sans .procreate>.mp4`. Les espaces, les caractères
  cyrilliques et les caractères spéciaux sont conservés tels quels.
- **Dossier de sortie** : par défaut `output/timelaps/`, relatif au répertoire
  courant, créé automatiquement. Un autre peut être donné comme deuxième
  argument : `procreepy input/ ~/Videos/procreate`.
- **Relancer est sans danger** : les vidéos déjà présentes sont ignorées. Pour
  tout reconstruire : `--force` (`-f`).
- **`-r`** descend aussi dans les sous-dossiers ; leur structure est reproduite
  dans le résultat (`input/2025/Cat.procreate` → `output/timelaps/2025/Cat.mp4`),
  si bien que des noms identiques dans des dossiers différents n'entrent pas
  en collision.
- **Un fichier défectueux n'arrête pas les autres.** Un fichier sans time-lapse
  (enregistrement désactivé) est un avertissement, pas une erreur. Un fichier
  corrompu est une erreur : il figure dans le résumé final et le code de sortie
  devient `1`.
- Les fichiers cachés (`._Foo.procreate`, laissés par macOS lors de la copie)
  sont ignorés.
- Les fichiers d'origine ne sont jamais modifiés.
Exemple de sortie (tout est envoyé vers stderr) :

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```
`--list` et `--verify` acceptent aussi un répertoire et parcourent tous ses fichiers.

### Découpage : vidéo + projet allégé (`--split`)

L'idée : les time-lapses prennent plus de place que le dessin lui-même — par
exemple, lors d'une sauvegarde sur iPad, on peut vouloir les conserver séparément.
`--split` écrit, à côté de chaque `MP4` terminé, une copie allégée du projet
**sans** rien sous `video/` :

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (le time-lapse, lossless)
                     →  artwork.procreepy.procreate  (le même projet, sans video/)
```
En mode par lots, même principe : à côté de chaque `X.mp4` apparaît un
`X.procreepy.procreate`. Tous les autres éléments de l'archive (calques,
`Info.plist`, aperçus) sont conservés octet par octet : ordre, méthodes de
compression et horodatages compris. Le `.procreate` d'origine n'est pas modifié ;
la copie allégée ne peut pas être écrite sur stdout, donc `--split` exige un
fichier `OUTPUT`.

### Diagnostic

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
Analyse chaque segment directement depuis l'archive (y compris la vérification
CRC à l'intérieur du ZIP), affiche un rapport ligne par ligne et vérifie que les
segments peuvent être assemblés sans réencodage. **Aucune vidéo de sortie n'est
créée.** `--list` ne lit que le répertoire du ZIP.

## Options

| Option | Rôle |
|---|---|
| `-r`, `--recursive` | entrée sous forme de répertoire : parcourir aussi les sous-dossiers |
| `-f`, `--force` | entrée sous forme de répertoire : écraser les vidéos déjà présentes |
| `--strict` | traiter les numéros de segments manquants comme une erreur (avertissement par défaut) |
| `--reencode` | accepté pour la compatibilité avec les anciens scripts ; aucun réencodage, toujours du stream copy |
| `--split` | écrire `X.procreepy.procreate` à côté de chaque `MP4` — le projet sans `video/` |
| `--tmpdir DIR` | où placer les fichiers temporaires |
| `-q`, `--quiet` | n'afficher que les avertissements et les erreurs |

## Fonctionnement

1. `INPUT` est un fichier ou stdin. Stdin (et toute entrée non positionnable)
   est d'abord copié dans un fichier temporaire, car ZIP exige un accès aléatoire.
2. Le ZIP est validé (en lecture seule) et les entrées
   `video/segments/segment-N.mp4` sont localisées.
3. Tri numérique. Les trous dans la numérotation constituent un avertissement ;
   les noms sans numéro sont ignorés avec un avertissement.
4. Chaque segment est analysé directement depuis le ZIP (sans extraction complète) :
   boîtes MP4, tailles des pistes et paramètres du codec. À la première corruption,
   l'opération s'arrête.
5. Vérification de compatibilité (résolution, codec, ensembles SPS/PPS, audio).
   Sinon, `-c copy` produirait silencieusement des données invalides —
   l'incompatibilité est donc une erreur accompagnée d'un message clair, pas une
   surprise dans la vidéo finale.
6. Le MP4 moov-first est assemblé : `ftyp`, `moov` (toutes les pistes, extraites
   des segments), puis `mdat` après `mdat` dans l'ordre de lecture.
7. Le fichier temporaire, s'il existe, est toujours supprimé — en cas de succès,
   d'erreur, de Ctrl+C ou de SIGTERM.

### Écriture vers un fichier et vers stdout

Les deux chemins assemblent le même MP4 moov-first : l'atome moov est écrit en
premier, car les images sont copiées directement depuis les segments source et
les métadonnées sont connues avant l'écriture. Pour un fichier, c'est un MP4
« classique », adapté aussi bien aux lecteurs qu'aux éditeurs ; exactement le
même fichier part dans un tube — `> artwork.mp4` produit le même résultat qu'un
`procreepy artwork.procreate artwork.mp4` explicite.
  * **La sortie vers un fichier** est atomique : un `.partial` est créé à côté de
    la cible et renommé uniquement après réussite. Un échec ne laisse aucun reste
    et ne corrompt jamais un fichier existant.
  * stdout n'est jamais pollué par du texte. Toutes les lignes de journal
    (`level=INFO`/`WARN`/`ERROR`, un enregistrement structuré key=value par ligne)
    vont vers stderr. La seule exception est le rapport `--list`/`--verify`, où
    stdout est le résultat. Si stdout est un terminal, l'utilitaire refuse d'y
    écrire un MP4 binaire.

### Fichiers temporaires et Fedora

Sous Fedora, `/tmp` est un tmpfs en mémoire vive. Les segments de time-lapse
peuvent atteindre des centaines de mégaoctets et, lors d'une lecture depuis
stdin, l'intégralité du `.procreate` est copiée. Le répertoire temporaire est
donc choisi ainsi : `--tmpdir` → `$TMPDIR` → `/var/tmp` (sur disque) → le
répertoire temporaire système. Si le disque est plein pendant cette copie,
un message d'erreur clair avec une indication (utiliser `--tmpdir` sur un plus
gros répertoire sur disque) est fourni au lieu d'un simple « No space left ».

## Codes de sortie

| Code | Signification |
|---|---|
| 0 | succès |
| 1 | erreur inattendue ; en mode par lots — au moins un fichier a échoué |
| 2 | mauvais arguments ; la sortie écraserait l'entrée ; stdout est un terminal |
| 3 | entrée introuvable, vide ou non-ZIP |
| 4 | pas de `video/segments` dans l'archive (aucun time-lapse n'a été enregistré) |
| 5 | segment corrompu ; numérotation ambiguë ou manquante (`--strict`) |
| 6 | réservé (inutilisé : aucune dépendance externe) |
| 7 | segments incompatibles pour le stream copy |
| 8 | réservé (inutilisé : aucune dépendance externe) |
| 9 | échec d'écriture du résultat ou des fichiers temporaires |
| 130 | interrompu (Ctrl+C / SIGTERM) |

## Tests

```bash
make test          # or: go test ./...
```
Les vrais fichiers `.procreate` ne sont pas nécessaires : les tests construisent
des ZIP à partir de segments MP4 générés (voir `internal/testkit`). Les contrôles
sont structurels : analyse du MP4 produit, ordre des boîtes, nombre d'échantillons,
contenu de `mdat`. `-race` n'est pas nécessaire, mais fonctionne si un compilateur
C est installé.
Sont couverts : fichier ordinaire, absence de `video/segments`, segment unique,
segments dans le désordre (`segment-9`/`segment-10`), stdin, stdout, espaces et
caractères spéciaux dans les noms, ZIP corrompus et tronqués, MP4 corrompus et
tronqués, dommages CRC, erreurs d'écriture (`/dev/full`), segments incompatibles
et mode par lots complet.

## Ce que l'utilitaire ne fait délibérément pas

Il n'analyse pas `Document.archive` (NSKeyedArchive), ne touche pas à `*.lz4`,
ne restaure pas les calques et ne rend pas l'image. Si un time-lapse n'a pas
été enregistré dans le fichier, cet utilitaire ne peut pas le récupérer depuis
l'historique du dessin. Remarque : `lz4 -t` sur un `.lz4` extrait d'un `.procreate`
n'est pas un contrôle d'intégrité — il ne s'agit pas de trames LZ4 autonomes.

## Références du format

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licence

Licence Apache 2.0, voir `LICENSE`.

---

## Langues

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
