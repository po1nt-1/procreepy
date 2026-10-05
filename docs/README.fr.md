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
et les reconstruit en un seul MP4 moov-first en copiant les images telles
quelles. En mode par lots, il écrit aussi, pour chaque œuvre convertie, une
copie du projet réimportable sans le time-lapse, et — avec `--psd` — un PSD
Photoshop en calques. Le chemin du time-lapse n'ouvre jamais
`Document.archive`, les calques ni les blocs raster (`*.lz4`) ; seul `--psd`
le fait.

## Exigences

Aucune dépendance externe : ni `ffmpeg` ni `ffprobe` ne sont nécessaires. La compilation requiert seulement Go (version indiquée dans `go.mod`) et `make` (présent sur toutes les plateformes prises en charge ; sur les systèmes minimaux, il s'installe via le gestionnaire de paquets).
Le Makefile est le point d'entrée canonique de la compilation : il verrouille le même environnement hermétique que celui utilisé par la CI (mode hors ligne pour les modules, toolchain locale, sans cgo) et localise automatiquement la toolchain Go.

```bash
make build      # tout compiler et produire un ./procreepy exécutable
make check      # gofmt + compilation + vet + suite complète de tests
```

`make build` marque le binaire avec `dev-<short sha>` (hors checkout Git : `dev-nogit`) afin que `procreepy --version` indique son origine (les tarballs de release portent plutôt le tag). Sans `make`, l'équivalent brut est `go build ./... && go build -o procreepy ./cmd/procreepy` (ce binaire indique `dev`, ou `dev-<commit>` lorsque le stamping VCS est disponible).

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
Toutes les compilations sont statiques (sans cgo) : un binaire Linux fonctionne sur n'importe quelle distribution, quelle que soit sa version de glibc. Les pipelines CI (GitLab et GitHub) construisent exactement ces cibles à chaque commit et, Windows étant la plateforme principale, exécutent en outre la suite de tests sur le véritable binaire `windows/amd64` : en natif sur le runner hébergé de GitHub et sous Wine sur GitLab, qui ne dispose que de runners Linux (voir `ci/windows-wine/`) ; le job `dist` publie les tarballs avec un manifeste `SHA256SUMS`, et les jobs `repro:*` prouvent la reproductibilité bit à bit des binaires.

- Linux/macOS : aucune installation, exécutez directement le binaire.
- Windows : le binaire n'est pas signé ; SmartScreen peut donc afficher « Protection de votre ordinateur » — choisissez **Plus d'informations → Exécuter quand même**.

## Exécution dans un conteneur

Chaque tag de version publie également une image multi-architecture (`linux/amd64`, `linux/arm64`) dans le registre du projet, de sorte que la CLI s'exécute sans chaîne d'outils Go et sans décompresser d'archive. Cela couvre aussi Apple Silicon : Docker Desktop et `podman machine` lancent une machine virtuelle `linux/arm64` sur un Mac de la série M, donc la variante arm64 est choisie automatiquement — sans l'option `--platform` et sans émulation.

```bash
# un seul fichier — montez le répertoire courant et utilisez des chemins internes
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# mode par lots : un dossier en entrée, un dossier en sortie
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest input/ output/

# stdin → stdout ne nécessite aucun montage
docker run --rm -i registry.gitlab.com/po1nt-1/procreepy:latest - \
  < artwork.procreate > artwork.mp4
```

`podman` remplace `docker` à l'identique. Pour figer une version, utilisez `:1.2.3` au lieu de `:latest` (les tags d'image n'ont pas de préfixe `v`) ; `latest` n'est jamais déplacé sur un tag de préversion.

- **Propriété des fichiers.** L'image s'exécute sous l'uid 65532 (`distroless/static:nonroot`). Sur un hôte Linux, ajoutez `--user "$(id -u):$(id -g)"` pour que les fichiers de sortie vous appartiennent ; Docker Desktop sur macOS gère lui-même la propriété du montage et n'a besoin de rien.
- **Aucun shell à l'intérieur.** L'image ne contient que le binaire statique : `docker run … --help` ou `… --verify file.procreate` fonctionnent, mais il n'y a pas de `sh` dans laquelle entrer.
- **Les fichiers temporaires** vont dans la couche inscriptible du conteneur, pas dans le montage. Un gros timelapse peut y demander de la place ; `--tmpdir /data/tmp` les déplace sur le volume monté.
- **Projets privés.** L'accès au registre suit la visibilité du projet : un projet public se télécharge anonymement, sinon exécutez d'abord `docker login registry.gitlab.com`.

## Utilisation

Fichier unique :

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
```
Les quatre combinaisons `INPUT`/`OUTPUT` sont prises en charge :
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Si `OUTPUT` est omis, la sortie est
stdout. Si `OUTPUT` est un répertoire existant, la vidéo y est placée sous son
nom d'origine (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Mode par lots : un dossier de `.procreate` → des dossiers de vidéos et de projets

Le scénario « j'ai `input/` rempli de fichiers `.procreate` et je veux les
résultats dans `output/` » :

```bash
procreepy input/
```

Chaque œuvre convertie donne une paire — le time-lapse et un projet
réimportable sans lui :

```text
input/                               output/
├── Portrait of a Cat.procreate  →   ├── timelapses/Portrait of a Cat.mp4
│                                    ├── projects/Portrait of a Cat.procreepy.procreate
├── Landscape v2.procreate       →   ├── timelapses/Landscape v2.mp4
│                                    └── projects/Landscape v2.procreepy.procreate
└── No Timelapse.procreate              (ignoré avec un avertissement — rien écrit)
```
- **Noms** : `<nom d'origine sans .procreate>`, avec `.mp4` ou
  `.procreepy.procreate` ajouté. Les espaces, les caractères cyrilliques et les
  caractères spéciaux sont conservés tels quels ; sur un système de fichiers
  insensible à la casse, les noms qui ne diffèrent que par la casse reçoivent
  les suffixes `-2`, `-3`, …
- **Dossier de sortie** : par défaut `output/`, relatif au répertoire courant,
  créé automatiquement. Un autre peut être donné comme deuxième argument :
  `procreepy input/ ~/Videos/procreate`.
- **Le projet allégé** : `projects/NAME.procreepy.procreate` est la même
  archive sans les membres `video/segments/segment-N.mp4` ; tous les autres
  membres sont portés octet par octet (ordre, méthodes de compression,
  horodatages). Le projet conserve la date et l'heure de modification de sa
  source, de sorte que sa réimportation dans Procreate ne réordonne pas la
  galerie.
- **Relancer est sans danger** : une entrée dont le time-lapse et le projet
  existent déjà est ignorée ; une paire inachevée est régénérée en entier. Pour
  tout reconstruire : `--force` (`-f`).
- **`-r`** descend aussi dans les sous-dossiers ; leur structure est reproduite
  dans les deux arbres, si bien que des noms identiques dans des dossiers
  différents n'entrent pas en collision.
- **Un fichier défectueux n'arrête pas les autres.** Un fichier sans
  time-lapse (enregistrement désactivé) est un avertissement, pas une erreur :
  rien n'est écrit pour lui. Un fichier corrompu est une erreur : il figure
  dans le résumé final et le code de sortie devient `1`.
- **Ensembles atomiques** : les sorties d'une entrée (time-lapse + projet, plus
  le PSD avec `--psd`) sont écrites dans des fichiers provisoires et publiées
  ensemble, ou pas du tout. Une exécution ratée ne laisse jamais une vidéo
  orpheline à côté d'un projet manquant.
- Les fichiers cachés (`._Foo.procreate`, laissés par macOS lors de la copie)
  sont ignorés.
- Les fichiers d'origine ne sont jamais modifiés.

Exemple de sortie (tout est envoyé vers stderr ; l'exécution se termine avec
le code de sortie `1` à cause du fichier corrompu) :

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` est le nombre de fichiers de segments retirés du projet
allégé, et `video_size` leur taille totale (compressée) dans l'archive. Relancer
la même commande signale, pour chaque paire existante :

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

`--list` et `--verify` acceptent aussi un répertoire et parcourent tous ses fichiers.

### Export PSD (`--psd`)

```bash
procreepy --psd input/ out/
```

Entrée en répertoire uniquement. À côté de chaque paire convertie,
`out/psd/NAME.psd` est écrit, publié de façon atomique avec le reste de
l'ensemble :

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out-psd/psd/Portrait of a Cat.psd" layers=3
```

Ce que le PSD porte :

- l'arbre des calques (groupes, ordre), noms de calques (Unicode), visibilité,
  opacité, modes de fusion, limites et état de verrouillage ;
- des pixels RGBA 8 bits pour chaque calque, compressés en PackBits ;
- les DPI et le profil ICC incorporé ;
- un composite fusionné repris tel quel du propre aplatissement de Procreate
  (quand celui-ci manque ou est abîmé, les calques visibles sont composés en
  mode Normal en guise d'approximation).

Ce qu'il ne porte pas — le PSD est un export, pas une restitution sans perte :

- les masques de calques et la sémantique exacte du calage sur le calque
  inférieur ne survivent pas ;
- les calques de texte gardent leurs pixels mais pas leurs données de texte
  éditables ;
- l'alpha direct (non prémultiplié) ne peut pas être récupéré exactement :
  Procreate stocke des tuiles 8 bits prémultipliées, donc les couleurs de
  frange aux bords des calques peuvent différer légèrement ;
- les toiles plus larges ou plus hautes que 30000 pixels sont refusées
  d'emblée (limite du format PSD, par opposition à PSB).

Le projet `.procreate` reste la copie maîtresse ; considérez le PSD comme un
instantané pour Photoshop et autres importateurs.

### Diagnostic

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
Analyse chaque segment directement depuis l'archive (y compris la vérification
CRC à l'intérieur du ZIP), affiche un rapport ligne par ligne et vérifie que les
segments peuvent être assemblés sans réencodage. **Aucune vidéo de sortie n'est
créée.** `--list` ne lit que le répertoire du ZIP.

## Options

| Option | Rôle |
|---|---|
| `-h`, `--help` | afficher l'aide et quitter |
| `--list` | lister les segments dans l'ordre de lecture et quitter |
| `--verify` | vérifier chaque segment ; ne créer aucune vidéo de sortie |
| `-r`, `--recursive` | entrée sous forme de répertoire : parcourir aussi les sous-dossiers |
| `-f`, `--force` | entrée sous forme de répertoire : écraser les sorties déjà présentes |
| `--strict` | traiter les numéros de segments manquants comme une erreur (avertissement par défaut) |
| `--psd` | entrée sous forme de répertoire : exporter en outre un PSD en calques par œuvre |
| `--tmpdir DIR` | où placer les fichiers temporaires (par défaut : `$TMPDIR`, sinon `/var/tmp`, sinon le répertoire temporaire système) |
| `-q`, `--quiet` | n'afficher que les avertissements et les erreurs |
| `--version` | afficher le numéro de version et quitter |
| `--` | arrêter l'analyse des options ; traiter le reste comme positionnel |

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
7. Tout ce que l'exécution promet (le MP4, le projet allégé, le PSD) est
   préparé dans des fichiers provisoires et publié en un seul ensemble, et
   seulement après la réussite du dernier ; en mode par lots, l'entrée suivante
   est tentée quoi qu'il arrive.
8. Le fichier temporaire, s'il existe, est toujours supprimé — en cas de succès,
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
  * Spécificité Windows : le renommage final est un `MoveFileEx` avec
    remplacement de la cible existante ; `--force` et la régénération d'un
    ensemble incomplet remplacent donc les sorties existantes sur place,
    comme sur Unix. Il n'est pas strictement atomique comme le rename POSIX ;
    la différence pratique n'apparaît que dans un cas — si la cible est
    encore ouverte par un autre programme (par exemple un lecteur multimédia
    qui détient l'MP4 précédent), le renommage est refusé avec une erreur
    lisible `Access is denied`, l'ancien fichier reste intact et une nouvelle
    exécution après fermeture du programme réussit.
  * stdout n'est jamais pollué par du texte. Toutes les lignes de journal
    (`level=INFO`/`WARN`/`ERROR`, un enregistrement structuré key=value par ligne)
    vont vers stderr. La seule exception est le rapport `--list`/`--verify`, où
    stdout est le résultat. Si stdout est un terminal, l'utilitaire refuse d'y
    écrire un MP4 binaire.

### Couleurs de console

Quand stderr est un terminal interactif et que la variable d'environnement
`NO_COLOR` n'est pas définie, les jetons de niveau `WARN` et `ERROR` sont
colorés (jaune et rouge en gras) ; `INFO` reste sans couleur. Les tubes, les
redirections, la CI et les tests conservent le format plain octet par octet, de
sorte que rien de scripté ne change. Il n'existe délibérément pas d'option
`--color`.

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
| 2 | mauvais arguments (dont `--psd` avec un seul fichier, ou un dossier de résultats dirigé vers stdout) ; OUTPUT est le même fichier que INPUT ; stdout est un terminal |
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

Le chemin du time-lapse n'analyse pas `Document.archive` (NSKeyedArchive), ne
touche pas à `*.lz4`, ne restaure pas les calques et ne rend pas l'image ;
`--psd` analyse le document — pour l'export décrit plus haut, avec les limites
de fidélité décrites là. Si un time-lapse n'a pas été enregistré dans le
fichier, cet utilitaire ne peut pas le récupérer depuis l'historique du dessin.
Remarque : `lz4 -t` sur un `.lz4` extrait d'un `.procreate` n'est pas un
contrôle d'intégrité — il ne s'agit pas de trames LZ4 autonomes.

## Références du format

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licence

Licence Apache 2.0, voir `LICENSE`.

---

## Langues

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
