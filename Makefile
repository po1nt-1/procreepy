# procreepy build entry point: the canonical local commands CI runs.
#
# Every recipe inherits the pinned offline/toolchain environment below, so
# `make` behaves exactly like CI regardless of the ambient shell. The module
# has zero dependencies, hence GOPROXY=off is always safe to enforce.

export GOTOOLCHAIN = local
export GOPROXY = off
# -buildvcs=false keeps every make-driven go command (build, vet, test,
# coverage) independent of the checkout's git state: some CI checkouts
# expose a git that refuses go's VCS probes (exit 128), and the build must
# stay free of VCS state anyway. The version still comes from VER_FLAGS.
export GOFLAGS = -mod=readonly -buildvcs=false
export CGO_ENABLED = 0

# Locate the Go toolchain: `go` on PATH first, then the canonical
# /usr/local/go install. The directory is prepended to PATH so the bare
# go/gofmt invocations inside recipes resolve the same way.
GO := $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
export PATH := $(dir $(GO)):$(PATH)
PKG := ./cmd/procreepy

# Name of the release artifacts: an explicit VERSION= wins; locally the
# default is dev-<short sha>, the same rule CI uses off-tag.
VERSION ?= dev-$(shell git rev-parse --short HEAD 2>/dev/null || echo nogit)
# Artifact name suffix for `repro` (CI passes glibc / musl).
FLAVOR ?= glibc
# Computed at parse time per invocation, so each `cross` sub-make gets its
# own: windows binaries carry .exe.
EXE := $(if $(filter windows,$(GOOS)),.exe,)
BIN := procreepy-$(VERSION)-$(GOOS)-$(GOARCH)$(EXE)

.DEFAULT_GOAL := check
.PHONY: check fmt fmt-check build vet test coverage release cross repro clean

check: fmt-check build vet test

# The version stamped into the binary: $(VERSION) is dev-<short sha> on
# branches and the tag itself in a tag pipeline, so every build reports
# where it came from (`procreepy --version`).
VER_FLAGS = -trimpath -ldflags="-s -w -X procreepy/internal/cli.versionStr=$(VERSION)"

fmt:
	gofmt -w .

fmt-check:
	@UNFORMATTED=$$(gofmt -l .) && if [ -n "$$UNFORMATTED" ]; then printf 'not formatted (run "make fmt"):\n%s\n' "$$UNFORMATTED"; exit 1; fi

# Compiles the whole module and drops a runnable ./procreepy carrying the
# dev-<sha> version, the same way `release` versions its tarballs.
build:
	$(GO) build ./...
	$(GO) build $(VER_FLAGS) -o procreepy $(PKG)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -count=1

# Self-contained, mirrors the CI test stage. e2e must NEVER run under
# -coverprofile: its TestMain self-aggregates the repo-level cover.e2e.out
# and a second writer would collide. So e2e is re-run uninstrumented, the
# non-e2e packages are profiled (./internal/... scope keeps cmd/ measured
# exactly once, from the e2e binary), and the two profiles merge into the
# Cobertura report plus the merged text profile.
coverage:
	$(GO) test ./internal/e2e/ -count=1
	$(GO) test $$(go list ./internal/... | grep -v internal/e2e) -count=1 -coverprofile=xpkg.out -coverpkg=./internal/...
	$(GO) run ./tools/cov2cobertura -o coverage.xml -merged merged.out -strip procreepy/ xpkg.out cover.e2e.out
	@$(GO) tool cover -func merged.out | grep '^total'

# One normalized release artifact for a single GOOS/GOARCH pair — the same
# step CI runs seven times over the matrix:
#   make release GOOS=linux GOARCH=arm64 [VERSION=v1.2.3]
release:
	@test -n "$(GOOS)" && test -n "$(GOARCH)" || { echo 'usage: make release GOOS=<os> GOARCH=<arch> [VERSION=v1.2.3]' >&2; exit 2; }
	$(GO) build $(VER_FLAGS) -buildvcs=false -o "$(BIN)" $(PKG)
	mkdir -p dist
	mv "$(BIN)" dist/
	# Normalized tar+gzip (fixed mtime/owner, no gzip timestamp) keeps the
	# tarball bit-identical between runs of the same source.
	tar --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - -C dist "$(BIN)" | gzip -n > "dist/$(BIN).tar.gz"
	rm "dist/$(BIN)"
	sha256sum "dist/$(BIN).tar.gz"

# The whole CI release matrix into dist/.
cross:
	@for pair in linux/amd64 linux/arm64 linux/arm windows/amd64 windows/arm64 darwin/amd64 darwin/arm64; do \
	  GOOS="$${pair%%/*}" GOARCH="$${pair##*/}" $(MAKE) --no-print-directory release VERSION="$(VERSION)"; \
	done

# Reproducibility proof on one machine: two builds of the same source
# separated by a cold build cache must hash identically. FLAVOR names the
# resulting repro-<FLAVOR>.sha256 (CI: glibc vs musl images).
repro:
	$(GO) build $(VER_FLAGS) -buildvcs=false -o /tmp/pass1 $(PKG)
	rm -rf "$$(go env GOCACHE)"
	$(GO) build $(VER_FLAGS) -buildvcs=false -o /tmp/pass2 $(PKG)
	@H1="$$(sha256sum /tmp/pass1 | cut -d' ' -f1)" && H2="$$(sha256sum /tmp/pass2 | cut -d' ' -f1)" && test "$$H1" = "$$H2" && echo "$$H1" > "repro-$(FLAVOR).sha256"

clean:
	rm -rf dist procreepy procreepy.exe
	rm -f xpkg.out merged.out coverage.xml cover.e2e.out cover.base.out
	rm -f repro-*.sha256
	rm -f /tmp/pass1 /tmp/pass2
