package e2e

import "testing"

// wantHelp is the exact stdout of --help (with a trailing newline).
const wantHelp = `usage: procreepy [options] INPUT [OUTPUT]

Extract the archived timelapse from .procreate files into ready-made MP4 videos.

positional arguments:
  INPUT             file.procreate, a directory of them, or - for stdin
  OUTPUT            output.mp4, a directory, or - for stdout
options:
  -h, --help        show this help message and exit
  --list            list the segments in playback order and exit
  --verify          check every segment; create no output video
  -r, --recursive   directory input: also process sub-directories
  -f, --force       directory input: overwrite videos that already exist (default: skip them)
  --strict          treat missing segment numbers as errors instead of warnings
  --reencode        accepted for compatibility; stream copy is always used
  --split           write a video-less .procreepy.procreate next to each MP4 (requires an OUTPUT path, not stdout)
  --tmpdir DIR      where to put temporary files (default: $TMPDIR, else /var/tmp, else the system temp directory)
  -q, --quiet       only print warnings and errors to stderr
  --version         show program's version number and exit
  --                stop option parsing; treat the remaining arguments as positional
examples:
  procreepy artwork.procreate artwork.mp4
  procreepy artwork.procreate > artwork.mp4
  cat artwork.procreate | procreepy - > artwork.mp4

  procreepy input/                    convert every .procreate in input/ -> output/timelaps/
  procreepy input/ videos/            same, but write into videos/
  procreepy -r input/                 also process sub-directories (mirrored in the output)
  procreepy --list artwork.procreate  list the segments, in playback order
  procreepy --verify artwork.procreate  check every segment; create no output video

  procreepy --split artwork.procreate   artwork.mp4 + artwork.procreepy.procreate
                                        (the slimmed project without the video)
INPUT may be a file, a directory, or - for stdin.
OUTPUT may be a file, a directory, or - for stdout.
For a single file, omitted OUTPUT means stdout; for directory input, omitted OUTPUT defaults to output/timelaps/.
Messages and diagnostics go to stderr; stdout carries only video (or the --list/--verify report).
`

func TestVersionAndHelp(t *testing.T) {
	tt := []struct {
		name string
		args []string
		want string
	}{
		{"version", []string{"--version"}, "procreepy dev\n"},
		{"help_short", []string{"-h"}, wantHelp},
		{"help_long", []string{"--help"}, wantHelp},
		// -h fires as soon as it is met, before end-of-scan validation.
		{"help_beats_bad_flag", []string{"-h", "a.procreate", "extra"}, wantHelp},
		{"help_beats_surplus", []string{"a.procreate", "extra", "-h"}, wantHelp},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			check(t, run(t, dir, nil, tc.args...), 0, tc.want, "")
		})
	}
}

func TestUsageErrors(t *testing.T) {
	tt := []struct {
		name string
		args []string
		msg  string
		bare bool // true: raised after parsing (no usage banner)
	}{
		{"no_args", nil, "the following arguments are required: INPUT", false},
		{"dashdash_only", []string{"--"}, "the following arguments are required: INPUT", false},
		{"cluster_no_input", []string{"-rq"}, "the following arguments are required: INPUT", false},
		{"unknown_long", []string{"--bogus"}, "unrecognized arguments: --bogus", false},
		{"unknown_short", []string{"-x"}, "unrecognized arguments: -x", false},
		{"one_extra", []string{"a.procreate", "out.mp4", "extra"}, "unrecognized arguments: extra", false},
		{"two_extra", []string{"a.procreate", "out.mp4", "x", "y"}, "unrecognized arguments: x y", false},
		{"list_verify", []string{"--list", "--verify", "a.procreate"},
			"argument --verify: not allowed with argument --list", false},
		{"verify_list", []string{"--verify", "--list", "a.procreate"},
			"argument --list: not allowed with argument --verify", false},
		{"list_eq", []string{"--list=1", "a.procreate"}, "argument --list: ignored explicit argument '1'", false},
		{"version_eq", []string{"--version=x"}, "argument --version: ignored explicit argument 'x'", false},
		{"help_eq", []string{"--help=y"}, "argument --help: ignored explicit argument 'y'", false},
		{"tmpdir_missing_val", []string{"--tmpdir"}, "argument --tmpdir: expected one argument", false},
		{"tmpdir_option_val", []string{"--tmpdir", "--list", "a.procreate"}, "argument --tmpdir: expected one argument", false},
		{"list_with_output", []string{"--list", "a.procreate", "out.mp4"},
			"--list and --verify take a single INPUT and no OUTPUT", true},
		{"verify_with_output", []string{"--verify", "a.procreate", "out.mp4"},
			"--list and --verify take a single INPUT and no OUTPUT", true},
		{"split_list", []string{"--split", "--list", "a.procreate"},
			"--split cannot be combined with --list or --verify", true},
		{"split_verify", []string{"--split", "--verify", "a.procreate"},
			"--split cannot be combined with --list or --verify", true},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			want := usageErr(tc.msg)
			if tc.bare {
				want = actionErr(tc.msg)
			}
			check(t, run(t, dir, nil, tc.args...), 2, "", want)
		})
	}
}
