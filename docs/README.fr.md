# procreepy

Un petit utilitaire Unix multiplateforme (Linux, Windows, macOS) : extrait
le time-lapse d'archivage déjà prêt d'un fichier `.procreate` et assemble ses
segments en un seul MP4. Rien n'est réencodé (stream copy), rien n'est rendu.

`.procreate` est un ZIP. Si l'enregistrement du time-lapse était activé, il
contient

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

L'utilitaire prend exactement ces fichiers : il les trie **numériquement**
(`segment-9` avant `segment-10`), analyse la structure MP4 de chaque segment
et les reconstruit en un MP4 moov-first en copiant les images telles quelles.
Il n'ouvre jamais `Document.archive`, les calques ni les blocs raster
(`*.lz4`).

## Exigences

Aucune dépendance externe : pas de `ffmpeg`, pas de `ffprobe`.
Pour la compilation, seul Go est nécessaire (la version est dans `go.mod`).

```bash
go build -o procreepy ./cmd/procreepy    # or: make bin
```

### Compiler pour d'autres systèmes d'exploitation

Le projet est en Go pur et se croise proprement pour toutes les cibles prises
en charge. Depuis n'importe quelle plateforme, chacune de ces commandes
fonctionne :

| Cible | Commande |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64 bits (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32 bits | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64 bits | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |

Toutes les compilations sont statiques (sans cgo) : un binaire Linux
s'exécute sur n'importe quelle distribution, quelle que soit sa version de
glibc. La pipeline GitLab CI compile exactement ces cibles à chaque commit ;
le job `dist` publie les tarballs avec un manifeste `SHA256SUMS`, et les
jobs `repro:*` prouvent que les binaires sont reproductibles bit à bit.

- Linux/macOS : aucune étape d'installation, exécutez le binaire
  directement.
- Windows : le binaire n'est pas signé, SmartScreen peut donc afficher « Votre
  PC a été protégé » — choisissez **Plus d'informations → Exécuter quand
  même**.

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

Les quatre combinaisons `INPUT`/`OUTPUT` sont prises en charge : `FILE
OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. Si `OUTPUT` est omis, c'est stdout. Si
`OUTPUT` est un répertoire existant, la vidéo y est placée sous le nom
d'origine (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Mode par lots : un dossier de `.procreate` → un dossier de vidéos

Le scénario « j'ai `input/` plein de `.procreate` et je veux les vidéos dans
`output/timelaps/` » :

```bash
procreepy input/
```

```text
input/                               output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate            └── (ignoré, avec un avertissement)
```

- **Noms** : `<nom d'origine sans .procreate>.mp4`. Espaces, cyrillique et
  caractères spéciaux sont conservés tels quels.
- **Dossier de sortie** : par défaut `output/timelaps/` relativement au
  répertoire courant, créé automatiquement. Un autre peut être donné comme
  deuxième argument : `procreepy input/ ~/Videos/procreate`.
- **Relancer est sûr** : les vidéos qui existent déjà sont ignorées. Pour
  tout reconstruire : `--force` (`-f`).
- **`-r`** descend aussi dans les sous-dossiers ; la structure est reproduite
  dans le résultat (`input/2025/Cat.procreate` →
  `output/timelaps/2025/Cat.mp4`), ainsi les noms identiques dans différents
  dossiers ne se heurtent pas.
- **Un fichier défectueux n'arrête pas les autres.** Un fichier sans
  time-lapse (enregistrement désactivé) est un avertissement, pas une erreur.
  Un fichier corrompu est une erreur : il figure dans le résumé final et le
  code de sortie devient `1`.
- Les fichiers cachés (`._Foo.procreate`, laissés par macOS lors de la copie)
  sont ignorés.
- Les originaux ne sont jamais modifiés.

Sortie d'exemple (tout va vers stderr) :

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`--list` et `--verify` acceptent aussi un répertoire et parcourent tous les
fichiers.

### Découpage : vidéo + projet allégé (`--split`)

L'idée : les time-lapses prennent plus de place que le dessin lui-même — par
exemple, lors d'une sauvegarde sur iPad, on peut vouloir les garder séparés.
`--split` écrit, à côté de chaque `MP4` terminé, une copie allégée du projet
**sans** rien sous `video/` :

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (le time-lapse, lossless)
                     →  artwork.procreepy.procreate  (le même projet, sans video/)
```

En mode par lots, pareil : à côté de chaque `X.mp4` apparaît un
`X.procreepy.procreate`. Tous les autres membres de l'archive (calques,
`Info.plist`, aperçus) sont portés byte pour byte : l'ordre, les méthodes de
compression et les horodatages sont préservés. Le `.procreate` d'origine
n'est pas modifié ; la copie allégée ne peut pas être écrite sur stdout,
donc `--split` exige un `OUTPUT` fichier.

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

Analyse chaque segment directement dans l'archive (y compris la vérification
CRC à l'intérieur du ZIP), affiche un rapport ligne par ligne et contrôle que
les segments peuvent être joints sans réencodage. **Aucune vidéo de sortie
n'est créée.** `--list` ne lit que le répertoire du ZIP.

## Options

| Option | Rôle |
|---|---|
| `-r`, `--recursive` | entrée répertoire : descendre aussi dans les sous-dossiers |
| `-f`, `--force` | entrée répertoire : écraser les vidéos déjà présentes |
| `--strict` | traiter les numéros de segment manquants comme une erreur (avertissement par défaut) |
| `--reencode` | acceptée pour la compatibilité avec de vieux scripts ; aucun réencodage, toujours du stream copy |
| `--split` | écrire `X.procreepy.procreate` à côté de chaque `MP4` — le projet sans `video/` |
| `--tmpdir DIR` | où extraire les segments |
| `-q`, `--quiet` | n'afficher que les avertissements et erreurs |

## Fonctionnement

1. `INPUT` est un fichier ou stdin. Stdin (et toute entrée non positionnable)
   est d'abord transvasé dans un fichier temporaire, car le ZIP exige un
   accès aléatoire.
2. Le ZIP est validé (lecture seule), puis les entrées
   `video/segments/segment-N.mp4` sont repérées.
3. Tri numérique. Les trous de numérotation sont un avertissement ; les noms
   sans numéro sont ignorés avec un avertissement.
4. Chaque segment est analysé directement dans le ZIP (sans extraction
   complète) : boîtes MP4, tailles des pistes, paramètres du codec. Au premier
   dégât — arrêt.
5. Vérification de compatibilité (résolution, codec, ensembles SPS/PPS,
   audio). Sinon `-c copy` produirait du bruit en silence — l'incompatibilité
   est donc une erreur avec un message clair, pas une surprise dans la vidéo
   finie.
6. Le MP4 moov-first est assemblé : `ftyp`, `moov` (toutes les pistes,
   découpées dans les segments), puis `mdat` après `mdat` dans l'ordre de
   lecture.
7. Le fichier temporaire (s'il existe) est toujours supprimé — en cas de
   succès, d'erreur, de Ctrl+C et de SIGTERM.

### Écriture vers un fichier et vers stdout

Les deux chemins produisent le même MP4 moov-first : l'atome moov est écrit
en premier, car les images sont copiées directement depuis les segments
source et les métadonnées sont connues avant l'écriture. Pour un fichier,
c'est un MP4 « classique », utilisable autant par les lecteurs que les
éditeurs ; le fichier exact part aussi dans le tube — `> artwork.mp4` donne
le même résultat qu'un `procreepy artwork.procreate artwork.mp4` explicite.

- La sortie vers un **fichier** est atomique : un fichier `.partial` à côté
  de la cible, renommé seulement après succès. Un échec ne laisse pas de
  restes et ne corrompt jamais un fichier existant.
- stdout n'est jamais pollué par du texte. Tous les enregistrements
  `level=INFO`/`WARN`/`ERROR` (une ligne structurée key=value par enregistrement)
  vont vers stderr. L'unique exception est le
  rapport `--list`/`--verify`, où stdout *est* le résultat. Si stdout est un
  terminal, l'utilitaire refuse d'y déverser un MP4 binaire.

### Fichiers temporaires et Fedora

Sur Fedora, `/tmp` est un tmpfs en mémoire. Les segments d'un time-lapse
peuvent peser des centaines de Mo, et en lisant depuis stdin tout le
`.procreate` est transvasé. C'est pourquoi le répertoire temporaire est
choisi ainsi : `--tmpdir` → `$TMPDIR` → `/var/tmp` (sur disque). L'espace
libre est vérifié avant extraction ; s'il manque, vous obtenez une erreur
claire avec un indice au lieu d'un « No space left » en pleine tâche.

## Codes de sortie

| Code | Signification |
|---|---|
| 0 | succès |
| 1 | erreur inattendue ; en mode par lots — au moins un fichier a échoué |
| 2 | mauvais arguments ; la sortie écraserait l'entrée ; stdout est un terminal |
| 3 | entrée introuvable, vide ou non-ZIP |
| 4 | pas de `video/segments` dans l'archive (aucun time-lapse enregistré) |
| 5 | segment corrompu ; numérotation ambiguë ou manquante (`--strict`) |
| 6 | réservé (inutilisé : pas de dépendances externes) |
| 7 | segments incompatibles pour le stream copy |
| 8 | réservé (inutilisé : pas de dépendances externes) |
| 9 | échec de l'écriture du résultat ou des fichiers temporaires |
| 130 | interruption (Ctrl+C / SIGTERM) |

## Tests

```bash
go test ./...
```

Les vrais `.procreate` ne sont pas nécessaires : les tests construisent des
ZIP à partir de segments MP4 générés (voir `internal/testkit`). Les
vérifications sont structurelles : analyse du MP4 résultant, ordre des
boîtes, nombre d'échantillons, contenu de `mdat`. `-race` n'est pas
nécessaire mais fonctionne si un compilateur C est installé.

Couvert : le fichier ordinaire, l'absence de `video/segments`, un seul
segment, segments désordonnés (`segment-9`/`segment-10`), stdin, stdout,
espaces et caractères spéciaux dans les noms, ZIP corrompus et tronqués,
MP4 corrompus et tronqués, dégâts de CRC, erreurs d'écriture (`/dev/full`),
segments incompatibles, et tout le mode par lots.

## Ce que l'utilitaire ne fait pas volontairement

Il n'analyse pas `Document.archive` (NSKeyedArchive), ne touche pas les
`*.lz4`, ne restaure pas les calques et ne rend pas l'image. Si un
time-lapse n'a pas été enregistré dans le fichier, cet utilitaire ne peut
pas le récupérer depuis l'historique du dessin. Remarque : `lz4 -t` sur un
`.lz4` sorti d'un `.procreate` n'est pas une vérification d'intégrité — ce
ne sont pas des trames LZ4 autonomes.

## Références au format

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## Licence

Apache License 2.0, voir `LICENSE`.

---

## Langues

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
