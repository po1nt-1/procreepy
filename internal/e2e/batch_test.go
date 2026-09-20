package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// stdInput fills dir/input with a.procreate and b.procreate (both 3 segments).
func stdInput(t *testing.T, dir string) {
	t.Helper()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	writeArchive(t, filepath.Join(dir, "input"), "b.procreate", stdThree())
}

func TestBatchBasic(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	wantErr := batchStart(2, "input", "output/timelaps") +
		batchConverted("input/a.procreate", "output/timelaps/a.mp4") +
		batchConverted("input/b.procreate", "output/timelaps/b.mp4") +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
	eqBytes(t, "a.mp4", readAll(t, filepath.Join(dir, "output", "timelaps", "a.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	eqBytes(t, "b.mp4", readAll(t, filepath.Join(dir, "output", "timelaps", "b.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
}

func TestBatchExplicitOut(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	wantErr := batchStart(2, "input", "vids") +
		batchConverted("input/a.procreate", "vids/a.mp4") +
		batchConverted("input/b.procreate", "vids/b.mp4") +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input", "vids"), 0, "", wantErr)
	if _, err := os.Stat(filepath.Join(dir, "vids", "a.mp4")); err != nil {
		t.Error(err)
	}
}

// A trailing slash is passed through verbatim into the header (quirk).
func TestBatchTrailingSlashOut(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	wantErr := batchStart(2, "input", "vids/") +
		batchConverted("input/a.procreate", "vids/a.mp4") +
		batchConverted("input/b.procreate", "vids/b.mp4") +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input", "vids/"), 0, "", wantErr)
}

func TestBatchRecursive(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input", "sub"), "c.procreate", stdThree())
	writeArchive(t, filepath.Join(dir, "input", "sub", "deep"), "d.procreate", stdThree())
	wantErr := batchStart(2, "input", "output/timelaps") +
		batchConverted("input/sub/c.procreate", "output/timelaps/sub/c.mp4") +
		batchConverted("input/sub/deep/d.procreate", "output/timelaps/sub/deep/d.mp4") +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "-r", "input"), 0, "", wantErr)
	if _, err := os.Stat(filepath.Join(dir, "output", "timelaps", "sub", "deep", "d.mp4")); err != nil {
		t.Error(err)
	}
}

func TestBatchSkipAndForce(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	first := batchStart(1, "input", "output/timelaps") +
		batchConverted("input/a.procreate", "output/timelaps/a.mp4") +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", first)

	second := batchStart(1, "input", "output/timelaps") +
		batchSkipped("input/a.procreate", "output/timelaps/a.mp4") +
		batchDone(0, 1, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", second)

	third := batchStart(1, "input", "output/timelaps") +
		batchConverted("input/a.procreate", "output/timelaps/a.mp4") +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "-f", "input"), 0, "", third)
}

func TestBatchMixedFailures(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input")
	writeArchive(t, in, "a.procreate", stdThree())
	writeArchive(t, in, "b.procreate", stdArchiveEntries(0))
	if err := os.WriteFile(filepath.Join(in, "c.procreate"), []byte("definitely not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArchiveCorrupted(t, in, "d.procreate", stdThree(), "video/segments/segment-1.mp4")

	wantErr := batchStart(4, "input", "output/timelaps") +
		batchConverted("input/a.procreate", "output/timelaps/a.mp4") +
		batchNoVideo("input/b.procreate") +
		batchFailed("input/c.procreate",
			"input is not a valid ZIP archive: input/c.procreate (not a .procreate file, or truncated/corrupted)") +
		batchFailed("input/d.procreate",
			"segment video/segments/segment-1.mp4 is corrupted inside the archive: zip: checksum error") +
		batchDone(1, 0, 1, 2)
	check(t, run(t, dir, nil, "input"), 1, "", wantErr)
	eqBytes(t, "a.mp4", readAll(t, filepath.Join(dir, "output", "timelaps", "a.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	if _, err := os.Stat(filepath.Join(dir, "output", "timelaps", "b.mp4")); !os.IsNotExist(err) {
		t.Error("b.mp4 should not exist")
	}
}

// Only no-video files are a soft skip: the batch still succeeds.
func TestBatchOnlyNoSegments(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "b.procreate", stdArchiveEntries(0))
	wantErr := batchStart(1, "input", "output/timelaps") +
		batchNoVideo("input/b.procreate") +
		batchDone(0, 0, 1, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
}

func TestBatchStdoutRejected(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	check(t, run(t, dir, nil, "input", "-"), 2, "",
		actionErr("cannot write several videos to stdout; give an output directory (default: output/timelaps/)"))
}

func TestBatchOutputIsFile(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "weird.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	check(t, run(t, dir, nil, "input", "weird.mp4"), 2, "",
		actionErr("output path exists and is not a directory: weird.mp4"))
}

func TestBatchEmpty(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "input"), 0o755); err != nil {
			t.Fatal(err)
		}
		check(t, run(t, dir, nil, "input"), 3, "",
			actionErr("no .procreate files found in input (use -r to look in sub-directories)"))
	})
	t.Run("recursive", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "input"), 0o755); err != nil {
			t.Fatal(err)
		}
		check(t, run(t, dir, nil, "-r", "input"), 3, "",
			actionErr("no .procreate files found in input"))
	})
}

// AppleDouble resource forks (._*) are not treated as archives.
func TestBatchSkipsDotfiles(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "input")
	if err := os.MkdirAll(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "._a.procreate"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeArchive(t, in, "a.procreate", stdThree())
	wantErr := batchStart(1, "input", "output/timelaps") +
		batchConverted("input/a.procreate", "output/timelaps/a.mp4") +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
}

// --split works per-file inside a batch; the slimmed line precedes the arrow.
func TestBatchSplit(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	removed := int64(len(segN(1)) + len(segN(2)) + len(segN(3)))
	// prepare() absolutizes the output path, so the slim line shows abs paths.
	slim := filepath.Join(dir, "output", "timelaps", "a.procreepy.procreate")
	wantErr := batchStart(1, "input", "output/timelaps") +
		batchSlimmed(slim, 4, removed) +
		batchConverted("input/a.procreate", "output/timelaps/a.mp4") +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "--split", "input"), 0, "", wantErr)
	if _, err := os.Stat(filepath.Join(dir, "output", "timelaps", "a.procreepy.procreate")); err != nil {
		t.Error("slimmed archive missing:", err)
	}
}
