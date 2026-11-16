package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// withStdin runs body with os.Stdin replaced by f: a test cannot change what the process was started with.
func withStdin(t *testing.T, f *os.File, body func()) {
	t.Helper()
	prev := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = prev })
	body()
}

// TestStdinOnDevNullIsNotATerminal: /dev/null is a character device that answers every question with end of file. It is not a terminal,
// so no prompt is shown, and a clear is refused with the hint to pass --yes rather than read as "cancelled by user".
func TestStdinOnDevNullIsNotATerminal(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	withStdin(t, null, func() {
		if stdinIsTerminal() {
			t.Error("/dev/null must not count as a terminal")
		}
		if confirmFunc() != nil {
			t.Error("with stdin on /dev/null there must be no prompt")
		}
		gate := confirmWithPrefs(confirmFunc(), false, func() bool { return false }, io.Discard, "com.example.app", "emulator-5554")
		dc := &runner.DeviceContext{AppID: "com.example.app", Serial: "emulator-5554", Confirm: gate}
		if err := dc.AskClearData(false); err == nil || !strings.Contains(err.Error(), "pass --yes (-y)") {
			t.Errorf("a clear with stdin on /dev/null must be refused with the hint, got %v", err)
		}
	})
}

// TestStdinPipeIsNotATerminal: a pipe (CI, a script) is not a terminal either, so it gets no prompt.
func TestStdinPipeIsNotATerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	withStdin(t, r, func() {
		if stdinIsTerminal() {
			t.Error("a pipe must not count as a terminal")
		}
		if confirmFunc() != nil {
			t.Error("with stdin on a pipe there must be no prompt")
		}
	})
}

// TestStdinOnZeroIsNotATerminal: /dev/zero is a character device that is neither the null device nor a terminal, so the terminal check
// itself must refuse it: the ioctl that asks the terminal driver is not optional.
func TestStdinOnZeroIsNotATerminal(t *testing.T) {
	zero, err := os.Open("/dev/zero")
	if err != nil {
		t.Skipf("no /dev/zero on this system: %v", err)
	}
	defer zero.Close()
	withStdin(t, zero, func() {
		if stdinIsTerminal() {
			t.Error("/dev/zero must not count as a terminal")
		}
	})
}
