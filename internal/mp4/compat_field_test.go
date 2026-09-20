package mp4

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

// TestMergeRejectsSingleFieldDifference: each individual incompatibility must
// be rejected, and the error must name exactly the field that differs.
func TestMergeRejectsSingleFieldDifference(t *testing.T) {
	seg := buildTwoTrackMP4(t)
	cases := []struct {
		name    string
		mutate  func(*TrackState)
		wantErr string // DifferText portion
	}{
		{"handler", func(x *TrackState) { x.Handler = "soun" }, `fields "stream type", "handler" differ`},
		{"fourcc", func(x *TrackState) { x.FourCC = "avc3" }, `field "codec fourcc" differs`},
		{"width", func(x *TrackState) { x.Width = 322 }, `field "width" differs`},
		{"height", func(x *TrackState) { x.Height = 242 }, `field "height" differs`},
		{"sample rate", func(x *TrackState) { x.AudioRate = 48000 }, `field "sample rate" differs`},
		{"channels", func(x *TrackState) { x.AudioChans = 1 }, `field "channels" differs`},
		{"timescale", func(x *TrackState) { x.Timescale = 60 }, `field "timescale" differs`},
		{"config box", func(x *TrackState) { x.ConfigBox = "hvcC" }, `field "codec configuration" differs`},
		{"config bytes", func(x *TrackState) { x.ConfigData = []byte{1, 2, 4} }, `field "avcC" differs`},
		{"config missing", func(x *TrackState) { x.ConfigData = nil }, `field "avcC" differs`},
		{"matrix", func(x *TrackState) { x.Matrix[0] = 0x18000 }, `field "color matrix" differs`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m1, _ := parseSeg(t, seg)
			m2, _ := parseSeg(t, seg)
			tc.mutate(m2.Tracks[0])
			_, err := Merge([]*Movie{m1, m2})
			if !errors.Is(err, ErrIncompatible) {
				t.Fatalf("Merge: err = %v, want ErrIncompatible", err)
			}
			want := "segments are not stream-copy compatible: stream copy is impossible: " + tc.wantErr + " (segment 2)"
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err.Error(), want)
			}
		})
	}
	t.Run("mvhd timescale", func(t *testing.T) {
		m1, _ := parseSeg(t, seg)
		m2, _ := parseSeg(t, seg)
		m2.Mvhd.Timescale = 600
		_, err := Merge([]*Movie{m1, m2})
		if !errors.Is(err, ErrIncompatible) {
			t.Fatalf("Merge: err = %v, want ErrIncompatible", err)
		}
		want := `segments are not stream-copy compatible: stream copy is impossible: field "mvhd timescale" differs (segment 2)`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})
	t.Run("stream count", func(t *testing.T) {
		m1, _ := parseSeg(t, seg)
		m2, _ := parseSeg(t, seg)
		m2.Tracks = m2.Tracks[:1] // drop the audio track
		_, err := Merge([]*Movie{m1, m2})
		if !errors.Is(err, ErrIncompatible) {
			t.Fatalf("Merge: err = %v, want ErrIncompatible", err)
		}
		want := `segments are not stream-copy compatible: stream copy is impossible: field "stream count" differs (segment 2)`
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})
}

func parseSeg(t *testing.T, seg []byte) (*Movie, []byte) {
	t.Helper()
	m, err := Parse(bytes.NewReader(seg), int64(len(seg)))
	if err != nil {
		t.Fatalf("parse segment: %v", err)
	}
	return m, seg
}

// TestMergeAcceptsDifferentStss pins the product decision: the stss table is
// per-segment keyframe placement, not stream-copy compatibility, so segments
// that differ only in stss must merge.
func TestMergeAcceptsDifferentStss(t *testing.T) {
	segA := stssSegFile(t, nil)
	m1, _ := parseSeg(t, segA)
	segB := stssSegFile(t, []uint32{1})
	m2, _ := parseSeg(t, segB)
	if _, err := Merge([]*Movie{m1, m2}); err != nil {
		t.Fatalf("Merge with differing stss = %v, want success", err)
	}
}

// TestMergeAcceptsMatchingStss is the control: identical stss tables merge.
func TestMergeAcceptsMatchingStss(t *testing.T) {
	seg := stssSegFile(t, []uint32{1, 2})
	m1, _ := parseSeg(t, seg)
	m2, _ := parseSeg(t, seg)
	if _, err := Merge([]*Movie{m1, m2}); err != nil {
		t.Fatalf("Merge with matching stss: %v", err)
	}
}

// TestMergeMixedStss: the merged stss is the union of the segments' sync
// samples with shifted sample numbers; a segment without stss contributes
// all of its samples. The emitted file must parse and carry that table.
func TestMergeMixedStss(t *testing.T) {
	cases := []struct {
		name  string
		aStss []uint32 // nil = no stss box
		bStss []uint32
		want  []uint32
	}{
		{"none-then-table", nil, []uint32{1, 2}, []uint32{1, 2, 3, 4}},
		{"table-then-none", []uint32{1}, nil, []uint32{1, 3, 4}},
		{"table-then-none-all", []uint32{1, 2}, nil, []uint32{1, 2, 3, 4}},
		{"both", []uint32{1, 2}, []uint32{2}, []uint32{1, 2, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ab, bb := stssSegFile(t, tc.aStss), stssSegFile(t, tc.bStss)
			am, _ := parseSeg(t, ab)
			bm, _ := parseSeg(t, bb)
			mg, err := Merge([]*Movie{am, bm})
			if err != nil {
				t.Fatalf("Merge: %v", err)
			}
			if got := mg.tracks[0].sync; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("merged sync = %v, want %v", got, tc.want)
			}
			payloads := [][]byte{
				ab[am.Mdat[0].Start:am.Mdat[0].End],
				bb[bm.Mdat[0].Start:bm.Mdat[0].End],
			}
			out, err := mg.EmitToBytes(payloads)
			if err != nil {
				t.Fatalf("EmitToBytes: %v", err)
			}
			rm, err := Parse(bytes.NewReader(out), int64(len(out)))
			if err != nil {
				t.Fatalf("resulting MP4 is invalid: %v", err)
			}
			rt := rm.Tracks[0]
			if !rt.HasSyncTable {
				t.Fatal("resulting track lost its stss table")
			}
			if !reflect.DeepEqual(rt.SyncSamples, tc.want) {
				t.Errorf("resulting sync = %v, want %v", rt.SyncSamples, tc.want)
			}
			checkSyncTable(t, rt)
		})
	}
}

// checkSyncTable asserts the random-access invariants of a sync table:
// entries sorted, unique, and within the sample range.
func checkSyncTable(t *testing.T, tr *TrackState) {
	t.Helper()
	n := uint32(len(tr.SampleSizes))
	prev := uint32(0)
	for _, s := range tr.SyncSamples {
		if s < 1 || s > n || s <= prev {
			t.Errorf("sync sample %d out of range or order (samples=%d, prev=%d)", s, n, prev)
			break
		}
		prev = s
	}
}

// stssSegFile builds a video-only 2-sample segment with an optional stss.
func stssSegFile(t *testing.T, stss []uint32) []byte {
	t.Helper()
	sizes := []uint32{10, 12}
	sps, pps := baselineSPS(), []byte{0x27, 0x05, 0xeb}
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
		if len(stss) > 0 {
			kids = append(kids, EncSTSS(stss))
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
	out = append(out, NewBox("mdat", bytes.Repeat([]byte{0xab}, total))...)
	return out
}
