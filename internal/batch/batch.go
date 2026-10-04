// Package batch turns a directory of .procreate files into separate trees of
// timelapses, re-importable projects and (optionally) PSD exports. One broken
// file never stops the run: failures are collected and reported at the end, and
// the exit code is non-zero if any file failed.
package batch

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"procreepy/internal/procreate"
	"procreepy/internal/video"
)

func isProcreate(name string) bool {
	// Dot-files are skipped on purpose: macOS leaves "._Foo.procreate"
	// resource forks behind when files are copied around, and they are not
	// ZIP archives.
	return !strings.HasPrefix(name, ".") && strings.HasSuffix(strings.ToLower(name), procreateExt)
}

// Discover lists the candidate archives below root. Without recursive only
// regular files directly in root count; with it, sub-directories are walked
// (depth-first, siblings alphabetical, dot-directories pruned). Scandir
// errors are swallowed in recursive mode, like os.walk.
func Discover(root string, recursive bool) ([]string, error) {
	if !recursive {
		ents, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		var found []string
		for _, e := range ents { // ReadDir results are sorted by name
			if e.IsDir() || !isProcreate(e.Name()) {
				continue
			}
			p := filepath.Join(root, e.Name())
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				found = append(found, p)
			}
		}
		return found, nil
	}
	var found []string
	var walk func(dir string)
	walk = func(dir string) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		var files, dirs []string
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			st, err := os.Stat(p) // follows symlinks, like os.walk
			if err != nil {
				continue
			}
			if st.IsDir() {
				dirs = append(dirs, p)
			} else if isProcreate(e.Name()) {
				files = append(files, p)
			}
		}
		sort.Strings(files)
		sort.Strings(dirs)
		found = append(found, files...)
		for _, d := range dirs {
			walk(d)
		}
	}
	walk(root)
	return found, nil
}

// ConvertDirectory converts every .procreate in inDir (optionally recursive).
// Each input yields an MP4 under OUTPUT/timelapses, a video-less project under
// OUTPUT/projects and, with cfg.PSD, a layered export under OUTPUT/psd. It
// returns the exit code (0, or 1 when any file failed) and an error for
// run-aborting problems (bad command line, unwritable output root).
func ConvertDirectory(ctx context.Context, log *slog.Logger, inDir, outputArg string,
	cfg video.Config, force, recursive bool) (int, error) {

	if outputArg == "-" {
		return 0, &video.UsageError{Msg: "cannot write a directory of results to stdout; give an " +
			"output directory (default: " + DefaultOutputDir + "/)"}
	}
	outDir := outputArg
	if outDir == "" {
		outDir = DefaultOutputDir
	}
	if st, err := os.Stat(outDir); err == nil && !st.IsDir() {
		return 0, &video.UsageError{Msg: "output path exists and is not a directory: " + outDir}
	}
	if err := checkOutputNesting(inDir, outDir, recursive); err != nil {
		return 0, &video.UsageError{Msg: err.Error()}
	}
	files, err := Discover(inDir, recursive)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		hint := ""
		if !recursive {
			hint = " (use -r to look in sub-directories)"
		}
		return 0, &procreate.InputError{Msg: "no .procreate files found in " + inDir + hint}
	}
	roots := NewRoots(outDir, cfg.PSD)
	for _, d := range []string{roots.Timelapses, roots.Projects, roots.PSD} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return 0, &video.WriteError{Msg: "cannot create " + d + ": " + strerror(err)}
		}
	}

	items := Plan(files, inDir, roots, recursive)
	log.InfoContext(ctx, "batch conversion started", "files", len(items), "input", withSlash(inDir),
		"timelapses", withSlash(roots.Timelapses), "projects", withSlash(roots.Projects))

	converted, existed, noVideo, failed := 0, 0, 0, 0
	for _, it := range items {
		if cerr := ctx.Err(); cerr != nil {
			return 0, cerr
		}
		if !force {
			switch outputState(it.Targets) {
			case allPresent:
				log.InfoContext(ctx, "skipped, outputs already exist (use --force to overwrite)",
					"input", it.Src, "timelapse", it.Targets.Timelapse, "project", it.Targets.Project)
				existed++
				continue
			case somePresent:
				// A half-finished earlier run: regenerating the whole set is the
				// only way back to a consistent pair, so say so rather than
				// skipping an input whose project is missing.
				log.WarnContext(ctx, "outputs are incomplete, regenerating the whole set",
					"input", it.Src)
			}
		}
		res, err := video.ConvertItem(ctx, log, it.Src, video.Targets(it.Targets), cfg)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return 0, cerr
			}
			if errors.Is(err, context.Canceled) {
				return 0, err
			}
			if errors.Is(err, procreate.ErrNoSegments) {
				log.WarnContext(ctx, "no timelapse video inside, skipped", "input", it.Src)
				noVideo++
			} else {
				log.ErrorContext(ctx, "file conversion failed", "input", it.Src, "err", err)
				failed++
			}
			continue
		}
		log.InfoContext(ctx, "converted", "input", it.Src, "timelapse", it.Targets.Timelapse,
			"project", it.Targets.Project, "removed_segments", res.Removed,
			"video_size", video.HumanBytes(res.RemovedBytes))
		if res.PSDWritten {
			log.InfoContext(ctx, "psd exported", "input", it.Src, "psd", it.Targets.PSD,
				"layers", res.Layers)
		}
		converted++
	}

	log.InfoContext(ctx, "batch completed", "converted", converted, "existed", existed,
		"no_video", noVideo, "failed", failed)
	if failed > 0 {
		return 1, nil
	}
	return 0, nil
}

// presence describes how much of one input's output set is already on disk.
type presence int

const (
	nonePresent presence = iota
	somePresent
	allPresent
)

// outputState reports the state of a whole output set rather than of a single
// file: an input whose MP4 survived but whose project did not is not done, and
// treating it as done is how a half-converted directory stays half-converted.
func outputState(t Targets) presence {
	want := t.all()
	have := 0
	for _, p := range want {
		if _, err := os.Stat(p); err == nil {
			have++
		}
	}
	switch have {
	case 0:
		return nonePresent
	case len(want):
		return allPresent
	default:
		return somePresent
	}
}

// withSlash renders a directory with a trailing separator, without doubling one
// the user already typed.
func withSlash(dir string) string {
	sep := string(os.PathSeparator)
	if strings.HasSuffix(dir, sep) || strings.HasSuffix(dir, "/") {
		return dir
	}
	return dir + sep
}

// DiagnoseDirectory runs --list / --verify (via action) over every
// .procreate file in inDir. Reports go to stdout; warnings and errors go to
// the log. It returns the exit code and an error for run-aborting problems
// (such as a broken pipe while printing a report).
func DiagnoseDirectory(log *slog.Logger, inDir string, recursive bool,
	action func(input string) (string, error)) (int, error) {

	files, err := Discover(inDir, recursive)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		hint := ""
		if !recursive {
			hint = " (use -r to look in sub-directories)"
		}
		return 0, &procreate.InputError{Msg: "no .procreate files found in " + inDir + hint}
	}
	failed := 0
	for i, src := range files {
		if i > 0 {
			if err := video.PrintReport("\n"); err != nil {
				return 0, err
			}
		}
		report, aerr := action(src)
		if err := video.PrintReport(report); err != nil {
			return 0, err
		}
		if aerr != nil {
			if errors.Is(aerr, procreate.ErrNoSegments) {
				log.Warn("no timelapse video inside", "input", src)
			} else {
				log.Error("diagnosis failed", "input", src, "err", aerr)
				failed++
			}
		}
	}
	if failed > 0 {
		return 1, nil
	}
	return 0, nil
}

// strerror renders an OS error like strerror(3), capitalized.
func strerror(err error) string {
	var en syscall.Errno
	if errors.As(err, &en) && en != 0 {
		s := en.Error()
		if s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return err.Error()
}
