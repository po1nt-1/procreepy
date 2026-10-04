package silica_test

import (
	"context"
	"image"
	"image/color"
	"testing"

	"procreepy/internal/procreate"
	"procreepy/internal/silica"
	"procreepy/internal/testkit"
)

// openProject writes a synthetic project and opens it as a document source.
func openProject(t *testing.T) (*procreate.Archive, *silica.Document) {
	t.Helper()
	p := testkit.Project(1).Write(t, t.TempDir(), "in.procreate")
	a, err := procreate.Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	doc, err := silica.ReadDocument(a)
	if err != nil {
		t.Fatalf("ReadDocument: %v", err)
	}
	return a, doc
}

func TestReadDocument(t *testing.T) {
	_, doc := openProject(t)

	if doc.Width != testkit.CanvasW || doc.Height != testkit.CanvasH {
		t.Errorf("canvas = %dx%d, want %dx%d", doc.Width, doc.Height, testkit.CanvasW, testkit.CanvasH)
	}
	if doc.TileSize != testkit.TileSize {
		t.Errorf("tile size = %d, want %d", doc.TileSize, testkit.TileSize)
	}
	if doc.DPI != 132 {
		t.Errorf("dpi = %d, want 132", doc.DPI)
	}
	if doc.ICCName != "sRGB IEC61966-2.1" {
		t.Errorf("icc name = %q", doc.ICCName)
	}
	if len(doc.ICCProfile) == 0 {
		t.Error("icc profile was not carried through")
	}
	if want := (color.NRGBA{R: 255, G: 255, B: 255, A: 255}); doc.Background != want {
		t.Errorf("background = %v, want %v", doc.Background, want)
	}
	if doc.BackgroundHidden {
		t.Error("background should be visible")
	}
	if len(doc.UnmappedBlends) != 0 {
		t.Errorf("unmapped blends = %v, want none", doc.UnmappedBlends)
	}

	// The tree is one group containing one raster layer.
	if len(doc.Layers) != 1 {
		t.Fatalf("top-level nodes = %d, want 1", len(doc.Layers))
	}
	g := doc.Layers[0]
	if !g.IsGroup {
		t.Fatal("the top-level node should be a group")
	}
	if g.Name != "Group 1" {
		t.Errorf("group name = %q, want %q", g.Name, "Group 1")
	}
	if len(g.Children) != 1 {
		t.Fatalf("group children = %d, want 1", len(g.Children))
	}
	l := g.Children[0]
	if l.IsGroup {
		t.Error("child should be a raster layer")
	}
	if l.Name != "Ink" {
		t.Errorf("layer name = %q, want %q", l.Name, "Ink")
	}
	if l.Hidden || l.Clipped {
		t.Errorf("layer flags: hidden=%v clipped=%v, want both false", l.Hidden, l.Clipped)
	}
	if l.Opacity != 1 {
		t.Errorf("opacity = %v, want 1", l.Opacity)
	}
	if l.Raster == nil || l.Raster.UUID != testkit.LayerUUID() {
		t.Errorf("raster = %+v, want UUID %s", l.Raster, testkit.LayerUUID())
	}

	// The composite is exposed separately and must not appear in the stack: it is
	// Procreate's flattened render, not a user layer.
	if doc.Composite == nil || doc.Composite.UUID != testkit.CompositeUUID() {
		t.Errorf("composite = %+v", doc.Composite)
	}
	if flat := doc.FlatLayers(); len(flat) != 1 {
		t.Errorf("FlatLayers = %d entries, want 1", len(flat))
	}
}

// TestImageUnpremultiplies checks the tile decode path end to end, including the
// grid mapping and the premultiplied-to-straight alpha conversion.
func TestImageUnpremultiplies(t *testing.T) {
	_, doc := openProject(t)
	layer := doc.FlatLayers()[0]

	img, err := doc.Image(context.Background(), layer.Raster)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != testkit.CanvasW {
		t.Errorf("image width = %d, want %d", got, testkit.CanvasW)
	}

	if got := img.Bounds().Dy(); got != testkit.CanvasH {
		t.Errorf("image height = %d, want %d", got, testkit.CanvasH)
	}

	// The top rows are the opaque ink colour, round-tripped through
	// premultiplication. Sampling the far column as well as the near one proves
	// the grid is assembled across, not just down.
	want := color.NRGBA{R: 0x20, G: 0x40, B: 0x60, A: 0xff}
	for _, p := range [][2]int{{1, 1}, {testkit.CanvasW - 1, 1}, {1, testkit.TileSize + 1}} {
		if got := img.NRGBAAt(p[0], p[1]); got != want {
			t.Errorf("opaque pixel at (%d,%d) = %v, want %v", p[0], p[1], got, want)
		}
	}

	// The last row of tiles is half-transparent; un-premultiplying an 8-bit value
	// cannot be exact, so allow the rounding the conversion necessarily
	// introduces.
	got := img.NRGBAAt(1, (testkit.GridRows-1)*testkit.TileSize+1)
	tr := color.NRGBA{R: 0xc0, G: 0x10, B: 0x30, A: 0x80}
	if got.A != tr.A {
		t.Errorf("translucent alpha = %d, want %d", got.A, tr.A)
	}
	for _, c := range []struct {
		name      string
		got, want uint8
	}{{"R", got.R, tr.R}, {"G", got.G, tr.G}, {"B", got.B, tr.B}} {
		if diff(c.got, c.want) > 2 {
			t.Errorf("translucent %s = %d, want ~%d", c.name, c.got, c.want)
		}
	}
}

// TestImageMissingTilesAreTransparent: Procreate stores the grid sparsely, so a
// position with no tile is untouched canvas rather than an error.
func TestImageMissingTilesAreTransparent(t *testing.T) {
	_, doc := openProject(t)
	blank := &silica.Raster{UUID: "00000000-0000-0000-0000-000000000000"}
	img, err := doc.Image(context.Background(), blank)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]int{{0, 0}, {testkit.CanvasW - 1, testkit.CanvasH - 1}} {
		if a := img.NRGBAAt(p[0], p[1]).A; a != 0 {
			t.Errorf("pixel (%d,%d) alpha = %d, want 0", p[0], p[1], a)
		}
	}
}

func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestRasterBounds checks the tight bounding box, which is what keeps exported
// layer records small.
func TestRasterBounds(t *testing.T) {
	_, doc := openProject(t)
	layer := doc.FlatLayers()[0]
	b, err := doc.RasterBounds(context.Background(), layer.Raster)
	if err != nil {
		t.Fatal(err)
	}
	// The synthetic layer inks its whole grid, so the bounds are the canvas.
	want := image.Rect(0, 0, testkit.CanvasW, testkit.CanvasH)
	if b != want {
		t.Errorf("bounds = %s, want %s", b, want)
	}

	// A layer with no tiles at all has empty bounds, not an error.
	empty := &silica.Raster{UUID: "00000000-0000-0000-0000-000000000000"}
	eb, err := doc.RasterBounds(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	if !eb.Empty() {
		t.Errorf("bounds of a blank layer = %s, want empty", eb)
	}
}

func TestReadDocumentMissingMember(t *testing.T) {
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "video/segments/segment-1.mp4", Data: []byte("v")},
	}}
	p := a.Write(t, t.TempDir(), "in.procreate")
	arch, err := procreate.Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer arch.Close()
	if _, err := silica.ReadDocument(arch); err == nil {
		t.Fatal("want an error when Document.archive is absent")
	}
}

func TestReadDocumentGarbage(t *testing.T) {
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "Document.archive", Data: []byte("bplist00 but truncated nonsense")},
	}}
	p := a.Write(t, t.TempDir(), "in.procreate")
	arch, err := procreate.Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer arch.Close()
	if _, err := silica.ReadDocument(arch); err == nil {
		t.Fatal("want an error for a malformed Document.archive")
	}
}

// TestCancelledImage stops before decoding rather than producing a partial
// canvas that would look like a successful read.
func TestCancelledImage(t *testing.T) {
	_, doc := openProject(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := doc.Image(ctx, doc.FlatLayers()[0].Raster); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := doc.RasterBounds(ctx, doc.FlatLayers()[0].Raster); err != context.Canceled {
		t.Fatalf("bounds err = %v, want context.Canceled", err)
	}
}

func TestBlendKey(t *testing.T) {
	if k, ok := silica.BlendKey(0); k != "norm" || !ok {
		t.Errorf("blend 0 = %q, %v", k, ok)
	}
	if k, ok := silica.BlendKey(1); k != "mul " || !ok {
		t.Errorf("blend 1 = %q, %v", k, ok)
	}
	// An index outside the table falls back to Normal but reports itself so the
	// caller can warn instead of silently writing the wrong mode.
	if k, ok := silica.BlendKey(9999); k != "norm" || ok {
		t.Errorf("unknown blend = %q, %v; want norm, false", k, ok)
	}
}

// TestUnmappedBlendsReported: a layer using a blend index this build does not
// know about must be surfaced, not silently downgraded.
func TestUnmappedBlendsReported(t *testing.T) {
	raw := testkit.DocumentArchiveBlend(testkit.CanvasW, testkit.CanvasH, 9001)
	a := testkit.Archive{ProcreateStyle: true, Entries: []testkit.Entry{
		{Name: "Document.archive", Data: raw},
	}}
	p := a.Write(t, t.TempDir(), "in.procreate")
	arch, err := procreate.Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer arch.Close()
	doc, err := silica.ReadDocument(arch)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.UnmappedBlends) != 1 || doc.UnmappedBlends[0] != 9001 {
		t.Errorf("UnmappedBlends = %v, want [9001]", doc.UnmappedBlends)
	}
}
