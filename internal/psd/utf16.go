package psd

import "unicode/utf16"

// utf16Encode converts a Go string to UTF-16 code units for the luni block that
// carries a layer's real name.
func utf16Encode(s string) []uint16 { return utf16.Encode([]rune(s)) }
