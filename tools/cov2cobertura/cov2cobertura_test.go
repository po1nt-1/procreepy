package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const profileA = `mode: set
demo/m/lib/p.go:10.2,12.1 2 1
demo/m/lib/p.go:14.2,15.1 1 0
demo/m/lib/q.go:5.2,6.1 1 0
`

const profileB = `mode: set
demo/m/lib/q.go:5.2,6.1 1 1
demo/m/other/r.go:1.2,3.1 2 1
`

func TestMergeAndCobertura(t *testing.T) {
	dir := t.TempDir()
	pa, pb := filepath.Join(dir, "a.out"), filepath.Join(dir, "b.out")
	if err := os.WriteFile(pa, []byte(profileA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pb, []byte(profileB), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := parseProfile(pa)
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseProfile(pb)
	if err != nil {
		t.Fatal(err)
	}
	u := merge(append(a, b...))
	if len(u) != 4 {
		t.Fatalf("merged blocks = %d, want 4", len(u))
	}
	byPos := map[string]block{}
	for _, blk := range u {
		byPos[blk.pos] = blk
	}
	if got := byPos["demo/m/lib/q.go:5.2,6.1"].count; got != 1 {
		t.Errorf("union must take max count: q.go = %d, want 1", got)
	}
	if got := byPos["demo/m/lib/p.go:14.2,15.1"].count; got != 0 {
		t.Errorf("uncovered block must stay 0, got %d", got)
	}

	merged := filepath.Join(dir, "merged.out")
	if err := writeMerged(merged, u); err != nil {
		t.Fatal(err)
	}
	again, err := parseProfile(merged)
	if err != nil {
		t.Fatalf("merged profile must re-parse: %v", err)
	}
	if len(again) != 4 {
		t.Fatalf("merged blocks = %d, want 4", len(again))
	}

	xmlPath := filepath.Join(dir, "coverage.xml")
	nfiles, valid, covered, err := writeCobertura(xmlPath, "demo/m/", ".", u)
	if err != nil {
		t.Fatal(err)
	}
	if nfiles != 3 {
		t.Errorf("files = %d, want 3", nfiles)
	}
	// p.go: 10,11,12 covered; 14,15 not; q.go: 5,6 covered; r.go: 1,2,3 covered
	if valid != 10 || covered != 8 {
		t.Errorf("lines = %d/%d, want 8/10", covered, valid)
	}
	raw, err := os.ReadFile(xmlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `filename="lib/p.go"`) {
		t.Error("module prefix not stripped from filename")
	}
	if strings.Contains(string(raw), "demo/m/") {
		t.Error("module prefix leaked into the report")
	}
	var doc struct {
		XMLName xml.Name `xml:"coverage"`
		Sources struct {
			Source []string `xml:"source"`
		} `xml:"sources"`
		Packages []struct {
			XMLName xml.Name `xml:"package"`
			Name    string   `xml:"name,attr"`
			Classes []struct {
				XMLName  xml.Name `xml:"class"`
				Filename string   `xml:"filename,attr"`
				Lines    []struct {
					Number int `xml:"number,attr"`
					Hits   int `xml:"hits,attr"`
				} `xml:"lines>line"`
			} `xml:"classes>class"`
		} `xml:"packages>package"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(doc.Packages) != 2 { // lib, other
		t.Fatalf("packages = %d, want 2", len(doc.Packages))
	}
	if doc.Sources.Source == nil || doc.Sources.Source[0] != "." {
		t.Errorf("sources = %v, want [.] ", doc.Sources.Source)
	}
}
