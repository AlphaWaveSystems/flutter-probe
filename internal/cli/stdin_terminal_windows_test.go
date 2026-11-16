//go:build windows

package cli

import (
	"os"
	"testing"
)

// TestWindowsNulIsNotAConsole: NUL is the Windows null device. GetConsoleMode refuses it, so it gets no prompt.
func TestWindowsNulIsNotAConsole(t *testing.T) {
	nul, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer nul.Close()
	if fdIsTerminal(nul.Fd()) {
		t.Error("NUL must not count as a console")
	}
}

// TestWindowsInvalidHandlesAreNotConsoles: the NULL handle and an invalid handle are not consoles, and converting the handle does not panic.
func TestWindowsInvalidHandlesAreNotConsoles(t *testing.T) {
	for _, fd := range []uintptr{0, ^uintptr(0)} {
		if fdIsTerminal(fd) {
			t.Errorf("handle %#x must not count as a console", fd)
		}
	}
}
