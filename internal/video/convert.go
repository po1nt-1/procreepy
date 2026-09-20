package video

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"procreepy/internal/mp4"
	"procreepy/internal/procreate"
)

// warnFn adapts the procreate package's printf-style Warn callback to a slog
// logger, keeping the domain package logger-free.
func warnFn(log *slog.Logger, ctx context.Context) func(format string, args ...any) {
	return func(format string, args ...any) {
		log.WarnContext(ctx, fmt.Sprintf(format, args...))
	}
}

// Convert joins the archive's timelapse segments into one moov-first MP4 and
// writes it to out (stdout or a file). With cfg.Split it additionally writes
// a video-less copy of the archive (.procreepy.procreate) beside the MP4.
// It returns the total duration in seconds.
func Convert(ctx context.Context, log *slog.Logger, inputArg string, out Output, cfg Config, chatty bool) (float64, error) {
	if err := out.prepare(inputArg); err != nil {
		return 0, err
	}
	if cfg.Split && out.Kind != OutFile {
		return 0, &UsageError{Msg: "--split writes a slimmed .procreepy.procreate, so it needs a " +
			"file or directory OUTPUT, not stdout"}
	}
	in, err := resolveInput(inputArg, cfg)
	if err != nil {
		return 0, err
	}
	defer in.close()

	arch, err := procreate.Open(in.path, in.label)
	if err != nil {
		return 0, err
	}
	defer arch.Close()

	segs, err := arch.Segments(procreate.Options{Strict: cfg.Strict, Warn: warnFn(log, ctx)})
	if err != nil {
		return 0, err
	}
	if chatty {
		log.InfoContext(ctx, "segments found", "input", in.label, "count", len(segs))
	}

	movies := make([]*mp4.Movie, len(segs))
	for i, seg := range segs {
		if err := ctxErr(ctx); err != nil {
			return 0, err
		}
		m, err := arch.ParseSegment(seg.Name)
		if err != nil {
			return 0, err
		}
		if !m.HasVideo() {
			return 0, &procreate.BadSegmentError{Msg: "segment " + seg.Name + " has no video stream"}
		}
		if len(m.Mdat) != 1 {
			return 0, fmt.Errorf("%w: segment %s has %d mdat boxes (want 1)",
				procreate.ErrBadSegment, seg.Name, len(m.Mdat))
		}
		movies[i] = m
	}
	if err := checkCompatibility(segs, movies); err != nil {
		return 0, err
	}

	mg, err := mp4.Merge(movies)
	if err != nil {
		if errors.Is(err, mp4.ErrIncompatible) {
			return 0, &IncompatibleError{Msg: err.Error()}
		}
		return 0, err
	}

	if chatty {
		log.InfoContext(ctx, "joining segments (stream copy)")
	}

	destTxt, err := writeOut(ctx, arch, segs, mg, out)
	if err != nil {
		return 0, err
	}
	if cfg.Split {
		slim := slimPath(out.Path)
		removed, removedBytes, err := procreate.SplitTimelapse(in.path, slim)
		if err != nil {
			return 0, &WriteError{Msg: "cannot write slimmed archive " + slim + ": " + strerror(err)}
		}
		log.InfoContext(ctx, "slimmed archive written", "path", slim, "removed_files", removed,
			"video_size", humanBytes(removedBytes))
	}
	if chatty {
		log.InfoContext(ctx, "conversion completed", "output", destTxt, "duration_s", mg.DurationSeconds())
	}
	return mg.DurationSeconds(), nil
}

// humanBytes renders a byte count like "3.4 MiB" (one decimal below 100).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	s := float64(n) / float64(div)
	if s < 100 {
		return fmt.Sprintf("%.1f %ciB", s, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%d %ciB", int64(s), "KMGTPE"[exp])
}

// checkCompatibility raises IncompatibleError with the original tool's
// diagnostic when any segment differs from the first one.
func checkCompatibility(segs []procreate.Segment, movies []*mp4.Movie) error {
	if len(movies) < 2 {
		return nil
	}
	ref := movies[0]
	var bad []int
	for i := 1; i < len(movies); i++ {
		if !mp4.SameStreams(ref, movies[i]) || movies[i].Mvhd.Timescale != ref.Mvhd.Timescale {
			bad = append(bad, i)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("  %s: %s", segs[0].Name, ref.StreamsSummary())}
	for i := 0; i < len(bad) && i < 5; i++ {
		lines = append(lines, fmt.Sprintf("  %s: %s", segs[bad[i]].Name, movies[bad[i]].StreamsSummary()))
	}
	if len(bad) > 5 {
		lines = append(lines, fmt.Sprintf("  ... and %d more", len(bad)-5))
	}
	return &IncompatibleError{Msg: "segments have different stream parameters, so they cannot be " +
		"joined with stream copy (-c copy):\n" + strings.Join(lines, "\n")}
}

// writeOut emits ftyp+moov+mdat(s) to the resolved destination.
func writeOut(ctx context.Context, arch *procreate.Archive, segs []procreate.Segment,
	mg *mp4.Merged, out Output) (string, error) {

	switch out.Kind {
	case OutStdout:
		signal.Ignore(syscall.SIGPIPE)
		if _, err := emit(ctx, arch, segs, mg, os.Stdout); err != nil {
			devnullStdout()
			if errIs(err, syscall.EPIPE) {
				return "", &WriteError{Msg: "failed to write to stdout: broken pipe"}
			}
			return "", &WriteError{Msg: "failed to write the output: " + strerror(err)}
		}
		return "stdout", nil

	case OutDevice:
		f, err := os.OpenFile(out.Path, os.O_WRONLY, 0)
		if err != nil {
			return "", &WriteError{Msg: "cannot write " + out.Path + ": " + strerror(err)}
		}
		_, err = emit(ctx, arch, segs, mg, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return "", classifyWrite(ctx, err)
		}
		return out.Name, nil

	default: // OutFile: write a sibling .partial and rename.
		partial := out.partialName()
		f, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return "", &WriteError{Msg: "cannot write " + out.Path + ": " + strerror(err)}
		}
		_, err = emit(ctx, arch, segs, mg, f)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(partial)
			return "", classifyWrite(ctx, err)
		}
		if err := os.Rename(partial, out.Path); err != nil {
			os.Remove(partial)
			return "", &WriteError{Msg: "cannot write " + out.Path + ": " + strerror(err)}
		}
		return out.Name, nil
	}
}

// classifyWrite turns a mid-stream failure into the right user-facing error.
// Context cancellation is surfaced as-is so the CLI can report "interrupted".
func classifyWrite(ctx context.Context, err error) error {
	if ctxErr(ctx) != nil {
		return ctx.Err()
	}
	if errIs(err, syscall.EPIPE) {
		return &WriteError{Msg: "failed to write to stdout: broken pipe"}
	}
	return &WriteError{Msg: "failed to write the output: " + strerror(err)}
}

// emit writes ftyp, moov, then one mdat box per segment whose payload is
// streamed straight out of the archive (CRC verified).
func emit(ctx context.Context, arch *procreate.Archive, segs []procreate.Segment,
	mg *mp4.Merged, w io.Writer) (int64, error) {

	var n int64
	write := func(b []byte) error {
		k, err := w.Write(b)
		n += int64(k)
		return err
	}
	if err := write(mg.Ftyp()); err != nil {
		return n, err
	}
	if err := write(mg.Moov()); err != nil {
		return n, err
	}
	for k, seg := range segs {
		size := mg.MdatPayloadSize(k)
		head, err := mp4.MdatHead(size)
		if err != nil {
			return n, err
		}
		if err := write(head); err != nil {
			return n, err
		}
		m, err := arch.StreamMdatCtx(ctx, seg.Name, mg.MdatSourceStart(k), size, w)
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
