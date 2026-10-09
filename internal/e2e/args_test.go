package e2e

import "testing"

// wantHelp is the exact stdout of --help (with a trailing newline), captured
// from a real run of the instrumented binary.
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
  -f, --force       directory input: overwrite outputs that already exist (default: skip them)
  --strict          treat missing segment numbers as errors instead of warnings
  --psd             directory input: also export a layered .psd per artwork
  --no-zip          directory input: leave the projects as a directory instead of procreate.zip
  --tmpdir DIR      where to put temporary files (default: $TMPDIR, else /var/tmp, else the system temp directory)
  -q, --quiet       only print warnings and errors to stderr
  --version         show program's version number and exit
  --                stop option parsing; treat the remaining arguments as positional
examples:
  procreepy artwork.procreate artwork.mp4
  procreepy artwork.procreate > artwork.mp4
  cat artwork.procreate | procreepy - > artwork.mp4

  procreepy input/                    every .procreate in input/ -> input_procreepy/
  procreepy input/ out/               same, but write into out/
  procreepy -r input/ out/            also process sub-directories (mirrored in both output trees)
  procreepy --psd input/ out/         additionally export a layered .psd per artwork
  procreepy --list artwork.procreate  list the segments, in playback order
  procreepy --verify artwork.procreate  check every segment; create no output video

A directory INPUT produces two trees under OUTPUT, one per purpose:
  OUTPUT/mp4/NAME.mp4                         the joined timelapse
  OUTPUT/procreate/NAME.procreepy.procreate   the project without the timelapse
With --psd, OUTPUT/psd/NAME.psd is written too.
Each result keeps the dates of its source, so re-importing a project into
Procreate does not reshuffle the gallery. Originals are never modified.
An artwork recorded with the timelapse off still yields a project (and a PSD),
just no video.

INPUT may be a file, a directory, or - for stdin.
OUTPUT may be a file, a directory, or - for stdout.
For a single file, omitted OUTPUT means stdout; for directory input, omitted OUTPUT defaults to <INPUT>_procreepy/ in the current directory.
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
		{"psd_list", []string{"--psd", "--list", "a.procreate"},
			"--psd cannot be combined with --list or --verify", true},
		{"psd_verify", []string{"--psd", "--verify", "a.procreate"},
			"--psd cannot be combined with --list or --verify", true},
		{"psd_file_input", []string{"--psd", "in.procreate", "out.mp4"},
			"--psd needs a directory INPUT; it writes into OUTPUT/psd/", true},
		// --reencode and --split no longer exist; they parse as unknown flags.
		{"reencode_removed", []string{"--reencode", "a.procreate", "out.mp4"},
			"unrecognized arguments: --reencode", false},
		{"split_removed", []string{"--split", "a.procreate", "out.mp4"},
			"unrecognized arguments: --split", false},
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
