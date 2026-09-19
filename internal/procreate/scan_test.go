package procreate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func ms(names ...string) []Member {
	out := make([]Member, len(names))
	for i, n := range names {
		out[i] = Member{Name: n}
	}
	return out
}

func TestScanNumericOrder(t *testing.T) {
	f, err := Scan(ms(
		"video/segments/segment-10.mp4",
		"video/segments/segment-2.mp4",
		"video/segments/segment-1.mp4",
	))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var nums []int
	for _, s := range f.Segments {
		nums = append(nums, s.Number)
	}
	if !reflect.DeepEqual(nums, []int{1, 2, 10}) {
		t.Fatalf("order = %v, want [1 2 10]", nums)
	}
}

func TestScanCaseInsensitive(t *testing.T) {
	f, err := Scan(ms("VIDEO/SEGMENTS/Segment-3.MP4"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(f.Segments) != 1 || f.Segments[0].Number != 3 {
		t.Fatalf("segments = %+v, want [3]", f.Segments)
	}
	if f.Segments[0].Name != "VIDEO/SEGMENTS/Segment-3.MP4" {
		t.Fatalf("name = %q, want original case preserved", f.Segments[0].Name)
	}
}

func TestScanScope(t *testing.T) {
	f, err := Scan(ms(
		"video/segments/segment-1.mp4",
		"video/segments/outtake.mp4", // .mp4, wrong name -> ignored
		"video/segments/thumb.png",   // not .mp4 -> untouched
		"other/segment-9.mp4",        // outside the dir -> untouched
		"video/segments/",            // directory entry
	))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(f.Segments) != 1 || f.Segments[0].Number != 1 {
		t.Fatalf("segments = %+v, want [1]", f.Segments)
	}
	if !reflect.DeepEqual(f.Ignored, []string{"video/segments/outtake.mp4"}) {
		t.Fatalf("ignored = %v", f.Ignored)
	}
}

func TestScanClash(t *testing.T) {
	_, err := Scan(ms("video/segments/segment-7.mp4", "video/segments/segment-07.mp4"))
	if !errors.Is(err, ErrBadSegment) {
		t.Fatalf("err = %v, want ErrBadSegment", err)
	}
	if !strings.Contains(err.Error(), "segment-7.mp4, video/segments/segment-07.mp4 all map to segment 7") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestScanClashReportsSmallest(t *testing.T) {
	_, err := Scan(ms(
		"video/segments/segment-3.mp4",
		"video/segments/Segment-3.MP4",
		"video/segments/segment-2.mp4",
		"video/segments/SEGMENT-2.MP4",
	))
	if !errors.Is(err, ErrBadSegment) {
		t.Fatalf("err = %v, want ErrBadSegment", err)
	}
	if !strings.Contains(err.Error(), "all map to segment 2") {
		t.Fatalf("message = %q, want the smallest clash (2)", err.Error())
	}
}

func TestFindSegmentsNone(t *testing.T) {
	_, err := FindSegments(ms("canvas/proj/project.dat"), Options{})
	if !errors.Is(err, ErrNoSegments) {
		t.Fatalf("err = %v, want ErrNoSegments", err)
	}
	if _, err := FindSegments(nil, Options{AllowEmpty: true}); err != nil {
		t.Fatalf("AllowEmpty: %v", err)
	}
}

func TestFindSegmentsIgnoredOnly(t *testing.T) {
	_, err := FindSegments(ms("video/segments/outtake.mp4"), Options{})
	if !errors.Is(err, ErrBadSegment) {
		t.Fatalf("err = %v, want ErrBadSegment", err)
	}
	if !strings.Contains(err.Error(), "order cannot be determined") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestFindSegmentsWarnings(t *testing.T) {
	var warns []string
	segs, err := FindSegments(ms(
		"video/segments/segment-1.mp4",
		"video/segments/outtake.mp4",
		"video/segments/segment-3.mp4",
	), Options{Warn: func(f string, a ...any) {
		warns = append(warns, fmt.Sprintf(f, a...))
	}})
	if err != nil {
		t.Fatalf("FindSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("segments = %+v", segs)
	}
	want := []string{
		"ignoring video/segments/outtake.mp4: name does not match segment-<number>.mp4",
		"segment numbers missing: 2 (the video would have gaps)",
	}
	if !reflect.DeepEqual(warns, want) {
		t.Fatalf("warnings = %q, want %q", warns, want)
	}
}

func TestFindSegmentsStrictGap(t *testing.T) {
	_, err := FindSegments(ms(
		"video/segments/segment-1.mp4",
		"video/segments/segment-3.mp4",
	), Options{Strict: true})
	if !errors.Is(err, ErrBadSegment) {
		t.Fatalf("err = %v, want ErrBadSegment", err)
	}
	if !strings.Contains(err.Error(), "segment numbers missing: 2") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestFormatRanges(t *testing.T) {
	if got := formatRanges([]int{1, 2, 3, 7, 9, 10}); got != "1-3, 7, 9-10" {
		t.Fatalf("formatRanges = %q", got)
	}
	if got := formatRanges([]int{5}); got != "5" {
		t.Fatalf("formatRanges = %q", got)
	}
	if got := formatRanges([]int{4, 7, 9}); got != "4, 7, 9" {
		t.Fatalf("formatRanges = %q", got)
	}
}
