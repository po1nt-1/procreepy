package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"procreepy/internal/testkit"
)

func stdThree() map[string][]byte { return stdArchiveEntries(3) }

func TestConvertFile(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	wantErr := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: out.mp4 (~6.0 s of video)\n"
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 0, "", wantErr)
	eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
	assertNoPartial(t, dir)
}

func TestConvertToStdout(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	r := run(t, dir, nil, "in.procreate")
	wantErr := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: stdout (~6.0 s of video)\n"
	if r.Code != 0 {
		t.Fatalf("exit = %d, stderr %q", r.Code, r.Stderr)
	}
	if r.Stderr != wantErr {
		t.Errorf("stderr mismatch:\ngot:  %q\nwant: %q", r.Stderr, wantErr)
	}
	eqBytes(t, "stdout", []byte(r.Stdout), expectedMP4(t, segN(1), segN(2), segN(3)))
}

func TestConvertToDirectory(t *testing.T) {
	for _, out := range []string{"out/", "out"} {
		t.Run(out, func(t *testing.T) {
			dir := t.TempDir()
			writeArchive(t, dir, "in.procreate", stdThree())
			if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
				t.Fatal(err)
			}
			wantErr := "info: in.procreate: 3 segment(s)\n" +
				"info: joining segments (stream copy)\n" +
				"info: done: out/in.mp4 (~6.0 s of video)\n"
			check(t, run(t, dir, nil, "in.procreate", out), 0, "", wantErr)
			eqBytes(t, "out/in.mp4", readAll(t, filepath.Join(dir, "out", "in.mp4")),
				expectedMP4(t, segN(1), segN(2), segN(3)))
		})
	}
}

func TestConvertToMissingDir(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	// Plain path form: the parent is reported as an absolute path.
	absParent := filepath.Join(dir, "nodir")
	check(t, run(t, dir, nil, "in.procreate", "nodir/out.mp4"), 9, "",
		"error: output directory does not exist: "+absParent+"\n")
	// Trailing-slash form: the spelling is kept as given.
	check(t, run(t, dir, nil, "in.procreate", "nodir/"), 9, "",
		"error: output directory does not exist: nodir/\n")
}

func TestConvertGapWarn(t *testing.T) {
	tt := []struct {
		name   string
		nums   []int
		gapMsg string
	}{
		{"single_gap", []int{1, 2, 4}, "warning: segment numbers missing: 3 (the video would have gaps)\n"},
		{"range_gap", []int{1, 5, 6}, "warning: segment numbers missing: 2-4 (the video would have gaps)\n"},
		{"multi_gap", []int{1, 3, 7}, "warning: segment numbers missing: 2, 4-6 (the video would have gaps)\n"},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			entries := map[string][]byte{"Document/document.data": []byte("doc"), "video/segments/": {}}
			var segs [][]byte
			for _, n := range tc.nums {
				entries[fmt.Sprintf("video/segments/segment-%d.mp4", n)] = segN(n)
				segs = append(segs, segN(n))
			}
			writeArchive(t, dir, "in.procreate", entries)
			r := run(t, dir, nil, "in.procreate", "out.mp4")
			wantErr := tc.gapMsg +
				fmt.Sprintf("info: in.procreate: %d segment(s)\n", len(segs)) +
				"info: joining segments (stream copy)\n" +
				fmt.Sprintf("info: done: out.mp4 (~%.1f s of video)\n", float64(len(segs)*2))
			if r.Code != 0 {
				t.Fatalf("exit = %d, stderr %q", r.Code, r.Stderr)
			}
			if r.Stderr != wantErr {
				t.Errorf("stderr mismatch:\ngot:  %q\nwant: %q", r.Stderr, wantErr)
			}
			eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")), expectedMP4(t, segs...))
		})
	}
}

func TestConvertStrictGap(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	delete(entries, "video/segments/segment-2.mp4")
	writeArchive(t, dir, "in.procreate", entries)
	check(t, run(t, dir, nil, "--strict", "in.procreate", "out.mp4"), 5, "",
		"error: segment numbers missing: 2 (the video would have gaps)\n")
	if _, err := os.Stat(filepath.Join(dir, "out.mp4")); !os.IsNotExist(err) {
		t.Error("out.mp4 should not exist")
	}
}

func TestConvertNonOneStart(t *testing.T) {
	// Gap checking always spans 1..last, so a lone segment-5 warns about 1-4.
	dir := t.TempDir()
	entries := map[string][]byte{
		"Document/document.data":       []byte("doc"),
		"video/segments/":              {},
		"video/segments/segment-5.mp4": segN(5),
	}
	writeArchive(t, dir, "in.procreate", entries)
	wantErr := "warning: segment numbers missing: 1-4 (the video would have gaps)\n" +
		"info: in.procreate: 1 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: out.mp4 (~2.0 s of video)\n"
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 0, "", wantErr)
	eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")), expectedMP4(t, segN(5)))
}

func TestConvertIgnoresStrayMP4(t *testing.T) {
	dir := t.TempDir()
	entries := stdThree()
	entries["video/segments/thumb.mp4"] = []byte("not an mp4")
	writeArchive(t, dir, "in.procreate", entries)
	wantErr := "warning: ignoring video/segments/thumb.mp4: name does not match segment-<number>.mp4\n" +
		"info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: out.mp4 (~6.0 s of video)\n"
	check(t, run(t, dir, nil, "in.procreate", "out.mp4"), 0, "", wantErr)
	eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
}

func TestStdin(t *testing.T) {
	arc := testkit.ArchiveBytes(stdThree(), false)
	t.Run("to_file", func(t *testing.T) {
		dir := t.TempDir()
		wantErr := "info: <stdin>: 3 segment(s)\n" +
			"info: joining segments (stream copy)\n" +
			"info: done: out.mp4 (~6.0 s of video)\n"
		check(t, run(t, dir, arc, "-", "out.mp4"), 0, "", wantErr)
		eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
			expectedMP4(t, segN(1), segN(2), segN(3)))
	})
	t.Run("to_stdout", func(t *testing.T) {
		dir := t.TempDir()
		r := run(t, dir, arc, "-")
		wantErr := "info: <stdin>: 3 segment(s)\n" +
			"info: joining segments (stream copy)\n" +
			"info: done: stdout (~6.0 s of video)\n"
		if r.Code != 0 {
			t.Fatalf("exit = %d, stderr %q", r.Code, r.Stderr)
		}
		if r.Stderr != wantErr {
			t.Errorf("stderr mismatch:\ngot:  %q\nwant: %q", r.Stderr, wantErr)
		}
		eqBytes(t, "stdout", []byte(r.Stdout), expectedMP4(t, segN(1), segN(2), segN(3)))
	})
	t.Run("empty", func(t *testing.T) {
		dir := t.TempDir()
		check(t, run(t, dir, nil, "-"), 3, "", "error: stdin is empty: <stdin>\n")
	})
	t.Run("dir_output_rejected", func(t *testing.T) {
		dir := t.TempDir()
		check(t, run(t, dir, arc, "-", "out/"), 2, "",
			actionErr("cannot derive an output file name from stdin or a pipe; pass a file name as OUTPUT"))
	})
	t.Run("list", func(t *testing.T) {
		dir := t.TempDir()
		wantOut := "input: <stdin>\nsegments: 3\n" +
			"\n1 video/segments/segment-1.mp4\n2 video/segments/segment-2.mp4\n3 video/segments/segment-3.mp4\n"
		check(t, run(t, dir, arc, "--list", "-"), 0, wantOut, "")
	})
}

func TestQuiet(t *testing.T) {
	t.Run("suppresses_info", func(t *testing.T) {
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", stdThree())
		check(t, run(t, dir, nil, "-q", "in.procreate", "out.mp4"), 0, "", "")
		eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
			expectedMP4(t, segN(1), segN(2), segN(3)))
	})
	t.Run("keeps_warnings", func(t *testing.T) {
		dir := t.TempDir()
		entries := stdThree()
		delete(entries, "video/segments/segment-2.mp4")
		writeArchive(t, dir, "in.procreate", entries)
		check(t, run(t, dir, nil, "-q", "in.procreate", "out.mp4"), 0, "",
			"warning: segment numbers missing: 2 (the video would have gaps)\n")
	})
}

func TestReencodeNoop(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	wantErr := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: out.mp4 (~6.0 s of video)\n"
	check(t, run(t, dir, nil, "--reencode", "in.procreate", "out.mp4"), 0, "", wantErr)
	eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
		expectedMP4(t, segN(1), segN(2), segN(3)))
}

func TestTmpdir(t *testing.T) {
	wantErr := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: out.mp4 (~6.0 s of video)\n"
	tt := []struct {
		name string
		args []string
	}{
		{"separate", []string{"--tmpdir", "td", "in.procreate", "out.mp4"}},
		{"equals", []string{"--tmpdir=td", "in.procreate", "out.mp4"}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeArchive(t, dir, "in.procreate", stdThree())
			if err := os.MkdirAll(filepath.Join(dir, "td"), 0o755); err != nil {
				t.Fatal(err)
			}
			check(t, run(t, dir, nil, tc.args...), 0, "", wantErr)
			eqBytes(t, "out.mp4", readAll(t, filepath.Join(dir, "out.mp4")),
				expectedMP4(t, segN(1), segN(2), segN(3)))
		})
	}
}

func TestDevNull(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	wantErr := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"info: done: /dev/null (~6.0 s of video)\n"
	check(t, run(t, dir, nil, "in.procreate", "/dev/null"), 0, "", wantErr)
}

func TestDevFull(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full not present")
	}
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	wantErr := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"error: failed to write the output: No space left on device\n"
	check(t, run(t, dir, nil, "in.procreate", "/dev/full"), 9, "", wantErr)
}

// TestBrokenPipe drives the binary with a stdout reader that dies after the
// first byte; the output (600 KB) far exceeds the 64 KB pipe buffer.
func TestBrokenPipe(t *testing.T) {
	dir := t.TempDir()
	entries := map[string][]byte{"Document/document.data": []byte("doc"), "video/segments/": {}}
	for i := 1; i <= 3; i++ {
		entries[fmt.Sprintf("video/segments/segment-%d.mp4", i)] =
			testkit.Segment(320, 240, []uint32{200000}, []uint32{700})
	}
	writeArchive(t, dir, "in.procreate", entries)

	cmd := exec.Command(bin, "in.procreate")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+t.TempDir())
	cmd.Stdin = bytes.NewReader(nil)
	pr, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.WaitDelay = 60 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(pr, make([]byte, 1)); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	pr.Close()
	err = cmd.Wait()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 9 {
		t.Fatalf("want exit 9, got %v (stderr %q)", err, errb.String())
	}
	want := "info: in.procreate: 3 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"error: failed to write to stdout: broken pipe\n"
	if errb.String() != want {
		t.Errorf("stderr mismatch:\ngot:  %q\nwant: %q", errb.String(), want)
	}
}

// TestInterrupt waits until the .partial write target appears (the run is
// then guaranteed to be mid-write), sends SIGINT, and pins the exit code,
// message, and the absence of leftovers. Polling instead of a fixed sleep
// keeps the test independent of machine speed.
func TestInterrupt(t *testing.T) {
	if testing.Short() {
		t.Skip("large")
	}
	dir := t.TempDir()
	const n = 30
	entries := map[string][]byte{"Document/document.data": []byte("doc"), "video/segments/": {}}
	for i := 1; i <= n; i++ {
		entries[fmt.Sprintf("video/segments/segment-%d.mp4", i)] =
			testkit.Segment(320, 240, []uint32{16 << 20}, []uint32{1024})
	}
	writeArchive(t, dir, "big.procreate", entries)

	cmd := exec.Command(bin, "big.procreate", "big.mp4")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+t.TempDir())
	cmd.Stdin = bytes.NewReader(nil)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	cmd.WaitDelay = 180 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(120 * time.Second)
	for {
		parts, _ := filepath.Glob(filepath.Join(dir, ".procreepy-*.partial"))
		if len(parts) > 0 {
			if st, err := os.Stat(parts[0]); err == nil && st.Size() > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("conversion finished before we could interrupt it (stderr %q)", errb.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 130 {
		t.Fatalf("want exit 130, got %v (stderr %q)", err, errb.String())
	}
	want := "info: big.procreate: 30 segment(s)\n" +
		"info: joining segments (stream copy)\n" +
		"error: interrupted\n"
	if got := errb.String(); got != want {
		t.Errorf("stderr = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "big.mp4")); !os.IsNotExist(err) {
		t.Error("big.mp4 must not exist after interrupt")
	}
	assertNoPartial(t, dir)
}

func TestSameFile(t *testing.T) {
	t.Run("identity", func(t *testing.T) {
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", stdThree())
		abs := filepath.Join(dir, "in.procreate")
		check(t, run(t, dir, nil, "in.procreate", "in.procreate"), 2, "",
			actionErr("OUTPUT is the same file as INPUT: "+abs))
	})
	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		writeArchive(t, dir, "in.procreate", stdThree())
		link := filepath.Join(dir, "link.procreate")
		if err := os.Symlink("in.procreate", link); err != nil {
			t.Skip("symlinks unavailable: ", err)
		}
		check(t, run(t, dir, nil, "in.procreate", "link.procreate"), 2, "",
			actionErr("OUTPUT is the same file as INPUT: "+link))
	})
}

func TestOutputDirNotWritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission bits are ignored")
	}
	dir := t.TempDir()
	writeArchive(t, dir, "in.procreate", stdThree())
	ro := filepath.Join(dir, "ro")
	if err := os.MkdirAll(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(ro, 0o755)
	check(t, run(t, dir, nil, "in.procreate", filepath.Join("ro", "out.mp4")), 9, "",
		"error: output directory is not writable: "+ro+"\n")
}

// assertNoPartial fails if an atomic-write temp file survived.
func assertNoPartial(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".procreepy-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
