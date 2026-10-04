// Package psd writes Adobe Photoshop documents from a parsed Procreate
// document, using only the standard library.
//
// The layout follows the published Photoshop file format: a 26-byte header, an
// empty colour-mode section, image resources, the layer and mask section, and
// finally the merged composite. Fidelity is documented in the README; in short,
// the layer tree, names, visibility, opacity, blend modes, bounds and RGBA
// pixels survive, while masks, clipping semantics and text layers do not, and
// straight-alpha colour cannot be recovered exactly from Procreate's
// premultiplied 8-bit tiles.
package psd

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"

	"procreepy/internal/silica"
)

// MaxDimension is the largest canvas edge a PSD (as opposed to a PSB) can
// address. Procreate canvases stay well inside it, but a document that does not
// is refused rather than truncated.
const MaxDimension = 30000

// Photoshop constants used below.
const (
	sigFile      = "8BPS"
	sigResource  = "8BIM"
	versionPSD   = 1
	depth8       = 8
	colorModeRGB = 3

	compressionRLE = 1

	chanAlpha = -1
	chanRed   = 0
	chanGreen = 1
	chanBlue  = 2

	// Section divider kinds for the lsct block that encodes layer groups.
	sectionOpenFolder = 1
	sectionDivider    = 3

	resourceResolution = 1005
	resourceICCProfile = 1039
)

// Write emits doc as a PSD to w and returns the number of layer records
// written (group markers included).
//
// w must be seekable: a layer record has to state the byte length of each of its
// channels before the channel data exists, so the lengths are written as
// placeholders and patched once the pixels have been compressed. That keeps peak
// memory at one layer instead of the whole document.
func Write(ctx context.Context, w io.WriteSeeker, doc *silica.Document) (int, error) {
	if doc.Width > MaxDimension || doc.Height > MaxDimension {
		return 0, fmt.Errorf("canvas is %dx%d, larger than the %d-pixel PSD limit",
			doc.Width, doc.Height, MaxDimension)
	}
	e := &encoder{w: w, doc: doc}
	if err := e.writeHeader(); err != nil {
		return 0, err
	}
	if err := e.writeColorMode(); err != nil {
		return 0, err
	}
	if err := e.writeResources(); err != nil {
		return 0, err
	}
	if err := e.writeLayers(ctx); err != nil {
		return e.count, err
	}
	if err := e.writeMerged(ctx); err != nil {
		return e.count, err
	}
	return e.count, nil
}

type encoder struct {
	w     io.WriteSeeker
	doc   *silica.Document
	off   int64 // bytes written so far, i.e. the current file offset
	count int   // layer records emitted
}

func (e *encoder) write(b []byte) error {
	n, err := e.w.Write(b)
	e.off += int64(n)
	return err
}

func (e *encoder) u8(v uint8) error { return e.write([]byte{v}) }
func (e *encoder) u16(v uint16) error {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	return e.write(b[:])
}
func (e *encoder) u32(v uint32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return e.write(b[:])
}
func (e *encoder) i16(v int16) error { return e.u16(uint16(v)) }
func (e *encoder) i32(v int32) error { return e.u32(uint32(v)) }

func (e *encoder) zeros(n int) error { return e.write(make([]byte, n)) }

// patchU32 rewrites a 4-byte big-endian value already on disk and returns to the
// end of the file.
func (e *encoder) patchU32(at int64, v uint32) error {
	if _, err := e.w.Seek(at, io.SeekStart); err != nil {
		return err
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	if _, err := e.w.Write(b[:]); err != nil {
		return err
	}
	_, err := e.w.Seek(e.off, io.SeekStart)
	return err
}

func (e *encoder) writeHeader() error {
	if err := e.write([]byte(sigFile)); err != nil {
		return err
	}
	if err := e.u16(versionPSD); err != nil {
		return err
	}
	if err := e.zeros(6); err != nil { // reserved
		return err
	}
	// Four channels: RGB plus the transparency of the merged result.
	if err := e.u16(4); err != nil {
		return err
	}
	if err := e.u32(uint32(e.doc.Height)); err != nil {
		return err
	}
	if err := e.u32(uint32(e.doc.Width)); err != nil {
		return err
	}
	if err := e.u16(depth8); err != nil {
		return err
	}
	return e.u16(colorModeRGB)
}

// writeColorMode writes an empty colour-mode section, which is what RGB mode
// requires (only indexed and duotone carry data here).
func (e *encoder) writeColorMode() error { return e.u32(0) }

func (e *encoder) writeResources() error {
	var body []byte
	body = appendResource(body, resourceResolution, resolutionInfo(e.doc.DPI))
	if len(e.doc.ICCProfile) > 0 {
		body = appendResource(body, resourceICCProfile, e.doc.ICCProfile)
	}
	if err := e.u32(uint32(len(body))); err != nil {
		return err
	}
	return e.write(body)
}

// appendResource appends one image resource block: signature, id, empty Pascal
// name, length, then the payload padded to an even length.
func appendResource(dst []byte, id uint16, data []byte) []byte {
	dst = append(dst, sigResource...)
	dst = binary.BigEndian.AppendUint16(dst, id)
	dst = append(dst, 0, 0) // empty name, padded to an even size
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(data)))
	dst = append(dst, data...)
	if len(data)%2 == 1 {
		dst = append(dst, 0)
	}
	return dst
}

// resolutionInfo builds the 16-byte ResolutionInfo resource. Procreate records
// a whole-number DPI; the resource stores it as a 16.16 fixed-point value.
func resolutionInfo(dpi int) []byte {
	if dpi <= 0 {
		dpi = 72
	}
	fixed := uint32(dpi) << 16
	b := make([]byte, 0, 16)
	b = binary.BigEndian.AppendUint32(b, fixed)
	b = binary.BigEndian.AppendUint16(b, 1) // display unit: pixels per inch
	b = binary.BigEndian.AppendUint16(b, 1) // width unit: inches
	b = binary.BigEndian.AppendUint32(b, fixed)
	b = binary.BigEndian.AppendUint16(b, 1)
	b = binary.BigEndian.AppendUint16(b, 1)
	return b
}

// record is one PSD layer record being assembled: the metadata is known up
// front, the channel lengths only after the pixels have been compressed.
type record struct {
	node   *silica.Node
	bounds image.Rectangle
	// lenAt holds the file offsets of the four channel-length fields, in the
	// order the channels are declared.
	lenAt [4]int64
	// section is the lsct kind (0 when the record is a plain raster layer).
	section int
}

// channelIDs is the declaration order of a layer's channels; the channel data
// blocks follow in the same order.
var channelIDs = [4]int16{chanAlpha, chanRed, chanGreen, chanBlue}

func (e *encoder) writeLayers(ctx context.Context) error {
	recs, err := e.planRecords(ctx)
	if err != nil {
		return err
	}

	maskSectionLenAt := e.off
	if err := e.u32(0); err != nil { // layer and mask section length, patched below
		return err
	}
	sectionStart := e.off

	layerInfoLenAt := e.off
	if err := e.u32(0); err != nil { // layer info length, patched below
		return err
	}
	layerInfoStart := e.off

	// A negative count tells Photoshop the merged image's first alpha channel is
	// the document's transparency rather than a spare channel.
	if err := e.i16(int16(-len(recs))); err != nil {
		return err
	}
	for i := range recs {
		if err := e.writeRecord(&recs[i]); err != nil {
			return err
		}
	}
	e.count = len(recs)

	for i := range recs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.writeChannels(ctx, &recs[i]); err != nil {
			return err
		}
	}

	// Photoshop reads the layer info length as a padded value; keep it even.
	if (e.off-layerInfoStart)%2 == 1 {
		if err := e.zeros(1); err != nil {
			return err
		}
	}
	if err := e.patchU32(layerInfoLenAt, uint32(e.off-layerInfoStart)); err != nil {
		return err
	}
	if err := e.u32(0); err != nil { // global layer mask info: none
		return err
	}
	return e.patchU32(maskSectionLenAt, uint32(e.off-sectionStart))
}

// planRecords flattens the layer tree into PSD layer records.
//
// Photoshop encodes a group as a run of records: a bounding-section divider
// below the group's contents and a folder record, carrying the group's name,
// above them. Records are stored bottom-most first, which is the order the
// document model already uses.
func (e *encoder) planRecords(ctx context.Context) ([]record, error) {
	var recs []record
	var walk func(nodes []*silica.Node) error
	walk = func(nodes []*silica.Node) error {
		for _, n := range nodes {
			if err := ctx.Err(); err != nil {
				return err
			}
			if n.IsGroup {
				recs = append(recs, record{node: n, section: sectionDivider})
				if err := walk(n.Children); err != nil {
					return err
				}
				recs = append(recs, record{node: n, section: sectionOpenFolder})
				continue
			}
			b, err := e.doc.RasterBounds(ctx, n.Raster)
			if err != nil {
				return err
			}
			recs = append(recs, record{node: n, bounds: b})
		}
		return nil
	}
	if err := walk(e.doc.Layers); err != nil {
		return nil, err
	}
	return recs, nil
}

func (e *encoder) writeRecord(r *record) error {
	b := r.bounds
	if err := e.i32(int32(b.Min.Y)); err != nil {
		return err
	}
	if err := e.i32(int32(b.Min.X)); err != nil {
		return err
	}
	if err := e.i32(int32(b.Max.Y)); err != nil {
		return err
	}
	if err := e.i32(int32(b.Max.X)); err != nil {
		return err
	}
	if err := e.u16(uint16(len(channelIDs))); err != nil {
		return err
	}
	for i, id := range channelIDs {
		if err := e.i16(id); err != nil {
			return err
		}
		r.lenAt[i] = e.off
		if err := e.u32(0); err != nil { // patched once the channel is compressed
			return err
		}
	}
	if err := e.write([]byte(sigResource)); err != nil {
		return err
	}
	key := "norm"
	opacity := uint8(255)
	clipping := uint8(0)
	flags := uint8(0)
	if n := r.node; n != nil {
		if r.section == 0 {
			key, _ = silica.BlendKey(n.Blend)
		}
		opacity = uint8(n.Opacity*255 + 0.5)
		if n.Clipped {
			clipping = 1
		}
		// Bit 1 of the flags means hidden, not visible.
		if n.Hidden {
			flags |= 1 << 1
		}
		if n.Locked {
			flags |= 1 << 0
		}
	}
	if err := e.write([]byte(key)); err != nil {
		return err
	}
	if err := e.u8(opacity); err != nil {
		return err
	}
	if err := e.u8(clipping); err != nil {
		return err
	}
	if err := e.u8(flags); err != nil {
		return err
	}
	if err := e.u8(0); err != nil { // filler
		return err
	}
	extra := buildExtra(r)
	if err := e.u32(uint32(len(extra))); err != nil {
		return err
	}
	return e.write(extra)
}

// buildExtra assembles a layer record's variable-length tail: the (absent) mask
// and blending-range sections, the legacy Pascal name, and the additional blocks
// that carry the Unicode name and the group marker.
func buildExtra(r *record) []byte {
	name := layerName(r)
	var b []byte
	b = binary.BigEndian.AppendUint32(b, 0) // layer mask data: none
	b = binary.BigEndian.AppendUint32(b, 0) // layer blending ranges: none
	b = appendPascalPadded(b, name, 4)
	// luni carries the real name: the legacy Pascal string is limited to 255
	// bytes of one encoding, and Photoshop prefers this block when present.
	b = appendBlock(b, "luni", appendUnicodeString(nil, name))
	if r.section != 0 {
		b = appendBlock(b, "lsct", binary.BigEndian.AppendUint32(nil, uint32(r.section)))
	}
	if len(b)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

// layerName picks what to call a record. Procreate leaves a layer's name empty
// when the user never renamed it, and a group's bounding divider is not shown to
// the user at all but still needs a name in the file.
func layerName(r *record) string {
	if r.section == sectionDivider {
		return "</Layer group>"
	}
	if r.node != nil && r.node.Name != "" {
		return r.node.Name
	}
	return "Layer"
}

func appendBlock(dst []byte, key string, data []byte) []byte {
	dst = append(dst, sigResource...)
	dst = append(dst, key...)
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(data)))
	dst = append(dst, data...)
	if len(data)%2 == 1 {
		dst = append(dst, 0)
	}
	return dst
}

// appendPascalPadded writes a length-prefixed string padded so the whole field
// is a multiple of pad bytes.
func appendPascalPadded(dst []byte, s string, pad int) []byte {
	b := []byte(s)
	if len(b) > 255 {
		b = b[:255]
	}
	start := len(dst)
	dst = append(dst, byte(len(b)))
	dst = append(dst, b...)
	for (len(dst)-start)%pad != 0 {
		dst = append(dst, 0)
	}
	return dst
}

// appendUnicodeString writes a 4-byte character count followed by UTF-16BE
// code units, the representation the luni block uses.
func appendUnicodeString(dst []byte, s string) []byte {
	u := utf16Encode(s)
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(u)))
	for _, c := range u {
		dst = binary.BigEndian.AppendUint16(dst, c)
	}
	return dst
}

// writeChannels compresses and writes one record's four channels, then patches
// the lengths its record reserved.
func (e *encoder) writeChannels(ctx context.Context, r *record) error {
	b := r.bounds
	if r.section != 0 || b.Empty() {
		// A group marker and an untouched layer both carry four empty channels:
		// the compression word alone, which Photoshop reads as zero rows.
		for i := range channelIDs {
			start := e.off
			if err := e.u16(compressionRLE); err != nil {
				return err
			}
			if err := e.patchU32(r.lenAt[i], uint32(e.off-start)); err != nil {
				return err
			}
		}
		return nil
	}
	img, err := e.doc.ImageIn(ctx, r.node.Raster, b)
	if err != nil {
		return err
	}
	w, h := b.Dx(), b.Dy()
	// Channel order must match the declaration order in the record.
	offsets := map[int16]int{chanAlpha: 3, chanRed: 0, chanGreen: 1, chanBlue: 2}
	line := make([]byte, w)
	packed := make([]byte, 0, w+w/64+16)
	counts := make([]uint16, h)
	rows := make([][]byte, h)
	for i, id := range channelIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		comp := offsets[id]
		total := 0
		for y := 0; y < h; y++ {
			src := img.Pix[y*img.Stride:]
			for x := 0; x < w; x++ {
				line[x] = src[x*4+comp]
			}
			packed = packBits(packed[:0], line)
			row := make([]byte, len(packed))
			copy(row, packed)
			rows[y] = row
			counts[y] = uint16(len(row))
			total += len(row)
		}
		start := e.off
		if err := e.u16(compressionRLE); err != nil {
			return err
		}
		for _, c := range counts {
			if err := e.u16(c); err != nil {
				return err
			}
		}
		for _, row := range rows {
			if err := e.write(row); err != nil {
				return err
			}
		}
		if err := e.patchU32(r.lenAt[i], uint32(e.off-start)); err != nil {
			return err
		}
		_ = total
	}
	return nil
}

// writeMerged writes the flattened composite that non-layered readers show.
//
// Procreate stores its own flattened render, so that is used verbatim rather
// than re-compositing the stack, which would mean reimplementing every blend
// mode and reproducing Procreate's results exactly. When it is missing the
// layers are composited in Normal mode as an approximation.
func (e *encoder) writeMerged(ctx context.Context) error {
	img, err := e.mergedImage(ctx)
	if err != nil {
		return err
	}
	w, h := e.doc.Width, e.doc.Height
	if err := e.u16(compressionRLE); err != nil {
		return err
	}
	// The merged image keeps one row-length table for every channel up front,
	// followed by all the packed rows; this differs from a layer channel, which
	// carries its own table.
	order := []int{0, 1, 2, 3} // R, G, B, A
	rows := make([][]byte, 0, h*len(order))
	line := make([]byte, w)
	packed := make([]byte, 0, w+w/64+16)
	for _, comp := range order {
		if err := ctx.Err(); err != nil {
			return err
		}
		for y := 0; y < h; y++ {
			src := img.Pix[y*img.Stride:]
			for x := 0; x < w; x++ {
				line[x] = src[x*4+comp]
			}
			packed = packBits(packed[:0], line)
			row := make([]byte, len(packed))
			copy(row, packed)
			rows = append(rows, row)
		}
	}
	for _, r := range rows {
		if err := e.u16(uint16(len(r))); err != nil {
			return err
		}
	}
	for _, r := range rows {
		if err := e.write(r); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) mergedImage(ctx context.Context) (*image.NRGBA, error) {
	canvas := image.Rect(0, 0, e.doc.Width, e.doc.Height)
	if e.doc.Composite != nil {
		img, err := e.doc.ImageIn(ctx, e.doc.Composite, canvas)
		if err == nil {
			return img, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// A damaged composite should not cost the caller the whole export; the
		// layer stack is still intact, so fall back to compositing it.
	}
	return e.compositeLayers(ctx, canvas)
}

// compositeLayers alpha-composites the visible raster layers in Normal mode.
func (e *encoder) compositeLayers(ctx context.Context, canvas image.Rectangle) (*image.NRGBA, error) {
	out := image.NewNRGBA(canvas)
	if !e.doc.BackgroundHidden {
		bg := e.doc.Background
		for i := 0; i < len(out.Pix); i += 4 {
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = bg.R, bg.G, bg.B, bg.A
		}
	}
	for _, n := range e.doc.FlatLayers() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if n.Hidden || n.Raster == nil {
			continue
		}
		b, err := e.doc.RasterBounds(ctx, n.Raster)
		if err != nil || b.Empty() {
			continue
		}
		img, err := e.doc.ImageIn(ctx, n.Raster, b)
		if err != nil {
			continue
		}
		alpha := n.Opacity
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				i := img.PixOffset(x, y)
				sa := float64(img.Pix[i+3]) / 255 * alpha
				if sa <= 0 {
					continue
				}
				o := out.PixOffset(x, y)
				da := float64(out.Pix[o+3]) / 255
				ra := sa + da*(1-sa)
				if ra <= 0 {
					continue
				}
				for c := 0; c < 3; c++ {
					s := float64(img.Pix[i+c])
					d := float64(out.Pix[o+c])
					out.Pix[o+c] = uint8((s*sa+d*da*(1-sa))/ra + 0.5)
				}
				out.Pix[o+3] = uint8(ra*255 + 0.5)
			}
		}
	}
	return out, nil
}
