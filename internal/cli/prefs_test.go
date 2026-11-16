package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// The question is answered only when the person's setting is on AND the device is positively a simulator or an emulator; otherwise the prompt
// (or no prompt) is exactly what it was.
func TestConfirmWithPrefs(t *testing.T) {
	asked := 0
	ask := func(string) bool { asked++; return false }
	var out bytes.Buffer
	for name, tc := range map[string]struct {
		allow, disposable bool
		wantAsk           bool
	}{
		"setting off, simulator":      {false, true, true},
		"setting on, physical device": {true, false, true},
		"setting off, physical":       {false, false, true},
	} {
		asked = 0
		got := confirmWithPrefs(ask, tc.allow, tc.disposable, &out)("clear app data")
		if got || (asked == 1) != tc.wantAsk {
			t.Errorf("%s: answer %v asked %d", name, got, asked)
		}
	}
	asked = 0
	out.Reset()
	if !confirmWithPrefs(ask, true, true, &out)("clear app data") || asked != 0 {
		t.Error("setting on and a simulator did not answer yes without asking")
	}
	if !strings.Contains(out.String(), "without asking") {
		t.Errorf("no line says it: %q", out.String())
	}
	// no terminal (a nil prompt): off stays nil (the step fails with the --yes hint), on for a simulator answers yes
	if confirmWithPrefs(nil, false, true, &out) != nil || confirmWithPrefs(nil, true, false, &out) != nil {
		t.Error("a nil prompt became non-nil without the setting and a disposable device")
	}
	if f := confirmWithPrefs(nil, true, true, &out); f == nil || !f("x") {
		t.Error("in CI the setting on a simulator should answer yes")
	}
}

func TestPrefsCommand(t *testing.T) {
	t.Setenv("PROBE_USER_CONFIG_DIR", t.TempDir())
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		c := rootCmd
		c.SetOut(&out)
		c.SetErr(&out)
		c.SetArgs(append([]string{"prefs"}, args...))
		err := c.Execute()
		c.SetArgs(nil)
		return out.String(), err
	}
	if out, err := run("show"); err != nil || !strings.Contains(out, "clear-data-without-asking: off") {
		t.Fatalf("show = %q, %v", out, err)
	}
	if out, err := run("set", "clear-data-without-asking", "on"); err != nil || !strings.Contains(out, ": on") || !userprefs.Load().ClearDataWithoutAsking {
		t.Fatalf("set on = %q, %v", out, err)
	}
	if out, err := run("show"); err != nil || !strings.Contains(out, ": on") {
		t.Fatalf("show after = %q, %v", out, err)
	}
	if _, err := run("set", "clear-data-without-asking", "maybe"); err == nil {
		t.Error("a bad value was accepted")
	}
	if _, err := run("set", "nonsense", "on"); err == nil {
		t.Error("an unknown setting was accepted")
	}
	if _, err := run("set", "clear-data-without-asking", "off"); err != nil || userprefs.Load().ClearDataWithoutAsking {
		t.Error("off was not saved")
	}
}
