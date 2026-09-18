"""Batch mode: a directory full of .procreate files -> a directory of MP4s.

    procreate-video input/                 # -> output/timelaps/<name>.mp4
    procreate-video input/ some/other/dir

Output names follow the originals: ``Sketch 12.procreate`` -> ``Sketch 12.mp4``.
One broken file never stops the run; failures are collected and reported at the
end, and the exit code is non-zero if any file failed.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Callable, List, Optional, Set, Tuple

from .core import (
    EXIT_FAILURE,
    EXIT_INPUT,
    EXIT_NO_SEGMENTS,
    EXIT_OK,
    EXIT_USAGE,
    EXIT_WRITE,
    Options,
    PvError,
    convert,
    derive_output_name,
    log,
    require_tool,
)

DEFAULT_OUTPUT_DIR = Path("output") / "timelaps"


def _is_procreate(name: str) -> bool:
    # Dot-files are skipped on purpose: macOS leaves "._Foo.procreate" resource
    # forks behind when files are copied around, and they are not ZIP archives.
    return not name.startswith(".") and name.lower().endswith(".procreate")


def discover(root: Path, recursive: bool) -> List[Path]:
    found: List[Path] = []
    if recursive:
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = sorted(d for d in dirnames if not d.startswith("."))
            found += [Path(dirpath) / f for f in sorted(filenames) if _is_procreate(f)]
    else:
        with os.scandir(root) as it:
            entries = sorted(it, key=lambda e: e.name)
        found = [Path(e.path) for e in entries if e.is_file() and _is_procreate(e.name)]
    return found


def _unique(dst: Path, used: Set[Path]) -> Path:
    cand, n = dst, 2
    while cand in used:
        cand = dst.with_name(f"{dst.stem}-{n}{dst.suffix}")
        n += 1
    return cand


def plan_outputs(
    files: List[Path], root: Path, out_dir: Path, recursive: bool
) -> List[Tuple[Path, Path]]:
    """Map every input to its output path. Sub-directories are mirrored with -r,
    and a name clash (a.procreate vs a.PROCREATE) gets a -2, -3 ... suffix."""
    used: Set[Path] = set()
    plan: List[Tuple[Path, Path]] = []
    for src in files:
        rel_dir = src.relative_to(root).parent if recursive else Path()
        dst = _unique(out_dir / rel_dir / derive_output_name(src), used)
        used.add(dst)
        plan.append((src, dst))
    return plan


def _find_files(root: Path, opts: Options) -> List[Path]:
    files = discover(root, opts.recursive)
    if not files:
        hint = "" if opts.recursive else " (use -r to look in sub-directories)"
        raise PvError(f"no .procreate files found in {root}{hint}", EXIT_INPUT)
    return files


def convert_directory(in_dir: str, output_arg: Optional[str], opts: Options) -> int:
    if output_arg == "-":
        raise PvError(
            "cannot write several videos to stdout; give an output directory "
            f"(default: {DEFAULT_OUTPUT_DIR}/)",
            EXIT_USAGE,
        )
    root = Path(in_dir)
    out_dir = Path(output_arg) if output_arg else DEFAULT_OUTPUT_DIR
    if out_dir.exists() and not out_dir.is_dir():
        raise PvError(f"output path exists and is not a directory: {out_dir}", EXIT_USAGE)

    files = _find_files(root, opts)
    require_tool("ffmpeg")  # fail once, up front, instead of once per file
    require_tool("ffprobe")
    try:
        out_dir.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        raise PvError(f"cannot create {out_dir}: {exc.strerror}", EXIT_WRITE)

    plan = plan_outputs(files, root, out_dir, opts.recursive)
    total = len(plan)
    log.info(f"{total} .procreate file(s) in {root} -> {out_dir}/")

    converted = existed = no_video = 0
    failed: List[str] = []
    for i, (src, dst) in enumerate(plan, 1):
        tag = f"[{i}/{total}] {src}"
        if dst.exists() and not opts.force:
            log.info(f"{tag}: skipped, {dst} already exists (use --force to overwrite)")
            existed += 1
            continue
        try:
            dst.parent.mkdir(parents=True, exist_ok=True)
            convert(str(src), dst, opts, chatty=False)
        except PvError as exc:
            if exc.code == EXIT_NO_SEGMENTS:
                log.warn(f"{tag}: no timelapse video inside, skipped")
                no_video += 1
            else:
                # Some messages already name the file; don't print the path twice.
                if str(src) in exc.message:
                    log.error(f"[{i}/{total}] {exc.message}")
                else:
                    log.error(f"{tag}: {exc.message}")
                failed.append(str(src))
            continue
        except OSError as exc:
            log.error(f"{tag}: {exc.strerror or exc}")
            failed.append(str(src))
            continue
        log.info(f"{tag} -> {dst}")
        converted += 1

    parts = [f"{converted} converted"]
    if existed:
        parts.append(f"{existed} already existed")
    if no_video:
        parts.append(f"{no_video} without timelapse")
    if failed:
        parts.append(f"{len(failed)} FAILED")
    log.info("summary: " + ", ".join(parts))
    return EXIT_FAILURE if failed else EXIT_OK


def diagnose_directory(
    in_dir: str, action: Callable[[str, Options], int], opts: Options
) -> int:
    """Run --list / --verify over every .procreate file in a directory."""
    files = _find_files(Path(in_dir), opts)
    failed = 0
    for i, src in enumerate(files):
        if i:
            print()
            sys.stdout.flush()
        try:
            action(str(src), opts)
        except PvError as exc:
            if exc.code == EXIT_NO_SEGMENTS:
                log.warn(f"{src}: no timelapse video inside")
            else:
                log.error(f"{src}: {exc.message}")
                failed += 1
    return EXIT_FAILURE if failed else EXIT_OK
