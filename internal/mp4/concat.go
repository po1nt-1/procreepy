package mp4

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// trackSig is the set of properties that must match across every segment for
// a stream-copy concat. It is stricter than ffprobe's pix_fmt comparison: the
// codec configuration bytes (avcC/hvcC/esds) must be byte-identical.
type trackSig struct {
	kind    string // "video" or "audio"
	handler string
	codec   string // human-facing name, e.g. "h264", "aac"
	fourcc  string
	width   uint16
	height  uint16
	pixfmt  string
	rate    uint32
	chans   uint8
	ts      uint32
	config  string // "avcC", "hvcC", "esds", ...
	confDat []byte
	matrix  [9]int64
	sync    bool
}

func (t *TrackState) sig() trackSig {
	kind := "video"
	if t.Handler == "soun" {
		kind = "audio"
	}
	return trackSig{
		kind: kind, handler: t.Handler, codec: t.CodecName, fourcc: t.FourCC,
		width: t.Width, height: t.Height, pixfmt: t.PixFmt,
		rate: t.AudioRate, chans: t.AudioChans, ts: t.Timescale,
		config: t.ConfigBox, confDat: t.ConfigData, matrix: t.Matrix,
		sync: t.HasSyncTable,
	}
}

func (s trackSig) same(o trackSig) bool {
	return s.kind == o.kind && s.handler == o.handler && s.fourcc == o.fourcc &&
		s.width == o.width && s.height == o.height &&
		s.rate == o.rate && s.chans == o.chans && s.ts == o.ts &&
		s.config == o.config && bytes.Equal(s.confDat, o.confDat) &&
		s.matrix == o.matrix && s.sync == o.sync
}

func (s trackSig) describe() string {
	if s.kind == "video" {
		return fmt.Sprintf("video %s %dx%d %s", s.codec, s.width, s.height, s.pixfmt)
	}
	return fmt.Sprintf("audio %s %dHz %dch", s.codec, s.rate, s.chans)
}

func movieSig(m *Movie) []trackSig {
	out := make([]trackSig, len(m.Tracks))
	for i, t := range m.Tracks {
		out[i] = t.sig()
	}
	return out
}

func sigsEqual(a, b []trackSig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].same(b[i]) {
			return false
		}
	}
	return true
}

func describeSigs(sigs []trackSig) string {
	if len(sigs) == 0 {
		return "no streams"
	}
	parts := make([]string, len(sigs))
	for i, s := range sigs {
		parts[i] = s.describe()
	}
	return strings.Join(parts, ", ")
}

// MergedTrack is one track of the concatenated result.
type MergedTrack struct {
	ref      *TrackState
	sizes    []uint32
	stts     []SttsEntry
	stsc     []StscEntry
	sync     []uint32
	ctts     []CttsEntry
	hasSync  bool
	hasCtts  bool
	signedCT bool
	// Per merged chunk: which segment it came from and its offset within that
	// segment's mdat payload.
	chunkSeg []int
	chunkRel []int64
}

// Merged is the concatenation plan for a set of compatible segments.
type Merged struct {
	ftyp      []byte
	movTS     uint32
	mvhdDur   uint64
	tracks    []*MergedTrack
	tkID      []uint32
	tkDur     []uint64
	mdDur     []uint64
	lang      []uint32
	segStart  []int64 // per segment: source mdat payload start (member-relative)
	segSize   []int64 // per segment: source mdat payload size
	useCo64   bool
	moovLen   int
	mdatBase  []int64
	finalized bool
}

// Merge combines several parsed, compatible MP4 segments into a single
// stream-copy concatenation plan. It returns ErrIncompatible (wrapped) when
// the segments differ in a way that forbids `-c copy`.
func Merge(movies []*Movie) (*Merged, error) {
	if len(movies) == 0 {
		return nil, fmt.Errorf("%w: nothing to merge", ErrBadStructure)
	}
	first := movies[0]
	for i, m := range movies {
		if m == nil {
			return nil, fmt.Errorf("%w: segment %d is nil", ErrBadStructure, i+1)
		}
		if len(m.Mdat) != 1 {
			return nil, fmt.Errorf("%w: segment %d has %d mdat boxes (want 1)", ErrUnsupported, i+1, len(m.Mdat))
		}
	}
	if !hasVideoTrack(first) {
		return nil, fmt.Errorf("%w: segment 1 has no video track", ErrUnsupported)
	}
	refSigs := movieSig(first)
	// Compatibility: every segment must match the first, stream by stream.
	var bad []string
	for si, m := range movies[1:] {
		if m.Mvhd.Timescale != first.Mvhd.Timescale {
			bad = append(bad, fmt.Sprintf("  segment %d: movie timescale %d (want %d)", si+2, m.Mvhd.Timescale, first.Mvhd.Timescale))
			continue
		}
		if !sigsEqual(refSigs, movieSig(m)) {
			bad = append(bad, fmt.Sprintf("  segment %d: %s", si+2, describeSigs(movieSig(m))))
		}
	}
	if len(bad) > 0 {
		lines := []string{fmt.Sprintf("  segment 1: %s", describeSigs(refSigs))}
		if len(bad) > 5 {
			lines = append(lines, bad[:5]...)
			lines = append(lines, fmt.Sprintf("  ... and %d more", len(bad)-5))
		} else {
			lines = append(lines, bad...)
		}
		return nil, fmt.Errorf("%w: stream copy is impossible\n%s", ErrIncompatible, strings.Join(lines, "\n"))
	}

	n := len(first.Tracks)
	mg := &Merged{
		ftyp:     first.Ftyp,
		movTS:    first.Mvhd.Timescale,
		mvhdDur:  0,
		tracks:   make([]*MergedTrack, n),
		tkID:     make([]uint32, n),
		tkDur:    make([]uint64, n),
		mdDur:    make([]uint64, n),
		lang:     make([]uint32, n),
		segStart: make([]int64, len(movies)),
		segSize:  make([]int64, len(movies)),
	}
	if len(mg.ftyp) == 0 {
		mg.ftyp = EncFTYP()
	}
	for si, m := range movies {
		mg.segStart[si] = m.Mdat[0].Start
		mg.segSize[si] = m.Mdat[0].End - m.Mdat[0].Start
		mg.mvhdDur += m.Mvhd.Duration
	}
	for ti := 0; ti < n; ti++ {
		ref := first.Tracks[ti]
		mt := &MergedTrack{ref: ref, hasSync: ref.HasSyncTable, hasCtts: ref.HasCtts, signedCT: ref.CttsSigned}
		for si, m := range movies {
			tr := m.Tracks[ti]
			baseStart := m.Mdat[0].Start
			chunkShift := len(mt.chunkSeg)
			sampleShift := len(mt.sizes)
			for _, e := range tr.Stts {
				appendRun(&mt.stts, e.Count, e.Delta)
			}
			for _, e := range tr.Stsc {
				appendStsc(mt, e.FirstChunk+uint32(chunkShift), e.SamplesPerChunk, e.Description)
			}
			if mt.hasSync {
				for _, s := range tr.SyncSamples {
					mt.sync = append(mt.sync, s+uint32(sampleShift))
				}
			}
			for _, e := range tr.Ctts {
				appendCtts(mt, e.Count, e.Offset)
			}
			mt.sizes = append(mt.sizes, tr.SampleSizes...)
			for _, off := range tr.Chunks {
				mt.chunkSeg = append(mt.chunkSeg, si)
				mt.chunkRel = append(mt.chunkRel, int64(off)-baseStart)
			}
			mg.tkDur[ti] += tr.TrackDuration
			mg.mdDur[ti] += tr.MediaDuration
		}
		mg.tkID[ti] = ref.ID
		mg.lang[ti] = ref.Language
		mg.tracks[ti] = mt
	}
	return mg, nil
}

func hasVideoTrack(m *Movie) bool {
	for _, t := range m.Tracks {
		if t.Handler == "vide" {
			return true
		}
	}
	return false
}

// appendRun merges a (count, delta) run into a stts-like slice when the
// previous run has the same delta.
func appendRun(dst *[]SttsEntry, count, delta uint32) {
	if n := len(*dst); n > 0 && (*dst)[n-1].Delta == delta {
		(*dst)[n-1].Count += count
		return
	}
	*dst = append(*dst, SttsEntry{Count: count, Delta: delta})
}

func appendStsc(mt *MergedTrack, firstChunk, spc, desc uint32) {
	if n := len(mt.stsc); n > 0 && mt.stsc[n-1].SamplesPerChunk == spc && mt.stsc[n-1].Description == desc {
		return // extends the previous run
	}
	mt.stsc = append(mt.stsc, StscEntry{FirstChunk: firstChunk, SamplesPerChunk: spc, Description: desc})
}

func appendCtts(mt *MergedTrack, count uint32, off int32) {
	if n := len(mt.ctts); n > 0 && mt.ctts[n-1].Offset == off {
		mt.ctts[n-1].Count += count
		return
	}
	mt.ctts = append(mt.ctts, CttsEntry{Count: count, Offset: off})
}

// HasVideo reports whether the movie has a video track.
func (m *Movie) HasVideo() bool { return hasVideoTrack(m) }

// StreamsSummary describes the movie's streams for diagnostics, e.g.
// "video h264 320x240 yuv420p, audio aac 44100Hz 2ch".
func (m *Movie) StreamsSummary() string { return describeSigs(movieSig(m)) }

// SameStreams reports whether a and b carry the same streams with
// byte-identical configurations (stream-copy compatibility, per track).
func SameStreams(a, b *Movie) bool { return sigsEqual(movieSig(a), movieSig(b)) }

// SegmentCount is the number of source segments.
func (m *Merged) SegmentCount() int { return len(m.segSize) }

// MdatSourceStart and MdatPayloadSize locate one segment's media data in its
// own (member) byte stream: read PayloadSize bytes starting at SourceStart.
func (m *Merged) MdatSourceStart(k int) int64 { return m.segStart[k] }
func (m *Merged) MdatPayloadSize(k int) int64 { return m.segSize[k] }

// DurationSeconds is the total decoded duration of the result.
func (m *Merged) DurationSeconds() float64 {
	return float64(m.mvhdDur) / float64(m.movTS)
}

func (m *Merged) trackOffsets(ti int, mt *MergedTrack, base []int64) []uint64 {
	offs := make([]uint64, len(mt.chunkSeg))
	for c := range mt.chunkSeg {
		offs[c] = uint64(base[mt.chunkSeg[c]] + mt.chunkRel[c])
	}
	return offs
}

func (m *Merged) moovBytes(useCo64 bool, base []int64) []byte {
	kids := make([][]byte, 0, 1+len(m.tracks))
	kids = append(kids, EncMVHD(m.movTS, m.mvhdDur))
	for ti, mt := range m.tracks {
		ref := mt.ref
		trkKids := make([][]byte, 0, 2)
		trkKids = append(trkKids, EncTKHD(m.tkID[ti], m.tkDur[ti], ref.Width, ref.Height, ref.Volume, ref.Matrix))
		stblKids := make([][]byte, 0, 7)
		stblKids = append(stblKids, ref.StsdRaw)
		stblKids = append(stblKids, EncSTTS(mt.stts))
		stblKids = append(stblKids, EncSTSC(mt.stsc))
		stblKids = append(stblKids, EncSTSZ(mt.sizes))
		stblKids = append(stblKids, EncSTCO(m.trackOffsets(ti, mt, base), useCo64))
		if mt.hasSync {
			stblKids = append(stblKids, EncSTSS(mt.sync))
		}
		if mt.hasCtts {
			stblKids = append(stblKids, EncCTTS(mt.ctts, mt.signedCT))
		}
		stbl := Container("stbl", stblKids...)
		var mediaHead []byte
		handlerName := "VideoHandler"
		if ref.Handler == "soun" {
			mediaHead = EncSMHD()
			handlerName = "SoundHandler"
		} else {
			mediaHead = EncVMHD()
		}
		minf := Container("minf", mediaHead, EncDINF(), stbl)
		mdia := Container("mdia", EncMDHD(ref.Timescale, m.mdDur[ti], m.lang[ti]), EncHDLR(ref.Handler, handlerName), minf)
		trkKids = append(trkKids, mdia)
		kids = append(kids, Container("trak", trkKids...))
	}
	return Container("moov", kids...)
}

// MdatHead renders the mdat box header for a payload of the given size; it
// switches to the 64-bit largesize form when the 32-bit size field would
// overflow.
func MdatHead(payload int64) ([]byte, error) {
	if payload < 0 || payload >= 1<<62 {
		return nil, fmt.Errorf("mp4: mdat payload too large")
	}
	total := 8 + payload
	b := make([]byte, 16)
	if total >= 1<<32 {
		binary.BigEndian.PutUint32(b[0:4], 1)
		binary.BigEndian.PutUint64(b[4:12], uint64(total))
		copy(b[12:16], "mdat")
		return b, nil
	}
	binary.BigEndian.PutUint32(b[0:4], uint32(total))
	copy(b[4:8], "mdat")
	return b[:8], nil
}

// MdatHeaderSize is the size in bytes of the mdat box header for a payload.
func MdatHeaderSize(payload int64) int {
	if 8+payload >= 1<<32 {
		return 16
	}
	return 8
}

// measure computes the moov size (for the given offset width) and the
// resulting per-segment mdat payload base offsets.
func (m *Merged) measure(useCo64 bool) (int, []int64) {
	zero := make([]int64, len(m.segSize))
	moovLen := len(m.moovBytes(useCo64, zero))
	base := make([]int64, len(m.segSize))
	// The first chunk offset lands at the start of the first mdat *payload*,
	// i.e. after ftyp + moov + the mdat box header.
	off := int64(len(m.ftyp)) + int64(moovLen)
	for k, ps := range m.segSize {
		off += int64(MdatHeaderSize(ps))
		base[k] = off
		off += ps
	}
	return moovLen, base
}

func (m *Merged) fits32(base []int64) bool {
	for _, mt := range m.tracks {
		for c := range mt.chunkSeg {
			if base[mt.chunkSeg[c]]+mt.chunkRel[c] >= 1<<32 {
				return false
			}
		}
	}
	return true
}

// Finalize picks stco vs co64 and fixes the output layout. It is called
// lazily by Emit/Moov/MdatBase.
func (m *Merged) Finalize() error {
	if m.finalized {
		return nil
	}
	moovLen, base := m.measure(false)
	if m.fits32(base) {
		m.useCo64 = false
	} else {
		moovLen, base = m.measure(true)
		m.useCo64 = true
	}
	m.moovLen = moovLen
	m.mdatBase = base
	m.finalized = true
	return nil
}

// Ftyp returns the leading ftyp box.
func (m *Merged) Ftyp() []byte { return m.ftyp }

// MoovSize returns the serialized moov box size.
func (m *Merged) MoovSize() int {
	if err := m.Finalize(); err != nil {
		panic(err)
	}
	return m.moovLen
}

// Moov returns the serialized moov box.
func (m *Merged) Moov() []byte {
	if err := m.Finalize(); err != nil {
		panic(err)
	}
	return m.moovBytes(m.useCo64, m.mdatBase)
}

// MdatBase is the output offset where segment k's mdat payload begins.
func (m *Merged) MdatBase(k int) int64 {
	if err := m.Finalize(); err != nil {
		panic(err)
	}
	return m.mdatBase[k]
}

// Emit writes the concatenated, moov-first MP4 to out. payloads[k] must be
// segment k's full mdat payload (MdatPayloadSize(k) bytes).
func (m *Merged) Emit(out io.Writer, payloads [][]byte) error {
	if err := m.Finalize(); err != nil {
		return err
	}
	if len(payloads) != len(m.segSize) {
		return fmt.Errorf("mp4: got %d mdat payloads, want %d", len(payloads), len(m.segSize))
	}
	if _, err := out.Write(m.ftyp); err != nil {
		return err
	}
	if _, err := out.Write(m.Moov()); err != nil {
		return err
	}
	for k, p := range payloads {
		if int64(len(p)) != m.segSize[k] {
			return fmt.Errorf("mp4: segment %d mdat payload is %d bytes, want %d", k+1, len(p), m.segSize[k])
		}
		head, err := MdatHead(int64(len(p)))
		if err != nil {
			return err
		}
		if _, err := out.Write(head); err != nil {
			return err
		}
		if _, err := out.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// EmitToBytes is a non-streaming convenience used by tests.
func (m *Merged) EmitToBytes(payloads [][]byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := m.Emit(&buf, payloads); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
