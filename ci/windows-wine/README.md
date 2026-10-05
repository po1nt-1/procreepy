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

The job is `allow_failure: true` throughout, so neither the automatic nor the
manual run can break a pipeline. Building the image itself is gated harder
still: `build:winci-image` only fires when `ci/windows-wine/**` changes, so the
multi-gigabyte build happens on image edits alone, not per commit.

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
