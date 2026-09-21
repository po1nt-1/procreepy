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
environment CI uses (`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=readonly`,
`CGO_ENABLED=0`), so `make` works from a bare shell. The raw `go` equivalents
stay valid.

| Task | Command |
|---|---|
| Definition of done | `make check` (fmt check, build, vet, full suite) |
| Format check | `make fmt-check` (fix with `make fmt`) |
| Build | `make build` |
| Vet | `make vet` |
| Full test suite | `make test` |
| Binary | `make bin` |
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

- Offline: `GOPROXY=off`, `GOFLAGS=-mod=readonly`, `GOTOOLCHAIN=local`.
  Never add third-party dependencies; `go.mod` is only module + go directive.
- `CGO_ENABLED=0` always; no C compiler in this environment, so `go test -race`
  is impossible — do not try.
- No `python3`; POSIX `sh` only (no bashisms in scripts).

## Layout

| Package | Responsibility |
|---|---|
| `cmd/procreepy` | entry point |
| `internal/cli` | argparse-compatible flag parsing, dispatch, slog logger, exit codes, help/version (exact text pinned by e2e) |
| `internal/batch` | directory mode: discovery, plan/collision suffixes, `ConvertDirectory` |
| `internal/video` | `Convert` orchestration, `--list`/`--verify` reports, I/O (spool, atomic `.partial` write), error types, per-OS shims `io_{linux,darwin,windows}.go` |
| `internal/procreate` | ZIP open/validate, segment scan (regex, numeric sort, gap/ambiguous/stray detection), `--split` rewrite |
| `internal/mp4` | custom MP4 box parser/writer: moov-first emit, stream copy, stco/co64 switch at 4 GiB |
| `internal/testkit` | builds synthetic MP4 segments and `.procreate` ZIPs for tests — no binary fixtures are committed |
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
  values with spaces/parens are quoted automatically.
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
- `batchStart`, `batchConverted`, `batchSkipped`, `batchNoVideo`,
  `batchFailed`, `batchDone`, `batchSlimmed` — batch records

## Docs

- `README.md` is the source of truth; translations live in
  `docs/README.{ar,de,es,fr,hi,id,pt,ru,zh-CN}.md`.
- Mechanical wording changes (sample output blocks, stderr descriptions,
  option tables) must be applied to **all** translations; free prose may be
  left to translators.
- Sample outputs in docs must be **authentic**: capture them from a real
  `procreepy` binary run, never invent them.

## CI (GitLab, `.gitlab-ci.yml`; GitHub mirror in `.github/workflows/`)

Stages: test (`make check` + `make coverage` -> Cobertura artifact),
build (`make release` x matrix: linux amd64/arm64/arm, windows amd64/arm64,
darwin amd64/arm64; normalized reproducible tarballs + SHA256SUMS), verify
(`make repro` on glibc vs musl, bit-for-bit proof). Do not weaken
`GOTOOLCHAIN=local`, `GOPROXY=off`, `CGO_ENABLED=0`, or the build flags —
the Makefile enforces the same flags locally.

## Workflow

- Never commit, push, or create tags unless explicitly asked.
- `docs/` may contain temporary working files; anything marked "temporary"
  gets deleted when its work is finished, not committed long-term.
