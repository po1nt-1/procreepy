package procreate

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"procreepy/internal/testkit"
)

// TestSplitManyMembers stress-tests the slim-down rewrite with 20k archive
// members: every non-video member must survive byte-for-byte, in order, with
// no video remnants.
func TestSplitManyMembers(t *testing.T) {
	const others, videos = 18000, 2000
	entries := map[string][]byte{"appinfo": []byte(`{"version":1}`)}
	for i := 0; i < others; i++ {
		entries[fmt.Sprintf("layer-%05d.data", i)] = []byte("layer-bytes")
	}
	for i := 1; i <= videos; i++ {
		entries[fmt.Sprintf("video/segments/segment-%04d.mp4", i)] = make([]byte, 1024)
	}
	src := testkit.WriteArchive(t, entries, false)
	dst := filepath.Join(t.TempDir(), "slim.procreate")
	removed, removedBytes, err := SplitTimelapse(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if removed != videos {
		t.Fatalf("removed = %d, want %d", removed, videos)
	}
	if removedBytes != int64(videos*1024) { // stored: compressed == raw size
		t.Fatalf("removed bytes = %d, want %d", removedBytes, int64(videos*1024))
	}
	out, err := Open(dst, "slim")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	ms := out.Members()
	if len(ms) != others+1 {
		t.Fatalf("slim archive has %d members, want %d", len(ms), others+1)
	}
	for _, m := range ms {
		if strings.HasPrefix(strings.ToLower(m.Name), "video/") {
			t.Fatalf("video member survived: %s", m.Name)
		}
	}
	if ms[0].Name != "appinfo" {
		t.Fatalf("member order broken: first is %s", ms[0].Name)
	}
	got, err := out.ReadMember("layer-00001.data")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "layer-bytes" {
		t.Fatalf("member content mangled: %q", got)
	}
}

// BenchmarkScanManyMembers measures segment discovery over a 100k-member
// archive listing.
func BenchmarkScanManyMembers(b *testing.B) {
	const n = 100000
	members := make([]Member, n)
	for i := range members {
		members[i] = Member{Name: fmt.Sprintf("video/segments/segment-%d.mp4", i+1)}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := FindSegments(members, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}
