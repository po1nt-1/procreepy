//go:build windows

package video

import (
	"os"
	"syscall"
	"unsafe"
)

var procGetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")

// isTTY reports whether fd refers to a console screen buffer.
func isTTY(fd int) bool {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(uintptr(fd), uintptr(unsafe.Pointer(&mode)), 0)
	return r != 0
}

// errorNoData is Win32's ERROR_NO_DATA, "The pipe is being closed." Of the
// two codes Windows uses for a vanished pipe reader, syscall exports only
// ERROR_BROKEN_PIPE ("The pipe has been ended."), so this one is spelled out
// here: x/sys/windows would break the zero-dependency invariant.
const errorNoData syscall.Errno = 232

// isBrokenPipe reports whether err means the reader on the other end went
// away. Windows never raises EPIPE — writing to a pipe whose reader has
// closed yields ERROR_NO_DATA or ERROR_BROKEN_PIPE — so without this mapping
// the message degraded to the raw Win32 text. EPIPE stays in the set in case
// the runtime ever normalizes it.
func isBrokenPipe(err error) bool {
	return errIs(err, syscall.EPIPE) ||
		errIs(err, syscall.ERROR_BROKEN_PIPE) ||
		errIs(err, errorNoData)
}

// devnullStdout repoints stdout at NUL: after a broken pipe so late writes
// cannot surface more I/O errors.
func devnullStdout() {
	f, err := os.OpenFile("NUL:", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	os.Stdout = f
}
