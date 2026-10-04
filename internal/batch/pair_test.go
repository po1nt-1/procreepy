package batch

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"procreepy/internal/testkit"
	"procreepy/internal/video"
)

// assertNoPartials fails if any scratch file survived under root.
func assertNoPartials(t *testing.T, root string) {
	t.Helper()
	var left []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		n := d.Name()
		if strings.HasSuffix(n, ".partial") || strings.HasSuffix(n, ".tmp") ||
			strings.HasPrefix(n, ".procreepy-") {
			left = append(left, p)
		}
		return nil
	})
	if len(left) > 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

// writeProject writes a synthetic project with n segments and the given mtime.
func writeProject(t *testing.T, dir, name string, n int, mtime time.Time) string {
	t.Helper()
	p := testkit.Project(n).Write(t, dir, name)
	if !mtime.IsZero() {
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func run(t *testing.T, in, out string, cfg video.Config, force, recursive bool) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code, err := ConvertDirectory(context.Background(), bufLogger(&buf), in, out, cfg, force, recursive)
	if err != nil {
		t.Fatalf("ConvertDirectory: %v\n%s", err, buf.String())
	}
	return code, buf.String()
}

// TestProjectKeepsSourceTimestamp is the whole point of the project output: the
// re-importable archive must look as old as the artwork it came from, or
// Procreate reorders the gallery on import.
func TestProjectKeepsSourceTimestamp(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	want := time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)
	src := writeProject(t, in, "Cat.procreate", 2, want)

	if code, log := run(t, in, out, video.Config{}, false, false); code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	proj := filepath.Join(out, ProjectDir, "Cat"+ProjectSuffix)
	st, err := os.Stat(proj)
	if err != nil {
		t.Fatal(err)
	}
	if d := st.ModTime().Sub(want); d > time.Second || d < -time.Second {
		t.Errorf("project mtime = %v, want %v", st.ModTime(), want)
	}
	// The source itself must not have moved.
	sst, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if d := sst.ModTime().Sub(want); d > time.Second || d < -time.Second {
		t.Errorf("source mtime changed to %v", sst.ModTime())
	}
	assertNoPartials(t, out)
}

// TestIncompleteOutputSetIsRegenerated: an earlier run that produced the MP4 but
// not the project left the input half-done, and a skip keyed on a single file
// would leave it that way forever.
func TestIncompleteOutputSetIsRegenerated(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})

	if code, log := run(t, in, out, video.Config{}, false, false); code != 0 {
		t.Fatalf("first run: code=%d\n%s", code, log)
	}
	proj := filepath.Join(out, ProjectDir, "Cat"+ProjectSuffix)
	if err := os.Remove(proj); err != nil {
		t.Fatal(err)
	}

	code, log := run(t, in, out, video.Config{}, false, false)
	if code != 0 {
		t.Fatalf("second run: code=%d\n%s", code, log)
	}
	if !strings.Contains(log, "outputs are incomplete, regenerating the whole set") {
		t.Errorf("expected the half-done input to be regenerated:\n%s", log)
	}
	if _, err := os.Stat(proj); err != nil {
		t.Errorf("project was not restored: %v", err)
	}
	if !strings.Contains(log, "converted=1") {
		t.Errorf("summary should count it as converted:\n%s", log)
	}
}

// TestCompleteOutputSetIsSkipped is the other half of the same rule.
func TestCompleteOutputSetIsSkipped(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})
	run(t, in, out, video.Config{}, false, false)

	_, log := run(t, in, out, video.Config{}, false, false)
	if !strings.Contains(log, "existed=1") || !strings.Contains(log, "converted=0") {
		t.Errorf("second run should skip everything:\n%s", log)
	}
}

// TestForceOverwritesBothOutputs checks --force replaces existing files in both
// trees, which on Windows depends on rename replacing an existing target.
func TestForceOverwritesBothOutputs(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})
	run(t, in, out, video.Config{}, false, false)

	mp4 := filepath.Join(out, TimelapseDir, "Cat.mp4")
	proj := filepath.Join(out, ProjectDir, "Cat"+ProjectSuffix)
	// Replace both with junk, so a run that does not overwrite is visible.
	for _, p := range []string{mp4, proj} {
		if err := os.WriteFile(p, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if code, log := run(t, in, out, video.Config{}, true, false); code != 0 {
		t.Fatalf("force run: code=%d\n%s", code, log)
	}
	for _, p := range []string{mp4, proj} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) == "stale" {
			t.Errorf("%s was not overwritten by --force", p)
		}
	}
	assertNoPartials(t, out)
}

// TestOutputInsideInputRejected: with -r the projects written into the output
// tree are .procreate files, so a nested output would feed them back as inputs.
func TestOutputInsideInputRejected(t *testing.T) {
	in := t.TempDir()
	writeProject(t, in, "Cat.procreate", 1, time.Time{})
	out := filepath.Join(in, "out")

	_, err := ConvertDirectory(context.Background(), bufLogger(&bytes.Buffer{}), in, out,
		video.Config{}, false, true)
	if err == nil {
		t.Fatal("want an error for an output directory inside the input tree")
	}
	if !strings.Contains(err.Error(), "inside the input tree") {
		t.Fatalf("err = %v", err)
	}
}

// TestRecursiveMirrorsBothTrees checks one relative path drives both outputs.
func TestRecursiveMirrorsBothTrees(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, filepath.Join(in, "2025"), "Cat.procreate", 2, time.Time{})

	if code, log := run(t, in, out, video.Config{}, false, true); code != 0 {
		t.Fatalf("code=%d\n%s", code, log)
	}
	for _, p := range []string{
		filepath.Join(out, TimelapseDir, "2025", "Cat.mp4"),
		filepath.Join(out, ProjectDir, "2025", "Cat"+ProjectSuffix),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
}

// TestSpacesAndUnicodePaths covers the names Windows users actually have.
func TestSpacesAndUnicodePaths(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out dir (1)")
	names := []string{"My Artwork (2).procreate", "пельмеши.procreate", "日本語.procreate"}
	for _, n := range names {
		writeProject(t, in, n, 1, time.Time{})
	}
	if code, log := run(t, in, out, video.Config{}, false, false); code != 0 {
		t.Fatalf("code=%d\n%s", code, log)
	}
	for _, n := range names {
		st := strings.TrimSuffix(n, ".procreate")
		for _, p := range []string{
			filepath.Join(out, TimelapseDir, st+".mp4"),
			filepath.Join(out, ProjectDir, st+ProjectSuffix),
		} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("missing %s: %v", p, err)
			}
		}
	}
	assertNoPartials(t, out)
}

// TestCancelledBatchLeavesNoPartials: cancelling mid-run must report
// context.Canceled and leave nothing half-written behind.
func TestCancelledBatchLeavesNoPartials(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	for _, n := range []string{"a.procreate", "b.procreate", "c.procreate"} {
		writeProject(t, in, n, 3, time.Time{})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	_, err := ConvertDirectory(ctx, bufLogger(&buf), in, out, video.Config{}, false, false)
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertNoPartials(t, out)
	for _, tree := range []string{TimelapseDir, ProjectDir} {
		ents, err := os.ReadDir(filepath.Join(out, tree))
		if err != nil {
			continue
		}
		if len(ents) != 0 {
			t.Errorf("%s is not empty after cancellation: %v", tree, ents)
		}
	}
}

// TestNoTimelapseProducesNoOutputs: an artwork recorded with the timelapse
// turned off yields neither an MP4 nor a project.
func TestNoTimelapseProducesNoOutputs(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "Document.archive", Data: []byte("doc")},
	}}.Write(t, in, "Quiet.procreate")

	code, log := run(t, in, out, video.Config{}, false, false)
	if code != 0 {
		t.Fatalf("code = %d, want 0\n%s", code, log)
	}
	if !strings.Contains(log, "no timelapse video inside, skipped") {
		t.Errorf("log:\n%s", log)
	}
	for _, p := range []string{
		filepath.Join(out, TimelapseDir, "Quiet.mp4"),
		filepath.Join(out, ProjectDir, "Quiet"+ProjectSuffix),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s should not exist", p)
		}
	}
	assertNoPartials(t, out)
}

// TestCorruptInputProducesNoOutputs: a failure must not publish a partial pair.
func TestCorruptInputProducesNoOutputs(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(filepath.Join(in, "Bad.procreate"),
		[]byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, log := run(t, in, out, video.Config{}, false, false)
	if code != 1 {
		t.Fatalf("code = %d, want 1\n%s", code, log)
	}
	for _, p := range []string{
		filepath.Join(out, TimelapseDir, "Bad.mp4"),
		filepath.Join(out, ProjectDir, "Bad"+ProjectSuffix),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s should not exist", p)
		}
	}
	assertNoPartials(t, out)
}

// TestPSDExport checks --psd adds a third tree without disturbing the other two.
func TestPSDExport(t *testing.T) {
	in, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeProject(t, in, "Cat.procreate", 2, time.Time{})

	code, log := run(t, in, out, video.Config{PSD: true}, false, false)
	if code != 0 {
		t.Fatalf("code = %d\n%s", code, log)
	}
	psd := filepath.Join(out, PSDDir, "Cat.psd")
	st, err := os.Stat(psd)
	if err != nil {
		t.Fatalf("missing PSD: %v", err)
	}
	if st.Size() < 100 {
		t.Errorf("PSD is only %d bytes", st.Size())
	}
	if !strings.Contains(log, "psd exported") {
		t.Errorf("log:\n%s", log)
	}
	for _, p := range []string{
		filepath.Join(out, TimelapseDir, "Cat.mp4"),
		filepath.Join(out, ProjectDir, "Cat"+ProjectSuffix),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	assertNoPartials(t, out)
}
