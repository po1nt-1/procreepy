package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"procreepy/internal/batch"
	"procreepy/internal/procreate"
	"procreepy/internal/testkit"
)

// runCLI executes the command, capturing both stdout and stderr, and returns
// the exit code plus both streams. Video bytes bypass the writer injection and
// go straight to os.Stdout, so that global is swapped too.
func runCLI(t *testing.T, argv ...string) (int, string, string) {
	t.Helper()

	or, ow, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut := os.Stdout
	os.Stdout = ow
	outBuf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(outBuf, or)
		close(done)
	}()

	errBuf := &bytes.Buffer{}
	code := run(argv, ow, errBuf)

	ow.Close()
	os.Stdout = oldOut
	<-done
	or.Close()
	return code, outBuf.String(), errBuf.String()
}
func goodArchivePath(t *testing.T) string {
	t.Helper()
	seg1 := testkit.Segment(320, 240, []uint32{100, 200}, []uint32{50, 60})
	seg2 := testkit.Segment(320, 240, []uint32{300}, []uint32{70})
	return testkit.WriteArchive(t, map[string][]byte{
		"video/segments/segment-1.mp4": seg1,
		"video/segments/segment-2.mp4": seg2,
	}, false)
}
func assertMoovFirst(t *testing.T, data []byte) {
	t.Helper()
	if len(data) < 32 || string(data[4:8]) != "ftyp" {
		t.Fatalf("not an MP4: % x", data[:min(16, len(data))])
	}
	ftypSz := int(uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]))
	if len(data) < ftypSz+8 || string(data[ftypSz+4:ftypSz+8]) != "moov" {
		t.Fatalf("not moov-first: % x", data[:min(32, len(data))])
	}
}
func TestVersion(t *testing.T) {
	code, out, _ := runCLI(t, "--version")
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if want := "procreepy " + versionToken() + "\n"; out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
}
func TestHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		code, out, _ := runCLI(t, flag)
		if code != 0 {
			t.Fatalf("%s: code = %d, want 0", flag, code)
		}
		expected := []string{
			"usage: procreepy [options] INPUT [OUTPUT]",
			"INPUT             file.procreate, a directory of them, or - for stdin",
			"OUTPUT            output.mp4, a directory, or - for stdout",
			"--list            list the segments in playback order and exit",
			"--verify          check every segment; create no output video",
			"--psd             directory input: also export a layered .psd per artwork",
			"--tmpdir DIR      where to put temporary files (default: $TMPDIR, else /var/tmp, else the system temp directory)",
			"-q, --quiet       only print warnings and errors to stderr",
			"--                stop option parsing; treat the remaining arguments as positional",
			"For a single file, omitted OUTPUT means stdout; for directory input, omitted OUTPUT defaults to <INPUT>" + batch.OutputSuffix + "/ in the current directory.",
			"Messages and diagnostics go to stderr; stdout carries only video (or the --list/--verify report).",
		}
		for _, want := range expected {
			if !strings.Contains(out, want) {
				t.Errorf("%s: help output missing %q:\n%s", flag, want, out)
			}
		}
	}
}
func TestUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"no args", []string{}, "the following arguments are required: INPUT"},
		{"unknown long flag", []string{"--zzz", "a"}, "unrecognized arguments: --zzz"},
		{"unknown short flag", []string{"-x", "a"}, "unrecognized arguments: -x"},
		{"extra positional", []string{"a", "b", "c"}, "unrecognized arguments: c"},
		{"tmpdir missing value", []string{"--tmpdir"}, "argument --tmpdir: expected one argument"},
		{"tmpdir option-looking value", []string{"--tmpdir", "-r", "a"}, "argument --tmpdir: expected one argument"},
		{"bool with value", []string{"--list=1", "a"}, "argument --list: ignored explicit argument '1'"},
		{"list verify order 1", []string{"--list", "--verify", "a"}, "argument --verify: not allowed with argument --list"},
		{"list verify order 2", []string{"--verify", "--list", "a"}, "argument --list: not allowed with argument --verify"},
	}
	for _, tc := range cases {
		code, _, errOut := runCLI(t, tc.argv...)
		if code != 2 {
			t.Errorf("%s: code = %d, want 2 (stderr: %s)", tc.name, code, errOut)
		}
		if !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: stderr missing %q:\n%s", tc.name, tc.want, errOut)
		}
		if !strings.Contains(errOut, "usage: procreepy [options] INPUT [OUTPUT]") {
			t.Errorf("%s: stderr missing usage line:\n%s", tc.name, errOut)
		}
	}
}
func TestListFile(t *testing.T) {
	in := goodArchivePath(t)
	code, out, errOut := runCLI(t, "--list", in)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(out, "segments: 2") || !strings.Contains(out, "input: "+in) {
		t.Fatalf("stdout:\n%s", out)
	}
}
func TestListWithOutputRejected(t *testing.T) {
	in := goodArchivePath(t)
	code, _, errOut := runCLI(t, "--list", in, "out.mp4")
	if code != 2 {
		t.Fatalf("code = %d, want 2 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, "--list and --verify take a single INPUT and no OUTPUT") {
		t.Fatalf("stderr:\n%s", errOut)
	}
}
func TestMissingInput(t *testing.T) {
	code, _, errOut := runCLI(t, "--list", "nope.procreate")
	if code != 3 {
		t.Fatalf("code = %d, want 3 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, `level=ERROR msg="input does not exist: nope.procreate"`) {
		t.Fatalf("stderr:\n%s", errOut)
	}
}
func TestBadZip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "junk.procreate")
	if err := os.WriteFile(p, []byte("this is not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := runCLI(t, p)
	if code != 3 {
		t.Fatalf("code = %d, want 3 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, "input is not a valid ZIP archive") {
		t.Fatalf("stderr:\n%s", errOut)
	}
}
func TestNoSegments(t *testing.T) {
	p := testkit.WriteArchive(t, map[string][]byte{"canvas.bin": []byte("no video")}, false)
	code, _, errOut := runCLI(t, p)
	if code != 4 {
		t.Fatalf("code = %d, want 4 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, "no video/segments in this archive") {
		t.Fatalf("stderr:\n%s", errOut)
	}
}
func TestBadSegment(t *testing.T) {
	seg := testkit.Segment(320, 240, []uint32{100, 200}, []uint32{50, 60})
	trunc := seg[:len(seg)/2]
	p := testkit.WriteArchive(t, map[string][]byte{"video/segments/segment-1.mp4": trunc}, false)
	code, _, errOut := runCLI(t, p)
	if code != 5 {
		t.Fatalf("code = %d, want 5 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, `level=ERROR msg=`) {
		t.Fatalf("stderr:\n%s", errOut)
	}
}
func TestIncompatible(t *testing.T) {
	seg1 := testkit.Segment(320, 240, []uint32{100}, []uint32{50})
	seg2 := testkit.Segment(640, 480, []uint32{100}, []uint32{50})
	p := testkit.WriteArchive(t, map[string][]byte{
		"video/segments/segment-1.mp4": seg1,
		"video/segments/segment-2.mp4": seg2,
	}, false)
	code, _, errOut := runCLI(t, p)
	if code != 7 {
		t.Fatalf("code = %d, want 7 (stderr: %s)", code, errOut)
	}
	// The e2e suite pins the message byte-for-byte; here we only check the
	// shape (slog escapes the embedded quotes in the raw log line).
	if !strings.Contains(errOut, "incompatible segments") ||
		!strings.Contains(errOut, "differ\"") {
		t.Fatalf("stderr:\n%s", errOut)
	}
}
func TestConvertToFile(t *testing.T) {
	in := goodArchivePath(t)
	out := filepath.Join(t.TempDir(), "out.mp4") // parent directories are not created automatically (matching the original)
	code, _, errOut := runCLI(t, in, out)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	assertMoovFirst(t, data)
}
func TestConvertDerivesName(t *testing.T) {
	in := goodArchivePath(t)
	dir := t.TempDir()
	code, _, errOut := runCLI(t, in, dir)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	base := filepath.Base(in)
	want := filepath.Join(dir, strings.TrimSuffix(base, filepath.Ext(base))+".mp4")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("derived output missing: %s", want)
	}
}
func TestConvertToStdout(t *testing.T) {
	in := goodArchivePath(t)
	code, out, errOut := runCLI(t, in, "-")
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	assertMoovFirst(t, []byte(out))
}
func TestQuietSuppressesInfo(t *testing.T) {
	in := goodArchivePath(t)
	out := filepath.Join(t.TempDir(), "q.mp4")
	code, _, errOut := runCLI(t, "-q", in, out)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	if errOut != "" {
		t.Fatalf("quiet mode printed diagnostics (Info is suppressed; nothing else is expected):\n%s", errOut)
	}
}
func TestBatchDirectory(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"a.procreate": testkit.ArchiveBytes(map[string][]byte{
			"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
		}, false),
		"notes.txt": []byte("ignore me"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outDir := filepath.Join(t.TempDir(), "videos")
	code, _, errOut := runCLI(t, root, outDir)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	got := filepath.Join(outDir, batch.TimelapseDir, "a.mp4")
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	assertMoovFirst(t, data)
}

// TestRemovedFlagsRejected: --split and --reencode are gone. Splitting is what
// directory mode does, and --reencode never re-encoded anything, so both are
// now ordinary unknown options rather than silent no-ops.
func TestRemovedFlagsRejected(t *testing.T) {
	in := goodArchivePath(t)
	for _, flag := range []string{"--split", "--reencode"} {
		code, _, errOut := runCLI(t, flag, in)
		if code != 2 {
			t.Errorf("%s: code = %d, want 2 (stderr: %s)", flag, code, errOut)
		}
		if !strings.Contains(errOut, "unrecognized arguments: "+flag) {
			t.Errorf("%s: stderr:\n%s", flag, errOut)
		}
	}
}

// TestPSDNeedsDirectory: the PSD tree only exists in directory mode, so asking
// for it on a single file is a usage error rather than a silently ignored flag.
func TestPSDNeedsDirectory(t *testing.T) {
	in := goodArchivePath(t)
	code, _, errOut := runCLI(t, "--psd", in, filepath.Join(t.TempDir(), "out.mp4"))
	if code != 2 {
		t.Fatalf("code = %d, want 2 (stderr: %s)", code, errOut)
	}
	if !strings.Contains(errOut, "--psd needs a directory INPUT") {
		t.Fatalf("stderr:\n%s", errOut)
	}
}

func TestPSDConflictsWithListVerify(t *testing.T) {
	in := goodArchivePath(t)
	for _, argv := range [][]string{{"--psd", "--list", in}, {"--psd", "--verify", in}} {
		code, _, errOut := runCLI(t, argv...)
		if code != 2 {
			t.Errorf("%v: code = %d, want 2 (stderr: %s)", argv, code, errOut)
		}
		if !strings.Contains(errOut, "--psd cannot be combined with --list or --verify") {
			t.Errorf("%v: stderr:\n%s", argv, errOut)
		}
	}
}

// TestDirectoryModeWritesBothTrees is the headline contract: one directory in,
// two trees out, with no flag asking for it.
func TestDirectoryModeWritesBothTrees(t *testing.T) {
	inDir := t.TempDir()
	testkit.Project(2).Write(t, inDir, "a.procreate")
	outDir := filepath.Join(t.TempDir(), "out")

	// --no-zip keeps the projects as loose files, which is what this test reads.
	// The packed default has its own coverage in internal/batch.
	code, _, errOut := runCLI(t, "--no-zip", inDir, outDir)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr: %s)", code, errOut)
	}
	data, err := os.ReadFile(filepath.Join(outDir, batch.TimelapseDir, "a.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	assertMoovFirst(t, data)

	slim := filepath.Join(outDir, batch.ProjectDir, "a"+batch.ProjectSuffix)
	a, err := procreate.Open(slim, slim)
	if err != nil {
		t.Fatalf("open project: %v", err)
	}
	defer a.Close()
	for _, m := range a.Members() {
		if procreate.IsSegmentName(m.Name) {
			t.Fatalf("timelapse segment survived in the project: %s", m.Name)
		}
	}
}
func TestParseArgsDoubleDash(t *testing.T) {
	a, act, err := parseArgs([]string{"--", "weird --name", "out.mp4"})
	if err != nil || act != actNone {
		t.Fatalf("err = %v, act = %v", err, act)
	}
	if a.input != "weird --name" || a.output != "out.mp4" || !a.hasOutput {
		t.Fatalf("args = %+v", a)
	}
}
func TestParseArgsClusteredShorts(t *testing.T) {
	a, _, err := parseArgs([]string{"-rqf", "in", "out"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.recursive || !a.quiet || !a.force {
		t.Fatalf("args = %+v", a)
	}
}

func TestParseArgsInterleaved(t *testing.T) {
	a, _, err := parseArgs([]string{"--strict", "in", "-q", "out", "--tmpdir=/tmp/x"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.strict || !a.quiet || a.tmpdir != "/tmp/x" || a.input != "in" || a.output != "out" {
		t.Fatalf("args = %+v", a)
	}
}
