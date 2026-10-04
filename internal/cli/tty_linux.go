//go:build linux

package cli

import (
	"syscall"
	"unsafe"
)

// isTerminal reports whether fd refers to a terminal device (TCGETS ioctl).
func isTerminal(fd int) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
