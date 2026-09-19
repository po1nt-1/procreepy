package mp4

import (
	"bytes"
	"errors"
	"testing"
)

func cat(b ...[]byte) []byte {
	var out []byte
	for _, s := range b {
		out = append(out, s...)
	}
	return out
}

// ---- fixture extensions ----------------------------------------------------

// highSPS builds a minimal High-profile (100) SPS: NAL/profile/level bytes
// followed by the bit stream h264SPS consumes after the header — sps_id
// ue(0) and chroma_format_idc ue.
func highSPS(chromaUE string) []byte {
	body := bitsToBytes("1" + chromaUE)
	return append([]byte{0x67, 100, 0, 30}, body...)
}

// chroma UE codes: 0, 1, 2, 3.
var chromaCodes = map[string]string{"0": "1", "1": "010", "2": "011", "3": "00100"}

// avcCWith builds an avcC whose header profile byte matches the SPS.
func avcCWith(sps, pps []byte, profile byte) []byte {
	c := make([]byte, 0, 10+len(sps)+2+len(pps))
	c = append(c, 1, profile, 0, 30, 0xff, 0xe1)
	c = append(c, byte(len(sps)>>8), byte(len(sps)&0xff))
	c = append(c, sps...)
	c = append(c, 1)
	c = append(c, byte(len(pps)>>8), byte(len(pps)&0xff))
	c = append(c, pps...)
	return NewBox("avcC", c)
}

// esdsValidPayload builds an esds box payload (version+flags excluded; the
// caller wraps with FullBox). The descriptor layout is aligned so the
// flat tag/length walker in esdsFindDecoderASC descends the ES_Descriptor
// and lands on the DecoderConfigDescriptor (tag 0x04).
func esdsValidPayload(aot, freqIdx, chans byte) []byte {
	asc := []byte{(aot << 3) | freqIdx, chans << 4}
	dcd := []byte{0x40, 0x15}     // objectType, streamType
	dcd = append(dcd, 0, 0, 0)    // maxBufferSize
	dcd = append(dcd, 0, 0, 0, 0) // maxBitrate
	dcd = append(dcd, 0, 0, 0, 0) // avgBitrate
	dcd = append(dcd, asc...)
	out := []byte{0x03, 0x14}           // ES_Descriptor, 20-byte body
	out = append(out, 0x00, 0x01, 0x00) // es_id + flag pair (walker steps over it)
	out = append(out, 0x04, byte(len(dcd)))
	out = append(out, dcd...)
	return out
}

// buildRichSeg assembles a progressive MP4 with two video chunks (two
// samples each), one audio chunk, optional stss/ctts, an optional esds for
// the audio track (nil keeps the plain esdsAAC fixture), and 64-bit v1
// headers when bigDur is true. offBias is added to every chunk offset.
// len(videoSizes) must be an even number >= 2.
func buildRichSeg(w, h uint16, videoSizes, audioSizes []uint32, sps []byte, prof byte,
	audioEsds []byte, stss []uint32, ctts []CttsEntry, cttsSigned, bigDur bool, offBias int64) []byte {
	pps := []byte{0x27, 0x05, 0xeb}
	vCount := uint32(len(videoSizes))
	aCount := uint32(len(audioSizes))
	vDur := uint64(vCount) * 30
	aDur := uint64(aCount) * 1024
	if bigDur {
		vDur, aDur = 1<<40, 1<<40
	}

	nvChunks := int(vCount) / 2
	var vTotal, aTotal int
	for _, s := range videoSizes {
		vTotal += int(s)
	}
	for _, s := range audioSizes {
		aTotal += int(s)
	}

	vStsc := make([]StscEntry, nvChunks)
	for i := 0; i < nvChunks; i++ {
		vStsc[i] = StscEntry{FirstChunk: uint32(i + 1), SamplesPerChunk: 2, Description: 1}
	}

	audioStsd := stsdAudio(2, 44100, 4)
	if audioEsds != nil {
		audioStsd = FullBox("stsd", 0, 0, append(be32(1),
			NewBox("mp4a", append(make([]byte, 28), audioEsdsAsBox()...))...))
	}

	buildMoov := func(vOffs []uint64, audioOff uint64) []byte {
		vStblKids := [][]byte{
			stsdVideo(w, h, sps, pps),
			EncSTTS([]SttsEntry{{Count: vCount, Delta: 30}}),
			EncSTSC(vStsc),
			EncSTSZ(videoSizes),
			EncSTCO(vOffs, false),
		}
		if stss != nil {
			vStblKids = append(vStblKids, EncSTSS(stss))
		}
		if ctts != nil {
			vStblKids = append(vStblKids, EncCTTS(ctts, cttsSigned))
		}
		vTrak := Container("trak",
			EncTKHD(1, vDur, w, h, 0x0100, identityMatrixVals),
			Container("mdia",
				EncMDHD(30, vDur, 0),
				EncHDLR("vide", "VideoHandler"),
				Container("minf", EncVMHD(), EncDINF(), Container("stbl", vStblKids...)),
			),
		)
		aTrak := Container("trak",
			EncTKHD(2, aDur, 0, 0, 0x0100, identityMatrixVals),
			Container("mdia",
				EncMDHD(44100, aDur, 0),
				EncHDLR("soun", "SoundHandler"),
				Container("minf", EncSMHD(), EncDINF(), Container("stbl",
					audioStsd,
					EncSTTS([]SttsEntry{{Count: aCount, Delta: 1024}}),
					EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: aCount, Description: 1}}),
					EncSTSZ(audioSizes),
					EncSTCO([]uint64{audioOff}, false),
				)),
			),
		)
		return Container("moov", EncMVHD(30, vDur), vTrak, aTrak)
	}

	ftyp := EncFTYP()
	zeroV := make([]uint64, nvChunks)
	_ = zeroV
	moov0 := buildMoov(zeroV, 0)
	mdatBase := int64(len(ftyp)) + int64(len(moov0)) + 8 + offBias
	vOffs := make([]uint64, nvChunks)
	pos := uint64(0)
	for c := 0; c < nvChunks; c++ {
		vOffs[c] = uint64(mdatBase) + pos
		pos += uint64(videoSizes[2*c])
		pos += uint64(videoSizes[2*c+1])
	}
	audioOff := uint64(mdatBase) + uint64(vTotal)
	moov := buildMoov(vOffs, audioOff)

	data := make([]byte, vTotal+aTotal)
	for i := range data {
		data[i] = byte((i * 31) % 251)
	}
	file := make([]byte, 0, len(ftyp)+len(moov)+8+len(data))
	file = append(file, ftyp...)
	file = append(file, moov...)
	file = append(file, NewBox("mdat", data)...)
	return file
}

// audioEsdsAsBox is a placeholder to keep the audioEsds wiring simple; the
// real box is built inline by the caller via esdsValidPayload.
func audioEsdsAsBox() []byte {
	return FullBox("esds", 0, 0, esdsValidPayload(2, 4, 5))
}

func TestRichSegmentRoundTrip(t *testing.T) {
	sps := highSPS(chromaCodes["1"])
	src := buildRichSeg(320, 240, []uint32{10, 12, 14, 16}, []uint32{100, 120},
		sps, 100, esdsValidPayload(2, 4, 5), []uint32{1, 3},
		[]CttsEntry{{Count: 2, Offset: 0}, {Count: 2, Offset: 60}}, false, false, 0)
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	v := m.Tracks[0]
	if len(v.Chunks) != 2 {
		t.Fatalf("video chunks = %d, want 2", len(v.Chunks))
	}
	if len(v.SyncSamples) != 2 || v.SyncSamples[0] != 1 || v.SyncSamples[1] != 3 || !v.HasSyncTable {
		t.Fatalf("sync samples = %v (has=%v)", v.SyncSamples, v.HasSyncTable)
	}
	if len(v.Ctts) != 2 || v.Ctts[1].Offset != 60 || !v.HasCtts || v.CttsSigned {
		t.Fatalf("ctts = %v (has=%v signed=%v)", v.Ctts, v.HasCtts, v.CttsSigned)
	}
	if v.PixFmt != "yuv420p" {
		t.Fatalf("pixfmt = %q, want yuv420p", v.PixFmt)
	}
	sp, err := v.SamplesPerChunk()
	if err != nil || len(sp) != 2 || sp[0] != 2 || sp[1] != 2 {
		t.Fatalf("SamplesPerChunk = %v err=%v", sp, err)
	}
	cb, err := v.ChunkByteSizes()
	if err != nil || cb[0] != 22 || cb[1] != 30 {
		t.Fatalf("ChunkByteSizes = %v err=%v", cb, err)
	}
	if v.chunkCount() != 2 {
		t.Fatalf("chunkCount = %d, want 2", v.chunkCount())
	}
	a := m.Tracks[1]
	if a.AudioChans != 5 {
		t.Fatalf("audio chans = %d, want 5 (esds override)", a.AudioChans)
	}
	if a.AudioRate != 44100 {
		t.Fatalf("audio rate = %d, want 44100", a.AudioRate)
	}

	// Merge two copies and round-trip through EmitToBytes.
	mg, err := Merge([]*Movie{m, m})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	pl := [][]byte{src[m.Mdat[0].Start:m.Mdat[0].End], src[m.Mdat[0].Start:m.Mdat[0].End]}
	out, err := mg.EmitToBytes(pl)
	if err != nil {
		t.Fatalf("EmitToBytes: %v", err)
	}
	m2, err := Parse(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	v2 := m2.Tracks[0]
	if v2.SampleCount != 8 {
		t.Fatalf("merged samples = %d, want 8", v2.SampleCount)
	}
	want := []uint32{1, 3, 5, 7}
	if len(v2.SyncSamples) != 4 {
		t.Fatalf("merged sync = %v", v2.SyncSamples)
	}
	for i, s := range want {
		if v2.SyncSamples[i] != s {
			t.Fatalf("merged sync = %v, want %v", v2.SyncSamples, want)
		}
	}
	if !v2.HasCtts || len(v2.Ctts) != 4 {
		t.Fatalf("merged ctts = %v has=%v", v2.Ctts, v2.HasCtts)
	}
}

func TestRichSegmentSignedCttsAndV1Headers(t *testing.T) {
	sps := baselineSPS()
	src := buildRichSeg(320, 240, []uint32{10, 12}, []uint32{100},
		sps, 66, nil, nil, []CttsEntry{{Count: 2, Offset: -30}}, true, true, 0)
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	v := m.Tracks[0]
	if !v.CttsSigned || len(v.Ctts) != 1 || v.Ctts[0].Offset != -30 {
		t.Fatalf("ctts = %v signed=%v", v.Ctts, v.CttsSigned)
	}
	if m.Mvhd.Duration != 1<<40 {
		t.Fatalf("mvhd duration = %d, want 2^40 (v1 header)", m.Mvhd.Duration)
	}
	if v.TrackDuration != 1<<40 || v.MediaDuration != 1<<40 {
		t.Fatalf("tkhd/mdhd durations = %d/%d, want 2^40", v.TrackDuration, v.MediaDuration)
	}
	if m.Mvhd.Timescale != 30 || v.Timescale != 30 {
		t.Fatalf("timescales = %d/%d", m.Mvhd.Timescale, v.Timescale)
	}
}

func TestParseChunkOutsideMdat(t *testing.T) {
	sps := baselineSPS()
	src := buildRichSeg(320, 240, []uint32{10, 12}, []uint32{100},
		sps, 66, nil, nil, nil, false, false, 1<<20)
	if _, err := Parse(bytes.NewReader(src), int64(len(src))); err == nil {
		t.Fatal("expected validation error for chunks outside mdat")
	}
}

// ---- direct parser error paths ---------------------------------------------

func TestParseMvhdErrors(t *testing.T) {
	var m Movie
	cases := []struct {
		name string
		pay  []byte
	}{
		{"short", []byte{0, 0, 0, 0}},
		{"v0 truncated", FullBox("mvhd", 0, 0, make([]byte, 12))[8:]},
		{"v1 truncated", FullBox("mvhd", 1, 0, make([]byte, 20))[8:]},
		{"bad version", FullBox("mvhd", 2, 0, make([]byte, 28))[8:]},
		{"zero timescale v0", func() []byte {
			p := make([]byte, 16)
			return FullBox("mvhd", 0, 0, p)[8:]
		}()},
	}
	for _, c := range cases {
		if err := m.parseMvhd(c.pay); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestParseTkhdErrors(t *testing.T) {
	var tr TrackState
	cases := []struct {
		name string
		pay  []byte
	}{
		{"short", []byte{0, 0, 0}},
		{"v0 truncated", FullBox("tkhd", 0, 0, make([]byte, 10))[8:]},
		{"v1 truncated", FullBox("tkhd", 1, 0, make([]byte, 20))[8:]},
		{"bad version", FullBox("tkhd", 2, 0, make([]byte, 92))[8:]},
		{"zero id", func() []byte {
			p := make([]byte, 80)
			return FullBox("tkhd", 0, 0, p)[8:] // id at p[8:12] is 0
		}()},
	}
	for _, c := range cases {
		if err := parseTkhd(c.pay, &tr); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestParseMdhdErrors(t *testing.T) {
	var tr TrackState
	cases := []struct {
		name string
		pay  []byte
	}{
		{"short", []byte{0, 0, 0}},
		{"v0 truncated", FullBox("mdhd", 0, 0, make([]byte, 8))[8:]},
		{"v1 truncated", FullBox("mdhd", 1, 0, make([]byte, 20))[8:]},
		{"bad version", FullBox("mdhd", 2, 0, make([]byte, 28))[8:]},
		{"zero timescale", func() []byte {
			p := make([]byte, 22) // timescale at p[8:12] is 0
			return FullBox("mdhd", 0, 0, p)[8:]
		}()},
	}
	for _, c := range cases {
		if err := parseMdhd(c.pay, &tr); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestParseTrakNoMdia(t *testing.T) {
	trak := Container("trak", EncTKHD(1, 10, 320, 240, 0x0100, identityMatrixVals))
	if _, err := parseTrak(trak[8:]); err == nil {
		t.Fatal("expected no-mdia error")
	}
}

func TestParseMdiaIncomplete(t *testing.T) {
	valid := func() []byte {
		return Container("mdia",
			EncMDHD(30, 90, 0),
			EncHDLR("vide", "VideoHandler"),
			Container("minf", EncVMHD(), EncDINF(), Container("stbl")))
	}
	var tr TrackState
	if err := parseMdia(valid()[8:], &tr); err == nil {
		t.Error("expected mdia error (empty stbl)")
	}
	// Missing hdlr.
	noHdlr := Container("mdia", EncMDHD(30, 90, 0),
		Container("minf", EncVMHD(), EncDINF(), Container("stbl")))
	if err := parseMdia(noHdlr[8:], &tr); err == nil {
		t.Error("expected missing-hdlr error")
	}
	// Bad handler.
	badHdlr := Container("mdia",
		EncMDHD(30, 90, 0),
		EncHDLR("subt", "Sub"),
		Container("minf", EncVMHD(), EncDINF(), Container("stbl")))
	if err := parseMdia(badHdlr[8:], &tr); err == nil {
		t.Error("expected bad-handler error")
	}
	// Truncated hdlr payload.
	shortHdlr := FullBox("hdlr", 0, 0, []byte("vide"))
	mShort := Container("mdia",
		EncMDHD(30, 90, 0),
		shortHdlr,
		Container("minf", EncVMHD(), EncDINF(), Container("stbl")))
	if err := parseMdia(mShort[8:], &tr); err == nil {
		t.Error("expected hdlr truncation error")
	}
}

func TestParseMinfNoStbl(t *testing.T) {
	var tr TrackState
	if err := parseMinf(Container("minf", EncVMHD())[8:], &tr); err == nil {
		t.Fatal("expected no-stbl error")
	}
	// Garbage minf payload (children scan error).
	if err := parseMinf([]byte("junk"), &tr); err == nil {
		t.Fatal("expected children error")
	}
}

func TestParseStblIncomplete(t *testing.T) {
	var tr TrackState
	stbl := Container("stbl",
		stsdVideo(320, 240, baselineSPS(), []byte{0x27, 0x05, 0xeb}),
		EncSTTS([]SttsEntry{{1, 30}}),
		EncSTSC([]StscEntry{{1, 1, 1}}),
		EncSTSZ([]uint32{10}),
		// stco missing
	)
	if err := parseStbl(stbl[8:], &tr); err == nil {
		t.Fatal("expected incomplete-stbl error")
	}
}

func TestParseTableErrors(t *testing.T) {
	// stts
	if _, err := parseStts([]byte{0, 0, 0}); err == nil {
		t.Error("stts: short header")
	}
	if _, err := parseStts(FullBox("stts", 1, 0, cat(be32(1), be32(1), be32(1)))[8:]); err == nil {
		t.Error("stts: bad version")
	}
	if _, err := parseStts(FullBox("stts", 0, 0, cat(be32(1), be32(1)))[8:]); err == nil {
		t.Error("stts: wrong length")
	}
	if _, err := parseStts(FullBox("stts", 0, 0, be32(0))[8:]); err == nil {
		t.Error("stts: zero entries")
	}
	if _, err := parseStts(FullBox("stts", 0, 0, cat(be32(2), be32(1), be32(1)))[8:]); err == nil {
		t.Error("stts: truncated body")
	}
	// stsc
	if _, err := parseStsc([]byte{0, 0, 0}); err == nil {
		t.Error("stsc: short header")
	}
	if _, err := parseStsc(FullBox("stsc", 1, 0, cat(be32(1), be32(1), be32(1)))[8:]); err == nil {
		t.Error("stsc: bad version")
	}
	if _, err := parseStsc(FullBox("stsc", 0, 0, cat(be32(1), be32(1)))[8:]); err == nil {
		t.Error("stsc: wrong length")
	}
	if _, err := parseStsc(FullBox("stsc", 0, 0, be32(0))[8:]); err == nil {
		t.Error("stsc: zero entries")
	}
	if _, err := parseStsc(FullBox("stsc", 0, 0, cat(be32(2), be32(1), be32(1), be32(1)))[8:]); err == nil {
		t.Error("stsc: truncated body")
	}
	// stsz
	if _, _, err := parseStsz([]byte{0, 0, 0}); err == nil {
		t.Error("stsz: short header")
	}
	if _, _, err := parseStsz(FullBox("stsz", 1, 0, cat(be32(0), be32(1)))[8:]); err == nil {
		t.Error("stsz: bad version")
	}
	if _, _, err := parseStsz(FullBox("stsz", 0, 0, be32(0))[8:]); err == nil {
		t.Error("stsz: truncated")
	}
	if _, _, err := parseStsz(FullBox("stsz", 0, 0, cat(be32(0), be32(3), be32(1)))[8:]); err == nil {
		t.Error("stsz: truncated sample table")
	}
	// Uniform path.
	n, sizes, err := parseStsz(FullBox("stsz", 0, 0, cat(be32(5), be32(3)))[8:])
	if err != nil || n != 3 || len(sizes) != 3 || sizes[0] != 5 {
		t.Errorf("stsz uniform = %d %v err %v", n, sizes, err)
	}
}

func TestParseChunkOffsets(t *testing.T) {
	if _, err := parseChunkOffsets(&Box{Pay: []byte{0, 0, 0}}); err == nil {
		t.Error("short header")
	}
	if _, err := parseChunkOffsets(&Box{Pay: FullBox("stco", 1, 0, cat(be32(1), be32(1)))[8:]}); err == nil {
		t.Error("bad version")
	}
	if _, err := parseChunkOffsets(&Box{Pay: FullBox("stco", 0, 0, cat(be32(2), be32(1)))[8:]}); err == nil {
		t.Error("truncated")
	}
	// co64 width path.
	pay := FullBox("co64", 0, 0, cat(be32(2), be64(0x1122334455), be64(6)))[8:]
	offs, err := parseChunkOffsets(&Box{Type: "co64", Pay: pay})
	if err != nil || len(offs) != 2 || offs[0] != 0x1122334455 || offs[1] != 6 {
		t.Errorf("co64 = %v err %v", offs, err)
	}
}

func TestParseStssCtts(t *testing.T) {
	if _, err := parseStss([]byte{0, 0, 0}); err == nil {
		t.Error("stss: short")
	}
	if _, err := parseStss(FullBox("stss", 1, 0, cat(be32(1), be32(1)))[8:]); err == nil {
		t.Error("stss: bad version")
	}
	if _, err := parseStss(FullBox("stss", 0, 0, cat(be32(2), be32(1)))[8:]); err == nil {
		t.Error("stss: truncated")
	}
	got, err := parseStss(FullBox("stss", 0, 0, cat(be32(2), be32(1), be32(4)))[8:])
	if err != nil || got[0] != 1 || got[1] != 4 {
		t.Errorf("stss = %v err %v", got, err)
	}

	if _, _, err := parseCtts([]byte{0, 0, 0}); err == nil {
		t.Error("ctts: short")
	}
	if _, _, err := parseCtts(FullBox("ctts", 2, 0, cat(be32(1), be32(1), be32(1)))[8:]); err == nil {
		t.Error("ctts: bad version")
	}
	if _, _, err := parseCtts(FullBox("ctts", 0, 0, cat(be32(1), be32(1)))[8:]); err == nil {
		t.Error("ctts: wrong length")
	}
	if _, _, err := parseCtts(FullBox("ctts", 0, 0, cat(be32(2), be32(1), be32(1)))[8:]); err == nil {
		t.Error("ctts: truncated")
	}
}

func TestParseStsdErrors(t *testing.T) {
	var tr TrackState
	if err := parseStsd(&Box{Type: "stsd", Pay: []byte{0, 0, 0}}, &tr); err == nil {
		t.Error("short stsd")
	}
	if err := parseStsd(&Box{Type: "stsd", Pay: FullBox("stsd", 0, 0, be32(0))[8:]}, &tr); err == nil {
		t.Error("empty stsd")
	}
	// Video entry without dimensions.
	avc1 := NewBox("avc1", make([]byte, 78))
	stsd := FullBox("stsd", 0, 0, append(be32(1), avc1...))
	tr.Handler = "vide"
	if err := parseStsd(&Box{Type: "stsd", Pay: stsd[8:]}, &tr); err == nil {
		t.Error("expected no-dimensions error")
	}
}

func TestParseSampleEntryEdges(t *testing.T) {
	// Visual entry truncated.
	se, err := parseSampleEntry(Box{Type: "avc1", Pay: make([]byte, 77)})
	if err == nil || se.fourcc != "avc1" {
		t.Errorf("avc1 short: %+v err %v", se, err)
	}
	// Audio entry truncated.
	if _, err := parseSampleEntry(Box{Type: "mp4a", Pay: make([]byte, 27)}); err == nil {
		t.Error("mp4a short: expected error")
	}
	// Audio entry whose header rate is zero: esds must supply rate/chans.
	hdr := make([]byte, 28)
	hdr[7] = 1
	entry := Box{Type: "mp4a", Pay: append(hdr, FullBox("esds", 0, 0, esdsValidPayload(2, 4, 5))...)}
	se, err = parseSampleEntry(entry)
	if err != nil {
		t.Fatalf("parseSampleEntry: %v", err)
	}
	if se.audioRate != 44100 || se.audioChan != 5 {
		t.Errorf("esds override: rate=%d chans=%d, want 44100/5", se.audioRate, se.audioChan)
	}
	// Entry body that is not scannable boxes: kept, config ignored.
	junk := make([]byte, 80)
	copy(junk[78:], "junk")
	if _, err := parseSampleEntry(Box{Type: "avc1", Pay: junk}); err != nil {
		t.Errorf("junk body should be tolerated: %v", err)
	}
}

func TestCodecName(t *testing.T) {
	cases := map[string]string{
		"avc1": "h264", "avc3": "h264", "hvc1": "hevc", "hev1": "hevc",
		"vp09": "vp9", "av01": "av1", "ac-3": "ac3", "ac-4": "ec3",
		"alac": "alac", "Opus": "opus", "flac": "flac", "xyz1": "xyz1",
	}
	for in, want := range cases {
		if got := codecName(in, nil); got != want {
			t.Errorf("codecName(%q) = %q, want %q", in, got, want)
		}
	}
	// mp4a falls back to aac regardless of esds validity.
	if got := codecName("mp4a", []byte{1, 2}); got != "aac" {
		t.Errorf("mp4a bad esds = %q", got)
	}
	esds := FullBox("esds", 0, 0, esdsValidPayload(5, 4, 2))[8:]
	if got := codecName("mp4a", esds); got != "aac" {
		t.Errorf("mp4a aot5 = %q", got)
	}
}

func TestPixFmt(t *testing.T) {
	base := func(cfg []byte) *TrackState {
		return &TrackState{Handler: "vide", ConfigBox: "avcC", ConfigData: cfg}
	}
	if got := pixFmt(&TrackState{Handler: "soun"}); got != "" {
		t.Errorf("non-video pixfmt = %q", got)
	}
	if got := pixFmt(&TrackState{Handler: "vide"}); got != "unknown" {
		t.Errorf("no-config pixfmt = %q", got)
	}
	if got := pixFmt(base([]byte{1, 2, 3})); got != "unknown" {
		t.Errorf("short avcC pixfmt = %q", got)
	}
	if got := pixFmt(base(avcC(baselineSPS(), []byte{0x27})[8:])); got != "yuv420p" {
		t.Errorf("baseline pixfmt = %q", got)
	}
	for chroma, want := range map[string]string{"0": "gray", "2": "yuv422p", "3": "yuv444p"} {
		cfg := avcCWith(highSPS(chromaCodes[chroma]), []byte{0x27}, 100)
		if got := pixFmt(base(cfg[8:])); got != want {
			t.Errorf("high chroma %s pixfmt = %q, want %q", chroma, got, want)
		}
	}
	// Unparseable SPS inside avcC.
	bad := append([]byte{1, 100, 0, 30, 0xff, 0xe1}, 0x00, 0x05)
	bad = append(bad, make([]byte, 5)...)
	if got := pixFmt(base(bad)); got != "unknown" {
		t.Errorf("bad SPS pixfmt = %q", got)
	}
}

// ---- exp-golomb / SPS / config ---------------------------------------------

func TestExpGolomb(t *testing.T) {
	g := &expGolomb{bits: bitsToBytes("101")}
	v, err := g.u(3)
	if err != nil || v != 5 {
		t.Errorf("u(3) = %d err %v, want 5", v, err)
	}
	if _, err := g.u(5); err != nil {
		t.Errorf("u(5) = %v", err)
	}
	if _, err := g.bit(); err == nil {
		t.Error("bit past end: expected error")
	}
	cases := map[string]uint32{"1": 0, "010": 1, "011": 2, "00100": 3, "00011110": 14}
	for bits, want := range cases {
		g := &expGolomb{bits: bitsToBytes(bits)}
		v, err := g.ue()
		if err != nil || v != want {
			t.Errorf("ue(%q) = %d err %v, want %d", bits, v, err, want)
		}
	}
	g = &expGolomb{bits: make([]byte, 4)} // 32 zeros
	if _, err := g.ue(); err == nil {
		t.Error("ue: expected >31 zeros error")
	}
	g = &expGolomb{bits: bitsToBytes("00")}
	if _, err := g.u(9); err == nil {
		t.Error("u: expected truncation error")
	}
}

func TestProfileName(t *testing.T) {
	cases := map[uint8]string{
		66: "baseline", 77: "main", 88: "extended", 100: "high",
		110: "high 10", 122: "high 4:2:2", 244: "high 4:4:4 predictive", 99: "profile 99",
	}
	for p, want := range cases {
		if got := ProfileName(p); got != want {
			t.Errorf("ProfileName(%d) = %q, want %q", p, got, want)
		}
	}
}

func TestH264SPS(t *testing.T) {
	// Baseline (assumed 4:2:0).
	p, l, chroma, ok := h264SPS(baselineSPS())
	if !ok || p != 66 || l != 30 || chroma != 1 {
		t.Errorf("baseline = %d %d %d %v", p, l, chroma, ok)
	}
	// Three-byte start code (canonical).
	p, _, _, ok = h264SPS(append([]byte{0, 0, 1}, baselineSPS()...))
	if !ok || p != 66 {
		t.Errorf("start-code prefix: %d %v", p, ok)
	}
	// Quirk: 00 00 00 + SPS strips one zero pair and re-aligns, putting the
	// SPS first byte in the profile slot.
	p, _, _, ok = h264SPS(append([]byte{0, 0, 0}, baselineSPS()...))
	if !ok || p != 0x67 {
		t.Errorf("3-zero prefix: %d %v", p, ok)
	}
	// Quirk: 00 00 00 00 + SPS leaves profile 0.
	p, _, _, ok = h264SPS(append([]byte{0, 0, 0, 0}, baselineSPS()...))
	if !ok || p != 0 {
		t.Errorf("4-zero prefix: %d %v", p, ok)
	}
	// Truncated.
	if _, _, _, ok = h264SPS([]byte{0x67, 0x42}); ok {
		t.Error("expected failure for short SPS")
	}
	// High profile with each chroma.
	for chromaStr, want := range map[string]int{"0": 0, "1": 1, "2": 2, "3": 3} {
		p, _, c, ok := h264SPS(highSPS(chromaCodes[chromaStr]))
		if !ok || p != 100 || c != want {
			t.Errorf("high %s: %d %d %v", chromaStr, p, c, ok)
		}
	}
	// High profile truncated before chroma field.
	high := highSPS(chromaCodes["1"])
	if _, _, _, ok = h264SPS(high[:len(high)-2]); ok {
		t.Error("expected failure for truncated high SPS")
	}
	// SPS whose sps_id ue overruns (all zero bits after header).
	bad := append([]byte{0x67, 0x64, 0, 0x1e}, make([]byte, 4)...)
	if _, _, _, ok = h264SPS(bad); ok {
		t.Error("expected failure for zero-bit SPS body")
	}
}

func TestParseESDS(t *testing.T) {
	// Valid.
	pay := append([]byte{0, 0, 0, 0}, esdsValidPayload(2, 4, 5)...)
	aot, freq, chans, ok := parseESDS(pay)
	if !ok || aot != 2 || freq != 44100 || chans != 5 {
		t.Errorf("valid esds = %d %d %d %v", aot, freq, chans, ok)
	}
	// Short payload.
	if _, _, _, ok = parseESDS([]byte{0, 0}); ok {
		t.Error("short esds: expected false")
	}
	// Out-of-range freq index.
	if _, _, _, ok = parseESDS(append([]byte{0, 0, 0, 0}, esdsValidPayload(2, 15, 2)...)); ok {
		t.Error("freq 15: expected false")
	}
	// aot 0.
	if _, _, _, ok = parseESDS(append([]byte{0, 0, 0, 0}, esdsValidPayload(0, 4, 2)...)); ok {
		t.Error("aot 0: expected false")
	}
	// Descriptor overruns the payload.
	full := append([]byte{0, 0, 0, 0}, esdsValidPayload(2, 4, 5)...)
	if _, _, _, ok = parseESDS(full[:len(full)-1]); ok {
		t.Error("truncated descriptor: expected false")
	}
}

func TestEsdsDescLength(t *testing.T) {
	l, n := esdsDescLength([]byte{0x7f}, 0)
	if l != 0x7f || n != 1 {
		t.Errorf("1-byte = %d %d", l, n)
	}
	l, n = esdsDescLength([]byte{0x81, 0x01}, 0)
	if l != 129 || n != 2 {
		t.Errorf("2-byte = %d %d", l, n)
	}
	l, n = esdsDescLength([]byte{0x80, 0x80, 0x80, 0x80}, 0)
	if n != 4 {
		t.Errorf("4-byte = l %d n %d", l, n)
	}
	l, n = esdsDescLength([]byte{0x80}, 0)
	if l >= 0 || n >= 0 {
		t.Errorf("malformed = %d %d", l, n)
	}
}

func TestEsdsFindDecoderASC(t *testing.T) {
	// DecoderConfigDescriptor body too short.
	pay := []byte{0, 0, 0, 0, 0x04, 0x08, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := esdsFindDecoderASC(pay, 4, len(pay)); got != nil {
		t.Errorf("short DCD: got %v", got)
	}
	// ES descriptor with no DCD inside.
	pay = []byte{0, 0, 0, 0, 0x03, 0x02, 0, 0}
	if got := esdsFindDecoderASC(pay, 4, len(pay)); got != nil {
		t.Errorf("no DCD: got %v", got)
	}
	// Trailing tag without a length byte.
	pay = []byte{0, 0, 0, 0, 0x04}
	if got := esdsFindDecoderASC(pay, 4, len(pay)); got != nil {
		t.Errorf("dangling tag: got %v", got)
	}
	// Valid: expect the ASC bytes back.
	pay = append([]byte{0, 0, 0, 0}, esdsValidPayload(2, 4, 5)...)
	want := []byte{(0x02 << 3) | 4, 0x50}
	if got := esdsFindDecoderASC(pay, 4, len(pay)); !bytes.Equal(got, want) {
		t.Errorf("ASC = %v, want %v", got, want)
	}
}

func TestParseAVCConfig(t *testing.T) {
	if _, _, ok := parseAVCConfig([]byte{1, 2, 3, 4}); ok {
		t.Error("short avcC: expected false")
	}
	// nSPS == 0: profile/level from header, ok.
	p := []byte{1, 66, 0, 30, 0xff, 0xe0}
	if pr, lv, ok := parseAVCConfig(p); !ok || pr != 66 || lv != 30 {
		t.Errorf("nSPS0 = %d %d %v", pr, lv, ok)
	}
	// SPS length overruns.
	p = append([]byte{1, 66, 0, 30, 0xff, 0xe1}, 0x00, 0x09)
	pr, lv, ok := parseAVCConfig(p)
	if !ok || pr != 66 {
		t.Errorf("overrun = %d %d %v", pr, lv, ok)
	}
	// SPS length zero.
	p = append([]byte{1, 66, 0, 30, 0xff, 0xe1}, 0x00, 0x00)
	if _, _, ok := parseAVCConfig(p); !ok {
		t.Error("zero SPS len: expected true (early exit)")
	}
	// Truncated after SPS lengths (no PPS count byte).
	p = []byte{1, 66, 0, 30, 0xff, 0xe1, 0x00, 0x02, 1, 2}
	if _, _, ok := parseAVCConfig(p); !ok {
		t.Error("missing PPS count: expected true (early exit)")
	}
	// PPS length overruns.
	p = []byte{1, 66, 0, 30, 0xff, 0xe1, 0x00, 0x02, 1, 2, 0, 0x00, 0x09}
	if _, _, ok := parseAVCConfig(p); !ok {
		t.Error("PPS overrun: expected true")
	}
}

func TestSpsChromaFromConfig(t *testing.T) {
	if got := spsChromaFromConfig([]byte{1, 2, 3}); got != -1 {
		t.Errorf("short = %d", got)
	}
	if got := spsChromaFromConfig(append([]byte{1, 66, 0, 30, 0xff, 0xe0}, 0)); got != -1 {
		t.Error("nSPS 0: expected -1")
	}
	cfg := avcCWith(highSPS(chromaCodes["2"]), []byte{0x27}, 100)
	if got := spsChromaFromConfig(cfg[8:]); got != 2 {
		t.Errorf("high 422 = %d", got)
	}
	// Truncated SPS length.
	bad := append([]byte{1, 100, 0, 30, 0xff, 0xe1}, 0x00, 0x05, 1, 2, 3)
	if got := spsChromaFromConfig(bad); got != -1 {
		t.Errorf("overrun = %d", got)
	}
}

// ---- state validation -------------------------------------------------------

func validTrack() *TrackState {
	return &TrackState{
		Handler: "vide", FourCC: "avc1",
		SampleCount: 4, SampleSizes: []uint32{10, 12, 14, 16},
		Stts:   []SttsEntry{{4, 30}},
		Stsc:   []StscEntry{{1, 2, 1}, {2, 2, 1}},
		Chunks: []uint64{100, 122},
	}
}

func TestTrackValidateErrors(t *testing.T) {
	mod := func(mutate func(*TrackState)) error {
		tr := validTrack()
		mutate(tr)
		return tr.validate(90, 200)
	}
	cases := map[string]func(*TrackState){
		"video no samples": func(t *TrackState) { t.SampleCount = 0; t.SampleSizes = nil; t.Stts = nil; t.Stsc = nil },
		"no stsc":          func(t *TrackState) { t.Stsc = nil },
		"stsc not at 1":    func(t *TrackState) { t.Stsc[0].FirstChunk = 2 },
		"stsc out of order": func(t *TrackState) {
			t.Stsc = []StscEntry{{1, 1, 1}, {1, 1, 1}}
			t.SampleCount = 2
			t.SampleSizes = []uint32{10, 12}
			t.Stts = []SttsEntry{{2, 30}}
		},
		"stsc zero spc": func(t *TrackState) { t.Stsc[0].SamplesPerChunk = 0 },
		"stsc mismatch": func(t *TrackState) { t.Stsc[1].SamplesPerChunk = 3 },
		"stsc past end": func(t *TrackState) {
			t.Stsc = []StscEntry{{5, 1, 1}}
			t.SampleCount = 1
			t.SampleSizes = []uint32{10}
			t.Stts = []SttsEntry{{1, 30}}
		},
		"stsz mismatch":    func(t *TrackState) { t.SampleSizes = t.SampleSizes[:3] },
		"stts mismatch":    func(t *TrackState) { t.Stts = []SttsEntry{{3, 30}} },
		"sync zero":        func(t *TrackState) { t.SyncSamples = []uint32{0} },
		"sync too big":     func(t *TrackState) { t.SyncSamples = []uint32{9} },
		"sync not sorted":  func(t *TrackState) { t.SyncSamples = []uint32{2, 2} },
		"chunks desc":      func(t *TrackState) { t.Chunks = []uint64{122, 100} },
		"chunk outside":    func(t *TrackState) { t.Chunks = []uint64{900, 950} },
		"too big for mdat": func(t *TrackState) { t.SampleSizes = []uint32{100, 102, 104, 106} },
	}
	for name, mutate := range cases {
		if err := mod(mutate); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if err := validTrack().validate(90, 200); err != nil {
		t.Errorf("valid track: %v", err)
	}
}

func TestMovieValidateErrors(t *testing.T) {
	m := &Movie{}
	if err := m.Validate(); err == nil {
		t.Error("no tracks: expected error")
	}
	m = &Movie{Tracks: []*TrackState{validTrack()}}
	if err := m.Validate(); err == nil {
		t.Error("no mdat: expected error")
	}
	m = &Movie{Tracks: []*TrackState{validTrack()}, Mdat: []Region{{0, 10}, {5, 20}}}
	if err := m.Validate(); err == nil {
		t.Error("overlapping mdat: expected error")
	}
	m = &Movie{
		Tracks: []*TrackState{{Handler: "vide", SampleCount: 0}},
		Mdat:   []Region{{0, 10}},
	}
	if err := m.Validate(); err == nil {
		t.Error("wrapped track error: expected error")
	}
}

// ---- read.go ----------------------------------------------------------------

func TestReaderBounds(t *testing.T) {
	data := []byte("hello world")
	rd := NewReader(bytes.NewReader(data), int64(len(data)))
	if rd.Size() != 11 {
		t.Fatalf("Size = %d", rd.Size())
	}
	buf := make([]byte, 2)
	if _, err := rd.ReadAt(buf, -1); err == nil {
		t.Error("negative offset: expected error")
	}
	if _, err := rd.ReadAt(buf, 10); err == nil {
		t.Error("oversized read: expected error")
	}
	copy(buf, "hi")
	if n, err := rd.ReadAt(buf, 9); err != nil || n != 2 {
		t.Errorf("boundary read = %d %v, want 2 nil", n, err)
	}
	// A short underlying read is passed through as-is.
	sd := NewReader(shortReader{}, 100)
	buf2 := make([]byte, 4)
	if n, err := sd.ReadAt(buf2, 0); err != nil || n != 1 {
		t.Errorf("short read = %d %v, want 1 nil", n, err)
	}
}

// shortReader returns one byte without an error.
type shortReader struct{}

func (shortReader) ReadAt(p []byte, off int64) (int, error) { return 1, nil }

func TestReaderWrapsEOF(t *testing.T) {
	rd := NewReader(bytes.NewReader(nil), 100)
	if _, err := rd.ReadAt(make([]byte, 4), 0); err == nil {
		t.Fatal("expected error from empty reader")
	}
}

type errReader struct{}

func (errReader) ReadAt(p []byte, off int64) (int, error) { return 0, errors.New("unexpected EOF") }

func TestReaderUnexpectedEOFWrap(t *testing.T) {
	rd := NewReader(errReader{}, 100)
	if _, err := rd.ReadAt(make([]byte, 4), 0); err != ErrTruncated {
		t.Errorf("err = %v, want ErrTruncated", err)
	}
}

func TestScanBoxesErrors(t *testing.T) {
	seg := func(b []byte) *Reader { return NewReader(bytes.NewReader(b), int64(len(b))) }
	cases := map[string][]byte{
		"trailing 7":        []byte{1, 2, 3, 4, 5, 6, 7},
		"invalid fourcc":    {0, 0, 0, 8, 'm', 0x01, 'd', 'a'},
		"tiny box":          {0, 0, 0, 4, 'a', 'b', 'c', 'd'},
		"overflow":          {0, 0, 0, 20, 'm', 'd', 'a', 't'},
		"largesize too big": {0, 0, 0, 1, 'm', 'd', 'a', 't', 0, 0, 0, 0, 0, 0, 0x10, 0},
		"largesize < hdr":   {0, 0, 0, 1, 'm', 'd', 'a', 't', 0, 0, 0, 0, 0, 0, 0, 10},
	}
	for name, b := range cases {
		rd := seg(b)
		if _, err := ScanBoxes(rd, 0, int64(len(b))); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// Range errors.
	if _, err := ScanBoxes(seg([]byte{0, 0, 0, 8, 'a', 'b', 'c', 'd'}), 0, 999); err == nil {
		t.Error("end beyond size")
	}
	if _, err := ScanBoxes(seg([]byte{}), 1, 0); err == nil {
		t.Error("off > end")
	}
	// Size-0 box extends to end: must succeed.
	b := []byte{0, 0, 0, 0, 'a', 'b', 'c', 'd', 1, 2, 3}
	boxes, err := ScanBoxes(seg(b), 0, int64(len(b)))
	if err != nil || len(boxes) != 1 || boxes[0].Size != int64(len(b)) {
		t.Errorf("size-0 box = %+v err %v", boxes, err)
	}
	// Largesize form with 12-byte payload box: 16-byte header.
	b = append([]byte{0, 0, 0, 1, 'b', 'i', 'g', 'x', 0, 0, 0, 0, 0, 0, 0, 28}, make([]byte, 12)...)
	boxes, err = ScanBoxes(seg(b), 0, int64(len(b)))
	if err != nil || len(boxes) != 1 || boxes[0].Size != 28 || len(boxes[0].Pay) != 12 {
		t.Errorf("largesize box = %+v err %v", boxes, err)
	}
	// Reader whose underlying read fails mid-payload.
	b = []byte{0, 0, 0, 12, 'm', 'd', 'a', 't', 1, 2, 3}
	rd := NewReader(bytes.NewReader(b), int64(len(b)))
	if _, err := ScanBoxes(rd, 0, int64(len(b))); err == nil {
		t.Error("payload read error: expected")
	}
}

func TestValidFourCC(t *testing.T) {
	if err := validFourCC([]byte{'m', 0x00, 'd', 'a'}); err == nil {
		t.Error("control char: expected error")
	}
	if err := validFourCC([]byte{'a', 'b', 'c', 'd'}); err != nil {
		t.Errorf("valid fourcc: %v", err)
	}
}

func TestVersionFlags(t *testing.T) {
	if _, _, err := versionFlags([]byte{0, 0}); err == nil {
		t.Error("short: expected error")
	}
	ver, flags, err := versionFlags([]byte{1, 0x12, 0x34, 0x56})
	if err != nil || ver != 1 || flags != 0x123456 {
		t.Errorf("versionFlags = %d %d %v", ver, flags, err)
	}
}

func TestSliceReader(t *testing.T) {
	sr := &sliceReader{b: []byte("abc")}
	buf := make([]byte, 2)
	if _, err := sr.ReadAt(buf, 5); err == nil {
		t.Error("past end: expected error")
	}
	if _, err := sr.ReadAt(buf, 2); err == nil {
		t.Error("short: expected error")
	}
	if _, err := sr.ReadAt(buf, 1); err != nil || string(buf) != "bc" {
		t.Errorf("read = %q %v", buf, err)
	}
}

// ---- write.go ----------------------------------------------------------------

func TestEncSTSSRoundTrip(t *testing.T) {
	box := EncSTSS([]uint32{1, 3})
	if string(box[4:8]) != "stss" {
		t.Fatalf("fourcc = %q", box[4:8])
	}
	got, err := parseStss(box[8:])
	if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("round trip = %v err %v", got, err)
	}
}

func TestEncCTTSRoundTrip(t *testing.T) {
	unsigned := EncCTTS([]CttsEntry{{2, 0}, {2, 60}}, false)
	if unsigned[8] != 0 {
		t.Errorf("unsigned version = %d", unsigned[8])
	}
	got, signed, err := parseCtts(unsigned[8:])
	if err != nil || signed || len(got) != 2 || got[1].Offset != 60 {
		t.Errorf("unsigned round trip = %v %v %v", got, signed, err)
	}
	signedBox := EncCTTS([]CttsEntry{{2, -30}}, true)
	if signedBox[8] != 1 {
		t.Errorf("signed version = %d", signedBox[8])
	}
	got, signed, err = parseCtts(signedBox[8:])
	if err != nil || !signed || got[0].Offset != -30 {
		t.Errorf("signed round trip = %v %v %v", got, signed, err)
	}
}

func TestPad4(t *testing.T) {
	if got := pad4("abcdef"); got != "abcd" {
		t.Errorf("long = %q", got)
	}
	if got := pad4("ab"); got != "ab\x00\x00" {
		t.Errorf("short = %q", got)
	}
	if got := pad4("abcd"); got != "abcd" {
		t.Errorf("exact = %q", got)
	}
}

func TestEncV1Headers(t *testing.T) {
	mvhd := EncMVHD(30, 1<<40)
	if mvhd[8] != 1 {
		t.Fatalf("mvhd version = %d", mvhd[8])
	}
	var m Movie
	if err := m.parseMvhd(mvhd[8:]); err != nil || m.Mvhd.Duration != 1<<40 {
		t.Errorf("mvhd v1 = %+v err %v", m.Mvhd, err)
	}
	tkhd := EncTKHD(7, 1<<40, 320, 240, 0x0100, identityMatrixVals)
	if tkhd[8] != 1 {
		t.Fatalf("tkhd version = %d", tkhd[8])
	}
	var tr TrackState
	if err := parseTkhd(tkhd[8:], &tr); err != nil || tr.ID != 7 || tr.TrackDuration != 1<<40 {
		t.Errorf("tkhd v1 = %+v err %v", tr, err)
	}
	mdhd := EncMDHD(44100, 1<<40, 0x4e5544)
	if mdhd[8] != 1 {
		t.Fatalf("mdhd version = %d", mdhd[8])
	}
	var t2 TrackState
	if err := parseMdhd(mdhd[8:], &t2); err != nil || t2.MediaDuration != 1<<40 || t2.Language != 0x4e5544 {
		t.Errorf("mdhd v1 = %+v err %v", t2, err)
	}
	// Small durations stay v0.
	if EncMVHD(30, 90)[8] != 0 || EncMDHD(30, 90, 0)[8] != 0 || EncTKHD(1, 90, 0, 0, 0, identityMatrixVals)[8] != 0 {
		t.Error("small headers should be version 0")
	}
}

// ---- concat.go ---------------------------------------------------------------

func audioOnlyMovie() *Movie {
	return &Movie{
		Ftyp:   EncFTYP(),
		Mvhd:   Mvhd{Timescale: 30, Duration: 90},
		Mdat:   []Region{{Start: 0, End: 10}},
		Tracks: []*TrackState{{Handler: "soun", SampleCount: 1, SampleSizes: []uint32{10}}},
	}
}

func TestMergeErrors(t *testing.T) {
	if _, err := Merge(nil); err == nil {
		t.Error("empty: expected error")
	}
	src := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Merge([]*Movie{nil}); err == nil {
		t.Error("nil movie: expected error")
	}
	two := *m
	two.Mdat = append(two.Mdat, Region{Start: 1000, End: 2000})
	if _, err := Merge([]*Movie{&two}); err == nil {
		t.Error("two mdats: expected error")
	}
	if _, err := Merge([]*Movie{audioOnlyMovie()}); err == nil {
		t.Error("no video: expected error")
	}
	m2 := *m
	m2.Mvhd.Timescale = 60
	if _, err := Merge([]*Movie{m, &m2}); err == nil {
		t.Error("timescale mismatch: expected error")
	}
	// More than five bad segments triggers the truncation branch.
	var ms []*Movie
	ms = append(ms, m)
	for i := 0; i < 6; i++ {
		bad := *m
		bad.Mvhd.Timescale = 60
		ms = append(ms, &bad)
	}
	if _, err := Merge(ms); err == nil {
		t.Error("six bad: expected error")
	}
}

func TestMergeFtypFallbackAndAccessors(t *testing.T) {
	src := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	m.Ftyp = nil
	mg, err := Merge([]*Movie{m})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mg.Ftyp(), EncFTYP()) {
		t.Error("ftyp fallback mismatch")
	}
	if mg.SegmentCount() != 1 {
		t.Errorf("SegmentCount = %d", mg.SegmentCount())
	}
	if mg.MdatSourceStart(0) != m.Mdat[0].Start || mg.MdatPayloadSize(0) != m.Mdat[0].End-m.Mdat[0].Start {
		t.Errorf("mdat source = %d %d", mg.MdatSourceStart(0), mg.MdatPayloadSize(0))
	}
	if mg.DurationSeconds() != 90.0/30.0 {
		t.Errorf("DurationSeconds = %f", mg.DurationSeconds())
	}
	moov := mg.Moov()
	if mg.MoovSize() != len(moov) {
		t.Errorf("MoovSize %d != len(Moov) %d", mg.MoovSize(), len(moov))
	}
	if mg.MdatBase(0) < 0 {
		t.Errorf("MdatBase = %d", mg.MdatBase(0))
	}
	// Second Finalize call is a no-op.
	if err := mg.Finalize(); err != nil {
		t.Errorf("second Finalize: %v", err)
	}
}

func TestEmitToWriter(t *testing.T) {
	src := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	mg, err := Merge([]*Movie{m, m})
	if err != nil {
		t.Fatal(err)
	}
	pl := [][]byte{src[m.Mdat[0].Start:m.Mdat[0].End], src[m.Mdat[0].Start:m.Mdat[0].End]}
	var buf bytes.Buffer
	if err := mg.Emit(&buf, pl); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if _, err := Parse(bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if err := mg.Emit(&buf, pl[:1]); err == nil {
		t.Error("wrong payload count: expected error")
	}
	if err := mg.Emit(&buf, [][]byte{pl[0][:5], pl[1]}); err == nil {
		t.Error("wrong payload size: expected error")
	}
}

// failAtWriter fails after writing n bytes total.
type failAtWriter struct {
	buf       bytes.Buffer
	failAfter int
}

func (w *failAtWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.failAfter {
		return 0, errors.New("simulated write failure")
	}
	n := len(p)
	if w.buf.Len()+n > w.failAfter {
		n = w.failAfter - w.buf.Len()
	}
	w.buf.Write(p[:n])
	return n, nil
}

func TestEmitWriteErrors(t *testing.T) {
	src := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	mg, _ := Merge([]*Movie{m})
	pl := [][]byte{src[m.Mdat[0].Start:m.Mdat[0].End]}
	ftypLen, moovLen := len(mg.Ftyp()), mg.MoovSize()
	heads := ftypLen + moovLen + 8
	for name, failAfter := range map[string]int{
		"ftyp":    0,
		"moov":    ftypLen,
		"head":    ftypLen + moovLen,
		"payload": heads,
	} {
		w := &failAtWriter{failAfter: failAfter}
		if err := mg.Emit(w, pl); err == nil {
			t.Errorf("%s: expected write error", name)
		}
	}
	if _, err := mg.EmitToBytes(nil); err == nil {
		t.Error("EmitToBytes: expected error")
	}
}

func TestMdatHead(t *testing.T) {
	h, err := MdatHead(10)
	if err != nil || len(h) != 8 {
		t.Errorf("small = %v err %v", h, err)
	}
	if MdatHeaderSize(10) != 8 || MdatHeaderSize(1<<32) != 16 {
		t.Error("MdatHeaderSize")
	}
	h, err = MdatHead(1 << 32)
	if err != nil || len(h) != 16 || h[3] != 1 || string(h[12:16]) != "mdat" {
		t.Errorf("large = %v err %v", h, err)
	}
	if _, err := MdatHead(-1); err == nil {
		t.Error("negative: expected error")
	}
	if _, err := MdatHead(1 << 62); err == nil {
		t.Error("huge: expected error")
	}
}

func TestMergedCo64Path(t *testing.T) {
	src := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	mg, err := Merge([]*Movie{m})
	if err != nil {
		t.Fatal(err)
	}
	// Force the chunk offsets past 4 GiB so co64 must be selected.
	mg.tracks[0].chunkRel[0] = 1 << 40
	if err := mg.Finalize(); err != nil {
		t.Fatal(err)
	}
	if !mg.useCo64 {
		t.Fatal("expected co64 selection")
	}
	// The serialized moov must carry the 64-bit co64 box.
	if !bytes.Contains(mg.Moov(), []byte("co64")) {
		t.Error("moov missing co64 box")
	}
}

func TestSameStreamsAndSummary(t *testing.T) {
	src := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatal(err)
	}
	if !SameStreams(m, m) {
		t.Error("self comparison should match")
	}
	one := *m
	one.Tracks = one.Tracks[:1]
	if SameStreams(m, &one) {
		t.Error("different track counts should differ")
	}
	if m.StreamsSummary() == "" || m.StreamsSummary() == "no streams" {
		t.Errorf("summary = %q", m.StreamsSummary())
	}
	if got := describeSigs(nil); got != "no streams" {
		t.Errorf("empty sigs = %q", got)
	}
	if !m.HasVideo() {
		t.Error("HasVideo")
	}
	if audioOnlyMovie().HasVideo() {
		t.Error("audio-only HasVideo")
	}
}
