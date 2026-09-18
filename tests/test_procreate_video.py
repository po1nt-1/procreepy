"""Tests for procreate-video.

The main checks go through ffprobe/ffmpeg (frame counts, duration, decoded
colours, full-decode playability) rather than `cmp`: two independent muxing
runs are not required to produce identical bytes.
"""

from __future__ import annotations

import os
import signal
import struct
import subprocess
import sys
import threading
import time
import zipfile
from pathlib import Path

import pytest

from conftest import (
    HAVE_FFMPEG, LAUNCHER, FRAMES_PER_SEGMENT, assert_playable, expected_colors,
    frame_colors, make_procreate, needs_ffmpeg, needs_x264, probe_video, seg_members, sha256,
)
from procreate_video.batch import plan_outputs
from procreate_video.core import _fmt_ranges, derive_output_name, find_segments, write_concat_list


def frames(n_segments: int) -> int:
    return n_segments * FRAMES_PER_SEGMENT


# ============================================================================
# single file
# ============================================================================
@needs_ffmpeg
def test_normal_procreate_to_file(tmp_path, cli, make_pc):
    src = make_pc(n=3)
    before = sha256(src)
    out = tmp_path / "result.mp4"

    r = cli(src, out)

    assert r.code == 0, r.err
    assert r.out == b"", "stdout must stay empty when OUTPUT is a file"
    info = probe_video(out)
    assert info["frames"] == frames(3)
    assert info["duration"] == pytest.approx(1.5, abs=0.1)
    assert frame_colors(out) == expected_colors([1, 2, 3])
    assert_playable(out)
    assert sha256(src) == before, "the .procreate must never be modified"
    stray = sorted(p.name for p in tmp_path.iterdir() if p.is_file())
    assert stray == ["art.procreate", "result.mp4"], "no .partial leftovers"


@needs_ffmpeg
def test_single_segment(tmp_path, cli, make_pc):
    src = make_pc(n=1)
    out = tmp_path / "one.mp4"
    r = cli(src, out)
    assert r.code == 0, r.err
    assert probe_video(out)["frames"] == frames(1)
    assert frame_colors(out) == expected_colors([1])
    assert_playable(out)


@needs_ffmpeg
def test_twelve_segments_are_joined_numerically(tmp_path, cli, make_pc):
    """segment-10 must come after segment-9, not right after segment-1."""
    shuffled = [10, 2, 11, 1, 9, 12, 3, 8, 4, 7, 5, 6]  # ZIP order is irrelevant too
    src = make_pc(numbers=shuffled)
    out = tmp_path / "twelve.mp4"
    r = cli(src, out)
    assert r.code == 0, r.err
    assert probe_video(out)["frames"] == frames(12)
    assert frame_colors(out) == expected_colors(range(1, 13))


@needs_ffmpeg
def test_spaces_and_special_characters_in_names(tmp_path, cli, make_pc):
    name = 'Мой рисунок (v2) it\'s "final" & $HOME `x`; #1.procreate'
    src = make_pc(name=name, n=2)
    out_dir = tmp_path / "out dir [1]"
    out_dir.mkdir()
    out = out_dir / "результат it's \"x\" & y.mp4"
    weird_tmp = tmp_path / "tmp dir it's odd"
    weird_tmp.mkdir()

    r = cli(src, out, "--tmpdir", weird_tmp)

    assert r.code == 0, r.err
    assert frame_colors(out) == expected_colors([1, 2])
    assert not list(weird_tmp.iterdir()), "--tmpdir must be cleaned up too"


@needs_ffmpeg
def test_output_into_existing_directory_uses_original_name(tmp_path, cli, make_pc):
    src = make_pc(name="Sunset study.procreate", n=2)
    out_dir = tmp_path / "videos"
    out_dir.mkdir()
    r = cli(src, out_dir)
    assert r.code == 0, r.err
    assert (out_dir / "Sunset study.mp4").is_file()


# ============================================================================
# stdin / stdout
# ============================================================================
@needs_ffmpeg
def test_stdin_input_to_file(tmp_path, cli, make_pc):
    src = make_pc(n=3)
    out = tmp_path / "from_stdin.mp4"
    r = cli("-", out, stdin=src.read_bytes())
    assert r.code == 0, r.err
    assert frame_colors(out) == expected_colors([1, 2, 3])
    assert_playable(out)


@needs_ffmpeg
@pytest.mark.parametrize("args", [(), ("-",)], ids=["omitted", "dash"])
def test_stdout_output_is_pure_video(tmp_path, cli, make_pc, args):
    src = make_pc(n=3)
    r = cli(src, *args)
    assert r.code == 0, r.err
    assert r.out[4:8] == b"ftyp", "stdout must start with an MP4 header, not text"
    assert "info:" in r.err, "messages belong on stderr"

    saved = tmp_path / "stdout.mp4"
    saved.write_bytes(r.out)
    assert probe_video(saved)["frames"] == frames(3)
    assert frame_colors(saved) == expected_colors([1, 2, 3])
    assert_playable(saved)  # any stray text in the stream would upset the decoder


@needs_ffmpeg
def test_stdin_to_stdout_matches_file_to_file(tmp_path, cli, make_pc):
    """The scenario from the spec. Compared through ffprobe, not `cmp`."""
    src = make_pc(n=5)
    file_out = tmp_path / "result.mp4"
    assert cli(src, file_out).code == 0

    r = cli("-", "-", stdin=src.read_bytes())
    assert r.code == 0, r.err
    pipe_out = tmp_path / "result2.mp4"
    pipe_out.write_bytes(r.out)

    a, b = probe_video(file_out), probe_video(pipe_out)
    assert (a["codec"], a["size"], a["frames"]) == (b["codec"], b["size"], b["frames"])
    assert a["duration"] == pytest.approx(b["duration"], abs=0.1)
    assert frame_colors(file_out) == frame_colors(pipe_out)


@needs_ffmpeg
def test_quiet_leaves_stderr_empty(tmp_path, cli, make_pc):
    r = cli(make_pc(n=2), tmp_path / "q.mp4", "-q")
    assert r.code == 0 and r.err == ""


@needs_ffmpeg
def test_refuses_to_dump_video_to_a_terminal(cli, make_pc):
    import pty

    src = make_pc(n=1)
    master, slave = pty.openpty()
    try:
        proc = subprocess.run([sys.executable, str(LAUNCHER), str(src)], stdin=subprocess.DEVNULL,
                              stdout=slave, stderr=subprocess.PIPE, timeout=60)
    finally:
        os.close(master)
        os.close(slave)
    assert proc.returncode == 2
    assert b"terminal" in proc.stderr


@needs_ffmpeg
def test_write_error_on_stdout_gives_nonzero_exit(tmp_path, make_pc):
    src = make_pc(n=2)
    with open("/dev/full", "wb") as full:
        proc = subprocess.run([sys.executable, str(LAUNCHER), str(src)], stdin=subprocess.DEVNULL,
                              stdout=full, stderr=subprocess.PIPE, timeout=60)
    assert proc.returncode == 9, proc.stderr
    assert b"error:" in proc.stderr


@needs_ffmpeg
def test_write_error_on_output_file(tmp_path, cli, make_pc):
    r = cli(make_pc(n=2), "/dev/full")
    assert r.code == 9, r.err
    r = cli(make_pc(n=2), tmp_path / "no-such-dir" / "x.mp4")
    assert r.code == 9 and "does not exist" in r.err


@needs_ffmpeg
def test_stdout_redirected_to_a_regular_file(tmp_path, make_pc):
    """`procreate-video x.procreate > x.mp4` -- the shell hands us a seekable file."""
    src = make_pc(n=3)
    out = tmp_path / "redirected.mp4"
    with open(out, "wb") as fh:
        proc = subprocess.run([sys.executable, str(LAUNCHER), str(src)], stdin=subprocess.DEVNULL,
                              stdout=fh, stderr=subprocess.PIPE, timeout=60)
    assert proc.returncode == 0, proc.stderr
    assert frame_colors(out) == expected_colors([1, 2, 3])
    assert_playable(out)


@needs_ffmpeg
@pytest.mark.parametrize("sig", [signal.SIGINT, signal.SIGTERM], ids=["SIGINT", "SIGTERM"])
def test_interruption_cleans_up_temp_files(tmp_path, segments, sig):
    """Ctrl+C / kill in the middle of a long run must leave nothing behind."""
    src = make_procreate(tmp_path / "long.procreate",
                         {f"video/segments/segment-{n}.mp4": segments[0] for n in range(1, 401)})
    scratch = tmp_path / "scratch"
    scratch.mkdir()
    out = tmp_path / "out.mp4"
    proc = subprocess.Popen([sys.executable, str(LAUNCHER), str(src), str(out)],
                            env={**os.environ, "TMPDIR": str(scratch)},
                            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        for _ in range(100):  # wait until it is really busy inside its temp dir
            if any(scratch.iterdir()):
                break
            time.sleep(0.05)
        time.sleep(0.5)
        if proc.poll() is not None:
            pytest.skip("finished before the signal could be sent (machine too fast)")
        proc.send_signal(sig)
        _, err = proc.communicate(timeout=60)
    finally:
        if proc.poll() is None:
            proc.kill()
    assert proc.returncode == 130, err
    assert not list(scratch.iterdir()), "temp directory must be removed"
    assert not out.exists() and not [p for p in tmp_path.iterdir() if p.suffix == ".partial"]


@needs_ffmpeg
def test_fifo_input_is_spooled_like_stdin(tmp_path, cli, make_pc):
    """A named pipe / `<(cat x)` is not seekable, so it is buffered first."""
    src = make_pc(n=2)
    fifo = tmp_path / "pipe.procreate"
    os.mkfifo(fifo)

    def feed():
        with open(fifo, "wb") as fh:
            fh.write(src.read_bytes())

    writer = threading.Thread(target=feed, daemon=True)
    writer.start()
    out = tmp_path / "o.mp4"
    r = cli(fifo, out)
    writer.join(timeout=10)
    assert r.code == 0, r.err
    assert frame_colors(out) == expected_colors([1, 2])


@needs_ffmpeg
def test_no_name_is_invented_for_pipes_and_stdin(tmp_path, cli):
    out_dir = tmp_path / "videos"
    out_dir.mkdir()
    fifo = tmp_path / "pipe.procreate"
    os.mkfifo(fifo)
    assert cli(fifo, out_dir).code == 2
    assert cli("-", out_dir, stdin=b"whatever").code == 2
    assert not list(out_dir.iterdir())


# ============================================================================
# bad input
# ============================================================================
@needs_ffmpeg
def test_missing_input_file(tmp_path, cli):
    r = cli(tmp_path / "nope.procreate", tmp_path / "o.mp4")
    assert r.code == 3 and r.err.startswith("error:") and "does not exist" in r.err


@needs_ffmpeg
def test_empty_stdin(tmp_path, cli):
    r = cli("-", tmp_path / "o.mp4", stdin=b"")
    assert r.code == 3 and "stdin is empty" in r.err


@needs_ffmpeg
def test_empty_file(tmp_path, cli):
    f = tmp_path / "empty.procreate"
    f.write_bytes(b"")
    r = cli(f, tmp_path / "o.mp4")
    assert r.code == 3 and "empty" in r.err


@needs_ffmpeg
def test_not_a_zip(tmp_path, cli):
    f = tmp_path / "junk.procreate"
    f.write_bytes(os.urandom(8192))
    out = tmp_path / "o.mp4"
    r = cli(f, out)
    assert r.code == 3 and "not a valid ZIP" in r.err
    assert not out.exists()


@needs_ffmpeg
def test_truncated_zip(tmp_path, cli, make_pc):
    data = make_pc(n=3).read_bytes()
    f = tmp_path / "cut.procreate"
    f.write_bytes(data[: len(data) // 2])  # the central directory is gone
    r = cli(f, tmp_path / "o.mp4")
    assert r.code == 3 and "not a valid ZIP" in r.err


@needs_ffmpeg
def test_no_video_segments(tmp_path, cli):
    src = make_procreate(tmp_path / "plain.procreate")
    out = tmp_path / "o.mp4"
    r = cli(src, out)
    assert r.code == 4 and "video/segments" in r.err
    assert not out.exists()
    assert cli(src).out == b"", "stdout stays clean on failure"


@needs_ffmpeg
def test_segments_without_numbers(tmp_path, cli):
    src = make_procreate(tmp_path / "x.procreate", {"video/segments/first.mp4": b"a",
                                                    "video/segments/second.mp4": b"b"})
    r = cli(src, tmp_path / "o.mp4")
    assert r.code == 5 and "segment-<number>.mp4" in r.err


@needs_ffmpeg
def test_gap_in_numbers_warns_but_still_joins(tmp_path, cli, make_pc):
    src = make_pc(numbers=[1, 2, 4, 5])
    out = tmp_path / "gap.mp4"
    r = cli(src, out)
    assert r.code == 0, r.err
    assert "warning:" in r.err and "missing: 3" in r.err.replace("segment numbers ", "")
    assert probe_video(out)["frames"] == frames(4)


@needs_ffmpeg
def test_gap_in_numbers_is_an_error_with_strict(tmp_path, cli, make_pc):
    out = tmp_path / "gap.mp4"
    r = cli(make_pc(numbers=[1, 2, 4]), out, "--strict")
    assert r.code == 5 and not out.exists()


@needs_ffmpeg
def test_missing_first_segment_is_reported(tmp_path, cli, make_pc):
    r = cli(make_pc(numbers=[3, 4]), tmp_path / "o.mp4")
    assert r.code == 0 and "1-2" in r.err


@needs_ffmpeg
def test_duplicate_segment_numbers_are_ambiguous(tmp_path, cli, segments):
    src = make_procreate(tmp_path / "d.procreate", {
        "video/segments/segment-1.mp4": segments[0],
        "video/segments/segment-01.mp4": segments[1]})
    r = cli(src, tmp_path / "o.mp4")
    assert r.code == 5 and "ambiguous" in r.err


# ============================================================================
# corrupted segments
# ============================================================================
@needs_ffmpeg
def test_garbage_mp4_inside_valid_zip(tmp_path, cli, segments):
    members = seg_members(segments, [1, 2, 3])
    members["video/segments/segment-2.mp4"] = os.urandom(4096)
    src = make_procreate(tmp_path / "g.procreate", members)
    out = tmp_path / "o.mp4"
    r = cli(src, out)
    assert r.code == 5 and "segment-2.mp4" in r.err and "corrupted" in r.err
    assert not out.exists()


@needs_ffmpeg
def test_truncated_mp4_inside_valid_zip(tmp_path, cli, segments):
    members = seg_members(segments, [1, 2, 3])
    members["video/segments/segment-3.mp4"] = segments[2][: len(segments[2]) // 2]
    src = make_procreate(tmp_path / "t.procreate", members)
    r = cli(src, tmp_path / "o.mp4")
    assert r.code == 5 and "segment-3.mp4" in r.err


@needs_ffmpeg
def test_zip_crc_damage_in_a_segment(tmp_path, cli, segments):
    src = make_procreate(tmp_path / "c.procreate", seg_members(segments, [1, 2]))
    raw = bytearray(src.read_bytes())
    with zipfile.ZipFile(src) as zf:
        zi = zf.getinfo("video/segments/segment-2.mp4")
    # Local file header: 30 fixed bytes, then name and extra field (lengths at +26/+28).
    name_len, extra_len = struct.unpack_from("<HH", raw, zi.header_offset + 26)
    payload = zi.header_offset + 30 + name_len + extra_len
    assert bytes(raw[payload:payload + 64]) == segments[1][:64]
    raw[payload + 100] ^= 0xFF  # payload is stored uncompressed: this only breaks the CRC
    src.write_bytes(bytes(raw))

    r = cli(src, tmp_path / "o.mp4")
    assert r.code == 5 and "segment-2.mp4" in r.err and "corrupted" in r.err


@needs_ffmpeg
def test_failed_run_keeps_an_existing_output_intact(tmp_path, cli, segments):
    members = seg_members(segments, [1, 2])
    members["video/segments/segment-2.mp4"] = b"garbage"
    src = make_procreate(tmp_path / "bad.procreate", members)
    out = tmp_path / "keep.mp4"
    out.write_bytes(b"PRECIOUS")
    r = cli(src, out)
    assert r.code == 5
    assert out.read_bytes() == b"PRECIOUS"
    assert not [p for p in tmp_path.iterdir() if p.suffix == ".partial"]


@needs_ffmpeg
def test_incompatible_segments_are_refused_for_stream_copy(tmp_path, cli, segments, big_segment):
    src = make_procreate(tmp_path / "mixed.procreate", {
        "video/segments/segment-1.mp4": segments[0],
        "video/segments/segment-2.mp4": big_segment})
    out = tmp_path / "o.mp4"
    r = cli(src, out)
    assert r.code == 7 and "64x64" in r.err and "128x128" in r.err and "--reencode" in r.err
    assert not out.exists()


@needs_ffmpeg
@needs_x264
def test_reencode_rescues_incompatible_segments(tmp_path, cli, segments, big_segment):
    src = make_procreate(tmp_path / "mixed.procreate", {
        "video/segments/segment-1.mp4": segments[0],
        "video/segments/segment-2.mp4": big_segment})
    out = tmp_path / "o.mp4"
    r = cli(src, out, "--reencode")
    assert r.code == 0, r.err
    info = probe_video(out)
    assert info["frames"] == frames(2) and info["codec"] == "h264"
    assert_playable(out)


# ============================================================================
# missing tools
# ============================================================================
def _bin_dir_with(tmp_path: Path, *tools: str) -> Path:
    d = tmp_path / ("bin-" + "-".join(tools) if tools else "bin-empty")
    d.mkdir()
    for t in tools:
        real = subprocess.run(["which", t], capture_output=True, text=True).stdout.strip()
        (d / t).symlink_to(real)
    return d


def test_missing_ffmpeg(tmp_path, cli):
    src = make_procreate(tmp_path / "a.procreate", {"video/segments/segment-1.mp4": b"x"})
    tools = ("ffprobe",) if HAVE_FFMPEG else ()
    r = cli(src, tmp_path / "o.mp4", env={"PATH": str(_bin_dir_with(tmp_path, *tools))})
    assert r.code == 6 and "ffmpeg not found" in r.err


@needs_ffmpeg
def test_missing_ffprobe(tmp_path, cli):
    src = make_procreate(tmp_path / "a.procreate", {"video/segments/segment-1.mp4": b"x"})
    r = cli(src, tmp_path / "o.mp4", env={"PATH": str(_bin_dir_with(tmp_path, "ffmpeg"))})
    assert r.code == 6 and "ffprobe not found" in r.err


@needs_ffmpeg
def test_list_needs_no_external_tools(tmp_path, cli, make_pc):
    r = cli("--list", make_pc(n=2), env={"PATH": str(_bin_dir_with(tmp_path))})
    assert r.code == 0 and "segments: 2" in r.out.decode()


@needs_ffmpeg
def test_verify_needs_ffprobe(tmp_path, cli, make_pc):
    r = cli("--verify", make_pc(n=2), env={"PATH": str(_bin_dir_with(tmp_path, "ffmpeg"))})
    assert r.code == 6 and "ffprobe not found" in r.err


# ============================================================================
# --list / --verify
# ============================================================================
@needs_ffmpeg
def test_list_output_format(cli, make_pc, tmp_path):
    src = make_pc(numbers=[10, 2, 12, 1, 11, 3, 9, 4, 8, 5, 7, 6])
    r = cli("--list", src)
    assert r.code == 0, r.err
    lines = r.out.decode().splitlines()
    assert lines[0] == f"input: {src}"
    assert lines[1] == "segments: 12"
    assert lines[2] == ""
    assert lines[3] == "1  video/segments/segment-1.mp4"
    assert lines[11] == "9  video/segments/segment-9.mp4"
    assert lines[12] == "10 video/segments/segment-10.mp4"
    assert lines[14] == "12 video/segments/segment-12.mp4"
    assert len(lines) == 15


@needs_ffmpeg
def test_list_from_stdin(cli, make_pc):
    r = cli("--list", "-", stdin=make_pc(n=3).read_bytes())
    assert r.code == 0 and "input: <stdin>" in r.out.decode() and "segments: 3" in r.out.decode()


@needs_ffmpeg
def test_list_without_segments(cli, tmp_path):
    r = cli("--list", make_procreate(tmp_path / "p.procreate"))
    assert r.code == 4 and "segments: 0" in r.out.decode()


@needs_ffmpeg
def test_verify_ok_creates_no_video(tmp_path, cli, make_pc):
    src = make_pc(n=4)
    before = set(os.listdir(tmp_path))
    r = cli("--verify", src)
    assert r.code == 0, r.err
    assert "verify: ok" in r.out.decode() and r.out.decode().count(" ok ") == 4
    assert set(os.listdir(tmp_path)) == before, "--verify must not write anything"


@needs_ffmpeg
def test_verify_reports_every_bad_segment(tmp_path, cli, segments):
    members = seg_members(segments, [1, 2, 3, 4])
    members["video/segments/segment-2.mp4"] = b"junk"
    members["video/segments/segment-4.mp4"] = b"junk"
    r = cli("--verify", make_procreate(tmp_path / "v.procreate", members))
    assert r.code == 5
    out = r.out.decode()
    assert out.count("FAIL") == 2 and out.count(" ok ") == 2


@needs_ffmpeg
def test_list_takes_no_output_argument(cli, make_pc, tmp_path):
    r = cli("--list", make_pc(n=1), tmp_path / "x.mp4")
    assert r.code == 2


# ============================================================================
# safety
# ============================================================================
@needs_ffmpeg
def test_output_may_not_overwrite_the_input(cli, make_pc):
    src = make_pc(n=2)
    before = sha256(src)
    r = cli(src, src)
    assert r.code == 2 and "same file" in r.err
    assert sha256(src) == before


# ============================================================================
# batch: input/ -> output/timelaps/
# ============================================================================
def _input_dir(tmp_path: Path, segments, with_broken=True) -> Path:
    d = tmp_path / "input"
    d.mkdir()
    make_procreate(d / "Alpha.procreate", seg_members(segments, [1, 2]))
    make_procreate(d / "Beta sketch (2).procreate", seg_members(segments, [3, 4, 5]))
    make_procreate(d / "NoTimelapse.procreate")
    (d / "._Alpha.procreate").write_bytes(b"\x00\x05\x16\x07 AppleDouble junk")
    (d / "notes.txt").write_text("not an artwork")
    if with_broken:
        (d / "Broken.procreate").write_bytes(b"this is not a zip")
    return d


def _tree_hash(d: Path) -> dict:
    return {p.name: sha256(p) for p in d.iterdir() if p.is_file()}


@needs_ffmpeg
def test_batch_default_output_dir(tmp_path, cli, segments):
    d = _input_dir(tmp_path, segments)
    before = _tree_hash(d)

    r = cli("input", cwd=tmp_path)

    out_dir = tmp_path / "output" / "timelaps"
    assert sorted(p.name for p in out_dir.iterdir()) == ["Alpha.mp4", "Beta sketch (2).mp4"]
    assert probe_video(out_dir / "Alpha.mp4")["frames"] == frames(2)
    assert probe_video(out_dir / "Beta sketch (2).mp4")["frames"] == frames(3)
    assert frame_colors(out_dir / "Beta sketch (2).mp4") == expected_colors([3, 4, 5])
    assert r.out == b""
    assert r.code == 1, "one file (Broken) failed"
    bad_line = next(ln for ln in r.err.splitlines() if "Broken.procreate" in ln)
    assert bad_line.startswith("error: [") and "not a valid ZIP" in bad_line
    assert bad_line.count("Broken.procreate") == 1, "the path must not be repeated"
    assert "NoTimelapse.procreate: no timelapse" in r.err
    assert "summary: 2 converted, 1 without timelapse, 1 FAILED" in r.err
    assert "._Alpha" not in r.err, "AppleDouble files are ignored"
    assert _tree_hash(d) == before, "inputs are never modified"


@needs_ffmpeg
def test_batch_all_good_exits_zero_and_rerun_skips(tmp_path, cli, segments):
    _input_dir(tmp_path, segments, with_broken=False)

    r1 = cli("input", cwd=tmp_path)
    assert r1.code == 0, r1.err
    out_dir = tmp_path / "output" / "timelaps"
    alpha = out_dir / "Alpha.mp4"
    stamp = alpha.stat().st_mtime_ns

    r2 = cli("input", cwd=tmp_path)  # idempotent: existing videos are left alone
    assert r2.code == 0
    assert "summary: 0 converted, 2 already existed" in r2.err
    assert alpha.stat().st_mtime_ns == stamp

    r3 = cli("input", "--force", cwd=tmp_path)
    assert r3.code == 0 and "summary: 2 converted" in r3.err
    assert alpha.stat().st_mtime_ns != stamp


@needs_ffmpeg
def test_batch_explicit_output_dir(tmp_path, cli, segments):
    _input_dir(tmp_path, segments, with_broken=False)
    r = cli(tmp_path / "input", tmp_path / "my" / "videos")
    assert r.code == 0, r.err
    assert (tmp_path / "my" / "videos" / "Alpha.mp4").is_file()


@needs_ffmpeg
def test_batch_recursive_mirrors_subdirs_and_avoids_clashes(tmp_path, cli, segments):
    root = tmp_path / "input"
    (root / "2025").mkdir(parents=True)
    (root / "2026").mkdir()
    make_procreate(root / "2025" / "Cat.procreate", seg_members(segments, [1]))
    make_procreate(root / "2026" / "Cat.procreate", seg_members(segments, [2]))
    make_procreate(root / "Top.procreate", seg_members(segments, [3]))

    flat = cli(root, tmp_path / "flat")  # without -r only the top level is used
    assert flat.code == 0, flat.err
    assert [p.name for p in (tmp_path / "flat").rglob("*.mp4")] == ["Top.mp4"]

    r = cli("-r", root, tmp_path / "out")
    assert r.code == 0, r.err
    got = sorted(str(p.relative_to(tmp_path / "out")) for p in (tmp_path / "out").rglob("*.mp4"))
    assert got == ["2025/Cat.mp4", "2026/Cat.mp4", "Top.mp4"]
    assert frame_colors(tmp_path / "out" / "2026" / "Cat.mp4") == expected_colors([2])


def test_batch_plan_disambiguates_same_stem(tmp_path):
    files = [tmp_path / "a.procreate", tmp_path / "a.PROCREATE", tmp_path / "a.Procreate"]
    plan = plan_outputs(files, tmp_path, tmp_path / "out", recursive=False)
    assert [dst.name for _, dst in plan] == ["a.mp4", "a-2.mp4", "a-3.mp4"]


@needs_ffmpeg
def test_batch_input_dir_without_procreate_files(tmp_path, cli, segments):
    (tmp_path / "empty").mkdir()
    r = cli(tmp_path / "empty")
    assert r.code == 3 and "no .procreate files" in r.err

    only_sub = tmp_path / "only_sub"
    (only_sub / "nested").mkdir(parents=True)
    make_procreate(only_sub / "nested" / "x.procreate", seg_members(segments, [1]))
    r = cli(only_sub)
    assert r.code == 3 and "use -r" in r.err


@needs_ffmpeg
def test_batch_cannot_go_to_stdout(tmp_path, cli, segments):
    _input_dir(tmp_path, segments)
    r = cli(tmp_path / "input", "-")
    assert r.code == 2 and r.out == b""


@needs_ffmpeg
def test_list_and_verify_accept_a_directory(tmp_path, cli, segments):
    _input_dir(tmp_path, segments, with_broken=False)
    rl = cli("--list", tmp_path / "input")
    assert rl.code == 0, rl.err
    assert rl.out.decode().count("input: ") == 3 and "segments: 0" in rl.out.decode()
    rv = cli("--verify", tmp_path / "input")
    assert rv.code == 0, rv.err
    assert rv.out.decode().count("verify: ok") == 2


# ============================================================================
# unit tests (no ffmpeg needed)
# ============================================================================
def test_segments_sort_numerically_not_lexicographically(tmp_path):
    names = [f"video/segments/segment-{n}.mp4" for n in (10, 2, 1, 11, 9, 3, 12, 4, 5, 6, 7, 8)]
    src = make_procreate(tmp_path / "a.procreate", {n: b"x" for n in names})
    with zipfile.ZipFile(src) as zf:
        segs = find_segments(zf)
    assert [s.number for s in segs] == list(range(1, 13))
    assert [s.name for s in segs] != sorted(names), "plain string sort would be wrong"


def test_raster_chunks_and_other_files_are_ignored(tmp_path):
    src = make_procreate(tmp_path / "a.procreate", {"video/segments/segment-1.mp4": b"x"})
    with zipfile.ZipFile(src) as zf:
        assert [s.number for s in find_segments(zf)] == [1]  # .lz4 chunks never looked at


def test_fmt_ranges():
    assert _fmt_ranges([1, 2, 3, 7, 9, 10]) == "1-3, 7, 9-10"
    assert _fmt_ranges([5]) == "5"


def test_derive_output_name_keeps_the_original_name():
    assert derive_output_name(Path("in/My Art v2.procreate")) == "My Art v2.mp4"
    assert derive_output_name(Path("a.b.c.PROCREATE")) == "a.b.c.mp4"


def test_concat_list_escapes_quotes(tmp_path):
    lst = tmp_path / "concat.txt"
    write_concat_list([Path("/tmp/it's here/segment-1.mp4"), Path("/tmp/x/segment-2.mp4")], lst)
    assert lst.read_text() == (
        "ffconcat version 1.0\n"
        "file '/tmp/it'\\''s here/segment-1.mp4'\n"
        "file '/tmp/x/segment-2.mp4'\n"
    )
