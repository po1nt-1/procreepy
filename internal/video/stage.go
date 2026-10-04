package video

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// staged is one output file being written next to its final location. Nothing
// reaches the final path until publish, so an interrupted or failed run leaves
// the previous contents (or nothing) rather than a truncated artifact.
//
// The temporary name comes from os.CreateTemp instead of a pid-derived one: two
// procreepy processes pointed at the same output directory would otherwise pick
// the same scratch file and corrupt each other's work.
type staged struct {
	f     *countedFile
	tmp   string
	final string
	// mtime, when non-zero, is stamped on the file before it is published.
	mtime time.Time
	done  bool
}

// stage creates the destination directory and opens a scratch file beside the
// final path.
func stage(final string) (*staged, error) {
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, &WriteError{Msg: "cannot create " + dir + ": " + strerror(err)}
	}
	file, err := os.CreateTemp(dir, ".procreepy-*.partial")
	if err != nil {
		return nil, &WriteError{Msg: "cannot write " + final + ": " + strerror(err)}
	}
	// CreateTemp opens at 0600; published outputs are ordinary files, so widen
	// them to the usual default before anyone can see them under the final name.
	// A failure here is not fatal: the bytes matter more than the mode, and on
	// Windows the call is a no-op anyway.
	_ = file.Chmod(0o644)
	return &staged{f: &countedFile{file: file}, tmp: file.Name(), final: final}, nil
}

// countedFile wraps the scratch file and tracks the highest byte position
// reached, so publish can verify that the file on disk holds exactly what the
// writer produced before the rename (plan: check size and basic invariants
// first). Writers that seek backwards to patch a header (the PSD writer does)
// therefore count as the size they left, not the sum of their writes.
type countedFile struct {
	file *os.File
	off  int64 // current file position
	peak int64 // furthest position written
}

func (c *countedFile) Write(p []byte) (int, error) {
	k, err := c.file.Write(p)
	c.off += int64(k)
	if c.off > c.peak {
		c.peak = c.off
	}
	return k, err
}

// Seek passes through so consumers needing io.WriteSeeker (the PSD writer
// patches header offsets) keep working.
func (c *countedFile) Seek(offset int64, whence int) (int64, error) {
	off, err := c.file.Seek(offset, whence)
	if err == nil {
		c.off = off
	}
	return off, err
}

func (c *countedFile) Close() error { return c.file.Close() }

// high reports the furthest position written: the size the finished file
// must have.
func (c *countedFile) high() int64 { return c.peak }

// setModTime asks publish to carry a source timestamp onto the output.
func (s *staged) setModTime(t time.Time) { s.mtime = t }

// publish closes the scratch file and moves it onto the final path. The
// timestamp is stamped before the move so the published file never briefly
// carries the wrong one.
func (s *staged) publish() error {
	if s.done {
		return nil
	}
	if err := s.f.Close(); err != nil {
		s.remove()
		return &WriteError{Msg: "cannot write " + s.final + ": " + strerror(err)}
	}
	// Before the timestamp and the rename: what the writer did not report
	// must never be published under the final name.
	if err := verifyWritten(s.tmp, s.f.high()); err != nil {
		s.remove()
		return &WriteError{Msg: "cannot write " + s.final + ": " + err.Error()}
	}
	if !s.mtime.IsZero() {
		if err := os.Chtimes(s.tmp, s.mtime, s.mtime); err != nil {
			s.remove()
			return &WriteError{Msg: "cannot set the timestamp of " + s.final + ": " + strerror(err)}
		}
	}
	// os.Rename replaces an existing target on both POSIX and Windows (there it
	// is MoveFileEx with MOVEFILE_REPLACE_EXISTING), so no per-OS shim is needed.
	if err := os.Rename(s.tmp, s.final); err != nil {
		s.remove()
		return &WriteError{Msg: "cannot write " + s.final + ": " + strerror(err)}
	}
	s.done = true
	return nil
}

// verifyWritten reports whether path holds exactly want bytes on disk. A
// short or padded write that never surfaced as an error must not be
// published; stat is the ground truth at the moment of the rename.
func verifyWritten(path string, want int64) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Size() != want {
		return fmt.Errorf("size mismatch: %d bytes reported, %d on disk", want, st.Size())
	}
	return nil
}

// discard throws the scratch file away. It is safe to call after publish and
// safe to call twice, so it works as a plain defer.
func (s *staged) discard() {
	if s.done {
		return
	}
	s.f.Close()
	s.remove()
}

func (s *staged) remove() { os.Remove(s.tmp) }

// discardAll cleans up a whole item's staged outputs.
func discardAll(all []*staged) {
	for _, s := range all {
		s.discard()
	}
}
