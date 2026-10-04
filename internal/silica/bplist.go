package silica

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf16"
)

// Ref is an NSKeyedArchiver object reference (a bplist UID), i.e. an index into
// the archive's $objects array.
type Ref uint64

// plist decodes Apple's binary property list container.
//
// Procreate's Document.archive is an NSKeyedArchiver graph inside a bplist, and
// the module carries no third-party dependencies, so the container is parsed
// here. Only the type markers Procreate actually emits are handled; anything
// else is reported rather than guessed at.
type plist struct {
	data    []byte
	offsets []uint64
	refSize int
}

const (
	trailerLen   = 32
	bplistHeader = "bplist00"
)

func parsePlist(data []byte) (*plist, any, error) {
	if len(data) < len(bplistHeader)+trailerLen {
		return nil, nil, errors.New("too short to be a binary property list")
	}
	if string(data[:6]) != "bplist" {
		return nil, nil, errors.New("not a binary property list")
	}
	tr := data[len(data)-trailerLen:]
	offSize := int(tr[6])
	refSize := int(tr[7])
	numObjects := binary.BigEndian.Uint64(tr[8:16])
	topObject := binary.BigEndian.Uint64(tr[16:24])
	tableOffset := binary.BigEndian.Uint64(tr[24:32])

	if offSize < 1 || offSize > 8 || refSize < 1 || refSize > 8 {
		return nil, nil, fmt.Errorf("implausible bplist trailer (offset size %d, ref size %d)", offSize, refSize)
	}
	if numObjects > uint64(len(data)) {
		return nil, nil, fmt.Errorf("bplist claims %d objects in %d bytes", numObjects, len(data))
	}
	end := tableOffset + numObjects*uint64(offSize)
	if tableOffset > uint64(len(data)) || end > uint64(len(data)) {
		return nil, nil, errors.New("bplist offset table is out of range")
	}
	offsets := make([]uint64, numObjects)
	for i := range offsets {
		offsets[i] = beUint(data[tableOffset+uint64(i*offSize):], offSize)
	}
	p := &plist{data: data, offsets: offsets, refSize: refSize}
	v, err := p.object(topObject, 0)
	if err != nil {
		return nil, nil, err
	}
	return p, v, nil
}

// maxPlistDepth stops a malicious or corrupt archive from driving the decoder
// into unbounded recursion.
const maxPlistDepth = 32

func (p *plist) object(i uint64, depth int) (any, error) {
	if depth > maxPlistDepth {
		return nil, errors.New("bplist nesting is too deep")
	}
	if i >= uint64(len(p.offsets)) {
		return nil, fmt.Errorf("bplist object reference %d out of range", i)
	}
	off := p.offsets[i]
	if off >= uint64(len(p.data)) {
		return nil, fmt.Errorf("bplist object %d starts past the end", i)
	}
	marker := p.data[off]
	hi, lo := marker>>4, marker&0x0f
	switch hi {
	case 0x0:
		switch lo {
		case 0x0:
			return nil, nil
		case 0x8:
			return false, nil
		case 0x9:
			return true, nil
		}
		return nil, fmt.Errorf("unsupported bplist singleton %#02x", marker)
	case 0x1: // integer, 2^lo bytes, big endian
		n := 1 << lo
		if err := p.bounds(off+1, uint64(n)); err != nil {
			return nil, err
		}
		return int64(beUint(p.data[off+1:], n)), nil
	case 0x2: // float
		n := 1 << lo
		if err := p.bounds(off+1, uint64(n)); err != nil {
			return nil, err
		}
		if n == 4 {
			return float64(math.Float32frombits(binary.BigEndian.Uint32(p.data[off+1:]))), nil
		}
		if n == 8 {
			return math.Float64frombits(binary.BigEndian.Uint64(p.data[off+1:])), nil
		}
		return nil, fmt.Errorf("unsupported bplist real width %d", n)
	case 0x3: // date: seconds since 2001-01-01, as a float64
		if err := p.bounds(off+1, 8); err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(p.data[off+1:])), nil
	case 0x4: // data
		n, base, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		if err := p.bounds(base, n); err != nil {
			return nil, err
		}
		return p.data[base : base+n], nil
	case 0x5: // ASCII string
		n, base, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		if err := p.bounds(base, n); err != nil {
			return nil, err
		}
		return string(p.data[base : base+n]), nil
	case 0x6: // UTF-16BE string
		n, base, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		if err := p.bounds(base, n*2); err != nil {
			return nil, err
		}
		u := make([]uint16, n)
		for k := uint64(0); k < n; k++ {
			u[k] = binary.BigEndian.Uint16(p.data[base+2*k:])
		}
		return string(utf16.Decode(u)), nil
	case 0x8: // UID: an NSKeyedArchiver object reference
		n := int(lo) + 1
		if err := p.bounds(off+1, uint64(n)); err != nil {
			return nil, err
		}
		return Ref(beUint(p.data[off+1:], n)), nil
	case 0xa, 0xc: // array, set
		n, base, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		if err := p.bounds(base, n*uint64(p.refSize)); err != nil {
			return nil, err
		}
		out := make([]any, n)
		for k := uint64(0); k < n; k++ {
			v, err := p.object(beUint(p.data[base+uint64(p.refSize)*k:], p.refSize), depth+1)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case 0xd: // dictionary
		n, base, err := p.count(off, lo)
		if err != nil {
			return nil, err
		}
		if err := p.bounds(base, 2*n*uint64(p.refSize)); err != nil {
			return nil, err
		}
		m := make(map[string]any, n)
		for k := uint64(0); k < n; k++ {
			key, err := p.object(beUint(p.data[base+uint64(p.refSize)*k:], p.refSize), depth+1)
			if err != nil {
				return nil, err
			}
			val, err := p.object(beUint(p.data[base+uint64(p.refSize)*(n+k):], p.refSize), depth+1)
			if err != nil {
				return nil, err
			}
			m[keyString(key)] = val
		}
		return m, nil
	}
	return nil, fmt.Errorf("unsupported bplist marker %#02x", marker)
}

// count reads an object's element count, which is either packed into the low
// nibble of the marker or, when that nibble is 0xf, stored as an integer after
// it.
func (p *plist) count(off uint64, lo byte) (n, base uint64, err error) {
	if lo != 0x0f {
		return uint64(lo), off + 1, nil
	}
	if err := p.bounds(off+1, 1); err != nil {
		return 0, 0, err
	}
	m := p.data[off+1]
	if m>>4 != 0x1 {
		return 0, 0, fmt.Errorf("bplist long count has marker %#02x, want an integer", m)
	}
	w := 1 << (m & 0x0f)
	if err := p.bounds(off+2, uint64(w)); err != nil {
		return 0, 0, err
	}
	return beUint(p.data[off+2:], w), off + 2 + uint64(w), nil
}

func (p *plist) bounds(off, n uint64) error {
	if off > uint64(len(p.data)) || n > uint64(len(p.data)) || off+n > uint64(len(p.data)) {
		return errors.New("bplist object runs past the end of the file")
	}
	return nil
}

func beUint(b []byte, n int) uint64 {
	var v uint64
	for i := 0; i < n && i < len(b); i++ {
		v = v<<8 | uint64(b[i])
	}
	return v
}

func keyString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprint(v)
	}
}
