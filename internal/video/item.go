package video

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"procreepy/internal/mp4"
	"procreepy/internal/procreate"
	"procreepy/internal/psd"
	"procreepy/internal/silica"
)

// Targets are the output paths of one archive. PSD is empty when PSD export is
// off.
type Targets struct {
	Timelapse string
	Project   string
	PSD       string
}

// ItemResult reports what one archive produced.
type ItemResult struct {
	Segments        int
	DurationSeconds float64
	Kept            int   // members carried into the project
	Removed         int   // timelapse segments dropped
	RemovedBytes    int64 // compressed size of those segments
	Layers          int   // PSD layers written
	PSDWritten      bool
	// NoTimelapse records that the archive held no timelapse, so no video was
	// written. The project and the PSD still were.
	NoTimelapse bool
}

// ConvertItem produces every requested artifact for one .procreate file.
//
// All outputs are written to scratch files first and published only once the
// last one has succeeded, so a failure part-way through leaves no orphan MP4
// next to a missing project, and a caller that treats the returned error as
// "this input did not happen" is telling the truth.
func ConvertItem(ctx context.Context, log *slog.Logger, src string, tg Targets,
	cfg Config) (ItemResult, error) {

	var res ItemResult

	// The source timestamp has to be read before anything is written: it is what
	// the published project carries, so that re-importing into Procreate keeps
	// the artwork's place in the gallery's ordering.
	st, err := os.Stat(src)
	if err != nil {
		return res, &procreate.InputError{Msg: "cannot access input " + src + ": " + strerror(err)}
	}
	srcModTime := st.ModTime()
	// Windows records a creation time and Explorer sorts by it; where it cannot
	// be read, the modification time is the closest honest stand-in.
	srcCreateTime := createTimeOf(st)
	if srcCreateTime.IsZero() {
		srcCreateTime = srcModTime
	}

	arch, err := procreate.Open(src, src)
	if err != nil {
		return res, err
	}
	defer arch.Close()

	// An artwork drawn with timelapse recording off is not a failure here: the
	// project and the PSD are still worth producing, and refusing the whole
	// input would write nothing at all for it. Only the video is skipped. The
	// single-file path (Convert) still treats a missing timelapse as an error,
	// because there the video is the only thing asked for.
	segs, err := arch.Segments(procreate.Options{Strict: cfg.Strict, Warn: warnFn(log, ctx)})
	if err != nil {
		if !errors.Is(err, procreate.ErrNoSegments) {
			return res, err
		}
		res.NoTimelapse = true
	}

	var mg *mp4.Merged
	wantVideo := tg.Timelapse != "" && !res.NoTimelapse
	if wantVideo {
		res.Segments = len(segs)
		movies, err := parseSegments(ctx, arch, segs)
		if err != nil {
			return res, err
		}
		if err := checkCompatibility(segs, movies); err != nil {
			return res, err
		}
		mg, err = mp4.Merge(movies)
		if err != nil {
			if errors.Is(err, mp4.ErrIncompatible) {
				return res, &IncompatibleError{Msg: err.Error()}
			}
			return res, err
		}
		res.DurationSeconds = mg.DurationSeconds()
	}

	var pending []*staged
	defer func() { discardAll(pending) }()

	if wantVideo {
		video, err := stage(tg.Timelapse)
		if err != nil {
			return res, err
		}
		pending = append(pending, video)
		if _, err := emit(ctx, arch, segs, mg, video.f); err != nil {
			return res, classifyWrite(ctx, err)
		}
	}

	project, err := stage(tg.Project)
	if err != nil {
		return res, err
	}
	pending = append(pending, project)
	slim, err := arch.WriteSlimmed(ctx, project.f)
	if err != nil {
		if ctxErr(ctx) != nil {
			return res, ctx.Err()
		}
		return res, &WriteError{Msg: "cannot write " + tg.Project + ": " + strerror(err)}
	}
	res.Kept, res.Removed, res.RemovedBytes = slim.Kept, slim.Removed, slim.RemovedBytes

	if tg.PSD != "" {
		doc, err := silica.ReadDocument(arch)
		if err != nil {
			return res, &WriteError{Msg: "cannot export " + tg.PSD + ": " + strerror(err)}
		}
		sheet, err := stage(tg.PSD)
		if err != nil {
			return res, err
		}
		pending = append(pending, sheet)
		n, err := psd.Write(ctx, sheet.f, doc)
		if err != nil {
			if ctxErr(ctx) != nil {
				return res, ctx.Err()
			}
			return res, &WriteError{Msg: "cannot write " + tg.PSD + ": " + strerror(err)}
		}
		res.Layers, res.PSDWritten = n, true
	}

	// Every artifact stands for the same artwork, so every one of them carries
	// the source's dates: the project so that re-importing it keeps the gallery
	// order, and the video and the PSD so a folder of results sorts the way the
	// originals do instead of collapsing onto the moment of conversion.
	for _, s := range pending {
		s.setTimes(srcCreateTime, srcModTime)
		if err := s.publish(); err != nil {
			return res, err
		}
	}
	return res, nil
}

// parseSegments reads and parses every segment, checking for cancellation
// between them.
func parseSegments(ctx context.Context, arch *procreate.Archive, segs []procreate.Segment) ([]*mp4.Movie, error) {
	movies := make([]*mp4.Movie, len(segs))
	for i, seg := range segs {
		if err := ctxErr(ctx); err != nil {
			return nil, err
		}
		m, err := arch.ParseSegmentCtx(ctx, seg.Name)
		if err != nil {
			return nil, err
		}
		if !m.HasVideo() {
			return nil, &procreate.BadSegmentError{Msg: "segment " + seg.Name + " has no video stream"}
		}
		if len(m.Mdat) != 1 {
			return nil, fmt.Errorf("%w: segment %s has %d mdat boxes (want 1)",
				procreate.ErrBadSegment, seg.Name, len(m.Mdat))
		}
		movies[i] = m
	}
	return movies, nil
}
