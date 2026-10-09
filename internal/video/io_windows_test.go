//go:build windows

package video

import (
	"io/fs"
	"syscall"
	"testing"
)

// Every Win32 code for a vanished pipe reader must collapse onto the one
// "broken pipe" message, so the stdout contract reads identically on all
// platforms. The errnos arrive wrapped in a PathError: that is how
// os.File.Write hands them back.
func TestIsBrokenPipeWindows(t *testing.T) {
	for _, en := range []syscall.Errno{syscall.ERROR_BROKEN_PIPE, errorNoData, syscall.EPIPE} {
		if !isBrokenPipe(&fs.PathError{Op: "write", Path: "stdout", Err: en}) {
			t.Errorf("errno %d (%v) not recognized as a broken pipe", uintptr(en), en)
		}
	}
	if isBrokenPipe(&fs.PathError{Op: "write", Path: "out.mp4", Err: syscall.ERROR_ACCESS_DENIED}) {
		t.Error("ERROR_ACCESS_DENIED must not read as a broken pipe")
	}
}
