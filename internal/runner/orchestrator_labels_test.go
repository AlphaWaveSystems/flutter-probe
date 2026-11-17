package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// unsafeInLine is a character that must never reach a terminal from a device id or name.
func unsafeInLine(s string) (rune, bool) {
	for _, r := range s {
		switch {
		case r == '\r', r == 0x1b, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0x200e, r == 0x200f:
			return r, true
		}
	}
	return 0, false
}

// A parallel run names each lane by its device. A device whose name and id hold a new line with a fake "pass --yes" line, an escape
// sequence, a bidi override and a zero-width space prints them as plain text on the lane's own line. The color codes probe adds itself
// are allowed; the lane label is checked on its own.
func TestParallelRunPrintsHostileDeviceLabelsCleaned(t *testing.T) {
	dir := t.TempDir()
	f := writeHarnessFile(t, dir, "a.probe", "test \"a\"\n  tap \"Fine\"\n")
	h := newHarnessRun(t, harnessConfig(), [][]string{{f}})
	const name = "Pixel\n  pass --yes to continue\x1b[2J‮​"
	h.po.devices[0].DeviceName = name
	h.po.devices[0].DeviceID = "emu\x1b]0;t\x07‮-​1\nX"

	var runErr error
	out := captureStdout(t, func() { _, runErr = h.po.Run(context.Background()) })
	if runErr != nil {
		t.Fatalf("run: %v", runErr)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") || strings.HasPrefix(strings.TrimSpace(line), "X") {
			t.Errorf("a device label forged its own line: %q", line)
		}
		label := line
		for _, code := range []string{"\033[32m", "\033[33m", "\033[31m", "\033[0m"} {
			label = strings.ReplaceAll(label, code, "")
		}
		if r, bad := unsafeInLine(label); bad {
			t.Errorf("a line prints %U from the device: %q", r, line)
		}
	}
	if !strings.Contains(out, "Pixel pass --yes to continue") {
		t.Errorf("the lane line must show the name's visible text: %q", out)
	}
}

// The labels are for display only: the lane's id and name stay as they are, and a long id is cut by characters, not bytes.
func TestLaneLabelsAreDisplayOnly(t *testing.T) {
	po := NewParallelOrchestrator(&config.Config{}, nil, []DeviceRun{{DeviceID: "id\nx", DeviceName: "n\x1b"}}, RunOptions{}, 47100)
	if po.devices[0].DeviceID != "id\nx" || po.devices[0].DeviceName != "n\x1b" {
		t.Errorf("the lane must keep its id and name: %+v", po.devices[0])
	}
	if got := po.shortID("日本語のデバイス名前とても長い"); got != "日本語のデバイス名前とて" {
		t.Errorf("shortID cuts by characters: %q", got)
	}
	if got := po.shortID("emulator-5554"); got != "emulator-555" {
		t.Errorf("shortID of a plain serial is unchanged from before: %q", got)
	}
}

// A lane that cannot connect is named in its error, and in the run's summary, by its id: as plain text on one line. A lane with no name
// is listed by its id the same way.
func TestFailedLaneErrorNamesTheDeviceCleaned(t *testing.T) {
	dir := t.TempDir()
	f := writeHarnessFile(t, dir, "a.probe", "test \"a\"\n  tap \"Fine\"\n")
	h := newHarnessRun(t, harnessConfig(), [][]string{{f}})
	h.po.devices[0].DeviceName = ""
	h.po.devices[0].DeviceID = "emu\n  pass --yes to continue\x1b[2J‮"
	h.po.devices[0].Port = 1 // no fake agent listens here: the connect fails

	var runErr error
	out := captureStdout(t, func() { _, runErr = h.po.Run(context.Background()) })
	text := out
	if runErr != nil {
		text += "\n" + runErr.Error()
	}
	if !strings.Contains(text, "pass --yes to continue") {
		t.Fatalf("the failed lane must be named: %q", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "pass --yes") {
			t.Errorf("a device id forged its own line: %q", line)
		}
		label := line
		for _, code := range []string{"\033[32m", "\033[33m", "\033[31m", "\033[0m"} {
			label = strings.ReplaceAll(label, code, "")
		}
		if r, bad := unsafeInLine(label); bad {
			t.Errorf("a line prints %U from the device id: %q", r, line)
		}
	}
}
