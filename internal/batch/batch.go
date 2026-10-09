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
// Each input yields an MP4 under OUTPUT/mp4, a video-less project under
// OUTPUT/procreate and, with cfg.PSD, a layered export under OUTPUT/psd. With
// zipProjects, a clean run then replaces the project directory with
// OUTPUT/procreate.zip, which is what gets transferred back to an iPad. It
// returns the exit code (0, or 1 when any file failed) and an error for
// run-aborting problems (bad command line, unwritable output root).
func ConvertDirectory(ctx context.Context, log *slog.Logger, inDir, outputArg string,
	cfg video.Config, force, recursive, zipProjects bool) (int, error) {

	if outputArg == "-" {
		return 0, &video.UsageError{Msg: "cannot write a directory of results to stdout; give an " +
			"output directory (default: " + DefaultOutputDir(inDir) + "/)"}
	}
	outDir := outputArg
	if outDir == "" {
		outDir = DefaultOutputDir(inDir)
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

	// When the projects live in an archive, the archive is where "already done"
	// is recorded: packing removes the directory, so a stat on the project path
	// would report every artwork as missing and rebuild all of them.
	onDisk := func(p string) bool { _, err := os.Stat(p); return err == nil }
	exists := onDisk
	if zipProjects {
		packed := projectZipPaths(outDir)
		exists = func(p string) bool { return onDisk(p) || packed[p] }
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
			switch outputState(it.Targets, exists) {
			case allPresent:
				log.InfoContext(ctx, "skipped, outputs already exist (use --force to overwrite)",
					"input", it.Src, "timelapse", it.Targets.Timelapse, "project", it.Targets.Project)
				existed++
				continue
			case somePresent:
				// An artwork recorded with the timelapse off is complete without a
				// video, so its set looks half-finished forever. Confirm that
				// against the archive before deciding, or every such artwork would
				// be rebuilt on every run.
				if onlyTimelapseMissing(it.Targets, exists) {
					if has, herr := procreate.HasTimelapse(it.Src); herr == nil && !has {
						log.InfoContext(ctx, "skipped, outputs already exist (use --force to overwrite)",
							"input", it.Src, "timelapse", it.Targets.Timelapse,
							"project", it.Targets.Project)
						existed++
						continue
					}
				}
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
			log.ErrorContext(ctx, "file conversion failed", "input", it.Src, "err", err)
			failed++
			continue
		}
		if res.NoTimelapse {
			// Not a failure and not a skip: the project (and the PSD, when asked
			// for) were written, there was simply no video to extract.
			log.InfoContext(ctx, "no timelapse inside, wrote the project without a video",
				"input", it.Src, "project", it.Targets.Project)
			noVideo++
		} else {
			log.InfoContext(ctx, "converted", "input", it.Src, "timelapse", it.Targets.Timelapse,
				"project", it.Targets.Project, "removed_segments", res.Removed,
				"video_size", video.HumanBytes(res.RemovedBytes))
		}
		if res.PSDWritten {
			log.InfoContext(ctx, "psd exported", "input", it.Src, "psd", it.Targets.PSD,
				"layers", res.Layers)
		}
		if !res.NoTimelapse {
			converted++
		}
	}

	// Pack only after a clean run. With failures on the board the directory is
	// the material for working out what went wrong and for resuming, and
	// replacing it with an archive would take that away.
	if zipProjects && failed == 0 {
		n, perr := packProjects(ctx, outDir)
		if perr != nil {
			if cerr := ctx.Err(); cerr != nil {
				return 0, cerr
			}
			return 0, &video.WriteError{Msg: "cannot pack the projects into " +
				filepath.Join(outDir, ProjectArchiveName) + ": " + strerror(perr)}
		}
		log.InfoContext(ctx, "projects packed for transfer",
			"archive", filepath.Join(outDir, ProjectArchiveName), "projects", n)
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
func outputState(t Targets, exists func(string) bool) presence {
	want := t.all()
	have := 0
	for _, p := range want {
		if exists(p) {
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

// onlyTimelapseMissing reports whether every target but the video is present.
// That is the shape a finished artwork without a recorded timelapse leaves
// behind, and the only case worth re-opening the archive to confirm.
func onlyTimelapseMissing(t Targets, exists func(string) bool) bool {
	if exists(t.Timelapse) {
		return false
	}
	for _, p := range t.all() {
		if p == t.Timelapse {
			continue
		}
		if !exists(p) {
			return false
		}
	}
	return true
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
