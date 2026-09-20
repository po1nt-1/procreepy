package video

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"

	"procreepy/internal/mp4"
	"procreepy/internal/procreate"
)

// List reports the timelapse segments (in playback order). The report goes to
// stdout via the returned string; warnings go to stderr as they occur.
func List(ctx context.Context, log *slog.Logger, inputArg string, cfg Config) (string, error) {
	in, err := resolveInput(inputArg, cfg)
	if err != nil {
		return "", err
	}
	defer in.close()
	arch, err := procreate.Open(in.path, in.label)
	if err != nil {
		return "", err
	}
	defer arch.Close()
	segs, err := arch.Segments(procreate.Options{AllowEmpty: true, Warn: warnFn(log, ctx)})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "input: %s\nsegments: %d\n", in.label, len(segs))
	if len(segs) > 0 {
		sb.WriteString("\n")
		width := len(strconv.Itoa(segs[len(segs)-1].Number))
		for _, s := range segs {
			fmt.Fprintf(&sb, "%-*d %s\n", width, s.Number, s.Name)
		}
	}
	if len(segs) == 0 {
		return sb.String(), &procreate.NoSegmentsError{Msg: "no video/segments in this archive"}
	}
	return sb.String(), nil
}

// Verify parses every segment and reports its health. Like List, the report
// is returned for the caller to print on stdout.
func Verify(ctx context.Context, log *slog.Logger, inputArg string, cfg Config) (string, error) {
	in, err := resolveInput(inputArg, cfg)
	if err != nil {
		return "", err
	}
	defer in.close()
	arch, err := procreate.Open(in.path, in.label)
	if err != nil {
		return "", err
	}
	defer arch.Close()
	segs, err := arch.Segments(procreate.Options{Strict: cfg.Strict, Warn: warnFn(log, ctx)})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "input: %s\nsegments: %d\n\n", in.label, len(segs))
	width := len(strconv.Itoa(segs[len(segs)-1].Number))
	var goodSegs []procreate.Segment
	var good []*mp4.Movie
	total := 0.0
	failed := 0
	for _, seg := range segs {
		if err := ctxErr(ctx); err != nil {
			return sb.String(), err
		}
		m, perr := arch.ParseSegment(seg.Name)
		if perr != nil {
			failed++
			fmt.Fprintf(&sb, "%-*d FAIL  %s\n", width, seg.Number, firstLine(perr.Error()))
			continue
		}
		goodSegs = append(goodSegs, seg)
		good = append(good, m)
		if m.Mvhd.Timescale != 0 {
			total += float64(m.Mvhd.Duration) / float64(m.Mvhd.Timescale)
		}
		dur := "n/a"
		if m.Mvhd.Timescale != 0 {
			dur = fmt.Sprintf("%.2fs", float64(m.Mvhd.Duration)/float64(m.Mvhd.Timescale))
		}
		fmt.Fprintf(&sb, "%-*d ok    %s  %s  %s\n", width, seg.Number,
			m.StreamsSummary(), dur, seg.Name)
	}
	if failed > 0 {
		return sb.String(), &procreate.BadSegmentError{Msg: fmt.Sprintf(
			"verify failed: %d of %d segment(s) are bad", failed, len(segs))}
	}

	if err := checkCompatibility(goodSegs, good); err != nil {
		return sb.String(), err
	}
	fmt.Fprintf(&sb, "\nverify: ok, %d segment(s), ~%.1f s of video", len(good), total)
	return sb.String(), nil
}

// PrintReport writes a --list/--verify report to stdout. A downstream reader
// that went away (e.g. `--list | head`) is reported like any other broken
// pipe.
func PrintReport(s string) error {
	if _, err := os.Stdout.WriteString(s); err != nil {
		devnullStdout()
		if errIs(err, syscall.EPIPE) {
			return &WriteError{Msg: "failed to write to stdout: broken pipe"}
		}
		return &WriteError{Msg: "failed to write the output: " + strerror(err)}
	}
	return nil
}
