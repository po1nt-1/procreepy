package mp4

import (
	"bytes"
	"strings"
)

// Procreate writes two variants of the same H.264 stream into one recording:
// the SPS carried by quiet segments' avcC boxes declares
// max_num_ref_frames = 2, while active segments declare 4. Everything else —
// profile, level, dimensions, chroma format, the PPS — is identical. The
// field only hints at decoder picture-buffer capacity (the PPS keeps the
// default active reference count at 1), so segments differing in it are
// stream-copy compatible; the merged output carries the larger variant.

// configDataCompat reports whether two track configurations are compatible
// for a stream-copy concat: byte-identical, or — for avcC — identical in
// every decode-relevant aspect except SPS max_num_ref_frames. Input it cannot
// fully parse is reported as non-equivalent, leaving the byte comparison as
// the fallback.
func configDataCompat(aBox string, a []byte, bBox string, b []byte) bool {
	if aBox != bBox {
		return false
	}
	if aBox != "avcC" {
		return bytes.Equal(a, b)
	}
	return avcCMatchesExceptNRef(a, b)
}

// avcCSections splits an avcC payload into its fixed 6-byte header, the SPS
// byte strings, the PPS byte strings, and any trailing bytes (Procreate pads
// the box with four of them). The walk must stay in bounds; trailing bytes
// are kept, not rejected.
func avcCSections(pay []byte) (hdr, tail []byte, sps, pps [][]byte, ok bool) {
	if len(pay) < 6 {
		return nil, nil, nil, nil, false
	}
	nSPS := int(pay[5] & 0x1f)
	if nSPS == 0 {
		return nil, nil, nil, nil, false
	}
	off := 6
	for k := 0; k < nSPS; k++ {
		if off+2 > len(pay) {
			return nil, nil, nil, nil, false
		}
		l := int(u16be(pay[off : off+2]))
		off += 2
		if l <= 0 || off+l > len(pay) {
			return nil, nil, nil, nil, false
		}
		sps = append(sps, pay[off:off+l])
		off += l
	}
	if off+1 > len(pay) {
		return nil, nil, nil, nil, false
	}
	nPPS := int(pay[off] & 0xff)
	off++
	for k := 0; k < nPPS; k++ {
		if off+2 > len(pay) {
			return nil, nil, nil, nil, false
		}
		l := int(u16be(pay[off : off+2]))
		off += 2
		if l <= 0 || off+l > len(pay) {
			return nil, nil, nil, nil, false
		}
		pps = append(pps, pay[off:off+l])
		off += l
	}
	return pay[0:6], pay[off:], sps, pps, true
}

// spsNRefFields are the SPS 7.3.2.1 fields up to and including
// max_num_ref_frames; endBit is the bit position right after that field.
type spsNRefFields struct {
	id             uint32
	profile        uint8
	level          uint8
	chromaFormat   uint32 // assumed 1 where the profile omits the field
	separatePlane  bool
	bitDepthLuma   uint32
	bitDepthChroma uint32
	qpprimeYZero   bool
	scalingPresent bool
	lfnM4          uint32
	pocType        uint32
	lmcM4          uint32 // poc type 0
	deltaBottom    uint32 // poc type 1
	offNonRef      int32
	offTopBottom   int32
	cycleLen       uint32
	cycleOffsets   []int32
	nref           uint32
	endBit         int
}

func (f spsNRefFields) equalExceptNRef(o spsNRefFields) bool {
	if f.id != o.id || f.profile != o.profile || f.level != o.level ||
		f.chromaFormat != o.chromaFormat || f.separatePlane != o.separatePlane ||
		f.bitDepthLuma != o.bitDepthLuma || f.bitDepthChroma != o.bitDepthChroma ||
		f.qpprimeYZero != o.qpprimeYZero || f.lfnM4 != o.lfnM4 ||
		f.pocType != o.pocType || f.lmcM4 != o.lmcM4 ||
		f.deltaBottom != o.deltaBottom || f.offNonRef != o.offNonRef ||
		f.offTopBottom != o.offTopBottom || f.cycleLen != o.cycleLen {
		return false
	}
	if len(f.cycleOffsets) != len(o.cycleOffsets) {
		return false
	}
	for i := range f.cycleOffsets {
		if f.cycleOffsets[i] != o.cycleOffsets[i] {
			return false
		}
	}
	return true
}

// spsUpToNRef decodes an H.264 SPS byte string (no start code) up to and
// including max_num_ref_frames.
func spsUpToNRef(b []byte) (spsNRefFields, bool) {
	var f spsNRefFields
	if len(b) < 4 {
		return f, false
	}
	g := &expGolomb{bits: b}
	// NAL header: forbidden_zero_bit(1) nal_ref_idc(2) nal_unit_type(5).
	f.profile = b[1]
	f.level = b[3]
	g.pos = 32
	var err error
	if f.id, err = g.ue(); err != nil {
		return f, false
	}
	if spsNeedsChromaFormat(f.profile) {
		if f.chromaFormat, err = g.ue(); err != nil {
			return f, false
		}
		if f.chromaFormat == 0 || f.chromaFormat > 3 {
			return f, false
		}
		if f.chromaFormat == 3 {
			v, err := g.u(1)
			if err != nil {
				return f, false
			}
			f.separatePlane = v == 1
		}
		if f.bitDepthLuma, err = g.ue(); err != nil {
			return f, false
		}
		if f.bitDepthChroma, err = g.ue(); err != nil {
			return f, false
		}
		v, err := g.u(1)
		if err != nil {
			return f, false
		}
		f.qpprimeYZero = v == 1
		v, err = g.u(1)
		if err != nil {
			return f, false
		}
		f.scalingPresent = v == 1
	} else {
		f.chromaFormat = 1
	}
	if f.lfnM4, err = g.ue(); err != nil {
		return f, false
	}
	if f.pocType, err = g.ue(); err != nil {
		return f, false
	}
	switch f.pocType {
	case 0:
		if f.lmcM4, err = g.ue(); err != nil {
			return f, false
		}
	case 1:
		if f.deltaBottom, err = g.ue(); err != nil {
			return f, false
		}
		if f.offNonRef, err = g.se(); err != nil {
			return f, false
		}
		if f.offTopBottom, err = g.se(); err != nil {
			return f, false
		}
		if f.cycleLen, err = g.ue(); err != nil {
			return f, false
		}
		f.cycleOffsets = make([]int32, f.cycleLen)
		for i := range f.cycleOffsets {
			if f.cycleOffsets[i], err = g.se(); err != nil {
				return f, false
			}
		}
	case 2:
	default:
		return f, false
	}
	if f.nref, err = g.ue(); err != nil {
		return f, false
	}
	f.endBit = g.pos
	return f, true
}

// spsTailBits is the raw bit string of b from bit pos to the end of the
// buffer, with zero padding at the byte boundary stripped. SPS prefixes that
// agree through max_num_ref_frames decode to the same later fields iff their
// tails agree.
func spsTailBits(b []byte, pos int) string {
	total := len(b) * 8
	if pos < 0 || pos > total {
		return ""
	}
	var sb strings.Builder
	sb.Grow(total - pos)
	for i := pos; i < total; i++ {
		if (b[i/8]>>(7-i%8))&1 == 1 {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
	}
	return strings.TrimRight(sb.String(), "0")
}

// spsNRefEquivalent reports whether two SPS byte strings differ only in
// max_num_ref_frames. Scaling-list SPSes are rejected: the matrices sit
// before the compared fields and their absence/shape changes the field
// alignment, so equivalence cannot be judged without decoding them.
func spsNRefEquivalent(a, b []byte) bool {
	fa, oka := spsUpToNRef(a)
	fb, okb := spsUpToNRef(b)
	if !oka || !okb {
		return false
	}
	if fa.scalingPresent || fb.scalingPresent {
		return false
	}
	if !fa.equalExceptNRef(fb) {
		return false
	}
	return spsTailBits(a, fa.endBit) == spsTailBits(b, fb.endBit)
}

// avcCMatchesExceptNRef reports whether two avcC payloads carry identical
// H.264 configuration except possibly for SPS max_num_ref_frames.
func avcCMatchesExceptNRef(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	ha, ta, sa, pa, oka := avcCSections(a)
	hb, tb, sb, pb, okb := avcCSections(b)
	if !oka || !okb {
		return false
	}
	if !bytes.Equal(ha, hb) || !bytes.Equal(ta, tb) ||
		len(sa) != len(sb) || len(pa) != len(pb) {
		return false
	}
	for i := range sa {
		if !spsNRefEquivalent(sa[i], sb[i]) {
			return false
		}
	}
	for i := range pa {
		if !bytes.Equal(pa[i], pb[i]) {
			return false
		}
	}
	return true
}

// avcCMaxNRef returns the max_num_ref_frames declared by the first SPS of an
// avcC payload.
func avcCMaxNRef(pay []byte) (int, bool) {
	_, _, sps, _, ok := avcCSections(pay)
	if !ok {
		return 0, false
	}
	f, ok := spsUpToNRef(sps[0])
	if !ok {
		return 0, false
	}
	return int(f.nref), true
}

// pickStsd chooses the stsd bytes for a merged track. Among avcC
// configurations the one with the largest SPS max_num_ref_frames wins: the
// merged stream's DPB requirement is the maximum over its segments. When no
// configuration parses (or the track carries no avcC) the first segment's
// stsd is used unchanged.
func pickStsd(tracks []*TrackState) []byte {
	best := tracks[0].StsdRaw
	bestNref := -1
	for _, tr := range tracks {
		if tr.ConfigBox != "avcC" {
			continue
		}
		n, ok := avcCMaxNRef(tr.ConfigData)
		if !ok || n <= bestNref {
			continue
		}
		best, bestNref = tr.StsdRaw, n
	}
	return best
}
