//go:build windows

package batch

// caseInsensitiveFS drives the path-equality guard. Windows volumes compare
// path names without regard to case, so two spellings of one directory must be
// treated as the same place.
const caseInsensitiveFS = true
