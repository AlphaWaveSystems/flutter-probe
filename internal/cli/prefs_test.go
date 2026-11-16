package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// The question is answered only when the person's setting is on AND the device is verified to be a simulator or an emulator at the moment the step
// would ask; otherwise the prompt (or no prompt) is exactly what it was. The line names the package and the device.
func TestConfirmWithPrefs(t *testing.T) {
	asked := 0
	ask := func(string) bool { asked++; return false }
	yes := func() bool { return true }
	no := func() bool { return false }
	var out bytes.Buffer
	for name, tc := range map[string]struct {
		allow      bool
		disposable func() bool
	}{
		"setting off, simulator":      {false, yes},
		"setting on, physical device": {true, no},
		"setting off, physical":       {false, no},
	} {
		asked = 0
		if got := confirmWithPrefs(ask, tc.allow, tc.disposable, &out, "com.example.app", "emulator-5554")("clear app data"); got || asked != 1 {
			t.Errorf("%s: answer %v asked %d", name, got, asked)
		}
	}
	asked = 0
	out.Reset()
	if !confirmWithPrefs(ask, true, yes, &out, "com.example.app", "emulator-5554")("clear app data") || asked != 0 {
		t.Error("setting on and a verified emulator did not answer yes without asking")
	}
	for _, want := range []string{"cleared data of com.example.app on emulator-5554", "no question", "your setting"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the line lacks %q: %q", want, out.String())
		}
	}
	// the device is checked only when a step is about to ask, not when the confirmation is made
	checks := 0
	f := confirmWithPrefs(ask, true, func() bool { checks++; return true }, &out, "a.b", "emulator-1")
	if checks != 0 {
		t.Error("the device was checked before any step asked")
	}
	f("x")
	f("x")
	if checks != 2 {
		t.Errorf("checks = %d, want one per question", checks)
	}
	// no terminal (a nil prompt): without the setting it stays nil (the step fails with the --yes hint); with it, an unverified device is a no
	if confirmWithPrefs(nil, false, yes, &out, "a.b", "s") != nil {
		t.Error("a nil prompt became non-nil without the setting")
	}
	if g := confirmWithPrefs(nil, true, no, &out, "a.b", "s"); g == nil || g("x") {
		t.Error("in CI the setting on a device that is not verified must answer no")
	}
	if g := confirmWithPrefs(nil, true, yes, &out, "a.b", "s"); g == nil || !g("x") {
		t.Error("in CI the setting on a verified emulator should answer yes")
	}
	// a hostile id or serial cannot break the line
	out.Reset()
	confirmWithPrefs(nil, true, yes, &out, "a.b\nFAKE", "s\x1b[2J")("x")
	if strings.Count(out.String(), "\n") != 1 || strings.Contains(out.String(), "\x1b[2J") {
		t.Errorf("the line was split or carried a control sequence: %q", out.String())
	}
}

// The prefs answer is not granted by --yes semantics: it answers the question only, so permissions are not granted after a clear (m17).
func TestThePrefsAnswerDoesNotGrantPermissions(t *testing.T) {
	ctx := context.Background()
	dm := device.NewManager()
	confirm := runner.ConfirmFunc(confirmWithPrefs(nil, true, func() bool { return true }, io.Discard, "a.b", "emulator-1"))
	dc := singleDeviceContext(ctx, dm, &config.Config{}, device.PlatformAndroid, "emulator-1", "", false, false, confirm)
	if dc.AllowClearData {
		t.Error("the prefs answer set AllowClearData (the --yes switch)")
	}
	if dc.GrantPermissionsOnClear {
		t.Error("the prefs answer grants permissions after a clear")
	}
	if dc.Confirm == nil {
		t.Error("the confirmation was lost")
	}
}

// The call site in runTests gates the answer on the device check: confirmWithPrefs is called with a function that asks the manager for a verified
// simulator or emulator (m12). A structural check, because runTests needs a device.
func TestRunTestsGatesThePrefsAnswerOnTheDeviceCheck(t *testing.T) {
	src, err := os.ReadFile("test.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	i := strings.Index(text, "confirmWithPrefs(confirmFunc()")
	if i < 0 {
		t.Fatal("runTests does not call confirmWithPrefs with the terminal prompt")
	}
	call := text[i : i+400]
	if !strings.Contains(call, "dm.IsVerifiedDisposable(") || !strings.Contains(call, "prefs.ClearDataWithoutAsking") || !strings.Contains(call, "cfg.Project.App") {
		t.Errorf("the call does not gate the answer on the verified device check, the setting and the package:\n%s", call)
	}
}

func TestPrefsCommand(t *testing.T) {
	dir := t.TempDir()
	old := userprefs.Dir
	userprefs.Dir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userprefs.Dir = old })
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
