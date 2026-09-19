package procreate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"procreepy/internal/testkit"
)

const (
	segA = "video/segments/segment-1.mp4"
	segB = "video/segments/segment-2.mp4"
)

func testEntries() map[string][]byte {
	return map[string][]byte{
		"canvas/proj/project.dat": []byte("junk"),
		segA:                      testkit.Segment(320, 240, []uint32{10, 12, 14}, []uint32{100, 120}),
		segB:                      testkit.Segment(320, 240, []uint32{10, 12}, []uint32{100}),
	}
}

func TestOpenRejects(t *testing.T) {
	if _, err := Open("/nonexistent/dir/x.procreate", "x"); !errors.Is(err, ErrInput) {
		t.Fatalf("missing: err = %v, want ErrInput", err)
	}
	empty := t.TempDir() + "/empty.procreate"
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(empty, "empty"); !errors.Is(err, ErrInput) {
		t.Fatalf("empty: err = %v, want ErrInput", err)
	}
	garbage := t.TempDir() + "/garbage.procreate"
	if err := os.WriteFile(garbage, []byte("definitely not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(garbage, "garbage"); !errors.Is(err, ErrInput) {
		t.Fatalf("garbage: err = %v, want ErrInput", err)
	}
}

func TestArchiveFlow(t *testing.T) {
	for _, compress := range []bool{false, true} {
		compress := compress
		t.Run(methodName(compress), func(t *testing.T) {
			entries := testEntries()
			path := testkit.WriteArchive(t, entries, compress)
			arch, err := Open(path, path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer arch.Close()

			if arch.Label() != path {
				t.Fatalf("label = %q", arch.Label())
			}
			if n := len(arch.Members()); n != 3 {
				t.Fatalf("members = %d, want 3", n)
			}
			var warns []string
			segs, err := arch.Segments(Options{Warn: func(f string, a ...any) {
				warns = append(warns, fmt.Sprintf(f, a...))
			}})
			if err != nil {
				t.Fatalf("Segments: %v", err)
			}
			if len(segs) != 2 || segs[0].Number != 1 || segs[1].Number != 2 {
				t.Fatalf("segments = %+v, want [1 2]", segs)
			}
			if len(warns) != 0 {
				t.Fatalf("unexpected warnings: %q", warns)
			}

			m, err := arch.ParseSegment(segA)
			if err != nil {
				t.Fatalf("ParseSegment: %v", err)
			}
			if m.Mvhd.Timescale != 30 || m.Mvhd.Duration != 90 {
				t.Fatalf("mvhd = %+v, want ts=30 dur=90", m.Mvhd)
			}
			if len(m.Tracks) != 2 || m.Tracks[0].SampleCount != 3 || m.Tracks[1].SampleCount != 2 {
				t.Fatalf("tracks = %+v", m.Tracks)
			}
			if len(m.Mdat) != 1 {
				t.Fatalf("mdat regions = %d, want 1", len(m.Mdat))
			}

			// Stream the mdat payload and compare with the member bytes.
			reg := m.Mdat[0]
			var buf bytes.Buffer
			n, err := arch.StreamMdat(segA, reg.Start, reg.End-reg.Start, &buf)
			if err != nil {
				t.Fatalf("StreamMdat: %v", err)
			}
			if want := reg.End - reg.Start; n != want {
				t.Fatalf("streamed %d bytes, want %d", n, want)
			}
			if !bytes.Equal(buf.Bytes(), entries[segA][reg.Start:reg.End]) {
				t.Fatal("streamed mdat payload differs from the member bytes")
			}
		})
	}
}

func methodName(compress bool) string {
	if compress {
		return "deflated"
	}
	return "stored"
}

// localDataStart returns the offset of the first member's data inside a raw
// ZIP byte stream (single-entry archives only).
func localDataStart(raw []byte) int {
	if !bytes.Equal(raw[0:4], []byte{0x50, 0x4b, 0x03, 0x04}) {
		panic("bad local header signature")
	}
	nameLen := int(raw[26]) | int(raw[27])<<8
	extraLen := int(raw[28]) | int(raw[29])<<8
	return 30 + nameLen + extraLen
}

func TestCorruptCRCStored(t *testing.T) {
	entries := map[string][]byte{segA: testkit.Segment(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})}
	raw := testkit.ArchiveBytes(entries, false)
	dat := localDataStart(raw)
	raw[dat+3] ^= 0xff // corrupt one stored payload byte

	path := t.TempDir() + "/corrupt.procreate"
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()

	if _, err := arch.ParseSegment(segA); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("ParseSegment: err = %v, want ErrBadSegment", err)
	}
	var buf bytes.Buffer
	if _, err := arch.StreamMdat(segA, 0, 4, &buf); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("StreamMdat: err = %v, want ErrBadSegment", err)
	}
}

func TestStreamMdatBounds(t *testing.T) {
	path := testkit.WriteArchive(t, testEntries(), false)
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	var buf bytes.Buffer
	if _, err := arch.StreamMdat(segA, 0, 1<<20, &buf); !errors.Is(err, ErrBadSegment) {
		t.Fatalf("oversized read: err = %v, want ErrBadSegment", err)
	}
}

func TestSegmentNamesCarryThrough(t *testing.T) {
	// A case-variant member name must round-trip through the archive.
	seg := testkit.Segment(320, 240, []uint32{10}, []uint32{100})
	entries := map[string][]byte{"VIDEO/Segments/SEGMENT-1.MP4": seg}
	path := testkit.WriteArchive(t, entries, false)
	arch, err := Open(path, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer arch.Close()
	segs, err := arch.Segments(Options{})
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) != 1 || segs[0].Name != "VIDEO/Segments/SEGMENT-1.MP4" {
		t.Fatalf("segments = %+v", segs)
	}
	if _, err := arch.ParseSegment(segs[0].Name); err != nil {
		t.Fatalf("ParseSegment: %v", err)
	}
}
