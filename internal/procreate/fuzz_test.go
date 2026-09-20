package procreate

import (
	"strings"
	"testing"
)

// FuzzScan drives the segment scanner with arbitrary member-name lists
// (newline-separated; a trailing slash marks a directory entry). The
// contract: never panic, never invent a name, never report an out-of-order
// or duplicated number.
func FuzzScan(f *testing.F) {
	for _, s := range []string{
		"video/segments/segment-1.mp4\nvideo/segments/segment-2.mp4",
		"VIDEO/SEGMENTS/segment-00001.mp4",
		"video/segments/\nvideo/segments/segment-.mp4\nvideo/segments/extra.bin",
		"appinfo",
		"video/segments/segment-1.mp4\nvideo/segments/segment-1.mp4",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		lines := strings.Split(string(data), "\n")
		members := make([]Member, 0, len(lines))
		seen := make(map[string]bool, len(lines))
		for _, ln := range lines {
			isDir := strings.HasSuffix(ln, "/")
			name := strings.TrimSuffix(ln, "/")
			members = append(members, Member{Name: name, IsDir: isDir})
			seen[name] = true
		}
		segs, err := FindSegments(members, Options{})
		if err != nil {
			return
		}
		for i, s := range segs {
			if i > 0 && s.Number <= segs[i-1].Number {
				t.Fatalf("numbers out of order: %d after %d", s.Number, segs[i-1].Number)
			}
			if !seen[s.Name] {
				t.Fatalf("segment %q is not a member of the archive", s.Name)
			}
		}
	})
}
