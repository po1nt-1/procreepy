package batch

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
)

// ProjectArchiveName is the zip that stands in for the project directory.
//
// Getting a folder of projects back onto an iPad usually means one transfer, not
// one per file, so the default is to hand over a single archive. It carries a
// ProjectDir root inside, so unpacking on the tablet yields a folder rather
// than loose files in whatever directory the user happened to be in.
const ProjectArchiveName = ProjectDir + ".zip"

// projectZipPaths returns the project paths an existing archive already holds,
// keyed the way Targets.Project spells them.
//
// Packing deletes the directory, so without this a second run would see every
// project missing and rebuild all of them. A missing or unreadable archive
// yields no paths, which simply means nothing is known to be done yet.
func projectZipPaths(outDir string) map[string]bool {
	zr, err := zip.OpenReader(filepath.Join(outDir, ProjectArchiveName))
	if err != nil {
		return nil
	}
	defer zr.Close()
	out := make(map[string]bool, len(zr.File))
	for _, f := range zr.File {
		rel, ok := strippedEntry(f.Name)
		if !ok {
			continue
		}
		out[filepath.Join(outDir, ProjectDir, filepath.FromSlash(rel))] = true
	}
	return out
}

// strippedEntry removes the archive's root folder from an entry name, reporting
// whether the name belonged to it at all.
func strippedEntry(name string) (string, bool) {
	const prefix = ProjectDir + "/"
	if len(name) <= len(prefix) || name[:len(prefix)] != prefix {
		return "", false
	}
	rel := name[len(prefix):]
	if rel == "" || rel[len(rel)-1] == '/' {
		return "", false
	}
	return rel, true
}

// packProjects replaces the project directory with ProjectArchiveName and
// returns how many projects the archive holds.
//
// Entries from an archive left by an earlier run are carried over, so a resumed
// run adds to the set instead of narrowing it to whatever this run happened to
// produce. Files on disk win over same-named entries, which is what makes
// --force and the regeneration of an incomplete set take effect.
//
// Members are stored, not deflated: a .procreate file is itself a compressed
// zip, so a second pass costs time and saves nothing. Each entry keeps its
// file's modification time, which is the source artwork's, so the dates survive
// the trip through the archive.
func packProjects(ctx context.Context, outDir string) (int, error) {
	dir := filepath.Join(outDir, ProjectDir)

	rels, err := projectFiles(dir)
	if err != nil {
		return 0, err
	}
	if len(rels) == 0 {
		// Nothing new: every project was already in the archive. Drop the empty
		// skeleton so it does not sit next to the archive looking meaningful, and
		// leave the archive alone.
		os.Remove(dir)
		return len(projectZipPaths(outDir)), nil
	}

	fresh := make(map[string]bool, len(rels))
	for _, rel := range rels {
		fresh[path.Join(ProjectDir, filepath.ToSlash(rel))] = true
	}

	final := filepath.Join(outDir, ProjectArchiveName)
	tmp, err := os.CreateTemp(outDir, ".procreepy-*.partial")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	zw := zip.NewWriter(tmp)
	count, err := copyExistingEntries(ctx, final, fresh, zw)
	if err != nil {
		return 0, err
	}
	for _, rel := range rels {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if err := addProject(zw, dir, rel); err != nil {
			return 0, err
		}
		count++
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	// Publish before removing the directory: until the archive is in place, the
	// directory is the only copy of the work.
	if err := os.Rename(tmpName, final); err != nil {
		return 0, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}
	return count, nil
}

// projectFiles lists the project files under dir, relative to it. A directory
// that does not exist yields none rather than an error: a run in which every
// input was skipped produces no project tree at all.
func projectFiles(dir string) ([]string, error) {
	var rels []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		rels = append(rels, rel)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rels, nil
}

// copyExistingEntries transplants the entries of an earlier archive that this
// run did not reproduce, byte for byte and without recompressing them.
func copyExistingEntries(ctx context.Context, archive string, fresh map[string]bool,
	zw *zip.Writer) (int, error) {

	zr, err := zip.OpenReader(archive)
	if err != nil {
		return 0, nil // no previous archive, or not readable: start clean
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if fresh[f.Name] || f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.OpenRaw()
		if err != nil {
			return 0, err
		}
		hdr := f.FileHeader
		w, err := zw.CreateRaw(&hdr)
		if err != nil {
			return 0, err
		}
		if _, err := io.Copy(w, rc); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}

// addProject stores one project file under the archive's root folder.
func addProject(zw *zip.Writer, dir, rel string) error {
	full := filepath.Join(dir, rel)
	st, err := os.Stat(full)
	if err != nil {
		return err
	}
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     path.Join(ProjectDir, filepath.ToSlash(rel)),
		Method:   zip.Store,
		Modified: st.ModTime(),
	})
	if err != nil {
		return err
	}
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
