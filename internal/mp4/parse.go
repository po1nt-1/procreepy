package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Parse reads a whole progressive MP4 file from r (size bytes) and returns
// its essence. Fragmented layouts (top-level moof/mdat) are rejected.
// Top-level mdat payloads are never loaded: their media data can be
// gigabytes long and is only referenced by (start, end) for later streaming.
func Parse(r io.ReaderAt, size int64) (*Movie, error) {
	rd := NewReader(r, size)
	top, err := scanBoxes(rd, 0, size, func(typ string) bool { return typ == "mdat" })
	if err != nil {
		return nil, fmt.Errorf("read top level: %w", err)
	}
	m := &Movie{}
	moovSeen := false
	for _, b := range top {
		switch b.Type {
		case "ftyp":
			if m.Ftyp != nil {
				return nil, fmt.Errorf("%w: multiple ftyp boxes", ErrBadStructure)
			}
			raw := make([]byte, b.Size)
			if _, err := rd.ReadAt(raw, b.Off); err != nil {
				return nil, err
			}
			m.Ftyp = raw
		case "moov":
			if moovSeen {
				return nil, fmt.Errorf("%w: multiple moov boxes", ErrBadStructure)
			}
			moovSeen = true
			if b.Size-8 > MaxMoovSize {
				return nil, fmt.Errorf("%w: moov is %d bytes", ErrUnsupported, b.Size-8)
			}
			if err := m.parseMoov(b.Pay); err != nil {
				return nil, err
			}
		case "mdat":
			m.Mdat = append(m.Mdat, Region{Start: b.PayOff, End: b.Off + b.Size})
		case "moof", "traf", "mfra":
			return nil, fmt.Errorf("%w: fragmented MP4 is not supported", ErrUnsupported)
		}
	}
	if !moovSeen {
		return nil, fmt.Errorf("%w: no moov box", ErrUnsupported)
	}
	if len(m.Ftyp) == 0 {
		return nil, fmt.Errorf("%w: no ftyp box", ErrUnsupported)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Movie) parseMoov(pay []byte) error {
	kids, err := children(pay)
	if err != nil {
		return err
	}
	tracks := 0
	for _, b := range kids {
		switch b.Type {
		case "mvhd":
			if err := m.parseMvhd(b.Pay); err != nil {
				return err
			}
		case "trak":
			t, err := parseTrak(b.Pay)
			if err != nil {
				return err
			}
			m.Tracks = append(m.Tracks, t)
			tracks++
		case "udta", "meta":
			// metadata is not needed for stream copy
		default:
			// tolerate unknown boxes at this level
		}
	}
	if tracks == 0 {
		return fmt.Errorf("%w: moov has no tracks", ErrUnsupported)
	}
	return nil
}

func (m *Movie) parseMvhd(pay []byte) error {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return err
	}
	rest := pay[4:]
	switch ver {
	case 0:
		// creation(4) modification(4) timescale(4) duration(4)
		if len(rest) < 16 {
			return ErrTruncated
		}
		m.Mvhd.Timescale = u32be(rest[8:12])
		m.Mvhd.Duration = uint64(u32be(rest[12:16]))
	case 1:
		// creation(8) modification(8) timescale(4) duration(8)
		if len(rest) < 28 {
			return ErrTruncated
		}
		m.Mvhd.Timescale = u32be(rest[16:20])
		m.Mvhd.Duration = u64be(rest[20:28])
	default:
		return fmt.Errorf("%w: mvhd version %d", ErrUnsupported, ver)
	}
	if m.Mvhd.Timescale == 0 {
		return fmt.Errorf("%w: mvhd timescale is zero", ErrBadStructure)
	}
	return nil
}

func parseTrak(pay []byte) (*TrackState, error) {
	ch, err := children(pay)
	if err != nil {
		return nil, err
	}
	t := &TrackState{}
	var mdiaPay []byte
	sawEdits := false
	for _, b := range ch {
		switch b.Type {
		case "tkhd":
			if err := parseTkhd(b.Pay, t); err != nil {
				return nil, err
			}
		case "mdia":
			mdiaPay = b.Pay
		case "edts":
			sawEdits = true
		case "udta":
		default:
		}
	}
	if mdiaPay == nil {
		return nil, fmt.Errorf("%w: trak has no mdia", ErrBadStructure)
	}
	if err := parseMdia(mdiaPay, t); err != nil {
		return nil, err
	}
	_ = sawEdits // edit lists are dropped on purpose when concatenating
	return t, nil
}

func parseTkhd(pay []byte, t *TrackState) error {
	ver, flags, err := versionFlags(pay)
	if err != nil {
		return err
	}
	rest := pay[4:]
	// Version 0: creation(4) mod(4) id(4) res(4) dur(4) res(8)
	// layer(2) alt(2) vol(2) res(2) matrix(36) w(4) h(4)
	switch ver {
	case 0:
		if len(rest) < 80 {
			return ErrTruncated
		}
		t.ID = u32be(rest[8:12])
		t.TrackDuration = uint64(u32be(rest[16:20]))
		t.Volume = u16be(rest[32:34])
		copyMatrix(&t.Matrix, rest[36:72])
		t.FixedW = u32be(rest[72:76])
		t.FixedH = u32be(rest[76:80])
	case 1:
		// creation(8) mod(8) id(4) res(4) dur(8) res(8)
		// layer(2) alt(2) vol(2) res(2) matrix(36) w(4) h(4)
		if len(rest) < 92 {
			return ErrTruncated
		}
		t.ID = u32be(rest[16:20])
		t.TrackDuration = u64be(rest[24:32])
		t.Volume = u16be(rest[44:46])
		copyMatrix(&t.Matrix, rest[48:84])
		t.FixedW = u32be(rest[84:88])
		t.FixedH = u32be(rest[88:92])
	default:
		return fmt.Errorf("%w: tkhd version %d", ErrUnsupported, ver)
	}
	if t.ID == 0 {
		return fmt.Errorf("%w: tkhd track id is zero", ErrBadStructure)
	}
	_ = flags
	return nil
}

func copyMatrix(dst *[9]int64, raw []byte) {
	for i := 0; i < 9; i++ {
		dst[i] = int64(int32(u32be(raw[i*4:])))
	}
}

func parseMdia(pay []byte, t *TrackState) error {
	ch, err := children(pay)
	if err != nil {
		return err
	}
	var minfPay, hdlrPay, mdhdPay []byte
	for _, b := range ch {
		switch b.Type {
		case "mdhd":
			mdhdPay = b.Pay
		case "hdlr":
			hdlrPay = b.Pay
		case "minf":
			minfPay = b.Pay
		}
	}
	if mdhdPay == nil || hdlrPay == nil || minfPay == nil {
		return fmt.Errorf("%w: mdia incomplete", ErrBadStructure)
	}
	if err := parseMdhd(mdhdPay, t); err != nil {
		return err
	}
	ver, _, err := versionFlags(hdlrPay)
	_ = ver
	if err != nil {
		return err
	}
	if len(hdlrPay) < 4+4+4 {
		return ErrTruncated
	}
	t.Handler = string(hdlrPay[8:12])
	switch t.Handler {
	case "vide", "soun":
	default:
		return fmt.Errorf("%w: handler %q", ErrUnsupported, t.Handler)
	}
	return parseMinf(minfPay, t)
}

func parseMdhd(pay []byte, t *TrackState) error {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return err
	}
	rest := pay[4:]
	langAt := 16 // v0: creation(4) mod(4) timescale(4) duration(4)
	switch ver {
	case 0:
		if len(rest) < 16 {
			return ErrTruncated
		}
		t.Timescale = u32be(rest[8:12])
		t.MediaDuration = uint64(u32be(rest[12:16]))
	case 1:
		// creation(8) modification(8) timescale(4) duration(8)
		langAt = 8 + 8 + 4 + 8
		if len(rest) < 28 {
			return ErrTruncated
		}
		t.Timescale = u32be(rest[16:20])
		t.MediaDuration = u64be(rest[20:28])
	default:
		return fmt.Errorf("%w: mdhd version %d", ErrUnsupported, ver)
	}
	if t.Timescale == 0 {
		return fmt.Errorf("%w: mdhd timescale is zero", ErrBadStructure)
	}
	if len(rest) >= langAt+4 {
		t.Language = u32be(rest[langAt : langAt+4])
	}
	return nil
}

func parseMinf(pay []byte, t *TrackState) error {
	ch, err := children(pay)
	if err != nil {
		return err
	}
	var stblPay []byte
	for _, b := range ch {
		switch b.Type {
		case "vmhd", "smhd", "gmhd", "dinf", "stbl":
			if b.Type == "stbl" {
				stblPay = b.Pay
			}
		default:
		}
	}
	if stblPay == nil {
		return fmt.Errorf("%w: minf has no stbl", ErrBadStructure)
	}
	return parseStbl(stblPay, t)
}

func parseStbl(pay []byte, t *TrackState) error {
	ch, err := children(pay)
	if err != nil {
		return err
	}
	var stsd, stts, stsc, stsz, stco, stss, ctts *Box
	for i := range ch {
		switch ch[i].Type {
		case "stsd":
			stsd = &ch[i]
		case "stts":
			stts = &ch[i]
		case "stsc":
			stsc = &ch[i]
		case "stsz":
			stsz = &ch[i]
		case "stco":
			stco = &ch[i]
		case "co64":
			stco = &ch[i]
		case "stss":
			stss = &ch[i]
		case "ctts":
			ctts = &ch[i]
		}
	}
	if stsd == nil || stts == nil || stsc == nil || stsz == nil || stco == nil {
		return fmt.Errorf("%w: stbl incomplete", ErrBadStructure)
	}
	t.StsdRaw = rawBox(stsd)
	if err := parseStsd(stsd, t); err != nil {
		return err
	}
	t.Stts, err = parseStts(stts.Pay)
	if err != nil {
		return err
	}
	t.Stsc, err = parseStsc(stsc.Pay)
	if err != nil {
		return err
	}
	t.SampleCount, t.SampleSizes, err = parseStsz(stsz.Pay)
	if err != nil {
		return err
	}
	t.Chunks, err = parseChunkOffsets(stco)
	if err != nil {
		return err
	}
	if stss != nil {
		t.SyncSamples, err = parseStss(stss.Pay)
		if err != nil {
			return err
		}
		t.HasSyncTable = true
	}
	if ctts != nil {
		t.Ctts, t.CttsSigned, err = parseCtts(ctts.Pay)
		if err != nil {
			return err
		}
		t.HasCtts = true
	}
	if len(t.Stts) == 0 {
		return fmt.Errorf("%w: stts is empty", ErrBadStructure)
	}
	return nil
}

// rawBox returns a box with header reconstructed from the parent payload
// offset (children() already relativized Off).
func rawBox(b *Box) []byte {
	out := make([]byte, b.Size)
	binary.BigEndian.PutUint32(out, uint32(b.Size))
	copy(out[4:], b.Type)
	copy(out[8:], b.Pay)
	return out
}

func parseStts(pay []byte) ([]SttsEntry, error) {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return nil, err
	}
	if ver != 0 {
		return nil, fmt.Errorf("%w: stts version %d", ErrUnsupported, ver)
	}
	rest := pay[4:]
	if len(rest) < 4 || len(rest)%8 != 4 {
		return nil, ErrTruncated
	}
	n := int(u32be(rest[0:4]))
	if len(rest) < 4+n*8 || n == 0 {
		return nil, ErrTruncated
	}
	out := make([]SttsEntry, n)
	for i := 0; i < n; i++ {
		e := rest[4+i*8:]
		out[i] = SttsEntry{Count: u32be(e[0:4]), Delta: u32be(e[4:8])}
	}
	return out, nil
}

func parseStsc(pay []byte) ([]StscEntry, error) {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return nil, err
	}
	if ver != 0 {
		return nil, fmt.Errorf("%w: stsc version %d", ErrUnsupported, ver)
	}
	rest := pay[4:]
	if len(rest) < 4 || len(rest)%12 != 4 {
		return nil, ErrTruncated
	}
	n := int(u32be(rest[0:4]))
	if len(rest) < 4+n*12 || n == 0 {
		return nil, ErrTruncated
	}
	out := make([]StscEntry, n)
	for i := 0; i < n; i++ {
		e := rest[4+i*12:]
		out[i] = StscEntry{FirstChunk: u32be(e[0:4]), SamplesPerChunk: u32be(e[4:8]), Description: u32be(e[8:12])}
	}
	return out, nil
}

func parseStsz(pay []byte) (uint32, []uint32, error) {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return 0, nil, err
	}
	if ver != 0 {
		return 0, nil, fmt.Errorf("%w: stsz version %d", ErrUnsupported, ver)
	}
	rest := pay[4:]
	if len(rest) < 8 {
		return 0, nil, ErrTruncated
	}
	uniform := u32be(rest[0:4])
	count := u32be(rest[4:8])
	if count > MaxSamples {
		return 0, nil, fmt.Errorf("%w: stsz declares %d samples", ErrBadStructure, count)
	}
	if uniform != 0 {
		sizes := make([]uint32, count)
		for i := range sizes {
			sizes[i] = uniform
		}
		return count, sizes, nil
	}
	if len(rest) < 8+int(count)*4 {
		return 0, nil, ErrTruncated
	}
	sizes := make([]uint32, count)
	for i := uint32(0); i < count; i++ {
		sizes[i] = u32be(rest[8+i*4:])
	}
	return count, sizes, nil
}

func parseChunkOffsets(b *Box) ([]uint64, error) {
	ver, _, err := versionFlags(b.Pay)
	if err != nil {
		return nil, err
	}
	if ver != 0 {
		return nil, fmt.Errorf("%w: chunk offset table version %d", ErrUnsupported, ver)
	}
	rest := b.Pay[4:]
	if len(rest) < 4 {
		return nil, ErrTruncated
	}
	n := int(u32be(rest[0:4]))
	body := rest[4:]
	width := 4
	if b.Type == "co64" {
		width = 8
	}
	if len(body) < n*int(width) {
		return nil, ErrTruncated
	}
	out := make([]uint64, n)
	for i := 0; i < n; i++ {
		e := body[i*width:]
		if width == 8 {
			out[i] = u64be(e[:8])
		} else {
			out[i] = uint64(u32be(e[:4]))
		}
	}
	return out, nil
}

func parseStss(pay []byte) ([]uint32, error) {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return nil, err
	}
	if ver != 0 {
		return nil, fmt.Errorf("%w: stss version %d", ErrUnsupported, ver)
	}
	rest := pay[4:]
	if len(rest) < 4 {
		return nil, ErrTruncated
	}
	n := int(u32be(rest[0:4]))
	if len(rest) < 4+n*4 {
		return nil, ErrTruncated
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = u32be(rest[4+i*4:])
	}
	return out, nil
}

func parseCtts(pay []byte) ([]CttsEntry, bool, error) {
	ver, _, err := versionFlags(pay)
	if err != nil {
		return nil, false, err
	}
	if ver != 0 && ver != 1 {
		return nil, false, fmt.Errorf("%w: ctts version %d", ErrUnsupported, ver)
	}
	signed := ver == 1
	rest := pay[4:]
	if len(rest) < 4 || len(rest)%8 != 4 {
		return nil, signed, ErrTruncated
	}
	n := int(u32be(rest[0:4]))
	if len(rest) < 4+n*8 {
		return nil, signed, ErrTruncated
	}
	out := make([]CttsEntry, n)
	for i := 0; i < n; i++ {
		e := rest[4+i*8:]
		off := int32(u32be(e[4:8]))
		out[i] = CttsEntry{Count: u32be(e[0:4]), Offset: off}
	}
	return out, signed, nil
}

// sampleEntry is one parsed sample description.
type sampleEntry struct {
	fourcc    string
	width     uint16
	height    uint16
	audioRate uint32
	audioChan uint8
	// codec-specific configuration
	configBox  string
	configData []byte
}

func parseStsd(b *Box, t *TrackState) error {
	// stsd is a FullBox: payload = version(1) flags(3) entry_count(4) entries.
	if len(b.Pay) < 8 {
		return ErrTruncated
	}
	entries, err := children(b.Pay[8:])
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("%w: stsd is empty", ErrBadStructure)
	}
	first := entries[0]
	se, err := parseSampleEntry(first)
	if err != nil {
		return err
	}
	t.FourCC = se.fourcc
	t.Width = se.width
	t.Height = se.height
	t.AudioRate = se.audioRate
	t.AudioChans = se.audioChan
	t.ConfigBox = se.configBox
	t.ConfigData = se.configData
	t.CodecName = codecName(se.fourcc, se.configData)
	t.PixFmt = pixFmt(t)
	if t.Handler == "vide" && (t.Width == 0 || t.Height == 0) {
		return fmt.Errorf("%w: video sample has no dimensions", ErrBadStructure)
	}
	return nil
}

// parseSampleEntry parses one sample description box (visual or audio).
// b.Type is the fourcc; b.Pay is the entry body after size+fourcc.
func parseSampleEntry(b Box) (sampleEntry, error) {
	se := sampleEntry{fourcc: b.Type}
	pay := b.Pay
	// Offsets are relative to b.Pay, i.e. after the entry size+fourcc.
	// VisualSampleEntry: fixed header is 78 bytes; AudioSampleEntry: 28.
	headerOff := len(pay) // unknown entry formats: no codec boxes
	switch se.fourcc {
	case "avc1", "avc3", "hvc1", "hev1", "vp09", "av01", "mve1", "vvc1":
		if len(pay) < 78 {
			return se, ErrTruncated
		}
		se.width = u16be(pay[24:26])
		se.height = u16be(pay[26:28])
		headerOff = 78
	case "mp4a", "ac-3", "ac-4", "alac", "Opus":
		if len(pay) < 28 {
			return se, ErrTruncated
		}
		se.audioChan = uint8(u16be(pay[16:18]))
		se.audioRate = u32be(pay[24:28]) >> 16
		headerOff = 28
	}
	if headerOff > len(pay) {
		return se, nil
	}
	body := pay[headerOff:]
	ch, err := children(body)
	if err != nil {
		// Codec config is optional for display purposes; keep going.
		return se, nil
	}
	for _, cb := range ch {
		switch cb.Type {
		case "avcC", "hvcC", "vpcC", "av1C":
			se.configBox = cb.Type
			se.configData = append([]byte(nil), cb.Pay...)
		case "esds":
			if se.configBox == "" {
				se.configBox = "esds"
				se.configData = append([]byte(nil), cb.Pay...)
			}
			if aot, freq, chans, ok := parseESDS(cb.Pay); ok {
				if freq != 0 {
					se.audioRate = freq
				}
				if chans > 0 {
					se.audioChan = chans
				}
				_ = aot
			}
		}
	}
	return se, nil
}

// codecName maps an mp4 fourcc to the canonical name used in reports.
func codecName(fourcc string, config []byte) string {
	switch fourcc {
	case "avc1", "avc3":
		return "h264"
	case "hvc1", "hev1":
		return "hevc"
	case "vp09":
		return "vp9"
	case "av01":
		return "av1"
	case "mp4a":
		if aot, _, _, ok := parseESDS(config); ok {
			switch aot {
			case 1, 2, 5:
				return "aac"
			}
		}
		return "aac"
	case "ac-3":
		return "ac3"
	case "ac-4":
		return "ec3"
	case "alac":
		return "alac"
	case "Opus":
		return "opus"
	case "flac":
		return "flac"
	}
	return fourcc
}

func pixFmt(t *TrackState) string {
	if t.Handler != "vide" {
		return ""
	}
	if t.ConfigBox == "avcC" {
		if prof, _, ok := parseAVCConfig(t.ConfigData); ok {
			_ = prof
			if chroma := spsChromaFromConfig(t.ConfigData); chroma >= 0 {
				switch chroma {
				case 0:
					return "gray"
				case 1:
					return "yuv420p"
				case 2:
					return "yuv422p"
				case 3:
					return "yuv444p"
				}
			}
			return "unknown"
		}
	}
	return "unknown"
}
