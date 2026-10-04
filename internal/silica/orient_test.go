package silica

import (
	"image"
	"testing"
)

// TestTransformRoundTrip checks that point and pointToCanvas invert each other
// for every rotation and mirror combination. The inverse is what lets a cropped
// read work out which tiles it needs, and getting it wrong for the quarter-turns
// (where a mirror and a rotation do not commute) would silently read the wrong
// corner of the canvas.
func TestTransformRoundTrip(t *testing.T) {
	const w, h = 7, 4
	for rot := 0; rot < 4; rot++ {
		for _, fh := range []bool{false, true} {
			for _, fv := range []bool{false, true} {
				tr := transform{rot: rot, flipH: fh, flipV: fv}
				dw, dh := tr.dims(w, h)
				seen := map[[2]int]bool{}
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						nx, ny := tr.point(x, y, w, h)
						if nx < 0 || nx >= dw || ny < 0 || ny >= dh {
							t.Fatalf("rot=%d fh=%v fv=%v: (%d,%d) -> (%d,%d) outside %dx%d",
								rot, fh, fv, x, y, nx, ny, dw, dh)
						}
						if seen[[2]int{nx, ny}] {
							t.Fatalf("rot=%d fh=%v fv=%v: (%d,%d) collides", rot, fh, fv, nx, ny)
						}
						seen[[2]int{nx, ny}] = true
						bx, by := tr.pointToCanvas(nx, ny, w, h)
						if bx != x || by != y {
							t.Fatalf("rot=%d fh=%v fv=%v: (%d,%d) -> (%d,%d) -> (%d,%d)",
								rot, fh, fv, x, y, nx, ny, bx, by)
						}
					}
				}
				if len(seen) != w*h {
					t.Fatalf("rot=%d fh=%v fv=%v: covered %d of %d pixels", rot, fh, fv, len(seen), w*h)
				}
			}
		}
	}
}

func TestTransformFromOrientation(t *testing.T) {
	// Derived by comparing a real corpus against the thumbnails Procreate stored.
	cases := map[int]int{1: 3, 2: 1, 3: 2, 4: 0}
	for orientation, wantRot := range cases {
		if got := newTransform(orientation, false, false).rot; got != wantRot {
			t.Errorf("orientation %d -> rot %d, want %d", orientation, got, wantRot)
		}
	}
	// An orientation this build does not recognise must leave the pixels alone
	// rather than turn them the wrong way.
	if got := newTransform(99, false, false).rot; got != 0 {
		t.Errorf("unknown orientation -> rot %d, want 0", got)
	}
}

func TestTransformDims(t *testing.T) {
	for rot, swap := range map[int]bool{0: false, 1: true, 2: false, 3: true} {
		w, h := transform{rot: rot}.dims(10, 20)
		if swap && (w != 20 || h != 10) {
			t.Errorf("rot %d: dims = %dx%d, want 20x10", rot, w, h)
		}
		if !swap && (w != 10 || h != 20) {
			t.Errorf("rot %d: dims = %dx%d, want 10x20", rot, w, h)
		}
	}
}

func TestRectRoundTrip(t *testing.T) {
	const w, h = 9, 5
	r := image.Rect(2, 1, 7, 4)
	for rot := 0; rot < 4; rot++ {
		for _, fh := range []bool{false, true} {
			tr := transform{rot: rot, flipH: fh}
			d := tr.rectToDisplay(r, w, h)
			back := tr.rectToCanvas(d, w, h)
			if back != r {
				t.Errorf("rot=%d fh=%v: %v -> %v -> %v", rot, fh, r, d, back)
			}
		}
	}
}
