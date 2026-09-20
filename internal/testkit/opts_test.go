package testkit

import (
	"bytes"
	"testing"

	"procreepy/internal/mp4"
)

func parseSegment(t *testing.T, b []byte) *mp4.Movie {
	t.Helper()
	m, err := mp4.Parse(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("parse segment: %v", err)
	}
	return m
}

func TestSegmentStss(t *testing.T) {
	if v := parseSegment(t, Segment(320, 240, []uint32{10, 10}, []uint32{5})).Tracks[0]; v.HasSyncTable {
		t.Fatal("default segment unexpectedly has a sync table")
	}
	v := parseSegment(t, Segment(320, 240, []uint32{10, 10}, []uint32{5}, WithStss(1, 2))).Tracks[0]
	if !v.HasSyncTable || len(v.SyncSamples) != 2 || v.SyncSamples[0] != 1 || v.SyncSamples[1] != 2 {
		t.Fatalf("stss = %v has=%v, want [1 2] true", v.SyncSamples, v.HasSyncTable)
	}
}

func TestSegmentCodecConfig(t *testing.T) {
	base := parseSegment(t, Segment(320, 240, []uint32{10}, []uint32{5})).Tracks[0]
	alt := parseSegment(t, Segment(320, 240, []uint32{10}, []uint32{5},
		WithCodecConfig(BaselineSPS(), []byte{0x27, 0x05, 0xec}))).Tracks[0]
	if base.ConfigBox != "avcC" {
		t.Fatalf("ConfigBox = %q, want avcC", base.ConfigBox)
	}
	if bytes.Equal(base.ConfigData, alt.ConfigData) {
		t.Fatal("avcC payloads are identical, want them to differ")
	}
	if base.PixFmt != alt.PixFmt {
		t.Fatalf("pixfmt moved with the PPS: %q vs %q", base.PixFmt, alt.PixFmt)
	}
}

func TestSegmentMovieTimescale(t *testing.T) {
	m := parseSegment(t, Segment(320, 240, []uint32{10}, []uint32{5}, WithMovieTimescale(60)))
	if m.Mvhd.Timescale != 60 {
		t.Fatalf("mvhd timescale = %d, want 60", m.Mvhd.Timescale)
	}
}

func TestSegmentDeterministic(t *testing.T) {
	a := Segment(320, 240, []uint32{10, 10}, []uint32{5}, WithStss(1, 2), WithMovieTimescale(60))
	b := Segment(320, 240, []uint32{10, 10}, []uint32{5}, WithStss(1, 2), WithMovieTimescale(60))
	if !bytes.Equal(a, b) {
		t.Fatal("Segment output is not deterministic")
	}
}
