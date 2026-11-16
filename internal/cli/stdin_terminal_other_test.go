//go:build !darwin && !linux && !windows

package cli

import "testing"

// TestOtherSystemsFailClosed: where there is no terminal query, stdin is never taken for a terminal, so no prompt appears and a clear is
// refused with the hint. It is compiled and vetted for these systems; it runs only there.
func TestOtherSystemsFailClosed(t *testing.T) {
	if fdIsTerminal(0) {
		t.Error("without a terminal query stdin must not count as a terminal")
	}
}
