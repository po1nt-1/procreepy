// Package silica reads the Procreate document description (Document.archive)
// and the layer tiles it refers to. Document.archive is an NSKeyedArchiver graph
// inside an Apple binary property list; both are decoded here because the module
// takes no third-party dependencies.
package silica

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"sort"
)

// DocumentMember is the archive member holding the document description.
const DocumentMember = "Document.archive"

// Source supplies archive members. procreate.Archive satisfies it.
type Source interface {
	ReadMember(name string) ([]byte, error)
	HasMember(name string) bool
}

// Document is the part of a Procreate document that an image export needs.
//
// Width and Height, and every image this package returns, are in display space:
// the canvas as Procreate presents it. The stored tiles live in a canvas space
// that the document's orientation turns, and almost every real project is turned
// by something, so exporting the raw canvas would hand back a rotated artwork.
type Document struct {
	Width, Height int
	TileSize      int
	DPI           int

	// Orientation is Procreate's on-screen presentation of the canvas. Pixel data
	// and the declared size are always in canvas space; this only says how
	// Procreate turns the canvas for display, and it is what makes the stored
	// QuickLook thumbnail transposed relative to the canvas for some documents.
	Orientation         int
	FlippedHorizontally bool
	FlippedVertically   bool

	Background       color.NRGBA
	BackgroundHidden bool

	ICCProfile []byte
	ICCName    string

	// Layers is the layer tree, bottom-most entry first, which is the order a
	// compositor and the PSD layer records both want.
	Layers []*Node

	// Composite is Procreate's own flattened render of the artwork. It is what
	// the merged image of an export should be, rather than a re-composite that
	// would have to reimplement every blend mode.
	Composite *Raster

	// UnmappedBlends collects blend indices with no known Photoshop equivalent,
	// so a caller can tell the user instead of silently writing Normal.
	UnmappedBlends []int

	// canvasW and canvasH are the tile grid's own dimensions, before the
	// orientation is applied.
	canvasW, canvasH int
	rot              transform

	src Source
}

// Node is one entry in the layer tree: either a raster layer or a group.
type Node struct {
	Name    string
	Hidden  bool
	Locked  bool
	Clipped bool
	Opacity float64
	Blend   int

	// Children is non-nil only for groups.
	Children []*Node
	IsGroup  bool

	// Raster is set only for raster layers.
	Raster *Raster
}

// Raster names the tile set that holds one layer's pixels.
type Raster struct {
	UUID          string
	Width, Height int // as recorded on the layer
}

// silicaLayerType values seen in the wild: 0 is a user layer, 1 is the
// document's flattened composite, 2 is the document-level mask layer. Only user
// layers belong in an export's layer stack.
const (
	layerTypeUser      = 0
	layerTypeComposite = 1
)

// ReadDocument parses Document.archive out of src.
func ReadDocument(src Source) (*Document, error) {
	if !src.HasMember(DocumentMember) {
		return nil, fmt.Errorf("%s is missing; this is not a Procreate document", DocumentMember)
	}
	raw, err := src.ReadMember(DocumentMember)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", DocumentMember, err)
	}
	k, err := parseKeyed(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", DocumentMember, err)
	}
	root := k.object(k.top["root"])
	if root == nil {
		return nil, errors.New(DocumentMember + " has no root object")
	}
	if cn := k.className(root); cn != "SilicaDocument" {
		return nil, fmt.Errorf("%s root is a %s, not a SilicaDocument", DocumentMember, cn)
	}

	// The two components of the archived size map to the canvas transposed: the
	// first is the extent down the tile grid's rows and the second across its
	// columns. Taking them the other way round produces a canvas that decodes
	// into a mirrored artwork, which the corpus suite catches by comparing the
	// result against the thumbnail Procreate stored.
	h, w, err := parseCGSize(k.str(root["size"]))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", DocumentMember, err)
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%s: implausible canvas size %dx%d", DocumentMember, w, h)
	}
	tile := k.int(root["tileSize"])
	if tile <= 0 {
		return nil, fmt.Errorf("%s: implausible tile size %d", DocumentMember, tile)
	}

	orientation := k.int(root["orientation"])
	// The mirror flags name their axis in Procreate's own coordinates, and the
	// stored canvas is transposed relative to those, so the two swap on the way
	// in. Only one project in the reference corpus sets either flag, so this is
	// inferred from a single sample; it is still better than ignoring the flags,
	// which demonstrably mirrors that artwork.
	rot := newTransform(orientation, k.bool(root["flippedVertically"]), k.bool(root["flippedHorizontally"]))
	dw, dh := rot.dims(w, h)
	d := &Document{
		Width:               dw,
		Height:              dh,
		TileSize:            tile,
		DPI:                 k.int(root["SilicaDocumentArchiveDPIKey"]),
		Orientation:         orientation,
		FlippedHorizontally: k.bool(root["flippedHorizontally"]),
		FlippedVertically:   k.bool(root["flippedVertically"]),
		Background:          decodeFloatColor(k.data(root["backgroundColor"])),
		BackgroundHidden:    k.bool(root["backgroundHidden"]),
		canvasW:             w,
		canvasH:             h,
		rot:                 rot,
		src:                 src,
	}
	if prof := k.object(root["colorProfile"]); prof != nil {
		d.ICCProfile = k.data(prof["SiColorProfileArchiveICCDataKey"])
		d.ICCName = k.str(prof["SiColorProfileArchiveICCNameKey"])
	}
	if comp := k.object(root["composite"]); comp != nil && k.int(comp["type"]) == layerTypeComposite {
		d.Composite = rasterOf(k, comp)
	}

	unmapped := map[int]bool{}
	d.Layers = collectNodes(k, k.list(root["layers"]), unmapped, 0)
	for b := range unmapped {
		d.UnmappedBlends = append(d.UnmappedBlends, b)
	}
	sort.Ints(d.UnmappedBlends)
	return d, nil
}

// maxLayerDepth bounds group nesting, which also stops a cyclic archive from
// looping (a SilicaGroup's children can, in principle, point back at it).
const maxLayerDepth = 32

func collectNodes(k *keyed, refs []any, unmapped map[int]bool, depth int) []*Node {
	if depth > maxLayerDepth {
		return nil
	}
	var out []*Node
	for _, r := range refs {
		obj := k.object(r)
		if obj == nil {
			continue
		}
		switch k.className(obj) {
		case "SilicaGroup":
			n := &Node{
				IsGroup: true,
				Name:    k.str(obj["name"]),
				Hidden:  k.bool(obj["isHidden"]),
				Locked:  k.bool(obj["isLocked"]),
				Clipped: k.bool(obj["isClipped"]),
				Opacity: clamp01(k.float(obj["opacity"])),
			}
			n.Children = collectNodes(k, k.list(obj["children"]), unmapped, depth+1)
			out = append(out, n)
		case "SilicaLayer":
			if k.int(obj["type"]) != layerTypeUser {
				continue
			}
			blend := k.int(obj["blend"])
			if _, ok := blendKey(blend); !ok {
				unmapped[blend] = true
			}
			out = append(out, &Node{
				Name:    k.str(obj["name"]),
				Hidden:  k.bool(obj["hidden"]),
				Locked:  k.bool(obj["locked"]),
				Clipped: k.bool(obj["clipped"]),
				Opacity: clamp01(k.float(obj["opacity"])),
				Blend:   blend,
				Raster:  rasterOf(k, obj),
			})
		}
	}
	// Procreate archives a layer array top-most first, the way the layer list
	// reads on screen. Document.Layers is defined bottom-most first, which is
	// what a compositor and the PSD layer records both want, so flip here — at
	// the single boundary where the archive's convention is known. Reversing at
	// every level (this function recurses for a group's children) keeps a
	// group's contents in step with the group itself.
	//
	// Getting this wrong inverts the artwork and, because Photoshop encodes a
	// group as a divider below its contents and a folder record above them,
	// also makes every group unreadable rather than merely misplaced.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func rasterOf(k *keyed, obj map[string]any) *Raster {
	uuid := k.str(obj["UUID"])
	if uuid == "" {
		return nil
	}
	return &Raster{
		UUID:   uuid,
		Width:  k.int(obj["sizeWidth"]),
		Height: k.int(obj["sizeHeight"]),
	}
}

// decodeFloatColor reads Procreate's backgroundColor: four little-endian
// float32 components in 0..1.
func decodeFloatColor(b []byte) color.NRGBA {
	if len(b) < 16 {
		return color.NRGBA{A: 0xff}
	}
	f := func(i int) uint8 {
		v := math.Float32frombits(uint32(b[i]) | uint32(b[i+1])<<8 | uint32(b[i+2])<<16 | uint32(b[i+3])<<24)
		return uint8(clamp01(float64(v))*255 + 0.5)
	}
	return color.NRGBA{R: f(0), G: f(4), B: f(8), A: f(12)}
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// FlatLayers walks the tree depth-first and returns every raster layer,
// bottom-most first.
func (d *Document) FlatLayers() []*Node {
	var out []*Node
	var walk func([]*Node)
	walk = func(ns []*Node) {
		for _, n := range ns {
			if n.IsGroup {
				walk(n.Children)
				continue
			}
			out = append(out, n)
		}
	}
	walk(d.Layers)
	return out
}
