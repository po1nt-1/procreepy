// Package batch turns a directory of .procreate files into a directory of
// MP4s. One broken file never stops the run: failures are collected and
// reported at the end, and the exit code is non-zero if any file failed.
package batch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"procreepy/internal/procreate"
	"procreepy/internal/video"
)

// DefaultOutputDir is where batch mode writes when no OUTPUT is given.
const DefaultOutputDir = "output" + string(os.PathSeparator) + "timelaps"

// Pair maps one input archive to its output file.
type Pair struct{ Src, Dst string }

func isProcreate(name string) bool {
	// Dot-files are skipped on purpose: macOS leaves "._Foo.procreate"
	// resource forks behind when files are copied around, and they are not
	// ZIP archives.
	return !strings.HasPrefix(name, ".") && strings.HasSuffix(strings.ToLower(name), ".procreate")
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

func deriveName(input string) string {
	b := filepath.Base(input)
	i := strings.LastIndex(b, ".")
	if i <= 0 {
		return b + ".mp4"
	}
	return b[:i] + ".mp4"
}

func unique(dst string, used map[string]bool) string {
	cand, n := dst, 2
	base := filepath.Base(dst)
	i := strings.LastIndex(base, ".")
	stem, suf := base, ""
	if i > 0 {
		stem, suf = base[:i], base[i:]
	}
	for used[cand] {
		cand = filepath.Join(filepath.Dir(dst), fmt.Sprintf("%s-%d%s", stem, n, suf))
		n++
	}
	return cand
}

// PlanOutputs maps every input to its output path. Sub-directories are
// mirrored with recursive, and a name clash (a.procreate vs a.PROCREATE)
// gets a -2, -3 ... suffix.
func PlanOutputs(files []string, root, outDir string, recursive bool) []Pair {
	used := make(map[string]bool, len(files))
	plan := make([]Pair, 0, len(files))
	for _, src := range files {
		dst := filepath.Join(outDir, deriveName(src))
		if recursive {
			if rel, err := filepath.Rel(root, src); err == nil {
				dst = filepath.Join(outDir, filepath.Dir(rel), deriveName(src))
			}
		}
		dst = unique(dst, used)
		used[dst] = true
		plan = append(plan, Pair{Src: src, Dst: dst})
	}
	return plan
}

// ConvertDirectory joins every .procreate in inDir (optionally recursive)
// into MP4s under outDir (DefaultOutputDir when outputArg is empty). It
// returns the exit code (0, or 1 when any file failed) and an error for
// run-aborting problems (bad command line, unwritable output dir).
func ConvertDirectory(ctx context.Context, log *slog.Logger, inDir, outputArg string,
	cfg video.Config, force, recursive bool) (int, error) {

	if outputArg == "-" {
		return 0, &video.UsageError{Msg: "cannot write several videos to stdout; give an output " +
			"directory (default: " + DefaultOutputDir + "/)"}
	}
	outDir := outputArg
	if outDir == "" {
		outDir = DefaultOutputDir
	}
	if st, err := os.Stat(outDir); err == nil && !st.IsDir() {
		return 0, &video.UsageError{Msg: "output path exists and is not a directory: " + outDir}
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
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, &video.WriteError{Msg: "cannot create " + outDir + ": " + strerror(err)}
	}

	plan := PlanOutputs(files, inDir, outDir, recursive)
	log.InfoContext(ctx, "batch conversion started", "files", len(plan), "input", inDir, "output", outDir+"/")

	converted, existed, noVideo := 0, 0, 0
	failed := 0
	for _, p := range plan {
		if cerr := ctx.Err(); cerr != nil {
			return 0, cerr
		}
		if _, err := os.Stat(p.Dst); err == nil && !force {
			log.InfoContext(ctx, "skipped, output already exists (use --force to overwrite)",
				"input", p.Src, "output", p.Dst)
			existed++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p.Dst), 0o755); err != nil {
			log.ErrorContext(ctx, "cannot create output directory",
				"input", p.Src, "output_dir", filepath.Dir(p.Dst), "err", strerror(err))
			failed++
			continue
		}
		_, err := video.Convert(ctx, log, p.Src,
			video.Output{Kind: video.OutFile, Path: p.Dst, Name: p.Dst}, cfg, false)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return 0, cerr
			}
			if errors.Is(err, procreate.ErrNoSegments) {
				log.WarnContext(ctx, "no timelapse video inside, skipped", "input", p.Src)
				noVideo++
			} else {
				log.ErrorContext(ctx, "file conversion failed", "input", p.Src, "err", err)
				failed++
			}
			continue
		}
		log.InfoContext(ctx, "converted", "input", p.Src, "output", p.Dst)
		converted++
	}

	log.InfoContext(ctx, "batch completed", "converted", converted, "existed", existed,
		"no_video", noVideo, "failed", failed)
	if failed > 0 {
		return 1, nil
	}
	return 0, nil
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
