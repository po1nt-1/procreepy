// Package e2e pins the observable contract of the procreepy binary:
// exit codes, exact stdout/stderr bytes, and output file bytes. It builds
// the real binary (instrumented for coverage) and drives it as a black box,
// so refactors inside the internals cannot change public behavior silently.
package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"procreepy/internal/mp4"
	"procreepy/internal/testkit"
)

var (
	bin      string
	coverDir string
	runSeq   int64
)

func TestMain(m *testing.M) {
	b, err := buildBinary()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: build failed:", err)
		os.Exit(1)
	}
	bin = b
	cd, err := os.MkdirTemp("", "procreepy-e2e-cover-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.RemoveAll(b)
		os.Exit(1)
	}
	coverDir = cd
	code := m.Run()
	aggregateCover()
	os.RemoveAll(cd)
	os.RemoveAll(filepath.Dir(b))
	os.Exit(code)
}

func buildBinary() (string, error) {
	dir, err := os.MkdirTemp("", "procreepy-e2e-bin-")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "procreepy")
	cmd := exec.Command("go", "build", "-cover", "-o", bin, "procreepy/cmd/procreepy")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v: %s", err, out)
	}
	return bin, nil
}

type res struct {
	Code   int
	Stdout string
	Stderr string
}

// run invokes the binary with dir as the working directory. stdin nil means
// an immediately closed (empty) pipe, never a terminal. Coverage: the binary
// is built with -cover, so it drops a profile in GOCOVERDIR at exit.
func run(t *testing.T, dir string, stdin []byte, args ...string) res {
	t.Helper()
	prof := filepath.Join(coverDir, fmt.Sprintf("run-%04d", atomic.AddInt64(&runSeq, 1)))
	if err := os.MkdirAll(prof, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.WaitDelay = 90 * time.Second
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+prof)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v (stderr: %s)", args, err, errb.String())
		}
		code = ee.ExitCode()
	}
	return res{Code: code, Stdout: out.String(), Stderr: errb.String()}
}

// check asserts the full observable result: exit code, stdout, stderr.
func check(t *testing.T, r res, wantCode int, wantOut, wantErr string) {
	t.Helper()
	if r.Code != wantCode {
		t.Errorf("exit code = %d, want %d\nstdout: %q\nstderr: %q", r.Code, wantCode, r.Stdout, r.Stderr)
	}
	if r.Stdout != wantOut {
		t.Errorf("stdout mismatch:\ngot:  %q\nwant: %q", r.Stdout, wantOut)
	}
	if r.Stderr != wantErr {
		t.Errorf("stderr mismatch:\ngot:  %q\nwant: %q", r.Stderr, wantErr)
	}
}

// segN builds a 320x240 segment whose mdat payload depends on n, so any
// segment order swap is visible in the output bytes.
func segN(n int) []byte {
	return testkit.Segment(320, 240,
		[]uint32{1000 + uint32(n)*7, 900 + uint32(n)*3},
		[]uint32{700 + uint32(n)*5})
}

// stdArchiveEntries is a minimal .procreate: one non-video member, the
// segments directory entry, and n video segments 1..n.
func stdArchiveEntries(n int) map[string][]byte {
	e := map[string][]byte{
		"Document/document.data": []byte("fake-document-data"),
		"video/segments/":        {},
	}
	for i := 1; i <= n; i++ {
		e[fmt.Sprintf("video/segments/segment-%d.mp4", i)] = segN(i)
	}
	return e
}

// writeArchive writes an archive named dir/name and returns its path.
func writeArchive(t *testing.T, dir, name string, entries map[string][]byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, testkit.ArchiveBytes(entries, false), 0o644); err != nil {
		t.Fatalf("write archive %s: %v", p, err)
	}
	return p
}

// writeArchiveCorrupted writes the archive, then flips one byte inside
// member's stored data so the ZIP CRC check trips when it is read.
func writeArchiveCorrupted(t *testing.T, dir, name string, entries map[string][]byte, member string) string {
	t.Helper()
	p := writeArchive(t, dir, name, entries)
	raw := readAll(t, p)
	raw = corruptMember(raw, member)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatalf("rewrite %s: %v", p, err)
	}
	return p
}

// corruptMember flips one byte of member's data region in raw ZIP bytes.
func corruptMember(b []byte, member string) []byte {
	sig := []byte{0x50, 0x4B, 0x03, 0x04}
	off := 0
	for {
		i := bytes.Index(b[off:], sig)
		if i < 0 {
			panic("corruptMember: local header for " + member + " not found")
		}
		i += off
		nlen := int(b[i+26]) | int(b[i+27])<<8
		elen := int(b[i+28]) | int(b[i+29])<<8
		if string(b[i+30:i+30+nlen]) == member {
			b[i+30+nlen+elen+1] ^= 0xFF
			return b
		}
		off = i + 4
	}
}

// expectedMP4 is the reference concatenation: what the binary must emit.
func expectedMP4(t *testing.T, segs ...[]byte) []byte {
	t.Helper()
	movies := make([]*mp4.Movie, len(segs))
	payloads := make([][]byte, len(segs))
	for i, b := range segs {
		m, err := mp4.Parse(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatalf("parse reference segment %d: %v", i+1, err)
		}
		movies[i] = m
		payloads[i] = b[m.Mdat[0].Start:m.Mdat[0].End]
	}
	mg, err := mp4.Merge(movies)
	if err != nil {
		t.Fatalf("reference merge: %v", err)
	}
	out, err := mg.EmitToBytes(payloads)
	if err != nil {
		t.Fatalf("reference emit: %v", err)
	}
	return out
}

// readAll reads a file or fails the test.
func readAll(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

// eqBytes compares two byte streams with a short diff on mismatch.
func eqBytes(t *testing.T, p string, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	t.Errorf("%s: bytes differ (got %d bytes, want %d)", p, len(got), len(want))
	g := commonPrefix(got, want)
	ctx := 24
	from := g - ctx
	if from < 0 {
		from = 0
	}
	to := g + 1 + ctx
	if to > len(got) {
		to = len(got)
	}
	if to > len(want) {
		to = len(want)
	}
	t.Logf("first difference at byte %d; context:\ngot:  % x\nwant: % x", g, got[from:to], want[from:to])
}

func commonPrefix(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// usageErr renders the two-line stderr of a parse-stage usage error.
func usageErr(msg string) string {
	return "usage: procreepy [options] INPUT [OUTPUT]\nprocreepy: error: " + msg + "\n"
}

// actionErr renders the single-line stderr of an error raised after argument
// parsing (dispatch/convert stage): no usage banner, no prog prefix.
func actionErr(msg string) string {
	return "error: " + msg + "\n"
}

// diffStrings renders a human diff for small string mismatches.
func diffStrings(t *testing.T, what string, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	t.Errorf("%s mismatch:\ngot:  %q\nwant: %q", what, got, want)
	_ = strings.TrimSuffix(got, "\n")
}

// aggregateCover converts the per-run raw coverage directories (one per run,
// holding the binary covdata files the instrumented binary drops at exit)
// into one text profile at the repository root (cover.e2e.out).
func aggregateCover() {
	var dirs []string
	entries, err := os.ReadDir(coverDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(coverDir, e.Name()))
		}
	}
	if len(dirs) == 0 {
		return
	}
	out := filepath.Join("..", "..", "cover.e2e.out")
	cmd := exec.Command("go", "tool", "covdata", "textfmt",
		"-i="+strings.Join(dirs, ","), "-o="+out)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: coverage aggregation failed:", err)
	}
}

// humanBytes mirrors the video package's formatter (pinned behavior,
// duplicated here so the expectations stay self-contained).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	s := float64(n) / float64(div)
	if s < 100 {
		return fmt.Sprintf("%.1f %ciB", s, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%d %ciB", int64(s), "KMGTPE"[exp])
}
