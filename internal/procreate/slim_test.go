package procreate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"procreepy/internal/testkit"
)

// slim runs WriteSlimmed over an archive built from entries and returns the
// result plus the slimmed bytes.
func slim(t *testing.T, raw []byte) (SlimResult, []byte) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.procreate")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var buf bytes.Buffer
	res, err := a.WriteSlimmed(context.Background(), &buf)
	if err != nil {
		t.Fatalf("WriteSlimmed: %v", err)
	}
	return res, buf.Bytes()
}

func openZip(t *testing.T, raw []byte) *zip.Reader {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("result is not a valid ZIP: %v", err)
	}
	return zr
}

func names(zr *zip.Reader) []string {
	out := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

// TestSlimProcreateStyleArchive is the regression guard for the bug that made
// --split fail on every real project: zip.Writer ORs the data-descriptor flag
// into the FileHeader it is handed, and Procreate writes entries without a
// descriptor, so a rewriter that passes &f.FileHeader poisons its own reader and
// every member fails with zip.ErrChecksum. An archive built by Go's writer
// already has that flag set and so cannot catch it.
func TestSlimProcreateStyleArchive(t *testing.T) {
	src := testkit.Project(3)
	raw := src.Bytes()

	// Confirm the fixture really is descriptor-free, or the test proves nothing.
	for _, f := range openZip(t, raw).File {
		if f.Flags&0x8 != 0 {
			t.Fatalf("%s: fixture has the data-descriptor flag set; it cannot exercise the bug", f.Name)
		}
	}

	res, out := slim(t, raw)
	if res.Removed != 3 {
		t.Errorf("removed = %d, want 3", res.Removed)
	}
	zr := openZip(t, out)
	for _, f := range zr.File {
		if _, err := readAll(f); err != nil {
			t.Errorf("%s: cannot read back: %v", f.Name, err)
		}
	}
}

// TestSlimPreservesRawEntries checks the whole preservation contract: order,
// names, container fields, and the still-compressed payload.
func TestSlimPreservesRawEntries(t *testing.T) {
	src := testkit.Project(2)
	raw := src.Bytes()
	_, out := slim(t, raw)

	in := openZip(t, raw)
	got := openZip(t, out)

	var want []*zip.File
	for _, f := range in.File {
		if !IsSegmentName(f.Name) {
			want = append(want, f)
		}
	}
	if len(got.File) != len(want) {
		t.Fatalf("kept %d members, want %d\ngot:  %v\nwant: %v",
			len(got.File), len(want), names(got), fileNames(want))
	}
	for i, g := range got.File {
		w := want[i]
		if g.Name != w.Name {
			t.Errorf("member %d: name %q, want %q (order must be preserved)", i, g.Name, w.Name)
			continue
		}
		if g.Method != w.Method {
			t.Errorf("%s: method %d, want %d", g.Name, g.Method, w.Method)
		}
		if g.CRC32 != w.CRC32 {
			t.Errorf("%s: CRC %08x, want %08x", g.Name, g.CRC32, w.CRC32)
		}
		if g.CompressedSize64 != w.CompressedSize64 {
			t.Errorf("%s: compressed size %d, want %d", g.Name, g.CompressedSize64, w.CompressedSize64)
		}
		if g.UncompressedSize64 != w.UncompressedSize64 {
			t.Errorf("%s: uncompressed size %d, want %d", g.Name, g.UncompressedSize64, w.UncompressedSize64)
		}
		if !g.Modified.Equal(w.Modified) {
			t.Errorf("%s: modified %v, want %v", g.Name, g.Modified, w.Modified)
		}
		if g.Flags != w.Flags {
			t.Errorf("%s: flags %#04x, want %#04x", g.Name, g.Flags, w.Flags)
		}
		if !bytes.Equal(g.Extra, w.Extra) {
			t.Errorf("%s: extra field changed", g.Name)
		}
		// Raw comparison is the point: equal decompressed bytes would also hold
		// for an entry that had been decompressed and recompressed.
		if gs, ws := rawSum(t, g), rawSum(t, w); gs != ws {
			t.Errorf("%s: raw compressed bytes differ (%s vs %s)", g.Name, gs, ws)
		}
	}
}

// TestSlimKeepsNonSegmentVideoMembers pins the narrow removal rule: only
// video/segments/segment-N.mp4 goes. Anything else under video/ is metadata this
// tool does not understand and must not throw away.
func TestSlimKeepsNonSegmentVideoMembers(t *testing.T) {
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "Document.archive", Data: []byte("doc"), Method: zip.Deflate},
		{Name: "video/segments/segment-1.mp4", Data: testkit.Segment(320, 240, []uint32{100}, []uint32{50}), Method: zip.Deflate},
		{Name: "video/other-metadata.dat", Data: []byte("keep me"), Method: zip.Deflate},
		{Name: "video/active-0.mp4", Data: nil, Method: zip.Deflate},
		{Name: "video/segments/notes.txt", Data: []byte("keep me too"), Method: zip.Deflate},
		{Name: "video/segments/segment-2.mp4", Data: testkit.Segment(320, 240, []uint32{100}, []uint32{50}), Method: zip.Deflate},
	}}
	res, out := slim(t, a.Bytes())
	if res.Removed != 2 {
		t.Errorf("removed = %d, want 2", res.Removed)
	}
	want := []string{
		"Document.archive",
		"video/other-metadata.dat",
		"video/active-0.mp4",
		"video/segments/notes.txt",
	}
	if got := names(openZip(t, out)); !equal(got, want) {
		t.Errorf("kept members = %v, want %v", got, want)
	}
}

// TestSlimRemovesSegmentsCaseInsensitively mirrors the segment scanner, which
// matches names without regard to case.
func TestSlimRemovesSegmentsCaseInsensitively(t *testing.T) {
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "Document.archive", Data: []byte("doc")},
		{Name: "Video/Segments/Segment-1.MP4", Data: []byte("v")},
	}}
	res, out := slim(t, a.Bytes())
	if res.Removed != 1 {
		t.Errorf("removed = %d, want 1", res.Removed)
	}
	if got := names(openZip(t, out)); !equal(got, []string{"Document.archive"}) {
		t.Errorf("kept %v", got)
	}
}

// TestSlimKeepsDirectoryEntries leaves empty directory entries alone: they carry
// no payload, and dropping the ones under video/ would be the same broad guess
// that made the old rule unsafe.
func TestSlimKeepsDirectoryEntries(t *testing.T) {
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "video/segments/", Dir: true},
		{Name: "video/segments/segment-1.mp4", Data: []byte("v")},
	}}
	res, out := slim(t, a.Bytes())
	if res.Removed != 1 || res.Kept != 1 {
		t.Errorf("removed/kept = %d/%d, want 1/1", res.Removed, res.Kept)
	}
	if got := names(openZip(t, out)); !equal(got, []string{"video/segments/"}) {
		t.Errorf("kept %v, want the directory entry", got)
	}
}

// TestSlimNonASCIIAndMixedCompression covers Unicode member names and an archive
// that mixes stored and deflated entries, both of which occur in the wild.
func TestSlimNonASCIIAndMixedCompression(t *testing.T) {
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "документ.archive", Data: bytes.Repeat([]byte("данные"), 100), Method: zip.Deflate},
		{Name: "层/0~0.lz4", Data: []byte("stored"), Method: zip.Store},
		{Name: "video/segments/segment-1.mp4", Data: []byte("v"), Method: zip.Store},
	}}
	_, out := slim(t, a.Bytes())
	zr := openZip(t, out)
	if got := names(zr); !equal(got, []string{"документ.archive", "层/0~0.lz4"}) {
		t.Fatalf("kept %v", got)
	}
	if zr.File[0].Method != zip.Deflate || zr.File[1].Method != zip.Store {
		t.Errorf("compression methods not preserved: %d, %d", zr.File[0].Method, zr.File[1].Method)
	}
}

// TestSlimCarriesCorruptMemberThrough documents a deliberate consequence of raw
// copying: a member whose stored CRC does not match its data is reproduced
// exactly rather than failing the run. Verifying it would mean decompressing,
// which is what broke real projects; a damaged source stays equally damaged in
// the copy and is never made worse.
func TestSlimCarriesCorruptMemberThrough(t *testing.T) {
	a := testkit.Archive{Entries: []testkit.Entry{
		{Name: "canvas/project.dat", Data: []byte("junk-payload")},
	}}
	raw := a.Bytes()
	raw[localDataStart(raw)+1] ^= 0xff

	res, out := slim(t, raw)
	if res.Kept != 1 {
		t.Fatalf("kept = %d, want 1", res.Kept)
	}
	zr := openZip(t, out)
	if _, err := readAll(zr.File[0]); err == nil {
		t.Error("expected the corruption to survive; the copy silently repaired it")
	}
	in := openZip(t, raw)
	if rawSum(t, zr.File[0]) != rawSum(t, in.File[0]) {
		t.Error("corrupt member was not reproduced byte-for-byte")
	}
}

// TestSlimEmptyArchive: an archive with no members at all still yields a valid,
// empty ZIP rather than an error.
func TestSlimEmptyArchive(t *testing.T) {
	a := testkit.Archive{Entries: []testkit.Entry{
		{Name: "video/segments/segment-1.mp4", Data: []byte("v")},
	}}
	res, out := slim(t, a.Bytes())
	if res.Kept != 0 || res.Removed != 1 {
		t.Fatalf("kept/removed = %d/%d, want 0/1", res.Kept, res.Removed)
	}
	if got := len(openZip(t, out).File); got != 0 {
		t.Errorf("result has %d members, want 0", got)
	}
}

// TestSlimCancelledBeforeStart returns the context error without writing.
func TestSlimCancelledBeforeStart(t *testing.T) {
	p := testkit.Project(2).Write(t, t.TempDir(), "in.procreate")
	a, err := Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	if _, err := a.WriteSlimmed(ctx, &buf); !isCanceled(err) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestSlimCancelledDuringCopy cancels while a large member is streaming, which
// is the case a single pre-flight context check would miss.
func TestSlimCancelledDuringCopy(t *testing.T) {
	big := bytes.Repeat([]byte("procreate-layer-payload"), 400_000) // ~9 MiB
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "layer/0~0.lz4", Data: big, Method: zip.Store},
		{Name: "layer/0~1.lz4", Data: big, Method: zip.Store},
	}}
	p := a.Write(t, t.TempDir(), "big.procreate")
	arch, err := Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer arch.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel as soon as the copy is under way, so the cancellation has to be
	// noticed inside a member rather than between two of them.
	w := writerFunc(func(b []byte) (int, error) {
		cancel()
		return len(b), nil
	})
	if _, err := arch.WriteSlimmed(ctx, w); !isCanceled(err) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

func isCanceled(err error) bool { return err == context.Canceled }

// TestSlimManyMembers stress-tests the rewrite: every non-segment member must
// survive, in order, with no timelapse remnants.
func TestSlimManyMembers(t *testing.T) {
	const others, videos = 4000, 500
	a := testkit.Archive{ProcreateStyle: true}
	a.Entries = append(a.Entries, testkit.Entry{Name: "appinfo", Data: []byte(`{"version":1}`)})
	for i := 0; i < others; i++ {
		a.Entries = append(a.Entries, testkit.Entry{
			Name: fmt.Sprintf("layer-%05d.data", i), Data: []byte("layer-bytes"),
		})
	}
	for i := 1; i <= videos; i++ {
		a.Entries = append(a.Entries, testkit.Entry{
			Name: fmt.Sprintf("video/segments/segment-%04d.mp4", i), Data: make([]byte, 1024),
		})
	}
	res, out := slim(t, a.Bytes())
	if res.Removed != videos {
		t.Fatalf("removed = %d, want %d", res.Removed, videos)
	}
	if res.Kept != others+1 {
		t.Fatalf("kept = %d, want %d", res.Kept, others+1)
	}
	zr := openZip(t, out)
	if zr.File[0].Name != "appinfo" {
		t.Fatalf("member order broken: first is %s", zr.File[0].Name)
	}
	for _, f := range zr.File {
		if IsSegmentName(f.Name) {
			t.Fatalf("segment survived: %s", f.Name)
		}
	}
}

func fileNames(fs []*zip.File) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

func equal(a, b []string) bool {
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

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func rawSum(t *testing.T, f *zip.File) string {
	t.Helper()
	rc, err := f.OpenRaw()
	if err != nil {
		t.Fatalf("%s: OpenRaw: %v", f.Name, err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		t.Fatalf("%s: read raw: %v", f.Name, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
