//go:build !windows

package batch

// caseInsensitiveFS drives the path-equality guard. It stays false off Windows
// even though macOS volumes are usually case-insensitive too: folding case on a
// case-sensitive filesystem would reject two directories that really are
// distinct, and the nesting check already covers the case that matters.
const caseInsensitiveFS = false
