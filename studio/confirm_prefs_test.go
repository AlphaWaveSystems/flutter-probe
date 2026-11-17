package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// verified makes the device check answer v for the test.
func verified(t *testing.T, v bool) {
	t.Helper()
	old := verifyDisposable
	verifyDisposable = func(*connection) bool { return v }
	t.Cleanup(func() { verifyDisposable = old })
}

func withPrefs(t *testing.T, on bool) {
	t.Helper()
	dir := t.TempDir()
	old := userprefs.Dir
	userprefs.Dir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userprefs.Dir = old })
	if on {
		if err := userprefs.SetClearDataWithoutAsking(true); err != nil {
			t.Fatal(err)
		}
	}
}

// With the person's setting on, a simulator or an emulator is not asked: the step is allowed at once, and the window is told in a note.
func TestClearDataWithoutAskingOnASimulator(t *testing.T) {
	withPrefs(t, true)
	verified(t, true)
	c := newConfirmApp(t, true)
	c.a.conn.disposable = true
	c.a.conn.deviceName = "Pixel 8"
	c.a.conn.deviceID = "emulator-5554"
	answer := c.ask()
	select {
	case ok := <-answer:
		if !ok {
			t.Fatal("the step was refused although the setting is on and the device is a simulator")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the step waited for an answer that is not needed")
	}
	note := c.next(t, "confirm:note")[0].(string)
	for _, want := range []string{"Cleared data of com.example.app", "Pixel 8", "emulator-5554", "without asking"} {
		if !strings.Contains(note, want) {
			t.Errorf("the note lacks %q: %q", want, note)
		}
	}
	select {
	case ev := <-c.events:
		if ev.Name == "confirm:request" {
			t.Error("a question was shown")
		}
	default:
	}
}

// Everything else still asks: the setting off, a device that is not positively a simulator or an emulator (a phone, WiFi, unknown), and a run
// that is not there.
func TestClearDataStillAsksWhenItShould(t *testing.T) {
	for name, tc := range map[string]struct{ prefs, disposable bool }{
		"the setting off on a simulator":    {false, true},
		"the setting on, a physical device": {true, false},
		"the setting off on a physical":     {false, false},
	} {
		withPrefs(t, tc.prefs)
		verified(t, true)
		c := newConfirmApp(t, true)
		c.a.conn.disposable = tc.disposable
		answer := c.ask()
		req := c.next(t, "confirm:request")
		if len(req) == 0 {
			t.Fatalf("%s: no question", name)
		}
		select {
		case ok := <-answer:
			t.Fatalf("%s: the step was answered (%v) without the person", name, ok)
		case <-time.After(200 * time.Millisecond):
		}
		c.cancel()
		<-answer // the step has ended: nothing of it is left running for the next test
	}
	// no window to ask, setting on, physical device: refused, not allowed
	withPrefs(t, true)
	c := newConfirmApp(t, false)
	c.a.conn.disposable = false
	if ok := <-c.ask(); ok {
		t.Error("with nobody to ask and a physical device the step ran")
	}
}

// A project cannot turn it on: nothing in a workspace's probe.yaml (or any file in it) is read for this setting.
func TestAProjectCannotTurnClearDataWithoutAskingOn(t *testing.T) {
	withPrefs(t, false)
	ws := t.TempDir()
	_ = os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte("project:\n  app: com.example.app\nstudio:\n  clear_data_without_asking: true\nclearDataWithoutAsking: true\ndefaults:\n  clear_data_without_asking: true\n"), 0o644)
	_ = os.WriteFile(filepath.Join(ws, "prefs.json"), []byte(`{"clearDataWithoutAsking": true}`), 0o644)
	c := newConfirmApp(t, true)
	c.a.SetWorkspace(ws)
	c.a.conn.disposable = true
	answer := c.ask()
	c.next(t, "confirm:request") // it asked
	select {
	case ok := <-answer:
		t.Fatalf("a project's files answered the question: %v", ok)
	case <-time.After(200 * time.Millisecond):
	}
	c.cancel()
	<-answer
}

// The bound methods are for the window only: not the automation endpoint, and over the headless bridge only for a test server that says its page is
// the window.
func TestThePrefsMethodsAreWindowOnly(t *testing.T) {
	for _, name := range []string{"GetStudioPrefs", "SetClearDataWithoutAsking"} {
		if cat := boundMethods[name]; cat != catUserPrefs || !automationNever[cat] {
			t.Errorf("%s is %q, automationNever = %v", name, cat, automationNever[cat])
		}
	}
	t.Setenv("PROBE_STUDIO_E2E_ALLOW_CONFIRM_OVER_HTTP", "")
	if !methodRefusedOverHTTP("SetClearDataWithoutAsking") {
		t.Error("the bridge lets a script turn the setting on")
	}
	t.Setenv("PROBE_STUDIO_E2E_ALLOW_CONFIRM_OVER_HTTP", "1")
	if methodRefusedOverHTTP("SetClearDataWithoutAsking") {
		t.Error("the test server's page cannot press the box")
	}
	for _, rpc := range append(append([]string(nil), automationBackendMethods...), automationRPC...) {
		if strings.Contains(strings.ToLower(rpc), "prefs") || strings.Contains(strings.ToLower(rpc), "clear") {
			t.Errorf("the automation endpoint reaches %s", rpc)
		}
	}
}

// The form's methods read and write the person's file.
func TestStudioPrefsRoundTrip(t *testing.T) {
	withPrefs(t, false)
	a := &App{}
	if a.GetStudioPrefs().ClearDataWithoutAsking {
		t.Fatal("on by default")
	}
	if err := a.SetClearDataWithoutAsking(true); err != nil || !a.GetStudioPrefs().ClearDataWithoutAsking {
		t.Fatalf("set on: %v", err)
	}
	if err := a.SetClearDataWithoutAsking(false); err != nil || a.GetStudioPrefs().ClearDataWithoutAsking {
		t.Fatalf("set off: %v", err)
	}
}

// The list says emulator but the device does not confirm it (an Android phone with a serial that only looks like one): the question is shown.
func TestAnUnconfirmedDeviceStillAsks(t *testing.T) {
	withPrefs(t, true)
	verified(t, false)
	c := newConfirmApp(t, true)
	c.a.conn.disposable = true
	answer := c.ask()
	c.next(t, "confirm:request")
	select {
	case ok := <-answer:
		t.Fatalf("an unconfirmed device was answered (%v) without the person", ok)
	case <-time.After(200 * time.Millisecond):
	}
	c.cancel()
	<-answer
}

// The preferences folder of the headless test server is honoured only in headless mode; a desktop Studio ignores the variable.
func TestTheTestConfigDirIsHeadlessOnly(t *testing.T) {
	if testUserConfigDir(false, "/tmp/x") != "" {
		t.Error("a desktop Studio honoured the test variable")
	}
	if testUserConfigDir(true, "/tmp/x") != "/tmp/x" || testUserConfigDir(true, "") != "" {
		t.Error("the headless server did not")
	}
}

// A preferences file that is not trusted is reported to the form and the setting is off.
func TestTheFormSeesAWarningForAnUntrustedFile(t *testing.T) {
	withPrefs(t, true)
	path, _ := userprefs.Path()
	if err := os.Chmod(path, 0o666); err != nil {
		t.Skip("no chmod")
	}
	p := (&App{}).GetStudioPrefs()
	if p.ClearDataWithoutAsking || !strings.Contains(p.Warning, "other users") {
		t.Errorf("prefs = %+v", p)
	}
}

// fakeAdbManager is a device manager whose adb lists one emulator and answers ro.kernel.qemu with qemu (a shell script).
func fakeAdbManager(t *testing.T, qemu string) *device.Manager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for adb")
	}
	script := "#!/bin/sh\ncase \"$*\" in\n  *devices*) echo \"List of devices attached\"; echo \"emulator-5554 device product:sdk model:Pixel\"; exit 0;;\n  *getprop*ro.kernel.qemu*) echo \"" + qemu + "\"; exit 0;;\n  *) exit 0;;\nesac\n"
	path := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return device.NewManagerWithPaths(device.ToolPaths{ADB: path})
}

// The production path, not the test seam: with the setting on and a connection the list calls an emulator, a device that does not say so itself is
// still asked about, and one that does is not.
func TestTheProductionDeviceCheckGatesTheShortcut(t *testing.T) {
	withPrefs(t, true)
	verifyDisposable = nil // the real check, through the manager and its adb
	c := newConfirmApp(t, true)
	c.a.deviceMgr = fakeAdbManager(t, "")
	c.a.ctx = context.Background()
	c.a.conn.disposable = true
	c.a.conn.platform = device.PlatformAndroid
	c.a.conn.deviceID = "emulator-5554"
	answer := c.ask()
	c.next(t, "confirm:request") // the question was shown
	select {
	case ok := <-answer:
		t.Fatalf("a device that does not say it is an emulator was answered (%v) without the person", ok)
	case <-time.After(200 * time.Millisecond):
	}
	c.cancel()
	<-answer

	withPrefs(t, true)
	c2 := newConfirmApp(t, true)
	c2.a.deviceMgr = fakeAdbManager(t, "1")
	c2.a.ctx = context.Background()
	c2.a.conn.disposable = true
	c2.a.conn.platform = device.PlatformAndroid
	c2.a.conn.deviceID = "emulator-5554"
	if ok := answerOf(t, c2.ask()); !ok {
		t.Error("an emulator that says so was asked about")
	}
}

// The note says when the wipe happens when it is the before-every-test question that was answered without asking, and says "Cleared data" for a step.
func TestTheNoQuestionNoteSaysWhenTheWipeHappens(t *testing.T) {
	withPrefs(t, true)
	verified(t, true)
	for _, tc := range []struct {
		message string
		want    string
		notWant string
	}{
		{"This will permanently delete ALL app data for a.b on s BEFORE EVERY TEST of this run.", "Will clear data before every test", "Cleared data of"},
		{"This will permanently delete ALL app data for a.b on s.", "Cleared data of", "before every test"},
	} {
		c := newConfirmApp(t, true)
		c.a.conn.disposable = true
		out := make(chan bool, 1)
		go func() { out <- c.a.confirmClearData(c.dc)(tc.message) }()
		answerOf(t, out)
		note := c.next(t, "confirm:note")[0].(string)
		if !strings.Contains(note, tc.want) || strings.Contains(note, tc.notWant) || !strings.Contains(note, "com.example.app") {
			t.Errorf("%q: note %q", tc.message, note)
		}
	}
}
