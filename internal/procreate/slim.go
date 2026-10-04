package procreate

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
)

// SlimResult reports what WriteSlimmed did to an archive.
type SlimResult struct {
	Kept         int   // members carried over
	Removed      int   // timelapse segments dropped
	RemovedBytes int64 // compressed size of the dropped segments
}

// copyChunk bounds how much of a member moves between context checks, so a
// cancellation lands promptly even inside a multi-megabyte entry.
const copyChunk = 256 << 10

// WriteSlimmed writes a copy of the archive to w with the timelapse segments
// removed and every other member preserved.
//
// Members are transferred as raw, still-compressed ZIP entries. That keeps the
// compressed payload, CRC, sizes, compression method, MS-DOS timestamp, extra
// fields, and general-purpose flags bit-identical, and it is also the only
// correct way to do this: handing a *zip.File's FileHeader to
// zip.Writer.CreateHeader makes the writer OR the data-descriptor flag into the
// header the reader is still using, after which the reader looks for a
// descriptor that Procreate never wrote and fails every member with
// zip.ErrChecksum.
func (a *Archive) WriteSlimmed(ctx context.Context, w io.Writer) (SlimResult, error) {
	var res SlimResult
	zw := zip.NewWriter(w)
	for _, f := range a.zr.File {
		if err := ctxCheck(ctx); err != nil {
			return res, err
		}
		if IsSegmentName(f.Name) {
			res.Removed++
			res.RemovedBytes += int64(f.CompressedSize64)
			continue
		}
		if err := copyRaw(ctx, zw, f); err != nil {
			return res, err
		}
		res.Kept++
	}
	if err := zw.Close(); err != nil {
		return res, fmt.Errorf("cannot finish the slimmed archive: %w", err)
	}
	return res, nil
}

// copyRaw transfers one member without decompressing it. The header is copied
// by value: zip.Writer mutates the header it is given, and f.FileHeader lives
// inside the reader that is still serving the remaining members.
func copyRaw(ctx context.Context, zw *zip.Writer, f *zip.File) error {
	hdr := f.FileHeader
	dst, err := zw.CreateRaw(&hdr)
	if err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	src, err := f.OpenRaw()
	if err != nil {
		return fmt.Errorf("%s: %w", f.Name, err)
	}
	buf := make([]byte, copyChunk)
	for {
		if err := ctxCheck(ctx); err != nil {
			return err
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return fmt.Errorf("%s: %w", f.Name, werr)
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return fmt.Errorf("%s: %w", f.Name, rerr)
		}
	}
}
