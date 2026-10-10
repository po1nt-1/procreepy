# AGENTS.md — procreepy

procreepy extracts the archived timelapse from `.procreate` files (ZIP with
`video/segments/segment-N.mp4`) and joins the segments into one moov-first MP4
by stream copy. Pure Go, zero dependencies, no ffmpeg.

## Commands

```sh
export PATH=$PATH:/usr/local/go/bin   # Go 1.27.1 lives in /usr/local/go
export CGO_ENABLED=0                  # baseline for raw go commands; make sets it itself
```

All commands are make targets; the Makefile forces the same hermetic
environment CI uses (`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly -buildvcs=false`,
`CGO_ENABLED=0`), so `make` works from a bare shell. The raw `go` equivalents
stay valid.

| Task | Command |
|---|---|
| Definition of done | `make check` (fmt check, build, vet, full suite) |
| Format check | `make fmt-check` (fix with `make fmt`) |
| Build | `make build` (whole module + `./procreepy` stamped `dev-<short sha>`, `dev-nogit` outside a Git checkout; `procreepy --version` reports the token) |
| Vet | `make vet` |
| Full test suite | `make test` |
| Cross-compile | `make release GOOS=linux\|windows\|darwin GOARCH=amd64\|arm64\|arm` (linux arm: default GOARM=7); `make cross` for the whole CI matrix |
| Release flags | baked into `make release` / `make repro`: `-trimpath -buildvcs=false -ldflags="-s -w"` |
| Reproducibility | `make repro [FLAVOR=...]` -> `repro-<FLAVOR>.sha256` |
| Clean | `make clean` |

Definition of done (run all, all green, before reporting completion):
`make check`, plus `make cross` for the CI matrix.

Coverage (mirrors CI; the two writers must not collide): `make coverage`.
e2e self-aggregates `cover.e2e.out` and must NEVER run under
`-coverprofile`, so the target re-runs e2e uninstrumented, profiles
`./internal/...` minus e2e into `xpkg.out` (scoped so cmd/ is counted
exactly once, from the e2e binary), then merges both profiles into
`coverage.xml` (Cobertura) + `merged.out` and prints the total.

## Environment constraints

- Offline: `GOPROXY=off`, `GOFLAGS=-mod=readonly -buildvcs=false`,
  `GOTOOLCHAIN=local`. Never add third-party dependencies; `go.mod` is only
  module + go directive. Make-driven builds carry no VCS stamping (some CI
  checkouts expose a git go cannot probe); the version comes from the `-X`
  injection instead.
- `CGO_ENABLED=0` always; no C compiler in this environment, so `go test -race`
  is impossible — do not try.
- No `python3`; POSIX `sh` only (no bashisms in scripts).

## Layout

| Package | Responsibility |
|---|---|
| `cmd/procreepy` | entry point |
| `internal/cli` | argparse-compatible flag parsing, dispatch, slog logger, exit codes, help/version (exact text pinned by e2e) |
| `internal/batch` | directory mode: discovery, plan/collision suffixes, `ConvertDirectory`, packing projects into `procreate.zip` (`--no-zip` opts out; resume reads done-ness from the archive) |
| `internal/video` | `Convert` orchestration, `--list`/`--verify` reports, I/O (spool, atomic `.partial` write), error types, per-OS shims `io_{linux,darwin,windows}.go` |
| `internal/procreate` | ZIP open/validate, segment scan (regex, numeric sort, gap/ambiguous/stray detection), slimmed copy via raw-ZIP pass-through (`WriteSlimmed` on `CreateRaw`/`OpenRaw`, every kept member bit-identical) |
| `internal/mp4` | custom MP4 box parser/writer: moov-first emit, stream copy, stco/co64 switch at 4 GiB |
| `internal/procodec` | decodes the two compressed containers Procreate uses for layer tiles: Apple compression-lib LZ4 (`.lz4`) and bare LZO1X-1 (`.chunk`) |
| `internal/silica` | reads `Document.archive` (Apple binary plist / NSKeyedArchiver) and the layer tiles it references — feeds `--psd` |
| `internal/psd` | writes 8-bit RGBA PSDs: layer tree (groups+order), names, visibility/opacity/blend/bounds/locks, PackBits, DPI+ICC, merged composite (cached), embedded preview resource 1036; fidelity limits documented in `docs/usage.md` |
| `internal/testkit` | builds synthetic MP4 segments and `.procreate` ZIPs for tests — no binary fixtures are committed |
| `internal/fixture` | regression suite against a real `.procreate` corpus; every test skips unless `PROCREATE_FIXTURE_DIR` or `PROCREATE_FIXTURE_ZIP` is set |
| `internal/e2e` | golden-output suite: builds the real (instrumented) binary in `TestMain` and compares exit code + stdout + stderr byte-for-byte |
| `tools/cov2cobertura` | merges coverage profiles into a Cobertura report (GitLab MR diff annotations) |
| `Makefile` | build entry point: hermetic env + the canonical commands CI runs |

## Non-negotiable invariants

- **Public behavior is pinned by e2e.** Every user-visible byte (error
  messages, reports, help text, log lines, exit codes) has an expectation in
  `internal/e2e`. If you change any user-visible string, update the matching
  expectation in the same change and re-run the suite.
- **stdout purity.** stdout carries only the video or the `--list`/`--verify`
  report. All diagnostics go to stderr. Refuse to write video to a TTY stdout.
- **Logging = stdlib `log/slog` TextHandler on stderr** (`cli.newLogger`):
  no timestamp (dropped via `ReplaceAttr`, output must stay deterministic),
  `-q` raises the level to Warn. Durations render via `%g` (`6.0` -> `6`);
  values with spaces/parens are quoted automatically. On an interactive TTY
  with `NO_COLOR` unset, a `colorHandler` wrap puts SGR escapes around the
  `WARN` (yellow) and `ERROR` (bold red) level tokens only; `INFO` stays
  plain, and non-TTY output (pipes, redirects, CI, tests) is byte-identical
  plain — there is deliberately no `--color` flag.
  Usage/argparse errors stay on `fmt` (`procreepy: error: ...`, exit 2) —
  they are not log records.
- **Layering.** Domain packages (`mp4`, `procreate`) never log: they return
  typed errors and report warnings through `warnFn` callbacks; logging happens
  in `video`/`batch`/`cli`. No `slog.SetDefault`, no file loggers, no JSON
  output, no new CLI flags without a contract reason.
- **Exit codes are contract:** 0 ok, 1 generic/batch failure, 2 usage, 3
  input, 4 no segments, 5 bad segment, 6 & 8 reserved (never returned), 7
  incompatible, 9 write, 130 interrupted.
- **Determinism / reproducibility.** CI proves glibc and musl builds of the
  same source are bit-identical (`repro:*` jobs). Keep builds free of local
  paths, VCS state, wall-clock time, and map-iteration nondeterminism in
  output. Temp file names embed pid+random — that is fine, it is not in the
  output.
- **Safety.** Inputs are opened read-only (originals never modified); file
  output is atomic (`.partial` + rename); all temp dirs/files are removed on
  success, error, Ctrl-C, SIGTERM, and panic.
- **Tests generate their own fixtures** via `internal/testkit`; never commit
  binary fixtures. Fuzz and scale tests guard the MP4 parser — keep them
  passing.

## e2e harness

Expectations are built from helpers in `internal/e2e/e2e_test.go` — use them,
do not hand-roll expected strings:

- `run(t, dir, stdin, args...)` / `check(t, r, code, stdout, stderr)` — driver
- `usageErr(msg)`, `actionErr(msg)` — stderr with usage banner / plain error
- `slogLine(level, msg, kv...)` — one slog record (byte-exact renderer)
- `chattyBlock(input, count, output, secs)` — single-file verbose trio
- `gapWarn(missing)`, `strayWarn(member)` — scan warnings
- `batchStart(files, in, tl, proj)`, `batchConverted(src, tl, proj, removed,
  size)`, `batchSkipped(src, tl, proj)`, `batchIncomplete(src)`,
  `batchNoVideo(src)`, `batchFailed(src, err)`,
  `batchDone(conv, existed, noVideo, failed)` — batch records
- fixture inputs: `segN`, `stdArchiveEntries`, `stdInput`, `stdThree`,
  `writeArchive`, `writeArchiveCorrupted`, `expectedMP4`
- output inspection: `readAll`, `eqBytes`, `readZipMembers`,
  `assertProjectContents`, `sortedNames`, `assertNoPartial`, `humanBytes`

## Docs

- `README.md` is the canonical user document: a task-oriented router (what it
  is, who it is for, install per platform, the five main tasks, file-safety
  behavior, pointers onward). It must stay usable without reading any
  implementation detail.
- Depth lives in English-only companions, linked from the README and from every
  translation: `docs/usage.md` (full CLI reference, exit codes, batch
  semantics, PSD fidelity, container usage), `docs/troubleshooting.md`
  (symptom/cause/fix), `docs/how-it-works.md` (internals, format notes),
  `docs/development.md` (build, test, release, CI).
- Translations live in `docs/README.{ar,de,es,fr,hi,id,pt,ru,zh-CN}.md` and
  mirror `README.md` section for section, with the same commands, the same
  output examples and the same warnings.
- Mechanical wording changes (sample output blocks, stderr descriptions,
  option tables) must be applied to **all** translations; free prose may be
  left to translators.
- Do not move user-facing essentials out of `README.md` into the companions,
  and do not pull implementation detail back into it.
- Sample outputs in docs must be **authentic**: capture them from a real
  `procreepy` binary run, never invent them.
- Anything worth documenting (behavior, caveats, platform quirks) belongs in
  `README.md` **and in all translations** — with a high-quality translation
  matched to the established style and register of each file, not a word-for-word
  rendering.

## CI (GitLab `.gitlab-ci.yml` is the structural reference; GitHub
`.github/workflows/ci.yml` mirrors it job for job)

Stages: images (`build:winci-image` — kaniko-built Wine test image, fires only
on `ci/windows-wine/**` changes), test (`make check` + `make coverage` ->
Cobertura report, SAST, plus a Windows smoke job — build, vet and test with the
same hermetic flags against a real windows/amd64 binary; GitHub `test:windows`
runs natively on `windows-latest`, GitLab `test:windows` runs the same suite
under Wine because the project has Linux runners only — see
`ci/windows-wine/README.md`; the Wine job is gated to Windows-relevant changes
but blocking when it runs, as is the native GitHub job, which runs in every
pipeline — both hard-gate `build`. Tests whose premise Wine cannot reproduce
skip on `PROCREEPY_WINE` (see `testkit.UnderWine`) instead of having
their assertions weakened for every host),
build (`make release` x matrix: linux amd64/arm64/arm, windows amd64/arm64,
darwin amd64/arm64; normalized reproducible tarballs), verify (SHA256SUMS
manifest + `make repro` on glibc vs musl, bit-for-bit proof — both legs must be
given the same `VERSION`, or the stamp alone makes them differ), release
(GitLab-only semantic-release tagger on main; `goreleaser release` per tag
on both hosts, gated on `dist` and `repro:compare` so a tag cannot publish
binaries whose matrix build or glibc==musl proof never ran).
GitLab's `workflow:` gives a branch with an open MR a single MR pipeline that
runs the whole gate; a new job that must run everywhere extends
`.every-pipeline`, since a job without `rules:` never joins an MR pipeline
(the SAST/secret templates need `AST_ENABLE_MR_PIPELINES`, set globally).
GitHub encodes the stage order with `needs:` (no stages);
its SAST is CodeQL, coverage ships as an artifact, and there is no native
secret-detection job. The GitLab-only tagger is the single source of
version tags feeding both hosts, so it must not be mirrored. Keep the two
pipelines in sync: same job names, same commands. Do not weaken
`GOTOOLCHAIN=local`, `GOPROXY=off`, `CGO_ENABLED=0`, or the build flags —
the Makefile enforces the same flags locally.

## Workflow

- Never commit, push, or create tags unless explicitly asked.
- `docs/` may contain temporary working files; anything marked "temporary"
  gets deleted when its work is finished, not committed long-term.

### Commit messages

Short. A one-line subject is the default: imperative, no trailing period, under
~70 characters. Add a body only when the *why* would otherwise be lost — a
constraint that looks arbitrary, the defect being fixed, a decision someone
would otherwise undo. Two or three lines is already a long body here. Never
restate what the diff shows, and never list the files touched.

One topic per commit. If the subject needs an "and", it is two commits.

### Versioning — do not trip semantic-release

`.releaserc.json` runs semantic-release on `main` (GitLab only) with the
`conventionalcommits` preset, and that job is the single source of version tags
for both hosts. Under that preset a `feat:` subject cuts a minor tag, `fix:` and
`perf:` cut a patch tag, and a `!` marker or a `BREAKING CHANGE:` footer cuts a
major one — automatically, on push — and the new tag immediately starts the
`goreleaser` publish pipeline.

So write **prose subjects, never Conventional Commit prefixes**: "Stamp the
image with the commit timestamp", not "fix: image creation time". Releases are
tagged by hand (`v0.3.0` was an annotated tag pushed by the maintainer). A
prefix slipped into a routine commit publishes a release nobody asked for.

The cost is that prose subjects fall into the "Other" group of the goreleaser
changelog, which groups on those same prefixes. That is accepted. Use a prefix
only when a release is genuinely intended.
