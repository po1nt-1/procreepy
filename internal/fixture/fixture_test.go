// Package fixture holds the regression suite that runs against a real corpus of
// .procreate files. No binary fixtures are committed, so every test here is
// skipped unless PROCREATE_FIXTURE_DIR (a directory of projects) or
// PROCREATE_FIXTURE_ZIP (an archive of them) is set.
//
//	PROCREATE_FIXTURE_ZIP=/path/procshit.zip go test ./internal/fixture/ -v
package fixture

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"procreepy/internal/procreate"
)

var segmentRE = regexp.MustCompile(`(?i)^video/segments/segment-[0-9]+\.mp4$`)

// TestRealCorpusSplit is the end-to-end contract check on real data: it runs the
// real binary over a copy of the corpus and verifies every promise the tool
// makes about the projects it writes.
func TestRealCorpusSplit(t *testing.T) {
	src := corpus(t)
	bin := buildBinary(t)

	work := t.TempDir()
	in := filepath.Join(work, "in")
	out := filepath.Join(work, "out")
	inputs := stageCorpus(t, src, in)
	t.Logf("staged %d projects", len(inputs))

	// Record the originals so the run can be proved non-destructive.
	type orig struct {
		sum     string
		size    int64
		modTime time.Time
	}
	before := map[string]orig{}
	for _, p := range inputs {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		before[p] = orig{sum: fileSum(t, p), size: st.Size(), modTime: st.ModTime()}
	}

	cmd := exec.Command(bin, in, out)
	cmd.Dir = work
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstderr:\n%s", err, stderr.String())
	}

	// Originals must be untouched, bytes and timestamps alike.
	for _, p := range inputs {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		b := before[p]
		if got := fileSum(t, p); got != b.sum {
			t.Errorf("%s: input was modified", filepath.Base(p))
		}
		if !st.ModTime().Equal(b.modTime) {
			t.Errorf("%s: input mtime changed %v -> %v", filepath.Base(p), b.modTime, st.ModTime())
		}
	}

	projects := filepath.Join(out, "projects")
	timelapses := filepath.Join(out, "timelapses")
	if _, err := os.Stat(projects); err != nil {
		t.Fatalf("no projects tree: %v", err)
	}

	var withVideo, checked, noVideo int
	for _, p := range inputs {
		base := strings.TrimSuffix(filepath.Base(p), ".procreate")
		proj := filepath.Join(projects, base+".procreepy.procreate")
		mp4 := filepath.Join(timelapses, base+".mp4")

		hadVideo := countSegments(t, p) > 0
		if !hadVideo {
			noVideo++
			// An archive without a timelapse produces neither output.
			if _, err := os.Stat(proj); err == nil {
				t.Errorf("%s: has no timelapse but a project was written", base)
			}
			if _, err := os.Stat(mp4); err == nil {
				t.Errorf("%s: has no timelapse but an MP4 was written", base)
			}
			continue
		}
		withVideo++

		// The pair must exist together.
		mst, err := os.Stat(mp4)
		if err != nil {
			t.Errorf("%s: missing MP4: %v", base, err)
			continue
		}
		if mst.Size() == 0 {
			t.Errorf("%s: MP4 is empty", base)
		}
		pst, err := os.Stat(proj)
		if err != nil {
			t.Errorf("%s: missing project: %v", base, err)
			continue
		}

		// The project carries the source timestamp.
		if want := before[p].modTime; !sameModTime(pst.ModTime(), want) {
			t.Errorf("%s: project mtime = %v, want the source's %v", base, pst.ModTime(), want)
		}
		compareArchives(t, base, p, proj)
		checked++
	}
	t.Logf("projects with a timelapse: %d (verified %d), without: %d", withVideo, checked, noVideo)
	if withVideo == 0 {
		t.Fatal("no project in the corpus had a timelapse; the corpus looks wrong")
	}
}

// compareArchives asserts the slimmed project preserves every non-segment member
// exactly, down to the compressed bytes, and drops every segment.
func compareArchives(t *testing.T, label, srcPath, dstPath string) {
	t.Helper()
	sz, err := zip.OpenReader(srcPath)
	if err != nil {
		t.Errorf("%s: open source: %v", label, err)
		return
	}
	defer sz.Close()
	dz, err := zip.OpenReader(dstPath)
	if err != nil {
		t.Errorf("%s: the slimmed project is not a valid ZIP: %v", label, err)
		return
	}
	defer dz.Close()

	var wantNames []string
	wantByName := map[string]*zip.File{}
	for _, f := range sz.File {
		if segmentRE.MatchString(f.Name) {
			continue
		}
		wantNames = append(wantNames, f.Name)
		wantByName[f.Name] = f
	}
	var gotNames []string
	for _, f := range dz.File {
		gotNames = append(gotNames, f.Name)
		if segmentRE.MatchString(f.Name) {
			t.Errorf("%s: timelapse segment %s survived", label, f.Name)
		}
	}
	// Archive order must survive, not just the set of names.
	if !equalStrings(wantNames, gotNames) {
		t.Errorf("%s: member list/order changed\nwant %d entries, got %d", label, len(wantNames), len(gotNames))
		if d := firstDiff(wantNames, gotNames); d >= 0 {
			t.Errorf("%s: first difference at index %d: want %q, got %q",
				label, d, at(wantNames, d), at(gotNames, d))
		}
		return
	}
	for _, g := range dz.File {
		w := wantByName[g.Name]
		if w == nil {
			continue
		}
		if g.Method != w.Method {
			t.Errorf("%s: %s compression method %d, want %d", label, g.Name, g.Method, w.Method)
		}
		if g.CRC32 != w.CRC32 {
			t.Errorf("%s: %s CRC %08x, want %08x", label, g.Name, g.CRC32, w.CRC32)
		}
		if g.UncompressedSize64 != w.UncompressedSize64 {
			t.Errorf("%s: %s uncompressed size %d, want %d",
				label, g.Name, g.UncompressedSize64, w.UncompressedSize64)
		}
		if g.CompressedSize64 != w.CompressedSize64 {
			t.Errorf("%s: %s compressed size %d, want %d",
				label, g.Name, g.CompressedSize64, w.CompressedSize64)
		}
		if !g.Modified.Equal(w.Modified) {
			t.Errorf("%s: %s modified %v, want %v", label, g.Name, g.Modified, w.Modified)
		}
		if g.Flags != w.Flags {
			t.Errorf("%s: %s general-purpose flags %#04x, want %#04x", label, g.Name, g.Flags, w.Flags)
		}
		if !bytes.Equal(g.Extra, w.Extra) {
			t.Errorf("%s: %s extra field changed (%d bytes vs %d)",
				label, g.Name, len(g.Extra), len(w.Extra))
		}
		// The raw, still-compressed payload must be identical: comparing only the
		// decompressed bytes would pass even if the entry had been recompressed.
		if gs, ws := rawSum(t, g), rawSum(t, w); gs != ws {
			t.Errorf("%s: %s raw compressed bytes differ", label, g.Name)
		}
		// And the decompressed payload must still read back cleanly, which is what
		// proves Procreate itself can open the member.
		if gs, ws := memberSum(t, g), memberSum(t, w); gs != ws {
			t.Errorf("%s: %s decompressed bytes differ (%s vs %s)", label, g.Name, gs, ws)
		}
	}
}

func rawSum(t *testing.T, f *zip.File) string {
	t.Helper()
	rc, err := f.OpenRaw()
	if err != nil {
		t.Errorf("%s: OpenRaw: %v", f.Name, err)
		return "err"
	}
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		t.Errorf("%s: read raw: %v", f.Name, err)
		return "err"
	}
	return hex.EncodeToString(h.Sum(nil))
}

func memberSum(t *testing.T, f *zip.File) string {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Errorf("%s: Open: %v", f.Name, err)
		return "err"
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		t.Errorf("%s: read: %v", f.Name, err)
		return "err"
	}
	return hex.EncodeToString(h.Sum(nil))
}

func countSegments(t *testing.T, path string) int {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if procreate.IsSegmentName(f.Name) {
			n++
		}
	}
	return n
}

// sameModTime compares timestamps with filesystem granularity in mind: some
// filesystems keep whole seconds only, so sub-second drift is not a failure.
func sameModTime(got, want time.Time) bool {
	d := got.Sub(want)
	if d < 0 {
		d = -d
	}
	return d < time.Second
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func firstDiff(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "<none>"
}

// findProjects lists the .procreate files under root, skipping the macOS
// resource forks (._Name.procreate) that survive copying a folder off an iPad.
func findProjects(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "__MACOSX") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".procreate") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func fileSum(t *testing.T, p string) string {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// corpus returns a directory of .procreate files, unpacking
// PROCREATE_FIXTURE_ZIP into a temp directory when that is how it was supplied.
func corpus(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("PROCREATE_FIXTURE_DIR"); dir != "" {
		return dir
	}
	zipPath := os.Getenv("PROCREATE_FIXTURE_ZIP")
	if zipPath == "" {
		t.Skip("set PROCREATE_FIXTURE_DIR or PROCREATE_FIXTURE_ZIP to run the real-corpus suite")
	}
	dir := t.TempDir()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open %s: %v", zipPath, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if f.FileInfo().IsDir() || strings.HasPrefix(name, ".") ||
			!strings.EqualFold(filepath.Ext(name), ".procreate") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, name)
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			t.Fatal(err)
		}
		_, cerr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if cerr != nil {
			t.Fatal(cerr)
		}
		if err := os.Chtimes(dst, f.Modified, f.Modified); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// stageCorpus copies the corpus into the test's own input directory, preserving
// modification times, so the timestamp assertions have something to check and
// the caller's files are never at risk.
func stageCorpus(t *testing.T, src, dst string) []string {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	var out []string
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") ||
			!strings.EqualFold(filepath.Ext(e.Name()), ".procreate") {
			continue
		}
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		st, err := os.Stat(s)
		if err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(s)
		if err != nil {
			t.Fatal(err)
		}
		o, err := os.Create(d)
		if err != nil {
			in.Close()
			t.Fatal(err)
		}
		_, cerr := io.Copy(o, in)
		in.Close()
		o.Close()
		if cerr != nil {
			t.Fatal(cerr)
		}
		if err := os.Chtimes(d, st.ModTime(), st.ModTime()); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatalf("no .procreate files in %s", src)
	}
	return out
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "procreepy")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "procreepy/cmd/procreepy")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}
