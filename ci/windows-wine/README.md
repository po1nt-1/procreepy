# Windows tests under Wine (Linux host)

The native `test:windows` CI job needs a `windows`-tagged runner. The project
has none, so the job sat pending until it failed with
`stuck_pending_no_matching_runners`. This image runs the **same** Windows suite
on an ordinary Linux runner by executing a real `windows/amd64` Go toolchain
under [Wine](https://www.winehq.org/) — no KVM, no Windows license.

It works here because the module is pure Go (`CGO_ENABLED=0`) with per-OS shims
(`io_windows.go`), so it cross-compiles cleanly, and because Wine runs child PE
processes: `internal/e2e` builds and execs `procreepy.exe` at test time, and
Wine handles that transparently.

## Contents

| file | purpose |
|------|---------|
| `Containerfile` | Debian + `wine64` + two Go 1.27.1 toolchains (windows under Wine, linux native) |
| `run-tests.sh` | faithful: `wine go.exe build/vet/test ./...` — mirrors the old job 1:1 |
| `run-tests.hybrid.sh` | faster: unit packages cross-compiled natively and run as `.exe` under Wine; only `internal/e2e` uses the full Windows-Go-under-Wine path |

## Local use

```sh
# from the module root (the dir with go.mod)
podman build -t procreepy-winci ci/windows-wine

# faithful run (exact mirror of the CI job):
podman run --rm -v "$PWD":/src:Z procreepy-winci

# faster hybrid run:
podman run --rm -v "$PWD":/src:Z procreepy-winci run-tests.hybrid.sh
```

(`:Z` relabels the mount for SELinux hosts; drop it elsewhere.)

## CI

`.gitlab-ci.yml`'s `test:windows` job runs `run-tests.sh` on a normal Linux
runner, against an image built automatically — no manual bootstrap:

| pipeline | `build:winci-image` pushes | `test:windows` pulls |
|---|---|---|
| MR touching `ci/windows-wine/**` | `winci:1.27.1-$CI_COMMIT_REF_SLUG` | the same branch-scoped tag |
| default branch, same files changed | `winci:1.27.1` | `winci:1.27.1` |
| anything else | *(skipped)* | `winci:1.27.1` |

The branch-scoped tag exists so an unreviewed MR can never clobber the tag
other pipelines consume, while still testing its own image. The build runs in
the `images` stage, before `test`, with kaniko — no Docker daemon or
privileged runner required.

### When `test:windows` runs

Compiling Go under Wine is slow, so the job is gated rather than run on every
commit:

| pipeline | behavior |
|---|---|
| release tag `v*` | always runs — this is the gate that matters |
| branch or MR touching `**/*.go`, `go.mod`, `Makefile`, `ci/windows-wine/**` | runs |
| anything else (docs, translations, unrelated CI edits) | optional **manual** job, never automatic |

**Changing the run scripts needs a merge request, not a branch push.** The job
calls `run-tests.sh` by bare name, so it runs the copy baked into the image
(`COPY … /usr/local/bin/` in the Containerfile), not the one in the checkout.
`build:winci-image` rebuilds that image on an MR touching `ci/windows-wine/**`
(branch-scoped tag, which `test:windows` then consumes) or on the default
branch (the stable tag) — a plain branch push matches neither. Push a script
change as a branch and `test:windows` will run the *old* script and fail for a
reason that is not in your diff.

When the job runs automatically it is **blocking**: a failure fails the
pipeline and `build` never starts, matching GitHub's native `test:windows`.
Only the manual fallback in the last row keeps `allow_failure: true` — a
blocking manual job would park the pipeline waiting for a click nobody owes it.

That is safe because the tests Wine cannot faithfully reproduce skip
themselves on `PROCREEPY_WINE=1`, which `run-tests.sh` exports (see
`testkit.UnderWine`), rather than having their assertions weakened for every
host. All of them pass on native Windows — GitHub's `test:windows` is the
authority — so each skip is a Wine gap, not a product defect:

| test | why Wine cannot run it |
|---|---|
| `TestStdin` (e2e) | a piped stdin is not reachable from the Windows process; the binary reports `cannot buffer <stdin>: Path not found.` |
| `TestDevFull` (e2e) | `/dev/full` is reachable through drive `Z:`, but Wine renders ENOSPC as `Disk full.` instead of `No space left on device` |
| `TestSameFile/symlink` (e2e) | Wine creates the host symlink through `Z:` but does not resolve it to the same file the way Windows does |
| `TestDiscoverSymlinks` (batch) | same symlink gap: the live link is not resolved, so `Discover` drops it as if it dangled |

A red Wine job therefore means a real Windows regression. Before adding
another entry, check the same test on GitHub first: if it fails there too, it
is a product defect and must be fixed, not skipped. Fix the environment rather
than skipping when the gap is the image's, not Wine's: the image sets
`LANG=C.UTF-8` because under the POSIX default Wine cannot map non-ASCII file
names to the host, which made `TestSpacesAndUnicodePaths` fail with
`File not found.`

Building the image itself is gated harder still: `build:winci-image` only fires
when `ci/windows-wine/**` changes, so the multi-gigabyte build happens on image
edits alone, not per commit.

### Registry housekeeping

Nothing deletes the per-MR `winci:1.27.1-<branch-slug>` tags, and each one is
roughly 3 GB, so the project needs a cleanup policy (Settings → Packages and
registries → Clean up image tags). One policy covers every image repository in
the project, so it has to be safe for the release images too:

| field | value |
|---|---|
| Run cleanup | every week |
| Keep the most recent | 1 tag per image name |
| Keep tags matching | `latest\|\d+\.\d+\.\d+` |
| Remove tags older than | 14 days |
| Remove tags matching | `.*` |

The keep pattern is what makes this safe in both repositories at once: it pins
`latest`, every released `procreepy:X.Y.Z` (so a user who pinned a version
never gets `manifest unknown`), and the stable `winci:1.27.1` the job pulls —
while the branch-scoped `winci:1.27.1-<slug>` tags do not match it (GitLab
anchors these patterns) and age out. A Go bump to `winci:1.28.0` stays covered
without editing the policy.

Keep "most recent" at 1, not the default: it preserves the newest tags
regardless of age, and at ~3 GB per `winci` image a larger number quietly
parks tens of gigabytes against the namespace storage quota. Released images
do not depend on that counter — the keep pattern holds them.

To push the image by hand anyway (e.g. to seed a private registry):

```sh
podman build -t "$CI_REGISTRY_IMAGE/winci:1.27.1" ci/windows-wine
podman push       "$CI_REGISTRY_IMAGE/winci:1.27.1"
```

## Caveats

- Wine is not a byte-perfect Windows; edge cases (console, ACLs, exotic paths)
  may diverge. The job is `allow_failure: true` until proven stable — drop that
  once it's trusted so Windows regressions actually fail the pipeline.
- `WINEDEBUG=-all` is required: Wine's `fixme:`/`err:` chatter would otherwise
  pollute the byte-exact stdout/stderr the e2e tests assert on.
- **Wine 9.0 is the floor.** Go's Windows runtime resolves `ProcessPrng` from
  `bcryptprimitives.dll` before `main`, and Wine gained that DLL in 9.0. On
  Wine 8 (Debian bookworm) every Go `.exe`, `go.exe` included, aborts with
  `fatal error: bcryptprimitives.dll not found`. Hence the trixie base and the
  version guard in the `Containerfile` — keep both if you rebase the image.
