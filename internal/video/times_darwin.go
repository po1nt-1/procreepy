package video

import (
	"os"
	"time"
)

// createTimeOf reports no creation time: the birth time is in the stat buffer
// macOS fills in, but os.FileInfo does not expose it, and setCreateTime below
// cannot write one yet regardless.
func createTimeOf(os.FileInfo) time.Time { return time.Time{} }

// setCreateTime is a no-op on macOS for now.
//
// APFS does keep a birth time, and Finder's "Date Created" column reads it, so
// there is something worth setting here. The only interface is setattrlist(2),
// which the standard library does not wrap: it would mean a raw syscall through
// unsafe pointers into a hand-written attrlist struct. That is not code to ship
// unverified, and this project has no macOS machine or CI runner to verify it
// on — the suite runs on Linux and, for Windows, under Wine and a hosted
// runner.
//
// Until one exists, macOS behaves like Linux: the modification time is carried
// over by os.Chtimes and the creation time is the moment of conversion.
func setCreateTime(string, time.Time) error { return nil }
