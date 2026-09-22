// Package testkit builds synthetic MP4 segments and .procreate archives for
// tests.
package testkit

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"procreepy/internal/mp4"
)

var identityMatrix = [9]int64{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000}

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

// BaselineSPS is the SPS used by default in synthetic segments.
func BaselineSPS() []byte { return baselineSPS() }

// defaultPPS is the PPS used by default in synthetic segments.
var defaultPPS = []byte{0x27, 0x05, 0xeb}

// DefaultPPS is the PPS used by default in synthetic segments.
func DefaultPPS() []byte { return defaultPPS }

// BaselineSPSWithNRef returns the default baseline SPS with
// max_num_ref_frames = nref. Mirrors Procreate, which writes the same SPS
// with a drifting max_num_ref_frames into the segments of one recording.
func BaselineSPSWithNRef(nref uint32) []byte {
	bits := ""
	bits += "0" + "11" + "00111" // forbidden, nal_ref_idc=3, nal_unit_type=7 (SPS)
	bits += "01000010"           // profile_idc = 66 (baseline)
	bits += "00000000"           // constraint flags
	bits += "00011110"           // level_idc = 30
	bits += "1"                  // sps_id ue(0)
	bits += "1"                  // log2_max_frame_num_minus4 ue(0)
	bits += "1"                  // pic_order_cnt_type ue(0)
	bits += "1"                  // log2_max_pic_order_cnt_lsb_minus4 ue(0)
	bits += encUE(nref)          // max_num_ref_frames
	bits += "0"                  // gaps_in_frame_num_value_allowed
	bits += "00000101011"        // pic_width_in_mbs_minus1 ue(19) => 20 mbs = 320px
	bits += "000010110"          // pic_height_in_map_units_minus1 ue(14) => 15 = 240px
	bits += "1"                  // frame_mbs_only
	bits += "1"                  // direct_8x8_inference
	bits += "0"                  // frame_cropping
	bits += "0"                  // vui_parameters_present
	return bitsToBytes(bits)
}

// baselineSPS builds a minimal valid H.264 baseline SPS (320x240).
func baselineSPS() []byte { return BaselineSPSWithNRef(2) }

// encUE encodes n as an unsigned Exp-Golomb code (ITU-T H.264 9.1).
func encUE(n uint32) string {
	v := n + 1
	l := 1
	for x := v >> 1; x != 0; x >>= 1 {
		l++
	}
	return strings.Repeat("0", l-1) + fmt.Sprintf("%b", v)
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
	return mp4.NewBox("avcC", c)
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
	b[32] = 0
	b[33] = 0x48
	b[40] = 0 // frame_count
	b[41] = 1
	b[74] = 0
	b[75] = 0x18 // depth = 24
	b[76] = 0xff
	b[77] = 0xff
	return append(b, cfg...)
}

// audioEntry builds an mp4a sample entry (after size+fourcc).
func audioEntry(chans uint16, rate uint32, cfg []byte) []byte {
	b := make([]byte, 28)
	b[7] = 1                 // data_reference_index
	b[16] = byte(chans >> 8) // channelcount
	b[17] = byte(chans & 0xff)
	b[19] = 16 // samplesize
	// samplerate, 16.16 fixed point
	binary.BigEndian.PutUint32(b[24:28], uint32(rate)<<16)
	return append(b, cfg...)
}

// esdsAAC builds an esds box carrying an AAC AudioSpecificConfig.
func esdsAAC(freqIdx byte, chans uint16) []byte {
	asc := []byte{(0x02 << 3) | freqIdx, byte(chans) << 4} // aot=2 (AAC-LC)
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
	var out []byte
	out = append(out, 0x03) // ES_Descriptor
	out = append(out, byte(len(es)))
	out = append(out, es...)
	return mp4.FullBox("esds", 0, 0, out)
}

// segConf holds the tunable fields of a synthetic segment.
type segConf struct {
	sps, pps []byte
	mvTS     uint32
	stss     []uint32
}

// SegOpt customizes one synthetic segment. Pass different opts to the
// segments of a pair to make them differ in exactly one aspect.
type SegOpt func(*segConf)

// WithCodecConfig replaces the H.264 SPS/PPS carried in avcC. Pass the same
// SPS with a changed PPS to diverge only the codec configuration bytes; a
// changed SPS may also move the derived pixel format.
func WithCodecConfig(sps, pps []byte) SegOpt {
	return func(c *segConf) { c.sps, c.pps = sps, pps }
}

// WithMovieTimescale overrides the mvhd timescale.
func WithMovieTimescale(ts uint32) SegOpt {
	return func(c *segConf) { c.mvTS = ts }
}

// WithStss emits a sync sample table with the given 1-based sample numbers.
// Without the option the segment has no stss box, like most Procreate
// segments do.
func WithStss(nums ...uint32) SegOpt {
	return func(c *segConf) { c.stss = nums }
}

// Segment assembles a valid progressive two-track MP4: ftyp + moov + mdat.
// The video track is baseline H.264 at w x h (timescale 30, 30 ticks/sample);
// the audio track is AAC-LC 44100 Hz stereo (timescale 44100, 1024 ticks/
// sample). Each track has one chunk. The mdat payload bytes vary with
// position, so offset or length mistakes are visible in comparisons.
// opts override individual fields (sync table, codec config, movie timescale).
func Segment(w, h uint16, videoSizes, audioSizes []uint32, opts ...SegOpt) []byte {
	c := &segConf{sps: baselineSPS(), pps: defaultPPS, mvTS: 30}
	for _, o := range opts {
		o(c)
	}
	vCount := uint32(len(videoSizes))
	aCount := uint32(len(audioSizes))
	vDur := uint64(vCount) * 30
	aDur := uint64(aCount) * 1024

	stsdV := mp4.FullBox("stsd", 0, 0, append([]byte{0, 0, 0, 1},
		mp4.NewBox("avc1", videoEntry(w, h, avcC(c.sps, c.pps)))...))
	stsdA := mp4.FullBox("stsd", 0, 0, append([]byte{0, 0, 0, 1},
		mp4.NewBox("mp4a", audioEntry(2, 44100, esdsAAC(4, 2)))...))

	buildMoov := func(videoOff, audioOff uint64) []byte {
		vStblKids := [][]byte{
			stsdV,
			mp4.EncSTTS([]mp4.SttsEntry{{Count: vCount, Delta: 30}}),
			mp4.EncSTSC([]mp4.StscEntry{{FirstChunk: 1, SamplesPerChunk: vCount, Description: 1}}),
			mp4.EncSTSZ(videoSizes),
			mp4.EncSTCO([]uint64{videoOff}, false),
		}
		if c.stss != nil {
			vStblKids = append(vStblKids, mp4.EncSTSS(c.stss))
		}
		vStbl := mp4.Container("stbl", vStblKids...)
		vTrak := mp4.Container("trak",
			mp4.EncTKHD(1, vDur, w, h, 0x0100, identityMatrix),
			mp4.Container("mdia",
				mp4.EncMDHD(30, vDur, 0),
				mp4.EncHDLR("vide", "VideoHandler"),
				mp4.Container("minf", mp4.EncVMHD(), mp4.EncDINF(), vStbl),
			),
		)
		aStbl := mp4.Container("stbl",
			stsdA,
			mp4.EncSTTS([]mp4.SttsEntry{{Count: aCount, Delta: 1024}}),
			mp4.EncSTSC([]mp4.StscEntry{{FirstChunk: 1, SamplesPerChunk: aCount, Description: 1}}),
			mp4.EncSTSZ(audioSizes),
			mp4.EncSTCO([]uint64{audioOff}, false),
		)
		aTrak := mp4.Container("trak",
			mp4.EncTKHD(2, aDur, 0, 0, 0x0100, identityMatrix),
			mp4.Container("mdia",
				mp4.EncMDHD(44100, aDur, 0),
				mp4.EncHDLR("soun", "SoundHandler"),
				mp4.Container("minf", mp4.EncSMHD(), mp4.EncDINF(), aStbl),
			),
		)
		return mp4.Container("moov", mp4.EncMVHD(c.mvTS, vDur), vTrak, aTrak)
	}

	var vTotal, aTotal int
	for _, s := range videoSizes {
		vTotal += int(s)
	}
	for _, s := range audioSizes {
		aTotal += int(s)
	}
	data := make([]byte, vTotal+aTotal)
	for i := range data {
		data[i] = byte((i * 31) % 251)
	}
	ftyp := mp4.EncFTYP()
	moov0 := buildMoov(0, 0)
	mdatBase := int64(len(ftyp)) + int64(len(moov0)) + 8 // +8 mdat header
	moov := buildMoov(uint64(mdatBase), uint64(mdatBase)+uint64(vTotal))
	mdat := mp4.NewBox("mdat", data)

	file := make([]byte, 0, len(ftyp)+len(moov)+len(mdat))
	file = append(file, ftyp...)
	file = append(file, moov...)
	file = append(file, mdat...)
	return file
}

// ArchiveBytes returns the bytes of a ZIP archive holding the given
// entries. With compress, entries are DEFLATED; otherwise stored verbatim
// (as Procreate writes them).
func ArchiveBytes(entries map[string][]byte, compress bool) []byte {
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	method := zip.Store
	if compress {
		method = zip.Deflate
	}
	for _, n := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     n,
			Method:   method,
			Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC), // DOS epoch
		})
		if err != nil {
			panic(err)
		}
		if _, err := w.Write(entries[n]); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// WriteArchive writes such an archive to a fresh file and returns its path.
func WriteArchive(t *testing.T, entries map[string][]byte, compress bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.procreate")
	if err := os.WriteFile(p, ArchiveBytes(entries, compress), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	return p
}

// WriteSegmentArchive writes n identical timelapse segments
// (video/segments/segment-NNNN.mp4, 1-based) to a fresh ZIP, generating and
// streaming each segment into the archive as it is written — no whole-archive
// buffer is kept. It returns the file path and the total uncompressed
// media (mdat payload) size.
func WriteSegmentArchive(t *testing.T, n int, compress bool, videoSizes, audioSizes []uint32) (string, int64) {
	return WriteSegmentArchiveOpts(t, n, compress, videoSizes, audioSizes, nil)
}

// WriteSegmentArchiveOpts is like WriteSegmentArchive, but applies perSeg
// (indexed by segment number minus one, nil entries mean defaults) to make
// individual segments differ.
func WriteSegmentArchiveOpts(t *testing.T, n int, compress bool, videoSizes, audioSizes []uint32,
	perSeg [][]SegOpt) (string, int64) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "segments.procreate")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	fail := func(err error) {
		f.Close()
		t.Fatalf("write archive: %v", err)
	}
	zw := zip.NewWriter(f)
	method := zip.Store
	if compress {
		method = zip.Deflate
	}
	var media int64
	for i := 1; i <= n; i++ {
		var o []SegOpt
		if perSeg != nil && i <= len(perSeg) {
			o = perSeg[i-1]
		}
		seg := Segment(320, 240, videoSizes, audioSizes, o...)
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     fmt.Sprintf("video/segments/segment-%04d.mp4", i),
			Method:   method,
			Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC), // DOS epoch
		})
		if err != nil {
			fail(err)
		}
		if _, err := w.Write(seg); err != nil {
			fail(err)
		}
		var m int
		for _, s := range videoSizes {
			m += int(s)
		}
		for _, s := range audioSizes {
			m += int(s)
		}
		media += int64(m)
	}
	if err := zw.Close(); err != nil {
		fail(err)
	}
	if err := f.Close(); err != nil {
		fail(err)
	}
	return p, media
}
