package procodec

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Apple's compression_lib LZ4 stream format (the one behind
// COMPRESSION_LZ4) is a chain of tagged blocks rather than a bare LZ4 block,
// so a plain LZ4 decoder cannot read Procreate's ".lz4" tiles.
var (
	magicLZ4Compressed = [4]byte{'b', 'v', '4', '1'}
	magicLZ4Stored     = [4]byte{'b', 'v', '4', '-'}
	magicLZ4End        = [4]byte{'b', 'v', '4', '$'}
)

// ErrNotAppleLZ4 reports input that is not an Apple LZ4 stream.
var ErrNotAppleLZ4 = errors.New("procodec: not an Apple LZ4 stream")

// IsAppleLZ4 reports whether b starts with an Apple LZ4 block tag.
func IsAppleLZ4(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	var m [4]byte
	copy(m[:], b)
	return m == magicLZ4Compressed || m == magicLZ4Stored || m == magicLZ4End
}

// DecodeAppleLZ4 decompresses an Apple compression_lib LZ4 stream into at most
// maxOut bytes.
//
// Every block appends to one output buffer instead of being decoded in
// isolation: Apple's encoder emits matches that reach back into blocks already
// decoded, so per-block decoding loses the history those matches need.
func DecodeAppleLZ4(src []byte, maxOut int) ([]byte, error) {
	if !IsAppleLZ4(src) {
		return nil, ErrNotAppleLZ4
	}
	out := make([]byte, 0, min(maxOut, 1<<20))
	for i := 0; i+4 <= len(src); {
		var m [4]byte
		copy(m[:], src[i:i+4])
		switch m {
		case magicLZ4End:
			return out, nil
		case magicLZ4Stored:
			if i+8 > len(src) {
				return nil, errors.New("procodec: truncated bv4- header")
			}
			n := int(binary.LittleEndian.Uint32(src[i+4:]))
			i += 8
			if n < 0 || i+n > len(src) {
				return nil, errors.New("procodec: truncated bv4- payload")
			}
			if len(out)+n > maxOut {
				return nil, errOverrun(len(out), n, maxOut)
			}
			out = append(out, src[i:i+n]...)
			i += n
		case magicLZ4Compressed:
			if i+12 > len(src) {
				return nil, errors.New("procodec: truncated bv41 header")
			}
			dSize := int(binary.LittleEndian.Uint32(src[i+4:]))
			cSize := int(binary.LittleEndian.Uint32(src[i+8:]))
			i += 12
			if dSize < 0 || cSize < 0 || i+cSize > len(src) {
				return nil, errors.New("procodec: truncated bv41 payload")
			}
			if len(out)+dSize > maxOut {
				return nil, errOverrun(len(out), dSize, maxOut)
			}
			before := len(out)
			var err error
			if out, err = lz4Block(src[i:i+cSize], out, dSize); err != nil {
				return nil, err
			}
			if got := len(out) - before; got != dSize {
				return nil, fmt.Errorf("procodec: bv41 block decoded %d bytes, header declares %d", got, dSize)
			}
			i += cSize
		default:
			return nil, fmt.Errorf("procodec: unknown LZ4 block tag %q at offset %d", m, i)
		}
	}
	return nil, errors.New("procodec: LZ4 stream ended without a bv4$ tag")
}

// lz4Block appends one LZ4 block to dst, stopping once want bytes have been
// produced. dst carries the history of previous blocks and is never reset.
func lz4Block(src, dst []byte, want int) ([]byte, error) {
	produced, i := 0, 0
	for i < len(src) && produced < want {
		tok := src[i]
		i++
		litLen := int(tok >> 4)
		if litLen == 15 {
			n, ni, err := varLen(src, i, litLen)
			if err != nil {
				return nil, err
			}
			litLen, i = n, ni
		}
		if i+litLen > len(src) {
			return nil, errors.New("procodec: truncated LZ4 literals")
		}
		dst = append(dst, src[i:i+litLen]...)
		produced += litLen
		i += litLen
		if produced >= want {
			break
		}
		if i+2 > len(src) {
			return nil, errors.New("procodec: truncated LZ4 match offset")
		}
		off := int(binary.LittleEndian.Uint16(src[i:]))
		i += 2
		if off == 0 || off > len(dst) {
			return nil, fmt.Errorf("procodec: bad LZ4 match offset %d (history %d)", off, len(dst))
		}
		mLen := int(tok & 0x0f)
		if mLen == 15 {
			n, ni, err := varLen(src, i, mLen)
			if err != nil {
				return nil, err
			}
			mLen, i = n, ni
		}
		mLen += 4
		// Overlapping matches are legal and common (run-length fills), so the
		// copy has to advance byte by byte rather than use copy().
		s := len(dst) - off
		for k := 0; k < mLen; k++ {
			dst = append(dst, dst[s+k])
		}
		produced += mLen
	}
	return dst, nil
}

// varLen reads LZ4's 255-chained length extension starting at i.
func varLen(src []byte, i, base int) (n, next int, err error) {
	n = base
	for {
		if i >= len(src) {
			return 0, 0, errors.New("procodec: truncated LZ4 length")
		}
		b := src[i]
		i++
		n += int(b)
		if b != 255 {
			return n, i, nil
		}
	}
}

func errOverrun(have, add, limit int) error {
	return fmt.Errorf("procodec: output overrun (%d + %d > %d)", have, add, limit)
}
