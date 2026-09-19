package video

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"procreepy/internal/procreate"
	"procreepy/internal/ui"
)

// Config tunes an operation. Reencode is accepted for command-line
// compatibility with the original tool; stream copy is always used.
type Config struct {
	Strict   bool
	Reencode bool
	TmpDir   string
	Split    bool // also write a video-less .procreepy.procreate beside the MP4
}

// resolvedInput is a seekable copy of the input plus its user-facing label.
type resolvedInput struct {
	path    string
	label   string
	cleanup func()
}

func (r *resolvedInput) close() {
	if r.cleanup != nil {
		r.cleanup()
	}
}

// resolveInput returns (seekable path, label). stdin ("-") and non-regular
// files (pipes, process substitution) are spooled to a temp file, because a
// ZIP needs random access.
func resolveInput(arg string, log *ui.Log, cfg Config) (*resolvedInput, error) {
	_ = log
	if arg == "-" {
		if os.Stdin == nil {
			return nil, &procreate.InputError{Msg: "stdin is not available"}
		}
		if isTTY(int(os.Stdin.Fd())) {
			return nil, &UsageError{Msg: "stdin is a terminal; pipe a .procreate file into it " +
				"(cat artwork.procreate | procreepy - ...)"}
		}
		return spool(os.Stdin, "<stdin>", "stdin", cfg)
	}
	st, err := os.Stat(arg)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &procreate.InputError{Msg: "input does not exist: " + arg}
	}
	if err != nil {
		return nil, &procreate.InputError{Msg: "cannot access input " + arg + ": " + strerror(err)}
	}
	if st.IsDir() {
		return nil, &procreate.InputError{Msg: "input is a directory: " + arg}
	}
	if st.Mode().IsRegular() {
		return &resolvedInput{path: arg, label: arg}, nil
	}
	f, err := os.Open(arg)
	if err != nil {
		return nil, &procreate.InputError{Msg: "cannot read input " + arg + ": " + strerror(err)}
	}
	defer f.Close()
	return spool(f, arg, "input", cfg)
}

// spool copies r to a temp file and returns its path.
func spool(r io.Reader, label, noun string, cfg Config) (*resolvedInput, error) {
	base := tempBase(cfg)
	dir, err := os.MkdirTemp(base, "procreepy-")
	if err != nil {
		return nil, &WriteError{Msg: "cannot create a temporary directory: " + strerror(err)}
	}
	fail := func(err error) (*resolvedInput, error) {
		os.RemoveAll(dir)
		if errors.Is(err, syscall.ENOSPC) {
			return nil, &WriteError{Msg: "no space left while buffering " + label +
				"; use --tmpdir (or $TMPDIR) on a bigger disk-backed directory"}
		}
		return nil, &WriteError{Msg: "cannot buffer " + label + ": " + strerror(err)}
	}
	path := filepath.Join(dir, "input.procreate")
	out, err := os.Create(path)
	if err != nil {
		return fail(err)
	}
	_, err = io.Copy(out, r)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fail(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		os.RemoveAll(dir)
		return nil, &procreate.InputError{Msg: noun + " is empty: " + label}
	}
	return &resolvedInput{path: path, label: label, cleanup: func() { os.RemoveAll(dir) }}, nil
}

// tempBase mirrors the original precedence: --tmpdir, $TMPDIR, /var/tmp
// (preferred over /tmp, which on Fedora is RAM-backed), system default.
func tempBase(cfg Config) string {
	for _, c := range []string{cfg.TmpDir, os.Getenv("TMPDIR")} {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	if st, err := os.Stat("/var/tmp"); err == nil && st.IsDir() {
		if f, err := os.CreateTemp("/var/tmp", "procreepy-w-"); err == nil {
			f.Close()
			os.Remove(f.Name())
			return "/var/tmp"
		}
	}
	return ""
}

// isTTY reports whether fd refers to a terminal (TCGETS ioctl, Linux).
func isTTY(fd int) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&t)))
	return errno == 0
}

// strerror renders an OS error like strerror(3), capitalized:
// "No such file or directory".
func strerror(err error) string {
	var en syscall.Errno
	if errors.As(err, &en) && en != 0 {
		s := en.Error()
		if s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return err.Error()
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			return strings.TrimSpace(ln)
		}
	}
	return s
}

// ctxErr returns ctx.Err() when the context is done, else nil.
func ctxErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
