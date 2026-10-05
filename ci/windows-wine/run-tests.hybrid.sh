#!/usr/bin/env bash
# Hybrid runner: faster than run-tests.sh, same coverage of behavior.
#
#   * build + vet              -> native Linux Go, GOOS=windows (seconds)
#   * every non-e2e package    -> cross-compiled on the host with
#                                 `go test -c`, then the .exe is run under Wine
#   * internal/e2e             -> `wine go.exe test` (its TestMain builds and
#                                 execs procreepy.exe at run time, so it needs
#                                 the Windows toolchain inside Wine)
#
# The slow part of Go-under-Wine is compilation; here all compiling for the
# unit packages happens natively, and only their short test runs cross into
# Wine. Run from the module root (dir with go.mod).
#
#   podman run --rm -v "$PWD":/src:Z procreepy-winci run-tests.hybrid.sh

set -euo pipefail

# Same reason as run-tests.sh: the inherited working directory may be the
# image's /src rather than the checkout, and every `go` below addresses
# packages relative to the module root.
cd "${CI_PROJECT_DIR:-$PWD}"
if [ ! -f go.mod ]; then
  echo "run-tests.hybrid.sh: no go.mod in $PWD (run from the module root)" >&2
  exit 2
fi

readonly WIN_GO='/opt/go-win/bin/go.exe'
readonly E2E_PKG='procreepy/internal/e2e'

# Hermetic parity with the Makefile / CI test stage.
export GOTOOLCHAIN=local
export GOPROXY=off
export GOFLAGS=-mod=readonly
export CGO_ENABLED=0

# Host toolchain targets Windows for the cross-compile and the build/vet gate.
export GOOS=windows
export GOARCH=amd64

# Windows Go on PATH inside Wine, so e2e's runtime `go build`/`go tool covdata`
# resolve go.exe, and child PE processes (compiler, linker, procreepy.exe) run
# under the same prefix.
export WINEPATH='Z:\opt\go-win\bin'

workdir="$(mktemp -d)"
trap 'rm -rf "${workdir}"' EXIT

echo "== toolchains =="
go version                 # native linux host go (/opt/go-linux)
wine "${WIN_GO}" version   # windows go under wine

echo "== build (native, GOOS=windows) =="
go build -buildvcs=false ./...

echo "== vet (native, GOOS=windows) =="
go vet -buildvcs=false ./...

echo "== unit packages: cross-compile on host, run .exe under wine =="
failed=()
while IFS=$'\t' read -r pkg dir; do
  [[ "${pkg}" == "${E2E_PKG}" ]] && continue

  exe="${workdir}/$(printf '%s' "${pkg}" | tr '/.' '__').exe"
  go test -c -buildvcs=false -o "${exe}" "${pkg}"

  # `go test -c` writes nothing for a package with no test files; skip those.
  [[ -f "${exe}" ]] || { printf '   (no tests) %s\n' "${pkg}"; continue; }

  # Run with cwd = the package source dir so relative testdata/ paths resolve
  # exactly as `go test` would arrange them.
  printf '   run %s\n' "${pkg}"
  if ! ( cd "${dir}" && wine "${exe}" -test.count=1 ); then
    failed+=("${pkg}")
  fi
done < <(go list -f '{{.ImportPath}}{{"\t"}}{{.Dir}}' ./...)

echo "== internal/e2e: full build+run under wine =="
if ! wine "${WIN_GO}" test -buildvcs=false "./${E2E_PKG#procreepy/}" -count=1; then
  failed+=("${E2E_PKG}")
fi

if (( ${#failed[@]} > 0 )); then
  printf '\nFAILED packages:\n'
  printf '  %s\n' "${failed[@]}"
  exit 1
fi

echo "== done: windows suite passed under wine (hybrid) =="
