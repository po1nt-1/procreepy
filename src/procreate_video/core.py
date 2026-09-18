"""Core of procreate-video.

A ``.procreate`` file is a ZIP archive. When time-lapse recording was on, the
recording is stored as a series of ready-made MP4 files::

    video/segments/segment-1.mp4
    video/segments/segment-2.mp4
    ...

This module never touches Document.archive or the raster chunks. It only
extracts those MP4 segments, checks them with ffprobe and joins them with
FFmpeg's concat demuxer (stream copy, no re-encoding by default).

Rules that hold everywhere in this module:

* stdout is reserved for data (video bytes, or the report of --list/--verify);
  every message goes to stderr;
* the input archive is only ever opened read-only;
* temporary files live in a private directory that is always removed.
"""

from __future__ import annotations

import contextlib
import dataclasses
import errno
import json
import os
import re
import secrets
import shutil
import stat
import subprocess
import sys
import tempfile
import zipfile
import zlib
from pathlib import Path
from typing import List, Optional, Tuple

PROG = "procreate-video"

# ---- exit codes -----------------------------------------------------------
EXIT_OK = 0
EXIT_FAILURE = 1  # unexpected error / at least one file failed in batch mode
EXIT_USAGE = 2  # bad command line (argparse also uses 2)
EXIT_INPUT = 3  # input missing, empty, or not a valid ZIP
EXIT_NO_SEGMENTS = 4  # no video/segments in the archive
EXIT_BAD_SEGMENT = 5  # segment corrupted / bad numbering
EXIT_TOOL_MISSING = 6  # ffmpeg / ffprobe (or a needed encoder) is missing
EXIT_INCOMPATIBLE = 7  # segments cannot be joined with stream copy
EXIT_FFMPEG = 8  # ffmpeg failed for another reason
EXIT_WRITE = 9  # cannot write output / temp files
EXIT_INTERRUPTED = 130

SEGMENT_DIR = "video/segments/"
SEGMENT_RE = re.compile(r"^video/segments/segment-([0-9]+)\.mp4$", re.IGNORECASE)
COPY_BUF = 1024 * 1024

# Fragmented MP4 works on non-seekable outputs (pipes).
FRAG_FLAGS = "+frag_keyframe+empty_moov+default_base_moof"
# Regular MP4: moov atom moved to the front for fast start.
FILE_FLAGS = "+faststart"

_WRITE_ERROR_HINTS = (
    "broken pipe",
    "no space left",
    "input/output error",
    "permission denied",
    "read-only file system",
    "disk quota",
    "i/o error",
)


# ---- messages (stderr only) ----------------------------------------------
class _Log:
    def __init__(self) -> None:
        self.quiet = False

    @staticmethod
    def _emit(prefix: str, msg: str) -> None:
        try:
            print(f"{prefix}: {msg}", file=sys.stderr, flush=True)
        except (OSError, ValueError):
            pass

    def info(self, msg: str) -> None:
        if not self.quiet:
            self._emit("info", msg)

    def warn(self, msg: str) -> None:
        self._emit("warning", msg)

    def error(self, msg: str) -> None:
        self._emit("error", msg)


log = _Log()


class PvError(Exception):
    """An expected failure: message for the user plus a process exit code."""

    def __init__(self, message: str, code: int = EXIT_FAILURE) -> None:
        super().__init__(message)
        self.message = message
        self.code = code


@dataclasses.dataclass
class Options:
    force: bool = False  # batch: overwrite existing outputs
    strict: bool = False  # gaps in segment numbers are an error
    reencode: bool = False  # allow re-encoding when stream copy is impossible
    recursive: bool = False  # batch: descend into sub-directories
    tmpdir: Optional[str] = None


@dataclasses.dataclass
class Segment:
    number: int
    name: str  # member name inside the ZIP
    info: zipfile.ZipInfo

    @property
    def size(self) -> int:
        return self.info.file_size


@dataclasses.dataclass
class Probe:
    duration: Optional[float]
    signature: tuple  # what must match for `-c copy` concat
    time_base: Optional[str]
    rotation: Optional[float]


# ---- small helpers --------------------------------------------------------
def human(n: float) -> str:
    for unit in ("B", "KiB", "MiB", "GiB"):
        if n < 1024:
            return f"{n:.0f} {unit}" if unit == "B" else f"{n:.1f} {unit}"
        n /= 1024
    return f"{n:.1f} TiB"


def _first_line(text: str) -> str:
    for line in text.strip().splitlines():
        if line.strip():
            return line.strip()
    return "unknown error"


def _fmt_ranges(nums: List[int]) -> str:
    """[1, 2, 3, 7, 9, 10] -> '1-3, 7, 9-10'."""
    out: List[str] = []
    start = prev = None
    for n in sorted(nums) + [None]:  # type: ignore[list-item]
        if start is None:
            start = prev = n
        elif n is not None and n == prev + 1:
            prev = n
        else:
            out.append(str(start) if start == prev else f"{start}-{prev}")
            start = prev = n
    return ", ".join(out)


def derive_output_name(src: Path) -> str:
    """artwork.procreate -> artwork.mp4 (the name stays tied to the original)."""
    return src.stem + ".mp4"


def require_tool(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise PvError(
            f"{name} not found in PATH. On Fedora: sudo dnf install ffmpeg-free "
            "(or the full ffmpeg from RPM Fusion)",
            EXIT_TOOL_MISSING,
        )
    return path


# ---- temporary workspace --------------------------------------------------
class Workspace:
    """A lazily created private temp directory that is always removed.

    Base directory precedence: --tmpdir, $TMPDIR, /var/tmp, system default.
    /var/tmp is preferred over /tmp because on Fedora /tmp is a RAM-backed
    tmpfs, and extracted timelapse segments can be hundreds of megabytes.
    """

    def __init__(self, base: Optional[str]) -> None:
        self._base = base
        self._path: Optional[Path] = None

    @property
    def path(self) -> Path:
        if self._path is None:
            base = self._base or os.environ.get("TMPDIR") or None
            if base is None and os.path.isdir("/var/tmp") and os.access("/var/tmp", os.W_OK):
                base = "/var/tmp"
            if base is not None and not os.path.isdir(base):
                raise PvError(f"temporary directory does not exist: {base}", EXIT_WRITE)
            try:
                self._path = Path(tempfile.mkdtemp(prefix=f"{PROG}-", dir=base))
            except OSError as exc:
                raise PvError(f"cannot create a temporary directory: {exc.strerror}", EXIT_WRITE)
        return self._path

    def close(self) -> None:
        if self._path is not None:
            shutil.rmtree(self._path, ignore_errors=True)
            self._path = None

    def __enter__(self) -> "Workspace":
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()


def ensure_space(where: Path, needed: int, what: str) -> None:
    free = shutil.disk_usage(where).free
    if needed > free:
        raise PvError(
            f"not enough free space in {where} for {what}: need {human(needed)}, "
            f"have {human(free)}. Point --tmpdir (or $TMPDIR) at a bigger, "
            "disk-backed directory",
            EXIT_WRITE,
        )


# ---- input ----------------------------------------------------------------
def resolve_input(arg: str, ws: Workspace) -> Tuple[Path, str]:
    """Return (seekable path, label).

    stdin ("-") and non-seekable inputs (pipes, process substitution) are
    spooled to a temp file, because a ZIP needs random access.
    """
    if arg == "-":
        if sys.stdin is None or sys.stdin.closed:
            raise PvError("stdin is not available", EXIT_INPUT)
        if sys.stdin.isatty():
            raise PvError(
                "stdin is a terminal; pipe a .procreate file into it "
                "(cat artwork.procreate | procreate-video - ...)",
                EXIT_USAGE,
            )
        return _spool(sys.stdin.buffer, ws, "<stdin>", "stdin"), "<stdin>"

    try:
        st = os.stat(arg)
    except FileNotFoundError:
        raise PvError(f"input does not exist: {arg}", EXIT_INPUT)
    except OSError as exc:
        raise PvError(f"cannot access input {arg}: {exc.strerror}", EXIT_INPUT)
    if stat.S_ISDIR(st.st_mode):
        raise PvError(f"input is a directory: {arg}", EXIT_INPUT)
    if stat.S_ISREG(st.st_mode):
        return Path(arg), arg
    try:
        with open(arg, "rb") as fh:
            return _spool(fh, ws, arg, "input"), arg
    except OSError as exc:
        raise PvError(f"cannot read input {arg}: {exc.strerror}", EXIT_INPUT)


def _spool(stream, ws: Workspace, label: str, noun: str) -> Path:
    dest = ws.path / "input.procreate"
    try:
        with open(dest, "wb") as out:
            shutil.copyfileobj(stream, out, COPY_BUF)
        size = dest.stat().st_size
    except OSError as exc:
        if exc.errno == errno.ENOSPC:
            raise PvError(
                f"no space left while buffering {label}; use --tmpdir (or $TMPDIR) "
                "on a bigger disk-backed directory",
                EXIT_WRITE,
            )
        raise PvError(f"cannot buffer {label}: {exc.strerror}", EXIT_WRITE)
    if size == 0:
        raise PvError(f"{noun} is empty: {label}", EXIT_INPUT)
    return dest


def open_archive(path: Path, label: str) -> zipfile.ZipFile:
    try:
        size = path.stat().st_size
    except OSError as exc:
        raise PvError(f"cannot access input {label}: {exc.strerror}", EXIT_INPUT)
    if size == 0:
        raise PvError(f"input is empty: {label}", EXIT_INPUT)
    try:
        return zipfile.ZipFile(path, "r")  # read-only: the original is never modified
    except zipfile.BadZipFile:
        raise PvError(
            f"input is not a valid ZIP archive: {label} "
            "(not a .procreate file, or truncated/corrupted)",
            EXIT_INPUT,
        )
    except (zipfile.LargeZipFile, NotImplementedError, RuntimeError) as exc:
        raise PvError(f"cannot read ZIP archive {label}: {exc}", EXIT_INPUT)
    except OSError as exc:
        raise PvError(f"cannot read input {label}: {exc.strerror or exc}", EXIT_INPUT)


# ---- segments -------------------------------------------------------------
def scan_segments(zf: zipfile.ZipFile) -> Tuple[List[Segment], List[str]]:
    """Find video/segments/segment-N.mp4 members, sorted by N *numerically*.

    Returns (segments, ignored_mp4_names). Lexicographic order would put
    segment-10 before segment-2, which silently scrambles the timelapse.
    """
    by_number: dict = {}
    ignored: List[str] = []
    for zi in zf.infolist():
        name = zi.filename
        if zi.is_dir() or not name.lower().startswith(SEGMENT_DIR):
            continue
        match = SEGMENT_RE.match(name)
        if match:
            by_number.setdefault(int(match.group(1)), []).append(zi)
        elif name.lower().endswith(".mp4"):
            ignored.append(name)

    clashes = {n: v for n, v in by_number.items() if len(v) > 1}
    if clashes:
        n, members = next(iter(sorted(clashes.items())))
        names = ", ".join(m.filename for m in members)
        raise PvError(
            f"ambiguous segment numbering: {names} all map to segment {n}",
            EXIT_BAD_SEGMENT,
        )
    segments = [Segment(n, v[0].filename, v[0]) for n, v in sorted(by_number.items())]
    return segments, ignored


def find_segments(
    zf: zipfile.ZipFile, *, strict: bool = False, allow_empty: bool = False
) -> List[Segment]:
    segments, ignored = scan_segments(zf)
    for name in ignored:
        log.warn(f"ignoring {name}: name does not match segment-<number>.mp4")

    if not segments:
        if allow_empty:
            return []
        if ignored:
            raise PvError(
                f"video/segments has {len(ignored)} .mp4 file(s), but none is named "
                "segment-<number>.mp4, so the order cannot be determined",
                EXIT_BAD_SEGMENT,
            )
        raise PvError(
            "no video/segments in this archive (time-lapse recording was probably "
            "turned off for this artwork)",
            EXIT_NO_SEGMENTS,
        )

    present = {s.number for s in segments}
    lo, hi = min(1, segments[0].number), segments[-1].number
    missing = [n for n in range(lo, hi + 1) if n not in present]
    if missing:
        msg = (
            f"segment numbers missing: {_fmt_ranges(missing)} "
            "(the video would have gaps)"
        )
        if strict:
            raise PvError(msg, EXIT_BAD_SEGMENT)
        log.warn(msg)
    return segments


def extract_segment(zf: zipfile.ZipFile, seg: Segment, dest: Path) -> None:
    """Copy one segment out of the ZIP. Verifies the ZIP CRC as a side effect."""
    try:
        with zf.open(seg.info) as src, open(dest, "wb") as dst:
            shutil.copyfileobj(src, dst, COPY_BUF)
    except (zipfile.BadZipFile, zlib.error, EOFError) as exc:
        raise PvError(
            f"segment {seg.name} is corrupted inside the archive: {exc}", EXIT_BAD_SEGMENT
        )
    except (NotImplementedError, RuntimeError) as exc:
        raise PvError(f"cannot read segment {seg.name}: {exc}", EXIT_BAD_SEGMENT)
    except OSError as exc:
        if exc.errno == errno.ENOSPC:
            raise PvError(
                f"no space left while extracting {seg.name}; use --tmpdir (or $TMPDIR) "
                "on a bigger disk-backed directory",
                EXIT_WRITE,
            )
        raise PvError(f"I/O error while extracting {seg.name}: {exc.strerror or exc}")


# ---- ffprobe --------------------------------------------------------------
_PROBE_ENTRIES = (
    "format=duration:"
    "stream=index,codec_type,codec_name,width,height,pix_fmt,time_base,sample_rate,channels:"
    "stream_side_data=rotation"
)


def probe_segment(ffprobe: str, path: Path, label: str) -> Probe:
    cmd = [ffprobe, "-v", "error", "-hide_banner", "-of", "json",
           "-show_entries", _PROBE_ENTRIES, str(path)]
    try:
        res = subprocess.run(
            cmd, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, text=True, errors="replace",
        )
    except OSError as exc:
        raise PvError(f"cannot run ffprobe: {exc.strerror or exc}", EXIT_TOOL_MISSING)

    if res.returncode != 0:
        raise PvError(
            f"segment {label} is corrupted or not a valid MP4 "
            f"(ffprobe: {_first_line(res.stderr)})",
            EXIT_BAD_SEGMENT,
        )
    try:
        data = json.loads(res.stdout or "{}")
    except ValueError:
        raise PvError(f"segment {label}: unreadable ffprobe output", EXIT_BAD_SEGMENT)

    streams = data.get("streams") or []
    if not any(s.get("codec_type") == "video" for s in streams):
        raise PvError(f"segment {label} has no video stream", EXIT_BAD_SEGMENT)
    if res.stderr.strip():
        log.warn(f"{label}: ffprobe: {_first_line(res.stderr)}")

    signature = []
    time_base = rotation = None
    for s in streams:
        kind = s.get("codec_type")
        if kind == "video":
            signature.append(("video", s.get("codec_name"), s.get("width"),
                              s.get("height"), s.get("pix_fmt")))
            if time_base is None:
                time_base = s.get("time_base")
                for sd in s.get("side_data_list") or []:
                    if "rotation" in sd:
                        rotation = float(sd["rotation"])
        elif kind == "audio":
            signature.append(("audio", s.get("codec_name"),
                              s.get("sample_rate"), s.get("channels")))

    try:
        duration: Optional[float] = float((data.get("format") or {}).get("duration"))
    except (TypeError, ValueError):
        duration = None
    return Probe(duration, tuple(signature), time_base, rotation)


def describe_signature(sig: tuple) -> str:
    parts = []
    for s in sig:
        if s[0] == "video":
            parts.append(f"video {s[1]} {s[2]}x{s[3]} {s[4]}")
        else:
            parts.append(f"audio {s[1]} {s[2]}Hz {s[3]}ch")
    return ", ".join(parts) or "no streams"


def check_compatibility(segments: List[Segment], probes: List[Probe], *, reencode: bool) -> None:
    """Concat demuxer + `-c copy` needs identical codec parameters in every file."""
    ref_seg, ref = segments[0], probes[0]
    bad = [(s, p) for s, p in zip(segments[1:], probes[1:]) if p.signature != ref.signature]
    if bad:
        lines = [f"  {ref_seg.name}: {describe_signature(ref.signature)}"]
        lines += [f"  {s.name}: {describe_signature(p.signature)}" for s, p in bad[:5]]
        if len(bad) > 5:
            lines.append(f"  ... and {len(bad) - 5} more")
        if not reencode:
            raise PvError(
                "segments have different stream parameters, so they cannot be joined "
                "with stream copy (-c copy):\n" + "\n".join(lines)
                + "\n  re-run with --reencode to re-encode to H.264",
                EXIT_INCOMPATIBLE,
            )
        log.warn("segments differ in stream parameters; re-encoding\n" + "\n".join(lines))

    if any(p.time_base != ref.time_base for p in probes[1:]):
        log.warn("segments use different time bases; check the result if playback looks off")
    if any(p.rotation != ref.rotation for p in probes[1:]):
        log.warn(
            "segments carry different rotation metadata; stream copy applies the first "
            "segment's orientation to the whole video (--reencode honours each segment)"
        )


# ---- ffmpeg ---------------------------------------------------------------
def write_concat_list(paths: List[Path], list_path: Path) -> None:
    """ffconcat list; single quotes in paths are escaped as '\\''."""
    lines = [b"ffconcat version 1.0"]
    for p in paths:
        raw = os.fsencode(str(p))
        if b"\n" in raw or b"\r" in raw:
            raise PvError("temporary directory path contains a newline; use --tmpdir", EXIT_WRITE)
        lines.append(b"file '" + raw.replace(b"'", b"'\\''") + b"'")
    list_path.write_bytes(b"\n".join(lines) + b"\n")


def _ffmpeg_cmd(ffmpeg: str, list_path: Path, reencode: bool) -> List[str]:
    cmd = [ffmpeg, "-hide_banner", "-nostdin", "-nostats", "-loglevel", "error", "-y",
           "-f", "concat", "-safe", "0", "-i", str(list_path)]
    if reencode:
        cmd += ["-c:v", "libx264", "-preset", "medium", "-crf", "18",
                "-pix_fmt", "yuv420p", "-c:a", "aac"]
    else:
        cmd += ["-c", "copy"]
    return cmd


def _run_ffmpeg(cmd: List[str], *, stdout, reencode: bool) -> None:
    try:
        res = subprocess.run(cmd, stdin=subprocess.DEVNULL, stdout=stdout,
                             stderr=subprocess.PIPE, text=True, errors="replace")
    except OSError as exc:
        raise PvError(f"cannot run ffmpeg: {exc.strerror or exc}", EXIT_TOOL_MISSING)
    if res.returncode == 0:
        return

    low = res.stderr.lower()
    tail = "\n".join("  " + ln for ln in res.stderr.strip().splitlines()[-8:])
    if any(hint in low for hint in _WRITE_ERROR_HINTS):
        raise PvError(f"failed to write the output: {_first_line(res.stderr)}", EXIT_WRITE)
    if "unknown encoder" in low or "encoder not found" in low:
        raise PvError(
            "this ffmpeg has no libx264 encoder, which --reencode needs "
            "(Fedora: install ffmpeg from RPM Fusion)",
            EXIT_TOOL_MISSING,
        )
    hint = "" if reencode else "\n  (segments may be incompatible for stream copy; try --reencode)"
    raise PvError(f"ffmpeg failed to join the segments:\n{tail}{hint}", EXIT_FFMPEG)


def check_stdout() -> int:
    try:
        fd = sys.stdout.fileno()
    except (AttributeError, ValueError, OSError):
        raise PvError("stdout is not available", EXIT_WRITE)
    if os.isatty(fd):
        raise PvError(
            "refusing to write video data to a terminal; redirect stdout "
            "(> artwork.mp4) or give an OUTPUT path",
            EXIT_USAGE,
        )
    return fd


def mux_stdout(ffmpeg: str, list_path: Path, opts: Options) -> None:
    fd = check_stdout()
    sys.stdout.flush()
    cmd = _ffmpeg_cmd(ffmpeg, list_path, opts.reencode)
    cmd += ["-movflags", FRAG_FLAGS, "-f", "mp4", "pipe:1"]
    _run_ffmpeg(cmd, stdout=fd, reencode=opts.reencode)


def check_output_target(out: Path) -> None:
    out = out.absolute()
    if out.exists() and not out.is_file():
        return  # device / FIFO such as /dev/stdout: streamed as fragmented MP4
    parent = out.parent
    if not parent.is_dir():
        raise PvError(f"output directory does not exist: {parent}", EXIT_WRITE)
    if not os.access(parent, os.W_OK | os.X_OK):
        raise PvError(f"output directory is not writable: {parent}", EXIT_WRITE)


def mux_file(ffmpeg: str, list_path: Path, out: Path, opts: Options) -> None:
    out = out.absolute()
    base = _ffmpeg_cmd(ffmpeg, list_path, opts.reencode)

    if out.exists() and not out.is_file():
        cmd = base + ["-movflags", FRAG_FLAGS, "-f", "mp4", "file:" + str(out)]
        _run_ffmpeg(cmd, stdout=subprocess.DEVNULL, reencode=opts.reencode)
        return

    # Write next to the target and rename, so a failed run never leaves a
    # half-written .mp4 and never destroys an existing good one.
    partial = out.parent / f".{PROG}-{os.getpid()}-{secrets.token_hex(4)}.partial"
    cmd = base + ["-movflags", FILE_FLAGS, "-f", "mp4", str(partial)]
    try:
        _run_ffmpeg(cmd, stdout=subprocess.DEVNULL, reencode=opts.reencode)
        try:
            os.replace(partial, out)
        except OSError as exc:
            raise PvError(f"cannot write {out}: {exc.strerror or exc}", EXIT_WRITE)
    finally:
        with contextlib.suppress(OSError):
            partial.unlink()


# ---- public operations ----------------------------------------------------
def resolve_output(input_arg: str, output_arg: Optional[str]) -> Optional[Path]:
    """None means stdout. An existing directory (or 'dir/') gets <stem>.mp4 inside."""
    if output_arg is None or output_arg == "-":
        return None
    p = Path(output_arg)
    if output_arg.endswith(os.sep) or p.is_dir():
        if input_arg == "-" or (os.path.exists(input_arg) and not os.path.isfile(input_arg)):
            raise PvError(
                "cannot derive an output file name from stdin or a pipe; "
                "pass a file name as OUTPUT",
                EXIT_USAGE,
            )
        if not p.is_dir():
            raise PvError(f"output directory does not exist: {p}", EXIT_WRITE)
        return p / derive_output_name(Path(input_arg))
    return p


def convert(input_arg: str, out_path: Optional[Path], opts: Options, *, chatty: bool = True) -> None:
    """Build the full timelapse MP4. out_path=None writes MP4 to stdout."""
    ffmpeg = require_tool("ffmpeg")
    ffprobe = require_tool("ffprobe")
    if out_path is None:
        check_stdout()
    else:
        check_output_target(out_path)

    with Workspace(opts.tmpdir) as ws:
        src, label = resolve_input(input_arg, ws)
        if out_path is not None and out_path.exists():
            with contextlib.suppress(OSError):
                if os.path.samefile(src, out_path):
                    raise PvError(f"OUTPUT is the same file as INPUT: {out_path}", EXIT_USAGE)

        with open_archive(src, label) as zf:
            segments = find_segments(zf, strict=opts.strict)
            if chatty:
                log.info(f"{label}: {len(segments)} segment(s)")
            ensure_space(ws.path, sum(s.size for s in segments), "the extracted segments")

            paths: List[Path] = []
            probes: List[Probe] = []
            for seg in segments:  # extract + probe one by one: fail fast on a bad segment
                dest = ws.path / f"segment-{seg.number:06d}.mp4"
                extract_segment(zf, seg, dest)
                probes.append(probe_segment(ffprobe, dest, seg.name))
                paths.append(dest)

        check_compatibility(segments, probes, reencode=opts.reencode)
        list_path = ws.path / "concat.txt"
        write_concat_list(paths, list_path)

        how = "re-encoding to H.264" if opts.reencode else "stream copy"
        if chatty:
            log.info(f"joining segments ({how})")
        if out_path is None:
            mux_stdout(ffmpeg, list_path, opts)
        else:
            mux_file(ffmpeg, list_path, out_path, opts)

    if chatty:
        total = sum(p.duration or 0.0 for p in probes)
        dest_txt = "stdout" if out_path is None else str(out_path)
        log.info(f"done: {dest_txt} (~{total:.1f} s of video)")


def list_segments(input_arg: str, opts: Options) -> int:
    """--list: print the segments in numeric order (report goes to stdout)."""
    with Workspace(opts.tmpdir) as ws:
        src, label = resolve_input(input_arg, ws)
        with open_archive(src, label) as zf:
            segments = find_segments(zf, strict=False, allow_empty=True)
    print(f"input: {label}")
    print(f"segments: {len(segments)}")
    if segments:
        print()
        width = len(str(segments[-1].number))
        for seg in segments:
            print(f"{seg.number:<{width}} {seg.name}")
    sys.stdout.flush()
    if not segments:
        raise PvError("no video/segments in this archive", EXIT_NO_SEGMENTS)
    return EXIT_OK


def verify_segments(input_arg: str, opts: Options) -> int:
    """--verify: extract every segment and ffprobe it; no video is produced."""
    ffprobe = require_tool("ffprobe")
    with Workspace(opts.tmpdir) as ws:
        src, label = resolve_input(input_arg, ws)
        with open_archive(src, label) as zf:
            segments = find_segments(zf, strict=opts.strict)
            print(f"input: {label}")
            print(f"segments: {len(segments)}")
            print()
            width = len(str(segments[-1].number))
            probes: List[Probe] = []
            good: List[Segment] = []
            failed = 0
            for seg in segments:
                dest = ws.path / "segment.mp4"
                try:
                    extract_segment(zf, seg, dest)
                    pr = probe_segment(ffprobe, dest, seg.name)
                except PvError as exc:
                    failed += 1
                    print(f"{seg.number:<{width}} FAIL  {_first_line(exc.message)}")
                else:
                    probes.append(pr)
                    good.append(seg)
                    dur = f"{pr.duration:.2f}s" if pr.duration is not None else "n/a"
                    print(f"{seg.number:<{width}} ok    {describe_signature(pr.signature)}"
                          f"  {dur}  {seg.name}")
                finally:
                    with contextlib.suppress(OSError):
                        dest.unlink()
            sys.stdout.flush()

    if failed:
        raise PvError(f"verify failed: {failed} of {len(segments)} segment(s) are bad",
                      EXIT_BAD_SEGMENT)
    check_compatibility(good, probes, reencode=opts.reencode)
    total = sum(p.duration or 0.0 for p in probes)
    print(f"\nverify: ok, {len(segments)} segment(s), ~{total:.1f} s of video")
    sys.stdout.flush()
    return EXIT_OK
