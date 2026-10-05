# Usage reference

Complete reference for the `procreepy` command line. For a first run, start
with the [README](../README.md).

- [Input and output](#input-and-output)
- [Convert one file](#convert-one-file)
- [Convert a folder](#convert-a-folder)
- [Inspect before converting](#inspect-before-converting)
- [Export a PSD](#export-a-psd)
- [Container usage](#container-usage)
- [Options](#options)
- [Exit codes](#exit-codes)
- [Messages, logs and colors](#messages-logs-and-colors)
- [Temporary files](#temporary-files)

## Input and output

```text
procreepy [options] INPUT [OUTPUT]
```

`INPUT` is a `.procreate` file, a directory containing them, or `-` for stdin.
`OUTPUT` is a file name, a directory, or `-` for stdout.

| INPUT | OUTPUT | Result |
|---|---|---|
| file | file | the video is written to that name |
| file | directory (existing, or a name ending in a separator) | `DIR/NAME.mp4`, named after the input |
| file | `-` | the video goes to stdout |
| file | omitted | the video goes to stdout |
| `-` (stdin) | file, directory or `-` | same as above; a name cannot be derived from stdin, so a directory destination is an error |
| directory | directory | two output trees, see [Convert a folder](#convert-a-folder) |
| directory | omitted | the same, under `output/` |
| directory | `-` | error: a set of results cannot go to stdout |

All four single-file combinations are supported: `FILE OUTPUT`, `FILE -`,
`- OUTPUT`, `- -`.

## Convert one file

```bash
procreepy artwork.procreate artwork.mp4   # to a file
procreepy artwork.procreate > artwork.mp4 # to stdout, same bytes
cat artwork.procreate | procreepy - > artwork.mp4
procreepy artwork.procreate videos/       # videos/artwork.mp4
```

File output is published atomically: the video is written to a `.partial` file
next to the target and renamed into place only after it is complete. A failed
run leaves no stub and never corrupts a file that was already there.

Writing to stdout produces exactly the same bytes as writing to a file. If
stdout is a terminal, procreepy refuses rather than dumping binary data into
your session (exit code `2`).

## Convert a folder

```bash
procreepy input/              # -> output/
procreepy input/ out/         # -> out/
procreepy -r input/ out/      # including sub-folders
procreepy -f input/ out/      # rebuild everything
procreepy --psd input/ out/   # also export PSDs
```

Each converted artwork produces a pair of results in two trees:

```text
out/timelapses/NAME.mp4                  the joined timelapse
out/projects/NAME.procreepy.procreate    the project without the timelapse
out/psd/NAME.psd                         only with --psd
```

### Naming

`NAME` is the input file name without its `.procreate` extension. Spaces,
Cyrillic, CJK and other characters are preserved as-is. On case-insensitive file
systems, names that differ only in letter case would collide, so the second and
later ones get `-2`, `-3`, … suffixes (`a.procreate` and `A.procreate` become
`A.mp4` and `a-2.mp4`).

With `-r`, the sub-folder structure is mirrored inside both output trees, so
identical names in different source folders never collide.

Dot-files are skipped on purpose — this is what macOS leaves behind when copying
to a non-Apple file system (`._Foo.procreate`).

### The slim project

`projects/NAME.procreepy.procreate` is the same archive with the
`video/segments/segment-N.mp4` members removed. Every other member is carried
over byte for byte, preserving order, compression method and timestamps, so the
project remains a valid Procreate document. It keeps the modification time of
its source, so re-importing it into Procreate does not reshuffle your gallery.

This is also the way to reclaim space: the timelapse is usually the largest part
of a `.procreate` file.

### Re-running, skipping and failures

- An artwork whose whole output set is already present is **skipped**.
- A set that is only partly present — an earlier run was interrupted — is
  **regenerated as a whole**.
- `-f` (`--force`) overwrites outputs that already exist.
- Each artwork's results are staged to scratch files and published **together**.
  You never get a video without its project.
- A file with no recorded timelapse is a **warning**, not an error. Nothing is
  written for it and the run continues.
- A corrupt file is an **error**. The run continues with the remaining files, the
  failure appears in the summary, and the command exits with code `1`.

### What a run looks like

All of this goes to stderr, one structured record per line:

```text
level=INFO msg="batch conversion started" files=4 input=input/ timelapses=output/timelapses/ projects=output/projects/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate" removed_segments=17 video_size="6.7 MiB"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" timelapse="output/timelapses/Portrait of a Cat.mp4" project="output/projects/Portrait of a Cat.procreepy.procreate" removed_segments=18 video_size="4.1 MiB"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`removed_segments` is how many segment members were dropped from the slim
project; `video_size` is their total compressed size inside the archive.

A second run over the same input reports, per finished artwork:

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Landscape v2.procreate" timelapse="output/timelapses/Landscape v2.mp4" project="output/projects/Landscape v2.procreepy.procreate"
```

## Inspect before converting

Neither command writes a video or changes anything.

### `--list`

```bash
procreepy --list artwork.procreate
```

```text
input: artwork.procreate
segments: 18

1  video/segments/segment-1.mp4
2  video/segments/segment-2.mp4
...
17 video/segments/segment-17.mp4
18 video/segments/segment-18.mp4
```

Reads the archive's table of contents only: the segment members and their
playback order. Instant, even on large files. Use it to answer "is there a
timelapse in here?".

### `--verify`

```bash
procreepy --verify artwork.procreate
```

Parses every segment straight out of the archive — including the ZIP CRC check —
prints a line-by-line report, and checks that the segments can be joined without
re-encoding. Use it to answer "will the conversion work?".

```text
input: artwork.procreate
segments: 3

1 ok    video h264 320x240 yuv420p, audio aac 44100Hz 2ch  2.00s  video/segments/segment-1.mp4
2 ok    video h264 320x240 yuv420p, audio aac 44100Hz 2ch  2.00s  video/segments/segment-2.mp4
3 ok    video h264 320x240 yuv420p, audio aac 44100Hz 2ch  2.00s  video/segments/segment-3.mp4

verify: ok, 3 segment(s), ~6.0 s of video
```

A damaged segment is reported in place. For a single file the exit code is then
`5` (or `7` if the segments are merely incompatible with each other); for a
directory run it is `1`, since individual files are reported and the walk
continues.

```text
1 FAIL  segment video/segments/segment-1.mp4 is corrupted inside the archive: zip: checksum error
```

The difference in one line: `--list` inspects the archive structure, `--verify`
inspects the timelapse data itself and its compatibility.

Both accept a directory and then walk every file in it.

### `--strict`

By default, a gap in the segment numbering (`segment-1`, `segment-3`) is a
warning and the available segments are joined. With `--strict` a gap is an error
(exit code `5`) and nothing is produced. Segment names without a number are
always ignored with a warning.

## Export a PSD

```bash
procreepy --psd input/ out/
```

Directory input only. Given a single file, procreepy stops with exit code `2`
and `--psd needs a directory INPUT; it writes into OUTPUT/psd/`.

Each PSD is published atomically with the rest of that artwork's set:

```text
level=INFO msg="psd exported" input="input/Portrait of a Cat.procreate" psd="out/psd/Portrait of a Cat.psd" layers=3
```

**What the PSD preserves**

- the layer tree (groups and order), layer names in Unicode, visibility,
  opacity, blend modes, bounds and lock state;
- 8-bit RGBA pixels for every layer, PackBits-compressed;
- the DPI and the embedded ICC profile;
- a merged composite taken verbatim from Procreate's own flattened render. When
  that is missing or damaged, the visible layers are composited in Normal mode
  as an approximation.

**What it does not preserve**

- layer masks, and exact clipping-to-the-layer-below semantics;
- editable text — text layers keep their pixels, not their text data;
- exact straight (unpremultiplied) alpha. Procreate stores premultiplied 8-bit
  tiles, so colors in the soft fringe at layer edges can differ slightly;
- canvases wider or taller than 30000 pixels are refused outright, which is the
  PSD format limit (PSB is not produced).

The PSD is a snapshot for Photoshop and other importers. The `.procreate` file
stays the master copy.

## Container usage

Published for every release, for `linux/amd64` and `linux/arm64`:

```text
registry.gitlab.com/po1nt-1/procreepy:<version>
registry.gitlab.com/po1nt-1/procreepy:latest
```

Image tags carry no `v` prefix: the tag for release `v0.3.0` is `0.3.0`.
`latest` is never moved onto a prerelease. An SPDX SBOM is published next to
each image.

```bash
# one file
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest artwork.procreate artwork.mp4

# a folder
docker run --rm -v "$PWD":/data -w /data \
  registry.gitlab.com/po1nt-1/procreepy:latest input/ output/

# stdin to stdout needs no mount
docker run --rm -i registry.gitlab.com/po1nt-1/procreepy:latest - \
  < artwork.procreate > artwork.mp4
```

`podman` substitutes for `docker` as-is.

- **File ownership.** The image runs as uid 65532 (`distroless/static:nonroot`).
  On a Linux host add `--user "$(id -u):$(id -g)"` so the results belong to you.
  Docker Desktop on macOS maps mount ownership itself and needs nothing.
- **No shell inside.** The image contains only the static binary, so
  `docker run … --help` works but there is no `sh` to exec into.
- **Temporary files** land in the container's writable layer, not on the mount.
  `--tmpdir /data/tmp` moves them onto the mounted volume.
- **Registry access** follows project visibility; for a private project run
  `docker login registry.gitlab.com` first.

## Options

| Option | Purpose | Applies to | Example |
|---|---|---|---|
| `-h`, `--help` | show the help message and exit | always | `procreepy --help` |
| `--version` | show the version number and exit | always | `procreepy --version` |
| `--list` | list the segments in playback order; write no video | file or directory INPUT, no OUTPUT | `procreepy --list art.procreate` |
| `--verify` | check every segment; write no video | file or directory INPUT, no OUTPUT | `procreepy --verify art.procreate` |
| `-r`, `--recursive` | also process sub-directories, mirroring their structure | directory INPUT only | `procreepy -r input/ out/` |
| `-f`, `--force` | overwrite outputs that already exist (default: skip them) | directory INPUT only | `procreepy -f input/ out/` |
| `--psd` | also export a layered `.psd` per artwork into `OUTPUT/psd/` | directory INPUT only | `procreepy --psd input/ out/` |
| `--strict` | treat missing segment numbers as errors instead of warnings | file or directory INPUT | `procreepy --strict art.procreate out.mp4` |
| `--tmpdir DIR` | where to put temporary files | always | `procreepy --tmpdir /var/tmp input/` |
| `-q`, `--quiet` | print only warnings and errors to stderr | always | `procreepy -q input/` |
| `--` | stop option parsing; treat the rest as positional | always | `procreepy -- --odd-name.procreate out.mp4` |

Combinations that are rejected with exit code `2`:

| Rejected | Message |
|---|---|
| `--psd` with a file or stdin INPUT | `--psd needs a directory INPUT; it writes into OUTPUT/psd/` |
| `--psd` together with `--list` or `--verify` | `--psd cannot be combined with --list or --verify` |
| `--list` or `--verify` with an OUTPUT argument | `--list and --verify take a single INPUT and no OUTPUT` |
| a directory INPUT with `-` as OUTPUT | `cannot write a directory of results to stdout; give an output directory (default: output/)` |
| a directory INPUT whose OUTPUT exists as a file | `output path exists and is not a directory: NAME` |
| `OUTPUT` naming the same file as `INPUT` | `OUTPUT is the same file as INPUT: NAME` |
| a video destined for stdout when stdout is a terminal | `refusing to write video data to a terminal; redirect stdout …` |
| stdin as INPUT when stdin is a terminal | `stdin is a terminal; pipe a .procreate file into it …` |
| any unrecognized option | `procreepy: error: unrecognized arguments: …` |

`--split` and `--reencode` existed in earlier versions and have been removed.
They are now rejected as unrecognized options.

In `--list` and `--verify` modes, `-r` still applies — a directory INPUT is
walked recursively with it. `-f` has no effect there, because nothing is
written.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | unexpected error; for a directory run, at least one file failed |
| 2 | bad arguments or an impossible request (see the list above) |
| 3 | input not found, empty, or not a ZIP archive |
| 4 | no `video/segments` in the archive — no timelapse was recorded |
| 5 | a corrupt segment, or ambiguous/missing numbering under `--strict` |
| 6 | reserved, unused |
| 7 | segments cannot be joined without re-encoding |
| 8 | reserved, unused |
| 9 | failed to write the result or a temporary file |
| 130 | interrupted (Ctrl+C or SIGTERM) |

Codes 6 and 8 are reserved and never returned; the tool has no external
dependencies that could fail.

## Messages, logs and colors

- stdout carries only the result: the video, or the `--list`/`--verify` report.
- Everything else — progress, warnings, errors — goes to stderr as one
  structured `key=value` record per line, prefixed `level=INFO`, `level=WARN` or
  `level=ERROR`. This is stable enough to grep and parse.
- `-q` silences `INFO`, leaving warnings and errors.
- When stderr is an interactive terminal and `NO_COLOR` is unset, the `WARN` and
  `ERROR` tokens are colored (yellow and bold red). Pipes, redirects, CI and
  tests always get the plain byte-for-byte format. There is deliberately no
  `--color` option; set `NO_COLOR=1` to turn coloring off in a terminal.

## Temporary files

Timelapse segments can be hundreds of megabytes, and reading from stdin spools
the whole `.procreate` to disk first, because ZIP needs random access. The
temporary directory is chosen in this order:

1. `--tmpdir DIR`
2. `$TMPDIR`
3. `/var/tmp`, if it exists and is writable — preferred over `/tmp`, which is
   RAM-backed on Fedora and other distributions
4. the system default

If the disk fills up while spooling, the error says so and points at `--tmpdir`
instead of failing with a bare "no space left". Temporary files are removed on
success, on error, and on Ctrl+C or SIGTERM.
