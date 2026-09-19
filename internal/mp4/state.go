package mp4

import (
	"fmt"
)

// Region is a contiguous payload range inside the file (mdat payload).
type Region struct {
	Start int64
	End   int64
}

// Mvhd is the movie header.
type Mvhd struct {
	Timescale uint32
	Duration  uint64 // in Timescale units
}

// TrackState describes one parsed track of one segment.
type TrackState struct {
	ID      uint32
	Handler string // "vide", "soun", ...
	Volume  uint16
	Matrix  [9]int64
	FixedW  uint32 // 16.16 fixed point
	FixedH  uint32

	TrackDuration uint64 // tkhd duration, movie timescale

	Timescale     uint32
	MediaDuration uint64 // in Timescale units
	Language      uint32 // 0 => 'und'

	StsdRaw    []byte // the whole stsd box, to be copied verbatim
	FourCC     string
	Width      uint16
	Height     uint16
	AudioRate  uint32 // Hz
	AudioChans uint8
	ConfigBox  string // "avcC", "hvcC", "esds", ...
	ConfigData []byte // raw configuration payload (signature material)
	PixFmt     string // best-effort display name
	CodecName  string // e.g. "h264", "aac"

	SampleCount  uint32
	SampleSizes  []uint32
	Stts         []SttsEntry
	Stsc         []StscEntry
	Chunks       []uint64 // offsets, one per chunk
	SyncSamples  []uint32 // empty => every sample is a sync sample
	Ctts         []CttsEntry
	HasSyncTable bool
	HasCtts      bool
	CttsSigned   bool // composition offsets are signed (ctts version 1)
}

// SttsEntry is one run-length entry of a decode-time-to-sample table.
type SttsEntry struct {
	Count uint32
	Delta uint32
}

// StscEntry maps chunks to sample counts.
type StscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
	Description     uint32
}

// CttsEntry is one composition-time-offset run.
type CttsEntry struct {
	Count  uint32
	Offset int32
}

// Movie is the parsed essence of one MP4 file.
type Movie struct {
	Ftyp   []byte // raw ftyp box (header included)
	Mvhd   Mvhd
	Tracks []*TrackState
	Mdat   []Region // top-level mdat payloads, file order
}

// Validate checks structural invariants that ffprobe also enforces
// (table coverage, chunk ranges) and reports corrupted input.
func (m *Movie) Validate() error {
	if len(m.Tracks) == 0 {
		return fmt.Errorf("%w: no tracks", ErrUnsupported)
	}
	if len(m.Mdat) == 0 {
		return fmt.Errorf("%w: no media data (mdat)", ErrUnsupported)
	}
	var dataStart, dataEnd int64 = m.Mdat[0].Start, m.Mdat[0].End
	for _, r := range m.Mdat[1:] {
		if r.Start < dataEnd {
			return fmt.Errorf("%w: overlapping mdat boxes", ErrBadStructure)
		}
		dataEnd = r.End
	}
	for i, t := range m.Tracks {
		if err := t.validate(dataStart, dataEnd); err != nil {
			return fmt.Errorf("track %d (%s): %w", i+1, t.FourCC, err)
		}
	}
	return nil
}

func (t *TrackState) validate(dataStart, dataEnd int64) error {
	if t.SampleCount == 0 && t.Handler == "vide" {
		return fmt.Errorf("%w: video track has no samples", ErrUnsupported)
	}
	if err := t.checkStscCoverage(); err != nil {
		return err
	}
	if n := len(t.Chunks); n > 0 && len(t.Stsc) > 0 && t.Stsc[len(t.Stsc)-1].FirstChunk > uint32(n) {
		return fmt.Errorf("%w: stsc extends past the %d chunks", ErrBadStructure, n)
	}
	if int64(len(t.SampleSizes)) != int64(t.SampleCount) {
		return fmt.Errorf("%w: stsz count mismatch", ErrBadStructure)
	}
	if t.SampleCount != 0 {
		var sttsN uint32
		for _, e := range t.Stts {
			sttsN += e.Count
		}
		if sttsN != t.SampleCount {
			return fmt.Errorf("%w: stts covers %d samples, stsz has %d", ErrBadStructure, sttsN, t.SampleCount)
		}
	}
	if len(t.SyncSamples) > 0 {
		prev := uint32(0)
		for _, s := range t.SyncSamples {
			if s == 0 || s > t.SampleCount || s <= prev {
				return fmt.Errorf("%w: bad sync sample table", ErrBadStructure)
			}
			prev = s
		}
	}
	// Every chunk must lie inside the media data, in ascending order.
	var pos int64 = -1
	for _, off := range t.Chunks {
		if int64(off) < pos {
			return fmt.Errorf("%w: chunk offsets not ascending", ErrBadStructure)
		}
		pos = int64(off)
		if int64(off) < dataStart || int64(off) > dataEnd {
			return fmt.Errorf("%w: chunk offset %d outside mdat [%d,%d)", ErrBadStructure, off, dataStart, dataEnd)
		}
	}
	// A track holding more bytes than the whole mdat is certainly broken
	// (mdat holds every track, so this is a sound upper bound).
	var total uint64
	for _, s := range t.SampleSizes {
		total += uint64(s)
	}
	if total > uint64(dataEnd-dataStart) {
		return fmt.Errorf("%w: sample data larger than mdat", ErrBadStructure)
	}
	return nil
}

// chunkCount is authoritative from stco (one offset per chunk). stsc is only a
// run-length description of per-chunk sample counts and cannot, on its own,
// determine how many chunks exist.
func (t *TrackState) chunkCount() int {
	return len(t.Chunks)
}

func (t *TrackState) checkStscCoverage() error {
	if t.SampleCount == 0 {
		return nil
	}
	if len(t.Stsc) == 0 {
		return fmt.Errorf("%w: no chunk structure (stsc)", ErrBadStructure)
	}
	if t.Stsc[0].FirstChunk != 1 {
		return fmt.Errorf("%w: stsc does not start at chunk 1", ErrBadStructure)
	}
	// An stsc entry covers chunks [FirstChunk, next.FirstChunk) — or, for the
	// last entry, [FirstChunk, NumChunks+1) so it reaches the final chunk.
	var samples uint32
	for i, e := range t.Stsc {
		end := uint32(len(t.Chunks)) + 1
		if i+1 < len(t.Stsc) {
			end = t.Stsc[i+1].FirstChunk
		}
		if end <= e.FirstChunk {
			return fmt.Errorf("%w: stsc entries out of order", ErrBadStructure)
		}
		if e.SamplesPerChunk == 0 {
			return fmt.Errorf("%w: stsc entry with zero samples", ErrBadStructure)
		}
		samples += e.SamplesPerChunk * (end - e.FirstChunk)
	}
	if samples != t.SampleCount {
		return fmt.Errorf("%w: stsc covers %d samples, stsz has %d", ErrBadStructure, samples, t.SampleCount)
	}
	return nil
}

// TotalDuration is the sum of all sample durations (timescale units).
func (t *TrackState) TotalDuration() uint64 {
	var d uint64
	for _, e := range t.Stts {
		d += uint64(e.Count) * uint64(e.Delta)
	}
	return d
}

// SamplesPerChunk expands stsc into a per-chunk sample count table.
func (t *TrackState) SamplesPerChunk() ([]uint32, error) {
	n := t.chunkCount()
	out := make([]uint32, n)
	for i, e := range t.Stsc {
		// Number of consecutive chunks this entry governs.
		count := int(uint32(n) - e.FirstChunk + 1)
		if i+1 < len(t.Stsc) {
			count = int(t.Stsc[i+1].FirstChunk - e.FirstChunk)
		}
		for k := 0; k < count; k++ {
			out[int(e.FirstChunk-1)+k] = e.SamplesPerChunk
		}
	}
	return out, nil
}

// ChunkByteSizes sums sample sizes per chunk.
func (t *TrackState) ChunkByteSizes() ([]uint32, error) {
	spc, err := t.SamplesPerChunk()
	if err != nil {
		return nil, err
	}
	out := make([]uint32, len(spc))
	s := 0
	for c, cnt := range spc {
		for k := uint32(0); k < cnt; k++ {
			out[c] += t.SampleSizes[s]
			s++
		}
	}
	return out, nil
}
