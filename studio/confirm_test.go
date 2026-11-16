package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// confirmApp is a connected App with a page listening on the hub, and a run in progress.
type confirmApp struct {
	a      *App
	dc     *runner.DeviceContext
	events chan hubEvent
	run    context.Context
	cancel context.CancelFunc
}

func newConfirmApp(t *testing.T, withPage bool) *confirmApp {
	t.Helper()
	a := &App{hub: newEventHub()}
	dc := &runner.DeviceContext{AppID: "com.example.app\u202e", Platform: device.PlatformAndroid, Serial: "emu-1"}
	a.conn = &connection{deviceName: "Pixel\nTest " + strings.Repeat("x", 200), deviceCtx: dc, cfg: &config.Config{}}
	c := &confirmApp{a: a, dc: dc}
	if withPage {
		ch, unsub := a.hub.subscribe()
		t.Cleanup(unsub)
		c.events = ch
	}
	c.run, c.cancel = context.WithCancel(context.Background())
	a.setConfirmRun(c.run)
	t.Cleanup(c.cancel)
	return c
}

// next reads the next event of this name from the page's feed.
func (c *confirmApp) next(t *testing.T, name string) []any {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-c.events:
			if ev.Name == name {
				return ev.Data
			}
		case <-deadline:
			t.Fatalf("no %s event", name)
		}
	}
}

// ask runs the step's callback in the background and returns where its answer will be.
func (c *confirmApp) ask() chan bool {
	out := make(chan bool, 1)
	go func() { out <- c.a.confirmClearData(c.dc)("the engine's own text: EVIL") }()
	return out
}

func answerOf(t *testing.T, ch chan bool) bool {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("the step never got an answer")
		return false
	}
}

func TestConfirmAllowAndDenyFollowTheWindowsAnswer(t *testing.T) {
	for _, allow := range []bool{true, false} {
		c := newConfirmApp(t, true)
		out := c.ask()
		req := c.next(t, "confirm:request")[0].(ConfirmRequest)
		if !c.a.AnswerConfirm(req.ID, allow) {
			t.Fatalf("the answer was not accepted (allow=%v)", allow)
		}
		if got := answerOf(t, out); got != allow {
			t.Errorf("the step got %v, the window said %v", got, allow)
		}
		c.next(t, "confirm:close")
	}
}

// What the window shows is Studio's own: the engine's text never reaches it, and a name is one clean bounded line.
func TestConfirmRequestIsBuiltFromStudiosStateOnly(t *testing.T) {
	c := newConfirmApp(t, true)
	out := c.ask()
	req := c.next(t, "confirm:request")[0].(ConfirmRequest)
	c.a.AnswerConfirm(req.ID, false)
	answerOf(t, out)
	if req.Kind != "clear-app-data" || req.AppID != "com.example.app" {
		t.Errorf("request = %+v", req)
	}
	for _, v := range []string{req.AppID, req.DeviceName, req.Kind} {
		if strings.Contains(v, "EVIL") || strings.ContainsAny(v, "\n\u202e") {
			t.Errorf("a field carries engine text or an invisible character: %q", v)
		}
	}
	if len([]rune(req.DeviceName)) > maxConfirmTxt+1 || !strings.HasPrefix(req.DeviceName, "Pixel Test ") {
		t.Errorf("device name = %q", req.DeviceName)
	}
	if len(req.ID) != 32 {
		t.Errorf("the id is %d characters, want 32 hex digits (128 bits)", len(req.ID))
	}
}

func TestConfirmIdsAreRandomAndUsedOnce(t *testing.T) {
	c := newConfirmApp(t, true)
	out := c.ask()
	first := c.next(t, "confirm:request")[0].(ConfirmRequest)
	if c.a.AnswerConfirm("not-the-id", true) || c.a.AnswerConfirm("", true) || c.a.AnswerConfirm(strings.ToUpper(first.ID), true) {
		t.Error("a wrong id was accepted")
	}
	if !c.a.AnswerConfirm(first.ID, false) {
		t.Fatal("the right id was refused")
	}
	if c.a.AnswerConfirm(first.ID, true) { // a second window, or a repeat
		t.Error("the same id was accepted twice")
	}
	if got := answerOf(t, out); got {
		t.Error("a repeat answer turned a no into a yes")
	}
	// a second question asks again, with another id, and the old id does not answer it
	out2 := c.ask()
	second := c.next(t, "confirm:request")[0].(ConfirmRequest)
	if second.ID == first.ID {
		t.Error("two requests share an id")
	}
	if c.a.AnswerConfirm(first.ID, true) {
		t.Error("an old id answered a new question")
	}
	c.a.AnswerConfirm(second.ID, true)
	if !answerOf(t, out2) {
		t.Error("the second question did not take its own answer")
	}
}

// A second `clear app data` in the same run asks again: there is no memory of a yes.
func TestConfirmAsksEveryTimeAndOneAtATime(t *testing.T) {
	c := newConfirmApp(t, true)
	first, second := c.ask(), c.ask()
	req1 := c.next(t, "confirm:request")[0].(ConfirmRequest)
	select {
	case ev := <-c.events:
		if ev.Name == "confirm:request" {
			t.Fatal("a second question was shown while the first was open")
		}
	case <-time.After(150 * time.Millisecond):
	}
	c.a.AnswerConfirm(req1.ID, true)
	req2 := c.next(t, "confirm:request")[0].(ConfirmRequest)
	if req2.ID == req1.ID {
		t.Error("the second step reused the first answer")
	}
	c.a.AnswerConfirm(req2.ID, false)
	got := []bool{answerOf(t, first), answerOf(t, second)}
	if !(got[0] != got[1]) {
		t.Errorf("answers = %v, want one yes and one no", got)
	}
}

func TestConfirmTimesOutAsADenyAndSaysSo(t *testing.T) {
	old := confirmTimeout
	confirmTimeout = 60 * time.Millisecond
	defer func() { confirmTimeout = old }()
	c := newConfirmApp(t, true)
	out := c.ask()
	req := c.next(t, "confirm:request")[0].(ConfirmRequest)
	if answerOf(t, out) {
		t.Error("a question nobody answered was a yes")
	}
	c.next(t, "confirm:close")
	note := c.next(t, "confirm:note")[0].(string)
	if !strings.Contains(note, "Clear app data was not run: nothing answered in the window") {
		t.Errorf("note = %q", note)
	}
	if c.a.AnswerConfirm(req.ID, true) {
		t.Error("a late answer was accepted after the timeout")
	}
}

func TestConfirmIsClosedAndDeniedWhenTheRunIsCancelled(t *testing.T) {
	c := newConfirmApp(t, true)
	out := c.ask()
	req := c.next(t, "confirm:request")[0].(ConfirmRequest)
	c.cancel()
	if answerOf(t, out) {
		t.Error("a cancelled run got a yes")
	}
	if id := c.next(t, "confirm:close")[0].(string); id != req.ID {
		t.Errorf("the dialog that closed was %q, want %q", id, req.ID)
	}
	if !strings.Contains(c.next(t, "confirm:note")[0].(string), "run was cancelled") {
		t.Error("no note for the cancel")
	}
	if c.a.AnswerConfirm(req.ID, true) {
		t.Error("an answer after the cancel was accepted")
	}
}

// An answer is bound to the run that asked: after that run ended, a yes for its id is nothing.
func TestConfirmAnswerIsBoundToItsRun(t *testing.T) {
	c := newConfirmApp(t, true)
	out := c.ask()
	req := c.next(t, "confirm:request")[0].(ConfirmRequest)
	other, otherCancel := context.WithCancel(context.Background())
	defer otherCancel()
	c.a.setConfirmRun(other) // a different run is current now
	c.cancel()               // the one that asked is over
	if c.a.AnswerConfirm(req.ID, true) {
		t.Error("the answer of a finished run was accepted")
	}
	if answerOf(t, out) {
		t.Error("the step of a finished run got a yes")
	}
}

// With nobody to ask (no window, no page, no run, no connection) the step is not run, and nothing waits.
func TestConfirmWithNobodyToAskDenies(t *testing.T) {
	noPage := newConfirmApp(t, false)
	start := time.Now()
	if noPage.a.confirmClearData(noPage.dc)("x") {
		t.Error("a headless Studio with no page allowed a wipe")
	}
	if time.Since(start) > time.Second {
		t.Error("the step waited for a window that is not there")
	}
	noRun := newConfirmApp(t, true)
	noRun.a.clearConfirmRun()
	if noRun.a.confirmClearData(noRun.dc)("x") {
		t.Error("a step outside a run was allowed")
	}
	noConn := newConfirmApp(t, true)
	noConn.a.mu.Lock()
	noConn.a.conn = nil
	noConn.a.mu.Unlock()
	if noConn.a.confirmClearData(noConn.dc)("x") {
		t.Error("a step with no connection was allowed")
	}
	if (&App{}).AnswerConfirm("anything", true) {
		t.Error("an answer with nothing pending was accepted")
	}
}

// The Studio device context asks (it has a Confirm) and never skips the question (AllowClearData stays off): `clear app data` with nobody to
// ask is refused, and with a yes it reaches the device; with a no it does not.
func TestStudioClearAppDataReachesTheDeviceOnlyOnAYes(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "adb.log")
	script := filepath.Join(dir, "adb")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr := device.NewManagerWithPaths(device.ToolPaths{ADB: script})
	for _, tc := range []struct {
		name   string
		answer *bool
		wipes  bool
	}{{"yes", ptr(true), true}, {"no", ptr(false), false}, {"nobody", nil, false}} {
		_ = os.Remove(log)
		a := &App{hub: newEventHub(), deviceMgr: mgr, ctx: context.Background()}
		cfg := &config.Config{}
		cfg.Project.App = "com.example.app"
		dc := newStudioDeviceContext(a, cfg, device.PlatformAndroid, "emu-1", 48686, "")
		if dc.AllowClearData || dc.Confirm == nil {
			t.Fatalf("%s: AllowClearData=%v Confirm set=%v", tc.name, dc.AllowClearData, dc.Confirm != nil)
		}
		a.conn = &connection{deviceName: "Pixel", deviceCtx: dc, cfg: cfg}
		run, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		a.setConfirmRun(run)
		var wg sync.WaitGroup
		if tc.answer != nil {
			ch, unsub := a.hub.subscribe()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for ev := range ch {
					if ev.Name == "confirm:request" {
						a.AnswerConfirm(ev.Data[0].(ConfirmRequest).ID, *tc.answer)
						return
					}
				}
			}()
			defer unsub()
		}
		err := dc.ClearAppData(run)
		wg.Wait()
		cancel()
		out, _ := os.ReadFile(log)
		wiped := strings.Contains(string(out), "pm clear")
		if wiped != tc.wipes {
			t.Errorf("%s: pm clear ran = %v, want %v (err %v, adb log %q)", tc.name, wiped, tc.wipes, err, out)
		}
		if !tc.wipes && (err == nil || !strings.Contains(err.Error(), "clear app data")) {
			t.Errorf("%s: err = %v, want a refusal", tc.name, err)
		}
		if !tc.wipes && strings.Contains(err.Error(), "--yes") {
			t.Errorf("%s: the refusal tells a Studio user to pass --yes to probe test: %v", tc.name, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// The question is about the device its step would wipe. If the window moved to another connection since the run started, the step is not run
// and no dialog names the other device.
func TestConfirmForAConnectionThatIsNoLongerCurrentDeniesAndNamesNothing(t *testing.T) {
	c := newConfirmApp(t, true)
	// the window connected to device B while the run (started on A) goes on
	other := &runner.DeviceContext{AppID: "com.other.app", Platform: device.PlatformAndroid, Serial: "emu-2"}
	c.a.mu.Lock()
	c.a.conn = &connection{deviceName: "Device B", deviceCtx: other, cfg: &config.Config{}}
	c.a.mu.Unlock()
	start := time.Now()
	if c.a.confirmClearData(c.dc)("x") {
		t.Error("a step of the old connection was allowed after the window moved to another")
	}
	if time.Since(start) > time.Second {
		t.Error("the step waited for an answer about a device it is not on")
	}
	select {
	case ev := <-c.events:
		if ev.Name == "confirm:request" {
			t.Errorf("a dialog was shown for a step of another connection: %+v", ev.Data)
		}
	case <-time.After(100 * time.Millisecond):
	}
	// and the step of the connection that IS current names that connection's device and app
	out := make(chan bool, 1)
	go func() { out <- c.a.confirmClearData(other)("x") }()
	req := c.next(t, "confirm:request")[0].(ConfirmRequest)
	if req.DeviceName != "Device B" || req.AppID != "com.other.app" {
		t.Errorf("request = %+v, want Device B and com.other.app", req)
	}
	c.a.AnswerConfirm(req.ID, false)
	answerOf(t, out)
}

// Connect refuses while a run is going: the run keeps its device, and a second device would be named by the dialog and not wiped.
func TestConnectIsRefusedWhileARunIsActive(t *testing.T) {
	a := &App{ctx: context.Background()}
	_, cancel, err := a.claimRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Connect("some-device"); err == nil || !strings.Contains(err.Error(), "a run is in progress") {
		t.Errorf("Connect during a run = %v", err)
	}
	if _, err := a.ConnectWiFi("192.168.1.20", 48686, "token"); err == nil || !strings.Contains(err.Error(), "a run is in progress") {
		t.Errorf("ConnectWiFi during a run = %v", err)
	}
	a.releaseRun(cancel)
	if _, err := a.Connect(""); err == nil || strings.Contains(err.Error(), "a run is in progress") {
		t.Errorf("after the run, Connect says %v", err)
	}
}

// The bridge of the headless build refuses AnswerConfirm (a local script must not approve a wipe), unless the test server says its page is the window.
func TestAnswerConfirmIsRefusedOverHTTPUnlessTheTestEnvSaysSo(t *testing.T) {
	t.Setenv("PROBE_STUDIO_E2E_ALLOW_CONFIRM_OVER_HTTP", "")
	if !methodRefusedOverHTTP("AnswerConfirm") || !methodRefusedOverHTTP("PickWorkspace") {
		t.Error("the bridge allows a method that needs the window")
	}
	if _, err := callAppMethod(&App{}, "AnswerConfirm", nil); err == nil || !strings.Contains(err.Error(), "not available without a window") {
		t.Errorf("AnswerConfirm over the bridge = %v", err)
	}
	t.Setenv("PROBE_STUDIO_E2E_ALLOW_CONFIRM_OVER_HTTP", "1")
	if methodRefusedOverHTTP("AnswerConfirm") {
		t.Error("the test server's environment did not open AnswerConfirm")
	}
	if !methodRefusedOverHTTP("PickWorkspace") {
		t.Error("the test environment opened more than AnswerConfirm")
	}
	t.Setenv("PROBE_STUDIO_E2E_ALLOW_CONFIRM_OVER_HTTP", "true")
	if !methodRefusedOverHTTP("AnswerConfirm") {
		t.Error("anything but 1 opened AnswerConfirm")
	}
}

// A real run sets the run the question belongs to: a `clear app data` step of executeRun is asked about, and a yes reaches the device.
func TestExecuteRunSetsTheRunItsConfirmationsBelongTo(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "adb.log")
	script := filepath.Join(dir, "adb")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  clear app data\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub(), deviceMgr: device.NewManagerWithPaths(device.ToolPaths{ADB: script})}
	cfg := &config.Config{}
	cfg.Project.App = "com.example.app"
	cfg.Defaults.Timeout = 5 * time.Second
	cfg.Agent.TokenReadTimeout = 200 * time.Millisecond // the agent of this fake device never comes back: the reconnect after the wipe gives up soon
	cfg.Agent.DialTimeout = 200 * time.Millisecond
	cfg.Agent.ReconnectDelay = time.Millisecond
	cfg.Agent.ReconnectAttempts = 1
	dc := newStudioDeviceContext(a, cfg, device.PlatformAndroid, "emu-1", 48686, "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	conn := &connection{client: &runFake{}, deviceName: "Pixel", deviceCtx: dc, cfg: cfg, streamCancel: cancel, streamDone: done}
	a.conn = conn
	go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})
	events, unsub := a.hub.subscribe()
	defer unsub()
	asked := make(chan ConfirmRequest, 1)
	go func() {
		for ev := range events {
			if ev.Name == "confirm:request" {
				req := ev.Data[0].(ConfirmRequest)
				asked <- req
				a.AnswerConfirm(req.ID, true)
				return
			}
		}
	}()
	_, _ = a.executeRun(context.Background(), file, runSelection{}, RunPlan{})
	select {
	case req := <-asked:
		if req.AppID != "com.example.app" || req.DeviceName != "Pixel" {
			t.Errorf("the question = %+v", req)
		}
	default:
		t.Fatal("the run's `clear app data` step never asked: the run did not set the run its confirmations belong to")
	}
	if out, _ := os.ReadFile(log); !strings.Contains(string(out), "pm clear") {
		t.Errorf("a yes did not reach the device (adb log %q)", out)
	}
	a.mu.Lock()
	left := a.confirmRun
	cancelNow, doneNow := conn.streamCancel, conn.streamDone
	a.mu.Unlock()
	if left != nil {
		t.Error("the finished run is still the run confirmations belong to")
	}
	cancelNow()
	<-doneNow
}
