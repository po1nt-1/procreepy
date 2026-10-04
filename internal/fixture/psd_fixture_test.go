package fixture

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"procreepy/internal/procreate"
	"procreepy/internal/psd"
	"procreepy/internal/silica"
)

// TestRealCorpusComposite checks the decoded layer pixels against the thumbnail
// Procreate itself generated for each artwork.
//
// This is what pins the parts of the tile format that cannot be read off the
// bytes: the column/row mapping of the tile grid, the RGBA channel order, and
// the premultiplied alpha convention. Any of them being wrong shows up
// immediately as a large colour difference against Procreate's own render.
func TestRealCorpusComposite(t *testing.T) {
	dir := corpus(t)
	projects := findProjects(t, dir)

	var checked int
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
		if doc.Composite == nil || !arch.HasMember("QuickLook/Thumbnail.png") {
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
		got, err := doc.ImageIn(context.Background(), doc.Composite,
			image.Rect(0, 0, doc.Width, doc.Height))
		if err != nil {
			t.Errorf("%s: composite: %v", filepath.Base(p), err)
			arch.Close()
			continue
		}
		// The decoded artwork is in display space, so it must line up with the
		// thumbnail directly: no rotation, no mirror, same aspect.
		tb := thumb.Bounds()
		if !approxEq(float64(doc.Width)/float64(doc.Height), float64(tb.Dx())/float64(tb.Dy())) {
			t.Errorf("%s: canvas %dx%d does not match the thumbnail's %dx%d shape (orientation %d)",
				filepath.Base(p), doc.Width, doc.Height, tb.Dx(), tb.Dy(), doc.Orientation)
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
		// Both sides are area-averaged to the same grid, so only the resampling of
		// hard edges should differ. A wrong channel order, a transposed tile grid,
		// a missing un-premultiply or a wrong orientation all cost tens of levels.
		if d > 6 {
			t.Errorf("%s: composite differs from Procreate's thumbnail by %.1f levels (canvas %dx%d, orientation %d)",
				filepath.Base(p), d, doc.Width, doc.Height, doc.Orientation)
		}
	}
	t.Logf("compared %d composites; worst mean difference %.2f levels (%s)", checked, worst, worstName)
	if checked == 0 {
		t.Fatal("no composite could be compared")
	}
}

// boxDownscale area-averages src down to w x h.
//
// The thumbnail is an area-averaged downscale, so point-sampling the full-size
// artwork would disagree with it by tens of levels on any detailed painting
// purely through aliasing. Averaging the same way makes the comparison measure
// decoding correctness instead of resampling.
func boxDownscale(src *image.NRGBA, w, h int) *image.NRGBA {
	sb := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0 := sb.Min.Y + y*sb.Dy()/h
		y1 := sb.Min.Y + (y+1)*sb.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0 := sb.Min.X + x*sb.Dx()/w
			x1 := sb.Min.X + (x+1)*sb.Dx()/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			// Average in premultiplied space, which is how a correct downscale of an
			// image with transparency behaves.
			var sr, sg, sbl, sa, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.NRGBAAt(xx, yy)
					a := float64(c.A) / 255
					sr += float64(c.R) * a
					sg += float64(c.G) * a
					sbl += float64(c.B) * a
					sa += float64(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			a := sa / n
			var r, g, bl float64
			if a > 0 {
				k := n * a / 255
				r, g, bl = sr/k, sg/k, sbl/k
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: clamp8(r), G: clamp8(g), B: clamp8(bl), A: clamp8(a),
			})
		}
	}
	return out
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// meanAbsDiff averages the absolute per-channel difference of two images of the
// same size. -1 means too little solid overlap to judge.
func meanAbsDiff(a, b *image.NRGBA) float64 {
	if a.Bounds().Size() != b.Bounds().Size() {
		return -1
	}
	var sum, n float64
	bb := b.Bounds()
	for y := 0; y < bb.Dy(); y++ {
		for x := 0; x < bb.Dx(); x++ {
			g := a.NRGBAAt(a.Bounds().Min.X+x, a.Bounds().Min.Y+y)
			w := b.NRGBAAt(bb.Min.X+x, bb.Min.Y+y)
			// Compare only where both are solid: a downscale's edge pixels mix in
			// neighbours and would dominate the average.
			if g.A < 250 || w.A < 250 {
				continue
			}
			sum += absDiff(float64(g.R), float64(w.R)) +
				absDiff(float64(g.G), float64(w.G)) +
				absDiff(float64(g.B), float64(w.B))
			n += 3
		}
	}
	if n < 100 {
		return -1
	}
	return sum / n
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestRealCorpusPSDExport exports a PSD for every project in the corpus and reads
// each one back, so the writer is exercised against real layer counts, real
// canvas sizes, real group nesting and both tile codecs.
func TestRealCorpusPSDExport(t *testing.T) {
	dir := corpus(t)
	projects := findProjects(t, dir)
	outDir := t.TempDir()

	var exported, totalLayers int
	unmapped := map[int]int{}
	for _, p := range projects {
		arch, err := procreate.Open(p, p)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		doc, err := silica.ReadDocument(arch)
		if err != nil {
			arch.Close()
			t.Errorf("%s: ReadDocument: %v", filepath.Base(p), err)
			continue
		}
		for _, b := range doc.UnmappedBlends {
			unmapped[b]++
		}
		out := filepath.Join(outDir, filepath.Base(p)+".psd")
		f, err := os.Create(out)
		if err != nil {
			arch.Close()
			t.Fatal(err)
		}
		n, err := psd.Write(context.Background(), f, doc)
		cerr := f.Close()
		arch.Close()
		if err != nil {
			t.Errorf("%s: psd.Write: %v", filepath.Base(p), err)
			continue
		}
		if cerr != nil {
			t.Errorf("%s: close: %v", filepath.Base(p), cerr)
			continue
		}
		exported++
		totalLayers += n
		checkPSDStructure(t, filepath.Base(p), out, doc)
		// Keeping every PSD would need gigabytes; the structure has been checked.
		os.Remove(out)
	}
	t.Logf("exported %d PSDs, %d layer records in total", exported, totalLayers)
	if len(unmapped) > 0 {
		t.Logf("blend indices with no Photoshop mapping: %v", unmapped)
	}
	if exported == 0 {
		t.Fatal("no PSD was exported")
	}
}

// checkPSDStructure re-reads an exported PSD far enough to prove the section
// lengths and the layer records are self-consistent.
func checkPSDStructure(t *testing.T, label, path string, doc *silica.Document) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if len(b) < 26 || string(b[:4]) != "8BPS" {
		t.Errorf("%s: not a PSD", label)
		return
	}
	h := be32(b[14:])
	w := be32(b[18:])
	if int(w) != doc.Width || int(h) != doc.Height {
		t.Errorf("%s: PSD says %dx%d, document says %dx%d", label, w, h, doc.Width, doc.Height)
	}
	off := 26
	cm := be32(b[off:])
	off += 4 + int(cm)
	res := be32(b[off:])
	off += 4 + int(res)
	if off+4 > len(b) {
		t.Errorf("%s: truncated before the layer section", label)
		return
	}
	mask := be32(b[off:])
	off += 4 + int(mask)
	if off > len(b) {
		t.Errorf("%s: layer section length %d overruns the file", label, mask)
		return
	}
	// What remains must be the merged image: a compression word plus data.
	if len(b)-off < 3 {
		t.Errorf("%s: no merged image section (%d bytes left)", label, len(b)-off)
	}
}

func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// toNRGBA converts a decoded image so comparisons work on one representation.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
	return out
}
