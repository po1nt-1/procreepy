//go:build darwin

package cli

import (
	"syscall"
	"unsafe"
)

// isTerminal reports whether fd refers to a terminal device (TIOCGETA ioctl).
func isTerminal(fd int) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA),
		uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
