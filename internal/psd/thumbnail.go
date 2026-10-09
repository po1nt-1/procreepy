package psd

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
)

const (
	// resourceThumbnail is the image resource readers use for a preview:
	// Photoshop, Bridge, asset managers and the Windows thumbnail packs that
	// handle .psd all look here.
	//
	// It does not, on its own, make Explorer draw a preview. Windows renders
	// thumbnails only through a registered IThumbnailProvider for the extension,
	// and ships none for .psd — one arrives with Photoshop or a third-party
	// pack. Embedding the resource is what a file writer can do; installing a
	// shell extension is a different kind of program.
	resourceThumbnail = 1036

	// thumbnailFormatJPEG is kJpegRGB: the payload is a JFIF stream in RGB
	// order. (Resource 1033 is the same structure with the channels swapped,
	// which is why only 1036 is written.)
	thumbnailFormatJPEG = 1

	// thumbnailMaxSide bounds the long edge. 256 is what viewers ask for at
	// their largest, and at this size the JPEG is a few kilobytes beside a
	// full-resolution merged image that the document already carries.
	thumbnailMaxSide = 256

	// thumbnailQuality trades a little fidelity for a preview that stays small.
	thumbnailQuality = 80

	thumbnailBitsPerPixel = 24
	thumbnailPlanes       = 1
)

// thumbnailResource builds the body of resource 1036 from the document's merged
// image, or returns nil when no preview can be made. A document with nothing to
// show is not an error: the export is still valid without a preview.
func thumbnailResource(src *image.NRGBA) []byte {
	if src == nil {
		return nil
	}
	w, h := thumbnailSize(src.Bounds().Dx(), src.Bounds().Dy())
	if w <= 0 || h <= 0 {
		return nil
	}
	small := boxDownscale(src, w, h)

	// The preview is opaque: a JPEG has no alpha, so compose onto white rather
	// than letting transparent areas encode as black.
	flat := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(small.Pix); i += 4 {
		a := uint32(small.Pix[i+3])
		for c := 0; c < 3; c++ {
			v := uint32(small.Pix[i+c])*a + 255*(255-a)
			flat.Pix[i+c] = uint8((v + 127) / 255)
		}
		flat.Pix[i+3] = 0xff
	}

	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, flat, &jpeg.Options{Quality: thumbnailQuality}); err != nil {
		return nil
	}

	rowBytes := (w*thumbnailBitsPerPixel + 31) / 32 * 4
	out := make([]byte, 0, 28+jb.Len())
	out = binary.BigEndian.AppendUint32(out, thumbnailFormatJPEG)
	out = binary.BigEndian.AppendUint32(out, uint32(w))
	out = binary.BigEndian.AppendUint32(out, uint32(h))
	out = binary.BigEndian.AppendUint32(out, uint32(rowBytes))
	out = binary.BigEndian.AppendUint32(out, uint32(rowBytes*h*thumbnailPlanes))
	out = binary.BigEndian.AppendUint32(out, uint32(jb.Len()))
	out = binary.BigEndian.AppendUint16(out, thumbnailBitsPerPixel)
	out = binary.BigEndian.AppendUint16(out, thumbnailPlanes)
	return append(out, jb.Bytes()...)
}

// thumbnailSize scales w x h down so the long side is at most
// thumbnailMaxSide, keeping the aspect ratio and never scaling up.
func thumbnailSize(w, h int) (int, int) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if w <= thumbnailMaxSide && h <= thumbnailMaxSide {
		return w, h
	}
	if w >= h {
		return thumbnailMaxSide, max(1, h*thumbnailMaxSide/w)
	}
	return max(1, w*thumbnailMaxSide/h), thumbnailMaxSide
}

// boxDownscale area-averages src down to w x h.
//
// Point sampling a detailed painting down to 256 pixels aliases badly, and the
// standard library has no resampler. Averaging the source pixels that fall in
// each destination cell is a few lines and good enough for a preview, which is
// preferable to taking on a third-party image dependency: this module has none.
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
			var r, g, b, a, n uint32
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					i := src.PixOffset(sx, sy)
					r += uint32(src.Pix[i])
					g += uint32(src.Pix[i+1])
					b += uint32(src.Pix[i+2])
					a += uint32(src.Pix[i+3])
					n++
				}
			}
			if n == 0 {
				continue
			}
			o := out.PixOffset(x, y)
			out.Pix[o] = uint8(r / n)
			out.Pix[o+1] = uint8(g / n)
			out.Pix[o+2] = uint8(b / n)
			out.Pix[o+3] = uint8(a / n)
		}
	}
	return out
}
