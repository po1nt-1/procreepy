package e2e

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// stdInput fills dir/input with a.procreate and b.procreate (both 3 segments).
func stdInput(t *testing.T, dir string) {
	t.Helper()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	writeArchive(t, filepath.Join(dir, "input"), "b.procreate", stdThree())
}

// stdSize is the size the batch reports for the three std segments: the
// testkit stores members, so compressed equals raw.
func stdSize() int64 {
	return int64(len(segN(1)) + len(segN(2)) + len(segN(3)))
}

// stdKeep are the members of a std archive that survive the slimming.
func stdKeep() map[string][]byte {
	return map[string][]byte{
		"Document/document.data": []byte("fake-document-data"),
		"video/segments/":        {},
	}
}

// readZipMembers returns each member of the archive path keyed by name.
func readZipMembers(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer zr.Close()
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("%s: open member %s: %v", path, f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: read member %s: %v", path, f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

// assertProjectContents pins a slimmed project: exactly the members in keep
// are present, each byte-identical with the source; nothing else survives.
func assertProjectContents(t *testing.T, path string, keep map[string][]byte) {
	t.Helper()
	got := readZipMembers(t, path)
	if len(got) != len(keep) {
		t.Errorf("%s: members = %v, want %v", path, sortedNames(got), sortedNames(keep))
		return
	}
	for name, b := range keep {
		gb, ok := got[name]
		if !ok {
			t.Errorf("%s: missing member %s", path, name)
			continue
		}
		if !bytes.Equal(gb, b) {
			t.Errorf("%s: member %s differs (%d bytes, want %d)", path, name, len(gb), len(b))
		}
	}
}

func sortedNames(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestBatchBasic(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	wantErr := batchStart(2, "input", "output/timelapses", "output/projects") +
		batchConverted("input/a.procreate", "output/timelapses/a.mp4", "output/projects/a.procreepy.procreate", 3, stdSize()) +
		batchConverted("input/b.procreate", "output/timelapses/b.mp4", "output/projects/b.procreepy.procreate", 3, stdSize()) +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
	eqBytes(t, "a.mp4", readAll(t, filepath.Join(dir, "output", "timelapses", "a.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	eqBytes(t, "b.mp4", readAll(t, filepath.Join(dir, "output", "timelapses", "b.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	assertProjectContents(t, filepath.Join(dir, "output", "projects", "a.procreepy.procreate"), stdKeep())
	assertProjectContents(t, filepath.Join(dir, "output", "projects", "b.procreepy.procreate"), stdKeep())
}

func TestBatchExplicitOut(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	wantErr := batchStart(2, "input", "vids/timelapses", "vids/projects") +
		batchConverted("input/a.procreate", "vids/timelapses/a.mp4", "vids/projects/a.procreepy.procreate", 3, stdSize()) +
		batchConverted("input/b.procreate", "vids/timelapses/b.mp4", "vids/projects/b.procreepy.procreate", 3, stdSize()) +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input", "vids"), 0, "", wantErr)
	if _, err := os.Stat(filepath.Join(dir, "vids", "timelapses", "a.mp4")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vids", "projects", "a.procreepy.procreate")); err != nil {
		t.Error(err)
	}
}

// A trailing slash on OUTPUT is normalized away by path joining: the header
// shows the canonical tree names, not the verbatim spelling.
func TestBatchTrailingSlashOut(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	wantErr := batchStart(2, "input", "vids/timelapses", "vids/projects") +
		batchConverted("input/a.procreate", "vids/timelapses/a.mp4", "vids/projects/a.procreepy.procreate", 3, stdSize()) +
		batchConverted("input/b.procreate", "vids/timelapses/b.mp4", "vids/projects/b.procreepy.procreate", 3, stdSize()) +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input", "vids/"), 0, "", wantErr)
}

func TestBatchRecursive(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input", "sub"), "c.procreate", stdThree())
	writeArchive(t, filepath.Join(dir, "input", "sub", "deep"), "d.procreate", stdThree())
	wantErr := batchStart(2, "input", "output/timelapses", "output/projects") +
		batchConverted("input/sub/c.procreate", "output/timelapses/sub/c.mp4", "output/projects/sub/c.procreepy.procreate", 3, stdSize()) +
		batchConverted("input/sub/deep/d.procreate", "output/timelapses/sub/deep/d.mp4", "output/projects/sub/deep/d.procreepy.procreate", 3, stdSize()) +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "-r", "input"), 0, "", wantErr)
	if _, err := os.Stat(filepath.Join(dir, "output", "timelapses", "sub", "deep", "d.mp4")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "output", "projects", "sub", "deep", "d.procreepy.procreate")); err != nil {
		t.Error(err)
	}
}

func TestBatchSkipAndForce(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	tl := "output/timelapses/a.mp4"
	pr := "output/projects/a.procreepy.procreate"
	start := batchStart(1, "input", "output/timelapses", "output/projects")
	conv := batchConverted("input/a.procreate", tl, pr, 3, stdSize())

	check(t, run(t, dir, nil, "input"), 0, "", start+conv+batchDone(1, 0, 0, 0))

	// Both halves of the pair exist: the input is skipped untouched.
	check(t, run(t, dir, nil, "input"), 0, "",
		start+batchSkipped("input/a.procreate", tl, pr)+batchDone(0, 1, 0, 0))

	// --force rewrites everything unconditionally.
	check(t, run(t, dir, nil, "-f", "input"), 0, "", start+conv+batchDone(1, 0, 0, 0))
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

	wantErr := batchStart(4, "input", "output/timelapses", "output/projects") +
		batchConverted("input/a.procreate", "output/timelapses/a.mp4", "output/projects/a.procreepy.procreate", 3, stdSize()) +
		batchNoVideo("input/b.procreate") +
		batchFailed("input/c.procreate",
			"input is not a valid ZIP archive: "+fsPath("input/c.procreate")+
				" (not a .procreate file, or truncated/corrupted)") +
		batchFailed("input/d.procreate",
			"segment video/segments/segment-1.mp4 is corrupted inside the archive: zip: checksum error") +
		batchDone(1, 0, 1, 2)
	check(t, run(t, dir, nil, "input"), 1, "", wantErr)
	eqBytes(t, "a.mp4", readAll(t, filepath.Join(dir, "output", "timelapses", "a.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	if _, err := os.Stat(filepath.Join(dir, "output", "timelapses", "b.mp4")); !os.IsNotExist(err) {
		t.Error("b.mp4 should not exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "output", "projects", "b.procreepy.procreate")); !os.IsNotExist(err) {
		t.Error("b's project should not exist")
	}
}

// Only no-video files are a soft skip: the batch still succeeds.
func TestBatchOnlyNoSegments(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "b.procreate", stdArchiveEntries(0))
	wantErr := batchStart(1, "input", "output/timelapses", "output/projects") +
		batchNoVideo("input/b.procreate") +
		batchDone(0, 0, 1, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
}

func TestBatchStdoutRejected(t *testing.T) {
	dir := t.TempDir()
	stdInput(t, dir)
	check(t, run(t, dir, nil, "input", "-"), 2, "",
		actionErr("cannot write a directory of results to stdout; give an output directory (default: output/)"))
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
	wantErr := batchStart(1, "input", "output/timelapses", "output/projects") +
		batchConverted("input/a.procreate", "output/timelapses/a.mp4", "output/projects/a.procreepy.procreate", 3, stdSize()) +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
}

// The slimmed project keeps every member that is not a timelapse segment,
// byte for byte; the segments (and only the segments) disappear.
func TestBatchProjectContent(t *testing.T) {
	dir := t.TempDir()
	entries := map[string][]byte{
		"Document/document.data":       []byte("precious document"),
		"Document/archive":             []byte("more precious bytes"),
		"video/segments/":              {},
		"video/active-0.mp4":           {},
		"video/other-metadata.dat":     []byte("surviving metadata"),
		"video/segments/segment-1.mp4": segN(1),
		"video/segments/segment-2.mp4": segN(2),
	}
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", entries)
	size := int64(len(segN(1)) + len(segN(2)))
	wantErr := batchStart(1, "input", "output/timelapses", "output/projects") +
		batchConverted("input/a.procreate", "output/timelapses/a.mp4", "output/projects/a.procreepy.procreate", 2, size) +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)

	keep := map[string][]byte{}
	for name, b := range entries {
		if strings.HasPrefix(name, "video/segments/segment-") && strings.HasSuffix(name, ".mp4") {
			continue
		}
		keep[name] = b
	}
	assertProjectContents(t, filepath.Join(dir, "output", "projects", "a.procreepy.procreate"), keep)
}

// The published project carries the source's modification time, so a re-run
// after a copy into Procreate keeps the artwork where it stood in the gallery.
func TestBatchPreservesMtime(t *testing.T) {
	dir := t.TempDir()
	in := writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	when := time.Date(2021, 3, 4, 5, 6, 7, 123456789, time.UTC)
	if err := os.Chtimes(in, when, when); err != nil {
		t.Fatal(err)
	}
	wantErr := batchStart(1, "input", "output/timelapses", "output/projects") +
		batchConverted("input/a.procreate", "output/timelapses/a.mp4", "output/projects/a.procreepy.procreate", 3, stdSize()) +
		batchDone(1, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)

	st, err := os.Stat(filepath.Join(dir, "output", "projects", "a.procreepy.procreate"))
	if err != nil {
		t.Fatal(err)
	}
	diff := st.ModTime().Sub(when)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Errorf("project mtime = %s, want the source mtime %s", st.ModTime(), when)
	}
}

// Case-collision: the first name in byte order keeps the stem, the other gets
// the -2 suffix. On a case-sensitive filesystem both archives coexist and
// ReadDir sorts "A.procreate" before "a.procreate".
func TestBatchCaseCollision(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "A.procreate", stdThree())
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	wantErr := batchStart(2, "input", "output/timelapses", "output/projects") +
		batchConverted("input/A.procreate", "output/timelapses/A.mp4", "output/projects/A.procreepy.procreate", 3, stdSize()) +
		batchConverted("input/a.procreate", "output/timelapses/a-2.mp4", "output/projects/a-2.procreepy.procreate", 3, stdSize()) +
		batchDone(2, 0, 0, 0)
	check(t, run(t, dir, nil, "input"), 0, "", wantErr)
	for _, p := range []string{
		filepath.Join("output", "timelapses", "A.mp4"),
		filepath.Join("output", "timelapses", "a-2.mp4"),
		filepath.Join("output", "projects", "A.procreepy.procreate"),
		filepath.Join("output", "projects", "a-2.procreepy.procreate"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Error(p, ":", err)
		}
	}
}

// Losing one half of the output pair means the input is not done: the batch
// says so loudly and regenerates the whole set. --force never consults the
// existing state at all.
func TestBatchIncompleteRegenerates(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, filepath.Join(dir, "input"), "a.procreate", stdThree())
	tl := "output/timelapses/a.mp4"
	pr := "output/projects/a.procreepy.procreate"
	start := batchStart(1, "input", "output/timelapses", "output/projects")
	conv := batchConverted("input/a.procreate", tl, pr, 3, stdSize())
	check(t, run(t, dir, nil, "input"), 0, "", start+conv+batchDone(1, 0, 0, 0))

	if err := os.Remove(filepath.Join(dir, tl)); err != nil {
		t.Fatal(err)
	}
	check(t, run(t, dir, nil, "input"), 0, "",
		start+batchIncomplete("input/a.procreate")+conv+batchDone(1, 0, 0, 0))

	if err := os.Remove(filepath.Join(dir, pr)); err != nil {
		t.Fatal(err)
	}
	check(t, run(t, dir, nil, "-f", "input"), 0, "", start+conv+batchDone(1, 0, 0, 0))
}
