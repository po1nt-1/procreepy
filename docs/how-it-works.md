# How it works

Implementation notes. Nothing here is needed to use the tool — see the
[README](../README.md) for that, and [usage.md](usage.md) for the full command
reference.

## What a `.procreate` file is

A `.procreate` document is a ZIP archive. When Timelapse Recording is enabled,
Procreate stores the finished timelapse inside it, already encoded, split into
numbered pieces:

```text
video/segments/segment-1.mp4
video/segments/segment-2.mp4
...
```

Those files are ordinary MP4s. Extracting the timelapse therefore means reading
them out of the archive and concatenating them — not rendering anything. That is
why procreepy needs no ffmpeg and no GPU, and why it is lossless: the encoded
frames are copied across untouched.

The rest of the archive — `Document.archive` (an NSKeyedArchiver plist), the
layer tiles (`*.lz4`), thumbnails — is what holds the artwork itself. The
timelapse path never reads any of it. Only `--psd` does.

## The conversion pipeline

1. **Input.** `INPUT` is a file or stdin. Stdin, and any non-seekable input, is
   spooled to a temporary file first, because ZIP requires random access to its
   central directory.
2. **Archive validation.** The ZIP is opened read-only and the
   `video/segments/segment-N.mp4` members are located.
3. **Numeric sort.** Segments are ordered by their number, not lexically, so
   `segment-9` precedes `segment-10`. A gap in the numbering is a warning (an
   error under `--strict`); a segment name without a number is ignored with a
   warning.
4. **Per-segment parsing.** Each segment is parsed directly out of the archive,
   without extracting it: MP4 box structure, track sizes, codec parameters. The
   ZIP CRC is checked as the data streams past. The first corruption stops the
   run.
5. **Compatibility check.** Resolution, codec, SPS/PPS parameter sets and audio
   layout must match across segments. Without this check a stream copy would
   silently produce a video that plays as garbage after the first boundary, so a
   mismatch is a hard error with a message naming the difference.
6. **Assembly.** The output MP4 is written moov-first: `ftyp`, then `moov` with
   all tracks spliced together from the segments' own metadata, then the `mdat`
   payloads back to back in playback order.
7. **Publication.** Everything a run promises — the MP4, the slim project, the
   PSD — is staged to scratch files and put in place as one set only after the
   last one succeeded. In a folder run the next artwork is attempted regardless.
8. **Cleanup.** Temporary files are removed on success, on error, on Ctrl+C and
   on SIGTERM.

### Why moov-first

The metadata is fully known before any payload is written, because frames are
copied rather than encoded. Writing `moov` first costs nothing and produces a
file that players and editors can open without scanning to the end — the same
property as a "faststart" MP4. The identical byte stream is produced whether the
destination is a file or a pipe, so `> artwork.mp4` and an explicit
`procreepy artwork.procreate artwork.mp4` give the same result.

### Large files

Sample offsets are written as 32-bit `stco` entries while they fit, and promoted
to 64-bit `co64` when the payload crosses 4 GiB. The code path exists for very
long timelapses; it is not exercised by the test corpus, which stays small.

## Atomic output

- **A single file** is written to a `.partial` file next to the target and
  renamed into place only after the content is complete and flushed. A failed or
  interrupted run leaves no stub, and an existing file at the target is either
  fully replaced or left exactly as it was.
- **A folder run** publishes per artwork. The timelapse, the slim project and
  the PSD for one input become visible together. You never end up with a video
  whose project is missing, which is what makes a re-run able to tell "finished"
  from "interrupted": a partial set is rebuilt as a whole.
- **On Unix** the final step is `rename(2)`, which is atomic within a file
  system.
- **On Windows** it is `MoveFileEx` with replace-existing. That is not atomic in
  the POSIX sense, and the practical difference shows up in exactly one case: if
  the target is still open in another program, the rename is refused with a
  readable `Access is denied`, the old file survives untouched, and a rerun after
  closing that program succeeds.

## The slim project

`projects/NAME.procreepy.procreate` is the input archive with the
`video/segments/segment-N.mp4` members removed and nothing else changed. Members
are copied at the raw-ZIP level: the stored bytes, compression method, order and
per-entry timestamps are preserved rather than recompressed, so the result is
bit-identical to the source apart from the dropped members. The file's
modification time is set to the source's, so re-importing it into Procreate does
not reshuffle the gallery by date.

## PSD export

`--psd` is the only path that reads the artwork. It parses `Document.archive`
(the NSKeyedArchiver plist describing the layer tree), decompresses the per-layer
tiles, and assembles a Photoshop document: layer records with names, bounds,
opacity, blend modes and flags, 8-bit RGBA channel data compressed with PackBits,
the DPI and ICC profile, and a merged composite.

The composite is taken verbatim from Procreate's own flattened render when the
archive has one. When it is missing or damaged, the visible layers are
composited in Normal mode as an approximation — which is why the blend modes of
a complex document may not be reproduced pixel-exactly in the flattened preview,
even though each layer's own pixels are intact.

Fidelity limits are listed in [usage.md](usage.md#export-a-psd). The important
structural ones: Procreate stores premultiplied 8-bit tiles, so straight alpha
cannot be recovered exactly; masks and clipping semantics have no direct PSD
equivalent as used here; and the PSD format caps dimensions at 30000 pixels,
beyond which the export is refused rather than silently switching to PSB.

## Temporary files

The temporary directory is chosen as `--tmpdir` → `$TMPDIR` → `/var/tmp` →
system default. `/var/tmp` is preferred over `/tmp` deliberately: on Fedora and
others `/tmp` is a tmpfs in RAM, and spooling a multi-hundred-megabyte archive
there would consume memory. Running out of space produces a message that names
the condition and points at `--tmpdir`, instead of a bare "no space left".

## Testing approach

The suite needs no real `.procreate` files. `internal/testkit` builds ZIP
archives out of synthesised MP4 segments and synthesised documents, so the tests
are hermetic and fast. The assertions are structural: the resulting MP4 is
parsed back, box order and sample counts are checked, `mdat` content is compared
byte for byte.

`internal/e2e` goes through the real compiled binary as a black box and pins the
observable contract: exit codes and the exact bytes of stdout and stderr. That
is what keeps the CLI help, log lines and reports from drifting unnoticed.

Covered cases include: the ordinary file, a missing `video/segments`, a single
segment, out-of-order numbering (`segment-9` before `segment-10`), stdin and
stdout, spaces and non-ASCII in names, corrupt and truncated ZIPs, corrupt and
truncated MP4s, CRC damage, write failures against `/dev/full`, incompatible
segments, and the whole folder workflow.

See [development.md](development.md) for how to run all of it.

## What the tool deliberately does not do

- It does not reconstruct a timelapse that was never recorded. There is no
  drawing history to replay — the timelapse is a video Procreate produced while
  you worked, and if recording was off it simply is not in the file.
- It does not repair damaged data. A corrupt segment is reported, not patched.
- It does not re-encode. Incompatible segments are refused rather than
  transcoded, because transcoding would change the pixels the user came for.
- It does not touch the originals, ever, in any mode.

A note on integrity checks: running `lz4 -t` against a `.lz4` file taken out of
a `.procreate` is not a valid test. Those are not standalone LZ4 frames.

## Format references

These projects documented the `.procreate` format independently and were useful
while writing the parser:

- Silica Viewer — https://github.com/heyzoish/silica-viewer
- Silicate — https://github.com/axaril/silicate
- ProcreateViewer — https://github.com/NothingData/ProcreateViewer
