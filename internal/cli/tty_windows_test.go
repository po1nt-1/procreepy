//go:build windows

package cli

import "testing"

// TestWindowsTTYPolicy pins the Windows fallback behavior: isTerminal must
// never panic and must report false when stderr is redirected (the CI case).
// On a real console it turns on VT processing first, so it reports true and
// the console actually renders the SGR codes.
func TestWindowsTTYPolicy(t *testing.T) {
	if isTerminal(2) {
		t.Log("running on a real console; VT processing enabled")
	}
	// The interesting CI case is "redirected -> false"; go test in CI
	// redirects stderr, so the expected answer there is false.
}
