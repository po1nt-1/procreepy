// Command cov2cobertura merges Go coverage profiles (text format) and
// renders their union as a Cobertura XML report for GitLab MR diff
// annotations. The project keeps zero third-party dependencies, so this
// replaces the network-bound gocover-cobertura tool.
//
// Usage:
//
//	cov2cobertura [-o coverage.xml] [-merged merged.out] [-strip procreepy/] profile.out [...]
//
// -o        output Cobertura XML (default coverage.xml)
// -merged   also write the merged text profile (for go tool cover -func)
// -strip    path prefix removed from file names so they become
//
//	repository-relative (default: none)
package main

import (
	"bufio"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var posRE = regexp.MustCompile(`^(.+):(\d+)\.\d+,(\d+)\.\d+$`)

type block struct {
	pos    string // "file:startLine.col,endLine.col"
	file   string
	startL int
	endL   int // last covered line, inclusive
	stmts  int
	count  int
}

func parseProfile(name string) ([]block, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []block
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("%s:%d: malformed profile line %q", name, lineNo, line)
		}
		m := posRE.FindStringSubmatch(fields[0])
		if m == nil {
			return nil, fmt.Errorf("%s:%d: bad position %q", name, lineNo, fields[0])
		}
		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%s:%d: bad numbers", name, lineNo)
		}
		startL, _ := strconv.Atoi(m[2])
		endL, _ := strconv.Atoi(m[3])
		if endL <= startL {
			endL = startL
		}
		out = append(out, block{pos: fields[0], file: m[1], startL: startL, endL: endL, stmts: stmts, count: count})
	}
	return out, sc.Err()
}

// merge unions the blocks: for each position the highest count wins, which
// is the correct OR of "executed at least once" across suites.
func merge(all []block) []block {
	best := map[string]block{}
	for _, b := range all {
		old, ok := best[b.pos]
		if !ok || b.count > old.count || (b.count == old.count && b.stmts > old.stmts) {
			best[b.pos] = b
		}
	}
	out := make([]block, 0, len(best))
	for _, b := range best {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	return out
}

func writeMerged(path string, bs []block) error {
	var sb strings.Builder
	sb.WriteString("mode: set\n")
	for _, b := range bs {
		fmt.Fprintf(&sb, "%s %d %d\n", b.pos, b.stmts, b.count)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

type rates struct{ valid, covered int }

func (r rates) ratio() float64 {
	if r.valid == 0 {
		return 0
	}
	return float64(r.covered) / float64(r.valid)
}

func fmtRate(r float64) string { return strconv.FormatFloat(r, 'f', 4, 64) }

func esc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func writeCobertura(path, strip, source string, bs []block) (files, valid, covered int, err error) {
	// file -> line -> max hits
	lines := map[string]map[int]int{}
	for _, b := range bs {
		f := strings.TrimPrefix(b.file, strip)
		lm := lines[f]
		if lm == nil {
			lm = map[int]int{}
			lines[f] = lm
		}
		for l := b.startL; l <= b.endL; l++ {
			if old, ok := lm[l]; !ok || b.count > old {
				lm[l] = b.count
			}
		}
	}
	fileList := make([]string, 0, len(lines))
	for f := range lines {
		fileList = append(fileList, f)
	}
	sort.Strings(fileList)

	var total rates
	classXML := map[string]*strings.Builder{} // package -> classes
	pkgOrder := []string{}
	for _, f := range fileList {
		dir := ""
		if i := strings.LastIndex(f, "/"); i >= 0 {
			dir = f[:i]
		}
		pkg := strings.NewReplacer("/", "_", "\\", "_").Replace(dir)
		if pkg == "" {
			pkg = "."
		}
		lm := lines[f]
		var cr rates
		var lb strings.Builder
		nums := make([]int, 0, len(lm))
		for l := range lm {
			nums = append(nums, l)
		}
		sort.Ints(nums)
		cr.valid = len(nums)
		for _, l := range nums {
			hits := lm[l]
			if hits > 0 {
				cr.covered++
			}
			fmt.Fprintf(&lb, "            <line number=\"%d\" hits=\"%d\" branch=\"false\"/>\n", l, hits)
		}
		total.valid += cr.valid
		total.covered += cr.covered
		name := f
		if i := strings.LastIndex(f, "/"); i >= 0 {
			name = f[i+1:]
		}
		if classXML[pkg] == nil {
			classXML[pkg] = &strings.Builder{}
			pkgOrder = append(pkgOrder, pkg)
		}
		fmt.Fprintf(classXML[pkg], "        <class name=%q filename=%q line-rate=%q line-covered=\"%d\" lines-valid=\"%d\" branch-rate=\"0.0\" branch-covered=\"0\" branches-valid=\"0\">\n          <lines>\n%s          </lines>\n        </class>\n",
			esc(name), esc(f), fmtRate(cr.ratio()), cr.covered, cr.valid, lb.String())
	}
	sort.Strings(pkgOrder)

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" ?>` + "\n")
	sb.WriteString("<!DOCTYPE coverage SYSTEM \"http://cobertura.sourceforge.net/xml/coverage-04.dtd\">\n")
	fmt.Fprintf(&sb, "<coverage line-rate=\"%s\" lines-covered=\"%d\" lines-valid=\"%d\" branch-rate=\"0.0\" branches-covered=\"0\" branches-valid=\"0\" complexity=\"0\" timestamp=\"0\">\n",
		fmtRate(total.ratio()), total.covered, total.valid)
	sb.WriteString("  <sources>\n")
	fmt.Fprintf(&sb, "    <source>%s</source>\n", esc(source))
	sb.WriteString("  </sources>\n  <packages>\n")
	for _, pkg := range pkgOrder {
		pkgFiles := []string{}
		for _, f := range fileList {
			dir := ""
			if i := strings.LastIndex(f, "/"); i >= 0 {
				dir = f[:i]
			}
			if strings.NewReplacer("/", "_", "\\", "_").Replace(dir) == pkg || (pkg == "." && dir == "") {
				pkgFiles = append(pkgFiles, f)
			}
		}
		var pr rates
		for _, f := range pkgFiles {
			pr.valid += len(lines[f])
			for _, h := range lines[f] {
				if h > 0 {
					pr.covered++
				}
			}
		}
		fmt.Fprintf(&sb, "    <package name=%q line-rate=%q line-covered=\"%d\" lines-valid=\"%d\" branch-rate=\"0.0\" branch-covered=\"0\" branches-valid=\"0\">\n      <classes>\n%s      </classes>\n    </package>\n",
			esc(pkg), fmtRate(pr.ratio()), pr.covered, pr.valid, classXML[pkg].String())
	}
	sb.WriteString("  </packages>\n</coverage>\n")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return 0, 0, 0, err
	}
	// Round-trip: the document must be well-formed XML.
	if err := checkWellFormed(sb.String()); err != nil {
		return 0, 0, 0, fmt.Errorf("produced invalid XML: %w", err)
	}
	return len(lines), total.valid, total.covered, nil
}

func checkWellFormed(doc string) error {
	d := xml.NewDecoder(strings.NewReader(doc))
	for {
		tok, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			return err
		}
		_ = tok
	}
}

func main() {
	out := flag.String("o", "coverage.xml", "output Cobertura XML path")
	mergedOut := flag.String("merged", "", "optionally write the merged text profile here")
	strip := flag.String("strip", "", "file-name prefix to remove (module root)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: cov2cobertura [-o coverage.xml] [-merged merged.out] [-strip procreepy/] profile.out ...")
		os.Exit(2)
	}
	var all []block
	for _, p := range flag.Args() {
		bs, err := parseProfile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cov2cobertura:", err)
			os.Exit(1)
		}
		all = append(all, bs...)
	}
	u := merge(all)
	if *mergedOut != "" {
		if err := writeMerged(*mergedOut, u); err != nil {
			fmt.Fprintln(os.Stderr, "cov2cobertura:", err)
			os.Exit(1)
		}
	}
	source, err := os.Getwd()
	if err != nil {
		source = "."
	}
	nfiles, valid, covered, err := writeCobertura(*out, *strip, source, u)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cov2cobertura:", err)
		os.Exit(1)
	}
	pct := 0.0
	if valid > 0 {
		pct = float64(covered) / float64(valid)
	}
	fmt.Printf("cov2cobertura: %d files, %d/%d lines covered (%.1f%%) -> %s\n",
		nfiles, covered, valid, pct*100, *out)
}
