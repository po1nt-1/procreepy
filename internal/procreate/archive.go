package procreate

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"procreepy/internal/mp4"
)

// Archive is an opened .procreate file (a ZIP archive), read-only. The
// original file is never modified.
type Archive struct {
	label string
	zr    *zip.Reader
	file  *os.File
}

// Open opens path as a .procreate archive. label is the user-facing input
// name used in diagnostics (the path itself, or "-" for piped input).
func Open(path, label string) (*Archive, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, &InputError{Msg: fmt.Sprintf("cannot access input %s: %s", label, osErr(err))}
	}
	if fi.Size() == 0 {
		return nil, &InputError{Msg: "input is empty: " + label}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, &InputError{Msg: fmt.Sprintf("cannot read input %s: %s", label, osErr(err))}
	}
	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		f.Close()
		return nil, &InputError{Msg: "input is not a valid ZIP archive: " + label +
			" (not a .procreate file, or truncated/corrupted)"}
	}
	return &Archive{label: label, zr: zr, file: f}, nil
}

// Label is the user-facing input name.
func (a *Archive) Label() string { return a.label }

// Close releases the underlying file handle.
func (a *Archive) Close() error { return a.file.Close() }

// Members lists every archive entry in archive order.
func (a *Archive) Members() []Member {
	out := make([]Member, 0, len(a.zr.File))
	for _, f := range a.zr.File {
		out = append(out, Member{Name: f.Name, IsDir: f.FileInfo().IsDir()})
	}
	return out
}

// Segments discovers ordered time-lapse segments in this archive.
func (a *Archive) Segments(opts Options) ([]Segment, error) {
	return FindSegments(a.Members(), opts)
}

// ReadMember returns the full uncompressed bytes of a member (CRC verified).
func (a *Archive) ReadMember(name string) ([]byte, error) {
	f, err := a.member(name)
	if err != nil {
		return nil, err
	}
	rc, err := f.Open()
	if err != nil {
		return nil, &BadSegmentError{Msg: fmt.Sprintf("cannot read %s: %v", name, err)}
	}
	defer rc.Close()
	buf, err := io.ReadAll(rc)
	if err != nil {
		return nil, &BadSegmentError{Msg: fmt.Sprintf("%s is corrupted inside the archive: %v", name, err)}
	}
	return buf, nil
}

// ParseSegment reads one segment member out of the archive (verifying its
// CRC) and parses it as a progressive MP4.
func (a *Archive) ParseSegment(name string) (*mp4.Movie, error) {
	f, err := a.member(name)
	if err != nil {
		return nil, err
	}
	rc, err := f.Open()
	if err != nil {
		return nil, &BadSegmentError{Msg: fmt.Sprintf("cannot read segment %s: %v", name, err)}
	}
	buf, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, &BadSegmentError{Msg: fmt.Sprintf("segment %s is corrupted inside the archive: %v", name, err)}
	}
	movie, err := mp4.Parse(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, &BadSegmentError{Msg: fmt.Sprintf("segment %s: %v", name, err)}
	}
	if err := movie.Validate(); err != nil {
		return nil, &BadSegmentError{Msg: fmt.Sprintf("segment %s: %v", name, err)}
	}
	return movie, nil
}

// StreamMdat copies segSize bytes starting at segStart (offsets inside the
// member's uncompressed byte stream) to dst. The remainder of the member is
// drained so the ZIP CRC is still verified. It returns the number of bytes
// written to dst.
func (a *Archive) StreamMdat(name string, segStart, segSize int64, dst io.Writer) (int64, error) {
	f, err := a.member(name)
	if err != nil {
		return 0, err
	}
	rc, err := f.Open()
	if err != nil {
		return 0, &BadSegmentError{Msg: fmt.Sprintf("cannot read segment %s: %v", name, err)}
	}
	defer rc.Close()
	corrupt := func(err error) error {
		return &BadSegmentError{Msg: fmt.Sprintf("segment %s is corrupted inside the archive: %v", name, err)}
	}
	if segStart > 0 {
		if _, err := io.CopyN(io.Discard, rc, segStart); err != nil {
			return 0, corrupt(err)
		}
	}
	n, err := io.CopyN(dst, rc, segSize)
	if err == nil && n != segSize {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return n, corrupt(err)
	}
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return n, corrupt(err)
	}
	return n, nil
}

// StreamMdatCtx is StreamMdat that checks ctx (once per MiB) so an
// interrupted run unwinds cleanly through the caller's defers.
func (a *Archive) StreamMdatCtx(ctx context.Context, name string, segStart, segSize int64, dst io.Writer) (int64, error) {
	f, err := a.member(name)
	if err != nil {
		return 0, err
	}
	rc, err := f.Open()
	if err != nil {
		return 0, &BadSegmentError{Msg: fmt.Sprintf("cannot read segment %s: %v", name, err)}
	}
	defer rc.Close()
	corrupt := func(err error) error {
		return &BadSegmentError{Msg: fmt.Sprintf("segment %s is corrupted inside the archive: %v", name, err)}
	}
	if err := ctxSkip(ctx, rc, segStart); err != nil {
		return 0, corrupt(err)
	}
	n, err := ctxCopyN(ctx, dst, rc, segSize)
	if err == nil && n != segSize {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		return n, corrupt(err)
	}
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return n, corrupt(err)
	}
	return n, nil
}

func ctxCheck(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func ctxSkip(ctx context.Context, r io.Reader, n int64) error {
	buf := make([]byte, 1<<20)
	for n > 0 {
		if err := ctxCheck(ctx); err != nil {
			return err
		}
		k := int64(len(buf))
		if n < k {
			k = n
		}
		m, err := r.Read(buf[:k])
		if m > 0 {
			n -= int64(m)
		}
		if err == io.EOF {
			if n > 0 {
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
		if m == 0 {
			return io.ErrUnexpectedEOF
		}
	}
	return nil
}

func ctxCopyN(ctx context.Context, dst io.Writer, src io.Reader, n int64) (int64, error) {
	buf := make([]byte, 1<<20)
	var written int64
	for n > 0 {
		if err := ctxCheck(ctx); err != nil {
			return written, err
		}
		k := int64(len(buf))
		if n < k {
			k = n
		}
		m, rerr := src.Read(buf[:k])
		if m > 0 {
			w, werr := dst.Write(buf[:m])
			if werr != nil {
				return written, werr
			}
			written += int64(w)
			n -= int64(m)
		}
		if rerr == io.EOF {
			if n > 0 {
				return written, io.ErrUnexpectedEOF
			}
			return written, nil
		}
		if rerr != nil {
			return written, rerr
		}
		if m == 0 {
			return written, io.ErrUnexpectedEOF
		}
	}
	return written, nil
}

func (a *Archive) member(name string) (*zip.File, error) {
	for _, f := range a.zr.File {
		if f.Name == name {
			return f, nil
		}
	}
	return nil, &BadSegmentError{Msg: fmt.Sprintf("segment %s: no such member in %s", name, a.label)}
}

// osErr renders an OS error like strerror(3) ("No such file or directory").
func osErr(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		s := pe.Err.Error()
		if s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return err.Error()
}
