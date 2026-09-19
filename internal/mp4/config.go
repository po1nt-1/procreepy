package mp4

import (
	"fmt"
)

// expGolomb decodes unsigned and signed exp-Golomb values from a bit stream.
type expGolomb struct {
	bits []byte
	pos  int // bit position
}

func (g *expGolomb) bit() (int, error) {
	if g.pos/8 >= len(g.bits) {
		return 0, ErrTruncated
	}
	v := (g.bits[g.pos/8] >> (7 - g.pos%8)) & 1
	g.pos++
	return int(v), nil
}

func (g *expGolomb) u(n int) (uint32, error) {
	var v uint32
	for i := 0; i < n; i++ {
		b, err := g.bit()
		if err != nil {
			return 0, err
		}
		v = v<<1 | uint32(b)
	}
	return v, nil
}

func (g *expGolomb) ue() (uint32, error) {
	zeros := 0
	for {
		b, err := g.bit()
		if err != nil {
			return 0, err
		}
		if b == 1 {
			break
		}
		zeros++
		if zeros > 31 {
			return 0, ErrTruncated
		}
	}
	if zeros == 0 {
		return 0, nil
	}
	rest, err := g.u(zeros)
	if err != nil {
		return 0, err
	}
	return (1 << uint(zeros)) - 1 + rest, nil
}

// parseESDS extracts the audio specific config from an esds box payload.
func parseESDS(pay []byte) (aot uint8, freq uint32, chans uint8, ok bool) {
	if len(pay) < 4 {
		return 0, 0, 0, false
	}
	asc := esdsFindDecoderASC(pay, 4, len(pay))
	if len(asc) < 2 {
		return 0, 0, 0, false
	}
	// AudioSpecificConfig is raw bits: aot(5) freqIdx(4) channelConfig(4) ...
	aot = uint8(asc[0] >> 3)
	si := uint32(asc[0] & 0x0f)
	if si >= uint32(len(esdsFreqs)) {
		return 0, 0, 0, false
	}
	freq = esdsFreqs[si]
	chans = uint8(asc[1] >> 4)
	if aot == 0 {
		return 0, 0, 0, false
	}
	return aot, freq, chans, true
}

// esdsDescLength reads a variable-length descriptor length starting at pay[i].
// It returns the decoded length and the number of bytes consumed; n < 0 on
// malformed input.
func esdsDescLength(pay []byte, i int) (length, n int) {
	l := 0
	for n = 0; n < 4 && i+n < len(pay); n++ {
		c := pay[i+n]
		l = l<<7 | int(c&0x7f)
		if c&0x80 == 0 {
			return l, n + 1
		}
	}
	if n == 4 {
		return l, 4
	}
	return -1, -1
}

// esdsFindDecoderASC walks the ES descriptor tree and returns the audio
// specific config of the first DecoderConfigDescriptor (tag 0x04).
func esdsFindDecoderASC(pay []byte, start, end int) []byte {
	i := start
	for i < end {
		if i+2 > end {
			return nil
		}
		tag := pay[i]
		length, n := esdsDescLength(pay, i+1)
		if n < 0 || length < 0 {
			return nil
		}
		bodyStart := i + 1 + n
		bodyEnd := bodyStart + length
		if bodyEnd > end {
			return nil
		}
		i = bodyEnd
		switch tag {
		case 0x03: // ES_Descriptor: descend
			if found := esdsFindDecoderASC(pay, bodyStart, bodyEnd); found != nil {
				return found
			}
		case 0x04: // DecoderConfigDescriptor
			if bodyEnd-bodyStart >= 14 { // 13-byte header + at least 1 ASC byte
				return pay[bodyStart+13 : bodyEnd]
			}
			return nil
		}
	}
	return nil
}

var esdsFreqs = []uint32{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 11025, 8000, 7350}

// parseAVCConfig returns profile_idc and level_idc from an avcC payload.
func parseAVCConfig(pay []byte) (profile, level uint8, ok bool) {
	if len(pay) < 6 {
		return 0, 0, false
	}
	profile = pay[1]
	level = pay[3]
	nSPS := int(pay[5] & 0x1f)
	if nSPS == 0 {
		return profile, level, true
	}
	off := 6
	for k := 0; k < nSPS; k++ {
		if off+2 > len(pay) {
			return profile, level, true
		}
		l := int(u16be(pay[off : off+2]))
		off += 2
		if l <= 0 || off+l > len(pay) {
			return profile, level, true
		}
		off += l
	}
	if off+1 > len(pay) {
		return profile, level, true
	}
	nPPS := int(pay[off] & 0xff)
	off += 1
	for k := 0; k < nPPS; k++ {
		if off+2 > len(pay) {
			return profile, level, true
		}
		l := int(u16be(pay[off : off+2]))
		off += 2
		if l <= 0 || off+l > len(pay) {
			return profile, level, true
		}
		off += l
	}
	return profile, level, true
}

// h264SPS extracts profile/level/chroma format from an Annex-B SPS.
func h264SPS(b []byte) (profile, level uint8, chroma int, ok bool) {
	// strip the start code
	for len(b) >= 3 && b[0] == 0 && b[1] == 0 {
		if b[2] == 1 {
			b = b[3:]
			break
		}
		if b[2] == 0 {
			b = b[2:]
			continue
		}
		break
	}
	if len(b) < 4 {
		return 0, 0, -1, false
	}
	profile = b[1]
	level = b[3]
	g := &expGolomb{bits: b[4:]}
	// sps_id
	if _, err := g.ue(); err != nil {
		return 0, 0, -1, false
	}
	needChroma := profile == 100 || profile == 110 || profile == 122 || profile == 244 ||
		profile == 44 || profile == 83 || profile == 86 || profile == 118 ||
		profile == 128 || profile == 138 || profile == 139 || profile == 134 || profile == 135
	if !needChroma {
		return profile, level, 1, true // baseline/main: yuv420p or gray; assume 4:2:0
	}
	v, err := g.ue()
	if err != nil {
		return 0, 0, -1, false
	}
	chroma = int(v)
	return profile, level, chroma, true
}

// spsChromaFromConfig walks an avcC looking for the first SPS.
func spsChromaFromConfig(pay []byte) int {
	_, _, ok := parseAVCConfig(pay)
	if !ok || len(pay) < 6 {
		return -1
	}
	nSPS := int(pay[5] & 0x1f)
	if nSPS == 0 {
		return -1
	}
	off := 6
	for k := 0; k < nSPS; k++ {
		if off+2 > len(pay) {
			return -1
		}
		l := int(u16be(pay[off : off+2]))
		off += 2
		if l <= 0 || off+l > len(pay) {
			return -1
		}
		if _, _, chroma, ok := h264SPS(pay[off : off+l]); ok {
			return chroma
		}
		off += l
	}
	return -1
}

// ProfileName renders an H.264 profile idc for messages.
func ProfileName(profile uint8) string {
	switch profile {
	case 66:
		return "baseline"
	case 77:
		return "main"
	case 88:
		return "extended"
	case 100:
		return "high"
	case 110:
		return "high 10"
	case 122:
		return "high 4:2:2"
	case 244:
		return "high 4:4:4 predictive"
	}
	return fmt.Sprintf("profile %d", profile)
}
