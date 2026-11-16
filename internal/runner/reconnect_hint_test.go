package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func hintExecutor(a *ceAdb, crashOn bool) *Executor {
	e := ceExecutor(a, crashOn)
	e.deviceCtx.Port = 48792
	return e
}

func liveAdb(forward string) *ceAdb {
	return &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{"4100"}, forward: forward}
}

func TestTheHintNamesADeadAppAsTheCause(t *testing.T) {
	dead := func() *ceAdb {
		return &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, forward: "emulator-5600 tcp:48792 tcp:48792\n"}
	}
	// the app ended during the step, with and without the evidence flag
	e := hintExecutor(dead(), false)
	if h := e.reconnectHint(context.Background()); !strings.Contains(h, "the app "+cePkg+" is not running: it ended during this step") || !strings.Contains(h, "--crash-evidence") || strings.Contains(h, "forward") {
		t.Errorf("flag off: %q", h)
	}
	e = hintExecutor(dead(), true)
	if h := e.reconnectHint(context.Background()); !strings.Contains(h, "is not running: it ended during this step (see the crash evidence for this test)") || strings.Contains(h, "--crash-evidence") {
		t.Errorf("flag on: %q", h)
	}
	// after the test's own kill step it says that instead, and says how to start the app
	e = hintExecutor(dead(), true)
	e.appStopped = true
	if h := e.reconnectHint(context.Background()); !strings.Contains(h, "the test stopped it (`kill the app`)") || !strings.Contains(h, "open the app") || strings.Contains(h, "ended during") {
		t.Errorf("after a kill: %q", h)
	}
}

// An adb forward of this device's own is never blamed; one that belongs to another device is named, with the command that removes it.
func TestTheHintBlamesAForwardOnlyWhenItBelongsToAnotherDevice(t *testing.T) {
	if h := hintExecutor(liveAdb("emulator-5600 tcp:48792 tcp:48792\n"), true).reconnectHint(context.Background()); h != "" {
		t.Errorf("the device's own forward was blamed: %q", h)
	}
	h := hintExecutor(liveAdb("emulator-5554 tcp:48792 tcp:48792\nemulator-5600 tcp:50000 tcp:50000\n"), false).reconnectHint(context.Background())
	if !strings.Contains(h, "agent port 48792 is forwarded to another device (emulator-5554)") || !strings.Contains(h, "adb -s emulator-5554 forward --remove tcp:48792") || !strings.Contains(h, "--agent-port") {
		t.Errorf("another device's forward: %q", h)
	}
	for name, fwd := range map[string]string{"no forward": "", "another port only": "emulator-5554 tcp:50000 tcp:50000\n", "garbage": "not a forward line\n\n tcp:48792\n"} {
		if h := hintExecutor(liveAdb(fwd), true).reconnectHint(context.Background()); h != "" {
			t.Errorf("%s: %q", name, h)
		}
	}
	// the dead-app cause comes first: a forward of another device does not hide it
	dead := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, forward: "emulator-5554 tcp:48792 tcp:48792\n"}
	if h := hintExecutor(dead, true).reconnectHint(context.Background()); !strings.Contains(h, "is not running") || strings.Contains(h, "another device") {
		t.Errorf("dead app and another forward: %q", h)
	}
}

func TestTheHintIsEmptyWhenNothingCanBeSaid(t *testing.T) {
	a := liveAdb("")
	a.dateErr = errors.New("device offline")
	if h := hintExecutor(a, true).reconnectHint(context.Background()); h != "" || len(a.calls) != 1 {
		t.Errorf("unreadable device: %q after %d calls", h, len(a.calls))
	}
	for name, mod := range map[string]func(*DeviceContext){
		"ios":        func(dc *DeviceContext) { dc.Platform = device.PlatformIOS },
		"no app id":  func(dc *DeviceContext) { dc.AppID = "" },
		"bad app id": func(dc *DeviceContext) { dc.AppID = "a.b;rm" },
		"no serial":  func(dc *DeviceContext) { dc.Serial = " " },
		"no adb":     func(dc *DeviceContext) { dc.CrashADB = nil },
	} {
		a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}
		e := hintExecutor(a, true)
		mod(e.deviceCtx)
		if h := e.reconnectHint(context.Background()); h != "" || len(a.calls) != 0 {
			t.Errorf("%s: %q after %d adb calls", name, h, len(a.calls))
		}
	}
	e := NewExecutor(nil, nil, nil, time.Second, false)
	if e.reconnectHint(context.Background()) != "" {
		t.Error("a cloud run got a hint")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}
	if h := hintExecutor(b, true).reconnectHint(ctx); h != "" || len(b.calls) != 0 {
		t.Errorf("a cancelled run: %q after %d calls", h, len(b.calls))
	}
}

// Through a step whose reconnect fails: the hint is on the step's error, the failure itself is unchanged, and a run whose app is alive gets nothing added.
func TestAFailedReconnectCarriesTheHint(t *testing.T) {
	run := func(a *ceAdb, appStopped bool) string {
		e := hintExecutor(a, false)
		e.appStopped = appStopped
		e.client = &ceClient{log: new([]string)}
		e.SetReconnectPolicy(1, time.Hour)
		e.timeout = 80 * time.Millisecond
		err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
		if err == nil {
			t.Fatal("the step passed")
		}
		return err.Error()
	}
	msg := run(&ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}, false)
	if !strings.Contains(msg, "broken pipe") || !strings.Contains(msg, "auto-reconnect attempt 1 failed") || !strings.Contains(msg, "the app "+cePkg+" is not running: it ended during this step") {
		t.Errorf("dead app: %q", msg)
	}
	if i, j := strings.Index(msg, "auto-reconnect attempt 1 failed"), strings.Index(msg, "is not running"); j < i {
		t.Errorf("the hint comes before the reconnect failure it explains: %q", msg)
	}
	if msg := run(&ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}, true); !strings.Contains(msg, "the test stopped it") {
		t.Errorf("after a kill: %q", msg)
	}
	if msg := run(liveAdb("emulator-5600 tcp:48792 tcp:48792\n"), false); strings.Contains(msg, "is not running") || strings.Contains(msg, "forwarded to another device") {
		t.Errorf("a live app with its own forward: %q", msg)
	}
}

// Every reconnect starts from options that call the host port this CLI's own forward on Android, so a dial error never blames an adb forward for it.
func TestReconnectDialOptionsCallTheAndroidPortTheCLIsOwn(t *testing.T) {
	dc := &DeviceContext{Platform: device.PlatformAndroid, Port: 48792, DevicePort: 48792, Serial: "emulator-5600", Manager: device.NewManager()}
	o := dc.reconnectDialOptions("tok")
	if !o.OwnAdbForward || o.Port != 48792 || o.Token != "tok" || o.Host != "127.0.0.1" || o.OnConnectRefused == nil {
		t.Errorf("android options = %+v", o)
	}
	ios := &DeviceContext{Platform: device.PlatformIOS, Port: 48792}
	if o := ios.reconnectDialOptions("tok"); o.OwnAdbForward {
		t.Error("iOS options claim an adb forward")
	}
}

// N5: `open link` can start a stopped app, so after a kill it clears the stopped state when it works; a link that failed to open does not.
func TestOpenLinkLaunchesAStoppedApp(t *testing.T) {
	e := &Executor{appStopped: true}
	e.noteAppState(parser.ActionStep{Verb: parser.VerbOpenLink}, errors.New("failed"))
	if !e.appStopped {
		t.Error("a link that failed to open cleared the state")
	}
	e.noteAppState(parser.ActionStep{Verb: parser.VerbOpenLink}, nil)
	if e.appStopped {
		t.Error("a link that opened left the app marked as stopped")
	}
}

// N7: a kill step that fails, run through runStep, leaves the app unmarked, and a kill that works after it marks it.
func TestAFailingKillThroughRunStepLeavesTheAppUnmarked(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}
	e := failingAdbExecutor(t, a)
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbKill}); err == nil {
		t.Fatal("the kill did not fail (the premise of the test)")
	}
	if e.appStopped {
		t.Error("a kill that failed marked the app as stopped")
	}
	e.appStopped = true // an earlier kill that worked
	_ = e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbKill})
	if !e.appStopped {
		t.Error("a failing second kill cleared the state of the first")
	}
}
