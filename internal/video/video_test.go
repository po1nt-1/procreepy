package video

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"procreepy/internal/mp4"
	"procreepy/internal/procreate"
	"procreepy/internal/testkit"
)

// discardLog is a logger that swallows everything (unit tests don't care
// about the human-readable output).
func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

func twoSegArchive(t *testing.T, compress bool) string {
	t.Helper()
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100, 200}, []uint32{50, 60}),
		"video/segments/segment-2.mp4": testkit.Segment(320, 240, []uint32{150}, []uint32{70}),
	}
	return testkit.WriteArchive(t, entries, compress)
}

// parseMP4 parses a whole MP4 file from disk.
func parseMP4(t *testing.T, path string) *mp4.Movie {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	m, err := mp4.Parse(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	return m
}

// checkConcat asserts the output is a valid moov-first concatenation of the
// two fixture segments (3 video / 3 audio samples, sizes in order).
func checkConcat(t *testing.T, m *mp4.Movie) {
	t.Helper()
	if len(m.Tracks) != 2 {
		t.Fatalf("tracks: got %d, want 2", len(m.Tracks))
	}
	v, a := m.Tracks[0], m.Tracks[1]
	if v.Handler != "vide" || a.Handler != "soun" {
		t.Fatalf("handlers: %s/%s", v.Handler, a.Handler)
	}
	wantV := []uint32{100, 200, 150}
	wantA := []uint32{50, 60, 70}
	if len(v.SampleSizes) != 3 || len(a.SampleSizes) != 3 {
		t.Fatalf("sample counts: video=%d audio=%d (want 3/3)", len(v.SampleSizes), len(a.SampleSizes))
	}
	for i, w := range wantV {
		if v.SampleSizes[i] != w {
			t.Errorf("video size[%d] = %d, want %d", i, v.SampleSizes[i], w)
		}
	}
	for i, w := range wantA {
		if a.SampleSizes[i] != w {
			t.Errorf("audio size[%d] = %d, want %d", i, a.SampleSizes[i], w)
		}
	}
	if m.Mvhd.Duration != 90 || m.Mvhd.Timescale != 30 {
		t.Errorf("movie duration: %d/%d, want 90/30", m.Mvhd.Duration, m.Mvhd.Timescale)
	}
	// moov-first layout: ftyp then moov, mdat boxes afterwards.
	rd := mp4.NewReader(bytes.NewReader(m.Ftyp), int64(len(m.Ftyp)))
	b0, err := mp4.ScanBoxes(rd, 0, int64(len(m.Ftyp)))
	if err != nil || len(b0) != 1 || string(b0[0].Type) != "ftyp" {
		t.Fatalf("first box: %v (%v)", b0, err)
	}
}

func TestConvertToFileStored(t *testing.T) {
	in := twoSegArchive(t, false)
	out := filepath.Join(t.TempDir(), "out.mp4")
	o := Output{Kind: OutFile, Path: out, Name: out}
	dur, err := Convert(context.Background(), discardLog(), in, o, Config{}, true)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if dur != 3.0 {
		t.Errorf("duration = %v, want 3.0", dur)
	}
	checkConcat(t, parseMP4(t, out))
	// no leftover partial file
	d, _ := os.ReadDir(filepath.Dir(out))
	for _, e := range d {
		if strings.Contains(e.Name(), ".partial") {
			t.Errorf("leftover %s", e.Name())
		}
	}
}

func TestConvertToFileDeflated(t *testing.T) {
	in := twoSegArchive(t, true)
	out := filepath.Join(t.TempDir(), "out.mp4")
	if _, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutFile, Path: out, Name: out},
		Config{}, false); err != nil {
		t.Fatalf("convert: %v", err)
	}
	checkConcat(t, parseMP4(t, out))
}

func TestConvertOverwritesExisting(t *testing.T) {
	in := twoSegArchive(t, false)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	if err := os.WriteFile(out, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutFile, Path: out, Name: out},
		Config{}, false); err != nil {
		t.Fatalf("convert: %v", err)
	}
	if b, _ := os.ReadFile(out); bytes.Equal(b, []byte("junk")) {
		t.Fatal("existing output was not replaced")
	}
	checkConcat(t, parseMP4(t, out))
}

func TestConvertSameFile(t *testing.T) {
	in := twoSegArchive(t, false)
	var ue *UsageError
	_, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutFile, Path: in, Name: in},
		Config{}, false)
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UsageError", err)
	}
	if !strings.Contains(err.Error(), "same file") {
		t.Errorf("message: %q", err)
	}
}

func TestConvertIncompatible(t *testing.T) {
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
		"video/segments/segment-2.mp4": testkit.Segment(640, 480, []uint32{100}, []uint32{50}),
	}
	in := testkit.WriteArchive(t, entries, false)
	var inc *IncompatibleError
	_, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutStdout}, Config{}, false)
	if !errors.As(err, &inc) {
		t.Fatalf("err = %v, want IncompatibleError", err)
	}
	wantMsg := `incompatible segments "video/segments/segment-1.mp4" and "video/segments/segment-2.mp4": fields "width", "height" differ`
	if msg := err.Error(); msg != wantMsg {
		t.Errorf("message = %q, want %q", msg, wantMsg)
	}
	if want := []mp4.Field{"width", "height"}; !reflect.DeepEqual(inc.Fields, want) {
		t.Errorf("Fields = %v, want %v", inc.Fields, want)
	}
}

func TestConvertBadSegment(t *testing.T) {
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
		"video/segments/segment-2.mp4": []byte("garbage, not an MP4"),
	}
	in := testkit.WriteArchive(t, entries, false)
	_, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutStdout}, Config{}, false)
	if !errors.Is(err, procreate.ErrBadSegment) {
		t.Fatalf("err = %v, want ErrBadSegment", err)
	}
}

func TestConvertNoSegments(t *testing.T) {
	in := testkit.WriteArchive(t, map[string][]byte{"Document.data": {1, 2, 3}}, false)
	_, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutStdout}, Config{}, false)
	if !errors.Is(err, procreate.ErrNoSegments) {
		t.Fatalf("err = %v, want ErrNoSegments", err)
	}
}

func TestConvertMissingInput(t *testing.T) {
	_, err := Convert(context.Background(), discardLog(), "/nope/none.procreate",
		Output{Kind: OutStdout}, Config{}, false)
	if !errors.Is(err, procreate.ErrInput) {
		t.Fatalf("err = %v, want ErrInput", err)
	}
}

func TestConvertNonexistentOutputDir(t *testing.T) {
	in := twoSegArchive(t, false)
	o := Output{Kind: OutFile, Path: filepath.Join(t.TempDir(), "no", "such", "dir", "x.mp4"), Name: "x.mp4"}
	var we *WriteError
	_, err := Convert(context.Background(), discardLog(), in, o, Config{}, false)
	if !errors.As(err, &we) {
		t.Fatalf("err = %v, want WriteError", err)
	}
	if !strings.Contains(err.Error(), "output directory does not exist") {
		t.Errorf("message: %q", err)
	}
}

func TestResolveOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "art.procreate")
	cases := []struct {
		name, in, out string
		hasOut        bool
		want          OutputKind
		wantPath      string
		wantErr       string
	}{
		{name: "omitted", in: in, want: OutStdout},
		{name: "dash", in: in, out: "-", hasOut: true, want: OutStdout},
		{name: "plain file", in: in, out: "x.mp4", hasOut: true, want: OutFile, wantPath: "x.mp4"},
		{name: "missing dir slash", in: in, out: "no/such/", hasOut: true, wantErr: "output directory does not exist"},
		{name: "stdin to dir", in: "-", out: dir + "/", hasOut: true, wantErr: "cannot derive an output file name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, err := ResolveOutput(tc.in, tc.out, tc.hasOut)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if o.Kind != tc.want {
				t.Fatalf("kind = %v, want %v", o.Kind, tc.want)
			}
			if tc.wantPath != "" && o.Path != tc.wantPath {
				t.Fatalf("path = %s, want %s", o.Path, tc.wantPath)
			}
		})
	}
	t.Run("existing dir derives name", func(t *testing.T) {
		o, err := ResolveOutput(in, dir+"/", true)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		want := filepath.Join(dir, "art.mp4")
		if o.Kind != OutFile || o.Path != want {
			t.Fatalf("got {%v %s}, want file %s", o.Kind, o.Path, want)
		}
	})
}

func TestListReport(t *testing.T) {
	entries := map[string][]byte{
		"video/segments/segment-2.mp4":  testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
		"video/segments/segment-10.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
	}
	in := testkit.WriteArchive(t, entries, false)
	report, err := List(context.Background(), discardLog(), in, Config{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := "input: " + in + "\n" +
		"segments: 2\n" +
		"\n" +
		"2  video/segments/segment-2.mp4\n" +
		"10 video/segments/segment-10.mp4\n"
	if report != want {
		t.Fatalf("report:\n%s\nwant:\n%s", report, want)
	}
}

func TestListEmpty(t *testing.T) {
	in := testkit.WriteArchive(t, map[string][]byte{"x": {1}}, false)
	report, err := List(context.Background(), discardLog(), in, Config{})
	if !errors.Is(err, procreate.ErrNoSegments) {
		t.Fatalf("err = %v, want ErrNoSegments", err)
	}
	want := "input: " + in + "\nsegments: 0\n"
	if report != want {
		t.Fatalf("report:\n%q\nwant:\n%q", report, want)
	}
}

func TestVerifyOk(t *testing.T) {
	in := twoSegArchive(t, false)
	report, err := Verify(context.Background(), discardLog(), in, Config{})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	want := "input: " + in + "\n" +
		"segments: 2\n" +
		"\n" +
		"1 ok    video h264 320x240 yuv420p, audio aac 44100Hz 2ch  2.00s  video/segments/segment-1.mp4\n" +
		"2 ok    video h264 320x240 yuv420p, audio aac 44100Hz 2ch  1.00s  video/segments/segment-2.mp4\n" +
		"\n" +
		"verify: ok, 2 segment(s), ~3.0 s of video\n"
	if report != want {
		t.Fatalf("report:\n%q\nwant:\n%q", report, want)
	}
}

func TestVerifyBadSegment(t *testing.T) {
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
		"video/segments/segment-2.mp4": []byte("garbage"),
	}
	in := testkit.WriteArchive(t, entries, false)
	report, err := Verify(context.Background(), discardLog(), in, Config{})
	if !errors.Is(err, procreate.ErrBadSegment) {
		t.Fatalf("err = %v, want ErrBadSegment", err)
	}
	if !strings.Contains(err.Error(), "verify failed: 1 of 2 segment(s) are bad") {
		t.Errorf("message: %q", err)
	}
	if !strings.Contains(report, "2 FAIL") {
		t.Errorf("report missing FAIL line:\n%s", report)
	}
}

func TestVerifyIncompatible(t *testing.T) {
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
		"video/segments/segment-2.mp4": testkit.Segment(640, 480, []uint32{100}, []uint32{50}),
	}
	in := testkit.WriteArchive(t, entries, false)
	var inc *IncompatibleError
	_, err := Verify(context.Background(), discardLog(), in, Config{})
	if !errors.As(err, &inc) {
		t.Fatalf("err = %v, want IncompatibleError", err)
	}
}

// stdinPipe points os.Stdin at a pipe carrying data (restored on return).
func stdinPipe(t *testing.T, data []byte) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	old := os.Stdin
	os.Stdin = os.NewFile(r.Fd(), "stdin")
	t.Cleanup(func() { os.Stdin = old })
}

func TestListViaStdin(t *testing.T) {
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100}, []uint32{50}),
	}
	stdinPipe(t, testkit.ArchiveBytes(entries, false))
	report, err := List(context.Background(), discardLog(), "-", Config{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := "input: <stdin>\nsegments: 1\n\n1 video/segments/segment-1.mp4\n"
	if report != want {
		t.Fatalf("report:\n%q\nwant:\n%q", report, want)
	}
}

func TestSpoolEmptyStdin(t *testing.T) {
	stdinPipe(t, nil)
	_, err := List(context.Background(), discardLog(), "-", Config{})
	if !errors.Is(err, procreate.ErrInput) {
		t.Fatalf("err = %v, want ErrInput", err)
	}
	if !strings.Contains(err.Error(), "stdin is empty: <stdin>") {
		t.Errorf("message: %q", err)
	}
}

func TestMdatHead(t *testing.T) {
	h, err := mp4.MdatHead(7)
	if err != nil {
		t.Fatal(err)
	}
	if got := int(be32(h)); got != 15 {
		t.Errorf("size = %d, want 15", got)
	}
	if string(h[4:8]) != "mdat" {
		t.Errorf("type = %q", h[4:8])
	}
	big, err := mp4.MdatHead(1 << 32)
	if err != nil {
		t.Fatal(err)
	}
	if len(big) != 16 || be32(big) != 1 || be64(big[4:12]) != 1<<32+8 {
		t.Errorf("largesize header: % x", big)
	}
	if string(big[12:16]) != "mdat" {
		t.Errorf("largesize type: %q", big[12:16])
	}
}

type epipeWriter struct{ n int }

func (e epipeWriter) Write(p []byte) (int, error) { e.n++; return 0, syscall.EPIPE }

func TestClassifyWrite(t *testing.T) {
	var we *WriteError
	err := classifyWrite(context.Background(), syscall.EPIPE)
	if !errors.As(err, &we) || we.Msg != "failed to write to stdout: broken pipe" {
		t.Errorf("EPIPE -> %v", err)
	}
	err = classifyWrite(context.Background(), syscall.ENOSPC)
	if !errors.As(err, &we) || we.Msg != "failed to write the output: No space left on device" {
		t.Errorf("ENOSPC -> %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := classifyWrite(ctx, io.ErrClosedPipe); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx -> %v", err)
	}
}

func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func be64(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}
