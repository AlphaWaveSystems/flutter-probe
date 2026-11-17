package device

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const hostileAppID = "com.example\n  pass --yes to continue\x1b[2J\u202e\u200b"

// unsafeRune is a character that must never reach a terminal from a device or app name.
func unsafeRune(s string) (rune, bool) {
	for _, r := range s {
		switch {
		case r == '\n', r == '\r', r == 0x1b, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0x200e, r == 0x200f:
			return r, true
		}
	}
	return 0, false
}

// The token error names the app on one line, with no control, bidi or zero-width character, and keeps its marker.
func TestTokenErrorNamesTheAppCleaned(t *testing.T) {
	msg := TokenAppNotRunningError(5*time.Second, hostileAppID).Error()
	if r, bad := unsafeRune(msg); bad {
		t.Errorf("the error prints %U from the app id: %q", r, msg)
	}
	if !strings.Contains(msg, AppNotRunningMarker) || !strings.Contains(msg, "pass --yes to continue") {
		t.Errorf("the visible text and the marker must stay: %q", msg)
	}
}

// Installing and launching prints the app path, the serial and the app id; each is printed cleaned, and adb still gets the real values.
func TestInstallAndLaunchPrintsNamesCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake adb is a shell script")
	}
	dir := t.TempDir()
	argsLog := filepath.Join(dir, "args.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + argsLog + "'\nexit 0\n"
	adb := filepath.Join(dir, "adb")
	if err := os.WriteFile(adb, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManagerWithPaths(ToolPaths{ADB: adb})
	serial := "R5\x1b]0;title\x07X"
	out := captureStdout(t, func() {
		_ = m.InstallAndLaunchApp(context.Background(), serial, PlatformAndroid, "/tmp/app\u202e.apk", hostileAppID, false)
	})
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if r, bad := unsafeRune(line); bad || strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
			t.Errorf("a printed line carries %U or a forged line from a name: %q", r, line)
		}
	}
	if !strings.Contains(out, "Installing") || !strings.Contains(out, "Launching") {
		t.Fatalf("the progress lines are missing: %q", out)
	}
	// Every adb call (install, launch) gets the real serial: only the printed text is cleaned.
	logged, _ := os.ReadFile(argsLog)
	calls := strings.Split(strings.TrimRight(string(logged), "\n"), "\n")
	if len(calls) < 2 {
		t.Fatalf("want an install and a launch call, got %q", logged)
	}
	for _, call := range calls {
		if strings.Contains(call, "install") || strings.Contains(call, "monkey") || strings.Contains(call, "am start") {
			if !strings.Contains(call, serial) {
				t.Errorf("adb must get the real serial, not the cleaned one: %q", call)
			}
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}
