//go:build linux

package video

import (
	"os"
	"syscall"
	"unsafe"
)

// isTTY reports whether fd refers to a terminal (TCGETS ioctl).
func isTTY(fd int) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&t)))
	return errno == 0
}

// devnullStdout points fd 1 at /dev/null so the shell does not complain
// about the dying writer after a broken pipe. dup3 with zero flags has the
// semantics of dup2, which the arm64 syscall table omits.
func devnullStdout() {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	syscall.Dup3(int(f.Fd()), 1, 0)
	f.Close()
}
