package e2e

import (
	"archive/zip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// slimMembers opens the slimmed archive and returns member names (in order)
// plus their uncompressed contents.
func slimMembers(t *testing.T, p string) ([]string, map[string][]byte) {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatalf("open slimmed archive: %v", err)
	}
	defer zr.Close()
	names := make([]string, 0, len(zr.File))
	content := make(map[string][]byte, len(zr.File))
	methods := make(map[string]uint16)
	for _, f := range zr.File {
		names = append(names, f.Name)
		methods[f.Name] = f.Method
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open member %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read member %s: %v", f.Name, err)
		}
		content[f.Name] = b
	}
	for n, m := range methods {
		if m != zip.Store {
			t.Errorf("member %s: compression method = %d, want %d (stored)", n, m, zip.Store)
		}
	}
	return names, content
}

func TestSplitFile(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	removed := int64(len(segN(1)) + len(segN(2)) + len(segN(3)))
	slim := filepath.Join(dir, "out.procreepy.procreate")
	wantErr := slogLine(slog.LevelInfo, "segments found", "input", "in.procreate", "count", 3) +
		slogLine(slog.LevelInfo, "joining segments (stream copy)") +
		slogLine(slog.LevelInfo, "slimmed archive written", "path", slim, "removed_files", 4,
			"video_size", humanBytes(removed)) +
		slogLine(slog.LevelInfo, "conversion completed", "output", "out.mp4", "duration_s", 6.0)
	check(t, run(t, dir, nil, "--split", "in.procreate", "out.mp4"), 0, "", wantErr)
	eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))

	names, content := slimMembers(t, slim)
	if len(names) != 1 || names[0] != "Document/document.data" {
		t.Fatalf("slimmed members = %v, want [Document/document.data]", names)
	}
	if string(content["Document/document.data"]) != "fake-document-data" {
		t.Errorf("document.data content = %q", content["Document/document.data"])
	}
}

// --split with a non-video member preserves order and bytes exactly.
func TestSplitPreservesOthers(t *testing.T) {
	dir := t.TempDir()
	art := []byte("png-bytes-here")
	entries := map[string][]byte{
		"Assets/art.png":               art,
		"Document/document.data":       []byte("fake-document-data"),
		"video/segments/":              {},
		"video/segments/segment-1.mp4": segN(1),
		"video/segments/segment-2.mp4": segN(2),
		"video/segments/segment-3.mp4": segN(3),
		"video/thumbnail.jpg":          []byte("jpeg-bytes"),
	}
	writeArchive(t, dir, "in.procreate", entries)
	run(t, dir, nil, "-q", "--split", "in.procreate", "out.mp4")
	names, content := slimMembers(t, filepath.Join(dir, "out.procreepy.procreate"))
	want := []string{"Assets/art.png", "Document/document.data"}
	if len(names) != len(want) {
		t.Fatalf("slimmed members = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("member %d = %s, want %s", i, names[i], want[i])
		}
	}
	if string(content["Assets/art.png"]) != string(art) {
		t.Error("art.png content changed")
	}
}

func TestSplitToDirectory(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	removed := int64(len(segN(1)) + len(segN(2)) + len(segN(3)))
	slim := filepath.Join(dir, "out", "in.procreepy.procreate")
	wantErr := slogLine(slog.LevelInfo, "segments found", "input", "in.procreate", "count", 3) +
		slogLine(slog.LevelInfo, "joining segments (stream copy)") +
		slogLine(slog.LevelInfo, "slimmed archive written", "path", slim, "removed_files", 4,
			"video_size", humanBytes(removed)) +
		slogLine(slog.LevelInfo, "conversion completed", "output", "out/in.mp4", "duration_s", 6.0)
	check(t, run(t, dir, nil, "--split", "in.procreate", "out"), 0, "", wantErr)
	eqBytes(t, "out/in.mp4", readAll(t, filepath.Join(dir, "out", "in.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	if _, err := os.Stat(slim); err != nil {
		t.Error("slimmed archive missing:", err)
	}
}

// -q silences the slimmed line too (it is an info message).
func TestSplitQuiet(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	check(t, run(t, dir, nil, "-q", "--split", "in.procreate", "out.mp4"), 0, "", "")
	if _, err := os.Stat(filepath.Join(dir, "out.mp4")); err != nil {
		t.Error("out.mp4 missing:", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.procreepy.procreate")); err != nil {
		t.Error("slimmed archive missing:", err)
	}
}

func TestSplitNeedsFileOrDir(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	check(t, run(t, dir, nil, "--split", "in.procreate"), 2, "",
		actionErr("--split writes a slimmed .procreepy.procreate, so it needs a "+
			"file or directory OUTPUT, not stdout"))
	check(t, run(t, dir, nil, "--split", "in.procreate", "-"), 2, "",
		actionErr("--split writes a slimmed .procreepy.procreate, so it needs a "+
			"file or directory OUTPUT, not stdout"))
}

// Without a video there is nothing to split: no MP4, no slimmed archive.
func TestSplitNoVideo(t *testing.T) {
	dir := t.TempDir()
	entries := map[string][]byte{
		"Document/document.data": []byte("fake-document-data"),
		"video/segments/":        {},
	}
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "--split", "in.procreate", "out.mp4"), 4, "",
		actionErr("no video/segments in this archive (time-lapse recording was probably turned off for this artwork)"))
	if _, err := os.Stat(filepath.Join(dir, "out.mp4")); !os.IsNotExist(err) {
		t.Error("out.mp4 should not exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "out.procreepy.procreate")); !os.IsNotExist(err) {
		t.Error("slimmed archive should not exist")
	}
}
