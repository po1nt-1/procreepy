# Troubleshooting

Each entry gives the symptom, the cause, what to run to confirm it, and what to
do. Exit codes are listed in [usage.md](usage.md#exit-codes).

- [The command is not found](#the-command-is-not-found)
- [Windows says the app is not recognized](#windows-says-the-app-is-not-recognized)
- [macOS refuses to open the program](#macos-refuses-to-open-the-program)
- [There is no timelapse in the file](#there-is-no-timelapse-in-the-file)
- [The file is not a valid Procreate file](#the-file-is-not-a-valid-procreate-file)
- [A segment is corrupted](#a-segment-is-corrupted)
- [The segments are incompatible](#the-segments-are-incompatible)
- [Some segment numbers are missing](#some-segment-numbers-are-missing)
- [Nothing was written and the output looks skipped](#nothing-was-written-and-the-output-looks-skipped)
- [Refusing to write video data to a terminal](#refusing-to-write-video-data-to-a-terminal)
- [stdin is a terminal](#stdin-is-a-terminal)
- [--psd does nothing or is rejected](#--psd-does-nothing-or-is-rejected)
- [Not enough temporary disk space](#not-enough-temporary-disk-space)
- [The output file is open in another program](#the-output-file-is-open-in-another-program)
- [The output directory is not writable](#the-output-directory-is-not-writable)
- [The folder run finished with warnings or errors](#the-folder-run-finished-with-warnings-or-errors)

## The command is not found

**Symptom**

```text
procreepy: command not found
'procreepy' is not recognized as an internal or external command
```

**Cause** — the program is not on your PATH. Unpacking an archive does not
install anything; the file sits wherever you extracted it.

**What to do** — either run it by path from the folder you unpacked it into:

```bash
./procreepy --version          # macOS, Linux
procreepy.exe --version        # Windows, from that folder
```

or install it once so the bare name works from anywhere:

```bash
sudo install -m 755 procreepy /usr/local/bin/   # Linux
sudo mv ./procreepy /usr/local/bin/             # macOS
```

## Windows says the app is not recognized

**Symptom** — a blue full-screen window: *"Windows protected your PC"*, naming
an *"unrecognized app"*, with only a **Don't run** button in sight.

**Cause** — Microsoft Defender SmartScreen flags executables it has not seen
distributed widely. The released binaries are not signed with a paid
code-signing certificate, so every new release starts out unrecognized. This is
not a malware detection.

**What to do** — click **More info**, then **Run anyway**. If you would rather
not bypass it, use the [container image](usage.md#container-usage), or build the
binary yourself from source — see [development.md](development.md).

## macOS refuses to open the program

**Symptom**

```text
"procreepy" cannot be opened because the developer cannot be verified
zsh: killed   ./procreepy
```

**Cause** — downloaded files get a quarantine attribute, and the binary is not
notarized by Apple.

**What to do** — clear the attribute once, after unpacking:

```bash
xattr -d com.apple.quarantine ./procreepy
./procreepy --version
```

## There is no timelapse in the file

**Symptom** — for a single file, exit code `4` and:

```text
level=ERROR msg="no video/segments in this archive (time-lapse recording was probably turned off for this artwork)"
```

For a folder run it is not an error: the slim project (and, under `--psd`, the
PSD) is written anyway, only the video is skipped, and the run continues:

```text
level=INFO msg="no timelapse inside, wrote the project without a video" input="input/Sketch.procreate" project="output/procreate/Sketch.procreepy.procreate"
```

Such artworks are counted under `no_video` in the batch summary, not `failed`.

**Cause** — Procreate's Timelapse Recording was off for that artwork, so no
video was ever stored inside the file.

**What to run**

```bash
procreepy --list artwork.procreate
```

If it reports `segments: 0`, there is nothing to extract.

**What to do** — no video can be recovered: procreepy extracts the timelapse
Procreate recorded and cannot reconstruct one from layers or undo history. The
slim project you still get is useful on its own (it is just the artwork without a
timelapse). For future artworks, enable Timelapse Recording in Procreate's canvas
settings before you start drawing.

## The file is not a valid Procreate file

**Symptom** — exit code `3`:

```text
level=ERROR msg="input is not a valid ZIP archive: artwork.procreate (not a .procreate file, or truncated/corrupted)"
```

**Cause** — the file is not a `.procreate` document at all, or the copy is
incomplete: an interrupted download, a cloud-sync placeholder that was never
materialised, or a transfer that dropped bytes.

**What to do**

- Check the file size against the original. A few kilobytes where you expect
  megabytes means a sync placeholder — open the file once in Finder or your
  cloud client to force a full download.
- Re-copy or re-export the file from the iPad.
- Confirm it is really a Procreate document and not, for example, a
  `.procreate`-renamed image.

## A segment is corrupted

**Symptom** — exit code `5`, and with `--verify` a per-segment report:

```text
1 FAIL  segment video/segments/segment-1.mp4 is corrupted inside the archive: zip: checksum error
```

**Cause** — the stored timelapse data fails its checksum inside the archive, or
the MP4 structure inside a segment is truncated. The damage is in the source
file.

**What to run**

```bash
procreepy --verify artwork.procreate
```

**What to do** — procreepy stops at the first corrupt segment rather than
writing a half-valid video; it cannot repair the data. Try an earlier copy or
backup of the `.procreate` file. If only later segments are damaged, there is
currently no option to keep the healthy prefix.

## The segments are incompatible

**Symptom** — exit code `7`:

```text
level=ERROR msg="segments are not stream-copy compatible: stream copy is impossible: ... (segment 5)"
```

**Cause** — the timelapse pieces do not share the same video properties, so they
cannot be concatenated without re-encoding. The message names the specific
difference, and the segment where it appears.

**What to do** — procreepy copies frames and never re-encodes, so it refuses
instead of silently producing a broken video. Use a video editor or a
re-encoding tool on the individual segments if you need them joined. You can
list them with `procreepy --list` to see how many there are.

## Some segment numbers are missing

**Symptom** — a warning, and the video is still produced:

```text
level=WARN msg="segment numbers missing: 2 (the video would have gaps)"
```

**Cause** — the numbered sequence inside the archive has an **interior** hole
(a number missing between the first and last present). The available segments are
joined, so the timelapse jumps at that point. A sequence that merely starts above
1 — Procreate prunes the oldest segments as a recording grows — is complete and
does **not** trigger this warning.

**What to do** — accept the gap, or make it fatal with `--strict`, which turns
this into exit code `5` and writes nothing.

## Nothing was written and the output looks skipped

**Symptom**

```text
level=INFO msg="skipped, outputs already exist (use --force to overwrite)" input="input/Cat.procreate" ...
level=INFO msg="batch completed" converted=0 existed=3 no_video=0 failed=0
```

**Cause** — a folder run skips artworks whose complete output set is already
present. This is what makes re-running safe and cheap.

**What to do** — add `-f` to rebuild them:

```bash
procreepy -f input/ output/
```

## Refusing to write video data to a terminal

**Symptom** — exit code `2`:

```text
level=ERROR msg="refusing to write video data to a terminal; redirect stdout ..."
```

**Cause** — with a single file and no OUTPUT, the video goes to standard output,
and standard output is your terminal. Printing MP4 bytes there would scramble
the session.

**What to do** — name a destination, or redirect:

```bash
procreepy artwork.procreate artwork.mp4
procreepy artwork.procreate > artwork.mp4
```

## stdin is a terminal

**Symptom** — exit code `2`:

```text
level=ERROR msg="stdin is a terminal; pipe a .procreate file into it ..."
```

**Cause** — `-` as INPUT means "read the archive from standard input", but
nothing was piped in.

**What to do** — pipe a file, or pass the path instead:

```bash
cat artwork.procreate | procreepy - > artwork.mp4
procreepy artwork.procreate artwork.mp4
```

## `--psd` does nothing or is rejected

**Symptom** — exit code `2`:

```text
level=ERROR msg="--psd needs a directory INPUT; it writes into OUTPUT/psd/"
level=ERROR msg="--psd cannot be combined with --list or --verify"
```

**Cause** — PSD export is part of the folder workflow. It writes into
`OUTPUT/psd/`, which only exists for a directory run, and it has no meaning in
the inspection modes.

**What to do** — put the file in a folder and convert the folder:

```bash
mkdir input && mv artwork.procreate input/
procreepy --psd input/ output/
```

The PSD appears as `output/psd/artwork.psd`. See
[what it preserves](usage.md#export-a-psd).

## Not enough temporary disk space

**Symptom** — a write error mentioning the temporary directory, exit code `9`,
with a hint:

```text
... ; use --tmpdir (or $TMPDIR) on a bigger disk-backed directory
```

**Cause** — timelapses can be hundreds of megabytes, and reading from stdin
spools the whole archive to disk first. On Fedora and some other distributions
`/tmp` is RAM-backed and small.

**What to do** — point the temporary directory at a real disk with room:

```bash
procreepy --tmpdir /var/tmp input/ output/
```

In a container, `--tmpdir /data/tmp` puts them on the mounted volume instead of
the container's writable layer.

## The output file is open in another program

**Symptom** — on Windows, a write error naming the target file:

```text
Access is denied
```

**Cause** — procreepy publishes results by renaming a finished temporary file
over the target. Windows refuses that while another program holds the target
open — typically a media player still showing the previous MP4.

**What to do** — close the program holding the file and run the command again.
The existing file is left untouched, so nothing is lost.

## The output directory is not writable

**Symptom** — exit code `9`:

```text
level=ERROR msg="output directory is not writable: /path/to/dir"
```

**Cause** — the destination exists but the current user cannot create files in
it.

**What to do** — choose a directory you own, or fix the permissions. In a
container on a Linux host, add `--user "$(id -u):$(id -g)"` so the process runs
as you — see [container usage](usage.md#container-usage).

## The folder run finished with warnings or errors

**Symptom** — exit code `1` and a summary:

```text
level=INFO msg="batch completed" converted=2 existed=0 no_video=1 failed=1
```

**Cause** — one bad file does not stop a folder run. Files that failed are
counted in `failed`, and the exit code becomes `1` so a script can notice.
`no_video` counts artworks without a recorded timelapse, which are warnings
rather than failures.

**What to do** — read the per-file `level=ERROR` lines above the summary; each
names the input and the reason, and each reason has an entry on this page. The
artworks that converted are complete and usable — results are published per
artwork, so a failure elsewhere does not affect them.
