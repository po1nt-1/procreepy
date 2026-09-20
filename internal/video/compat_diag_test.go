package video

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"procreepy/internal/mp4"
	"procreepy/internal/procreate"
	"procreepy/internal/testkit"
)

var compatSegNames = []string{
	"video/segments/segment-1.mp4",
	"video/segments/segment-2.mp4",
}

// incompatFor builds two segments (the second one with altOpts applied), runs
// checkCompatibility, and returns the typed error; it returns nil when the
// pair is compatible.
func incompatFor(t *testing.T, altOpts ...testkit.SegOpt) *IncompatibleError {
	t.Helper()
	segs := [][]byte{
		testkit.Segment(320, 240, []uint32{10, 10}, []uint32{5}),
		testkit.Segment(320, 240, []uint32{10, 10}, []uint32{5}, altOpts...),
	}
	movies := make([]*mp4.Movie, len(segs))
	ps := make([]procreate.Segment, len(segs))
	for i, s := range segs {
		m, err := mp4.Parse(bytes.NewReader(s), int64(len(s)))
		if err != nil {
			t.Fatalf("parse segment %d: %v", i+1, err)
		}
		movies[i] = m
		ps[i] = procreate.Segment{Number: i + 1, Name: compatSegNames[i]}
	}
	err := checkCompatibility(ps, movies)
	if err == nil {
		return nil
	}
	var ie *IncompatibleError
	if !errors.As(err, &ie) {
		t.Fatalf("checkCompatibility = %v, want IncompatibleError", err)
	}
	return ie
}

func TestIncompatibleNamesSegmentsAndFields(t *testing.T) {
	ie := incompatFor(t, testkit.WithCodecConfig(testkit.BaselineSPS(), []byte{0x27, 0x05, 0xec}))
	if ie == nil {
		t.Fatal("expected IncompatibleError for a PPS difference")
	}
	if want := []string{compatSegNames[0], compatSegNames[1]}; !reflect.DeepEqual(ie.Segments, want) {
		t.Errorf("Segments = %v, want %v", ie.Segments, want)
	}
	if want := []mp4.Field{"avcC"}; !reflect.DeepEqual(ie.Fields, want) {
		t.Errorf("Fields = %v, want %v", ie.Fields, want)
	}
	wantMsg := `incompatible segments "video/segments/segment-1.mp4" and "video/segments/segment-2.mp4": field "avcC" differs`
	if ie.Error() != wantMsg {
		t.Errorf("message = %q, want %q", ie.Error(), wantMsg)
	}
}

func TestIncompatibleNamesMultipleFields(t *testing.T) {
	ie := incompatFor(t,
		testkit.WithCodecConfig(testkit.BaselineSPS(), []byte{0x27, 0x05, 0xec}),
		testkit.WithMovieTimescale(600),
	)
	if ie == nil {
		t.Fatal("expected IncompatibleError")
	}
	if want := []mp4.Field{"avcC", "mvhd timescale"}; !reflect.DeepEqual(ie.Fields, want) {
		t.Errorf("Fields = %v, want %v", ie.Fields, want)
	}
	wantMsg := `incompatible segments "video/segments/segment-1.mp4" and "video/segments/segment-2.mp4": fields "avcC", "mvhd timescale" differ`
	if ie.Error() != wantMsg {
		t.Errorf("message = %q, want %q", ie.Error(), wantMsg)
	}
}

// TestStssDifferenceIsCompatible pins the product decision: an stss
// difference alone must not fail compatibility (real archives differ in
// keyframe placement between segments without any decode problem).
func TestStssDifferenceIsCompatible(t *testing.T) {
	if ie := incompatFor(t, testkit.WithStss(1, 2)); ie != nil {
		t.Fatalf("checkCompatibility with an stss-only difference = %v, want nil", ie)
	}
}

// Previously, different differences produced byte-identical messages, so a
// user could not tell what actually differed. Different differences must now
// be distinguishable.
func TestBlindSpotClosed(t *testing.T) {
	a := incompatFor(t, testkit.WithCodecConfig(testkit.BaselineSPS(), []byte{0x27, 0x05, 0xec}))
	b := incompatFor(t, testkit.WithMovieTimescale(600))
	if a == nil || b == nil {
		t.Fatalf("expected two incompatibilities, got %v / %v", a, b)
	}
	if a.Error() == b.Error() {
		t.Fatalf("indistinguishable messages for different differences:\n%s", a.Error())
	}
}

// An incompatibility is decided before the output is touched, so an
// unwritable target must not shadow the IncompatibleError.
func TestIncompatibleBeatsUnwritableTarget(t *testing.T) {
	targetDir := t.TempDir()
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{10, 10}, []uint32{5}),
		"video/segments/segment-2.mp4": testkit.Segment(320, 240, []uint32{10, 10}, []uint32{5},
			testkit.WithCodecConfig(testkit.BaselineSPS(), []byte{0x27, 0x05, 0xec})),
	}
	in := testkit.WriteArchive(t, entries, false)
	var inc *IncompatibleError
	_, err := Convert(context.Background(), discardLog(), in,
		Output{Kind: OutFile, Path: targetDir, Name: targetDir}, Config{}, false)
	if !errors.As(err, &inc) {
		t.Fatalf("err = %v, want IncompatibleError", err)
	}
}
