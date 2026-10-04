package silica

import (
	"context"
	"fmt"
	"image"
	"image/color"
)

// RasterBounds returns the tight display-space rectangle covering a layer's
// non-transparent pixels, or the empty rectangle when the layer has no ink.
//
// This scans tile alpha without assembling the canvas, so a PSD export can size
// its layer records before it has to hold a whole layer's pixels in memory.
// Procreate stores the tile grid sparsely, so most layers touch a small part of
// the canvas and the bounds shrink the export dramatically.
func (d *Document) RasterBounds(ctx context.Context, r *Raster) (image.Rectangle, error) {
	if r == nil {
		return image.Rectangle{}, fmt.Errorf("layer has no tile set")
	}
	cols, rows := d.tileGrid()
	maxTile := d.TileSize * d.TileSize * 4

	empty := true
	minX, minY := d.canvasW, d.canvasH
	maxX, maxY := -1, -1

	for row := 0; row < rows; row++ {
		if err := ctx.Err(); err != nil {
			return image.Rectangle{}, err
		}
		for col := 0; col < cols; col++ {
			px, tw, th, ok, err := d.decodeTile(r, col, row, maxTile)
			if err != nil {
				return image.Rectangle{}, err
			}
			if !ok {
				continue
			}
			for y := 0; y < th; y++ {
				for x := 0; x < tw; x++ {
					if px[tilePixel(x, y, th)+3] == 0 {
						continue
					}
					empty = false
					gx, gy := col*d.TileSize+x, row*d.TileSize+y
					minX, maxX = min(minX, gx), max(maxX, gx)
					minY, maxY = min(minY, gy), max(maxY, gy)
				}
			}
		}
	}
	if empty {
		return image.Rectangle{}, nil
	}
	return d.rot.rectToDisplay(image.Rect(minX, minY, maxX+1, maxY+1), d.canvasW, d.canvasH), nil
}

// ImageIn decodes a layer into an image covering exactly rect, in display-space
// coordinates. Pixels outside rect are discarded, so a cropped read costs only
// the memory the crop needs.
func (d *Document) ImageIn(ctx context.Context, r *Raster, rect image.Rectangle) (*image.NRGBA, error) {
	if r == nil {
		return nil, fmt.Errorf("layer has no tile set")
	}
	img := image.NewNRGBA(rect)
	if rect.Empty() {
		return img, nil
	}
	// Work out which tiles the requested region needs by mapping it back into the
	// canvas space the tile grid is laid out in.
	canvasRect := d.rot.rectToCanvas(rect, d.canvasW, d.canvasH)
	maxTile := d.TileSize * d.TileSize * 4
	firstCol := max(0, canvasRect.Min.X/d.TileSize)
	lastCol := min((d.canvasW-1)/d.TileSize, (canvasRect.Max.X-1)/d.TileSize)
	firstRow := max(0, canvasRect.Min.Y/d.TileSize)
	lastRow := min((d.canvasH-1)/d.TileSize, (canvasRect.Max.Y-1)/d.TileSize)

	for row := firstRow; row <= lastRow; row++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for col := firstCol; col <= lastCol; col++ {
			px, tw, th, ok, err := d.decodeTile(r, col, row, maxTile)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			ox, oy := col*d.TileSize, row*d.TileSize
			for y := 0; y < th; y++ {
				for x := 0; x < tw; x++ {
					gx, gy := d.rot.point(ox+x, oy+y, d.canvasW, d.canvasH)
					if gx < rect.Min.X || gx >= rect.Max.X || gy < rect.Min.Y || gy >= rect.Max.Y {
						continue
					}
					i := tilePixel(x, y, th)
					a := px[i+3]
					c := color.NRGBA{R: px[i], G: px[i+1], B: px[i+2], A: a}
					if a != 0 && a != 0xff {
						c.R = unpremul(px[i], a)
						c.G = unpremul(px[i+1], a)
						c.B = unpremul(px[i+2], a)
					}
					img.SetNRGBA(gx, gy, c)
				}
			}
		}
	}
	return img, nil
}

// decodeTile reads and decompresses one tile, reporting its clipped extent.
func (d *Document) decodeTile(r *Raster, col, row, maxTile int) (px []byte, tw, th int, ok bool, err error) {
	raw, present, err := d.readTile(r.UUID, col, row)
	if err != nil || !present {
		return nil, 0, 0, false, err
	}
	px, err = decodeTilePixels(raw, maxTile)
	if err != nil {
		return nil, 0, 0, false, fmt.Errorf("layer %s tile %d~%d: %w", r.UUID, row, col, err)
	}
	tw, th = d.tileExtent(col, row)
	if len(px) != tw*th*4 {
		return nil, 0, 0, false, fmt.Errorf("layer %s tile %d~%d: decoded %d bytes, want %d for %dx%d",
			r.UUID, row, col, len(px), tw*th*4, tw, th)
	}
	return px, tw, th, true, nil
}
