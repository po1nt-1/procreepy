package batch

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"procreepy/internal/video"
)

// zipNames lists the entry names of an archive.
func zipNames(t *testing.T, path string) []string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer zr.Close()
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

// TestProjectsArePackedForTransfer is the default path: getting a folder of
// projects back onto an iPad is one transfer, so the projects end up in a
// single archive with a folder inside it, and the loose directory is gone.
func TestProjectsArePackedForTransfer(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})
	writeProject(t, in, "Dog.procreate", 2, time.Time{})

	code, log := run(t, in, out, video.Config{}, false, false, true)
	if code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	if !strings.Contains(log, "projects packed for transfer") {
		t.Errorf("log:\n%s", log)
	}

	archive := filepath.Join(out, ProjectArchiveName)
	got := zipNames(t, archive)
	want := []string{
		ProjectDir + "/Cat" + ProjectSuffix,
		ProjectDir + "/Dog" + ProjectSuffix,
	}
	if strings.Join(sortedStrings(got), "|") != strings.Join(want, "|") {
		t.Errorf("entries = %v, want %v", got, want)
	}

	// The directory must be gone: leaving both would double the space the
	// projects take and make it unclear which one is the deliverable.
	if _, err := os.Stat(filepath.Join(out, ProjectDir)); !os.IsNotExist(err) {
		t.Errorf("project directory should have been removed, stat err = %v", err)
	}
	// The videos are untouched by packing.
	if _, err := os.Stat(filepath.Join(out, TimelapseDir, "Cat.mp4")); err != nil {
		t.Errorf("timelapse missing: %v", err)
	}
	assertNoPartials(t, out)
}

// TestPackedProjectsAreStored: a .procreate file is itself a compressed zip, so
// deflating it again costs time and saves nothing.
func TestPackedProjectsAreStored(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})

	if code, log := run(t, in, out, video.Config{}, false, false, true); code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	zr, err := zip.OpenReader(filepath.Join(out, ProjectArchiveName))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Method != zip.Store {
			t.Errorf("%s: method = %d, want Store", f.Name, f.Method)
		}
	}
}

// TestPackedProjectKeepsItsDate: the archive is how the projects reach the
// tablet, so the dates have to survive the trip inside it.
func TestPackedProjectKeepsItsDate(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	want := time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)
	writeProject(t, in, "Cat.procreate", 2, want)

	if code, log := run(t, in, out, video.Config{}, false, false, true); code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	zr, err := zip.OpenReader(filepath.Join(out, ProjectArchiveName))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		t.Fatalf("entries = %d, want 1", len(zr.File))
	}
	if d := zr.File[0].Modified.Sub(want); d > time.Second || d < -time.Second {
		t.Errorf("entry mtime = %v, want %v", zr.File[0].Modified, want)
	}
}

// TestRerunResumesFromTheArchive: packing removes the directory, so without
// consulting the archive a second run would see every project missing and
// rebuild all of them — the opposite of the skip behavior everywhere else.
func TestRerunResumesFromTheArchive(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})

	if code, log := run(t, in, out, video.Config{}, false, false, true); code != 0 {
		t.Fatalf("first run: code = %d\n%s", code, log)
	}
	_, log := run(t, in, out, video.Config{}, false, false, true)
	if !strings.Contains(log, "existed=1") || !strings.Contains(log, "converted=0") {
		t.Errorf("second run should skip the packed artwork:\n%s", log)
	}
	// And the archive must survive a run that produced nothing new.
	if _, err := os.Stat(filepath.Join(out, ProjectArchiveName)); err != nil {
		t.Errorf("archive lost on the second run: %v", err)
	}
}

// TestPackingAddsToAnExistingArchive: a new artwork joins the ones already
// packed instead of replacing them.
func TestPackingAddsToAnExistingArchive(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})
	if code, log := run(t, in, out, video.Config{}, false, false, true); code != 0 {
		t.Fatalf("first run: code = %d\n%s", code, log)
	}

	writeProject(t, in, "Dog.procreate", 2, time.Time{})
	if code, log := run(t, in, out, video.Config{}, false, false, true); code != 0 {
		t.Fatalf("second run: code = %d\n%s", code, log)
	}
	got := sortedStrings(zipNames(t, filepath.Join(out, ProjectArchiveName)))
	want := []string{
		ProjectDir + "/Cat" + ProjectSuffix,
		ProjectDir + "/Dog" + ProjectSuffix,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("entries = %v, want %v", got, want)
	}
}

// TestFailureKeepsTheProjectDirectory: with failures on the board the directory
// is the material for diagnosing and resuming, so it is not replaced.
func TestFailureKeepsTheProjectDirectory(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})
	if err := os.WriteFile(filepath.Join(in, "Bad.procreate"),
		[]byte("not a zip at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, log := run(t, in, out, video.Config{}, false, false, true)
	if code != 1 {
		t.Fatalf("code = %d, want 1\n%s", code, log)
	}
	if strings.Contains(log, "projects packed for transfer") {
		t.Errorf("a run with failures must not pack:\n%s", log)
	}
	if _, err := os.Stat(filepath.Join(out, ProjectDir, "Cat"+ProjectSuffix)); err != nil {
		t.Errorf("the project that did convert should still be on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, ProjectArchiveName)); !os.IsNotExist(err) {
		t.Errorf("no archive expected, stat err = %v", err)
	}
}

// TestPackingMirrorsSubdirectories: with -r the archive keeps the structure, so
// unpacking on the tablet reproduces the folders.
func TestPackingMirrorsSubdirectories(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, filepath.Join(in, "2025"), "Cat.procreate", 2, time.Time{})

	if code, log := run(t, in, out, video.Config{}, false, true, true); code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	got := zipNames(t, filepath.Join(out, ProjectArchiveName))
	want := ProjectDir + "/2025/Cat" + ProjectSuffix
	if len(got) != 1 || got[0] != want {
		t.Errorf("entries = %v, want [%s]", got, want)
	}
}

// TestNoZipLeavesTheDirectory is the opt-out.
func TestNoZipLeavesTheDirectory(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})

	if code, log := run(t, in, out, video.Config{}, false, false, false); code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	if _, err := os.Stat(filepath.Join(out, ProjectDir, "Cat"+ProjectSuffix)); err != nil {
		t.Errorf("project missing from the directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, ProjectArchiveName)); !os.IsNotExist(err) {
		t.Errorf("no archive expected, stat err = %v", err)
	}
}

// TestPackedProjectIsAValidArchive: the packed member must still be a readable
// .procreate, not just bytes of the right length — it is stored raw, and an
// off-by-one in the entry would only show up on import.
func TestPackedProjectIsAValidArchive(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})
	if code, log := run(t, in, out, video.Config{}, false, false, true); code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}

	outer, err := zip.OpenReader(filepath.Join(out, ProjectArchiveName))
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Close()
	if len(outer.File) != 1 {
		t.Fatalf("entries = %d, want 1", len(outer.File))
	}
	rc, err := outer.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	tmp := filepath.Join(t.TempDir(), "unpacked.procreate")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(f, rc); err != nil {
		t.Fatal(err)
	}
	f.Close()
	inner, err := zip.OpenReader(tmp)
	if err != nil {
		t.Fatalf("the packed project is not a readable zip: %v", err)
	}
	inner.Close()
}

// TestPackProjectsWithoutDirectory: a run in which everything was skipped
// produces no project tree, and packing must leave the archive alone rather
// than truncating it.
func TestPackProjectsWithoutDirectory(t *testing.T) {
	out := t.TempDir()
	n, err := packProjects(context.Background(), out)
	if err != nil {
		t.Fatalf("packProjects: %v", err)
	}
	if n != 0 {
		t.Errorf("projects = %d, want 0", n)
	}
	if _, err := os.Stat(filepath.Join(out, ProjectArchiveName)); !os.IsNotExist(err) {
		t.Errorf("no archive expected, stat err = %v", err)
	}
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
