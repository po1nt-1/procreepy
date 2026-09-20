package mp4

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// --- regression tests for the hardening fixes -------------------------------

// TestParseStszCountBounded: a uniform stsz may declare an arbitrary 32-bit
// sample count while carrying no per-sample table; before the cap such a
// 12-byte box forced a 16 GiB allocation.
func TestParseStszCountBounded(t *testing.T) {
	pay := make([]byte, 12)                 // version/flags + sample_size + sample_count
	binary.BigEndian.PutUint32(pay[4:8], 1) // uniform size
	binary.BigEndian.PutUint32(pay[8:12], ^uint32(0))
	if _, _, err := parseStsz(pay); err == nil {
		t.Fatal("hostile stsz count accepted")
	}
	binary.BigEndian.PutUint32(pay[8:12], 10)
	if _, sizes, err := parseStsz(pay); err != nil || len(sizes) != 10 {
		t.Fatalf("legit count rejected: sizes=%v err=%v", sizes, err)
	}
}

// TestEsdsDepthCap: ES_Descriptors may nest; past maxEsdsDepth the walk must
// give up cleanly instead of recursing (a hostile moov could otherwise nest
// the tree deep enough to overflow the goroutine stack).
func TestEsdsDepthCap(t *testing.T) {
	asc := []byte{0x14, 0x20} // aot=2 (AAC), 44100 Hz, 2 channels
	leaf := func() []byte {
		b := append([]byte{0x04, 15}, make([]byte, 13)...) // DecoderConfigDescriptor
		return append(b, asc...)
	}
	shallow := wrapES(wrapES(leaf())) // legal 2-level nesting
	if aot, freq, chans, ok := parseESDS(append([]byte{0, 0, 0, 0}, shallow...)); !ok || aot != 2 || freq != 44100 || chans != 2 {
		t.Fatalf("shallow esds: aot=%d freq=%d ch=%d ok=%v", aot, freq, chans, ok)
	}
	deep := leaf()
	for i := 0; i < maxEsdsDepth+5; i++ { // buried beyond the cap
		deep = wrapES(deep)
	}
	if _, _, _, ok := parseESDS(append([]byte{0, 0, 0, 0}, deep...)); ok {
		t.Fatal("decoder config found beyond the nesting cap")
	}
}

func wrapES(inner []byte) []byte {
	l := len(inner)
	return append([]byte{0x03, byte(0x80 | l>>7), byte(l & 0x7f)}, inner...)
}

// TestScanBoxesSkipPayload: the outline scan must record mdat geometry while
// leaving its payload unread, and must load every other box in full.
func TestScanBoxesSkipPayload(t *testing.T) {
	file := buildSegFile(320, 240, []uint32{4, 8}, []uint32{16})
	n := int64(len(file))
	rd := NewReader(bytes.NewReader(file), n)
	boxs, err := scanBoxes(rd, 0, n, func(typ string) bool { return typ == "mdat" })
	if err != nil {
		t.Fatal(err)
	}
	sawMdat := false
	for _, b := range boxs {
		if b.Type == "mdat" {
			sawMdat = true
			if b.Pay != nil {
				t.Fatal("mdat payload was materialized")
			}
			if b.PayOff != b.Off+8 || b.Off+b.Size != n {
				t.Fatalf("mdat geometry off: box at %d size %d, file is %d", b.Off, b.Size, n)
			}
		} else if b.Pay == nil {
			t.Fatalf("box %q payload not loaded", b.Type)
		}
	}
	if !sawMdat {
		t.Fatal("no mdat box scanned")
	}
	if boxs, err := ScanBoxes(rd, 0, n); err != nil {
		t.Fatal(err)
	} else {
		for _, b := range boxs {
			if b.Pay == nil {
				t.Fatalf("public ScanBoxes left %q without payload", b.Type)
			}
		}
	}
}

// TestParseKeepsMdatRegion: Parse must still expose the mdat payload range for
// streaming, now that the payload itself is never buffered.
func TestParseKeepsMdatRegion(t *testing.T) {
	file := buildSegFile(320, 240, []uint32{4, 8}, []uint32{16})
	m, err := Parse(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Mdat) != 1 {
		t.Fatalf("mdat regions: %d, want 1", len(m.Mdat))
	}
	r := m.Mdat[0]
	if r.End-r.Start != 28 { // 4+8 video + 16 audio
		t.Fatalf("mdat payload size = %d, want 28", r.End-r.Start)
	}
	if file[r.Start] != 0xab {
		t.Fatal("mdat region does not point at the media bytes")
	}
}

// --- scale tests -------------------------------------------------------------

// TestMergeManySegmentsRoundTrip concatenates 2000 segments and checks that
// the merged tables collapse and the emitted file reparses with the expected
// totals. This is the multi-thousand-file shape of a long timelapse.
func TestMergeManySegmentsRoundTrip(t *testing.T) {
	const n = 2000
	src := buildSegFile(320, 240, []uint32{100, 200}, []uint32{50})
	movies := make([]*Movie, n)
	payloads := make([][]byte, n)
	for i := 0; i < n; i++ {
		m, err := Parse(bytes.NewReader(src), int64(len(src)))
		if err != nil {
			t.Fatalf("segment %d: %v", i+1, err)
		}
		movies[i] = m
		r := m.Mdat[0]
		payloads[i] = src[r.Start:r.End]
	}
	mg, err := Merge(movies)
	if err != nil {
		t.Fatal(err)
	}
	if got := mg.SegmentCount(); got != n {
		t.Fatalf("segment count = %d, want %d", got, n)
	}
	v, a := mg.tracks[0], mg.tracks[1]
	if len(v.sizes) != 2*n || len(v.chunkSeg) != n {
		t.Fatalf("video: %d samples / %d chunks, want %d / %d", len(v.sizes), len(v.chunkSeg), 2*n, n)
	}
	if len(a.sizes) != n || len(a.chunkSeg) != n {
		t.Fatalf("audio: %d samples / %d chunks, want %d / %d", len(a.sizes), len(a.chunkSeg), n, n)
	}
	if len(v.stts) != 1 || v.stts[0].Count != uint32(2*n) || v.stts[0].Delta != 30 {
		t.Fatalf("video stts = %+v, want one run of %d @30", v.stts, 2*n)
	}
	if len(a.stts) != 1 || a.stts[0].Count != n || a.stts[0].Delta != 1024 {
		t.Fatalf("audio stts = %+v, want one run of %d @1024", a.stts, n)
	}
	if len(v.stsc) != 1 || v.stsc[0].FirstChunk != 1 || v.stsc[0].SamplesPerChunk != 2 {
		t.Fatalf("video stsc = %+v", v.stsc)
	}
	if got := mg.DurationSeconds(); got != float64(n*60)/30 {
		t.Fatalf("duration = %v, want %v", got, float64(n*60)/30)
	}
	out, err := mg.EmitToBytes(payloads)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Parse(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("reparse merged file: %v", err)
	}
	if len(m2.Tracks) != 2 {
		t.Fatalf("merged file: %d tracks, want 2", len(m2.Tracks))
	}
	m2v := m2.Tracks[0]
	if int(m2v.SampleCount) != 2*n || len(m2v.Chunks) != n {
		t.Fatalf("reparsed video: %d samples / %d chunks, want %d / %d",
			m2v.SampleCount, len(m2v.Chunks), 2*n, n)
	}
	var want []byte
	for _, p := range payloads {
		want = append(want, p...)
	}
	var got []byte
	for _, r := range m2.Mdat {
		got = append(got, out[r.Start:r.End]...)
	}
	if bytes.Compare(got, want) != 0 {
		t.Fatalf("merged media differs: got %d bytes, want %d", len(got), len(want))
	}
}

// TestMergedCo64LayoutCrosses4GiB checks the offset arithmetic when the
// concatenation is placed beyond the 4 GiB mark: co64 must be chosen and the
// per-segment mdat bases must reflect ftyp + moov + 16-byte large headers.
func TestMergedCo64LayoutCrosses4GiB(t *testing.T) {
	src := buildSegFile(320, 240, []uint32{10}, []uint32{10})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	mg, err := Merge([]*Movie{m, m})
	if err != nil {
		t.Fatal(err)
	}
	const gbig = int64(1) << 32
	mg.segStart = []int64{0, gbig}
	mg.segSize = []int64{gbig, gbig}
	if err := mg.Finalize(); err != nil {
		t.Fatal(err)
	}
	if !mg.useCo64 {
		t.Fatal("co64 expected for a layout beyond 4 GiB")
	}
	if !bytes.Contains(mg.Moov(), []byte("co64")) {
		t.Fatal("moov does not contain a co64 box")
	}
	hdr := int64(MdatHeaderSize(gbig))
	if got, want := mg.MdatBase(0), int64(len(mg.ftyp))+int64(mg.MoovSize())+hdr; got != want {
		t.Errorf("MdatBase(0) = %d, want %d", got, want)
	}
	if got, want := mg.MdatBase(1)-mg.MdatBase(0), gbig+hdr; got != want {
		t.Errorf("MdatBase(1)-MdatBase(0) = %d, want %d", got, want)
	}
}

// BenchmarkMergeMany measures the concatenation plan for 10000 segments.
func BenchmarkMergeMany(b *testing.B) {
	const n = 10000
	src := buildSegFile(320, 240, []uint32{64}, []uint32{32})
	movies := make([]*Movie, n)
	for i := range movies {
		m, err := Parse(bytes.NewReader(src), int64(len(src)))
		if err != nil {
			b.Fatal(err)
		}
		movies[i] = m
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Merge(movies); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseWideSegment measures parsing a segment with many samples in
// one chunk (dense sample tables).
func BenchmarkParseWideSegment(b *testing.B) {
	sizes := make([]uint32, 4000)
	for i := range sizes {
		sizes[i] = 100
	}
	src := buildSegFile(320, 240, sizes, []uint32{10})
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Parse(bytes.NewReader(src), int64(len(src))); err != nil {
			b.Fatal(err)
		}
	}
}
