package video

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyWritten(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyWritten(p, 5); err != nil {
		t.Fatalf("matching size rejected: %v", err)
	}
	if err := verifyWritten(p, 4); err == nil {
		t.Fatal("size mismatch not detected")
	}
	if err := verifyWritten(filepath.Join(dir, "missing"), 0); err == nil {
		t.Fatal("missing file not rejected")
	}
}

// TestPublishVerifiesSizeBeforeRename: the file on disk diverging from what
// the writer reported must abort the publish, leaving neither the scratch
// file nor a half-written final one.
func TestPublishVerifiesSizeBeforeRename(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "out.bin")
	s, err := stage(final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.f.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	extra, err := os.OpenFile(s.tmp, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extra.WriteString("junk"); err != nil {
		t.Fatal(err)
	}
	if err := extra.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(); err == nil {
		t.Fatal("publish succeeded despite the size mismatch")
	}
	if _, err := os.Stat(s.tmp); !os.IsNotExist(err) {
		t.Fatalf("scratch file left behind: %v", err)
	}
	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Fatalf("final file appeared despite the failed publish: %v", err)
	}
}

// TestPublishAcceptsHeaderPatchWrite: a writer that seeks back over written
// data to patch a header (like the PSD writer) must publish the file as-is,
// not a size padded by the patch write.
func TestPublishAcceptsHeaderPatchWrite(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "out.bin")
	s, err := stage(final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.f.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := s.f.Write([]byte("HDR!")); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "HDR!456789" {
		t.Fatalf("published %q, want %q", got, "HDR!456789")
	}
}

func TestPublishHappyPath(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "out.bin")
	s, err := stage(final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.f.Write([]byte("hello world")); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("published %q, want %q", got, "hello world")
	}
	if _, err := os.Stat(s.tmp); !os.IsNotExist(err) {
		t.Fatalf("scratch file left behind: %v", err)
	}
	s.discard() // a no-op after publish
}
