package mp4

import (
	"bytes"
	"testing"
)

// FuzzParseMP4 drives the parser with arbitrary bytes. The contract:
// malformed input yields an error (never a panic), and a successfully parsed
// movie keeps its structural invariants intact.
func FuzzParseMP4(f *testing.F) {
	good := buildSegFile(320, 240, []uint32{10, 20}, []uint32{50})
	seeds := [][]byte{
		good,
		buildSegFile(16, 16, []uint32{1}, []uint32{1}),
		good[:len(good)/2],          // truncated mid-moov
		good[:8],                    // one truncated box header
		bytes.Repeat([]byte{0}, 64), // all zeros
		{0, 0, 0, 1, 'm', 'd', 'a', 't', 0, 0, 0, 0, 0, 0, 0, 1}, // mdat, largesize=1
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := Parse(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		// Successful parse: invariants that Validate alone does not spell out.
		if m.Mvhd.Timescale == 0 {
			t.Fatal("parsed movie with zero movie timescale")
		}
		for _, tr := range m.Tracks {
			if tr.Timescale == 0 {
				t.Fatal("parsed track with zero timescale")
			}
			if uint32(len(tr.SampleSizes)) != tr.SampleCount {
				t.Fatalf("sample table mismatch: %d sizes for %d samples",
					len(tr.SampleSizes), tr.SampleCount)
			}
			if uint64(len(tr.Chunks)) > 1<<24 {
				t.Fatalf("implausible chunk count %d", len(tr.Chunks))
			}
		}
		for i, r := range m.Mdat {
			if r.Start < 0 || r.End > int64(len(data)) || r.End < r.Start {
				t.Fatalf("mdat[%d] region [%d,%d) outside a %d-byte file",
					i, r.Start, r.End, len(data))
			}
		}
	})
}
