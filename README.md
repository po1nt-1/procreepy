# procreepy

A small cross-platform utility (Linux, Windows, macOS): extracts the
ready-made archive timelapse from a `.procreate` file and joins its segments
into a single MP4. Nothing is re-encoded (stream copy), nothing is rendered.

`.procreate` is a ZIP archive. If timelapse recording was enabled, it
contains

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

The utility takes exactly these files: sorts them **numerically**
(`segment-9` before `segment-10`), parses the MP4 structure of every segment,
and rebuilds them into one moov-first MP4 with the frames copied as-is. It
never opens `Document.archive`, layers, or raster chunks (`*.lz4`).

## Requirements

No external dependencies: no `ffmpeg`, no `ffprobe`. Building requires
only Go (the version is in `go.mod`) and `make` (present on every
supported platform; on minimal systems it is a package-manager away).
The Makefile is the canonical build entry: it pins the same hermetic
environment CI uses (offline module mode, local toolchain, no cgo) and
self-locates the Go toolchain.

```bash
make build      # compile everything, drop a runnable ./procreepy
make check      # gofmt + build + vet + full test suite
```

`make build` stamps the binary with `dev-<commit>` so `procreepy
--version` reports where it came from (release tarballs carry the tag
instead). Without `make`, the raw equivalent is `go build ./... && go
build -o procreepy ./cmd/procreepy` (that binary reports `dev`, or
`dev-<commit>` when built inside a git checkout).

### Building for other operating systems

The project is pure Go and cross-compiles cleanly for every supported
target. From any platform:

| Target | Command |
|---|---|
| Linux x86-64 | `make release GOOS=linux GOARCH=amd64` |
| Linux ARM 64-bit (Raspberry Pi, Graviton) | `make release GOOS=linux GOARCH=arm64` |
| Linux ARM 32-bit | `make release GOOS=linux GOARCH=arm` |
| Windows x86-64 (10/11) | `make release GOOS=windows GOARCH=amd64` |
| Windows ARM 64-bit | `make release GOOS=windows GOARCH=arm64` |
| macOS Intel | `make release GOOS=darwin GOARCH=amd64` |
| macOS Apple Silicon (M1–M5) | `make release GOOS=darwin GOARCH=arm64` |

Each target produces a normalized `dist/procreepy-<version>-<os>-<arch>.tar.gz`
and prints its SHA-256; `make cross` builds the whole matrix at once, and
`make repro` proves a build is bit-for-bit reproducible. The raw equivalent
for one target is `GOOS=… GOARCH=… go build -o procreepy[.exe] ./cmd/procreepy`.

All builds are static (no cgo): a Linux binary runs on any distribution
regardless of its glibc version. The CI pipelines (GitLab and GitHub) build
exactly these targets on every commit; the `dist` job publishes the tarballs
plus a `SHA256SUMS` manifest, and the `repro:*` jobs prove the binaries are
bit-for-bit reproducible.

- Linux/macOS: no installation step, run the binary directly.
- Windows: the binary is unsigned, so SmartScreen may show "Protected your
  PC" — choose **More info → Run anyway**.

## Usage

Single file:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
cat artwork.procreate | procreepy - > artwork.mp4
procreepy --list artwork.procreate
procreepy --verify artwork.procreate
procreepy --split artwork.procreate artwork.mp4
```

All four `INPUT`/`OUTPUT` combinations are supported:
`FILE OUTPUT`, `FILE -`, `- OUTPUT`, `- -`. If `OUTPUT` is omitted, it is
stdout. If `OUTPUT` is an existing directory, the video is placed in it under
the original name (`procreepy art.procreate videos/` → `videos/art.mp4`).

### Batch mode: a folder of `.procreate` files → a folder of videos

The "I have `input/` full of `.procreate` files and want the videos in
`output/timelaps/`" scenario:

```bash
procreepy input/
```

```text
input/                              output/timelaps/
├── Portrait of a Cat.procreate →   ├── Portrait of a Cat.mp4
├── Landscape v2.procreate      →   ├── Landscape v2.mp4
└── No Timelapse.procreate              └── (skipped, with a warning)
```

- **Names**: `<original name without .procreate>.mp4`. Spaces, Cyrillic, and
  special characters are preserved as-is.
- **Output folder**: by default `output/timelaps/` relative to the current
  directory, created automatically. Another one can be given as the second
  argument: `procreepy input/ ~/Videos/procreate`.
- **Re-running is safe**: videos that already exist are skipped. To rebuild
  everything: `--force` (`-f`).
- **`-r`** also descends into sub-folders; the sub-folder structure is
  mirrored in the result (`input/2025/Cat.procreate` →
  `output/timelaps/2025/Cat.mp4`), so identical names in different folders do
  not collide.
- **One bad file does not stop the rest.** A file without a timelapse
  (recording was off) is a warning, not an error. A corrupt file is an error:
  it lands in the final summary, and the exit code becomes `1`.
- Hidden files (`._Foo.procreate`, which macOS leaves behind when copying)
  are ignored.
- Originals are never modified.

Sample output (all of it goes to stderr):

```text
level=INFO msg="batch conversion started" files=4 input=input output=output/timelaps/
level=ERROR msg="file conversion failed" input="input/Corrupt file.procreate" err="input is not a valid ZIP archive: input/Corrupt file.procreate (not a .procreate file, or truncated/corrupted)"
level=INFO msg=converted input="input/Landscape v2.procreate" output="output/timelaps/Landscape v2.mp4"
level=WARN msg="no timelapse video inside, skipped" input="input/No Timelapse.procreate"
level=INFO msg=converted input="input/Portrait of a Cat.procreate" output="output/timelaps/Portrait of a Cat.mp4"
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

`--list` and `--verify` also accept a directory and walk all files in it.

### Splitting: video + slimmed-down project (`--split`)

The point: timelapses take up more space than the drawing itself — for
example, when backing up to an iPad you may want to keep them separate.
`--split` writes, next to each finished `MP4`, a slimmed copy of the project
**without** anything under `video/`:

```bash
procreepy --split artwork.procreate artwork.mp4
```

```text
artwork.procreate  →  artwork.mp4                    (the timelapse, lossless)
                     →  artwork.procreepy.procreate  (the same project, minus video/)
```

Batch mode works the same way: next to every `X.mp4` an
`X.procreepy.procreate` appears. All other archive members (layers,
`Info.plist`, previews) are carried over byte for byte: order, compression
methods, and timestamps are preserved. The original `.procreate` is not
modified; the slimmed copy cannot be written to stdout, so `--split` requires
a file `OUTPUT`.

### Diagnostics

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

Parses every segment straight out of the archive (including the CRC check
inside the ZIP), prints a line-by-line report, and checks that the segments
can be joined without re-encoding. **No output video is created.** `--list`
only reads the ZIP directory.

## Options

| Option | What it does |
|---|---|
| `-r`, `--recursive` | directory input: descend into sub-folders as well |
| `-f`, `--force` | directory input: overwrite videos that already exist |
| `--strict` | treat missing segment numbers as an error (warning by default) |
| `--reencode` | accepted for compatibility with older scripts; there is no re-encoding, it is always stream copy |
| `--split` | write `X.procreepy.procreate` next to each `MP4` — the project minus `video/` |
| `--tmpdir DIR` | where to put temporary files |
| `-q`, `--quiet` | print only warnings and errors |

## How it works

1. `INPUT` is a file or stdin. Stdin (and any non-seekable input) is spooled
   to a temporary file first, because ZIP requires random access.
2. The ZIP is validated (read-only), and `video/segments/segment-N.mp4`
   entries are located.
3. Numeric sort. Gaps in the numbering are a warning; names without a number
   are ignored with a warning.
4. Every segment is parsed straight out of the ZIP (without full extraction):
   MP4 boxes, track sizes, codec parameters. On the first corruption — stop.
5. Compatibility check (resolution, codec, SPS/PPS sets, audio). Otherwise
   `-c copy` would silently produce garbage — so incompatibility is an error
   with a clear message, not a surprise in the finished video.
6. The moov-first MP4 is assembled: `ftyp`, `moov` (all tracks, sliced out of
   the segments), then `mdat` after `mdat` in playback order.
7. The temporary file (if any) is removed always — on success, on error, on
   Ctrl+C, and on SIGTERM.

### Writing to a file and to stdout

Both paths assemble the same moov-first MP4: the moov atom is written first,
because frames are copied straight from the source segments and the metadata
is known before writing begins. For a file this is a "classic" MP4, suitable
for players and editors alike; the exact same file goes into a pipe —
`> artwork.mp4` produces the same result as an explicit
`procreepy artwork.procreate artwork.mp4`.

- **File** output is atomic: a `.partial` file next to the target, renamed
  only after success. A failed run leaves no stubs and never corrupts an
  existing file.
- stdout is never polluted with text. All log lines (`level=INFO`/`WARN`/
  `ERROR`, one structured key=value record per line) go to stderr. The single
  exception is the `--list`/`--verify` report, where stdout *is* the result.
  If stdout is a terminal, the utility refuses to dump a binary MP4 into it.

### Temporary files and Fedora

On Fedora, `/tmp` is a tmpfs in RAM. Timelapse segments can be hundreds of
megabytes, and when reading from stdin the whole `.procreate` is spooled.
That is why the temp directory is chosen this way: `--tmpdir` → `$TMPDIR` →
`/var/tmp` (on disk) → the system default. If the disk fills up while
spooling, you get a clear error with a hint (use `--tmpdir` on a bigger
disk-backed directory) instead of a bare "No space left".

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | unexpected error; in batch mode — at least one file failed |
| 2 | bad arguments; output would overwrite input; stdout is a terminal |
| 3 | input not found, empty, or not a ZIP |
| 4 | no `video/segments` in the archive (no timelapse was recorded) |
| 5 | corrupt segment; ambiguous or missing numbering (`--strict`) |
| 6 | reserved (unused: no external dependencies) |
| 7 | segments are incompatible for stream copy |
| 8 | reserved (unused: no external dependencies) |
| 9 | failed to write the result or temporary files |
| 130 | interrupted (Ctrl+C / SIGTERM) |

## Tests

```bash
make test          # or: go test ./...
```

Real `.procreate` files are not needed: the tests build ZIPs out of
generated MP4 segments (see `internal/testkit`). The checks are structural:
parsing the resulting MP4, box order, sample counts, `mdat` content.
`-race` is not required but works when a C compiler is installed.

Covered: the ordinary file, missing `video/segments`, a single segment,
segments out of order (`segment-9`/`segment-10`), stdin, stdout, spaces and
special characters in names, corrupt and truncated ZIPs, corrupt and
truncated MP4s, CRC damage, write errors (`/dev/full`), incompatible
segments, and the whole batch mode.

## What the utility deliberately does not do

It does not parse `Document.archive` (NSKeyedArchive), does not touch
`*.lz4`, does not restore layers, and does not render the image. If a
timelapse was not recorded in the file, this utility cannot recover it from
the drawing history. Note: `lz4 -t` on a `.lz4` taken out of a `.procreate`
is not an integrity check — they are not standalone LZ4 frames.

## Format references

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer

## License

Apache License 2.0, see `LICENSE`.

---

## Languages

[English](README.md) · [Español](docs/README.es.md) · [Français](docs/README.fr.md) · [中文（简体）](docs/README.zh-CN.md) · [हिन्दी](docs/README.hi.md) · [العربية](docs/README.ar.md) · [Русский](docs/README.ru.md) · [Português](docs/README.pt.md) · [Deutsch](docs/README.de.md) · [Bahasa Indonesia](docs/README.id.md)
