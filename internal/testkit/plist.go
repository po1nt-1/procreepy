package testkit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// encodePlist serialises a value graph as a binary property list (bplist00).
//
// Only the object kinds a Procreate document needs are supported, and every
// object is emitted separately rather than deduplicated, which keeps the encoder
// short at the cost of a slightly larger file than Apple would write. Object
// references are 4 bytes wide throughout for the same reason.
func encodePlist(root any) []byte {
	e := &plistEncoder{}
	e.flatten(root)

	var body bytes.Buffer
	body.WriteString("bplist00")
	offsets := make([]uint64, len(e.objects))
	for i, o := range e.objects {
		offsets[i] = uint64(body.Len())
		e.encode(&body, o)
	}

	tableOffset := uint64(body.Len())
	for _, off := range offsets {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(off))
		body.Write(b[:])
	}
	var tr [32]byte
	tr[6] = 4 // offset entry size
	tr[7] = refSize
	binary.BigEndian.PutUint64(tr[8:16], uint64(len(e.objects)))
	binary.BigEndian.PutUint64(tr[16:24], 0) // top object is index 0
	binary.BigEndian.PutUint64(tr[24:32], tableOffset)
	body.Write(tr[:])
	return body.Bytes()
}

const refSize = 4

// plistEncoder assigns every value an index in the flat object table the format
// requires.
type plistEncoder struct {
	objects []any
}

// node is a value paired with the child indices it refers to, resolved during
// flattening so encode never has to recurse.
type node struct {
	value any
	keys  []uint32
	vals  []uint32
}

func (e *plistEncoder) flatten(v any) uint32 {
	idx := uint32(len(e.objects))
	e.objects = append(e.objects, nil) // reserve the slot before descending

	switch t := v.(type) {
	case map[string]any:
		names := make([]string, 0, len(t))
		for k := range t {
			names = append(names, k)
		}
		// Sorted keys keep the output deterministic.
		sort.Strings(names)
		n := &node{value: t}
		for _, k := range names {
			n.keys = append(n.keys, e.flatten(k))
		}
		for _, k := range names {
			n.vals = append(n.vals, e.flatten(t[k]))
		}
		e.objects[idx] = n
	case []any:
		n := &node{value: t}
		for _, item := range t {
			n.vals = append(n.vals, e.flatten(item))
		}
		e.objects[idx] = n
	default:
		e.objects[idx] = &node{value: v}
	}
	return idx
}

func (e *plistEncoder) encode(w *bytes.Buffer, obj any) {
	n := obj.(*node)
	switch t := n.value.(type) {
	case nil:
		w.WriteByte(0x00)
	case bool:
		if t {
			w.WriteByte(0x09)
		} else {
			w.WriteByte(0x08)
		}
	case int64:
		// Always 8 bytes: the reader accepts any width and this avoids a
		// sign-handling special case.
		w.WriteByte(0x13)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(t))
		w.Write(b[:])
	case float64:
		w.WriteByte(0x23)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], math.Float64bits(t))
		w.Write(b[:])
	case string:
		writeASCIIOrUTF16(w, t)
	case uidRef:
		// A UID's low nibble is its width minus one; four bytes matches refSize.
		w.WriteByte(0x83)
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(t))
		w.Write(b[:])
	case rawData:
		writeMarker(w, 0x40, len(t))
		w.Write(t)
	case []any:
		writeMarker(w, 0xa0, len(n.vals))
		writeRefs(w, n.vals)
	case map[string]any:
		writeMarker(w, 0xd0, len(n.keys))
		writeRefs(w, n.keys)
		writeRefs(w, n.vals)
	default:
		panic(fmt.Sprintf("testkit: cannot encode %T in a plist", n.value))
	}
}

func writeRefs(w *bytes.Buffer, refs []uint32) {
	for _, r := range refs {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], r)
		w.Write(b[:])
	}
}

// writeMarker writes a type marker whose low nibble is the element count, or the
// 0xf escape plus a separate integer when the count does not fit.
func writeMarker(w *bytes.Buffer, kind byte, n int) {
	if n < 15 {
		w.WriteByte(kind | byte(n))
		return
	}
	w.WriteByte(kind | 0x0f)
	w.WriteByte(0x13) // an 8-byte integer holds any count a test will produce
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n))
	w.Write(b[:])
}

// writeASCIIOrUTF16 picks the string encoding the format requires: the ASCII
// form cannot represent anything above U+007F.
func writeASCIIOrUTF16(w *bytes.Buffer, s string) {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		writeMarker(w, 0x50, len(s))
		w.WriteString(s)
		return
	}
	u := utf16Units(s)
	writeMarker(w, 0x60, len(u))
	for _, c := range u {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], c)
		w.Write(b[:])
	}
}
