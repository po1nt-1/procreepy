# Development

How to build, test and release procreepy. For using the tool, see the
[README](../README.md).

`AGENTS.md` in the repository root is a terse rulebook for automated
contributors — invariants, constraints, non-negotiables. This page is the
human-facing counterpart; where they overlap, `AGENTS.md` is the stricter one.

## Prerequisites

- **Go** — the required version is in [`go.mod`](../go.mod) (currently
  `go 1.27`). No other toolchain is needed.
- **make** — the canonical entry point. Available on every supported platform;
  on minimal systems it is one package away.
- **tar** and **gzip** — only for `make release` and `make cross`.

The module has **zero third-party dependencies**, and the build is hermetic:
every `make` recipe exports `GOTOOLCHAIN=local`, `GOPROXY=off`,
`GOFLAGS=-mod=readonly -buildvcs=false` and `CGO_ENABLED=0`. That means no
network access during a build, no toolchain auto-upgrade, and no dependence on
the checkout's git state. If a command works under `make`, it works the same way
in CI.

The Makefile locates Go itself: `go` on `PATH` first, then
`/usr/local/go/bin/go`.

## Quick start

```bash
git clone https://gitlab.com/po1nt-1/procreepy.git
cd procreepy
make check
```

`make check` is `fmt-check` + `build` + `vet` + `test`. It is the gate CI runs;
if it passes locally it should pass there.

## Make targets

| Target | What it does |
|---|---|
| `make check` | the full gate: format check, build, vet, tests (the default target) |
| `make build` | compiles the module and drops a runnable `./procreepy` stamped `dev-<short sha>` |
| `make test` | `go test ./... -count=1` |
| `make vet` | `go vet ./...` |
| `make fmt` | `gofmt -w .` |
| `make fmt-check` | fails, listing files, if anything is unformatted |
| `make coverage` | runs the suite with coverage and writes `coverage.xml` plus `merged.out`, printing the total |
| `make release GOOS=… GOARCH=…` | one normalized release tarball into `dist/` |
| `make cross` | the whole seven-target release matrix into `dist/` |
| `make repro` | proves the build is bit-for-bit reproducible on this machine |
| `make clean` | removes `dist/`, built binaries, coverage and repro artifacts |

`make build` stamps the version from git (`dev-<short sha>`, or `dev-nogit`
outside a checkout), so `procreepy --version` always says where the binary came
from. Release tarballs carry the tag instead.

Without `make`, the raw equivalent of a build is
`go build ./... && go build -o procreepy ./cmd/procreepy` — that binary reports
`dev`.

## Repository layout

| Path | Responsibility |
|---|---|
| `cmd/procreepy` | entry point |
| `internal/cli` | flag parsing, dispatch, logger, exit codes, help and version text |
| `internal/batch` | directory mode: discovery, output planning, collision suffixes, packing projects into `procreate.zip` |
| `internal/video` | conversion orchestration, `--list`/`--verify` reports, spooling and atomic writes, per-OS shims |
| `internal/procreate` | ZIP open and validate, segment scan and numeric sort, the slimmed raw-ZIP copy |
| `internal/mp4` | the MP4 box parser and writer: moov-first emit, stream copy, `stco`/`co64` switch |
| `internal/procodec` | the two compressed containers Procreate uses for layer tiles (LZ4, LZO1X-1) |
| `internal/silica` | reads `Document.archive` (binary plist / NSKeyedArchiver) and the tiles it references; feeds `--psd` |
| `internal/psd` | writes the 8-bit RGBA PSD, including the embedded preview resource |
| `internal/testkit` | builds synthetic MP4 segments and `.procreate` archives for tests |
| `internal/fixture` | regression suite against a real `.procreate` corpus |
| `internal/e2e` | golden-output suite driving the compiled binary as a black box |
| `tools/cov2cobertura` | merges coverage profiles into a Cobertura report |
| `ci/windows-wine` | container image that runs the Windows suite under Wine |

## Tests

```bash
make test
```

No binary fixtures are committed. `internal/testkit` synthesises MP4 segments
and `.procreate` archives in memory, so the suite is hermetic and fast.

**`internal/e2e`** builds the real binary in `TestMain` and compares exit code,
stdout and stderr byte for byte. Any change to help text, a log line or a report
format will fail here — that is the point. Update the golden in
`internal/e2e/args_test.go` deliberately, never to make a test pass.

**`internal/fixture`** runs against a corpus of real `.procreate` files. Every
test in it skips unless you point it at one:

```bash
PROCREATE_FIXTURE_DIR=/path/to/corpus make test
# or a single zip of them
PROCREATE_FIXTURE_ZIP=/path/to/corpus.zip make test
```

Some tests need more temporary space than a small RAM-backed `/tmp` provides. If
you see `no space left on device` from `TestInterrupt` or
`TestConvertLargeArchive`, point the suite at a disk-backed directory:

```bash
TMPDIR=/var/tmp make test
```

`-race` is not required and is unavailable without a C compiler
(`CGO_ENABLED=0`), but it works if you have one.

A few tests are skipped by platform, deliberately: the interrupt test needs
POSIX signals, the unwritable-directory test needs Unix permission bits, and
the `/dev/null` test needs a Unix device path — none of the three exists on
Windows. Two more need a case-sensitive filesystem, because their premise is a
pair of names differing only in case; they probe the mount and skip where it
folds case, as Windows and the default macOS volume do.

## Coverage

```bash
make coverage
```

Writes `coverage.xml` (Cobertura, consumed by GitLab for merge-request
annotations) and `merged.out`, then prints the total.

`internal/e2e` must never run under `-coverprofile`: its `TestMain`
self-aggregates a repo-level `cover.e2e.out`, and a second writer would collide.
The target therefore runs e2e uninstrumented, profiles the other packages, and
merges the two profiles.

## Cross compilation and release artifacts

```bash
make release GOOS=linux GOARCH=arm64            # one target
make release GOOS=windows GOARCH=amd64 VERSION=v1.2.3
make cross                                      # all seven
```

Supported pairs: `linux/amd64`, `linux/arm64`, `linux/arm`, `windows/amd64`,
`windows/arm64`, `darwin/amd64`, `darwin/arm64`. The code is pure Go with per-OS
shims, so everything cross-compiles from any host with `CGO_ENABLED=0`.

`make release` produces `dist/procreepy-<version>-<os>-<arch>.tar.gz` and prints
its SHA-256. The tarball is normalized — sorted entries, fixed mtime and owner,
no gzip timestamp — so the same source yields a bit-identical archive. Note that
this is the **local** naming scheme; the archives attached to published releases
are produced by GoReleaser and named `procreepy_<version>_<os>_<arch>.tar.gz`
(`.zip` for Windows).

## Reproducibility

```bash
make repro
```

Builds twice with a cold build cache in between and checks the two hashes match,
writing `repro-<FLAVOR>.sha256`. CI runs this on two different base images
(glibc and musl) and compares the results, which proves the binary depends on
neither the base image nor the build cache. Both legs are given the same
`VERSION` explicitly, because the stamped version string would otherwise differ
between images and defeat the comparison.

## CI

`.gitlab-ci.yml` is the structural reference; `.github/workflows/ci.yml` mirrors
it job for job, with the platform differences noted in its header comment.

Stages:

| Stage | Jobs |
|---|---|
| `images` | `build:winci-image` — builds the Wine test image with kaniko, only when `ci/windows-wine/**` changes |
| `test` | `test` (`make check` + `make coverage`), SAST, `test:windows` |
| `build` | the seven-target release matrix |
| `verify` | `dist` (aggregate + `SHA256SUMS`), `repro:glibc`, `repro:musl`, `repro:compare` |
| `secret-detection` | the GitLab template |
| `release` | `release` (tagging) and `goreleaser` (publishing, tag pipelines only; needs `dist` and `repro:compare`, so nothing publishes unproven) |

Releases are cut from a `v*` tag. The GitLab tag pipeline publishes the GitLab
release, the archives and the container image; the GitHub workflow publishes the
GitHub release for the same tag with `--skip=ko`, so the container image is
pushed from one host only.

## The Windows test environment

Windows is the primary user platform, so the suite runs against a real
`windows/amd64` binary on both hosts:

- **GitHub** uses a hosted `windows-latest` runner, natively.
- **GitLab** has Linux runners only, so the same suite runs under Wine in the
  image built from [`ci/windows-wine/`](../ci/windows-wine/README.md). That
  directory documents the image, the two run scripts, when the job fires, and
  the registry cleanup policy it needs.

To run the Windows suite locally on a Linux host:

```bash
podman build -t procreepy-winci ci/windows-wine
podman run --rm -v "$PWD":/src:Z procreepy-winci                      # faithful
podman run --rm -v "$PWD":/src:Z procreepy-winci run-tests.hybrid.sh  # faster
```

## Documentation

- [`README.md`](../README.md) is the canonical user document. The nine
  translations in `docs/README.*.md` must mirror its structure, commands,
  examples and warnings.
- [`docs/usage.md`](usage.md), [`docs/troubleshooting.md`](troubleshooting.md),
  [`docs/how-it-works.md`](how-it-works.md) and this page are English-only and
  linked from every translation.
- Sample output in any document must be captured from a real run, never
  invented. The e2e golden tests are the authority on exact bytes.
- A user-visible change means updating the help text, the e2e golden, the README
  and all translations in the same commit.
