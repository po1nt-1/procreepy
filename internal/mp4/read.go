// Package mp4 is a minimal ISO base media file format (MP4) reader and writer.
//
// It is deliberately narrow: it parses the parts of a progressive MP4 file
// that procreepy needs to validate and re-mux video segments (stream copy),
// and it refuses anything it does not understand. All parsing functions take
// an io.ReaderAt over the whole file and never assume trusted input.
package mp4

import (
	"errors"
	"fmt"
)

// Errors returned while parsing untrusted input.
var (
	ErrTruncated    = errors.New("truncated file")
	ErrBadStructure = errors.New("malformed box structure")
	ErrUnsupported  = errors.New("unsupported MP4 construct")
	ErrIncompatible = errors.New("segments are not stream-copy compatible")
)

// MaxMoovSize bounds how much of a file may be occupied by the moov box.
// A real timelapse segment has a small moov; anything bigger is either not
// what we expect or an attempt to exhaust memory.
const MaxMoovSize = 512 << 20

// Box is one top- or second-level box: header plus payload.
type Box struct {
	Type   string
	Off    int64 // offset of the box (size field) within the file
	Size   int64 // total size including headers
	PayOff int64 // offset of the payload
	Pay    []byte
}

// Reader is a bounds-checked io.ReaderAt wrapper.
type Reader struct {
	r interface {
		ReadAt(p []byte, off int64) (int, error)
	}
	size int64
}

// NewReader wraps r of the given total size.
func NewReader(r interface {
	ReadAt(p []byte, off int64) (int, error)
}, size int64) *Reader {
	return &Reader{r: r, size: size}
}

// Size returns the total number of bytes available.
func (rd *Reader) Size() int64 { return rd.size }

// ReadAt reads len(p) bytes at off, rejecting out-of-range accesses.
func (rd *Reader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || int64(len(p)) > rd.size-off {
		return 0, ErrTruncated
	}
	n, err := rd.r.ReadAt(p, off)
	if err != nil && err.Error() == "unexpected EOF" {
		err = ErrTruncated
	}
	return n, err
}

func validFourCC(b []byte) error {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return fmt.Errorf("%w: invalid box type %# x", ErrBadStructure, b)
		}
	}
	return nil
}

// ScanBoxes lists sibling boxes covering the half-open range [off, end).
// Each returned box carries its payload in full.
func ScanBoxes(rd *Reader, off, end int64) ([]Box, error) {
	if off < 0 || end > rd.size || off > end {
		return nil, ErrTruncated
	}
	var boxes []Box
	p := off
	for p < end {
		if end-p < 8 {
			return nil, fmt.Errorf("%w: %d trailing bytes at offset %d", ErrBadStructure, end-p, p)
		}
		var hdr [8]byte
		if _, err := rd.ReadAt(hdr[:], p); err != nil {
			return nil, err
		}
		size := int64(u32be(hdr[0:4]))
		typ := string(hdr[4:8])
		if err := validFourCC(hdr[4:8]); err != nil {
			return nil, err
		}
		header := int64(8)
		switch size {
		case 0: // box extends to the end of the file
			size = rd.size - p
		case 1:
			var large [8]byte
			if _, err := rd.ReadAt(large[:], p+8); err != nil {
				return nil, err
			}
			size = int64(u64be(large[:]))
			header = 16
			if large[0] != 0 || large[1] != 0 { // > 2^48: not realistic
				return nil, fmt.Errorf("%w: implausible box size at offset %d", ErrBadStructure, p)
			}
		default:
			if size < 8 {
				return nil, fmt.Errorf("%w: box %q at offset %d is %d bytes", ErrBadStructure, typ, p, size)
			}
		}
		if size < header {
			return nil, fmt.Errorf("%w: box %q at offset %d is %d bytes", ErrBadStructure, typ, p, size)
		}
		if p+size > end {
			return nil, fmt.Errorf("%w: box %q at offset %d overflows its container", ErrBadStructure, typ, p)
		}
		b := Box{Type: typ, Off: p, Size: size, PayOff: p + header}
		pay := size - header
		if pay > 0 {
			b.Pay = make([]byte, pay)
			if _, err := rd.ReadAt(b.Pay, b.PayOff); err != nil {
				return nil, err
			}
		}
		boxes = append(boxes, b)
		p += size
	}
	return boxes, nil
}

func u16be(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }

func u32be(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func u64be(b []byte) uint64 {
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v
}

// versionFlags extracts version and flags from a full box payload.
func versionFlags(pay []byte) (uint8, uint32, error) {
	if len(pay) < 4 {
		return 0, 0, ErrTruncated
	}
	return pay[0], uint32(pay[1])<<16 | uint32(pay[2])<<8 | uint32(pay[3]), nil
}

// children scans the boxes inside a container payload (e.g. moov, trak).
func children(pay []byte) ([]Box, error) {
	rd := NewReader(&sliceReader{b: pay}, int64(len(pay)))
	// Offsets are relative to the payload start.
	return ScanBoxes(rd, 0, int64(len(pay)))
}

// sliceReader adapts a []byte to io.ReaderAt semantics.
type sliceReader struct{ b []byte }

func (sr *sliceReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(sr.b)) {
		return 0, errors.New("unexpected EOF")
	}
	n := copy(p, sr.b[off:])
	if n < len(p) {
		return n, errors.New("unexpected EOF")
	}
	return n, nil
}
