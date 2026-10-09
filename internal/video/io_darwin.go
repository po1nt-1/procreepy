//go:build darwin

package video

import (
	"os"
	"syscall"
	"unsafe"
)

// isTTY reports whether fd refers to a terminal (TIOCGETA ioctl, BSD).
func isTTY(fd int) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA),
		uintptr(unsafe.Pointer(&t)))
	return errno == 0
}

// isBrokenPipe reports whether err means the reader on the other end went
// away. Unix raises EPIPE; see io_windows.go for the two codes Win32 uses.
func isBrokenPipe(err error) bool { return errIs(err, syscall.EPIPE) }

// devnullStdout points fd 1 at /dev/null so the shell does not complain
// about the dying writer after a broken pipe.
func devnullStdout() {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	syscall.Syscall(syscall.SYS_DUP2, f.Fd(), 1, 0)
	f.Close()
}
