package procreate

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"procreepy/internal/testkit"
)

func TestMemberLookupsMiss(t *testing.T) {
	path := testkit.WriteArchive(t, testEntries(), false)
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	if _, err := arch.ReadMember("nope"); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("ReadMember: err = %v, want ErrBadSegment", err)
	}
	if _, err := arch.ParseSegment("nope"); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("ParseSegment: err = %v, want ErrBadSegment", err)
	}
	var buf bytes.Buffer
	if _, err := arch.StreamMdat("nope", 0, 1, &buf); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("StreamMdat: err = %v, want ErrBadSegment", err)
	}
}

func TestOsErrPlainError(t *testing.T) {
	if got := osErr(errors.New("plain failure")); got != "plain failure" {
		t.Fatalf("osErr = %q", got)
	}
}

func TestReadMemberCorrupt(t *testing.T) {
	entries := map[string][]byte{segA: testkit.Segment(320, 240, []uint32{10, 12}, []uint32{100})}
	raw := testkit.ArchiveBytes(entries, false)
	raw[localDataStart(raw)+3] ^= 0xff
	path := t.TempDir() + "/corrupt.procreate"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	if _, err := arch.ReadMember(segA); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("ReadMember: err = %v, want ErrBadSegment", err)
	}
}

// TestParseSegmentValidateFail stores a segment whose mdat box has been
// chopped off: parsing succeeds, validation must reject it.
func TestParseSegmentValidateFail(t *testing.T) {
	raw := testkit.Segment(320, 240, []uint32{10, 12}, []uint32{100})
	off := 0
	for off+8 <= len(raw) {
		size := int(binary.BigEndian.Uint32(raw[off:]))
		if string(raw[off+4:off+8]) == "mdat" {
			break
		}
		off += size
	}
	if off == 0 || off+8 > len(raw) {
		t.Fatal("mdat box not found in fixture")
	}
	entries := map[string][]byte{segA: raw[:off]}
	path := testkit.WriteArchive(t, entries, false)
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	if _, err := arch.ParseSegment(segA); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("ParseSegment: err = %v, want ErrBadSegment", err)
	}
}

type errWriter struct{ err error }

func (w errWriter) Write(p []byte) (int, error) { return 0, w.err }

type shortWriter struct{}

// shortWriter honours only the first byte of each buffer: a legal but lazy
// io.Writer, which must surface as a short-stream error, not a panic.
func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 1, nil
}

func TestStreamMdatDestinationErrors(t *testing.T) {
	entries := testEntries()
	path := testkit.WriteArchive(t, entries, false)
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	if _, err := arch.StreamMdat(segA, 0, 16, errWriter{errors.New("disk full")}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("failing writer: err = %v, want raw write error", err)
	}
	if _, err := arch.StreamMdat(segA, 0, int64(len(entries[segA])), shortWriter{}); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("short writer: err = %v, want ErrBadSegment", err)
	}
}

func TestStreamMdatCancelledContext(t *testing.T) {
	path := testkit.WriteArchive(t, testEntries(), false)
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	// Cancelled before the seek: ctxSkip must abort.
	if _, err := arch.StreamMdatCtx(ctx, segA, 10, 16, &buf); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("cancelled skip: err = %v, want ErrBadSegment", err)
	}
	// Cancelled before the copy: ctxCopyN must abort.
	if _, err := arch.StreamMdatCtx(ctx, segA, 0, 16, &buf); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("cancelled copy: err = %v, want ErrBadSegment", err)
	}
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }

func TestCtxSkipBranches(t *testing.T) {
	bg := context.Background()
	if err := ctxSkip(bg, readFunc(func([]byte) (int, error) { return 0, io.EOF }), 5); !errors.Is(err, io.EOF) {
		t.Fatalf("early EOF: err = %v, want EOF", err)
	}
	if err := ctxSkip(bg, readFunc(func(p []byte) (int, error) { return copy(p, []byte("ab")), io.EOF }), 2); err != nil {
		t.Fatalf("exact EOF: err = %v, want nil", err)
	}
	if err := ctxSkip(bg, readFunc(func([]byte) (int, error) { return 0, errors.New("boom") }), 1); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("read error: err = %v, want boom", err)
	}
	if err := ctxSkip(bg, readFunc(func([]byte) (int, error) { return 0, nil }), 1); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("zero read: err = %v, want ErrUnexpectedEOF", err)
	}
}

func TestCtxCopyNBranches(t *testing.T) {
	bg := context.Background()
	var buf bytes.Buffer
	if _, _, err := ctxCopyN(bg, &buf, readFunc(func([]byte) (int, error) { return 0, errors.New("boom") }), 4); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("read error: err = %v, want boom", err)
	}
	n, werr, rerr := ctxCopyN(bg, &buf, readFunc(func([]byte) (int, error) { return 0, nil }), 4)
	if werr != nil || n != 0 || !errors.Is(rerr, io.ErrUnexpectedEOF) {
		t.Fatalf("zero read: n=%d werr=%v rerr=%v, want 0/nil/ErrUnexpectedEOF", n, werr, rerr)
	}
}

func TestSplitRenameToDir(t *testing.T) {
	src := testkit.WriteArchive(t, testEntries(), false)
	dst := t.TempDir() + "/slim-dir"
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SplitTimelapse(src, dst); err == nil || !strings.Contains(err.Error(), "cannot write") {
		t.Fatalf("rename onto directory: err = %v, want cannot write", err)
	}
}

func TestSplitCorruptNonVideoMember(t *testing.T) {
	entries := map[string][]byte{"canvas/proj/project.dat": []byte("junk")}
	raw := testkit.ArchiveBytes(entries, false)
	raw[localDataStart(raw)+1] ^= 0xff
	src := t.TempDir() + "/corrupt.procreate"
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir() + "/out.procreate"
	if _, _, err := SplitTimelapse(src, dst); err == nil {
		t.Fatal("expected rewrite error for corrupt member")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("partial output left behind: %v", err)
	}
}
