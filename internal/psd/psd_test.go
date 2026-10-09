package psd_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"procreepy/internal/procreate"
	"procreepy/internal/psd"
	"procreepy/internal/silica"
	"procreepy/internal/testkit"
)

// writePSD exports a synthetic project and returns the PSD bytes.
func writePSD(t *testing.T) ([]byte, *silica.Document) {
	t.Helper()
	p := testkit.Project(1).Write(t, t.TempDir(), "in.procreate")
	arch, err := procreate.Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	// The document reads tiles lazily, so the archive has to outlive the call:
	// the round-trip tests decode the same pixels again to compare against.
	t.Cleanup(func() { arch.Close() })
	doc, err := silica.ReadDocument(arch)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.psd")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	n, err := psd.Write(context.Background(), f, doc)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	// One group becomes two records (the folder marker and the bounding
	// divider) around its single raster layer.
	if n != 3 {
		t.Errorf("wrote %d layer records, want 3", n)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b, doc
}

// doc is a parsed PSD, enough of it to check what the writer promises.
type parsed struct {
	width, height int
	channels      int
	depth         int
	colorMode     int
	layers        []layer
	resources     map[int][]byte
}

type layer struct {
	top, left, bottom, right int32
	blend                    string
	opacity                  uint8
	clipping                 uint8
	flags                    uint8
	name                     string
	uniName                  string
	section                  int
	channelLens              map[int16]uint32
}

func (l layer) hidden() bool { return l.flags&(1<<1) != 0 }

// readPSD parses the parts of a PSD this package writes. It is a test-only
// reader: it exists so the exported structure is verified by decoding it rather
// than by comparing against bytes this same code produced.
func readPSD(t *testing.T, b []byte) parsed {
	t.Helper()
	r := &reader{t: t, b: b}
	if sig := string(r.take(4)); sig != "8BPS" {
		t.Fatalf("signature = %q, want 8BPS", sig)
	}
	if v := r.u16(); v != 1 {
		t.Fatalf("version = %d, want 1", v)
	}
	r.take(6)
	p := parsed{resources: map[int][]byte{}}
	p.channels = int(r.u16())
	p.height = int(r.u32())
	p.width = int(r.u32())
	p.depth = int(r.u16())
	p.colorMode = int(r.u16())

	if n := r.u32(); n != 0 {
		t.Errorf("colour mode section = %d bytes, want 0 for RGB", n)
	}

	resLen := int(r.u32())
	res := r.take(resLen)
	for len(res) >= 12 {
		if string(res[:4]) != "8BIM" {
			t.Fatalf("resource signature = %q", res[:4])
		}
		id := int(binary.BigEndian.Uint16(res[4:]))
		nameLen := int(res[6])
		// The Pascal name is padded to an even total length.
		skip := 6 + 1 + nameLen
		if skip%2 == 1 {
			skip++
		}
		size := int(binary.BigEndian.Uint32(res[skip:]))
		start := skip + 4
		p.resources[id] = res[start : start+size]
		adv := start + size
		if size%2 == 1 {
			adv++
		}
		res = res[adv:]
	}

	maskLen := int(r.u32())
	mask := r.take(maskLen)
	mr := &reader{t: t, b: mask}
	infoLen := int(mr.u32())
	info := mr.take(infoLen)
	ir := &reader{t: t, b: info}
	count := int(int16(ir.u16()))
	if count > 0 {
		t.Errorf("layer count = %d; want a negative count so the merged alpha is transparency", count)
	}
	if count < 0 {
		count = -count
	}
	for i := 0; i < count; i++ {
		var l layer
		l.top, l.left = int32(ir.u32()), int32(ir.u32())
		l.bottom, l.right = int32(ir.u32()), int32(ir.u32())
		nch := int(ir.u16())
		l.channelLens = map[int16]uint32{}
		for c := 0; c < nch; c++ {
			id := int16(ir.u16())
			l.channelLens[id] = ir.u32()
		}
		if sig := string(ir.take(4)); sig != "8BIM" {
			t.Fatalf("layer %d blend signature = %q", i, sig)
		}
		l.blend = string(ir.take(4))
		l.opacity = ir.take(1)[0]
		l.clipping = ir.take(1)[0]
		l.flags = ir.take(1)[0]
		ir.take(1) // filler
		extraLen := int(ir.u32())
		extra := ir.take(extraLen)
		er := &reader{t: t, b: extra}
		if n := er.u32(); n != 0 {
			t.Errorf("layer %d mask data = %d bytes, want 0", i, n)
		}
		if n := er.u32(); n != 0 {
			t.Errorf("layer %d blending ranges = %d bytes, want 0", i, n)
		}
		nameLen := int(er.take(1)[0])
		l.name = string(er.take(nameLen))
		// The name field, length byte included, is padded to a multiple of 4.
		if pad := (1 + nameLen) % 4; pad != 0 {
			er.take(4 - pad)
		}
		for er.left() >= 12 {
			if string(er.take(4)) != "8BIM" {
				break
			}
			key := string(er.take(4))
			n := int(er.u32())
			data := er.take(n)
			if n%2 == 1 {
				er.take(1)
			}
			switch key {
			case "luni":
				chars := int(binary.BigEndian.Uint32(data))
				u := make([]uint16, chars)
				for k := 0; k < chars; k++ {
					u[k] = binary.BigEndian.Uint16(data[4+2*k:])
				}
				l.uniName = utf16String(u)
			case "lsct":
				l.section = int(binary.BigEndian.Uint32(data))
			}
		}
		p.layers = append(p.layers, l)
	}
	return p
}

type reader struct {
	t *testing.T
	b []byte
	i int
}

func (r *reader) take(n int) []byte {
	r.t.Helper()
	if r.i+n > len(r.b) {
		r.t.Fatalf("PSD truncated: want %d bytes at offset %d, have %d", n, r.i, len(r.b)-r.i)
	}
	out := r.b[r.i : r.i+n]
	r.i += n
	return out
}
func (r *reader) u16() uint16 { return binary.BigEndian.Uint16(r.take(2)) }
func (r *reader) u32() uint32 { return binary.BigEndian.Uint32(r.take(4)) }
func (r *reader) left() int   { return len(r.b) - r.i }

func TestWriteHeaderAndResources(t *testing.T) {
	b, doc := writePSD(t)
	p := readPSD(t, b)

	if p.width != doc.Width || p.height != doc.Height {
		t.Errorf("size = %dx%d, want %dx%d", p.width, p.height, doc.Width, doc.Height)
	}
	if p.channels != 4 {
		t.Errorf("channels = %d, want 4", p.channels)
	}
	if p.depth != 8 {
		t.Errorf("depth = %d, want 8", p.depth)
	}
	if p.colorMode != 3 {
		t.Errorf("colour mode = %d, want 3 (RGB)", p.colorMode)
	}
	icc, ok := p.resources[1039]
	if !ok {
		t.Error("the ICC profile resource was not written")
	} else if !bytes.Equal(icc, doc.ICCProfile) {
		t.Error("the ICC profile was altered")
	}
	resn, ok := p.resources[1005]
	if !ok || len(resn) != 16 {
		t.Fatalf("resolution resource = %d bytes, want 16", len(resn))
	}
	if dpi := binary.BigEndian.Uint32(resn) >> 16; int(dpi) != doc.DPI {
		t.Errorf("resolution = %d dpi, want %d", dpi, doc.DPI)
	}
}

// TestGroupEncoding checks a group becomes the divider/contents/folder run
// Photoshop expects, bottom-most record first.
func TestGroupEncoding(t *testing.T) {
	b, _ := writePSD(t)
	p := readPSD(t, b)

	if len(p.layers) != 3 {
		t.Fatalf("layers = %d, want 3", len(p.layers))
	}
	if p.layers[0].section != 3 {
		t.Errorf("first record lsct = %d, want 3 (bounding section divider)", p.layers[0].section)
	}
	if p.layers[1].section != 0 {
		t.Errorf("middle record lsct = %d, want 0 (a raster layer)", p.layers[1].section)
	}
	if p.layers[2].section != 1 {
		t.Errorf("last record lsct = %d, want 1 (open folder)", p.layers[2].section)
	}
	if got := p.layers[2].uniName; got != "Group 1" {
		t.Errorf("folder name = %q, want %q", got, "Group 1")
	}
	if got := p.layers[1].uniName; got != "Ink" {
		t.Errorf("layer name = %q, want %q", got, "Ink")
	}
}

// TestLayerRecordFields pins the metadata a layer carries across.
func TestLayerRecordFields(t *testing.T) {
	b, _ := writePSD(t)
	p := readPSD(t, b)
	l := p.layers[1]

	if l.blend != "norm" {
		t.Errorf("blend = %q, want norm", l.blend)
	}
	if l.opacity != 255 {
		t.Errorf("opacity = %d, want 255", l.opacity)
	}
	if l.hidden() {
		t.Error("layer should be visible")
	}
	if l.clipping != 0 {
		t.Errorf("clipping = %d, want 0", l.clipping)
	}
	// Bounds are the tight ink box, not the whole canvas by default.
	if l.left != 0 || l.top != 0 || l.right != int32(testkit.CanvasW) || l.bottom != int32(testkit.CanvasH) {
		t.Errorf("bounds = (%d,%d)-(%d,%d)", l.left, l.top, l.right, l.bottom)
	}
	// All four channels must be declared, alpha included.
	for _, id := range []int16{-1, 0, 1, 2} {
		if _, ok := l.channelLens[id]; !ok {
			t.Errorf("channel %d is missing", id)
		}
	}
	// A group marker carries no pixels: its channels are the compression word only.
	for _, id := range []int16{-1, 0, 1, 2} {
		if got := p.layers[2].channelLens[id]; got != 2 {
			t.Errorf("group marker channel %d = %d bytes, want 2", id, got)
		}
	}
}

// TestHiddenAndOpacityAndBlend drives the fields through a document that sets
// them, since the default project leaves them at their neutral values.
func TestHiddenAndOpacityAndBlend(t *testing.T) {
	raw := testkit.DocumentArchiveLayer(testkit.CanvasW, testkit.CanvasH, testkit.LayerOpts{
		Hidden: true, Opacity: 0.5, Blend: 1, Clipped: true, Name: "Shade",
	})
	p := exportDoc(t, raw)
	l := p.layers[1]
	if !l.hidden() {
		t.Error("hidden layer was written as visible")
	}
	if l.opacity != 128 {
		t.Errorf("opacity = %d, want 128", l.opacity)
	}
	if l.blend != "mul " {
		t.Errorf("blend = %q, want %q", l.blend, "mul ")
	}
	if l.clipping != 1 {
		t.Errorf("clipping = %d, want 1", l.clipping)
	}
	if l.uniName != "Shade" {
		t.Errorf("name = %q", l.uniName)
	}
}

// TestUnicodeLayerName checks a non-ASCII name survives in the luni block, which
// is the one Photoshop prefers.
func TestUnicodeLayerName(t *testing.T) {
	raw := testkit.DocumentArchiveLayer(testkit.CanvasW, testkit.CanvasH,
		testkit.LayerOpts{Opacity: 1, Name: "Слой 日本語"})
	p := exportDoc(t, raw)
	if got := p.layers[1].uniName; got != "Слой 日本語" {
		t.Errorf("unicode name = %q", got)
	}
}

// TestUnnamedLayerGetsAName: Procreate leaves the name empty until the user
// renames a layer, and an empty PSD layer name shows up as blank in Photoshop.
func TestUnnamedLayerGetsAName(t *testing.T) {
	raw := testkit.DocumentArchiveLayer(testkit.CanvasW, testkit.CanvasH,
		testkit.LayerOpts{Opacity: 1, Name: ""})
	p := exportDoc(t, raw)
	if got := p.layers[1].uniName; got != "Layer" {
		t.Errorf("unnamed layer = %q, want %q", got, "Layer")
	}
}

func exportDoc(t *testing.T, documentArchive []byte) parsed {
	t.Helper()
	a := testkit.Project(1)
	for i := range a.Entries {
		if a.Entries[i].Name == "Document.archive" {
			a.Entries[i].Data = documentArchive
		}
	}
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
	out := filepath.Join(t.TempDir(), "out.psd")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := psd.Write(context.Background(), f, doc); err != nil {
		f.Close()
		t.Fatalf("Write: %v", err)
	}
	f.Close()
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return readPSD(t, b)
}

// TestEmptyLayerHasZeroBounds: a layer with no ink must not claim canvas-sized
// channels, or every export would carry a full transparent plane per blank layer.
func TestEmptyLayerHasZeroBounds(t *testing.T) {
	raw := testkit.DocumentArchiveLayer(testkit.CanvasW, testkit.CanvasH,
		testkit.LayerOpts{Opacity: 1, Name: "Blank", UUID: "deadbeef-0000-0000-0000-000000000000"})
	p := exportDoc(t, raw)
	l := p.layers[1]
	if l.right != 0 || l.bottom != 0 {
		t.Errorf("blank layer bounds = (%d,%d)-(%d,%d), want all zero", l.left, l.top, l.right, l.bottom)
	}
	for _, id := range []int16{-1, 0, 1, 2} {
		if got := l.channelLens[id]; got != 2 {
			t.Errorf("blank layer channel %d = %d bytes, want 2", id, got)
		}
	}
}

// TestCancelled stops the export instead of leaving a half-written PSD.
func TestCancelled(t *testing.T) {
	p := testkit.Project(1).Write(t, t.TempDir(), "in.procreate")
	arch, err := procreate.Open(p, p)
	if err != nil {
		t.Fatal(err)
	}
	defer arch.Close()
	doc, err := silica.ReadDocument(arch)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f, err := os.Create(filepath.Join(t.TempDir(), "out.psd"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := psd.Write(ctx, f, doc); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestOversizedCanvasRefused: a PSD cannot address more than 30000 pixels, and
// silently truncating would be worse than failing.
func TestOversizedCanvasRefused(t *testing.T) {
	raw := testkit.DocumentArchive(40000, 100)
	a := testkit.Project(1)
	for i := range a.Entries {
		if a.Entries[i].Name == "Document.archive" {
			a.Entries[i].Data = raw
		}
	}
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
	f, err := os.Create(filepath.Join(t.TempDir(), "out.psd"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := psd.Write(context.Background(), f, doc); err == nil {
		t.Fatal("want an error for a canvas beyond the PSD limit")
	}
}

// TestMergedImageRoundTrip decodes the merged composite back out of the file and
// checks it matches the pixels the document's composite layer holds. This is
// what proves the RLE encoder and the planar channel layout are right.
func TestMergedImageRoundTrip(t *testing.T) {
	b, doc := writePSD(t)
	got := decodeMerged(t, b, doc.Width, doc.Height)

	want, err := doc.ImageIn(context.Background(), doc.Composite,
		image.Rect(0, 0, doc.Width, doc.Height))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < doc.Height; y += 7 {
		for x := 0; x < doc.Width; x += 7 {
			g := got.NRGBAAt(x, y)
			w := want.NRGBAAt(x, y)
			if g != w {
				t.Fatalf("merged pixel (%d,%d) = %v, want %v", x, y, g, w)
			}
		}
	}
}

// decodeMerged reads the merged image section: a compression word, one row-length
// table covering every channel, then the packed rows.
func decodeMerged(t *testing.T, b []byte, w, h int) *image.NRGBA {
	t.Helper()
	r := &reader{t: t, b: b}
	r.take(26)
	r.take(int(r.u32())) // colour mode
	r.take(int(r.u32())) // resources
	r.take(int(r.u32())) // layer and mask

	if c := r.u16(); c != 1 {
		t.Fatalf("merged compression = %d, want 1 (RLE)", c)
	}
	const channels = 4
	counts := make([]int, channels*h)
	for i := range counts {
		counts[i] = int(r.u16())
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for c := 0; c < channels; c++ {
		for y := 0; y < h; y++ {
			line := unpackBits(t, r.take(counts[c*h+y]), w)
			for x := 0; x < w; x++ {
				o := img.PixOffset(x, y)
				img.Pix[o+c] = line[x]
			}
		}
	}
	return img
}

// unpackBits reverses the PackBits encoding, so the test does not trust the
// encoder to verify itself.
func unpackBits(t *testing.T, src []byte, want int) []byte {
	t.Helper()
	out := make([]byte, 0, want)
	for i := 0; i < len(src); {
		n := int(int8(src[i]))
		i++
		switch {
		case n >= 0:
			cnt := n + 1
			out = append(out, src[i:i+cnt]...)
			i += cnt
		case n > -128:
			cnt := 1 - n
			for k := 0; k < cnt; k++ {
				out = append(out, src[i])
			}
			i++
		}
	}
	if len(out) != want {
		t.Fatalf("unpacked %d bytes, want %d", len(out), want)
	}
	return out
}

// TestLayerChannelRoundTrip decodes a layer's own channels and compares them
// with the layer pixels, which checks the per-channel row tables too.
func TestLayerChannelRoundTrip(t *testing.T) {
	b, doc := writePSD(t)
	want, err := doc.ImageIn(context.Background(), doc.FlatLayers()[0].Raster,
		image.Rect(0, 0, doc.Width, doc.Height))
	if err != nil {
		t.Fatal(err)
	}
	got := decodeFirstRasterLayer(t, b)
	for y := 0; y < doc.Height; y += 5 {
		for x := 0; x < doc.Width; x += 5 {
			g, w := got.NRGBAAt(x, y), want.NRGBAAt(x, y)
			if g != w {
				t.Fatalf("layer pixel (%d,%d) = %v, want %v", x, y, g, w)
			}
		}
	}
}

// decodeFirstRasterLayer walks to the channel data of the first record that has
// pixels and rebuilds its image.
func decodeFirstRasterLayer(t *testing.T, b []byte) *image.NRGBA {
	t.Helper()
	r := &reader{t: t, b: b}
	r.take(26)
	r.take(int(r.u32()))
	r.take(int(r.u32()))
	mask := r.take(int(r.u32()))
	mr := &reader{t: t, b: mask}
	info := mr.take(int(mr.u32()))
	ir := &reader{t: t, b: info}
	count := int(int16(ir.u16()))
	if count < 0 {
		count = -count
	}
	type rec struct {
		bounds image.Rectangle
		chans  []struct {
			id int16
			n  uint32
		}
	}
	var recs []rec
	for i := 0; i < count; i++ {
		top, left := int32(ir.u32()), int32(ir.u32())
		bottom, right := int32(ir.u32()), int32(ir.u32())
		var rc rec
		rc.bounds = image.Rect(int(left), int(top), int(right), int(bottom))
		n := int(ir.u16())
		for c := 0; c < n; c++ {
			id := int16(ir.u16())
			ln := ir.u32()
			rc.chans = append(rc.chans, struct {
				id int16
				n  uint32
			}{id, ln})
		}
		ir.take(4 + 4 + 1 + 1 + 1 + 1) // signature, blend key, opacity, clipping, flags, filler
		ir.take(int(ir.u32()))         // extra data
		recs = append(recs, rc)
	}
	// Channel data follows every record, in record order.
	compIdx := map[int16]int{-1: 3, 0: 0, 1: 1, 2: 2}
	var out *image.NRGBA
	for _, rc := range recs {
		if rc.bounds.Empty() {
			for _, c := range rc.chans {
				ir.take(int(c.n))
			}
			continue
		}
		w, h := rc.bounds.Dx(), rc.bounds.Dy()
		img := image.NewNRGBA(rc.bounds)
		for _, c := range rc.chans {
			start := ir.i
			if cw := ir.u16(); cw != 1 {
				t.Fatalf("layer channel compression = %d, want 1", cw)
			}
			counts := make([]int, h)
			for y := range counts {
				counts[y] = int(ir.u16())
			}
			for y := 0; y < h; y++ {
				line := unpackBits(t, ir.take(counts[y]), w)
				for x := 0; x < w; x++ {
					o := img.PixOffset(rc.bounds.Min.X+x, rc.bounds.Min.Y+y)
					img.Pix[o+compIdx[c.id]] = line[x]
				}
			}
			if uint32(ir.i-start) != c.n {
				t.Fatalf("channel %d consumed %d bytes, record declared %d", c.id, ir.i-start, c.n)
			}
		}
		if out == nil {
			out = img
		}
	}
	if out == nil {
		t.Fatal("no raster layer with pixels was found")
	}
	return out
}

func utf16String(u []uint16) string {
	r := make([]rune, 0, len(u))
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 0xd800 && c < 0xdc00 && i+1 < len(u) {
			r = append(r, ((rune(c)-0xd800)<<10|(rune(u[i+1])-0xdc00))+0x10000)
			i++
			continue
		}
		r = append(r, rune(c))
	}
	return string(r)
}

var _ = fmt.Sprintf
var _ = color.NRGBA{}

// TestThumbnailResource: the export embeds a preview (image resource 1036), so
// Photoshop, Bridge and the Windows thumbnail packs that handle .psd have
// something to show without decoding the whole document.
func TestThumbnailResource(t *testing.T) {
	b, doc := writePSD(t)

	body := resourceSection(t, b)
	payload, ok := findResource(body, 1036)
	if !ok {
		t.Fatal("no thumbnail resource (id 1036) in the file")
	}
	if len(payload) < 28 {
		t.Fatalf("thumbnail payload is %d bytes, too short for the header", len(payload))
	}
	format := binary.BigEndian.Uint32(payload[0:4])
	w := int(binary.BigEndian.Uint32(payload[4:8]))
	h := int(binary.BigEndian.Uint32(payload[8:12]))
	rowBytes := int(binary.BigEndian.Uint32(payload[12:16]))
	sizeAfter := int(binary.BigEndian.Uint32(payload[20:24]))
	bpp := binary.BigEndian.Uint16(payload[24:26])
	planes := binary.BigEndian.Uint16(payload[26:28])

	if format != 1 {
		t.Errorf("format = %d, want 1 (kJpegRGB)", format)
	}
	if bpp != 24 || planes != 1 {
		t.Errorf("bitspixel/planes = %d/%d, want 24/1", bpp, planes)
	}
	if want := (w*24 + 31) / 32 * 4; rowBytes != want {
		t.Errorf("widthbytes = %d, want %d", rowBytes, want)
	}
	if sizeAfter != len(payload)-28 {
		t.Errorf("sizeafter = %d, want %d", sizeAfter, len(payload)-28)
	}
	// The preview must not be larger than the canvas, nor exceed the long-side
	// bound that keeps it small.
	if w > doc.Width || h > doc.Height {
		t.Errorf("preview %dx%d is larger than the canvas %dx%d", w, h, doc.Width, doc.Height)
	}
	if w > 256 || h > 256 {
		t.Errorf("preview %dx%d exceeds the 256-pixel bound", w, h)
	}

	img, err := jpeg.Decode(bytes.NewReader(payload[28:]))
	if err != nil {
		t.Fatalf("preview is not decodable JPEG: %v", err)
	}
	if got := img.Bounds(); got.Dx() != w || got.Dy() != h {
		t.Errorf("decoded %dx%d, header says %dx%d", got.Dx(), got.Dy(), w, h)
	}
}

// resourceSection returns the image-resource block body: a 26-byte header, the
// colour-mode section, then this section's own length.
func resourceSection(t *testing.T, b []byte) []byte {
	t.Helper()
	if len(b) < 34 {
		t.Fatalf("file is %d bytes, too short", len(b))
	}
	colorLen := int(binary.BigEndian.Uint32(b[26:30]))
	at := 30 + colorLen
	if len(b) < at+4 {
		t.Fatalf("file truncated before the resource section")
	}
	n := int(binary.BigEndian.Uint32(b[at : at+4]))
	at += 4
	if len(b) < at+n {
		t.Fatalf("resource section claims %d bytes, file has %d", n, len(b)-at)
	}
	return b[at : at+n]
}

// findResource walks the resource blocks looking for one id.
func findResource(body []byte, id uint16) ([]byte, bool) {
	for len(body) >= 12 {
		if string(body[0:4]) != "8BIM" {
			return nil, false
		}
		got := binary.BigEndian.Uint16(body[4:6])
		// The name is a Pascal string padded to an even length; the writer emits
		// an empty one, which is two zero bytes.
		at := 8
		size := int(binary.BigEndian.Uint32(body[at : at+4]))
		at += 4
		if len(body) < at+size {
			return nil, false
		}
		data := body[at : at+size]
		if got == id {
			return data, true
		}
		at += size
		if size%2 == 1 {
			at++
		}
		body = body[at:]
	}
	return nil, false
}
