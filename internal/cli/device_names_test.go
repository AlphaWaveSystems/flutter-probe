package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/device"
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
		ID:       "AAAA-​BBBB",
		Name:     "iPhone\n  pass --yes to continue\x1b[31m‮",
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
