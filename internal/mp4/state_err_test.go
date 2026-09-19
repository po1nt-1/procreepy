package mp4

import (
	"testing"
)

// A zero-sample audio track is legal (empty audio in some encodings); it must
// pass validation and exercise the early stsc-exempt return.
func TestTrackStateValidateZeroAudio(t *testing.T) {
	ts := &TrackState{Handler: "soun"}
	if err := ts.validate(0, 100); err != nil {
		t.Fatalf("zero-sample audio: %v", err)
	}
}

func TestChunkByteSizesRuns(t *testing.T) {
	ts := &TrackState{
		Chunks:      []uint64{0, 10},
		Stsc:        []StscEntry{{FirstChunk: 1, SamplesPerChunk: 2, Description: 1}},
		SampleSizes: []uint32{1, 2, 3, 4},
	}
	sizes, err := ts.ChunkByteSizes()
	if err != nil {
		t.Fatalf("ChunkByteSizes: %v", err)
	}
	if len(sizes) != 2 || sizes[0] != 3 || sizes[1] != 7 {
		t.Fatalf("sizes = %v, want [3 7]", sizes)
	}
}
