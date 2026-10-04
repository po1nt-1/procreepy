package silica

import "image"

// transform maps the stored canvas onto the artwork as Procreate presents it.
//
// Procreate keeps layer tiles in a fixed canvas space and records the
// presentation separately: a quarter-turn count in the document's orientation
// field, plus two mirror flags. The rotation mapping was derived by comparing
// every project in a real corpus against the thumbnail Procreate stored for it,
// and it agreed for all of them:
//
//	orientation 1 -> 270 degrees, 2 -> 90, 3 -> 180, 4 -> none
//
// An unknown orientation falls back to no rotation, which leaves the pixels
// alone rather than turning them the wrong way.
type transform struct {
	rot   int // quarter-turns clockwise, applied before the mirrors
	flipH bool
	flipV bool
}

func newTransform(orientation int, flipH, flipV bool) transform {
	rot := 0
	switch orientation {
	case 1:
		rot = 3
	case 2:
		rot = 1
	case 3:
		rot = 2
	}
	return transform{rot: rot, flipH: flipH, flipV: flipV}
}

// swapsAxes reports whether the transform exchanges width and height.
func (t transform) swapsAxes() bool { return t.rot%2 == 1 }

// dims maps canvas dimensions to display dimensions.
func (t transform) dims(w, h int) (int, int) {
	if t.swapsAxes() {
		return h, w
	}
	return w, h
}

// rotatePoint turns a point in a w x h image by rot quarter-turns clockwise.
func rotatePoint(x, y, w, h, rot int) (int, int) {
	switch rot {
	case 1:
		return h - 1 - y, x
	case 2:
		return w - 1 - x, h - 1 - y
	case 3:
		return y, w - 1 - x
	default:
		return x, y
	}
}

// point maps a canvas pixel to its display position; w and h are the canvas
// dimensions. The mirrors act on the rotated image, which is the order
// Procreate's own thumbnails are consistent with.
func (t transform) point(x, y, w, h int) (int, int) {
	nx, ny := rotatePoint(x, y, w, h, t.rot)
	dw, dh := t.dims(w, h)
	if t.flipH {
		nx = dw - 1 - nx
	}
	if t.flipV {
		ny = dh - 1 - ny
	}
	return nx, ny
}

// pointToCanvas is the inverse of point. The mirrors come off first because they
// were applied last, and only then is the rotation undone; doing it the other
// way round is wrong for the quarter-turns, where a mirror and a rotation do not
// commute.
func (t transform) pointToCanvas(x, y, w, h int) (int, int) {
	dw, dh := t.dims(w, h)
	if t.flipH {
		x = dw - 1 - x
	}
	if t.flipV {
		y = dh - 1 - y
	}
	return rotatePoint(x, y, dw, dh, (4-t.rot)%4)
}

// rectToCanvas maps a display-space rectangle back to canvas space, so a cropped
// read can work out which tiles it needs. Rotations and mirrors map corners to
// corners, so transforming the two extreme points and re-normalising is exact.
func (t transform) rectToCanvas(r image.Rectangle, canvasW, canvasH int) image.Rectangle {
	if r.Empty() {
		return image.Rectangle{}
	}
	x0, y0 := t.pointToCanvas(r.Min.X, r.Min.Y, canvasW, canvasH)
	x1, y1 := t.pointToCanvas(r.Max.X-1, r.Max.Y-1, canvasW, canvasH)
	return normalise(x0, y0, x1, y1)
}

// rectToDisplay maps a canvas-space rectangle into display space.
func (t transform) rectToDisplay(r image.Rectangle, canvasW, canvasH int) image.Rectangle {
	if r.Empty() {
		return image.Rectangle{}
	}
	x0, y0 := t.point(r.Min.X, r.Min.Y, canvasW, canvasH)
	x1, y1 := t.point(r.Max.X-1, r.Max.Y-1, canvasW, canvasH)
	return normalise(x0, y0, x1, y1)
}

func normalise(x0, y0, x1, y1 int) image.Rectangle {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	return image.Rect(x0, y0, x1+1, y1+1)
}
