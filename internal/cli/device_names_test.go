package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/deviceview"
)

// hiddenInPrint reports the first character in s that must never reach a terminal from a device name: an escape, a carriage return,
// a bidi control or a zero-width character. A newline is checked by the callers, per line.
func hiddenInPrint(s string) (rune, bool) {
	for _, r := range s {
		switch {
		case r == 0x1b, r == '\r', r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0x200e, r == 0x200f, r == 0xfeff:
			return r, true
		}
	}
	return 0, false
}

// probe device list prints the model name adb reports. A hostile name (an escape sequence that clears the screen, a bidi override, a
// zero-width space) is printed as plain text on its row.
func TestDeviceListPrintsAHostileNameCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake adb is a shell script")
	}
	dir := t.TempDir()
	// The model is one adb field, so it cannot hold a space or a new line; underscores become spaces.
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *devices*) printf 'List of devices attached\\nR5HOSTILE device product:x model:Pixel\\033[2J\\342\\200\\256_pass_--yes\\342\\200\\213 device:x\\n\\n' ;;\n" +
		"  *) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "adb"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out := captureStdoutForTest(t, func() {
		if err := deviceListCmd.RunE(deviceListCmd, nil); err != nil {
			t.Errorf("device list: %v", err)
		}
	})
	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "R5HOSTILE") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("the fake device is not listed:\n%q", out)
	}
	if r, bad := hiddenInPrint(row); bad {
		t.Errorf("the row prints %U from the device name: %q", r, row)
	}
	if !strings.Contains(row, "pass --yes") || !strings.Contains(row, "Pixel") {
		t.Errorf("the visible text of the name must stay: %q", row)
	}
}

// A device name from simctl's JSON can hold a new line. The refusal of probe device set names it on one line, so a fake second line
// ("pass --yes") cannot appear as if probe had printed it.
func TestDeviceSetRefusalNamesAHostileDeviceOnOneLine(t *testing.T) {
	hostile := device.Device{
		ID:       "AAAA-\u200bBBBB",
		Name:     "iPhone\n  pass --yes to continue\x1b[31m\u202e",
		State:    "Shutdown\r",
		Platform: device.PlatformIOS,
	}
	_, err := checkSettable(hostile)
	if err == nil {
		t.Fatal("a shut-down device must be refused")
	}
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Errorf("the message must be one line: %q", msg)
	}
	if r, bad := hiddenInPrint(msg); bad {
		t.Errorf("the message prints %U from the device: %q", r, msg)
	}
	physical := device.Device{ID: "X", Name: "iPhone\n  pass --yes", State: "online", Platform: device.PlatformIOS, Transport: "usb"}
	if _, err := checkSettable(physical); err == nil || strings.Contains(err.Error(), "\n") {
		t.Errorf("the physical-device refusal must be one line: %v", err)
	}
}

// The value used for adb and simctl is never changed: only what is printed is cleaned.
func TestCleaningIsForDisplayOnly(t *testing.T) {
	d := device.Device{ID: "emulator-5554", Name: "Pixel\x1b[2J", State: "device", Platform: device.PlatformAndroid}
	got, err := checkSettable(d)
	if err != nil || got.ID != "emulator-5554" || got.Name != "Pixel\x1b[2J" {
		t.Errorf("checkSettable must return the device unchanged: %+v, %v", got, err)
	}
}

// hostileName is a device name with a new line and a fake probe line, an escape sequence, a bidi override and a zero-width space. It
// is built from code points so this source stays ASCII.
var hostileName = "Pixel\n  pass --yes to continue\x1b[2J" + string(rune(0x202e)) + string(rune(0x200b))

// cleanOutput fails the test when out holds a line that starts with the forged text, or a character hiddenInPrint reports.
func cleanOutput(t *testing.T, what, out string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
			t.Errorf("%s: a device name forged its own line: %q", what, line)
		}
		if r, bad := hiddenInPrint(line); bad {
			t.Errorf("%s: a line prints %U from the device: %q", what, r, line)
		}
	}
}

// The device view's startup line names the device; in every output mode it is plain text on one line.
func TestDeviceViewStartupLineNamesAHostileDeviceCleaned(t *testing.T) {
	for _, mode := range []string{"json", "plain", "print-token"} {
		asJSON, printToken := mode == "json", mode == "print-token"
		var out, errOut syncBuf
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- serveDeviceView(ctx, deviceview.Options{
				Agent:  &stubAgent{},
				Device: deviceview.DeviceInfo{ID: "dev-1", Platform: "android", Name: hostileName},
			}, asJSON, printToken, &out, &errOut)
		}()
		deadline := time.Now().Add(3 * time.Second)
		for !strings.Contains(errOut.String(), "listening") && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errOut.String(), "Pixel pass --yes to continue") {
			t.Fatalf("%s: the startup line must name the device: %q", mode, errOut.String())
		}
		cleanOutput(t, mode, errOut.String())
	}
}

// With two ready devices, probe device view asks for one and lists their ids: as plain text on one line.
func TestDeviceViewChoiceListsHostileIDsCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake adb is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n  *devices*) printf 'List of devices attached\\nR5A\\033[2JX device\\nR5B\\033]0;t\\007Y device\\n\\n' ;;\n  *) exit 1 ;;\nesac\n"
	adb := filepath.Join(dir, "adb")
	if err := os.WriteFile(adb, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dm := device.NewManagerWithPaths(device.ToolPaths{ADB: adb})
	_, err := pickViewDevice(context.Background(), dm, "")
	if err == nil || !strings.Contains(err.Error(), "several devices are ready") {
		t.Fatalf("want the choice error, got %v", err)
	}
	if strings.Contains(err.Error(), "\x1b") || strings.Contains(err.Error(), "\x07") {
		t.Errorf("the choice lists raw ids: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "R5A [2JX") || !strings.Contains(err.Error(), "R5B ]0;t Y") {
		t.Errorf("the ids' visible text must stay: %q", err.Error())
	}
}

// The connect line of probe device view and the done line of probe device set name the device as plain text on one line.
func TestConnectAndSetDoneLinesNameAHostileDeviceCleaned(t *testing.T) {
	d := device.Device{ID: "R5\x1b]0;t\x07X", Name: hostileName}
	for what, line := range map[string]string{"connect": connectingLine(d), "set done": setDoneLine("locale", "de-DE", d)} {
		if !strings.Contains(line, "Pixel pass --yes to continue") {
			t.Errorf("%s: the line must name the device: %q", what, line)
		}
		if strings.Count(strings.TrimRight(line, "\n"), "\n") != 0 || strings.Contains(line, "\x07") {
			t.Errorf("%s: the line must be one clean line: %q", what, line)
		}
		cleanOutput(t, what, line)
	}
}
