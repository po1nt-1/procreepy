package batch

import (
	"path/filepath"
	"strings"
	"testing"
)

// slash renders a path with forward slashes so expectations read the same on
// Windows.
func slash(p string) string { return filepath.ToSlash(p) }

func TestPlanTwoOutputTrees(t *testing.T) {
	roots := NewRoots("out", false)
	items := Plan([]string{
		filepath.Join("in", "Cat.procreate"),
		filepath.Join("in", "House.procreate"),
	}, "in", roots, false)

	if len(items) != 2 {
		t.Fatalf("planned %d items, want 2", len(items))
	}
	want := []struct{ timelapse, project string }{
		{"out/timelapses/Cat.mp4", "out/projects/Cat.procreepy.procreate"},
		{"out/timelapses/House.mp4", "out/projects/House.procreepy.procreate"},
	}
	for i, w := range want {
		if got := slash(items[i].Targets.Timelapse); got != w.timelapse {
			t.Errorf("item %d timelapse = %q, want %q", i, got, w.timelapse)
		}
		if got := slash(items[i].Targets.Project); got != w.project {
			t.Errorf("item %d project = %q, want %q", i, got, w.project)
		}
		if items[i].Targets.PSD != "" {
			t.Errorf("item %d has a PSD target without --psd: %q", i, items[i].Targets.PSD)
		}
	}
}

func TestPlanRecursiveMirrorsOneRelativePath(t *testing.T) {
	roots := NewRoots("out", true)
	items := Plan([]string{filepath.Join("in", "2025", "Cat.procreate")}, "in", roots, true)
	if len(items) != 1 {
		t.Fatalf("planned %d items", len(items))
	}
	tg := items[0].Targets
	if got, want := slash(tg.Timelapse), "out/timelapses/2025/Cat.mp4"; got != want {
		t.Errorf("timelapse = %q, want %q", got, want)
	}
	if got, want := slash(tg.Project), "out/projects/2025/Cat.procreepy.procreate"; got != want {
		t.Errorf("project = %q, want %q", got, want)
	}
	if got, want := slash(tg.PSD), "out/psd/2025/Cat.psd"; got != want {
		t.Errorf("psd = %q, want %q", got, want)
	}
}

// TestPlanFlatIgnoresSubdirectories: without -r the relative directory is not
// mirrored, so a file found deeper still lands at the tree root.
func TestPlanFlatIgnoresSubdirectories(t *testing.T) {
	roots := NewRoots("out", false)
	items := Plan([]string{filepath.Join("in", "Cat.procreate")}, "in", roots, false)
	if got, want := slash(items[0].Targets.Timelapse), "out/timelapses/Cat.mp4"; got != want {
		t.Errorf("timelapse = %q, want %q", got, want)
	}
}

// TestPlanProjectNaming pins the project file name rules, including the ones a
// naive "replace the last extension" would get wrong.
func TestPlanProjectNaming(t *testing.T) {
	cases := []struct{ in, mp4, project string }{
		{"Cat.procreate", "Cat.mp4", "Cat.procreepy.procreate"},
		{"Artwork.final.procreate", "Artwork.final.mp4", "Artwork.final.procreepy.procreate"},
		{"Cat.PROCREATE", "Cat.mp4", "Cat.procreepy.procreate"},
		{"Cat.ProCreate", "Cat.mp4", "Cat.procreepy.procreate"},
		{"no extension", "no extension.mp4", "no extension.procreepy.procreate"},
		{"пельмеши.procreate", "пельмеши.mp4", "пельмеши.procreepy.procreate"},
		{"a (copy).procreate", "a (copy).mp4", "a (copy).procreepy.procreate"},
	}
	roots := NewRoots("out", false)
	for _, c := range cases {
		items := Plan([]string{filepath.Join("in", c.in)}, "in", roots, false)
		if got, want := filepath.Base(items[0].Targets.Timelapse), c.mp4; got != want {
			t.Errorf("%q: mp4 = %q, want %q", c.in, got, want)
		}
		if got, want := filepath.Base(items[0].Targets.Project), c.project; got != want {
			t.Errorf("%q: project = %q, want %q", c.in, got, want)
		}
	}
}

// TestPlanCollisionSuffixIsSharedAndDeterministic: a collision must renumber
// every output of the input together, so the MP4 and the project keep matching
// names.
func TestPlanCollisionSuffixIsSharedAndDeterministic(t *testing.T) {
	roots := NewRoots("out", true)
	in := []string{
		filepath.Join("in", "a.procreate"),
		filepath.Join("in", "a.procreate"),
		filepath.Join("in", "a.procreate"),
	}
	items := Plan(in, "in", roots, false)
	wantStems := []string{"a", "a-2", "a-3"}
	for i, stem := range wantStems {
		tg := items[i].Targets
		if got, want := filepath.Base(tg.Timelapse), stem+".mp4"; got != want {
			t.Errorf("item %d mp4 = %q, want %q", i, got, want)
		}
		if got, want := filepath.Base(tg.Project), stem+".procreepy.procreate"; got != want {
			t.Errorf("item %d project = %q, want %q", i, got, want)
		}
		if got, want := filepath.Base(tg.PSD), stem+".psd"; got != want {
			t.Errorf("item %d psd = %q, want %q", i, got, want)
		}
	}
	// Determinism: the same input order must always give the same names.
	again := Plan(in, "in", roots, false)
	for i := range items {
		if items[i].Targets != again[i].Targets {
			t.Errorf("item %d is not deterministic: %v vs %v", i, items[i].Targets, again[i].Targets)
		}
	}
}

// TestPlanCaseInsensitiveCollision is the Windows case: a.procreate and
// A.PROCREATE map to the same output name there, so they must be renumbered
// even on a case-sensitive filesystem. Producing the same plan on both keeps
// behaviour portable.
func TestPlanCaseInsensitiveCollision(t *testing.T) {
	roots := NewRoots("out", false)
	items := Plan([]string{
		filepath.Join("in", "a.procreate"),
		filepath.Join("in", "A.PROCREATE"),
	}, "in", roots, false)
	first := filepath.Base(items[0].Targets.Timelapse)
	second := filepath.Base(items[1].Targets.Timelapse)
	if first != "a.mp4" {
		t.Errorf("first = %q, want a.mp4", first)
	}
	if second != "A-2.mp4" {
		t.Errorf("second = %q, want A-2.mp4 (case-insensitive collision)", second)
	}
	if strings.EqualFold(first, second) {
		t.Errorf("%q and %q still collide when case is folded", first, second)
	}
}

// TestPlanSameNameDifferentDirectories: names only collide within one output
// directory, so mirrored sub-directories keep their natural names.
func TestPlanSameNameDifferentDirectories(t *testing.T) {
	roots := NewRoots("out", false)
	items := Plan([]string{
		filepath.Join("in", "2024", "Cat.procreate"),
		filepath.Join("in", "2025", "Cat.procreate"),
	}, "in", roots, true)
	if got, want := slash(items[0].Targets.Timelapse), "out/timelapses/2024/Cat.mp4"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := slash(items[1].Targets.Timelapse), "out/timelapses/2025/Cat.mp4"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckOutputNesting(t *testing.T) {
	cases := []struct {
		name            string
		in, out         string
		recursive, want bool
	}{
		{"separate trees", "/data/in", "/data/out", true, false},
		{"output inside input, recursive", "/data/in", "/data/in/out", true, true},
		{"output inside input, flat", "/data/in", "/data/in/out", false, false},
		{"output is the input", "/data/in", "/data/in", false, true},
		{"output is the input, recursive", "/data/in", "/data/in", true, true},
		{"sibling with a shared prefix", "/data/in", "/data/input", true, false},
		{"input inside output", "/data/out/in", "/data/out", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkOutputNesting(filepath.FromSlash(c.in), filepath.FromSlash(c.out), c.recursive)
			if (err != nil) != c.want {
				t.Fatalf("err = %v, want error: %v", err, c.want)
			}
		})
	}
}

func TestStem(t *testing.T) {
	cases := map[string]string{
		"Cat.procreate":           "Cat",
		"Cat.PROCREATE":           "Cat",
		"Artwork.final.procreate": "Artwork.final",
		"plain":                   "plain",
		"x.mp4":                   "x.mp4",
		".procreate":              ".procreate", // a dot-file, not an empty stem
	}
	for in, want := range cases {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q, want %q", in, got, want)
		}
	}
}
