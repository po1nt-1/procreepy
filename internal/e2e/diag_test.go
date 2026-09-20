package e2e

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestListSingle(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	wantOut := "input: in.procreate\nsegments: 3\n" +
		"\n" +
		"1 video/segments/segment-1.mp4\n" +
		"2 video/segments/segment-2.mp4\n" +
		"3 video/segments/segment-3.mp4\n"
	check(t, run(t, dir, nil, "--list", "in.procreate"), 0, wantOut, "")
}

func TestListWidth12(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdArchiveEntries(12))
	var sb strings.Builder
	sb.WriteString("input: in.procreate\nsegments: 12\n\n")
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&sb, "%-2d video/segments/segment-%d.mp4\n", i, i)
	}
	check(t, run(t, dir, nil, "--list", "in.procreate"), 0, sb.String(), "")
}

func TestListGapWarning(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	delete(entries, "video/segments/segment-2.mp4")
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "--list", "in.procreate"), 0,
		"input: in.procreate\nsegments: 2\n\n"+
			"1 video/segments/segment-1.mp4\n3 video/segments/segment-3.mp4\n",
		gapWarn("2"))
}

func TestListIgnoredStray(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	entries["video/segments/thumb.mp4"] = []byte("not an mp4")
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "--list", "in.procreate"), 0,
		"input: in.procreate\nsegments: 3\n"+
			"\n"+
			"1 video/segments/segment-1.mp4\n"+
			"2 video/segments/segment-2.mp4\n"+
			"3 video/segments/segment-3.mp4\n",
		strayWarn("video/segments/thumb.mp4"))
}

// --strict has no effect on --list (List never enables Options.Strict).
func TestListIgnoresStrict(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	delete(entries, "video/segments/segment-2.mp4")
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "--list", "--strict", "in.procreate"), 0,
		"input: in.procreate\nsegments: 2\n\n"+
			"1 video/segments/segment-1.mp4\n3 video/segments/segment-3.mp4\n",
		gapWarn("2"))
}

// --verify does honor --strict: the gap is fatal and no report is emitted.
func TestVerifyStrictGap(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	delete(entries, "video/segments/segment-2.mp4")
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "--verify", "--strict", "in.procreate"), 5, "",
		actionErr("segment numbers missing: 2 (the video would have gaps)"))
}

func TestListDirectory(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	writeArchive(t, filepath.Join(dir, "input"), "b.procreate", stdArchiveEntries(0))
	wantOut := "input: input/a.procreate\nsegments: 3\n" +
		"\n" +
		"1 video/segments/segment-1.mp4\n" +
		"2 video/segments/segment-2.mp4\n" +
		"3 video/segments/segment-3.mp4\n" +
		"\n" +
		"input: input/b.procreate\nsegments: 0\n"
	check(t, run(t, dir, nil, "--list", "input"), 0, wantOut,
		slogLine(slog.LevelWarn, "no timelapse video inside", "input", "input/b.procreate"))
}

func TestVerifySingle(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	sum := streamsOf(t, segN(1))
	wantOut := "input: in.procreate\nsegments: 3\n\n" +
		fmt.Sprintf("1 ok    %s  2.00s  video/segments/segment-1.mp4\n", sum) +
		fmt.Sprintf("2 ok    %s  2.00s  video/segments/segment-2.mp4\n", sum) +
		fmt.Sprintf("3 ok    %s  2.00s  video/segments/segment-3.mp4\n", sum) +
		"\nverify: ok, 3 segment(s), ~6.0 s of video"
	check(t, run(t, dir, nil, "--verify", "in.procreate"), 0, wantOut, "")
}

func TestVerifyCorrupt(t *testing.T) {
	dir := t.TempDir()
	writeArchiveCorrupted(t, dir, "in.procreate", stdThree(), "video/segments/segment-1.mp4")
	sum := streamsOf(t, segN(2))
	wantOut := "input: in.procreate\nsegments: 3\n\n" +
		"1 FAIL  segment video/segments/segment-1.mp4 is corrupted inside the archive: zip: checksum error\n" +
		fmt.Sprintf("2 ok    %s  2.00s  video/segments/segment-2.mp4\n", sum) +
		fmt.Sprintf("3 ok    %s  2.00s  video/segments/segment-3.mp4\n", sum)
	check(t, run(t, dir, nil, "--verify", "in.procreate"), 5, wantOut,
		actionErr("verify failed: 1 of 3 segment(s) are bad"))
}

func TestVerifyIncompatible(t *testing.T) {
	dir := t.TempDir()
	a := incompatibleSegA()
	b := incompatibleSegB()
	entries := map[string][]byte{
		"Document/document.data":       []byte("fake-document-data"),
		"video/segments/":              {},
		"video/segments/segment-1.mp4": a,
		"video/segments/segment-2.mp4": b,
	}
	writeArchive(t, dir, "in.procreate", entries)
	sumA, sumB := streamsOf(t, a), streamsOf(t, b)
	wantOut := "input: in.procreate\nsegments: 2\n\n" +
		fmt.Sprintf("1 ok    %s  2.00s  video/segments/segment-1.mp4\n", sumA) +
		fmt.Sprintf("2 ok    %s  2.00s  video/segments/segment-2.mp4\n", sumB)
	check(t, run(t, dir, nil, "--verify", "in.procreate"), 7, wantOut,
		actionErr("segments have different stream parameters, so they cannot be joined with stream copy (-c copy):\n"+
			fmt.Sprintf("  video/segments/segment-1.mp4: %s\n", sumA)+
			fmt.Sprintf("  video/segments/segment-2.mp4: %s", sumB)))
}

func TestVerifyDirectoryMixed(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	writeArchiveCorrupted(t, filepath.Join(dir, "input"), "c.procreate", stdThree(),
		"video/segments/segment-1.mp4")
	sum := streamsOf(t, segN(1))
	reportA := "input: input/a.procreate\nsegments: 3\n\n" +
		fmt.Sprintf("1 ok    %s  2.00s  video/segments/segment-1.mp4\n", sum) +
		fmt.Sprintf("2 ok    %s  2.00s  video/segments/segment-2.mp4\n", sum) +
		fmt.Sprintf("3 ok    %s  2.00s  video/segments/segment-3.mp4\n", sum) +
		"\nverify: ok, 3 segment(s), ~6.0 s of video"
	reportC := "input: input/c.procreate\nsegments: 3\n\n" +
		"1 FAIL  segment video/segments/segment-1.mp4 is corrupted inside the archive: zip: checksum error\n" +
		fmt.Sprintf("2 ok    %s  2.00s  video/segments/segment-2.mp4\n", sum) +
		fmt.Sprintf("3 ok    %s  2.00s  video/segments/segment-3.mp4\n", sum)
	check(t, run(t, dir, nil, "--verify", "input"), 1, reportA+"\n"+reportC,
		slogLine(slog.LevelError, "diagnosis failed", "input", "input/c.procreate",
			"err", "verify failed: 1 of 3 segment(s) are bad"))
}
