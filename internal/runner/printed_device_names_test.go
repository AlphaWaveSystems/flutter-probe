package runner

import (
	"context"
	"strings"
	"testing"
)

// unsafePrinted is a character that must never reach a terminal from an app id or a serial.
func unsafePrinted(s string) (rune, bool) {
	for _, r := range s {
		switch {
		case r == '\r', r == 0x1b, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0x200e, r == 0x200f:
			return r, true
		}
	}
	return 0, false
}

// The lifecycle steps print the app id and the serial (kill, launch, restart, clear data). With hostile values they print plain text
// on their own lines, and adb still gets the real values.
func TestLifecycleLinesPrintAppAndSerialCleaned(t *testing.T) {
	ctx := context.Background()
	const app = "com.example\n  pass --yes to continue\x1b[2J‮​"
	const serial = "emulator-5554\x1b]0;t\x07"
	for name, step := range map[string]func(*DeviceContext) error{
		"kill":       func(dc *DeviceContext) error { return dc.KillApp(ctx) },
		"launch":     func(dc *DeviceContext) error { return dc.LaunchApp(ctx) },
		"restart":    func(dc *DeviceContext) error { return dc.RestartApp(ctx) },
		"clear data": func(dc *DeviceContext) error { return dc.ClearAppData(ctx) },
	} {
		t.Run(name, func(t *testing.T) {
			dc, calls := recordingDevice(t, "")
			dc.AppID, dc.Serial = app, serial
			out := captureStdout(t, func() { _ = step(dc) })
			if strings.TrimSpace(out) == "" {
				t.Fatal("the step printed nothing")
			}
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
					t.Errorf("the app id forged its own line: %q", line)
				}
				plain := line
				for _, code := range []string{"\033[31m", "\033[32m", "\033[33m", "\033[36m", "\033[0m"} {
					plain = strings.ReplaceAll(plain, code, "")
				}
				if r, bad := unsafePrinted(plain); bad {
					t.Errorf("a line prints %U from the app id or the serial: %q", r, line)
				}
			}
			// adb gets the real serial: only the printed text is cleaned.
			got := calls()
			if len(got) == 0 {
				t.Fatal("no adb call was made")
			}
			for _, c := range got {
				if len(c) >= 2 && c[0] == "-s" && c[1] != serial {
					t.Errorf("adb got the serial %q, want the real one %q", c[1], serial)
				}
			}
		})
	}
}
