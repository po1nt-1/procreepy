package psd

// packBits compresses one scanline with the PackBits run-length encoding PSD
// calls "RLE". Layer pixels are mostly untouched canvas, so run-length coding
// shrinks a typical export by an order of magnitude; raw channel data would make
// a multi-layer 4K document unusably large.
//
// The encoding alternates control bytes: n in 0..127 means "the next n+1 bytes
// are literal", and n in -1..-127 means "repeat the next byte 1-n times".
func packBits(dst, src []byte) []byte {
	for i := 0; i < len(src); {
		// A run is only worth encoding at three bytes or more, except at the very
		// end of the line where two already breaks even.
		run := runLength(src, i)
		if run >= 3 || (run >= 2 && i+run == len(src)) {
			for run > 0 {
				n := min(run, 128)
				dst = append(dst, byte(int8(-(n - 1))), src[i])
				i += n
				run -= n
			}
			continue
		}
		start := i
		for i < len(src) && i-start < 128 {
			r := runLength(src, i)
			if r >= 3 || (r >= 2 && i+r == len(src)) {
				break
			}
			i += max(r, 1)
		}
		if i-start > 128 {
			i = start + 128
		}
		dst = append(dst, byte(i-start-1))
		dst = append(dst, src[start:i]...)
	}
	return dst
}

// runLength counts how many times src[i] repeats from i.
func runLength(src []byte, i int) int {
	n := 1
	for i+n < len(src) && src[i+n] == src[i] && n < 128 {
		n++
	}
	return n
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
