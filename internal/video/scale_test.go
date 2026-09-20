package video

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"procreepy/internal/mp4"
	"procreepy/internal/testkit"
)

// TestConvertLargeArchive converts ~512 MiB of media (500 × 1 MiB segments)
// end-to-end and asserts that live heap stays flat while doing so: media data
// must be streamed, not buffered, which is what makes tens-of-GB archives
// practical.
func TestConvertLargeArchive(t *testing.T) {
	if testing.Short() {
		t.Skip("large archive; skipped under -short")
	}
	const n = 500
	const perSeg = 1 << 20 // 1 MiB media per segment
	videoSizes := []uint32{perSeg / 2, perSeg / 2}
	audioSizes := []uint32{1024}
	in, media := testkit.WriteSegmentArchive(t, n, false, videoSizes, audioSizes)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	out := filepath.Join(t.TempDir(), "big.mp4")
	dur, err := Convert(context.Background(), quietLog(), in,
		Output{Kind: OutFile, Path: out, Name: out}, Config{}, false)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if want := float64(n*60) / 30; dur != want {
		t.Errorf("duration = %v, want %v", dur, want)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	m, err := mp4.Parse(f, fi.Size())
	if err != nil {
		t.Fatalf("output does not parse: %v", err)
	}
	if len(m.Tracks) != 2 {
		t.Fatalf("tracks = %d, want 2", len(m.Tracks))
	}
	if int(m.Tracks[0].SampleCount) != 2*n {
		t.Fatalf("video samples = %d, want %d", m.Tracks[0].SampleCount, 2*n)
	}
	if len(m.Mdat) != n {
		t.Fatalf("mdat boxes = %d, want %d", len(m.Mdat), n)
	}
	if fi.Size() < media {
		t.Errorf("output is %d bytes, smaller than its %d bytes of media", fi.Size(), media)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	const capDelta = 128 << 20 // index tables + reader overhead, never the media
	if delta := int64(after.HeapAlloc) - int64(before.HeapAlloc); delta > capDelta {
		t.Errorf("heap grew by %d MiB while converting %d MiB of media; media must stream", delta>>20, media>>20)
	}
}
