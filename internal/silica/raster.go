package silica

import (
	"context"
	"fmt"
	"image"

	"procreepy/internal/procodec"
)

// tileExts are the container suffixes Procreate has used for layer tiles.
// Which one a project uses depends on its age, and a single archive can mix
// them, so all of them are probed per tile.
var tileExts = []string{".chunk", ".lz4", ".lz4c"}

// Image decodes one layer's tiles into a full-canvas image, in display space.
//
// Tiles are addressed as <UUID>/<row>~<column> and hold premultiplied RGBA bytes
// clipped to the canvas, so the last column and last row are narrower or shorter
// than the tile size. Tiles that are absent are fully transparent: Procreate
// stores the grid sparsely, and a missing tile is the normal representation of
// untouched canvas rather than an error.
//
// The returned image is NRGBA (straight alpha). Un-premultiplying is lossy for
// partially transparent pixels: the original straight-alpha colour is not
// recoverable from 8-bit premultiplied data. See the README for the fidelity
// contract.
func (d *Document) Image(ctx context.Context, r *Raster) (*image.NRGBA, error) {
	return d.ImageIn(ctx, r, image.Rect(0, 0, d.Width, d.Height))
}

// tileGrid is the canvas-space tile grid.
func (d *Document) tileGrid() (cols, rows int) {
	return (d.canvasW + d.TileSize - 1) / d.TileSize, (d.canvasH + d.TileSize - 1) / d.TileSize
}

// tileExtent is the clipped size of the tile at a grid position.
func (d *Document) tileExtent(col, row int) (w, h int) {
	return min(d.TileSize, d.canvasW-col*d.TileSize), min(d.TileSize, d.canvasH-row*d.TileSize)
}

// decodeTilePixels decompresses one tile payload into raw premultiplied RGBA.
func decodeTilePixels(raw []byte, maxTile int) ([]byte, error) {
	return procodec.DecodeTile(raw, maxTile)
}

// readTile fetches the tile at a grid position. Tiles are named
// <UUID>/<row>~<column>, not column~row: both orders survive a size check
// because the two extents are simply swapped, so the ordering is pinned by the
// corpus suite's comparison against Procreate's own thumbnail instead.
func (d *Document) readTile(uuid string, col, row int) ([]byte, bool, error) {
	base := fmt.Sprintf("%s/%d~%d", uuid, row, col)
	for _, ext := range tileExts {
		name := base + ext
		if !d.src.HasMember(name) {
			continue
		}
		raw, err := d.src.ReadMember(name)
		if err != nil {
			return nil, false, fmt.Errorf("cannot read %s: %w", name, err)
		}
		return raw, true, nil
	}
	return nil, false, nil
}

// tilePixel returns the offset of a tile pixel in the decoded tile buffer.
//
// Tile bytes run down each column before moving to the next, so a tile is stored
// transposed relative to the way it is drawn. Together with the row~column
// member naming this makes the whole stored canvas transposed, which is why
// reading it the obvious way yields a mirrored artwork rather than an obviously
// scrambled one - a mistake no size check can catch, and which the corpus suite
// pins by comparing against Procreate's thumbnail.
func tilePixel(x, y, th int) int { return (x*th + y) * 4 }

// unpremul recovers a straight-alpha component, rounding to nearest and
// clamping: premultiplied values can exceed alpha slightly through Procreate's
// own rounding.
func unpremul(v, a uint8) uint8 {
	n := (int(v)*255 + int(a)/2) / int(a)
	if n > 255 {
		return 255
	}
	return uint8(n)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
