package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Output tree layout. Directory mode keeps the artifacts of one run in separate
// trees by purpose: timelapses are what most runs are for, projects are what
// goes back into Procreate, and PSDs are an optional heavier export.
const (
	// DefaultOutputDir is the OUTPUT root used when none is given.
	DefaultOutputDir = "output"
	// TimelapseDir holds the joined MP4s.
	TimelapseDir = "timelapses"
	// ProjectDir holds the re-importable .procreepy.procreate archives.
	ProjectDir = "projects"
	// PSDDir holds the layered PSD exports.
	PSDDir = "psd"

	// ProjectSuffix is appended to the source stem for the slimmed archive.
	ProjectSuffix = ".procreepy.procreate"

	procreateExt = ".procreate"
)

// Targets are the output paths one input produces. PSD is empty when PSD
// export is off.
type Targets struct {
	Timelapse string
	Project   string
	PSD       string
}

// all returns the non-empty targets, for collision and existence checks.
func (t Targets) all() []string {
	out := []string{t.Timelapse, t.Project}
	if t.PSD != "" {
		out = append(out, t.PSD)
	}
	return out
}

// Item pairs one input archive with everything it produces.
type Item struct {
	Src     string
	Targets Targets
}

// Roots are the per-purpose output directories under one OUTPUT root.
type Roots struct {
	Base       string
	Timelapses string
	Projects   string
	PSD        string // empty when PSD export is off
}

// NewRoots derives the output tree from an OUTPUT root.
func NewRoots(base string, psd bool) Roots {
	r := Roots{
		Base:       base,
		Timelapses: filepath.Join(base, TimelapseDir),
		Projects:   filepath.Join(base, ProjectDir),
	}
	if psd {
		r.PSD = filepath.Join(base, PSDDir)
	}
	return r
}

// stem strips one trailing .procreate from a file name, whatever its case, so
// Cat.procreate, Cat.PROCREATE and Artwork.final.procreate yield Cat, Cat and
// Artwork.final. Anything else keeps its whole name, which is why the extension
// is not simply cut at the last dot.
func stem(name string) string {
	b := filepath.Base(name)
	if len(b) > len(procreateExt) && strings.EqualFold(b[len(b)-len(procreateExt):], procreateExt) {
		return b[:len(b)-len(procreateExt)]
	}
	return b
}

// Plan maps every discovered input to its outputs.
//
// One shared stem drives all of an input's outputs, so the MP4, the project and
// the PSD of a given artwork always agree on their name, including the
// collision suffix. Collisions are detected case-insensitively because the
// primary platform is Windows, where Cat.procreate and CAT.PROCREATE would
// otherwise silently overwrite one another's output.
func Plan(files []string, inRoot string, roots Roots, recursive bool) []Item {
	used := make(map[string]bool, len(files))
	items := make([]Item, 0, len(files))
	for _, src := range files {
		relDir := ""
		if recursive {
			if rel, err := filepath.Rel(inRoot, src); err == nil {
				if d := filepath.Dir(rel); d != "." {
					relDir = d
				}
			}
		}
		base := stem(src)
		name := base
		for n := 2; used[collisionKey(relDir, name)]; n++ {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		used[collisionKey(relDir, name)] = true

		t := Targets{
			Timelapse: filepath.Join(roots.Timelapses, relDir, name+".mp4"),
			Project:   filepath.Join(roots.Projects, relDir, name+ProjectSuffix),
		}
		if roots.PSD != "" {
			t.PSD = filepath.Join(roots.PSD, relDir, name+".psd")
		}
		items = append(items, Item{Src: src, Targets: t})
	}
	return items
}

// collisionKey folds case so one numbering serves case-sensitive and
// case-insensitive filesystems alike; the result is therefore identical on
// Linux and Windows.
func collisionKey(relDir, name string) string {
	return strings.ToLower(filepath.Join(relDir, name))
}

// checkOutputNesting rejects an output root inside the input tree during a
// recursive run: the outputs are .procreate files themselves, so the next run
// (or the same walk) would pick them up as inputs and slim them again.
func checkOutputNesting(inDir, outDir string, recursive bool) error {
	in, err := filepath.Abs(inDir)
	if err != nil {
		return nil
	}
	out, err := filepath.Abs(outDir)
	if err != nil {
		return nil
	}
	if equalPath(in, out) {
		return fmt.Errorf("OUTPUT must not be the input directory itself: %s", outDir)
	}
	if recursive && withinPath(in, out) {
		return fmt.Errorf("OUTPUT %s is inside the input tree %s; with -r the generated "+
			"projects would be picked up as inputs on the next run", outDir, inDir)
	}
	return nil
}

// withinPath reports whether child sits below parent, comparing whole path
// elements so /in and /input are not treated as nested.
func withinPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func equalPath(a, b string) bool {
	if a == b {
		return true
	}
	// Windows and macOS default to case-insensitive paths; folding here keeps
	// the guard effective there without weakening it on Linux, where a genuine
	// case difference means a genuinely different directory that the nesting
	// check below still covers.
	return caseInsensitiveFS && strings.EqualFold(a, b)
}
