package fixture

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"testing"

	"procreepy/internal/procreate"
	"procreepy/internal/psd"
	"procreepy/internal/silica"
)

// blendNormal is Procreate's index for the Normal blend mode; see
// internal/silica/blend.go.
const blendNormal = 0

// TestRealCorpusLayerOrder pins the order of the layer stack by compositing it
// and comparing the result against the thumbnail Procreate stored.
//
// This is the only check that can catch an inverted stack. TestRealCorpusComposite
// compares doc.Composite, which the archive supplies already flattened, so it
// looks correct whatever order the layers are read in — and the synthetic
// documents in internal/testkit carry a single layer inside a single group,
// where order is unobservable. An inversion therefore slipped past every other
// test while breaking every exported PSD.
//
// Procreate archives the layer array top-most first; silica flips it so that
// Document.Layers is bottom-most first. Reading that convention backwards both
// inverts the artwork and destroys every group, because Photoshop encodes a
// group as a bounding divider below its contents and a folder record above them.
func TestRealCorpusLayerOrder(t *testing.T) {
	dir := corpus(t)
	projects := findProjects(t, dir)

	var checked, skipped int
	var worst float64
	worstName := ""
	for _, p := range projects {
		arch, err := procreate.Open(p, p)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		doc, err := silica.ReadDocument(arch)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			arch.Close()
			continue
		}
		if !arch.HasMember("QuickLook/Thumbnail.png") || !orderObservable(doc) {
			skipped++
			arch.Close()
			continue
		}
		raw, err := arch.ReadMember("QuickLook/Thumbnail.png")
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			arch.Close()
			continue
		}
		thumb, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Errorf("%s: thumbnail: %v", filepath.Base(p), err)
			arch.Close()
			continue
		}
		canvas := image.Rect(0, 0, doc.Width, doc.Height)
		got, err := psd.CompositeStack(context.Background(), doc, canvas)
		if err != nil {
			t.Errorf("%s: composite stack: %v", filepath.Base(p), err)
			arch.Close()
			continue
		}
		tb := thumb.Bounds()
		if !approxEq(float64(doc.Width)/float64(doc.Height), float64(tb.Dx())/float64(tb.Dy())) {
			skipped++
			arch.Close()
			continue
		}
		d := meanAbsDiff(boxDownscale(got, tb.Dx(), tb.Dy()), toNRGBA(thumb))
		arch.Close()
		if d < 0 {
			continue
		}
		checked++
		if d > worst {
			worst, worstName = d, filepath.Base(p)
		}
		// Only documents whose visible layers all blend Normally are compared, so
		// the stack composite should land within resampling noise of Procreate's
		// own render. An inverted stack puts the wrong artwork on top and costs
		// far more than this threshold on anything but a uniform wash.
		if d > 10 {
			t.Errorf("%s: layer stack differs from Procreate's thumbnail by %.1f levels "+
				"(canvas %dx%d, %d visible layers) — the stack order is probably inverted",
				filepath.Base(p), d, doc.Width, doc.Height, visibleLayers(doc))
		}
	}
	t.Logf("compared %d stacks (%d skipped as unsuitable); worst mean difference %.2f levels (%s)",
		checked, skipped, worst, worstName)
	if checked == 0 {
		t.Skip("no artwork in the corpus has an observable, Normal-blended layer order")
	}
}

// orderObservable reports whether compositing this document's stack can be
// compared against Procreate's render at all.
//
// It needs at least two visible layers — one layer looks the same upside down —
// and every one of them has to blend Normally and unclipped, because
// psd.CompositeStack implements Normal only. A document using Multiply or a
// clipping mask would differ from Procreate's thumbnail for reasons that have
// nothing to do with ordering, which would make this test flaky instead of
// meaningful. Hidden groups are excluded for the same reason: the flattening
// walk returns their children regardless.
func orderObservable(doc *silica.Document) bool {
	ok := true
	var walk func(nodes []*silica.Node)
	walk = func(nodes []*silica.Node) {
		for _, n := range nodes {
			if n.IsGroup {
				if n.Hidden || n.Opacity < 1 {
					ok = false
					return
				}
				walk(n.Children)
				continue
			}
			if n.Hidden || n.Raster == nil {
				continue
			}
			if n.Blend != blendNormal || n.Clipped {
				ok = false
				return
			}
		}
	}
	walk(doc.Layers)
	return ok && visibleLayers(doc) >= 2
}

// visibleLayers counts the raster layers that contribute to the render.
func visibleLayers(doc *silica.Document) int {
	n := 0
	for _, l := range doc.FlatLayers() {
		if !l.Hidden && l.Raster != nil {
			n++
		}
	}
	return n
}
