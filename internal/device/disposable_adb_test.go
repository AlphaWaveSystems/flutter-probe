package device

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeADB writes a script that stands in for adb: it lists one emulator, answers ro.kernel.qemu with qemu, and everything else with nothing; with
// hang set, `devices` does not return.
func fakeADB(t *testing.T, qemu string, hang bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for adb")
	}
	devices := `echo "List of devices attached"; echo "emulator-5554 device product:sdk model:Pixel"; exit 0`
	if hang {
		devices = `exec sleep 30`
	}
	script := "#!/bin/sh\ncase \"$*\" in\n  *devices*) " + devices + ";;\n  *getprop*ro.kernel.qemu*) echo \"" + qemu + "\"; exit 0;;\n  *) exit 0;;\nesac\n"
	path := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// The whole check through a real Manager and adb: an emulator-named device is verified only when it says so itself.
func TestIsVerifiedDisposableThroughAdb(t *testing.T) {
	ctx := context.Background()
	phone := NewManagerWithPaths(ToolPaths{ADB: fakeADB(t, "", false)})
	if phone.IsVerifiedDisposable(ctx, PlatformAndroid, "emulator-5554") {
		t.Error("a device named like an emulator that does not say so was verified")
	}
	emu := NewManagerWithPaths(ToolPaths{ADB: fakeADB(t, "1", false)})
	if !emu.IsVerifiedDisposable(ctx, PlatformAndroid, "emulator-5554") {
		t.Error("an emulator that says so was not verified")
	}
	if emu.IsVerifiedDisposable(ctx, PlatformAndroid, "emulator-9999") {
		t.Error("a serial that is not listed was verified")
	}
}

// A hung adb must not hang the question: the timeout is taken before the list, and the answer is no (m: the "8 s" bounds the whole check).
func TestIsVerifiedDisposableFailsClosedWhenAdbHangs(t *testing.T) {
	old := verifyTimeout
	verifyTimeout = 400 * time.Millisecond
	t.Cleanup(func() { verifyTimeout = old })
	m := NewManagerWithPaths(ToolPaths{ADB: fakeADB(t, "1", true)})
	start := time.Now()
	got := m.IsVerifiedDisposable(context.Background(), PlatformAndroid, "emulator-5554")
	if got {
		t.Error("a hung adb was taken for a verified emulator")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("the check took %v with a 400ms timeout: the list ran outside the timeout", d)
	}
}
