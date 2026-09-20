package video

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"procreepy/internal/procreate"
	"procreepy/internal/testkit"
)

func splitArchive(t *testing.T) string {
	t.Helper()
	canvas := make([]byte, 4096)
	for i := range canvas {
		canvas[i] = byte(i % 251)
	}
	entries := map[string][]byte{
		"video/segments/segment-1.mp4": testkit.Segment(320, 240, []uint32{100, 200}, []uint32{50, 60}),
		"video/segments/segment-2.mp4": testkit.Segment(320, 240, []uint32{150}, []uint32{70}),
		"Info.plist":                   []byte("<plist/>"),
		"Canvas/layer.bin":             canvas,
	}
	return testkit.WriteArchive(t, entries, false)
}

func TestConvertSplit(t *testing.T) {
	in := splitArchive(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.mp4")
	slim := filepath.Join(dir, "out.procreepy.procreate")

	if _, err := Convert(context.Background(), discardLog(), in,
		Output{Kind: OutFile, Path: out, Name: out}, Config{Split: true}, true); err != nil {
		t.Fatalf("convert: %v", err)
	}
	checkConcat(t, parseMP4(t, out))

	// The slimmed archive must exist, be a valid archive without video/, and
	// keep the rest byte-for-byte.
	a, err := procreate.Open(slim, slim)
	if err != nil {
		t.Fatalf("open slim: %v", err)
	}
	defer a.Close()
	if _, err := a.Segments(procreate.Options{}); !errors.Is(err, procreate.ErrNoSegments) {
		t.Fatalf("slim segments: err = %v, want ErrNoSegments", err)
	}
	orig, err := procreate.Open(in, in)
	if err != nil {
		t.Fatal(err)
	}
	defer orig.Close()

	var kept []string
	for _, m := range orig.Members() {
		if m.IsDir {
			continue
		}
		if strings.HasPrefix(strings.ToLower(m.Name), "video/") {
			continue
		}
		kept = append(kept, m.Name)
	}
	if len(kept) != 2 {
		t.Fatalf("original non-video members: %v", kept)
	}
	for _, m := range a.Members() {
		if m.IsDir {
			t.Fatalf("unexpected dir entry in slim: %s", m.Name)
		}
		got, err := a.ReadMember(m.Name)
		if err != nil {
			t.Fatal(err)
		}
		want, err := orig.ReadMember(m.Name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("slim %s: bytes differ", m.Name)
		}
	}
	if len(a.Members()) != 2 {
		t.Errorf("slim members: %v, want the two non-video ones", a.Members())
	}
}

func TestConvertSplitStdoutRejected(t *testing.T) {
	in := twoSegArchive(t, false)
	_, err := Convert(context.Background(), discardLog(), in, Output{Kind: OutStdout},
		Config{Split: true}, true)
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want UsageError", err)
	}
	if !strings.Contains(err.Error(), "--split") {
		t.Errorf("message missing --split: %s", err)
	}
}

func TestSlimPath(t *testing.T) {
	cases := map[string]string{
		"/x/artwork.mp4": "/x/artwork.procreepy.procreate",
		"x/out.mp4":      "x/out.procreepy.procreate",
		"/x/custom-name": "/x/custom-name.procreepy.procreate",
	}
	for in, want := range cases {
		if got := slimPath(in); got != want {
			t.Errorf("slimPath(%q) = %q, want %q", in, got, want)
		}
	}
}
