package video

import (
	"os"
	"time"
)

// createTimeOf reports no creation time. Linux can read a birth time through
// statx, but os.FileInfo does not carry it, and nothing here can set one
// anyway, so the caller falls back to the modification time.
func createTimeOf(os.FileInfo) time.Time { return time.Time{} }

// setCreateTime is a no-op on Linux: the kernel exposes a birth time through
// statx for reading, but offers no interface for setting one. Published outputs
// therefore carry the source's modification time (which os.Chtimes does set)
// and their own creation time.
//
// Returning nil rather than an error is deliberate — a platform that cannot
// record the attribute is not a failed conversion.
func setCreateTime(string, time.Time) error { return nil }
