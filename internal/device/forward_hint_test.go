package device

import (
	"context"
	"strings"
	"testing"
)

const hintList = "emulator-5554 tcp:48911 tcp:48686\nemulator-5600 tcp:48800 tcp:48686\n"

// ForwardHint speaks only when `adb forward --list` shows the port mapped to ANOTHER plain serial, and names that serial and the command that removes it.
func TestForwardHintBlamesOnlyAForwardTheListShows(t *testing.T) {
	ctx := context.Background()
	got := ForwardHint(ctx, forwardListRunner(hintList), 48911, "emulator-5600")
	for _, want := range []string{"agent port 48911 is forwarded to another device (emulator-5554)", "adb -s emulator-5554 forward --remove tcp:48911", "--agent-port"} {
		if !strings.Contains(got, want) {
			t.Errorf("hint %q lacks %q", got, want)
		}
	}
	if !strings.HasPrefix(got, " — ") || strings.Contains(got, "pid") {
		t.Errorf("hint must start with the separator and name no pid: %q", got)
	}
	for name, c := range map[string]struct {
		runner AdbRunner
		port   int
		own    string
	}{
		"no mapping for the port (the adb server holds it for another reason)": {forwardListRunner(hintList), 48999, "emulator-5600"},
		"an empty list":              {forwardListRunner(""), 48911, "emulator-5600"},
		"this device's own forward":  {forwardListRunner(hintList), 48800, "emulator-5600"},
		"an adb that fails":          {failingRunner{}, 48911, "emulator-5600"},
		"an adb that fails mid-list": {partialRunner(hintList), 48911, "emulator-5600"},
		"a serial that is not plain": {forwardListRunner("emu;rm tcp:48911 tcp:1\n"), 48911, "emulator-5600"},
		"no adb at all":              {nil, 48911, "emulator-5600"},
	} {
		if got := ForwardHint(ctx, c.runner, c.port, c.own); got != "" {
			t.Errorf("%s: want no hint, got %q", name, got)
		}
	}
}

// One read-only call, and none when the context is already cancelled.
func TestForwardHintAsksForTheListOnceAndRespectsCancellation(t *testing.T) {
	rec := &listRecorder{out: hintList}
	ForwardHint(context.Background(), rec, 48911, "emulator-5600")
	if len(rec.calls) != 1 || strings.Join(rec.calls[0], " ") != "forward --list" {
		t.Errorf("calls = %v, want one `forward --list`", rec.calls)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	rec = &listRecorder{out: hintList}
	if got := ForwardHint(cancelled, rec, 48911, "emulator-5600"); got != "" || len(rec.calls) != 0 {
		t.Errorf("a cancelled context must make no call and give no hint: %q %v", got, rec.calls)
	}
}
