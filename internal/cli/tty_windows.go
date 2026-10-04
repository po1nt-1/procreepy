//go:build windows

package cli

import (
	"syscall"
	"unsafe"
)

var (
	procGetStdHandle   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetStdHandle")
	procGetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")
	procSetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")
)

const (
	// ENABLE_VIRTUAL_TERMINAL_PROCESSING: makes the console interpret ANSI
	// escape sequences (Windows 10+).
	vtProcessing = 0x0004
)

// isTerminal reports whether stderr is a console able to display ANSI color.
// The console APIs address the process standard handles, so the fd is
// accepted only for symmetry with the Unix shims. Windows consoles ignore
// escape sequences until VT processing is switched on, so this switches it
// on; if the console refuses (older systems, redirected output) it falls
// back to false and the logger stays plain. The pattern follows
// internal/video/io_windows.go: Win32 BOOL results are read from the return
// value, not from the (meaningless here) error slot.
func isTerminal(fd int) bool {
	// STD_ERROR_HANDLE is the pseudo-handle index -12; as an unsigned
	// machine value that is ^uintptr(11).
	h, _, _ := procGetStdHandle.Call(^uintptr(11), 0, 0)
	if h == 0 || h == ^uintptr(0) { // NULL or INVALID_HANDLE_VALUE
		return false
	}
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)), 0)
	if r == 0 {
		return false
	}
	if mode&vtProcessing != 0 {
		return true
	}
	r, _, _ = procSetConsoleMode.Call(h, uintptr(mode|vtProcessing), 0)
	return r != 0
}
