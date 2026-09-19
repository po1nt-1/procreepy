package procreate

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SegmentDir is the archive directory that holds time-lapse segments.
const SegmentDir = "video/segments/"

var segmentRE = regexp.MustCompile(`(?i)^video/segments/segment-([0-9]+)\.mp4$`)

// Member is one ZIP archive entry, as seen by the scanner.
type Member struct {
	Name  string // member name as stored in the archive
	IsDir bool
}

// Segment is one discovered time-lapse segment.
type Segment struct {
	Number int
	Name   string // member name as stored in the archive
}

// Findings is the raw result of Scan.
type Findings struct {
	Segments []Segment
	Ignored  []string // .mp4 members under video/segments that do not match
}

// Scan finds video/segments/segment-N.mp4 members sorted by N numerically.
// (Lexicographic order would put segment-10 before segment-2, which silently
// scrambles the timelapse.) Clashing numbers are an error.
func Scan(members []Member) (Findings, error) {
	byNumber := make(map[int][]string)
	var ignored []string
	dirPrefix := strings.ToLower(SegmentDir)
	for _, m := range members {
		name := m.Name
		lower := strings.ToLower(name)
		if m.IsDir || !strings.HasPrefix(lower, dirPrefix) {
			continue
		}
		if mt := segmentRE.FindStringSubmatch(name); mt != nil {
			if n, err := strconv.Atoi(mt[1]); err == nil {
				byNumber[n] = append(byNumber[n], name)
				continue
			}
		}
		if strings.HasSuffix(lower, ".mp4") {
			ignored = append(ignored, name)
		}
	}
	for _, n := range clashNumbers(byNumber) {
		return Findings{}, fmt.Errorf(
			"%w: ambiguous segment numbering: %s all map to segment %d",
			ErrBadSegment, strings.Join(byNumber[n], ", "), n)
	}
	segs := make([]Segment, 0, len(byNumber))
	for n, names := range byNumber {
		segs = append(segs, Segment{Number: n, Name: names[0]})
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].Number < segs[j].Number })
	return Findings{Segments: segs, Ignored: ignored}, nil
}

// clashNumbers returns the segment numbers that map to more than one member,
// ascending, so the smallest clash is reported first.
func clashNumbers(byNumber map[int][]string) []int {
	var out []int
	for n, names := range byNumber {
		if len(names) > 1 {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// Options tunes FindSegments.
type Options struct {
	Strict     bool // missing segment numbers become an error
	AllowEmpty bool // an archive without segments is not an error
	Warn       func(format string, args ...any)
}

func (o Options) warnf(format string, args ...any) {
	if o.Warn != nil {
		o.Warn(format, args...)
	}
}

// FindSegments discovers ordered segments with diagnostics: non-matching
// .mp4 files are warned about, gaps in the numbering are warned about (or
// fatal with Strict), and an absent segment set is an error unless AllowEmpty.
func FindSegments(members []Member, opts Options) ([]Segment, error) {
	f, err := Scan(members)
	if err != nil {
		return nil, err
	}
	for _, name := range f.Ignored {
		opts.warnf("ignoring %s: name does not match segment-<number>.mp4", name)
	}
	if len(f.Segments) == 0 {
		if opts.AllowEmpty {
			return nil, nil
		}
		if len(f.Ignored) > 0 {
			return nil, &BadSegmentError{Msg: fmt.Sprintf(
				"video/segments has %d .mp4 file(s), but none is named segment-<number>.mp4, so the order cannot be determined",
				len(f.Ignored))}
		}
		return nil, &NoSegmentsError{Msg: "no video/segments in this archive " +
			"(time-lapse recording was probably turned off for this artwork)"}
	}
	present := make(map[int]bool, len(f.Segments))
	for _, s := range f.Segments {
		present[s.Number] = true
	}
	lo, hi := 1, f.Segments[len(f.Segments)-1].Number
	if f.Segments[0].Number < lo {
		lo = f.Segments[0].Number
	}
	var missing []int
	for n := lo; n <= hi; n++ {
		if !present[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		msg := fmt.Sprintf("segment numbers missing: %s (the video would have gaps)", formatRanges(missing))
		if opts.Strict {
			return nil, &BadSegmentError{Msg: msg}
		}
		opts.warnf("%s", msg)
	}
	return f.Segments, nil
}

// formatRanges renders ascending integers as "1-3, 7, 9-10".
func formatRanges(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	parts := make([]string, 0, len(nums))
	start, prev := nums[0], nums[0]
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for i := 1; i < len(nums); i++ {
		if nums[i] == prev+1 {
			prev = nums[i]
			continue
		}
		flush()
		start, prev = nums[i], nums[i]
	}
	flush()
	return strings.Join(parts, ", ")
}
