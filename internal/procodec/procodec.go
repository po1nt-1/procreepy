// Package procodec decodes the two compressed containers Procreate uses for
// layer tiles: Apple's compression_lib LZ4 stream (".lz4") and a bare LZO1X-1
// stream (".chunk"). Both are implemented here from the format definitions
// because the module carries no third-party dependencies.
package procodec

import "fmt"

// DecodeTile decompresses one layer tile, picking the codec from the payload
// rather than the file extension: a handful of projects in the wild mix the two
// containers inside a single archive, and the extension is not a reliable tag.
func DecodeTile(src []byte, maxOut int) ([]byte, error) {
	if IsAppleLZ4(src) {
		return DecodeAppleLZ4(src, maxOut)
	}
	out, err := DecodeLZO1X(src, maxOut)
	if err != nil {
		return nil, fmt.Errorf("tile is neither an Apple LZ4 nor an LZO1X stream: %w", err)
	}
	return out, nil
}
