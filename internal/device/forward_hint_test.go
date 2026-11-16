package device

import (
	"context"
	"strings"
	"testing"
	"time"
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

type ctxRecorder struct {
	deadline    time.Time
	hasDeadline bool
}

func (c *ctxRecorder) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	c.deadline, c.hasDeadline = ctx.Deadline()
	return []byte(hintList), nil
}

// The one adb call is bounded to 5 s (forwardHintTimeout), and a shorter deadline of the caller still wins.
func TestForwardHintIsBoundedToFiveSeconds(t *testing.T) {
	if forwardHintTimeout != 5*time.Second {
		t.Fatalf("forwardHintTimeout = %v, want 5s", forwardHintTimeout)
	}
	rec := &ctxRecorder{}
	start := time.Now()
	ForwardHint(context.Background(), rec, 48911, "emulator-5600")
	if !rec.hasDeadline {
		t.Fatal("the adb call has no deadline")
	}
	if d := rec.deadline.Sub(start); d < 4*time.Second || d > 5*time.Second+time.Second/2 {
		t.Errorf("the adb call's deadline is %v away, want about 5s", d)
	}

	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rec = &ctxRecorder{}
	ForwardHint(parent, rec, 48911, "emulator-5600")
	if d := time.Until(rec.deadline); !rec.hasDeadline || d > time.Second {
		t.Errorf("a shorter deadline of the caller must win, got %v", d)
	}
}
