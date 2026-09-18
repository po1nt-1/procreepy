"""Shared helpers.

No real artwork is needed: a .procreate file is just a ZIP, so the tests build
small ones out of generated solid-colour MP4 segments. Every segment gets its
own colour, which lets the tests check the *order* of the joined video by
decoding it, not just its length.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import zipfile
from pathlib import Path
from types import SimpleNamespace

import pytest

ROOT = Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / "procreate-video"

HAVE_FFMPEG = bool(shutil.which("ffmpeg") and shutil.which("ffprobe"))
needs_ffmpeg = pytest.mark.skipif(not HAVE_FFMPEG, reason="ffmpeg/ffprobe not installed")

FRAMES_PER_SEGMENT = 5
FPS = 10

# 12 colours, pairwise far apart, so lossy encoding can never blur two together.
PALETTE = [
    (255, 0, 0), (0, 255, 0), (0, 0, 255), (255, 255, 0),
    (255, 0, 255), (0, 255, 255), (255, 255, 255), (0, 0, 0),
    (255, 128, 0), (128, 0, 255), (128, 128, 128), (0, 255, 128),
]


def _has_encoder(name: str) -> bool:
    if not HAVE_FFMPEG:
        return False
    out = subprocess.run(["ffmpeg", "-hide_banner", "-encoders"],
                         capture_output=True, text=True).stdout
    return name in out


HAVE_X264 = _has_encoder("libx264")
needs_x264 = pytest.mark.skipif(not HAVE_X264, reason="ffmpeg has no libx264 encoder")
# libx264 when available (what Procreate-like files use), else the encoder
# every ffmpeg build has. The tool itself is codec-agnostic.
ENCODER = "libx264" if HAVE_X264 else "mpeg4"


def make_segment(path: Path, color_idx: int, size: str = "64x64") -> Path:
    r, g, b = PALETTE[color_idx]
    subprocess.run(
        ["ffmpeg", "-y", "-v", "error", "-f", "lavfi",
         "-i", f"color=c=0x{r:02X}{g:02X}{b:02X}:s={size}:r={FPS}",
         "-frames:v", str(FRAMES_PER_SEGMENT), "-c:v", ENCODER, "-pix_fmt", "yuv420p", str(path)],
        check=True,
    )
    return path


def make_procreate(path: Path, members: dict | None = None) -> Path:
    """Write a fake .procreate. `members` maps ZIP member name -> bytes and is
    added on top of the usual non-video content (Document.archive, thumbnail,
    a raster chunk with a misleading .lz4 name)."""
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("Document.archive", b"bplist00" + b"\0" * 64, zipfile.ZIP_DEFLATED)
        z.writestr("QuickLook/Thumbnail.png", b"\x89PNG fake", zipfile.ZIP_DEFLATED)
        z.writestr("11111111-2222-3333-4444-555555555555/0~0.lz4", os.urandom(256))
        for name, data in (members or {}).items():
            z.writestr(name, data, zipfile.ZIP_STORED)  # Procreate stores video uncompressed
    return path


@pytest.fixture(scope="session")
def segments(tmp_path_factory):
    """12 one-colour segments, index i has PALETTE[i]. Values are bytes."""
    if not HAVE_FFMPEG:
        pytest.skip("ffmpeg/ffprobe not installed")
    d = tmp_path_factory.mktemp("segments")
    return [make_segment(d / f"c{i}.mp4", i).read_bytes() for i in range(12)]


@pytest.fixture(scope="session")
def big_segment(tmp_path_factory):
    """A segment with a different resolution from the others."""
    if not HAVE_FFMPEG:
        pytest.skip("ffmpeg/ffprobe not installed")
    d = tmp_path_factory.mktemp("bigseg")
    return make_segment(d / "big.mp4", 3, size="128x128").read_bytes()


def seg_members(segments, numbers, order=None) -> dict:
    """{'video/segments/segment-N.mp4': bytes} where segment N shows PALETTE[N-1]."""
    return {f"video/segments/segment-{n}.mp4": segments[(n - 1) % 12] for n in (order or numbers)}


@pytest.fixture
def make_pc(tmp_path, segments):
    """Build a .procreate with segments 1..n (or explicit numbers) in tmp_path."""
    def build(name="art.procreate", n=3, numbers=None, extra=None, where=None):
        nums = list(numbers) if numbers is not None else list(range(1, n + 1))
        members = seg_members(segments, nums)
        members.update(extra or {})
        return make_procreate((where or tmp_path) / name, members)
    return build


@pytest.fixture
def cli(tmp_path):
    """Run the real launcher in a subprocess, with TMPDIR pointed at a private
    directory that must be empty afterwards (temp files are always cleaned up)."""
    scratch = tmp_path / "scratch-tmp"
    scratch.mkdir()

    def run(*args, stdin=None, env=None, cwd=None, expect_clean=True):
        e = os.environ.copy()
        e["TMPDIR"] = str(scratch)
        e.update(env or {})
        proc = subprocess.run(
            [sys.executable, str(LAUNCHER), *map(str, args)],
            input=stdin, stdin=None if stdin is not None else subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=e, cwd=cwd, timeout=180,
        )
        if expect_clean:
            leftovers = list(scratch.iterdir())
            assert not leftovers, f"temp files left behind: {leftovers}"
        return SimpleNamespace(code=proc.returncode, out=proc.stdout,
                               err=proc.stderr.decode("utf-8", "replace"))
    return run


# ---- checking produced videos ---------------------------------------------
def sha256(path: Path) -> str:
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def probe_video(path: Path) -> dict:
    """Decode-count frames, so this works for regular and fragmented MP4 alike."""
    res = subprocess.run(
        ["ffprobe", "-v", "error", "-count_frames", "-select_streams", "v:0", "-of", "json",
         "-show_entries", "stream=codec_name,width,height,nb_read_frames:format=duration,format_name",
         str(path)],
        capture_output=True, text=True, check=True,
    )
    data = json.loads(res.stdout)
    stream = data["streams"][0]
    return {
        "codec": stream["codec_name"], "size": (stream["width"], stream["height"]),
        "frames": int(stream["nb_read_frames"]),
        "duration": float(data["format"]["duration"]) if "duration" in data["format"] else None,
    }


def assert_playable(path: Path) -> None:
    """Fully decode the file; any decoder complaint fails the test."""
    res = subprocess.run(["ffmpeg", "-v", "error", "-i", str(path), "-f", "null", "-"],
                         capture_output=True, text=True)
    assert res.returncode == 0 and not res.stderr.strip(), res.stderr


def frame_colors(path: Path) -> list:
    """Palette index of every decoded frame, in playback order."""
    res = subprocess.run(
        ["ffmpeg", "-v", "error", "-i", str(path), "-vf", "scale=1:1:flags=area",
         "-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1"],
        capture_output=True, check=True,
    )
    raw = res.stdout
    out = []
    for i in range(0, len(raw), 3):
        px = tuple(raw[i:i + 3])
        out.append(min(range(len(PALETTE)),
                       key=lambda k: sum((a - b) ** 2 for a, b in zip(px, PALETTE[k]))))
    return out


def expected_colors(numbers) -> list:
    return [(n - 1) % 12 for n in numbers for _ in range(FRAMES_PER_SEGMENT)]
