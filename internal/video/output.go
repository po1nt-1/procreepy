package video

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// OutputKind is where the MP4 bytes go.
type OutputKind int

const (
	// OutStdout: MP4 to stdout.
	OutStdout OutputKind = iota
	// OutFile: regular file; written atomically via a sibling .partial file.
	OutFile
	// OutDevice: device or FIFO (e.g. /dev/stdout); written directly.
	OutDevice
)

// Output is a resolved output destination. Path is canonical (absolute for
// OutFile); Name is the user-facing spelling used in messages.
type Output struct {
	Kind OutputKind
	Path string
	Name string
}

// ResolveOutput interprets the OUTPUT argument. A nil/false output (or "-")
// means stdout. A trailing slash or an existing directory gets <stem>.mp4
// inside it.
func ResolveOutput(inputArg, outputArg string, hasOutput bool) (Output, error) {
	if !hasOutput || outputArg == "-" {
		return Output{Kind: OutStdout}, nil
	}
	if strings.HasSuffix(outputArg, string(os.PathSeparator)) || isDir(outputArg) {
		pipeLike := inputArg == "-"
		if !pipeLike {
			// A non-existent input is not pipe-like; it fails later with a
			// clearer "input does not exist" message.
			if st, err := os.Stat(inputArg); err == nil && !st.Mode().IsRegular() {
				pipeLike = true
			}
		}
		if pipeLike {
			return Output{}, &UsageError{Msg: "cannot derive an output file name from stdin or a " +
				"pipe; pass a file name as OUTPUT"}
		}
		if !isDir(outputArg) {
			return Output{}, &WriteError{Msg: "output directory does not exist: " + outputArg}
		}
		p := filepath.Join(outputArg, deriveName(inputArg))
		return Output{Kind: OutFile, Path: p, Name: p}, nil
	}
	return Output{Kind: OutFile, Path: outputArg, Name: outputArg}, nil
}

// deriveName maps "artwork.procreate" -> "artwork.mp4".
func deriveName(input string) string {
	b := filepath.Base(input)
	i := strings.LastIndex(b, ".")
	if i <= 0 {
		return b + ".mp4"
	}
	return b[:i] + ".mp4"
}

// slimPath maps an MP4 output path to the slimmed archive next to it:
// "artwork.mp4" -> "artwork.procreepy.procreate".
func slimPath(mp4Path string) string {
	b := filepath.Base(mp4Path)
	if i := strings.LastIndex(b, "."); i > 0 {
		b = b[:i]
	}
	return filepath.Join(filepath.Dir(mp4Path), b+".procreepy.procreate")
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isRegular(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// prepare validates the destination and (for OutFile) checks the classic
// pitfalls. It may downgrade OutFile to OutDevice when the target exists but
// is not a regular file.
func (o *Output) prepare(inputArg string) error {
	switch o.Kind {
	case OutStdout:
		if isTTY(int(os.Stdout.Fd())) {
			return &UsageError{Msg: "refusing to write video data to a terminal; redirect stdout " +
				"(> artwork.mp4) or give an OUTPUT path"}
		}
	case OutFile:
		abs, err := filepath.Abs(o.Path)
		if err != nil {
			return &UsageError{Msg: err.Error()}
		}
		o.Path = abs
		if st, err := os.Stat(abs); err == nil && !st.Mode().IsRegular() {
			o.Kind = OutDevice // device / FIFO: stream directly, no rename
			return nil
		}
		if stOut, errOut := os.Stat(abs); errOut == nil &&
			inputArg != "-" && isRegular(inputArg) {
			if stIn, errIn := os.Stat(inputArg); errIn == nil && sameFile(stIn, stOut) {
				return &UsageError{Msg: "OUTPUT is the same file as INPUT: " + o.Path}
			}
		}
		parent := filepath.Dir(o.Path)
		st, err := os.Stat(parent)
		if err != nil || !st.IsDir() {
			return &WriteError{Msg: "output directory does not exist: " + parent}
		}
		if probe, err := os.CreateTemp(parent, ".procreepy-w-"); err != nil {
			return &WriteError{Msg: "output directory is not writable: " + parent}
		} else {
			probe.Close()
			os.Remove(probe.Name())
		}
	}
	return nil
}

// partialName is the sibling temp file for an atomic OutFile write.
func (o Output) partialName() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return filepath.Join(filepath.Dir(o.Path),
		fmt.Sprintf(".procreepy-%d-%s.partial", os.Getpid(), hex.EncodeToString(b[:])))
}

func sameFile(a, b os.FileInfo) bool {
	sa, ok := a.Sys().(*syscall.Stat_t)
	sb, ok := b.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return sa.Dev == sb.Dev && sa.Ino == sb.Ino
}

// errIs reports whether err (or a wrapped cause) is the given OS errno.
func errIs(err error, en syscall.Errno) bool {
	var e syscall.Errno
	return errors.As(err, &e) && e == en
}
