package mp4

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// validTrak builds a minimal single-sample video trak accepted by parseTrak.
func validTrak() []byte {
	sps := baselineSPS()
	pps := []byte{0x27, 0x05, 0xeb}
	stbl := Container("stbl",
		stsdVideo(320, 240, sps, pps),
		EncSTTS([]SttsEntry{{Count: 1, Delta: 30}}),
		EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: 1, Description: 1}}),
		EncSTSZ([]uint32{10}),
		EncSTCO([]uint64{100}, false),
	)
	return Container("trak",
		EncTKHD(1, 30, 320, 240, 0, identityMatrixVals),
		Container("mdia",
			EncMDHD(30, 30, 0),
			EncHDLR("vide", "VideoHandler"),
			Container("minf", EncVMHD(), EncDINF(), stbl),
		),
	)
}

// validMoovBytes returns the serialized moov box from the standard fixture.
func validMoovBytes(t *testing.T) []byte {
	t.Helper()
	file := buildTwoTrackMP4(t)
	rd := NewReader(bytes.NewReader(file), int64(len(file)))
	boxes, err := ScanBoxes(rd, 0, int64(len(file)))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, b := range boxes {
		if b.Type == "moov" {
			return file[b.Off : b.Off+b.Size]
		}
	}
	t.Fatal("no moov in fixture")
	return nil
}

// badChild is a 12-byte box claiming a size that overflows its parent.
func badChild() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b, 0x10000000)
	copy(b[4:], "junk")
	return b
}

func TestParseMvhdErrorBranch(t *testing.T) {
	m := &Movie{}
	if err := m.parseMvhd(make([]byte, 3)); err == nil {
		t.Fatal("expected versionFlags error")
	}
}

func TestParseMoovErrorBranches(t *testing.T) {
	// Bad children payload.
	if err := (&Movie{}).parseMoov(badChild()); err == nil {
		t.Fatal("expected children error")
	}
	// mvhd parse failure.
	moov := Container("moov", NewBox("mvhd", make([]byte, 3)))
	if err := (&Movie{}).parseMoov(moov[8:]); err == nil {
		t.Fatal("expected mvhd error")
	}
	// trak parse failure.
	moov = Container("moov", EncMVHD(30, 30), Container("trak", NewBox("tkhd", make([]byte, 8))))
	if err := (&Movie{}).parseMoov(moov[8:]); err == nil {
		t.Fatal("expected trak error")
	}
	// udta / meta / unknown boxes are tolerated alongside a valid trak.
	moov = Container("moov",
		EncMVHD(30, 30),
		NewBox("udta", make([]byte, 8)),
		NewBox("meta", make([]byte, 8)),
		NewBox("junk", make([]byte, 8)),
		validTrak())
	if err := (&Movie{}).parseMoov(moov[8:]); err != nil {
		t.Fatalf("tolerated boxes: %v", err)
	}
	// No tracks.
	if err := (&Movie{}).parseMoov(Container("moov", EncMVHD(30, 30))[8:]); err == nil {
		t.Fatal("expected no-tracks error")
	}
}

func TestParseTrakErrorBranches(t *testing.T) {
	// children() error.
	if _, err := parseTrak(badChild()); err == nil {
		t.Fatal("expected children error")
	}
	// tkhd error, then a valid mdia tail from validTrak.
	vt := validTrak()
	mdiaOff := 0
	for i := 0; i+len("tkhd")+8 <= len(vt); i++ {
		_ = i
	}
	_ = mdiaOff
	tkhdBad := NewBox("tkhd", make([]byte, 8))
	pay := append([]byte{}, tkhdBad...)
	pay = append(pay, vt[findBox(vt, "mdia"):]...)
	if _, err := parseTrak(pay); err == nil {
		t.Fatal("expected tkhd error")
	}
	// mdia with a broken child propagates.
	if _, err := parseTrak(Container("mdia", badChild())); err == nil {
		t.Fatal("expected mdia children error")
	}
}

// findBox locates the start offset of a child box in a container's children
// (vt includes the trak header, so the first child starts at offset 8).
func findBox(trak []byte, fourcc string) int {
	p := 8
	for p < len(trak) {
		if p+8 > len(trak) {
			break
		}
		size := int(binary.BigEndian.Uint32(trak[p:]))
		if string(trak[p+4:p+8]) == fourcc {
			return p
		}
		p += size
	}
	return len(trak)
}

func TestParseTrakEdtsAndExtraBoxes(t *testing.T) {
	sps := baselineSPS()
	pps := []byte{0x27, 0x05, 0xeb}
	stbl := Container("stbl",
		stsdVideo(320, 240, sps, pps),
		EncSTTS([]SttsEntry{{Count: 1, Delta: 30}}),
		EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: 1, Description: 1}}),
		EncSTSZ([]uint32{10}),
		EncSTCO([]uint64{100}, false),
	)
	mdia := Container("mdia",
		EncMDHD(30, 30, 0),
		EncHDLR("vide", "VideoHandler"),
		Container("minf", EncVMHD(), EncDINF(), stbl),
	)
	pay := []byte(EncTKHD(1, 30, 320, 240, 0, identityMatrixVals))
	pay = append(pay, Container("edts", make([]byte, 24))...)
	pay = append(pay, NewBox("udta", make([]byte, 4))...)
	pay = append(pay, NewBox("junk", make([]byte, 4))...)
	pay = append(pay, mdia...)
	tr, err := parseTrak(pay)
	if err != nil {
		t.Fatalf("parseTrak with edts/udta/junk: %v", err)
	}
	if tr.ID != 1 {
		t.Fatalf("track id = %d, want 1", tr.ID)
	}
}

func TestParseMdiaErrorBranches(t *testing.T) {
	minfOK := Container("minf", EncVMHD(), EncDINF())
	tr := &TrackState{}
	// children() error.
	if err := parseMdia(badChild(), tr); err == nil {
		t.Fatal("expected children error")
	}
	// mdhd parse error (truncated), with hdlr/minf present.
	m := append(NewBox("mdhd", make([]byte, 3)), EncHDLR("vide", "V")...)
	m = append(m, minfOK...)
	if err := parseMdia(m, tr); err == nil {
		t.Fatal("expected mdhd error")
	}
	// hdlr versionFlags error, with a valid mdhd.
	m = append(EncMDHD(30, 30, 0), NewBox("hdlr", make([]byte, 3))...)
	m = append(m, minfOK...)
	if err := parseMdia(m, tr); err == nil {
		t.Fatal("expected hdlr error")
	}
}

func TestParseMinfToleratesUnknownBox(t *testing.T) {
	sps := baselineSPS()
	pps := []byte{0x27, 0x05, 0xeb}
	stbl := Container("stbl",
		stsdVideo(320, 240, sps, pps),
		EncSTTS([]SttsEntry{{Count: 1, Delta: 30}}),
		EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: 1, Description: 1}}),
		EncSTSZ([]uint32{10}),
		EncSTCO([]uint64{100}, false),
	)
	minf := Container("minf",
		EncVMHD(),
		NewBox("junk", make([]byte, 8)),
		EncDINF(),
		stbl,
	)
	ts := &TrackState{}
	ts.Handler = "vide"
	if err := parseMinf(minf[8:], ts); err != nil {
		t.Fatalf("minf with unknown box: %v", err)
	}
}

func TestParseStblErrorBranches(t *testing.T) {
	sps := baselineSPS()
	pps := []byte{0x27, 0x05, 0xeb}
	goodStsd := stsdVideo(320, 240, sps, pps)
	goodStts := EncSTTS([]SttsEntry{{Count: 1, Delta: 30}})
	goodStsc := EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: 1, Description: 1}})
	goodStsz := EncSTSZ([]uint32{10})
	goodStco := EncSTCO([]uint64{100}, false)

	stblPay := func(parts ...[]byte) []byte {
		var pay []byte
		for _, p := range parts {
			pay = append(pay, p...)
		}
		return pay
	}

	if err := parseStbl(badChild(), &TrackState{}); err == nil {
		t.Fatal("expected stbl children error")
	}
	// co64 chunk offsets are honored at parse time.
	ts := &TrackState{}
	if err := parseStbl(stblPay(goodStsd, goodStts, goodStsc, goodStsz, EncSTCO([]uint64{1 << 32}, true)), ts); err != nil {
		t.Fatalf("stbl with co64: %v", err)
	}
	if len(ts.Chunks) != 1 || ts.Chunks[0] != 1<<32 {
		t.Fatalf("co64 chunks = %v", ts.Chunks)
	}
	// Broken stsd.
	if err := parseStbl(stblPay(NewBox("stsd", make([]byte, 4)), goodStts, goodStsc, goodStsz, goodStco), &TrackState{}); err == nil {
		t.Fatal("expected stsd error")
	}
	// Broken stts.
	badStts := FullBox("stts", 0, 0, append(be32(3), make([]byte, 16)...))
	if err := parseStbl(stblPay(goodStsd, badStts, goodStsc, goodStsz, goodStco), &TrackState{}); err == nil {
		t.Fatal("expected stts error")
	}
	// Broken stsc.
	badStsc := FullBox("stsc", 0, 0, append(be32(2), make([]byte, 12)...))
	if err := parseStbl(stblPay(goodStsd, goodStts, badStsc, goodStsz, goodStco), &TrackState{}); err == nil {
		t.Fatal("expected stsc error")
	}
	// Broken stsz.
	badStsz := FullBox("stsz", 0, 0, append(append(be32(0), be32(5)...), make([]byte, 8)...))
	if err := parseStbl(stblPay(goodStsd, goodStts, goodStsc, badStsz, goodStco), &TrackState{}); err == nil {
		t.Fatal("expected stsz error")
	}
	// Broken stco (rest < 4).
	badStco := FullBox("stco", 0, 0, nil)
	if err := parseStbl(stblPay(goodStsd, goodStts, goodStsc, goodStsz, badStco), &TrackState{}); err == nil {
		t.Fatal("expected stco error")
	}
	// Broken stss.
	badStss := FullBox("stss", 0, 0, append(be32(3), make([]byte, 8)...))
	if err := parseStbl(stblPay(goodStsd, goodStts, goodStsc, goodStsz, goodStco, badStss), &TrackState{}); err == nil {
		t.Fatal("expected stss error")
	}
	// Broken ctts.
	badCtts := FullBox("ctts", 0, 0, be32(1))
	if err := parseStbl(stblPay(goodStsd, goodStts, goodStsc, goodStsz, goodStco, badCtts), &TrackState{}); err == nil {
		t.Fatal("expected ctts error")
	}
}

func TestParseStsdEntryErrors(t *testing.T) {
	// Entry list with an overflowing child.
	badList := append(be32(1), badChild()...)
	if err := parseStsd(&Box{Type: "stsd", Pay: badList}, &TrackState{}); err == nil {
		t.Fatal("expected entry list error")
	}
	// avc1 entry shorter than the 78-byte fixed header.
	short := NewBox("avc1", make([]byte, 20))
	if err := parseStsd(&Box{Type: "stsd", Pay: append(be32(1), short...)}, &TrackState{}); err == nil {
		t.Fatal("expected truncated sample entry")
	}
}

func TestParseTableTruncations(t *testing.T) {
	if _, err := parseStss(be32(0)); err == nil {
		t.Fatal("expected stss truncation")
	}
	b := &Box{Type: "stco", Pay: be32(0)}
	if _, err := parseChunkOffsets(b); err == nil {
		t.Fatal("expected stco truncation")
	}
}

// flakyFtypReader succeeds everywhere except the second read of the ftyp
// payload (the Parse-loop read after ScanBoxes already consumed it).
type flakyFtypReader struct {
	d         []byte
	ftypReads int
}

func (r *flakyFtypReader) ReadAt(p []byte, off int64) (int, error) {
	if off == 0 && len(p) > 8 {
		r.ftypReads++
		if r.ftypReads == 2 {
			return 0, errors.New("injected ftyp read failure")
		}
	}
	if off < 0 || off+int64(len(p)) > int64(len(r.d)) {
		return 0, io.ErrUnexpectedEOF
	}
	return copy(p, r.d[off:]), nil
}

// zeroReaderAt serves prefix bytes from memory and zeros beyond, up to size.
type zeroReaderAt struct {
	prefix []byte
	size   int64
}

func (r zeroReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.size {
		return 0, io.EOF
	}
	n := int64(len(p))
	if off+n > r.size {
		n = r.size - off
	}
	end := off + n
	var k int64
	if off < int64(len(r.prefix)) {
		k = int64(len(r.prefix))
		if k > end {
			k = end
		}
		copy(p, r.prefix[off:k])
	}
	for i := int(k); i < int(n); i++ {
		p[i] = 0
	}
	return int(n), nil
}

func TestParseTopLevelStructures(t *testing.T) {
	moovOK := validMoovBytes(t)
	ftyp := EncFTYP()

	// Multiple ftyp boxes.
	data := append(append(append([]byte{}, ftyp...), ftyp...), moovOK...)
	if _, err := Parse(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected multiple-ftyp error")
	}

	// ftyp payload read failure in the Parse loop.
	data = append(append([]byte{}, ftyp...), moovOK...)
	if _, err := Parse(&flakyFtypReader{d: data}, int64(len(data))); err == nil {
		t.Fatal("expected ftyp read error")
	}

	// Multiple moov boxes.
	data = append(append(append([]byte{}, ftyp...), moovOK...), NewBox("moov", make([]byte, 16))...)
	if _, err := Parse(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected multiple-moov error")
	}

	// No ftyp box.
	if _, err := Parse(bytes.NewReader(moovOK), int64(len(moovOK))); err == nil {
		t.Fatal("expected no-ftyp error")
	}

	// Moov with broken children.
	badMoov := Container("moov", badChild())
	data = append(append([]byte{}, ftyp...), badMoov...)
	if _, err := Parse(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected moov children error")
	}

	// Moov larger than MaxMoovSize: only the header is real, the payload is
	// served as zeros by a fake ReaderAt.
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint32(hdr, MaxMoovSize+16)
	copy(hdr[4:], "moov")
	prefix := append(append([]byte{}, ftyp...), hdr...)
	zr := zeroReaderAt{prefix: prefix, size: int64(len(prefix)-8) + MaxMoovSize + 16}
	if _, err := Parse(zr, zr.size); err == nil {
		t.Fatal("expected oversized-moov error")
	}
}
