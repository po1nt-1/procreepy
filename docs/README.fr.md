# procreepy

Transforme en MP4 ordinaire le timelapse que Procreate a déjà enregistré dans
votre fichier `.procreate`.

- Sans réencodage : les images sont copiées, la vidéo garde la qualité de Procreate.
- Vos fichiers `.procreate` d'origine ne sont jamais modifiés.
- Un fichier ou un dossier entier à la fois.
- Un seul programme. Pas de ffmpeg, pas de Python, pas de compte, rien à configurer.
- Fonctionne hors ligne sur Windows, macOS et Linux.

## Est-ce pour vous ?

Utilisez procreepy si :

- vous avez des fichiers `.procreate` ;
- l'**enregistrement du timelapse était actif** pendant que vous dessiniez (il
  l'est par défaut dans Procreate) ;
- vous voulez ce timelapse en MP4, pour le publier, le monter ou le conserver ;
- ou vous voulez retirer les données du timelapse de vos projets pour les alléger.

procreepy **ne peut pas** :

- créer un timelapse qui n'a jamais été enregistré — il ne fait qu'extraire un
  timelapse existant ;
- reconstruire un timelapse à partir de vos calques ou de l'historique d'annulation ;
- réparer un fichier `.procreate` endommagé ;
- exporter une image finale de votre œuvre (il peut exporter un PSD en calques,
  voir [Exporter en PSD](#exporter-en-psd)).

Vous ne savez pas si votre fichier contient un timelapse ?
[Vérifiez-le d'abord](#vérifier-un-fichier-avant-conversion) : une commande, qui
ne crée rien.

## Ce que vous obtenez

Un fichier en entrée, une vidéo en sortie :

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

Un dossier en entrée ; en sortie, une arborescence de vidéos et une archive de projets :

```text
input/                          input_procreepy/
├── Cat.procreate         →     ├── mp4/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── procreate.zip   (les projets allégés)
```

- `mp4/` contient les vidéos.
- `procreate.zip` contient une copie de chaque œuvre **sans le timelapse** :
  beaucoup plus légère, et réimportable dans Procreate. Tout le reste du projet
  est conservé octet pour octet. C'est un zip ordinaire contenant un dossier
  `procreate/`, prêt à être transféré sur un iPad ; l'option `--no-zip` laisse ce
  dossier `procreate/` sur le disque au lieu de l'archive.
- Le dossier de sortie est nommé d'après celui d'entrée (`input/` →
  `input_procreepy/`) et créé dans le répertoire d'où vous lancez la commande.
  Donnez un second chemin pour le choisir vous-même.
- Chaque sortie conserve la date du fichier dont elle provient, si bien que
  réimporter un projet dans Procreate ne réorganise pas votre galerie.
- `input/` reste exactement tel qu'il était.

## Installation

Téléchargez un programme prêt à l'emploi pour votre système depuis la page des
releases — ni Go ni outils de compilation ne sont nécessaires.

- GitLab : https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub : https://github.com/po1nt-1/procreepy/releases

Choisissez le fichier correspondant à votre ordinateur :

| Votre système | Téléchargement |
|---|---|
| Windows (la plupart des PC) | `procreepy_<version>_windows_amd64.zip` |
| Windows sur ARM | `procreepy_<version>_windows_arm64.zip` |
| Mac Apple Silicon (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Mac Intel | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (la plupart des PC) | `procreepy_<version>_linux_amd64.tar.gz` |
| Linux sur ARM (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| Linux sur ARM 32 bits | `procreepy_<version>_linux_arm.tar.gz` |

Sur un Mac, menu Pomme → « À propos de ce Mac » indique si vous avez Apple
Silicon ou Intel.

### Windows

1. Téléchargez `procreepy_<version>_windows_amd64.zip`.
2. Clic droit sur le fichier téléchargé → **Extraire tout** → choisissez un
   dossier que vous retrouverez, par exemple `Downloads\procreepy`.
3. Ouvrez ce dossier, cliquez dans la barre d'adresse en haut, tapez `cmd` et
   appuyez sur Entrée. Une fenêtre noire d'invite de commandes s'ouvre dans ce
   dossier.
4. Placez un fichier `.procreate` dans le même dossier et lancez :

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   Les guillemets ne servent que si le nom contient des espaces.

**À propos de l'avertissement SmartScreen.** Le programme n'est pas signé avec
un certificat Microsoft payant, donc au premier lancement Windows peut afficher
une fenêtre bleue : « Windows a protégé votre ordinateur », évoquant une
« application non reconnue ». Ce n'est pas une alerte antivirus : Windows
l'affiche pour tout programme qu'il n'a pas encore vu assez souvent. Pour
continuer, cliquez sur **Informations complémentaires**, puis sur le bouton
**Exécuter quand même**. Si vous préférez l'éviter, utilisez
l'[image de conteneur](#docker--podman).

### macOS

1. Téléchargez le `.tar.gz` correspondant à votre puce (`darwin_arm64` pour
   Apple Silicon, `darwin_amd64` pour Intel).
2. Ouvrez le Terminal (Applications → Utilitaires → Terminal) et allez dans vos
   téléchargements :

   ```bash
   cd ~/Downloads
   ```

3. Décompressez et autorisez l'exécution :

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   La ligne `xattr` retire l'attribut de quarantaine des téléchargements. Sans
   elle, macOS refuse de lancer le programme, car il n'est pas notarisé par Apple.

4. Convertissez un fichier :

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

Pour pouvoir taper `procreepy` depuis n'importe où, déplacez-le dans le PATH :
`sudo mv ./procreepy /usr/local/bin/`.

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

Le programme est lié statiquement : il fonctionne sur n'importe quelle
distribution, quelle que soit sa version de glibc. Pour l'installer pour tous les
utilisateurs : `sudo install -m 755 procreepy /usr/local/bin/`.

### Docker / Podman

Une image de conteneur est publiée à chaque release, pour `linux/amd64` et
`linux/arm64`. Sur un Mac Apple Silicon, la variante arm64 est choisie
automatiquement.

```bash
# Docker
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# Podman (rootless, Linux) : mappez l'utilisateur, réétiquetez avec :Z
podman run --rm --userns=keep-id --user "$(id -u):$(id -g)" \
  -v "$PWD":/data:Z -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

Les deux moteurs ne sont pas interchangeables option pour option : Podman
rootless exige `--userns=keep-id`, `--user` et — sur un hôte SELinux — un
montage en `:Z`, sans quoi l'exécution échoue sur les droits du répertoire de
sortie. Pour figer une version, utilisez `:0.3.0` au lieu de `:latest` (les tags
d'image n'ont pas de préfixe `v`). Détails sur la propriété des fichiers,
SELinux et le reste dans [usage.md](usage.md#container-usage).

### Compiler depuis les sources

Utile seulement si vous voulez modifier le code. Voir
[development.md](development.md).

### Vérifier le téléchargement (facultatif)

Chaque release publie aussi `CHECKSUMS.txt`. Pour confirmer que le téléchargement
est intact, affichez l'empreinte de votre fichier et comparez-la à la ligne
correspondante :

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # la valeur attendue
```

Sous Windows : `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`.

Les deux valeurs doivent être identiques. Étape facultative : elle détecte un
téléchargement tronqué ou altéré, rien de plus.

## Convertir un fichier

```bash
procreepy artwork.procreate artwork.mp4
```

Résultat :

```text
artwork.mp4
```

L'original `artwork.procreate` n'est pas modifié. Si `artwork.mp4` existe déjà,
il est remplacé, mais seulement après que la nouvelle vidéo a été écrite en
entier.

Vous pouvez aussi indiquer un dossier comme destination et laisser procreepy
nommer le fichier :

```bash
procreepy artwork.procreate videos/
```

Résultat : `videos/artwork.mp4`. Le dossier doit déjà exister.

## Convertir un dossier

```bash
procreepy input/
```

Lit chaque `.procreate` de `input/` et écrit dans `input_procreepy/`, comme
montré dans [Ce que vous obtenez](#ce-que-vous-obtenez). Pour choisir vous-même la
destination :

```bash
procreepy input/ ~/Videos/timelapses
```

Pour inclure les sous-dossiers (leur structure est reproduite en sortie) :

```bash
procreepy -r input/ output/
```

Ce qui se passe pendant l'exécution :

- La progression est signalée fichier par fichier, une ligne chacun.
- Un fichier dont le timelapse n'a jamais été enregistré produit tout de même son
  projet allégé (et son PSD avec `--psd`) — seule la vidéo est ignorée, avec une
  mention, et l'exécution continue.
- Un fichier endommagé est signalé comme erreur, l'exécution continue avec les
  autres, et la commande se termine avec le code de sortie `1` pour que les
  scripts le remarquent.
- Relancer la même commande ne refait pas le travail terminé : les œuvres dont
  les résultats sont déjà là sont ignorées. Ajoutez `-f` pour les régénérer.
- Après une exécution sans échec, les projets allégés sont empaquetés dans
  `procreate.zip` et le dossier `procreate/` est supprimé. Une exécution avec le
  moindre échec conserve le dossier non empaqueté, pour que vous puissiez
  l'examiner et reprendre. `--no-zip` conserve toujours le dossier.

## Vérifier un fichier avant conversion

Ces deux commandes ne créent aucune vidéo et ne modifient rien.

**Y a-t-il un timelapse dans ce fichier, et de quelle longueur ?**

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

`--list` lit la table des matières du fichier. C'est instantané, et cela vous dit
s'il y a un timelapse du tout.

**La conversion va-t-elle vraiment fonctionner ?**

```bash
procreepy --verify artwork.procreate
```

`--verify` va plus loin : il lit chaque segment du timelapse, cherche des
dommages et confirme que les segments peuvent être assemblés sans réencodage.
Plus lent que `--list`, et la réponse honnête à « est-ce que ça se convertira
proprement ? ».

Les deux acceptent aussi un dossier, et rendent alors compte de chaque fichier
qu'il contient.

## Exporter en PSD

```bash
procreepy --psd input/ output/
```

À côté de chaque vidéo et de chaque projet allégé, `output/psd/NAME.psd` est
écrit : un fichier Photoshop en calques, ouvrable dans Photoshop, Affinity Photo,
GIMP et équivalents.

`--psd` ne fonctionne **qu'avec un dossier en entrée**. Avec un fichier seul, la
commande s'arrête sur `--psd needs a directory INPUT; it writes into
OUTPUT/psd/`.

Le PSD est un export, pas une copie parfaite. Il conserve l'arborescence des
calques, la structure et l'ordre des groupes, leurs noms, la visibilité,
l'opacité, les modes de fusion et l'image elle-même ; il ne conserve **pas** les
masques de calque, les relations de détourage ni le texte modifiable. Lisez
[ce que le PSD conserve et ce qu'il perd](usage.md#export-a-psd) avant de
l'utiliser pour un travail final. Gardez le fichier `.procreate` comme copie de
référence.

Le PSD intègre aussi une petite image d'aperçu, si bien que les applications qui
le lisent (Photoshop, Affinity, GIMP) affichent une vignette. Cela ne suffit
**pas** à faire dessiner une vignette par l'Explorateur Windows : celui-ci
réclame un gestionnaire de vignettes enregistré pour `.psd`, que Windows ne
fournit pas (Photoshop ou un pack comme SageThumbs en fournit un), et il n'en
existe aucun pour `.procreate`.

## Ce qui arrive à vos fichiers

- **Vos originaux ne sont jamais modifiés.** procreepy ouvre les `.procreate` en
  lecture seule. Tout ce qu'il produit est écrit ailleurs.
- **Rien ne reste à moitié écrit.** Chaque résultat est d'abord construit dans un
  fichier temporaire, puis mis en place seulement une fois complet. Une exécution
  interrompue ou en échec ne laisse jamais de vidéo cassée et n'abîme pas un
  fichier déjà présent.
- **Les exécutions sur dossier publient par œuvre, en lot.** La vidéo, le projet
  allégé et le PSD d'une œuvre apparaissent ensemble ou pas du tout : vous
  n'obtenez jamais une vidéo sans son projet.
- **Les dates sont reportées.** Chaque sortie — vidéo, projet allégé et PSD —
  reçoit la date de modification du `.procreate` dont elle provient (et, sous
  Windows, aussi la date de création), si bien qu'un projet réimporté dans
  Procreate garde sa place dans la galerie.
- **Relancer est sûr.** Les œuvres terminées sont ignorées, que les projets soient
  encore un dossier ou déjà empaquetés dans `procreate.zip`. Un lot resté
  incomplet après une interruption est régénéré en entier. `-f` régénère tout.
- **Convertir un fichier remplace la destination** si elle existe, après que la
  nouvelle vidéo a été écrite en entier.
- **Les gros fichiers demandent de l'espace temporaire.** Les grands timelapses
  sont assemblés via un fichier temporaire. Si l'espace manque, pointez
  `--tmpdir` vers un disque plus spacieux.

## Si quelque chose ne va pas

| Ce que vous voyez | Ce que cela signifie |
|---|---|
| `procreepy: command not found` | Vous n'êtes pas dans le dossier où vous l'avez décompressé ; sur macOS/Linux utilisez `./procreepy`. |
| `no video/segments in the archive` | Aucun timelapse n'a été enregistré dans ce fichier. Il est irrécupérable. |
| `input is not a valid ZIP archive` | Ce n'est pas un fichier `.procreate`, ou le téléchargement/la copie est tronqué. |
| `segment ... is corrupted inside the archive` | Les données du timelapse sont endommagées. |
| `segments are incompatible` | Le timelapse a été enregistré à travers un changement de toile ou de qualité : impossible de l'assembler sans réencoder. |
| `refusing to write video data to a terminal` | Donnez un nom de fichier de destination, ou redirigez avec `> out.mp4`. |
| `Windows a protégé votre ordinateur` | Voir [la note sur SmartScreen](#windows). |
| `no space left` / erreurs d'écriture | Utilisez `--tmpdir` sur un disque avec plus d'espace libre. |

Chacun de ces cas, avec le symptôme exact et la conduite à tenir, est dans
[troubleshooting.md](troubleshooting.md).

## Référence des commandes

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` est un fichier `.procreate`, un dossier qui en contient, ou `-` pour
l'entrée standard. `OUTPUT` est un nom de fichier, un dossier, ou `-` pour la
sortie standard. Pour un fichier seul, omettre `OUTPUT` écrit la vidéo sur la
sortie standard ; pour un dossier, la valeur par défaut est `<INPUT>_procreepy/`
dans le répertoire courant.

| Option | Rôle | S'applique à |
|---|---|---|
| `-h`, `--help` | afficher l'aide et quitter | toujours |
| `--version` | afficher la version et quitter | toujours |
| `--list` | lister les segments du timelapse ; n'écrire aucune vidéo | fichier ou dossier |
| `--verify` | vérifier chaque segment ; n'écrire aucune vidéo | fichier ou dossier |
| `-r`, `--recursive` | traiter aussi les sous-dossiers | entrée dossier uniquement |
| `-f`, `--force` | écraser les résultats déjà présents | entrée dossier uniquement |
| `--psd` | exporter en plus un PSD en calques par œuvre | entrée dossier uniquement |
| `--no-zip` | laisser les projets en dossier `procreate/` au lieu d'empaqueter `procreate.zip` | entrée dossier uniquement |
| `--strict` | traiter les trous dans la numérotation des segments comme des erreurs, non des avertissements | fichier ou dossier |
| `--tmpdir DIR` | où placer les fichiers temporaires | toujours |
| `-q`, `--quiet` | n'afficher que les avertissements et les erreurs | toujours |
| `--` | arrêter la lecture des options ; traiter le reste comme des noms de fichiers | toujours |

Référence complète avec exemples, formats de sortie et codes de sortie :
[usage.md](usage.md).

## Comment ça marche

Un fichier `.procreate` est une archive ZIP. Quand l'enregistrement du timelapse
est actif, Procreate y stocke la vidéo terminée, découpée en morceaux numérotés
(`video/segments/segment-1.mp4`, `segment-2.mp4`, …). procreepy lit ces morceaux
directement dans l'archive, les trie numériquement, vérifie qu'ils partagent le
même codec et la même toile, et les coud en un seul MP4 en copiant les images
telles quelles. Rien n'est rendu, rien n'est réencodé : c'est pour cela que c'est
rapide et sans perte.

Le chemin du timelapse ne regarde jamais vos calques. Seul `--psd` lit l'œuvre
elle-même.

Détails — assemblage MP4, écriture atomique, stratégie des fichiers temporaires,
fidélité du PSD : [how-it-works.md](how-it-works.md).

## Développement

Compilation, tests, couverture, compilation croisée, release et CI :
[development.md](development.md).

Les prérequis sont Go (la version est dans [`go.mod`](../go.mod)) et `make`. Le
projet n'a aucune dépendance tierce.

```bash
make check   # vérification du format + build + vet + toute la suite de tests
```

## Licence

Apache License 2.0 — voir [LICENSE](../LICENSE).

## Langues

[English](../README.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文（简体）](README.zh-CN.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md) · [Русский](README.ru.md) · [Português](README.pt.md) · [Deutsch](README.de.md) · [Bahasa Indonesia](README.id.md)
