# procreepy

Turn the timelapse that Procreate already recorded inside your `.procreate`
file into a normal MP4 video.

- No re-encoding: the frames are copied, so the video keeps Procreate's own quality.
- Your original `.procreate` files are never modified.
- One file or a whole folder at a time.
- A single program file. No ffmpeg, no Python, no account, nothing to configure.
- Works offline on Windows, macOS and Linux.

## Is this for you?

Use procreepy if:

- you have `.procreate` files;
- **Timelapse Recording was on** while you drew (it is on by default in Procreate);
- you want that timelapse as an MP4 you can upload, edit or keep;
- or you want to clear timelapse data out of your projects to make them smaller.

procreepy **cannot**:

- create a timelapse that was never recorded — it only extracts an existing one;
- rebuild a timelapse from your layers or undo history;
- repair a damaged `.procreate` file;
- export a still image of your artwork (it can export a layered PSD, see
  [Export a PSD](#export-a-psd)).

Not sure whether your file has a timelapse? [Check it first](#check-a-file-first) —
that takes one command and creates nothing.

## What you get

One file in, one video out:

```text
my-art.procreate  →  procreepy  →  my-art.mp4
```

A folder in, a video tree and a project archive out:

```text
input/                          input_procreepy/
├── Cat.procreate         →     ├── mp4/
├── Landscape.procreate   →     │   ├── Cat.mp4
└── Sketch.procreate      →     │   ├── Landscape.mp4
                                │   └── Sketch.mp4
                                └── procreate.zip   (the slim projects)
```

- `mp4/` holds the videos.
- `procreate.zip` holds a copy of each artwork **with the timelapse removed** —
  much smaller, and you can import it back into Procreate. Everything else in the
  project is kept byte for byte. It is a plain zip with a `procreate/` folder
  inside, ready to drop onto an iPad; pass `--no-zip` to get that `procreate/`
  folder on disk instead of the archive.
- The output folder is named after your input (`input/` → `input_procreepy/`) and
  created in the directory you run the command from. Give a second path to choose
  it yourself.
- Every output keeps the date of the file it came from, so re-importing a project
  into Procreate does not reshuffle your gallery.
- `input/` is left exactly as it was.

## Install

Download a ready-made program for your system from the releases page — you do
not need Go or any build tools.

- GitLab: https://gitlab.com/po1nt-1/procreepy/-/releases
- GitHub: https://github.com/po1nt-1/procreepy/releases

Pick the file that matches your computer:

| Your system | Download |
|---|---|
| Windows (most PCs) | `procreepy_<version>_windows_amd64.zip` |
| Windows on ARM | `procreepy_<version>_windows_arm64.zip` |
| Mac with Apple Silicon (M1–M5) | `procreepy_<version>_darwin_arm64.tar.gz` |
| Mac with Intel | `procreepy_<version>_darwin_amd64.tar.gz` |
| Linux (most PCs) | `procreepy_<version>_linux_amd64.tar.gz` |
| Linux on ARM (Raspberry Pi, Graviton) | `procreepy_<version>_linux_arm64.tar.gz` |
| Linux on 32-bit ARM | `procreepy_<version>_linux_arm.tar.gz` |

On a Mac, click the Apple menu → About This Mac to see whether you have Apple
Silicon or Intel.

### Windows

1. Download `procreepy_<version>_windows_amd64.zip`.
2. Right-click the downloaded file → **Extract All** → pick a folder you can find
   again, for example `Downloads\procreepy`.
3. Open that folder, then click the address bar at the top, type `cmd` and press
   Enter. A black Command Prompt window opens in that folder.
4. Put a `.procreate` file in the same folder and run:

   ```text
   procreepy.exe "My Artwork.procreate" "My Artwork.mp4"
   ```

   Quotation marks matter only if the name contains spaces.

**About the SmartScreen warning.** The program is not signed with a paid
Microsoft certificate, so the first time you run it Windows may show a blue
window saying *"Windows protected your PC"* and name an *"unrecognized app"*.
This is not a virus report — Windows shows it for any program it has not seen
often enough yet. To continue, click **More info**, then the **Run anyway**
button that appears. If you prefer not to, use the
[container image](#docker--podman) instead.

### macOS

1. Download the `.tar.gz` for your chip (`darwin_arm64` for Apple Silicon,
   `darwin_amd64` for Intel).
2. Open Terminal (Applications → Utilities → Terminal) and go to your Downloads
   folder:

   ```bash
   cd ~/Downloads
   ```

3. Unpack it and allow it to run:

   ```bash
   tar -xzf procreepy_*_darwin_*.tar.gz
   xattr -d com.apple.quarantine ./procreepy
   ```

   The `xattr` line removes the download quarantine flag. Without it macOS
   refuses to start the program, because it is not notarized by Apple.

4. Convert a file:

   ```bash
   ./procreepy "My Artwork.procreate" "My Artwork.mp4"
   ```

To be able to type `procreepy` from anywhere, move it onto your PATH:
`sudo mv ./procreepy /usr/local/bin/`.

### Linux

```bash
tar -xzf procreepy_*_linux_amd64.tar.gz
./procreepy artwork.procreate artwork.mp4
```

The program is statically linked, so it runs on any distribution regardless of
its glibc version. To install it for all users:
`sudo install -m 755 procreepy /usr/local/bin/`.

### Docker / Podman

A container image is published for every release, for `linux/amd64` and
`linux/arm64`. On an Apple Silicon Mac the arm64 variant is selected
automatically.

```bash
# Docker
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# Podman (rootless, Linux): map your user and relabel the mount with :Z
podman run --rm --userns=keep-id --user "$(id -u):$(id -g)" \
  -v "$PWD":/data:Z -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4
```

The two engines are not interchangeable flag-for-flag: rootless Podman needs
`--userns=keep-id`, `--user` and — on an SELinux host — a `:Z` mount, or the run
fails with a permission error on the output. Pin a version with `:0.3.0` instead
of `:latest` (image tags carry no `v` prefix). See
[docs/usage.md](docs/usage.md#container-usage) for file ownership, SELinux and
other details.

### Build from source

Only needed if you want to change the code. See
[docs/development.md](docs/development.md).

### Verifying your download (optional)

Each release also publishes `CHECKSUMS.txt`. To confirm a download arrived
intact, print the hash of your file and compare it with the matching line in
that file:

```bash
sha256sum procreepy_0.3.0_linux_amd64.tar.gz    # Linux
shasum -a 256 procreepy_0.3.0_darwin_arm64.tar.gz   # macOS
grep darwin_arm64 CHECKSUMS.txt                 # the expected value
```

On Windows: `certutil -hashfile procreepy_0.3.0_windows_amd64.zip SHA256`.

The two values must be identical. This step is optional — it detects a
truncated or tampered download, not much else.

## Convert one file

```bash
procreepy artwork.procreate artwork.mp4
```

Result:

```text
artwork.mp4
```

The original `artwork.procreate` is not modified. If `artwork.mp4` already
exists it is replaced, but only after the new video has been written in full.

You can also hand it a folder as the destination, and let procreepy name the
file:

```bash
procreepy artwork.procreate videos/
```

Result: `videos/artwork.mp4`. The folder must already exist.

## Convert a folder

```bash
procreepy input/
```

This reads every `.procreate` file in `input/` and writes into
`input_procreepy/`, as shown in [What you get](#what-you-get). To choose the
destination yourself:

```bash
procreepy input/ ~/Videos/timelapses
```

To include sub-folders (their structure is mirrored in the output):

```bash
procreepy -r input/ output/
```

What happens as it runs:

- Progress is reported per file on the screen, one line each.
- A file whose timelapse was never recorded still produces its slim project (and
  its PSD under `--psd`) — only the video is skipped, with a note, and the run
  continues.
- A damaged file is reported as an error, the run still continues with the rest,
  and the command finishes with exit code `1` so scripts can notice.
- Running the same command twice does not redo finished work: artworks whose
  outputs are already there are skipped. Add `-f` to rebuild them anyway.
- After a run with no failures, the slim projects are packed into
  `procreate.zip` and the loose `procreate/` folder is removed. A run that had
  any failure keeps the folder unpacked, so you can inspect it and resume. Pass
  `--no-zip` to always keep the folder.

## Check a file first

Both of these commands create no video and change nothing.

**Is there a timelapse in this file, and how long is it?**

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

`--list` reads the file's table of contents. It is instant, and it tells you
whether a timelapse is there at all.

**Will the conversion actually work?**

```bash
procreepy --verify artwork.procreate
```

`--verify` goes further: it reads every timelapse segment, checks it for damage,
and confirms the segments can be joined without re-encoding. Slower than
`--list`, and the honest answer to "will this convert cleanly?".

Both accept a folder too, and then report on every file in it.

## Export a PSD

```bash
procreepy --psd input/ output/
```

Alongside each video and slim project, this writes `output/psd/NAME.psd`: a
layered Photoshop file you can open in Photoshop, Affinity Photo, GIMP and
similar.

`--psd` works with **folder input only**. Given a single file it stops with
`--psd needs a directory INPUT; it writes into OUTPUT/psd/`.

The PSD is an export, not a perfect copy. It keeps the layer tree, group
structure and order, names, visibility, opacity, blend modes and the image
itself; it does **not** keep layer masks, clipping relationships or editable
text. Read
[what the PSD preserves and what it loses](docs/usage.md#export-a-psd) before
using it for finished work. Keep the `.procreate` file as your master copy.

The PSD also embeds a small preview image, so apps that read it (Photoshop,
Affinity, GIMP) show a thumbnail. Note that this does **not** by itself make
Windows Explorer draw a thumbnail: Explorer needs a registered thumbnail handler
for `.psd`, which Windows does not ship (Photoshop or a pack such as SageThumbs
provides one), and none exists for `.procreate` at all.

## What happens to your files

- **Your originals are never modified.** procreepy opens `.procreate` files
  read-only. Everything it produces is written somewhere else.
- **Nothing is half-written.** Each result is built in a scratch file first and
  put in place only once it is complete. An interrupted or failed run never
  leaves a broken video behind, and never damages a file that was already there.
- **Folder runs publish per artwork, as a set.** The video, the slim project and
  the PSD for one artwork appear together or not at all — you never get a video
  without its project.
- **Dates are carried over.** Every output — video, slim project and PSD — is
  stamped with the modification date of the `.procreate` it came from (and, on
  Windows, the creation date too), so a project re-imported into Procreate keeps
  its place in your gallery.
- **Re-running is safe.** Finished artworks are skipped, whether the projects are
  still a folder or already packed into `procreate.zip`. A set left incomplete by
  an earlier interrupted run is rebuilt as a whole. `-f` rebuilds everything.
- **Converting one file replaces the destination** if it exists, after the new
  video is fully written.
- **Large files need temporary space.** Big timelapses are assembled through a
  temporary file. If you run out of space, point `--tmpdir` at a roomier disk.

## If something goes wrong

| What you see | What it means |
|---|---|
| `procreepy: command not found` | You are not in the folder you unpacked it into; use `./procreepy` on macOS/Linux. |
| `no video/segments in the archive` | No timelapse was recorded in that file. It cannot be recovered. |
| `input is not a valid ZIP archive` | Not a `.procreate` file, or the download/copy is truncated. |
| `segment ... is corrupted inside the archive` | The timelapse data is damaged. |
| `segments are incompatible` | The timelapse was recorded across a canvas or quality change, so it cannot be joined without re-encoding. |
| `refusing to write video data to a terminal` | Add a destination file name, or redirect with `> out.mp4`. |
| `Windows protected your PC` | See [the SmartScreen note](#windows). |
| `no space left` / write errors | Use `--tmpdir` on a disk with more free space. |

Each of these, with the exact symptom and what to do about it, is in
[docs/troubleshooting.md](docs/troubleshooting.md).

## Command reference

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` is a `.procreate` file, a folder of them, or `-` for standard input.
`OUTPUT` is a file name, a folder, or `-` for standard output. For a single
file, leaving `OUTPUT` out writes the video to standard output; for a folder it
defaults to `<INPUT>_procreepy/` in the current directory.

| Option | What it does | Applies to |
|---|---|---|
| `-h`, `--help` | show the help text and exit | always |
| `--version` | show the version and exit | always |
| `--list` | list the timelapse segments; write no video | file or folder |
| `--verify` | check every segment; write no video | file or folder |
| `-r`, `--recursive` | also process sub-folders | folder input |
| `-f`, `--force` | overwrite outputs that already exist | folder input |
| `--psd` | also export a layered PSD per artwork | folder input |
| `--no-zip` | leave the projects as a `procreate/` folder instead of `procreate.zip` | folder input |
| `--strict` | treat gaps in segment numbering as errors, not warnings | file or folder |
| `--tmpdir DIR` | where to put temporary files | always |
| `-q`, `--quiet` | print only warnings and errors | always |
| `--` | stop reading options; treat the rest as file names | always |

Full reference with examples, output formats and exit codes:
[docs/usage.md](docs/usage.md).

## How it works

A `.procreate` file is a ZIP archive. When timelapse recording is on, Procreate
stores the finished video inside it, split into numbered pieces
(`video/segments/segment-1.mp4`, `segment-2.mp4`, …). procreepy reads those
pieces straight out of the archive, sorts them numerically, checks that they
share the same codec and canvas, and stitches them into one MP4 by copying the
frames across untouched. Nothing is rendered and nothing is re-encoded, which is
why it is fast and lossless.

The timelapse path never looks at your layers. Only `--psd` reads the artwork
itself.

Details — MP4 assembly, atomic writes, temporary-file strategy, PSD fidelity:
[docs/how-it-works.md](docs/how-it-works.md).

## Development

Building, tests, coverage, cross-compilation, release and CI:
[docs/development.md](docs/development.md).

Requirements are Go (the version is in `go.mod`) and `make`. The project has
zero third-party dependencies.

```bash
make check   # format check + build + vet + the full test suite
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Languages

[English](README.md) · [Español](docs/README.es.md) · [Français](docs/README.fr.md) · [中文（简体）](docs/README.zh-CN.md) · [हिन्दी](docs/README.hi.md) · [العربية](docs/README.ar.md) · [Русский](docs/README.ru.md) · [Português](docs/README.pt.md) · [Deutsch](docs/README.de.md) · [Bahasa Indonesia](docs/README.id.md)
