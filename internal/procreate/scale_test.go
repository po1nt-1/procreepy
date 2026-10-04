package procreate

import (
	"fmt"
	"testing"
)

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
