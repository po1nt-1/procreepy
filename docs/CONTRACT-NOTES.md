# CONTRACT-NOTES (temporary working file — do not commit; delete in Phase 6)

Faithful transcription of the public contract promised by README.md, cross-checked
against the implementation. Used by Phases 1–5 to keep behavior identical (R3).
Status per item: OK = code matches; NOTE = deviation/inaccuracy to resolve.

## Invocation and positional args

| # | Promise | Code | Status |
|---|---------|------|--------|
| 1 | `procreepy [options] INPUT [OUTPUT]`, flags and positionals freely interleaved, `--` ends flags | cli.go parseArgs | OK |
| 2 | Unknown flag -> `unknown option` error; `--flag=value` unsupported for booleans (arg is "ignored") | parseArgs `--list=1` -> "unknown option ... (booleans take no value)" | OK |
| 3 | More than two positionals -> "at most two positional arguments (INPUT and OUTPUT)" | parseArgs | OK |
| 4 | `-` = stdin; second arg `-` = stdout | ResolveInput / ResolveOutput | OK |
| 5 | OUTPUT omitted -> stdout | ResolveOutput | OK |
| 6 | OUTPUT existing dir (or trailing slash) -> `<stem>.mp4` inside it, stem = INPUT name minus last extension (keeps `.procreate` when no other suffix) | ResolveOutput.isDir + deriveName | OK |
| 7 | `procreepy input/` (dir, no second arg) -> batch into `output/timelaps/` | ResolveOutput DirDefault | OK |
| 8 | `procreepy input/ outdir/` -> batch into outdir | ResolveOutput DirExplicit | OK |
| 9 | `procreepy input.procreate /dev/shm/out.mp4` etc. = explicit file path | ResolveOutput FileExplicit | OK |
| 10 | Dir INPUT with OUTPUT `-` -> error (cannot stream several videos to stdout) | batch.ConvertDirectory `IsStdout` -> UsageError | OK |
| 11 | Dir INPUT with explicit file OUTPUT (not dir) -> error "output path exists and is not a directory" (when it exists as a file) / treated as dir path otherwise | batch: `out != ""` + MkdirAll | OK |

## Batch mode

| # | Promise | Code | Status |
|---|---------|------|--------|
| 12 | Default output `output/timelaps/` relative to CWD | batch.DefaultOutputDir | OK |
| 13 | One video per input file, same name + `.mp4` | PlanOutputs deriveName | OK |
| 14 | `-r` mirrors sub-folder structure under output | PlanOutputs rel | OK |
| 15 | Name collisions on the flattened plan get suffixes `-2`, `-3`, ... preserving final extension | PlanOutputs | OK |
| 16 | Without `-r`, only files directly under the given dir | batch.Discover | OK |
| 17 | With `-r`, files of each dir first, then subdirs (sorted, depth-first); dot-prefixed entries ignored | batch.Discover walk | OK |
| 18 | Hidden dot-files everywhere skipped (`._x.procreate`) | isProcreate | OK |
| 19 | Existing outputs skipped with a note; `--force`/`-f` overwrites (partial-file replacement) | ConvertDirectory stat | OK |
| 20 | One bad file does not stop the run; per-file `[i/N]` progress; final `summary: ...` line; exit 1 if anything failed | ConvertDirectory | OK |
| 21 | No segments in a batch member -> `warning:` + "without timelapse" counter, not a failure | ConvertDirectory NoSegmentsError branch | OK |
| 22 | Originals never modified | Open is read-only | OK |
| 23 | No files found -> InputError "no .procreate files found in DIR" (+"(use -r to look in sub-directories)") -> exit 3 | batch.Discover/ConvertDirectory | OK |

## --split

| # | Promise | Code | Status |
|---|---------|------|--------|
| 24 | Writes `<name>.procreepy.procreate` next to the produced MP4 | SplitTimelapse slimPath | OK |
| 25 | Requires a file (or dir) OUTPUT, not stdout | Convert: `cfg.Split && out.Kind != OutFile` -> UsageError | OK |
| 26 | Non-video members copied byte-identically (name, order, method, timestamps) | rewriteZip via zr.File order + CopyFileHeader | OK |
| 27 | Works in batch: slim copy next to each X.mp4 | Convert (cfg.Split) in batch loop | OK |
| 28 | Cannot combine with --list/--verify | dispatch check -> UsageError | OK |

## --list / --verify

| # | Promise | Code | Status |
|---|---------|------|--------|
| 29 | Report on stdout; diagnostics on stderr | dispatch: PrintReport to stdout; Log to stderr | OK |
| 30 | `--list` report: `input: <label>\nsegments: N\n\n` + right-aligned-number table `num  path` (table only when N>0) | List() | OK |
| 31 | `--list` reads only the ZIP directory (no media parsing) | List -> Segments only | OK |
| 32 | `--verify` parses every segment (CRC checked), per-line `N ok  <streams> <dur> <path>` or `N FAIL  <reason>`, then `verify: ok, N segment(s), ~X.X s of video` (no trailing newline) | Verify() | OK |
| 33 | Any failing segment -> exit 5 (BadSegmentError "verify failed: ...") | Verify + exit-code map | OK |
| 34 | Accept directories; walk all members; honor `-r` | DiagnoseDirectory | OK |
| 35 | `--list`/`--verify` with OUTPUT -> UsageError "take a single INPUT and no OUTPUT" | dispatch | OK |
| 36 | `--list` with zero segments still prints `input:..\nsegments: 0\n` and exits 4 | List + NoSegmentsError | OK |

## Segment ordering

| # | Promise | Code | Status |
|---|---------|------|--------|
| 37 | Members matched case-insensitively by `video/segments/segment-<digits>.mp4` | segmentRE `(?i)` | OK |
| 38 | Sorted numerically, not lexicographically | scan.sort.Slice by Number | OK |
| 39 | Gaps -> `warning: segment numbers missing: a-b, c (the video would have gaps)`, still proceeds | convert.go missingRanges | OK |
| 40 | Same number twice -> BadSegmentError "ambiguous segment numbering" -> exit 5 | Scan | OK |
| 41 | .mp4 under segments/ not matching pattern -> warning "ignoring ... name does not match segment-<number>.mp4", excluded | Scan | OK |
| 42 | Files present but none numbered -> BadSegmentError "video/segments has N .mp4 file(s), but none is named segment-<number>.mp4..." -> exit 5 | Scan | OK |
| 43 | `video/segments/` absent/empty -> NoSegmentsError "no video/segments in this archive (time-lapse recording was probably turned off for this artwork)" -> exit 4 | Segments | OK |

## Conversion mechanics

| # | Promise | Code | Status |
|---|---------|------|--------|
| 44 | stdin / non-seekable input spooled to a temp file first | spool | OK |
| 45 | ZIP validity checked before reading (invalid -> InputError exit 3) | Open | OK |
| 46 | Segments parsed straight from the ZIP (no full extraction) | ParseSegment(r, hdr, strict) | OK |
| 47 | Compatibility: same handler/fourcc/codec config bytes/size/pixfmt/rate/channels/timescale/matrix/sync-presence per stream, in the same order; else IncompatibleError exit 7 with per-segment listing (max 5 lines + "and N more") | trackSig/same/Merge | OK |
| 48 | Each segment must have exactly one mdat (else BadSegmentError exit 5) and a video stream | Merge + Convert checks | OK |
| 49 | `--strict`: missing segments become hard errors (exit 5) instead of warnings | Config.Strict -> Errorf in convert.go | OK |
| 50 | `--reencode` accepted (reserved no-op) | cli flag + doc | OK |
| 51 | Output is moov-first: ftyp + moov + per-segment mdat, 64-bit stco/co64 switch when offsets >= 4 GiB | Merge.Finalize/Emit | OK |
| 52 | Atomic file output: temp `<name>.procreepy-<pid>-<8hex>.partial` in the target dir, fsync-ish (Sync) before Rename | writeOut | OK |
| 53 | Temp dirs/files removed on success, error, Ctrl-C, SIGTERM, panic | defers + panic recover in cli.run | OK |
| 54 | Nothing but the report goes to stdout | ui.Log -> Out (stderr default) | OK |
| 55 | Writing video to a TTY stdout refused -> UsageError exit 2 "refusing to write video data to a terminal..." | prepare() isTerminal | OK |
| 56 | Terminal stdin refused -> UsageError exit 2 "stdin is a terminal; pipe a .procreate file into it..." | resolveInput | OK |
| 57 | Temp storage selection: `--tmpdir` -> `$TMPDIR` -> `/var/tmp` (-> Go default) | input.tempBase | NOTE (see D2) |
| 58 | Insufficient disk space while spooling -> InputError "no space left while buffering ...; use --tmpdir (or $TMPDIR) on a bigger disk-backed directory" | spool ENOSPC branch | NOTE (see D2) |
| 59 | OUTPUT resolving to same inode as INPUT -> UsageError "OUTPUT is the same file as INPUT" | prepare() stat | OK |
| 60 | OUTPUT to existing non-regular file (device/FIFO) -> streamed directly, no rename | prepare OutDevice | OK |
| 61 | Output dir missing -> WriteError "output directory does not exist: D" (exit 9); not writable -> WriteError "output directory is not writable" | prepare probe | OK |
| 62 | Deriving a name from stdin/pipe OUTPUT -> UsageError "cannot derive an output file name from stdin or a pipe; pass a file name as OUTPUT" | prepare | OK |
| 63 | Broken pipe on stdout -> WriteError "failed to write to stdout: broken pipe" exit 9, then SIGPIPE ignored + stdout stubbed | writeOut/devnullStdout | OK |

## Messages and formats (pinned by Phase 1 e2e)

- version: `procreepy 0.1.0` on stdout.
- help: exact helpText on stdout; `--help` wins over errors.
- usage error: `usage: procreepy [options] INPUT [OUTPUT]` then `procreepy: error: <msg>` (stderr, exit 2).
- log prefixes: `info: `, `warning: `, `error: `; `-q` silences info only.
- chatty single-file (stderr): `<input>: N segment(s)`; `joining segments (stream copy)`; `done: <name> (~X.X s of video)`; with split: `slimmed archive -> <path> (removed N video file(s), ~X of video)`. Batch uses chatty=false.
- batch per-file: `info: [i/N] <src> -> <dst>`; skip: `info: [i/N] <dst>: skipped, <dst> already exists (use --force to overwrite)`; warn no-timelapse: `warning: [i/N] <src>: no timelapse video inside, skipped`; error dedups src already inside the message: `error: [i/N] <src>: <msg>` else appends.
- batch summary: `info: summary: N converted[, M already existed][, K without timelapse][, F FAILED]`.
- humanBytes: `B` under 1 KiB; one decimal below 100 units (`3.4 MiB`); integral above (`2 GiB`).
- incompatible: `error: segments have different stream parameters, so they cannot be joined with stream copy (-c copy):` + `  <name>: <streams>` lines.
- interrupted: `error: interrupted` (stderr, exit 130).
- env: `PROCREATE_VIDEO_DEBUG=1` re-throws panics (traceback); `TMPDIR` honored.

## Exit codes

0 ok (incl. batch-with-only-skips/without-timelapse) | 1 generic (incl. any batch failure, panic) | 2 usage | 3 input | 4 no segments | 5 bad segment (incl. verify fail, strict gaps) | 6 unused/reserved (never returned) | 7 incompatible | 8 unused/reserved (never returned) | 9 write | 130 interrupted (Ctrl-C/SIGTERM/cancel).

## Discrepancy list (README vs code)

- **D1** `--tmpdir` help text says "where to extract segments"; nothing is ever extracted (segments are parsed from the in-memory ZIP). The dir actually hosts the stdin spool temp dir. Wording fix candidate for Phase 5.
- **D2** README: "Free space is checked **before** unpacking" — implementation detects ENOSPC **during** the spool copy (mid-write) and reports it then. Also README omits the final fallback to the Go/system default when neither TMPDIR nor /var/tmp is usable. Docs-only wording fix candidate.
- **D3** README batch sample output shows the file name in the `error: [3/4] input/Corrupt file.procreate: ...` slot in a way that differs slightly from actual dedup format (actual: `error: [3/4] input/Corrupt file.procreate: input is not a valid ZIP archive: ...` is collapsed to `error: [3/4] input is not a valid ZIP archive: input/Corrupt file.procreate (...)` because the message already contains the name). Cosmetic; sample uses `...`.
- **D4** LICENSE file is Apache-2.0 (201 lines) — matches README claim; the earlier AGPL file in git history was replaced. No action.
- **D5** `docs/README.*.md` translations (ar/de/es/fr/hi/id/pt/ru/zh-CN) exist; any wording change made in Phase 5 should be noted for translators (translations stay as-is unless customer asks).
