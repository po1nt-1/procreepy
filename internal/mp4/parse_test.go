package mp4

import (
	"bytes"
	"testing"
)

// bitsToBytes packs a string of '0'/'1' into bytes, zero-padding the tail.
func bitsToBytes(s string) []byte {
	out := make([]byte, (len(s)+7)/8)
	for i, c := range s {
		if c == '1' {
			out[i/8] |= 1 << (7 - i%8)
		}
	}
	return out
}

// baselineSPS builds a minimal valid H.264 baseline SPS (320x240).
func baselineSPS() []byte {
	bits := ""
	bits += "0" + "11" + "00111" // forbidden, nal_ref_idc=3, nal_unit_type=7 (SPS)
	bits += "01000010"           // profile_idc = 66 (baseline)
	bits += "00000000"           // constraint flags
	bits += "00011110"           // level_idc = 30
	bits += "1"                  // sps_id ue(0)
	bits += "1"                  // log2_max_frame_num_minus4 ue(0)
	bits += "1"                  // pic_order_cnt_type ue(0)
	bits += "1"                  // log2_max_pic_order_cnt_lsb_minus4 ue(0)
	bits += "011"                // max_num_ref_frames ue(1)
	bits += "0"                  // gaps_in_frame_num_value_allowed
	bits += "00000101011"        // pic_width_in_mbs_minus1 ue(19) => 20 mbs = 320px
	bits += "000010110"          // pic_height_in_map_units_minus1 ue(14) => 15 = 240px
	bits += "1"                  // frame_mbs_only
	bits += "1"                  // direct_8x8_inference
	bits += "0"                  // frame_cropping
	bits += "0"                  // vui_parameters_present
	return bitsToBytes(bits)
}

// avcC builds an avcC config box containing the given SPS and PPS.
func avcC(sps, pps []byte) []byte {
	c := make([]byte, 0, 10+len(sps)+2+len(pps))
	c = append(c, 1)    // configurationVersion
	c = append(c, 66)   // profile (baseline, matches SPS)
	c = append(c, 0)    // profile_compat
	c = append(c, 30)   // level
	c = append(c, 0xff) // reserved(3)=111 lengthSizeMinusOne(2)=11
	c = append(c, 0xe1) // reserved(3)=111 numOfSPS(5)=1
	c = append(c, byte(len(sps)>>8), byte(len(sps)&0xff))
	c = append(c, sps...)
	c = append(c, 1) // numOfPPS
	c = append(c, byte(len(pps)>>8), byte(len(pps)&0xff))
	c = append(c, pps...)
	return NewBox("avcC", c)
}

// videoEntry builds an avc1 sample entry (after size+fourcc) at w x h.
func videoEntry(w, h uint16, cfg []byte) []byte {
	b := make([]byte, 78)
	b[7] = 1 // data_reference_index
	b[24] = byte(w >> 8)
	b[25] = byte(w & 0xff)
	b[26] = byte(h >> 8)
	b[27] = byte(h & 0xff)
	// horiz/vert resolution 72 dpi
	b[28] = 0
	b[29] = 0x48
	b[30] = 0
	b[31] = 0
	b[32] = 0
	b[33] = 0x48
	b[34] = 0
	b[35] = 0
	b[36] = 0 // reserved 4
	b[40] = 0 // frame_count
	b[41] = 1
	// depth = 24
	b[74] = 0
	b[75] = 0x18
	b[76] = 0xff // pre_defined
	b[77] = 0xff
	return append(b, cfg...)
}

// audioEntry builds an mp4a sample entry (after size+fourcc).
func audioEntry(chans uint16, rate uint32, cfg []byte) []byte {
	b := make([]byte, 28)
	b[6] = 0 // data_reference_index
	b[7] = 1
	b[16] = byte(chans >> 8) // channelcount
	b[17] = byte(chans & 0xff)
	b[18] = 0                              // samplesize high
	b[19] = 16                             // samplesize low
	copy(b[24:28], be32(uint32(rate)<<16)) // samplerate, 16.16 fixed point
	return append(b, cfg...)
}

// esdsAAC builds an esds box carrying an AAC AudioSpecificConfig.
func esdsAAC(freqIdx byte, chans uint16) []byte {
	asc := []byte{(0x02 << 3) | freqIdx, (byte(chans) << 4)} // aot=2 (AAC-LC)
	dcd := make([]byte, 0, 13+len(asc))
	dcd = append(dcd, 0x40)       // objectType
	dcd = append(dcd, 0x15)       // streamType (audio, with dep)
	dcd = append(dcd, 0, 0, 0)    // bufferSize
	dcd = append(dcd, 0, 0, 0, 0) // maxBitrate
	dcd = append(dcd, 0, 0, 0, 0) // avgBitrate
	dcd = append(dcd, asc...)
	es := make([]byte, 0, 7+len(dcd))
	es = append(es, 0x00, 0x02) // ES_ID
	es = append(es, 0x00)       // flags
	es = append(es, 0, 0, 0, 0) // OCRes+max
	es = append(es, dcd...)
	// wrap in descriptors with length prefixes
	var out []byte
	out = append(out, 0x03) // ES_Descriptor
	out = append(out, esdsLenByte(len(es)))
	out = append(out, es...)
	return FullBox("esds", 0, 0, out)
}

func esdsLenByte(n int) byte { return byte(n) }

// stsdVideo builds an stsd with a single avc1 entry.
func stsdVideo(w, h uint16, sps, pps []byte) []byte {
	entry := NewBox("avc1", videoEntry(w, h, avcC(sps, pps)))
	return FullBox("stsd", 0, 0, append(be32(1), entry...))
}

// stsdAudio builds an stsd with a single mp4a entry.
func stsdAudio(chans uint16, rate uint32, freqIdx byte) []byte {
	entry := NewBox("mp4a", audioEntry(chans, rate, esdsAAC(freqIdx, chans)))
	return FullBox("stsd", 0, 0, append(be32(1), entry...))
}

// buildSegFile assembles a valid progressive MP4: ftyp + moov(v,a) + mdat.
// The video track has len(videoSizes) samples (30 timescale, 30 tick each)
// and the audio track has len(audioSizes) samples (44100, 1024 each), one
// chunk per track. Layout: [ftyp][moov][mdat(video|audio)].
func buildSegFile(w, h uint16, videoSizes, audioSizes []uint32) []byte {
	sps := baselineSPS()
	pps := []byte{0x27, 0x05, 0xeb}
	vCount := uint32(len(videoSizes))
	aCount := uint32(len(audioSizes))
	vDur := uint64(vCount) * 30
	aDur := uint64(aCount) * 1024

	buildMoov := func(videoOff, audioOff uint64) []byte {
		vStbl := Container("stbl",
			stsdVideo(w, h, sps, pps),
			EncSTTS([]SttsEntry{{Count: vCount, Delta: 30}}),
			EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: vCount, Description: 1}}),
			EncSTSZ(videoSizes),
			EncSTCO([]uint64{videoOff}, false),
		)
		vTrak := Container("trak",
			EncTKHD(1, vDur, w, h, 0x0100, identityMatrixVals),
			Container("mdia",
				EncMDHD(30, vDur, 0),
				EncHDLR("vide", "VideoHandler"),
				Container("minf", EncVMHD(), EncDINF(), vStbl),
			),
		)
		aStbl := Container("stbl",
			stsdAudio(2, 44100, 4),
			EncSTTS([]SttsEntry{{Count: aCount, Delta: 1024}}),
			EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: aCount, Description: 1}}),
			EncSTSZ(audioSizes),
			EncSTCO([]uint64{audioOff}, false),
		)
		aTrak := Container("trak",
			EncTKHD(2, aDur, 0, 0, 0x0100, identityMatrixVals),
			Container("mdia",
				EncMDHD(44100, aDur, 0),
				EncHDLR("soun", "SoundHandler"),
				Container("minf", EncSMHD(), EncDINF(), aStbl),
			),
		)
		return Container("moov", EncMVHD(30, vDur), vTrak, aTrak)
	}

	vTotal, aTotal := 0, 0
	for _, s := range videoSizes {
		vTotal += int(s)
	}
	for _, s := range audioSizes {
		aTotal += int(s)
	}
	ftyp := EncFTYP()
	moov0 := buildMoov(0, 0)
	mdatBase := int64(len(ftyp)) + int64(len(moov0)) + 8 // +8 mdat header
	moov := buildMoov(uint64(mdatBase), uint64(mdatBase)+uint64(vTotal))

	mdat := NewBox("mdat", bytes.Repeat([]byte{0xab}, vTotal+aTotal))
	file := make([]byte, 0, len(ftyp)+len(moov)+len(mdat))
	file = append(file, ftyp...)
	file = append(file, moov...)
	file = append(file, mdat...)
	return file
}

// buildTwoTrackMP4 is the standard 3-frame / 2-sample test segment.
func buildTwoTrackMP4(t *testing.T) []byte {
	t.Helper()
	return buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
}

func TestParseTwoTrackMP4(t *testing.T) {
	file := buildTwoTrackMP4(t)
	m, err := Parse(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(m.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(m.Tracks))
	}
	if m.Mvhd.Timescale != 30 || m.Mvhd.Duration != 90 {
		t.Fatalf("mvhd = %+v, want ts=30 dur=90", m.Mvhd)
	}
	if len(m.Mdat) != 1 {
		t.Fatalf("mdat regions = %d, want 1", len(m.Mdat))
	}

	v := m.Tracks[0]
	if v.Handler != "vide" || v.ID != 1 {
		t.Fatalf("video track = %+v", v)
	}
	if v.FourCC != "avc1" || v.CodecName != "h264" {
		t.Fatalf("video codec = %q/%q, want avc1/h264", v.FourCC, v.CodecName)
	}
	if v.Width != 320 || v.Height != 240 {
		t.Fatalf("video dims = %dx%d, want 320x240", v.Width, v.Height)
	}
	if v.PixFmt != "yuv420p" {
		t.Fatalf("pixfmt = %q, want yuv420p", v.PixFmt)
	}
	if v.Timescale != 30 || v.MediaDuration != 90 {
		t.Fatalf("video mdhd = ts=%d dur=%d, want 30/90", v.Timescale, v.MediaDuration)
	}
	if v.SampleCount != 3 || len(v.SampleSizes) != 3 {
		t.Fatalf("video samples = %d", v.SampleCount)
	}
	if v.TotalDuration() != 90 {
		t.Fatalf("video total dur = %d, want 90", v.TotalDuration())
	}
	if len(v.Chunks) != 1 {
		t.Fatalf("video chunks = %d", len(v.Chunks))
	}

	a := m.Tracks[1]
	if a.Handler != "soun" || a.ID != 2 {
		t.Fatalf("audio track = %+v", a)
	}
	if a.CodecName != "aac" {
		t.Fatalf("audio codec = %q, want aac", a.CodecName)
	}
	if a.AudioRate != 44100 || a.AudioChans != 2 {
		t.Fatalf("audio rate/chans = %d/%d, want 44100/2", a.AudioRate, a.AudioChans)
	}
	if a.SampleCount != 2 {
		t.Fatalf("audio samples = %d, want 2", a.SampleCount)
	}
	if a.TotalDuration() != 2048 {
		t.Fatalf("audio total dur = %d, want 2048", a.TotalDuration())
	}
}

func TestParseRejectsFragmented(t *testing.T) {
	// ftyp + moof => fragmented, must be rejected.
	ftyp := EncFTYP()
	moof := Container("moof", make([]byte, 16))
	file := append(append([]byte{}, ftyp...), moof...)
	if _, err := Parse(bytes.NewReader(file), int64(len(file))); err == nil {
		t.Fatal("expected error for fragmented file")
	}
}

func TestParseRejectsMissingMoov(t *testing.T) {
	ftyp := EncFTYP()
	mdat := NewBox("mdat", []byte{1, 2, 3, 4})
	file := append(append([]byte{}, ftyp...), mdat...)
	if _, err := Parse(bytes.NewReader(file), int64(len(file))); err == nil {
		t.Fatal("expected error for missing moov")
	}
}

func TestParseRejectsTruncatedStsz(t *testing.T) {
	file := buildTwoTrackMP4(t)
	// Truncate the file mid-way through moov to trigger a structure error.
	cut := len(file) / 2
	if _, err := Parse(bytes.NewReader(file[:cut]), int64(cut)); err == nil {
		t.Fatal("expected error for truncated file")
	}
}

func TestEncSTCOSelectsCo64(t *testing.T) {
	box := EncSTCO([]uint64{1 << 32}, true)
	if string(box[4:8]) != "co64" {
		t.Fatalf("expected co64, got %q", box[4:8])
	}
	box32 := EncSTCO([]uint64{100}, false)
	if string(box32[4:8]) != "stco" {
		t.Fatalf("expected stco, got %q", box32[4:8])
	}
}

// TestConcatRoundTripSingle emits one segment and checks the result reparses
// to the same logical content with correct, self-consistent offsets.
func TestConcatRoundTripSingle(t *testing.T) {
	src := buildTwoTrackMP4(t)
	m, err := Parse(bytes.NewReader(src), int64(len(src)))
	if err != nil {
		t.Fatalf("parse src: %v", err)
	}
	mg, err := Merge([]*Movie{m})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	payload := src[m.Mdat[0].Start:m.Mdat[0].End]
	out, err := mg.EmitToBytes([][]byte{payload})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	outM, err := Parse(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("parse out: %v", err)
	}
	if len(outM.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(outM.Tracks))
	}
	v, a := outM.Tracks[0], outM.Tracks[1]
	if v.SampleCount != 3 || a.SampleCount != 2 {
		t.Fatalf("samples v/a = %d/%d, want 3/2", v.SampleCount, a.SampleCount)
	}
	if v.TotalDuration() != 90 || a.TotalDuration() != 2048 {
		t.Fatalf("durations v/a = %d/%d, want 90/2048", v.TotalDuration(), a.TotalDuration())
	}
	if v.Width != 320 || v.Height != 240 || v.CodecName != "h264" {
		t.Fatalf("video = %s %dx%d", v.CodecName, v.Width, v.Height)
	}
	if a.AudioRate != 44100 || a.AudioChans != 2 {
		t.Fatalf("audio = %dHz/%dch", a.AudioRate, a.AudioChans)
	}
	if len(v.Chunks) != 1 {
		t.Fatalf("video chunks = %d, want 1", len(v.Chunks))
	}
	if v.Chunks[0] != uint64(mg.MdatBase(0)) {
		t.Fatalf("video chunk off = %d, want %d", v.Chunks[0], mg.MdatBase(0))
	}
	if a.Chunks[0] != uint64(mg.MdatBase(0))+36 {
		t.Fatalf("audio chunk off = %d, want %d", a.Chunks[0], mg.MdatBase(0)+36)
	}
}

// TestConcatTwoSegments concatenates two identical segments and verifies the
// merged tables, durations, and chunk offsets.
func TestConcatTwoSegments(t *testing.T) {
	src := buildTwoTrackMP4(t)
	var movies []*Movie
	for i := 0; i < 2; i++ {
		m, err := Parse(bytes.NewReader(src), int64(len(src)))
		if err != nil {
			t.Fatalf("parse seg %d: %v", i, err)
		}
		movies = append(movies, m)
	}
	mg, err := Merge(movies)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	payloads := make([][]byte, 2)
	for i, m := range movies {
		payloads[i] = src[m.Mdat[0].Start:m.Mdat[0].End]
	}
	out, err := mg.EmitToBytes(payloads)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	outM, err := Parse(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("parse out: %v", err)
	}
	v, a := outM.Tracks[0], outM.Tracks[1]
	if v.SampleCount != 6 {
		t.Fatalf("video samples = %d, want 6", v.SampleCount)
	}
	if a.SampleCount != 4 {
		t.Fatalf("audio samples = %d, want 4", a.SampleCount)
	}
	if v.TotalDuration() != 180 {
		t.Fatalf("video dur = %d, want 180", v.TotalDuration())
	}
	if a.TotalDuration() != 4096 {
		t.Fatalf("audio dur = %d, want 4096", a.TotalDuration())
	}
	if outM.Mvhd.Duration != 180 || outM.Mvhd.Timescale != 30 {
		t.Fatalf("mvhd = %+v, want ts=30 dur=180", outM.Mvhd)
	}
	if len(v.Chunks) != 2 || len(a.Chunks) != 2 {
		t.Fatalf("chunks v/a = %d/%d, want 2/2", len(v.Chunks), len(a.Chunks))
	}
	if v.Chunks[0] != uint64(mg.MdatBase(0)) || v.Chunks[1] != uint64(mg.MdatBase(1)) {
		t.Fatalf("video chunk offs = %v, want [%d %d]", v.Chunks, mg.MdatBase(0), mg.MdatBase(1))
	}
	wantA0, wantA1 := uint64(mg.MdatBase(0))+36, uint64(mg.MdatBase(1))+36
	if a.Chunks[0] != wantA0 || a.Chunks[1] != wantA1 {
		t.Fatalf("audio chunk offs = %v, want [%d %d]", a.Chunks, wantA0, wantA1)
	}
	// stsz totals: 2*(10+12+14)=72 video, 2*(100+120)=440 audio.
	var vSum, aSum uint32
	for _, s := range v.SampleSizes {
		vSum += s
	}
	for _, s := range a.SampleSizes {
		aSum += s
	}
	if vSum != 72 || aSum != 440 {
		t.Fatalf("sample byte sums v/a = %d/%d, want 72/440", vSum, aSum)
	}
}

// TestConcatIncompatible rejects segments whose streams differ.
func TestConcatIncompatible(t *testing.T) {
	a := buildSegFile(320, 240, []uint32{10, 12, 14}, []uint32{100, 120})
	b := buildSegFile(640, 480, []uint32{10, 12, 14}, []uint32{100, 120}) // wider
	ma, err := Parse(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	mb, err := Parse(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("parse b: %v", err)
	}
	if _, err := Merge([]*Movie{ma, mb}); err == nil {
		t.Fatal("expected incompatibility error")
	}
}
