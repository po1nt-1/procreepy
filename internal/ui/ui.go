// Package ui handles user-facing messages. Every message goes to stderr:
// stdout is reserved for data (video bytes) or for the --list/--verify report.
package ui

import (
	"fmt"
	"io"
	"os"
)

// Log writes "info:" / "warning:" / "error:" lines to Out.
type Log struct {
	Out   io.Writer
	Quiet bool
}

// Default logs to os.Stderr.
var Default = &Log{Out: os.Stderr}

func (l *Log) emit(prefix, format string, args ...any) {
	fmt.Fprintf(l.Out, prefix+": "+format+"\n", args...)
}

// Info prints a progress line unless quiet.
func (l *Log) Info(format string, args ...any) {
	if l.Quiet {
		return
	}
	l.emit("info", format, args...)
}

// Warn prints a warning.
func (l *Log) Warn(format string, args ...any) { l.emit("warning", format, args...) }

// Error prints an error.
func (l *Log) Error(format string, args ...any) { l.emit("error", format, args...) }
