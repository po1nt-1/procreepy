"""Command line interface."""

from __future__ import annotations

import argparse
import os
import signal
import sys
from typing import List, Optional

from . import __version__
from .batch import DEFAULT_OUTPUT_DIR, convert_directory, diagnose_directory
from .core import (
    EXIT_FAILURE,
    EXIT_INTERRUPTED,
    EXIT_USAGE,
    EXIT_WRITE,
    PROG,
    Options,
    PvError,
    convert,
    list_segments,
    log,
    resolve_output,
    verify_segments,
)

EPILOG = f"""\
examples:
  {PROG} artwork.procreate artwork.mp4
  {PROG} artwork.procreate > artwork.mp4
  cat artwork.procreate | {PROG} - > artwork.mp4

  {PROG} input/                    every .procreate in input/ -> {DEFAULT_OUTPUT_DIR}/<name>.mp4
  {PROG} input/ videos/            same, into videos/
  {PROG} -r input/                 also look in sub-directories (mirrored in the output)

  {PROG} --list artwork.procreate  show the segments, in playback order
  {PROG} --verify artwork.procreate  check every segment with ffprobe, write nothing

INPUT may be a file, a directory of files, or - for stdin.
OUTPUT may be a file, a directory, or - / omitted for stdout (fragmented MP4).
Messages go to stderr; stdout carries only video (or the --list/--verify report).
"""


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog=PROG,
        usage=f"{PROG} [options] INPUT [OUTPUT]",
        description="Extract the archived timelapse from .procreate files as ready-made MP4 videos.",
        epilog=EPILOG,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    p.add_argument("input", metavar="INPUT", help="file.procreate, a directory of them, or - for stdin")
    p.add_argument("output", metavar="OUTPUT", nargs="?",
                   help="output.mp4, a directory, or - for stdout (default for a single file)")
    mode = p.add_mutually_exclusive_group()
    mode.add_argument("--list", action="store_true", help="list the segments and exit")
    mode.add_argument("--verify", action="store_true",
                      help="extract the segments and check them with ffprobe; no video is produced")
    p.add_argument("-r", "--recursive", action="store_true",
                   help="directory input: also process sub-directories")
    p.add_argument("-f", "--force", action="store_true",
                   help="directory input: overwrite videos that already exist (default: skip them)")
    p.add_argument("--strict", action="store_true",
                   help="treat missing segment numbers as an error instead of a warning")
    p.add_argument("--reencode", action="store_true",
                   help="re-encode to H.264 if segments cannot be joined with stream copy")
    p.add_argument("--tmpdir", metavar="DIR",
                   help="where to extract segments (default: $TMPDIR, else /var/tmp)")
    p.add_argument("-q", "--quiet", action="store_true", help="only print warnings and errors")
    p.add_argument("--version", action="version", version=f"{PROG} {__version__}")
    return p


def _on_sigterm(signum, frame) -> None:  # noqa: ARG001
    raise KeyboardInterrupt  # unwinds through `finally`, so temp files get removed


def _dispatch(args: argparse.Namespace, opts: Options) -> int:
    src = args.input
    is_dir = src != "-" and os.path.isdir(src)

    if args.list or args.verify:
        if args.output is not None:
            raise PvError("--list and --verify take a single INPUT and no OUTPUT", EXIT_USAGE)
        action = list_segments if args.list else verify_segments
        return diagnose_directory(src, action, opts) if is_dir else action(src, opts)

    if is_dir:
        return convert_directory(src, args.output, opts)

    convert(src, resolve_output(src, args.output), opts)
    return 0


def main(argv: Optional[List[str]] = None) -> int:
    args = build_parser().parse_args(argv)
    log.quiet = args.quiet
    opts = Options(
        force=args.force, strict=args.strict, reencode=args.reencode,
        recursive=args.recursive, tmpdir=args.tmpdir,
    )
    signal.signal(signal.SIGTERM, _on_sigterm)
    try:
        return _dispatch(args, opts)
    except PvError as exc:
        log.error(exc.message)
        return exc.code
    except KeyboardInterrupt:
        log.error("interrupted")
        return EXIT_INTERRUPTED
    except BrokenPipeError:
        # The reader of our stdout went away (e.g. `--list | head`).
        try:
            os.dup2(os.open(os.devnull, os.O_WRONLY), sys.stdout.fileno())
        except (OSError, ValueError):
            pass
        log.error("failed to write to stdout: broken pipe")
        return EXIT_WRITE
    except Exception as exc:  # noqa: BLE001 - last resort, keep the exit code honest
        if os.environ.get("PROCREATE_VIDEO_DEBUG"):
            raise
        log.error(f"unexpected {type(exc).__name__}: {exc} (set PROCREATE_VIDEO_DEBUG=1 for a traceback)")
        return EXIT_FAILURE


if __name__ == "__main__":
    sys.exit(main())
