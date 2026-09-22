package mp4

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// realSpsNRef2/realSpsNRef4 are the SPSes Procreate wrote to the quiet and
// active segments of a 1088x1088 recording (High 4:2:0, level 3.2); realPPS
// is the PPS they share. They differ only in max_num_ref_frames.
var (
	realSpsNRef2 = []byte{0x27, 0x64, 0x00, 0x20, 0xac, 0x56, 0xc0, 0x44, 0x02, 0x26, 0x9a, 0x80, 0x80, 0x80, 0x81}
	realSpsNRef4 = []byte{0x27, 0x64, 0x00, 0x20, 0xac, 0x56, 0x50, 0x11, 0x00, 0x89, 0xa6, 0xa0, 0x20, 0x20, 0x20, 0x40}
	realPPS      = []byte{0x28, 0xee, 0x3c, 0xb0}
)

// ueBits encodes n as an unsigned Exp-Golomb code (H.264 9.1).
func ueBits(n uint32) string {
	v := n + 1
	l := 1
	for x := v >> 1; x != 0; x >>= 1 {
		l++
	}
	return strings.Repeat("0", l-1) + fmt.Sprintf("%b", v)
}

// baseSPSNRef builds the 320x240 baseline SPS with the given
// max_num_ref_frames.
func baseSPSNRef(nref uint32) []byte {
	bits := ""
	bits += "0" + "11" + "00111" // nal header
	bits += "01000010"           // profile_idc = 66
	bits += "00000000"           // constraint flags
	bits += "00011110"           // level_idc = 30
	bits += "1"                  // sps_id ue(0)
	bits += "1"                  // log2_max_frame_num_minus4 ue(0)
	bits += "1"                  // pic_order_cnt_type ue(0)
	bits += "1"                  // log2_max_pic_order_cnt_lsb_minus4 ue(0)
	bits += ueBits(nref)         // max_num_ref_frames
	bits += "0"                  // gaps_in_frame_num_value_allowed
	bits += "00000101011"        // pic_width_in_mbs_minus1 ue(19) => 320px
	bits += "000010110"          // pic_height_in_map_units_minus1 ue(14) => 240px
	bits += "1"                  // frame_mbs_only
	bits += "1"                  // direct_8x8_inference
	bits += "0"                  // frame_cropping
	bits += "0"                  // vui_parameters_present
	return bitsToBytes(bits)
}

// highSPSAt builds a High (100) profile SPS for a picture of (wMB+1)*16 by
// (hUnits+1)*16 pixels with the given max_num_ref_frames; withCrop appends
// frame_cropping with unit offsets.
func highSPSAt(nref, wMB, hUnits uint32, withCrop bool) []byte {
	bits := ""
	bits += "0" + "11" + "00111" // nal header
	bits += "01100100"           // profile_idc = 100 (high)
	bits += "00000000"           // constraint flags
	bits += "00100000"           // level_idc = 32
	bits += "1"                  // sps_id ue(0)
	bits += "110"                // chroma_format_idc ue(1)
	bits += "1"                  // bit_depth_luma_minus8 ue(0)
	bits += "1"                  // bit_depth_chroma_minus8 ue(0)
	bits += "0"                  // qpprime_y_zero_transform_bypass
	bits += "0"                  // seq_scaling_matrix_present
	bits += "1"                  // log2_max_frame_num_minus4 ue(0)
	bits += "1"                  // pic_order_cnt_type ue(0)
	bits += "1"                  // log2_max_pic_order_cnt_lsb_minus4 ue(0)
	bits += ueBits(nref)         // max_num_ref_frames
	bits += "0"                  // gaps_in_frame_num_value_allowed
	bits += ueBits(wMB)          // pic_width_in_mbs_minus1
	bits += ueBits(hUnits)       // pic_height_in_map_units_minus1
	bits += "1"                  // frame_mbs_only
	bits += "1"                  // direct_8x8_inference
	if withCrop {
		bits += "1" + "010" + "010" + "010" + "010" // crop=1, 4x ue(1)
	} else {
		bits += "0" // frame_cropping
	}
	bits += "0" // vui_parameters_present
	return bitsToBytes(bits)
}

// avcCPay builds a raw avcC payload (no box header) with the given profile
// byte and a single SPS/PPS pair.
func avcCPay(profile byte, sps, pps []byte) []byte {
	c := make([]byte, 0, 10+len(sps)+2+len(pps))
	c = append(c, 1, profile, 0x60, 32, 0xff, 0xe1)
	c = append(c, byte(len(sps)>>8), byte(len(sps)&0xff))
	c = append(c, sps...)
	c = append(c, 1)
	c = append(c, byte(len(pps)>>8), byte(len(pps)&0xff))
	c = append(c, pps...)
	return c
}

// withTrail copies pay and appends the given trailer bytes.
func withTrail(pay []byte, trail ...byte) []byte {
	c := append([]byte(nil), pay...)
	return append(c, trail...)
}

func TestAvcCMatchesExceptNRef(t *testing.T) {
	real2 := avcCPay(100, realSpsNRef2, realPPS)
	real4 := avcCPay(100, realSpsNRef4, realPPS)
	basePPS := []byte{0x27, 0x05, 0xeb}
	level31 := append([]byte(nil), realSpsNRef4...)
	level31[3] = 31
	twoSps := func() []byte {
		c := []byte{1, 100, 0x60, 32, 0xff, 0xe2}
		c = append(c, 0, byte(len(realSpsNRef2)))
		c = append(c, realSpsNRef2...)
		c = append(c, 0, byte(len(realSpsNRef2)))
		c = append(c, realSpsNRef2...)
		c = append(c, 1, 0, byte(len(realPPS)))
		c = append(c, realPPS...)
		return c
	}()
	cases := []struct {
		name string
		a, b []byte
		want bool
	}{
		{"identical", real2, real2, true},
		{"corpus pair 2 vs 4", real2, real4, true},
		{"corpus pair 4 vs 2", real4, real2, true},
		{"baseline nref drift", avcCPay(66, baseSPSNRef(2), basePPS), avcCPay(66, baseSPSNRef(4), basePPS), true},
		{"pps differs", avcCPay(66, baseSPSNRef(2), basePPS), avcCPay(66, baseSPSNRef(2), []byte{0x27, 0x05, 0xec}), false},
		{"profile header differs", avcCPay(66, baseSPSNRef(2), basePPS), avcCPay(100, highSPSAt(2, 19, 14, false), realPPS), false},
		{"sps level differs", real2, avcCPay(100, level31, realPPS), false},
		{"width differs", avcCPay(100, highSPSAt(2, 19, 14, false), realPPS), avcCPay(100, highSPSAt(4, 20, 14, false), realPPS), false},
		{"crop differs", avcCPay(100, highSPSAt(2, 19, 14, false), realPPS), avcCPay(100, highSPSAt(2, 19, 14, true), realPPS), false},
		{"sps count differs", real2, twoSps, false},
		{"identical trailer", withTrail(real2, 0xfd, 0xf8, 0xf8, 0x00), withTrail(real2, 0xfd, 0xf8, 0xf8, 0x00), true},
		{"trailer differs", withTrail(real2, 0xfd, 0xf8, 0xf8, 0x00), withTrail(real2, 0xfe, 0xf8, 0xf8, 0x00), false},
		{"truncated header", real2[:5], real4, false},
		{"undecodable sps", avcCPay(100, realSpsNRef2[:4], realPPS), real2, false},
		{"garbage", []byte{1, 2, 3}, real2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := avcCMatchesExceptNRef(tc.a, tc.b); got != tc.want {
				t.Fatalf("avcCMatchesExceptNRef = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSpsUpToNRefRealPair(t *testing.T) {
	fa, ok := spsUpToNRef(realSpsNRef2)
	if !ok {
		t.Fatal("spsUpToNRef(nref=2 SPS) failed")
	}
	fb, ok := spsUpToNRef(realSpsNRef4)
	if !ok {
		t.Fatal("spsUpToNRef(nref=4 SPS) failed")
	}
	if fa.nref != 2 || fb.nref != 4 {
		t.Fatalf("nref = %d/%d, want 2/4", fa.nref, fb.nref)
	}
	if fa.profile != 100 || fb.profile != 100 || fa.level != 32 || fb.level != 32 {
		t.Fatalf("profile/level = %d/%d and %d/%d, want 100/32", fa.profile, fa.level, fb.profile, fb.level)
	}
	if fa.chromaFormat != 1 || fb.chromaFormat != 1 {
		t.Fatalf("chroma = %d/%d, want 1/1", fa.chromaFormat, fb.chromaFormat)
	}
	if !fa.equalExceptNRef(fb) {
		t.Error("prefix fields differ besides max_num_ref_frames")
	}
	if spsTailBits(realSpsNRef2, fa.endBit) != spsTailBits(realSpsNRef4, fb.endBit) {
		t.Error("post-nref tails differ")
	}
}

func TestAvcCMaxNRef(t *testing.T) {
	if n, ok := avcCMaxNRef(avcCPay(100, realSpsNRef2, realPPS)); !ok || n != 2 {
		t.Errorf("avcCMaxNRef(nref=2) = %d, %v; want 2, true", n, ok)
	}
	if n, ok := avcCMaxNRef(avcCPay(100, realSpsNRef4, realPPS)); !ok || n != 4 {
		t.Errorf("avcCMaxNRef(nref=4) = %d, %v; want 4, true", n, ok)
	}
	if _, ok := avcCMaxNRef([]byte{1, 2}); ok {
		t.Error("avcCMaxNRef on a truncated payload reported ok")
	}
}

func TestConfigDataCompatDispatch(t *testing.T) {
	real2 := avcCPay(100, realSpsNRef2, realPPS)
	real4 := avcCPay(100, realSpsNRef4, realPPS)
	if !configDataCompat("esds", []byte{1, 2}, "esds", []byte{1, 2}) {
		t.Error("identical esds payloads must be compatible")
	}
	if configDataCompat("esds", []byte{1, 2}, "esds", []byte{1, 3}) {
		t.Error("differing esds payloads must not be compatible")
	}
	if configDataCompat("avcC", real2, "hvcC", real2) {
		t.Error("differing config boxes must not be compatible")
	}
	if !configDataCompat("avcC", real2, "avcC", real4) {
		t.Error("nref-drifting avcC payloads must be compatible")
	}
}

// nrefSegFile builds a video-only 320x240 segment whose SPS carries
// max_num_ref_frames = nref.
func nrefSegFile(t *testing.T, nref uint32) []byte {
	t.Helper()
	sizes := []uint32{10, 12}
	sps, pps := baseSPSNRef(nref), []byte{0x27, 0x05, 0xeb}
	count := uint32(len(sizes))
	vDur := uint64(count) * 30
	total := 0
	for _, s := range sizes {
		total += int(s)
	}
	buildMoov := func(off uint64) []byte {
		kids := [][]byte{
			stsdVideo(320, 240, sps, pps),
			EncSTTS([]SttsEntry{{Count: count, Delta: 30}}),
			EncSTSC([]StscEntry{{FirstChunk: 1, SamplesPerChunk: count, Description: 1}}),
			EncSTSZ(sizes),
			EncSTCO([]uint64{off}, false),
		}
		stbl := Container("stbl", kids...)
		trak := Container("trak",
			EncTKHD(1, vDur, 320, 240, 0x0100, identityMatrixVals),
			Container("mdia",
				EncMDHD(30, vDur, 0),
				EncHDLR("vide", "VideoHandler"),
				Container("minf", EncVMHD(), EncDINF(), stbl),
			),
		)
		return Container("moov", EncMVHD(30, vDur), trak)
	}
	ftyp := EncFTYP()
	moov0 := buildMoov(0)
	mdatBase := int64(len(ftyp)) + int64(len(moov0)) + 8
	moov := buildMoov(uint64(mdatBase))
	out := make([]byte, 0, len(ftyp)+len(moov)+8+total)
	out = append(out, ftyp...)
	out = append(out, moov...)
	out = append(out, NewBox("mdat", bytes.Repeat([]byte{0xcd}, total))...)
	return out
}

func TestMergeNRefDriftTakesMaxNRefStsd(t *testing.T) {
	seg2 := nrefSegFile(t, 2)
	seg4 := nrefSegFile(t, 4)
	m2, _ := parseSeg(t, seg2)
	m4, _ := parseSeg(t, seg4)
	if bytes.Equal(m2.Tracks[0].ConfigData, m4.Tracks[0].ConfigData) {
		t.Fatal("test fixture bug: avcC payloads are identical")
	}
	mg, err := Merge([]*Movie{m2, m4})
	if err != nil {
		t.Fatalf("Merge with an nref-drifting pair = %v, want success", err)
	}
	if !bytes.Equal(mg.tracks[0].stsd, m4.Tracks[0].StsdRaw) {
		t.Error("merged stsd is not the max-nref (nref=4) variant")
	}
	payloads := [][]byte{
		seg2[m2.Mdat[0].Start:m2.Mdat[0].End],
		seg4[m4.Mdat[0].Start:m4.Mdat[0].End],
	}
	out, err := mg.EmitToBytes(payloads)
	if err != nil {
		t.Fatalf("EmitToBytes: %v", err)
	}
	rm, err := Parse(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("resulting MP4 is invalid: %v", err)
	}
	if !bytes.Equal(rm.Tracks[0].ConfigData, m4.Tracks[0].ConfigData) {
		t.Error("emitted avcC is not the max-nref variant")
	}
}

func TestMergeIdenticalConfigsKeepFirstStsd(t *testing.T) {
	seg := nrefSegFile(t, 2)
	m1, _ := parseSeg(t, seg)
	m2, _ := parseSeg(t, seg)
	mg, err := Merge([]*Movie{m1, m2})
	if err != nil {
		t.Fatalf("Merge with identical configs: %v", err)
	}
	if !bytes.Equal(mg.tracks[0].stsd, m1.Tracks[0].StsdRaw) {
		t.Error("merged stsd changed although all configs are identical")
	}
}
