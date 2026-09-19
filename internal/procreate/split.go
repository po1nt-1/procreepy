package procreate

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// videoPrefix marks archive members that belong to the time-lapse recording.
// SplitTimelapse drops everything under it (case-insensitively, mirroring the
// segment scanner).
const videoPrefix = "video/"

// SplitTimelapse rewrites the archive at srcPath to dstPath without every
// member under video/. All other members are preserved exactly: order,
// names, compression methods, timestamps, and bytes. The write is atomic:
// data goes to a sibling .partial file, then is renamed over dstPath.
//
// It returns the number of members removed and their combined size in the
// original archive (compressed), i.e. roughly how much smaller dstPath is.
func SplitTimelapse(srcPath, dstPath string) (removed int, removedBytes int64, err error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read %s: %s", srcPath, osErr(err))
	}
	defer src.Close()
	fi, err := src.Stat()
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read %s: %s", srcPath, osErr(err))
	}
	zr, err := zip.NewReader(src, fi.Size())
	if err != nil {
		return 0, 0, fmt.Errorf("%s is not a valid ZIP archive", srcPath)
	}
	dir := filepath.Dir(dstPath)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return 0, 0, fmt.Errorf("output directory does not exist: %s", dir)
	}
	partial := filepath.Join(dir, fmt.Sprintf(".procreepy-%d.slim.partial", os.Getpid()))
	if removed, removedBytes, err = rewriteZip(partial, zr); err != nil {
		os.Remove(partial)
		return 0, 0, err
	}
	if err := os.Rename(partial, dstPath); err != nil {
		os.Remove(partial)
		return 0, 0, fmt.Errorf("cannot write %s: %s", dstPath, osErr(err))
	}
	return removed, removedBytes, nil
}

// rewriteZip copies every non-video member of zr to a fresh archive at dst.
// Entries are streamed: a writer is finalized by the next CreateHeader or
// by closing the archive writer, so there is no per-entry Close.
func rewriteZip(dst string, zr *zip.Reader) (removed int, removedBytes int64, err error) {
	out, err := os.Create(dst)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot write %s: %s", dst, osErr(err))
	}
	wz := zip.NewWriter(out)
	fail := func(err error) (int, int64, error) {
		wz.Close()
		out.Close()
		return removed, removedBytes, err
	}
	for _, f := range zr.File {
		if isVideoMember(f.Name) {
			removed++
			removedBytes += int64(f.CompressedSize)
			continue
		}
		zw, err := wz.CreateHeader(&f.FileHeader)
		if err != nil {
			return fail(fmt.Errorf("%s: %v", f.Name, err))
		}
		rc, err := f.Open()
		if err != nil {
			return fail(fmt.Errorf("%s: %v", f.Name, err))
		}
		_, err = io.Copy(zw, rc)
		if cerr := rc.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fail(fmt.Errorf("%s: %v", f.Name, err))
		}
	}
	if err := wz.Close(); err != nil {
		return fail(err)
	}
	return removed, removedBytes, out.Close()
}

// isVideoMember reports whether an archive entry name lives under video/.
func isVideoMember(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), videoPrefix)
}
