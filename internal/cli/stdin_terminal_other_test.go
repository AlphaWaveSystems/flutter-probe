//go:build !darwin && !linux

package cli

import "testing"

// TestOtherSystemsFailClosed: where the terminal driver cannot be asked, stdin is never taken for a terminal, so no prompt appears and a
// clear is refused with the hint. Compiled and vetted for these systems; it runs only there.
func TestOtherSystemsFailClosed(t *testing.T) {
	if fdIsTerminal(0) {
		t.Error("without the terminal check stdin must not count as a terminal")
	}
}
