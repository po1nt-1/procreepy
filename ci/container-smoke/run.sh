#!/usr/bin/env bash
# Release smoke test for the published container image.
#
# This cannot run in the pipeline: the image is built daemonless with ko (and
# the Windows CI image with kaniko), and neither the shared runners nor those
# images can start a container engine. So it is a manual gate, run on a Linux
# box that has docker and/or podman, after the goreleaser job has pushed a tag.
#
#   ci/container-smoke/run.sh                 # both engines if present, :latest
#   ci/container-smoke/run.sh 0.3.0           # a specific tag (no `v` prefix)
#   ENGINES=podman ci/container-smoke/run.sh  # just one engine
#
# What it checks, per engine:
#   1. one file     -> MP4
#   2. a folder     -> mp4/ and procreate.zip (with the projects inside)
#   2b. --psd, only when PROCREATE_FIXTURE_DIR points at real artworks: the
#      synthetic fixtures carry no silica document, so a PSD export cannot
#      succeed on them. Mounted read-only.
#   3. stdin        -> stdout
#   4. every artifact is owned by the invoking user, not by uid 65532 or a
#      shifted subuid
#   5. no "permission denied" anywhere in the captured output
#
# And once, engine-independently:
#   6. the image creation date is a real date, not 1970-01-01 (the ko default
#      that `creation_time` in .goreleaser.yaml overrides).
#
# Run from the module root: the fixtures come from the repo's own generator.

set -euo pipefail

TAG="${1:-latest}"
IMAGE="${IMAGE:-registry.gitlab.com/po1nt-1/procreepy:$TAG}"

cd "${CI_PROJECT_DIR:-$PWD}"
if [ ! -f go.mod ]; then
  echo "run.sh: no go.mod in $PWD (run from the module root)" >&2
  exit 2
fi

# Pick the engines. Default to whatever is installed; an explicit ENGINES wins
# so a release check can insist on covering podman.
if [ -n "${ENGINES:-}" ]; then
  read -r -a engines <<<"$ENGINES"
else
  engines=()
  for e in docker podman; do
    command -v "$e" >/dev/null 2>&1 && engines+=("$e")
  done
fi
if [ "${#engines[@]}" -eq 0 ]; then
  echo "run.sh: neither docker nor podman found on PATH" >&2
  exit 2
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

note() { printf '\n== %s ==\n' "$*"; }
ok()   { printf '   ok   %s\n' "$*"; }
bad()  { printf '   FAIL %s\n' "$*" >&2; failures=$((failures + 1)); }

# Engine-specific run flags. These differ on purpose — the two engines are not
# interchangeable flag-for-flag on a rootless Linux host, which is the whole
# reason this script exists. Keep in sync with docs/usage.md#container-usage.
#
#   docker: --user is enough; the daemon runs as root and the uid maps straight
#           through to the host.
#   podman: rootless needs --userns=keep-id so the container uid is *your* uid
#           instead of a shifted subuid, and :Z to relabel the mount for
#           SELinux. Without either one the run fails on the output directory.
mount_flags() {
  case "$1" in
    podman) printf -- '-v\n%s:/data:Z\n-w\n/data\n' "$2" ;;
    *)      printf -- '-v\n%s:/data\n-w\n/data\n' "$2" ;;
  esac
}
user_flags() {
  case "$1" in
    podman) printf -- '--userns=keep-id\n--user\n%s:%s\n' "$(id -u)" "$(id -g)" ;;
    *)      printf -- '--user\n%s:%s\n' "$(id -u)" "$(id -g)" ;;
  esac
}

# --- fixtures -----------------------------------------------------------------
# good.procreate has a timelapse; empty.procreate has none, which exercises the
# "project written without a video" path. The broken fixtures the generator also
# writes are deliberately left out: a failed artwork suppresses the zip packing,
# and that would mask check 2.
note "fixtures"
GEN="$work/fx" go test ./internal/testkit -run TestGenFixture >/dev/null
mkdir -p "$work/in"
cp "$work/fx/good.procreate" "$work/fx/empty.procreate" "$work/in/"
ok "input prepared in $work/in"

# --- 6. image creation date ---------------------------------------------------
note "image metadata ($IMAGE)"
probe="${engines[0]}"
if ! "$probe" pull "$IMAGE" >/dev/null; then
  echo "run.sh: cannot pull $IMAGE — for a private project, log in first:" >&2
  echo "  $probe login registry.gitlab.com" >&2
  exit 2
fi
created="$("$probe" image inspect "$IMAGE" --format '{{.Created}}')"
printf '   created: %s\n' "$created"
case "$created" in
  1970-01-01*|0001-01-01*|"")
    bad "image creation date is the ko epoch default; check creation_time in .goreleaser.yaml" ;;
  *)
    ok "image creation date is a real date" ;;
esac

# Both published platforms should carry the same stamp. A host can usually pull
# the foreign architecture's manifest even when it cannot execute it, so this is
# an inspect-only check; skip rather than fail where the pull is refused.
for plat in linux/amd64 linux/arm64; do
  if ! "$probe" pull --platform "$plat" "$IMAGE" >/dev/null 2>&1; then
    printf '   skip %s (cannot pull this platform here)\n' "$plat"
    continue
  fi
  c="$("$probe" image inspect "$IMAGE" --format '{{.Created}}')"
  if [ "$c" = "$created" ]; then
    ok "$plat carries the same creation date"
  else
    bad "$plat has $c, expected $created"
  fi
done

# Leave the native image in place for the functional checks below.
"$probe" pull "$IMAGE" >/dev/null

# --- per-engine functional checks ---------------------------------------------
for engine in "${engines[@]}"; do
  note "$engine"
  command -v "$engine" >/dev/null || { bad "$engine not on PATH"; continue; }

  run_dir="$work/$engine"
  mkdir -p "$run_dir"
  cp -r "$work/in" "$run_dir/in"
  mapfile -t mflags < <(mount_flags "$engine" "$run_dir")
  mapfile -t uflags < <(user_flags "$engine")
  log="$run_dir/output.log"

  # 1. one file -> MP4
  if "$engine" run --rm "${uflags[@]}" "${mflags[@]}" "$IMAGE" \
       in/good.procreate one.mp4 >>"$log" 2>&1 && [ -s "$run_dir/one.mp4" ]; then
    ok "single file produced $(stat -c%s "$run_dir/one.mp4") bytes of MP4"
  else
    bad "single-file conversion"
  fi

  # 2. a folder -> mp4/ and procreate.zip
  #
  # Deliberately no --psd here. The testkit fixtures are segment-only archives
  # with no silica document, so a PSD export legitimately fails on them with
  # "Document.archive is missing; this is not a Procreate document" — and a
  # batch with any failure does not pack the zip, which would mask this check.
  # PSD export needs a real artwork; see the optional corpus pass below.
  #
  # Note procreate/ is *not* expected to survive: a clean run packs it into
  # procreate.zip and removes the directory.
  if "$engine" run --rm "${uflags[@]}" "${mflags[@]}" "$IMAGE" \
       in/ out/ >>"$log" 2>&1; then
    for want in out/mp4 out/procreate.zip; do
      if [ -e "$run_dir/$want" ]; then
        ok "batch produced $want"
      else
        bad "batch is missing $want"
      fi
    done
    # The projects themselves live inside the archive; prove it carries them.
    if command -v unzip >/dev/null 2>&1; then
      if unzip -l "$run_dir/out/procreate.zip" 2>/dev/null \
           | grep -q 'procreate/.*\.procreepy\.procreate'; then
        ok "procreate.zip contains the projects"
      else
        bad "procreate.zip has no procreate/*.procreepy.procreate entries"
      fi
    else
      printf '   skip zip listing (unzip not installed)\n'
    fi
  else
    bad "folder run"
  fi

  # 2b. PSD export, only against a real corpus. Opt in with the same variable
  # the Go corpus tests use:
  #   PROCREATE_FIXTURE_DIR=/path/to/real/files ci/container-smoke/run.sh
  if [ -n "${PROCREATE_FIXTURE_DIR:-}" ]; then
    if [ ! -d "$PROCREATE_FIXTURE_DIR" ]; then
      bad "PROCREATE_FIXTURE_DIR is not a directory: $PROCREATE_FIXTURE_DIR"
    else
      corpus_out="$run_dir/corpus"
      mkdir -p "$corpus_out"
      # Two mounts: the corpus read-only, so a smoke test can never write to
      # the originals, and a separate writable directory for the results.
      case "$engine" in
        podman) ro=":ro,Z"; rw=":Z" ;;
        *)      ro=":ro";   rw=""   ;;
      esac
      if "$engine" run --rm "${uflags[@]}" \
           -v "$PROCREATE_FIXTURE_DIR:/corpus$ro" \
           -v "$corpus_out:/data$rw" -w /data \
           "$IMAGE" --psd /corpus out/ >>"$log" 2>&1 \
         && [ -d "$corpus_out/out/psd" ]; then
        ok "corpus --psd produced $(find "$corpus_out/out/psd" -name '*.psd' | wc -l) PSD file(s)"
      else
        bad "corpus run with --psd"
      fi
    fi
  else
    printf '   skip --psd (set PROCREATE_FIXTURE_DIR to a real corpus)\n'
  fi

  # 3. stdin -> stdout, no mount and no user mapping needed
  if "$engine" run --rm -i "$IMAGE" - \
       <"$run_dir/in/good.procreate" >"$run_dir/stdio.mp4" 2>>"$log" \
     && [ -s "$run_dir/stdio.mp4" ]; then
    ok "stdin/stdout produced $(stat -c%s "$run_dir/stdio.mp4") bytes"
  else
    bad "stdin/stdout"
  fi

  # 4. ownership: everything written must belong to the invoking user
  foreign="$(find "$run_dir" \! -user "$(id -u)" -print -quit 2>/dev/null || true)"
  if [ -z "$foreign" ]; then
    ok "all artifacts owned by uid $(id -u)"
  else
    bad "not owned by you: $foreign (user mapping flags are wrong for $engine)"
  fi

  # 5. no permission errors in anything the run printed
  if grep -qiE 'permission denied|not writable' "$log"; then
    bad "permission errors in the output:"
    grep -iE 'permission denied|not writable' "$log" >&2
  else
    ok "no permission errors"
  fi
done

note "result"
if [ "$failures" -eq 0 ]; then
  echo "all container smoke checks passed for $IMAGE"
else
  echo "$failures container smoke check(s) failed for $IMAGE" >&2
  exit 1
fi
