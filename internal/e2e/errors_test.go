package e2e

import (
	"archive/zip"
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"procreepy/internal/mp4"
	"procreepy/internal/testkit"
)

func TestInputErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		dir := t.TempDir()
		check(t, run(t, dir, nil, "nope.procreate", "out.mp4"), 3, "",
			actionErr("input does not exist: nope.procreate"))
	})
	t.Run("empty_file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "in.procreate"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 3, "",
			actionErr("input is empty: in.procreate"))
	})
	t.Run("not_a_zip", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "in.procreate"), []byte("definitely not a zip"), 0o644); err != nil {
			t.Fatal(err)
		}
		check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 3, "",
			actionErr("input is not a valid ZIP archive: in.procreate (not a .procreate file, or truncated/corrupted)"))
	})
	t.Run("truncated_zip", func(t *testing.T) {
		dir := t.TempDir()
		whole := testkit.ArchiveBytes(stdThree(), false)
		if err := os.WriteFile(filepath.Join(dir, "in.procreate"), whole[:len(whole)/2], 0o644); err != nil {
			t.Fatal(err)
		}
		check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 3, "",
			actionErr("input is not a valid ZIP archive: in.procreate (not a .procreate file, or truncated/corrupted)"))
	})
}

func TestNoSegments(t *testing.T) {
	entries := map[string][]byte{
		"Document/document.data": []byte("fake-document-data"),
		"video/segments/":        {},
	}
	t.Run("convert", func(t *testing.T) {
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", entries)
		check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 4, "",
			actionErr("no video/segments in this archive (time-lapse recording was probably turned off for this artwork)"))
		if _, err := os.Stat(filepath.Join(dir, "out.mp4")); !os.IsNotExist(err) {
			t.Error("out.mp4 should not exist")
		}
	})
	t.Run("list", func(t *testing.T) {
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", entries)
		check(t, run(t, dir, nil, "--list", "in.procreate"), 4,
			"input: in.procreate\nsegments: 0\n",
			actionErr("no video/segments in this archive"))
	})
	t.Run("verify", func(t *testing.T) {
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", entries)
		check(t, run(t, dir, nil, "--verify", "in.procreate"), 4, "",
			actionErr("no video/segments in this archive (time-lapse recording was probably turned off for this artwork)"))
	})
	t.Run("unnumbered_only", func(t *testing.T) {
		e := map[string][]byte{
			"Document/document.data":      []byte("fake-document-data"),
			"video/segments/":             {},
			"video/segments/whatever.mp4": []byte("xx"),
		}
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", e)
		check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 5, "",
			strayWarn("video/segments/whatever.mp4")+
				actionErr("video/segments has 1 .mp4 file(s), but none is named segment-<number>.mp4, so the order cannot be determined"))
	})
}

// handZip writes members in the given order (the scanner reports clashes in
// archive order, which the map-based testkit cannot express).
func handZip(t *testing.T, members map[string][]byte, order []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAmbiguousNumbering(t *testing.T) {
	dir := t.TempDir()
	arc := handZip(t, map[string][]byte{
		"Document/document.data":       []byte("fake-document-data"),
		"video/segments/SEGMENT-1.MP4": segN(1),
		"video/segments/segment-1.mp4": segN(2),
	}, []string{"Document/document.data", "video/segments/",
		"video/segments/SEGMENT-1.MP4", "video/segments/segment-1.mp4"})
	if err := os.WriteFile(filepath.Join(dir, "in.procreate"), arc, 0o644); err != nil {
		t.Fatal(err)
	}
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 5, "",
		actionErr("bad segment: ambiguous segment numbering: video/segments/SEGMENT-1.MP4, video/segments/segment-1.mp4 all map to segment 1"))
}

func TestCorruptedSegment(t *testing.T) {
	dir := t.TempDir()
	writeArchiveCorrupted(t, dir, "in.procreate", stdThree(), "video/segments/segment-2.mp4")
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 5, "",
		slogLine(slog.LevelInfo, "segments found", "input", "in.procreate", "count", 3)+
			actionErr("segment video/segments/segment-2.mp4 is corrupted inside the archive: zip: checksum error"))
	if _, err := os.Stat(filepath.Join(dir, "out.mp4")); !os.IsNotExist(err) {
		t.Error("out.mp4 should not exist")
	}
}

// appendMdat attaches a second, tiny top-level mdat box to a valid segment.
func appendMdat(seg []byte) []byte {
	payload := []byte("junkjunk")
	head := make([]byte, 8)
	sz := uint32(len(head) + len(payload))
	head[0], head[1], head[2], head[3] = byte(sz>>24), byte(sz>>16), byte(sz>>8), byte(sz)
	copy(head[4:], "mdat")
	return append(append([]byte{}, seg...), append(head, payload...)...)
}

func TestTwoMdat(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	entries["video/segments/segment-1.mp4"] = appendMdat(segN(1))
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 5, "",
		slogLine(slog.LevelInfo, "segments found", "input", "in.procreate", "count", 3)+
			actionErr("bad segment: segment video/segments/segment-1.mp4 has 2 mdat boxes (want 1)"))
}

// incompatibleSegA/B differ in height, so SameStreams fails.
func incompatibleSegA() []byte {
	return testkit.Segment(320, 240, []uint32{1007, 903}, []uint32{705})
}

func incompatibleSegB() []byte {
	return testkit.Segment(320, 180, []uint32{1014, 906}, []uint32{710})
}

func streamsOf(t *testing.T, seg []byte) string {
	t.Helper()
	m, err := mp4.Parse(bytes.NewReader(seg), int64(len(seg)))
	if err != nil {
		t.Fatalf("parse %s: %v", seg, err)
	}
	return m.StreamsSummary()
}

func TestIncompatible(t *testing.T) {
	dir := t.TempDir()
	a, b := incompatibleSegA(), incompatibleSegB()
	entries := map[string][]byte{
		"Document/document.data":       []byte("fake-document-data"),
		"video/segments/":              {},
		"video/segments/segment-1.mp4": a,
		"video/segments/segment-2.mp4": b,
	}
	writeArchive(t, dir, "in.procreate", entries)
	if streamsOf(t, a) == streamsOf(t, b) {
		t.Fatal("test fixture bug: summaries should differ")
	}
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 7, "",
		slogLine(slog.LevelInfo, "segments found", "input", "in.procreate", "count", 2)+
			actionErr("incompatible segments \"video/segments/segment-1.mp4\" and "+
				"\"video/segments/segment-2.mp4\": field \"height\" differs"))
	if _, err := os.Stat(filepath.Join(dir, "out.mp4")); !os.IsNotExist(err) {
		t.Error("out.mp4 should not exist")
	}
}

// TestTmpdirFallback: an unusable --tmpdir silently falls back to
// $TMPDIR//var/tmp; the output is byte-identical to the default run.
func TestTmpdirFallback(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	wantErr := chattyBlock("in.procreate", 3, "out.mp4", 6.0)
	check(t, run(t, dir, nil, "--tmpdir", "does-not-exist", "in.procreate", "out.mp4"), 0, "", wantErr)
	eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
}
