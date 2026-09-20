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

// devnullStdout repoints stdout at NUL: after a broken pipe so late writes
// cannot surface more I/O errors.
func devnullStdout() {
	f, err := os.OpenFile("NUL:", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	os.Stdout = f
}
