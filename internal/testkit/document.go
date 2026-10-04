package testkit

import (
	"encoding/binary"
	"math"
)

// DocumentArchive builds a synthetic Document.archive: an NSKeyedArchiver graph
// in a binary property list, describing a canvas with one named raster layer
// (sharing LayerUUID with Project) inside one group, plus a composite layer.
//
// Writing the container by hand keeps the silica and psd packages testable
// without committing a real Procreate file.
func DocumentArchive(w, h int) []byte {
	return DocumentArchiveLayer(w, h, LayerOpts{Name: "Ink", Opacity: 1})
}

// DocumentArchiveBlend is DocumentArchive with the raster layer's blend index
// set, so tests can drive the blend-mode mapping (including indices this build
// does not know).
func DocumentArchiveBlend(w, h, blend int) []byte {
	return DocumentArchiveLayer(w, h, LayerOpts{Name: "Ink", Opacity: 1, Blend: blend})
}

// LayerOpts sets the raster layer's fields in a synthetic document.
type LayerOpts struct {
	Name    string
	Opacity float64
	Blend   int
	Hidden  bool
	Locked  bool
	Clipped bool
	// UUID overrides the tile set the layer points at; an unknown one models a
	// layer whose pixels were never drawn.
	UUID string
}

// DocumentArchiveLayer builds a document whose single raster layer carries opts.
func DocumentArchiveLayer(w, h int, opts LayerOpts) []byte {
	uuid := opts.UUID
	if uuid == "" {
		uuid = layerUUID
	}
	blend := opts.Blend
	b := newPlistBuilder()

	// $objects[0] is the archiver's canonical nil.
	nilRef := b.add(nil)
	_ = nilRef

	// Procreate archives the two extents in the order the tile grid uses, which
	// is height first; a synthetic document has to match or it would not model
	// the file it stands in for.
	sizeRef := b.add(cgSize(h, w))
	nameRef := b.add(opts.Name)
	groupNameRef := b.add("Group 1")

	layerClass := b.class("SilicaLayer", []string{"SilicaLayer", "NSObject"})
	groupClass := b.class("SilicaGroup", []string{"SilicaGroup", "NSObject"})
	docClass := b.class("SilicaDocument", []string{"SilicaDocument", "NSObject"})
	arrClass := b.class("NSMutableArray", []string{"NSMutableArray", "NSArray", "NSObject"})
	profClass := b.class("ValkyrieColorProfile", []string{"ValkyrieColorProfile", "NSObject"})

	profRef := b.add(map[string]any{
		"$class":                          profClass,
		"SiColorProfileArchiveICCNameKey": b.add("sRGB IEC61966-2.1"),
		"SiColorProfileArchiveICCDataKey": b.add(rawData(minimalICC())),
	})

	layerRef := b.add(map[string]any{
		"$class":     layerClass,
		"UUID":       b.add(uuid),
		"name":       nameRef,
		"hidden":     opts.Hidden,
		"locked":     opts.Locked,
		"clipped":    opts.Clipped,
		"opacity":    opts.Opacity,
		"blend":      int64(blend),
		"type":       int64(0),
		"sizeWidth":  int64(w),
		"sizeHeight": int64(h),
		"version":    int64(4),
		"mask":       nilRef,
		"text":       nilRef,
		"textPDF":    nilRef,
	})
	childrenRef := b.add(map[string]any{
		"$class":     arrClass,
		"NS.objects": []any{layerRef},
	})
	groupRef := b.add(map[string]any{
		"$class":      groupClass,
		"name":        groupNameRef,
		"children":    childrenRef,
		"isHidden":    false,
		"isLocked":    false,
		"isClipped":   false,
		"isCollapsed": false,
		"opacity":     float64(1),
	})
	layersRef := b.add(map[string]any{
		"$class":     arrClass,
		"NS.objects": []any{groupRef},
	})
	compositeRef := b.add(map[string]any{
		"$class":     layerClass,
		"UUID":       b.add(compositeUUID),
		"name":       nilRef,
		"hidden":     false,
		"opacity":    float64(1),
		"blend":      int64(0),
		"type":       int64(1),
		"sizeWidth":  int64(w),
		"sizeHeight": int64(h),
	})

	docRef := b.add(map[string]any{
		"$class":                      docClass,
		"size":                        sizeRef,
		"tileSize":                    int64(TileSize),
		"SilicaDocumentArchiveDPIKey": int64(132),
		"layers":                      layersRef,
		"composite":                   compositeRef,
		"colorProfile":                profRef,
		"backgroundColor":             b.add(rawData(floatColor(1, 1, 1, 1))),
		"backgroundHidden":            false,
		// Orientation 4 is Procreate's "no rotation", which keeps synthetic
		// documents in display space and leaves the rotation itself to the
		// real-corpus suite, where every value occurs.
		"orientation":         int64(4),
		"flippedHorizontally": false,
		"flippedVertically":   false,
		"version":             int64(2),
	})

	return b.finish(map[string]any{"root": docRef})
}

// TileSize is the tile edge synthetic documents declare. Procreate itself uses
// 256, but the value is a document field the reader honours, and a smaller grid
// keeps generated archives small while still covering multi-tile assembly and
// clipped edge tiles. The real-corpus suite exercises 256.
const TileSize = 64

const compositeUUID = "11111111-2222-3333-4444-555555555555"

// CompositeUUID is the UUID DocumentArchive uses for the flattened composite.
func CompositeUUID() string { return compositeUUID }

func cgSize(w, h int) string {
	return "{" + itoa(w) + ", " + itoa(h) + "}"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func floatColor(r, g, bl, a float32) []byte {
	out := make([]byte, 0, 16)
	for _, v := range []float32{r, g, bl, a} {
		out = binary.LittleEndian.AppendUint32(out, math.Float32bits(v))
	}
	return out
}

// minimalICC is a stand-in profile: the exporter copies the bytes through
// without interpreting them, so only the length and round-trip matter.
func minimalICC() []byte {
	b := make([]byte, 128)
	copy(b[36:], "acsp")
	binary.BigEndian.PutUint32(b, uint32(len(b)))
	return b
}

// rawData marks a byte slice so the builder emits a bplist data object rather
// than trying to treat it as something else.
type rawData []byte

// plistBuilder assembles the $objects table of an NSKeyedArchiver plist and
// serialises it as bplist00.
type plistBuilder struct {
	objects []any
	classes map[string]uidRef
}

// uidRef is a reference to an entry in the $objects table.
type uidRef uint64

func newPlistBuilder() *plistBuilder {
	return &plistBuilder{classes: map[string]uidRef{}}
}

func (b *plistBuilder) add(v any) uidRef {
	b.objects = append(b.objects, v)
	return uidRef(len(b.objects) - 1)
}

func (b *plistBuilder) class(name string, hierarchy []string) uidRef {
	if r, ok := b.classes[name]; ok {
		return r
	}
	names := make([]any, 0, len(hierarchy))
	for _, h := range hierarchy {
		names = append(names, h)
	}
	r := b.add(map[string]any{
		"$classname": name,
		"$classes":   names,
	})
	b.classes[name] = r
	return r
}

func (b *plistBuilder) finish(top map[string]any) []byte {
	// $objects[0] must be the string "$null"; NSKeyedArchiver uses reference 0 as
	// nil and readers special-case it.
	if len(b.objects) > 0 && b.objects[0] == nil {
		b.objects[0] = "$null"
	}
	root := map[string]any{
		"$version":  int64(100000),
		"$archiver": "NSKeyedArchiver",
		"$top":      top,
		"$objects":  toAnySlice(b.objects),
	}
	return encodePlist(root)
}

func toAnySlice(in []any) []any {
	out := make([]any, len(in))
	copy(out, in)
	return out
}
