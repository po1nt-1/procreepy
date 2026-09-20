package batch

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"procreepy/internal/mp4"
	"procreepy/internal/procreate"
	"procreepy/internal/testkit"
	"procreepy/internal/video"
)

// bufLogger builds a real TextHandler logger writing to buf, so assertions
// check the exact rendered lines.
func bufLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func goodArchBytes(t *testing.T) []byte {
	t.Helper()
	seg1 := testkit.Segment(320, 240, []uint32{100, 200}, []uint32{50, 60})
	seg2 := testkit.Segment(320, 240, []uint32{300}, []uint32{70})
	return testkit.ArchiveBytes(map[string][]byte{
		"video/segments/segment-1.mp4": seg1,
		"video/segments/segment-2.mp4": seg2,
	}, false)
}

func emptyArchBytes(t *testing.T) []byte {
	t.Helper()
	return testkit.ArchiveBytes(map[string][]byte{"canvas.bin": []byte("no video here")}, false)
}

func writeTree(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverFlat(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{
		"b.procreate":     goodArchBytes(t),
		"a.procreate":     goodArchBytes(t),
		"A.PROCREATE":     goodArchBytes(t),
		"z.txt":           []byte("nope"),
		".h.procreate":    goodArchBytes(t),
		"sub/x.procreate": goodArchBytes(t), // in a subdir: invisible without -r
	})
	if err := os.Symlink(filepath.Join(root, "a.procreate"), filepath.Join(root, "lnk.procreate")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "gone.procreate"), filepath.Join(root, "broken.procreate")); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A.PROCREATE", "a.procreate", "b.procreate", "lnk.procreate"}
	for i, w := range want {
		if i >= len(got) || filepath.Base(got[i]) != w {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries %v, want %d %v", len(got), got, len(want), want)
	}
}

func TestDiscoverRecursive(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{
		"x.procreate":            goodArchBytes(t),
		"sub/y.procreate":        goodArchBytes(t),
		"sub/b/first.procreate":  goodArchBytes(t),
		"sub/a/second.procreate": goodArchBytes(t),
		"sub/.d/skip.procreate":  goodArchBytes(t),
		"sub/junk.txt":           []byte("nope"),
	})
	got, err := Discover(root, true)
	if err != nil {
		t.Fatal(err)
	}
	var bases []string
	for _, g := range got {
		bases = append(bases, filepath.ToSlash(g[len(root)+1:]))
	}
	want := []string{"x.procreate", "sub/y.procreate", "sub/a/second.procreate", "sub/b/first.procreate"}
	if strings.Join(bases, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", bases, want)
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	nope := filepath.Join(t.TempDir(), "nope")
	if _, err := Discover(nope, false); err == nil {
		t.Fatal("non-recursive missing root: want error")
	}
	got, err := Discover(nope, true)
	if err != nil || len(got) != 0 {
		t.Fatalf("recursive missing root: got %v, %v; want empty, nil", got, err)
	}
}

func TestPlanOutputs(t *testing.T) {
	root := "in"
	plan := PlanOutputs([]string{"in/a.procreate", "in/sub/b.procreate"}, root, "out", true)
	if len(plan) != 2 {
		t.Fatalf("plan = %v", plan)
	}
	if plan[0].Dst != "out/a.mp4" || plan[1].Dst != "out/sub/b.mp4" {
		t.Fatalf("plan = %v", plan)
	}
	plan = PlanOutputs([]string{"in/a.procreate"}, root, "out", false)
	if plan[0].Dst != "out/a.mp4" {
		t.Fatalf("flat plan = %v", plan)
	}
	// Forced duplicate inputs exercise the -2/-3 suffixing.
	plan = PlanOutputs([]string{"in/a.procreate", "in/a.procreate"}, root, "out", false)
	if plan[0].Dst != "out/a.mp4" || plan[1].Dst != "out/a-2.mp4" {
		t.Fatalf("clash plan = %v", plan)
	}
}

func TestConvertDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{
		"bad.procreate":   []byte("this is not a zip archive at all"),
		"empty.procreate": emptyArchBytes(t),
		"ok.procreate":    goodArchBytes(t),
	})
	outDir := filepath.Join(t.TempDir(), "out")
	var logBuf bytes.Buffer
	log := bufLogger(&logBuf)

	code, err := ConvertDirectory(context.Background(), log, root, outDir, video.Config{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1 (one bad file); log:\n%s", code, logBuf.String())
	}
	logged := logBuf.String()
	for _, want := range []string{
		`level=INFO msg="batch conversion started" files=3`,
		`level=WARN msg="no timelapse video inside, skipped"`,
		`level=ERROR msg="file conversion failed"`,
		"input is not a valid ZIP archive",
		`level=INFO msg="batch completed" converted=1 existed=0 no_video=1 failed=1`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("log missing %q:\n%s", want, logged)
		}
	}
	if strings.Contains(logged, "already exists") {
		t.Errorf("first run must not skip anything:\n%s", logged)
	}
	okOut := filepath.Join(outDir, "ok.mp4")
	if _, err := os.Stat(okOut); err != nil {
		t.Fatalf("expected %s: %v", okOut, err)
	}
	f, err := os.Open(okOut)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := f.Stat()
	m, err := mp4.Parse(f, fi.Size())
	f.Close()
	if err != nil {
		t.Fatalf("output not a valid MP4: %v", err)
	}
	if len(m.Mdat) != 2 {
		t.Fatalf("output has %d mdat boxes, want 2", len(m.Mdat))
	}
	var head [40]byte
	f, _ = os.Open(okOut)
	_, _ = f.Read(head[:])
	f.Close()
	ftypSz := int(uint32(head[0])<<24 | uint32(head[1])<<16 | uint32(head[2])<<8 | uint32(head[3]))
	if string(head[ftypSz+4:ftypSz+8]) != "moov" {
		t.Fatalf("output is not moov-first: % x", head[:])
	}
	if _, err := os.Stat(filepath.Join(outDir, "empty.mp4")); err == nil {
		t.Error("empty.mp4 should not exist")
	}
	if _, err := os.Stat(filepath.Join(outDir, "bad.mp4")); err == nil {
		t.Error("bad.mp4 should not exist")
	}

	// Second run: ok.mp4 already exists -> skipped without --force.
	logBuf.Reset()
	code, err = ConvertDirectory(context.Background(), log, root, outDir, video.Config{}, false, false)
	if err != nil || code != 1 {
		t.Fatalf("rerun: code=%d err=%v\n%s", code, err, logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "already exists (use --force to overwrite)") {
		t.Errorf("rerun log missing skip message:\n%s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), `existed=1`) {
		t.Errorf("rerun log missing summary:\n%s", logBuf.String())
	}

	// Third run with --force: ok.mp4 is rewritten.
	logBuf.Reset()
	code, err = ConvertDirectory(context.Background(), log, root, outDir, video.Config{}, true, false)
	if err != nil || code != 1 {
		t.Fatalf("force run: code=%d err=%v\n%s", code, err, logBuf.String())
	}
	if !strings.Contains(logBuf.String(), `converted=1`) {
		t.Errorf("force run log:\n%s", logBuf.String())
	}
}

func TestConvertDirectoryErrors(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{"ok.procreate": goodArchBytes(t)})

	if _, err := ConvertDirectory(context.Background(), slog.New(slog.DiscardHandler), root, "-",
		video.Config{}, false, false); err == nil {
		t.Fatal("stdout output dir: want error")
	} else {
		var ue *video.UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %T %v", err, err)
		}
		if !strings.Contains(ue.Msg, "cannot write several videos to stdout") {
			t.Fatalf("msg = %q", ue.Msg)
		}
	}

	fileAsDir := filepath.Join(t.TempDir(), "plainfile")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ConvertDirectory(context.Background(), slog.New(slog.DiscardHandler), root, fileAsDir,
		video.Config{}, false, false); err == nil {
		t.Fatal("output path is a file: want error")
	} else if !strings.Contains(err.Error(), "output path exists and is not a directory") {
		t.Fatalf("err = %v", err)
	}

	empty := t.TempDir()
	if _, err := ConvertDirectory(context.Background(), slog.New(slog.DiscardHandler), empty, "",
		video.Config{}, false, false); err == nil {
		t.Fatal("empty dir: want error")
	} else {
		var ie *procreate.InputError
		if !errors.As(err, &ie) {
			t.Fatalf("err = %T %v", err, err)
		}
		if !strings.Contains(ie.Msg, "no .procreate files found in "+empty) ||
			!strings.Contains(ie.Msg, "(use -r to look in sub-directories)") {
			t.Fatalf("msg = %q", ie.Msg)
		}
	}
}

// captureStdout swaps os.Stdout for a pipe; the returned collector restores
// the real stdout and returns everything that was written.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	return func() string {
		w.Close()
		os.Stdout = old
		buf := &bytes.Buffer{}
		_, _ = buf.ReadFrom(r)
		return buf.String()
	}
}

func TestDiagnoseDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{
		"one.procreate":   goodArchBytes(t),
		"empty.procreate": emptyArchBytes(t),
	})
	var logBuf bytes.Buffer
	log := bufLogger(&logBuf)
	collect := captureStdout(t)

	code, err := DiagnoseDirectory(log, root, false,
		func(input string) (string, error) {
			return video.List(context.Background(), log, input, video.Config{})
		})
	out := collect()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0 (no-segment is a warning); log:\n%s", code, logBuf.String())
	}
	if !strings.Contains(out, "input: "+filepath.Join(root, "one.procreate")) {
		t.Errorf("stdout missing input line:\n%s", out)
	}
	if !strings.Contains(out, "segments: 2") || !strings.Contains(out, "segments: 0") {
		t.Errorf("stdout missing segment counts:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimRight(out, "\n"), "input:") ||
		strings.Count(out, "input:") != 2 {
		t.Errorf("stdout:\n%s", out)
	}
	if !strings.Contains(logBuf.String(), `level=WARN msg="no timelapse video inside"`) ||
		!strings.Contains(logBuf.String(), `input=`+filepath.Join(root, "empty.procreate")) {
		t.Errorf("log missing no-timelapse warning:\n%s", logBuf.String())
	}
}

func TestConvertDirectorySplit(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{"one.procreate": goodArchBytes(t)})
	outDir := filepath.Join(t.TempDir(), "out")
	var logBuf bytes.Buffer
	log := bufLogger(&logBuf)

	code, err := ConvertDirectory(context.Background(), log, root, outDir,
		video.Config{Split: true}, false, false)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v\n%s", code, err, logBuf.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "one.mp4")); err != nil {
		t.Fatalf("one.mp4 missing: %v", err)
	}
	slim := filepath.Join(outDir, "one.procreepy.procreate")
	if _, err := os.Stat(slim); err != nil {
		t.Fatalf("slim missing: %v", err)
	}
	a, err := procreate.Open(slim, slim)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.Segments(procreate.Options{}); err == nil {
		t.Error("slim still has segments")
	}
	if !strings.Contains(logBuf.String(), `msg="slimmed archive written"`) ||
		!strings.Contains(logBuf.String(), `path=`+slim) {
		t.Errorf("log missing slim line:\n%s", logBuf.String())
	}
}

func TestDiagnoseDirectoryFails(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string][]byte{"bad.procreate": []byte("junk")})
	var logBuf bytes.Buffer
	log := bufLogger(&logBuf)
	collect := captureStdout(t)

	code, err := DiagnoseDirectory(log, root, false,
		func(input string) (string, error) {
			return video.Verify(context.Background(), log, input, video.Config{})
		})
	collect()
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(logBuf.String(), `level=ERROR msg="diagnosis failed"`) ||
		!strings.Contains(logBuf.String(), `input=`+filepath.Join(root, "bad.procreate")) {
		t.Errorf("log:\n%s", logBuf.String())
	}
}
