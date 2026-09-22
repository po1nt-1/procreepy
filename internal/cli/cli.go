// Package cli implements the procreepy command line: argparse-style
// argument parsing, dispatch, and the exit-code mapping.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"procreepy/internal/batch"
	"procreepy/internal/procreate"
	"procreepy/internal/video"
)

const (
	prog = "procreepy"

	usageLine = prog + " [options] INPUT [OUTPUT]"

	description = "Extract the archived timelapse from .procreate files as ready-made MP4 videos."
)

// versionStr is the git tag at build time, injected via
// -ldflags "-X procreepy/internal/cli.versionStr=..."; dev builds keep
// the placeholder, which versionToken refines. The tag is the single
// source of truth for the version.
var versionStr = "dev"

// versionToken returns the --version value. An explicit -X injection
// (release, make) wins as-is; a plain "dev" build that carries VCS
// stamping (an in-tree `go build` without -X) reports dev-<short sha>
// instead of a bare "dev". Stamp-less builds (-buildvcs=false,
// out-of-tree) stay "dev".
func versionToken() string {
	if versionStr != "dev" {
		return versionStr
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "dev-" + s.Value[:7]
			}
		}
	}
	return "dev"
}

const epilog = `examples:
  procreepy artwork.procreate artwork.mp4
  procreepy artwork.procreate > artwork.mp4
  cat artwork.procreate | procreepy - > artwork.mp4

  procreepy input/                    every .procreate in input/ -> ` + batch.DefaultOutputDir + `/
  procreepy input/ videos/            same, into videos/
  procreepy -r input/                 also look in sub-directories (mirrored in the output)

  procreepy --list artwork.procreate  show the segments, in playback order
  procreepy --verify artwork.procreate  check every segment, write nothing

  procreepy --split artwork.procreate   artwork.mp4 + artwork.procreepy.procreate
                                        (the slimmed project without the video)

INPUT may be a file, a directory of files, or - for stdin.
OUTPUT may be a file, a directory, or - / omitted for stdout.
Messages go to stderr; stdout carries only video (or the --list/--verify report).`

const helpText = `usage: ` + usageLine + `

` + description + `

positional arguments:
  INPUT             file.procreate, a directory of them, or - for stdin
  OUTPUT            output.mp4, a directory, or - for stdout (default for a single file)

options:
  -h, --help        show this help message and exit
  --list            list the segments and exit
  --verify          check every segment, write nothing
  -r, --recursive   directory input: also process sub-directories
  -f, --force       directory input: overwrite videos that already exist (default: skip them)
  --strict          treat missing segment numbers as an error instead of a warning
  --reencode        accepted for compatibility; stream copy is always used
  --split           also write a video-less .procreepy.procreate beside each MP4
  --tmpdir DIR      where to put temporary files (default: $TMPDIR, else /var/tmp)
  -q, --quiet       only print warnings and errors
  --version         show program's version number and exit

` + epilog

// Exit codes (matching the original tool).
const (
	exitOK         = 0
	exitFailure    = 1
	exitUsage      = 2
	exitInput      = 3
	exitNoSegments = 4
	exitBadSegment = 5
	exitIncompat   = 7
	exitWrite      = 9
	exitInterrupt  = 130
)

type parsedArgs struct {
	input     string
	output    string
	hasOutput bool
	list      bool
	verify    bool
	recursive bool
	force     bool
	strict    bool
	reencode  bool
	split     bool
	quiet     bool
	tmpdir    string
}

type action int

const (
	actNone action = iota
	actVersion
	actHelp
)

// Run executes the command line and returns the process exit code.
func Run(argv []string) int {
	return run(argv, os.Stdout, os.Stderr)
}

// run is the writer-injectable core of Run (tests capture both streams).
func run(argv []string, stdout, stderr io.Writer) int {
	args, act, perr := parseArgs(argv)
	switch act {
	case actVersion:
		fmt.Fprintln(stdout, prog, versionToken())
		return exitOK
	case actHelp:
		fmt.Fprintln(stdout, helpText)
		return exitOK
	}
	if perr != nil {
		fmt.Fprintln(stderr, "usage: "+usageLine)
		fmt.Fprintf(stderr, "%s: error: %s\n", prog, perr)
		return exitUsage
	}
	log := newLogger(stderr, args.quiet)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		code     int
		err      error
		panicked bool
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				if os.Getenv("PROCREATE_VIDEO_DEBUG") != "" {
					panic(r)
				}
				log.Error("unexpected panic (set PROCREATE_VIDEO_DEBUG=1 for a traceback)",
					"type", fmt.Sprintf("%T", r), "value", fmt.Sprintf("%v", r))
				panicked = true
			}
		}()
		code, err = dispatch(ctx, log, args)
	}()

	if panicked {
		return exitFailure
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Error("interrupted")
			return exitInterrupt
		}
		log.Error(err.Error())
		return codeOf(err)
	}
	return code
}

// newLogger builds the one application logger: TextHandler on the injected
// stderr, INFO by default and WARN with -q. The time attribute is dropped so
// output stays deterministic (the terminal or CI system stamps it).
func newLogger(stderr io.Writer, quiet bool) *slog.Logger {
	level := slog.LevelInfo
	if quiet {
		level = slog.LevelWarn
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, v slog.Attr) slog.Attr {
			if v.Key == "time" {
				return slog.Attr{}
			}
			return v
		},
	}))
}

func dispatch(ctx context.Context, log *slog.Logger, a *parsedArgs) (int, error) {
	src := a.input
	isDir := src != "-" && isDirectory(src)
	cfg := video.Config{Strict: a.strict, Reencode: a.reencode, TmpDir: a.tmpdir, Split: a.split}

	if a.list || a.verify {
		if a.hasOutput {
			return 0, &video.UsageError{Msg: "--list and --verify take a single INPUT and no OUTPUT"}
		}
		if a.split {
			return 0, &video.UsageError{Msg: "--split cannot be combined with --list or --verify"}
		}
		action := func(input string) (string, error) {
			if a.list {
				return video.List(ctx, log, input, cfg)
			}
			return video.Verify(ctx, log, input, cfg)
		}
		if isDir {
			return batch.DiagnoseDirectory(log, src, a.recursive, action)
		}
		report, err := action(src)
		if perr := video.PrintReport(report); perr != nil {
			return 0, perr
		}
		return 0, err
	}

	if isDir {
		outArg := ""
		if a.hasOutput {
			outArg = a.output
		}
		return batch.ConvertDirectory(ctx, log, src, outArg, cfg, a.force, a.recursive)
	}

	out, err := video.ResolveOutput(src, a.output, a.hasOutput)
	if err != nil {
		return 0, err
	}
	if _, err := video.Convert(ctx, log, src, out, cfg, true); err != nil {
		return 0, err
	}
	return exitOK, nil
}

func codeOf(err error) int {
	var ue *video.UsageError
	if errors.As(err, &ue) {
		return exitUsage
	}
	var ie *procreate.InputError
	if errors.As(err, &ie) {
		return exitInput
	}
	var inc *video.IncompatibleError
	if errors.As(err, &inc) {
		return exitIncompat
	}
	var we *video.WriteError
	if errors.As(err, &we) {
		return exitWrite
	}
	switch {
	case errors.Is(err, procreate.ErrNoSegments):
		return exitNoSegments
	case errors.Is(err, procreate.ErrInput):
		return exitInput
	case errors.Is(err, procreate.ErrBadSegment):
		return exitBadSegment
	}
	return exitFailure
}

func isDirectory(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// parseArgs scans argv the way argparse does: flags and positionals may be
// interleaved, "--" ends flag parsing, unknown flags are an immediate error,
// surplus positionals are reported at the end, and --version/-h fire as soon
// as they are met.
func parseArgs(argv []string) (*parsedArgs, action, error) {
	a := &parsedArgs{}
	positional := 0
	var extra []string

	fail := func(format string, args ...any) (*parsedArgs, action, error) {
		return nil, actNone, fmt.Errorf(format, args...)
	}
	// storeTrue rejects an explicit value, the way argparse does
	// ("--list=1" -> "argument --list: ignored explicit argument '1'").
	storeTrue := func(name string, hasVal bool, val string) (*parsedArgs, action, error) {
		if hasVal {
			return nil, actNone, fmt.Errorf("argument --%s: ignored explicit argument '%s'", name, val)
		}
		return nil, actNone, nil
	}

	i := 0
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			for _, p := range argv[i+1:] {
				switch positional {
				case 0:
					a.input = p
				case 1:
					a.output, a.hasOutput = p, true
				default:
					extra = append(extra, p)
				}
				positional++
			}
			break
		}
		if strings.HasPrefix(arg, "--") {
			body := arg[2:]
			name, val, hasVal := body, "", false
			if eq := strings.IndexByte(body, '='); eq >= 0 {
				name, val, hasVal = body[:eq], body[eq+1:], true
			}
			switch name {
			case "version":
				if _, _, err := storeTrue("version", hasVal, val); err != nil {
					return nil, actNone, err
				}
				return a, actVersion, nil
			case "help":
				if _, _, err := storeTrue("help", hasVal, val); err != nil {
					return nil, actNone, err
				}
				return a, actHelp, nil
			case "list":
				if _, _, err := storeTrue("list", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.list = true
				if a.verify {
					return fail("argument --list: not allowed with argument --verify")
				}
			case "verify":
				if _, _, err := storeTrue("verify", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.verify = true
				if a.list {
					return fail("argument --verify: not allowed with argument --list")
				}
			case "recursive":
				if _, _, err := storeTrue("recursive", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.recursive = true
			case "force":
				if _, _, err := storeTrue("force", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.force = true
			case "strict":
				if _, _, err := storeTrue("strict", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.strict = true
			case "reencode":
				if _, _, err := storeTrue("reencode", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.reencode = true
			case "split":
				if _, _, err := storeTrue("split", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.split = true
			case "quiet":
				if _, _, err := storeTrue("quiet", hasVal, val); err != nil {
					return nil, actNone, err
				}
				a.quiet = true
			case "tmpdir":
				if hasVal {
					a.tmpdir = val
				} else if i+1 < len(argv) {
					next := argv[i+1]
					if len(next) > 1 && next[0] == '-' {
						// argparse does not consume option-looking values.
						return fail("argument --tmpdir: expected one argument")
					}
					i++
					a.tmpdir = next
				} else {
					return fail("argument --tmpdir: expected one argument")
				}
			default:
				return fail("unrecognized arguments: %s", arg)
			}
		} else if len(arg) > 1 && arg[0] == '-' {
			// Clustered short options, like argparse ("-rq").
			for pos := 1; pos < len(arg); pos++ {
				c := string(arg[pos])
				switch c {
				case "h":
					return a, actHelp, nil
				case "r":
					a.recursive = true
				case "f":
					a.force = true
				case "q":
					a.quiet = true
				default:
					return fail("unrecognized arguments: -%s", c)
				}
			}
		} else {
			switch positional {
			case 0:
				a.input = arg
			case 1:
				a.output, a.hasOutput = arg, true
			default:
				extra = append(extra, arg)
			}
			positional++
		}
		i++
	}

	if len(extra) > 0 {
		return fail("unrecognized arguments: %s", strings.Join(extra, " "))
	}
	if positional == 0 {
		return fail("the following arguments are required: INPUT")
	}
	return a, actNone, nil
}
