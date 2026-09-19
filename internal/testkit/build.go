// Package testkit builds synthetic MP4 segments and .procreate archives for
// tests.
package testkit

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
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

// Segment assembles a valid progressive two-track MP4: ftyp + moov + mdat.
// The video track is baseline H.264 at w x h (timescale 30, 30 ticks/sample);
// the audio track is AAC-LC 44100 Hz stereo (timescale 44100, 1024 ticks/
// sample). Each track has one chunk. The mdat payload bytes vary with
// position, so offset or length mistakes are visible in comparisons.
func Segment(w, h uint16, videoSizes, audioSizes []uint32) []byte {
	sps := baselineSPS()
	pps := []byte{0x27, 0x05, 0xeb}
	vCount := uint32(len(videoSizes))
	aCount := uint32(len(audioSizes))
	vDur := uint64(vCount) * 30
	aDur := uint64(aCount) * 1024

	stsdV := mp4.FullBox("stsd", 0, 0, append([]byte{0, 0, 0, 1},
		mp4.NewBox("avc1", videoEntry(w, h, avcC(sps, pps)))...))
	stsdA := mp4.FullBox("stsd", 0, 0, append([]byte{0, 0, 0, 1},
		mp4.NewBox("mp4a", audioEntry(2, 44100, esdsAAC(4, 2)))...))

	buildMoov := func(videoOff, audioOff uint64) []byte {
		vStbl := mp4.Container("stbl",
			stsdV,
			mp4.EncSTTS([]mp4.SttsEntry{{Count: vCount, Delta: 30}}),
			mp4.EncSTSC([]mp4.StscEntry{{FirstChunk: 1, SamplesPerChunk: vCount, Description: 1}}),
			mp4.EncSTSZ(videoSizes),
			mp4.EncSTCO([]uint64{videoOff}, false),
		)
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
		return mp4.Container("moov", mp4.EncMVHD(30, vDur), vTrak, aTrak)
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
