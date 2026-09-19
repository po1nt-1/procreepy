package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGenFixture writes ready-made samples for manual CLI checks.
// Run with: GEN=/tmp/fx go test ./internal/testkit -run TestGenFixture
func TestGenFixture(t *testing.T) {
	dir := os.Getenv("GEN")
	if dir == "" {
		t.Skip("set GEN=<dir> to generate fixtures")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	seg1 := Segment(320, 240, []uint32{100, 200}, []uint32{50, 60})
	seg2 := Segment(320, 240, []uint32{300}, []uint32{70})
	good := map[string][]byte{
		"video/segments/segment-1.mp4": seg1,
		"video/segments/segment-2.mp4": seg2,
	}
	if err := os.WriteFile(filepath.Join(dir, "good.procreate"),
		ArchiveBytes(good, false), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := map[string][]byte{"canvas.bin": []byte("no video here")}
	if err := os.WriteFile(filepath.Join(dir, "empty.procreate"),
		ArchiveBytes(empty, false), 0o644); err != nil {
		t.Fatal(err)
	}
	seg := Segment(320, 240, []uint32{100}, []uint32{50})
	bad := map[string][]byte{"video/segments/segment-1.mp4": seg[:len(seg)/2]}
	if err := os.WriteFile(filepath.Join(dir, "bad.procreate"),
		ArchiveBytes(bad, false), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "junk.procreate"), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixtures in %s", dir)
}
