package testkit

import "unicode/utf16"

// utf16Units converts a string to UTF-16 code units for the plist encoder.
func utf16Units(s string) []uint16 { return utf16.Encode([]rune(s)) }
