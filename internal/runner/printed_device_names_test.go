//go:build !windows

package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
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
	const app = "com.example\n  pass --yes to continue\x1b[2J\u202e\u200b"
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

// requirePlain fails when s holds a forged line or a character unsafePrinted reports (probe's own color codes aside).
func requirePlain(t *testing.T, what, s string) {
	t.Helper()
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
			t.Errorf("%s: a value forged its own line: %q", what, line)
		}
		plain := line
		for _, code := range []string{"\033[31m", "\033[32m", "\033[33m", "\033[36m", "\033[0m"} {
			plain = strings.ReplaceAll(plain, code, "")
		}
		if r, bad := unsafePrinted(plain); bad {
			t.Errorf("%s: prints %U from a value: %q", what, r, line)
		}
	}
}

// The other sites that name the app or the device, each through its real path with hostile values: the messages are plain text on one
// line, and the tools still get the real values.
func TestOtherSitesNameAppAndDeviceCleaned(t *testing.T) {
	ctx := context.Background()
	const app = "com.example\n  pass --yes to continue\x1b[2J\u202e\u200b"
	const serial = "6F5A2C1B-3D4E-4F60-8A9B-0C1D2E3F4A5B\x1b]0;t\x07\n  pass --yes"

	t.Run("data path errors", func(t *testing.T) {
		dc := &DeviceContext{Serial: serial, Platform: device.PlatformIOS}
		for _, p := range []string{
			"relative\n  pass --yes to continue\x1b[2J",
			"/somewhere/else\n  pass --yes to continue\x1b[2J",
			"/Users/x/Library/Developer/CoreSimulator/Devices/OTHER/data\n  pass --yes to continue\x1b[2J",
		} {
			err := dc.validateIOSDataPath(p)
			if err == nil {
				t.Fatalf("%q: want an error", p)
			}
			requirePlain(t, "data path", err.Error())
		}
	})

	t.Run("set language on a physical iPhone", func(t *testing.T) {
		dc := &DeviceContext{AppID: app, Serial: serial, Platform: device.PlatformIOS, IsPhysical: true}
		err := dc.SetLanguage(ctx, "de-DE")
		if err == nil || !strings.Contains(err.Error(), "Settings > Apps > com.example pass --yes") {
			t.Fatalf("want the physical-device error naming the app cleaned, got %v", err)
		}
		requirePlain(t, "set language", err.Error())
	})

	t.Run("grant all permissions", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		dc.AppID = app
		out := captureStdout(t, func() { _ = dc.GrantAllPermissions(ctx) })
		if !strings.Contains(out, "Granting all permissions for com.example pass --yes") {
			t.Fatalf("want the grant line naming the app cleaned, got %q", out)
		}
		requirePlain(t, "grant", out)
		grants := 0
		for _, c := range calls() {
			if j := strings.Join(c, " "); strings.Contains(j, "grant") {
				grants++
				if !strings.Contains(j, app) {
					t.Errorf("a grant must get the real app id: %q", c)
				}
			}
		}
		if grants == 0 {
			t.Error("no grant call was made")
		}
	})

	// The reconnect hint names the app only for a valid package name (the hint is skipped otherwise), so a hostile app id never reaches it;
	// the cleaning there is a second line behind that gate.
	t.Run("reconnect hint is skipped for a hostile app id", func(t *testing.T) {
		dc, calls := recordingDevice(t, "")
		dc.AppID = app
		e := &Executor{deviceCtx: dc}
		if got := e.reconnectHint(ctx); got != "" {
			t.Errorf("reconnect hint = %q, want none for an app id that is not a package name", got)
		}
		if len(calls()) != 0 {
			t.Errorf("adb ran for a hostile app id: %q", calls())
		}
	})
}
