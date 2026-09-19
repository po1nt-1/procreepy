package procreate

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type splitEnt struct {
	name     string
	method   uint16
	modified time.Time
	data     []byte
	isDir    bool
}

// buildSplitFixture writes a zip with mixed methods, directory entries, a
// case-variant video dir, and a lookalike "videobackup/" member.
func buildSplitFixture(t *testing.T, path string) []splitEnt {
	t.Helper()
	ents := []splitEnt{
		{"Info.plist", zip.Deflate, time.Date(2020, 1, 1, 10, 0, 0, 0, time.UTC),
			[]byte("<plist><dict><key>t</key><string>1</string></dict></plist>"), false},
		{"Canvas/", zip.Store, time.Date(2020, 1, 1, 11, 0, 0, 0, time.UTC), nil, true},
		{"Canvas/layer.bin", zip.Store, time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC), pattern(4096), false},
		{"video/", zip.Store, time.Date(2020, 1, 1, 13, 0, 0, 0, time.UTC), nil, true},
		{"video/segments/segment-1.mp4", zip.Store, time.Date(2020, 1, 1, 14, 0, 0, 0, time.UTC), pattern(1000), false},
		{"video/segments/segment-2.mp4", zip.Deflate, time.Date(2020, 1, 1, 15, 0, 0, 0, time.UTC), pattern(2000), false},
		{"VIDEO/extra.dat", zip.Store, time.Date(2020, 1, 1, 16, 0, 0, 0, time.UTC), pattern(64), false},
		{"videobackup/keep.bin", zip.Store, time.Date(2020, 1, 1, 17, 0, 0, 0, time.UTC), pattern(128), false},
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	wz := zip.NewWriter(out)
	for _, e := range ents {
		hd := &zip.FileHeader{Name: e.name, Method: e.method, Modified: e.modified}
		zw, err := wz.CreateHeader(hd) // a dir entry ends here; the next call finalizes it
		if err != nil {
			t.Fatal(err)
		}
		if e.isDir {
			continue
		}
		if _, err := zw.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := wz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	return ents
}

// pattern is deterministic pseudo-random bytes.
func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 37 % 251)
	}
	return b
}

func TestSplitTimelapse(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.procreate")
	ents := buildSplitFixture(t, src)

	dst := filepath.Join(dir, "slim.procreate")
	removed, removedBytes, err := SplitTimelapse(src, dst)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if removed != 4 {
		t.Fatalf("removed = %d, want 4 (video/ dir + 2 segments + VIDEO/extra.dat)", removed)
	}

	// removedBytes must equal the compressed size of the dropped members.
	sf, _ := os.Open(src)
	sfi, _ := sf.Stat()
	zr, err := zip.NewReader(sf, sfi.Size())
	sf.Close()
	if err != nil {
		t.Fatal(err)
	}
	var want int64
	for _, f := range zr.File {
		if isVideoMember(f.Name) {
			want += int64(f.CompressedSize)
		}
	}
	if removedBytes != want {
		t.Errorf("removedBytes = %d, want %d", removedBytes, want)
	}

	// The result keeps every other member, in order, with method, time, and
	// bytes intact.
	df, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()
	dfi, _ := df.Stat()
	dz, err := zip.NewReader(df, dfi.Size())
	if err != nil {
		t.Fatalf("result is not a zip: %v", err)
	}
	wantNames := []string{"Info.plist", "Canvas/", "Canvas/layer.bin", "videobackup/keep.bin"}
	if len(dz.File) != len(wantNames) {
		t.Fatalf("members: got %d, want %d", len(dz.File), len(wantNames))
	}
	orig := make(map[string]splitEnt)
	for _, e := range ents {
		orig[e.name] = e
	}
	for i, f := range dz.File {
		if f.Name != wantNames[i] {
			t.Fatalf("member[%d] = %s, want %s", i, f.Name, wantNames[i])
		}
		e := orig[f.Name]
		if f.Method != e.method {
			t.Errorf("%s: method = %d, want %d", f.Name, f.Method, e.method)
		}
		if !f.Modified.Equal(e.modified) {
			t.Errorf("%s: modified = %v, want %v", f.Name, f.Modified, e.modified)
		}
		if e.isDir {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var got []byte
		if got, err = io.ReadAll(rc); err == nil {
			err = rc.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, e.data) {
			t.Errorf("%s: data differs (%d vs %d bytes)", f.Name, len(got), len(e.data))
		}
	}
	for _, f := range dz.File {
		if isVideoMember(f.Name) {
			t.Errorf("video member survived: %s", f.Name)
		}
	}
	if leftovers := partials(dir); len(leftovers) > 0 {
		t.Errorf("leftover partial files: %v", leftovers)
	}
}

func partials(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if strings.Contains(e.Name(), ".partial") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestSplitTimelapseErrors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.procreate")
	buildSplitFixture(t, src)

	if _, _, err := SplitTimelapse(filepath.Join(dir, "missing.procreate"), filepath.Join(dir, "x")); err == nil {
		t.Error("missing src: want error")
	}
	if _, _, err := SplitTimelapse(src, filepath.Join(dir, "no-such-dir", "x")); err == nil {
		t.Error("missing dst parent: want error")
	}
	junk := filepath.Join(dir, "junk.procreate")
	if err := os.WriteFile(junk, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SplitTimelapse(junk, filepath.Join(dir, "x")); err == nil {
		t.Error("invalid zip: want error")
	}
	if leftovers := partials(dir); len(leftovers) > 0 {
		t.Errorf("leftover partial files after errors: %v", leftovers)
	}
}
