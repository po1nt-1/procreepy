#!/usr/bin/env bash
# Mirror of the native `test:windows` CI job, but every `go` command is the
# Windows toolchain executed under Wine. Run from the module root.
#
# Local use with podman (rootless is fine):
#
#   podman build -t procreepy-winci ci/windows-wine
#   podman run --rm -v "$PWD":/src:Z procreepy-winci
#
# The :Z relabels the mount for SELinux hosts (Fedora/RHEL). Drop it on
# non-SELinux hosts if it causes trouble.

set -euo pipefail

# Windows Go on PATH for Wine. Inside the Windows process `go` resolves to
# go.exe here; everything else (compile.exe, link.exe, the built
# procreepy.exe, go tool covdata) is spawned as a child PE process that Wine
# also runs — which is exactly what internal/e2e's TestMain relies on.
export WINEPATH='Z:\opt\go-win\bin'

# Thin wrapper so the steps below read like the CI script.
go() { wine /opt/go-win/bin/go.exe "$@"; }

echo "== toolchain =="
go version
go env GOROOT GOOS GOARCH GOTOOLCHAIN GOPROXY CGO_ENABLED

# Mirror the CI job's hermetic pin (persisted into the Wine prefix's go env).
go env -w GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0

echo "== build =="
go build -buildvcs=false ./...

echo "== vet =="
go vet -buildvcs=false ./...

echo "== test =="
# -count=1 defeats the test cache, matching CI. The e2e package builds and
# runs procreepy.exe under Wine inside here.
go test -buildvcs=false ./... -count=1

echo "== done: windows suite passed under wine =="
