package procodec_test

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"procreepy/internal/procodec"
)

// tileRE matches a layer tile member: <layer UUID>/<col>~<row>.<container>.
var tileRE = regexp.MustCompile(`^([0-9A-Fa-f-]{36})/(\d+)~(\d+)\.(chunk|lz4|lz4c)$`)

// TestFixtureTiles decodes every layer tile of every project in a real
// Procreate corpus. It is skipped unless PROCREATE_FIXTURE_DIR points at one,
// because no binary fixtures are committed.
func TestFixtureTiles(t *testing.T) {
	dir := os.Getenv("PROCREATE_FIXTURE_DIR")
	if dir == "" {
		t.Skip("set PROCREATE_FIXTURE_DIR to a directory of .procreate files")
	}
	projects := findProjects(t, dir)
	if len(projects) == 0 {
		t.Fatalf("no .procreate files under %s", dir)
	}

	const maxTile = 256 * 256 * 4
	var total, ok int
	byExt := map[string][2]int{}
	geometry := map[int]int{}

	for _, p := range projects {
		zr, err := zip.OpenReader(p)
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		for _, f := range zr.File {
			m := tileRE.FindStringSubmatch(f.Name)
			if m == nil {
				continue
			}
			ext := m[4]
			total++
			c := byExt[ext]
			c[0]++
			raw, err := readMember(f)
			if err != nil {
				t.Errorf("%s: %s: %v", filepath.Base(p), f.Name, err)
				byExt[ext] = c
				continue
			}
			out, err := procodec.DecodeTile(raw, maxTile)
			if err != nil {
				t.Errorf("%s: %s: %v", filepath.Base(p), f.Name, err)
				byExt[ext] = c
				continue
			}
			if len(out)%4 != 0 {
				t.Errorf("%s: %s: decoded %d bytes, not a whole number of RGBA pixels",
					filepath.Base(p), f.Name, len(out))
			}
			ok++
			c[1]++
			byExt[ext] = c
			geometry[len(out)]++
		}
		zr.Close()
	}
	var exts []string
	for e := range byExt {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	t.Logf("projects=%d tiles=%d decoded=%d", len(projects), total, ok)
	for _, e := range exts {
		t.Logf("  .%-5s total=%-6d decoded=%d", e, byExt[e][0], byExt[e][1])
	}
	var sizes []int
	for s := range geometry {
		sizes = append(sizes, s)
	}
	sort.Ints(sizes)
	for _, s := range sizes {
		t.Logf("  %d bytes (%d px) x%d", s, s/4, geometry[s])
	}
	if ok != total {
		t.Errorf("decoded %d of %d tiles", ok, total)
	}
}

func readMember(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func findProjects(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "__MACOSX") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".procreate") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}
