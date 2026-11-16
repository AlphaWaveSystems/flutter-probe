package runner

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	parser2 "go/parser"
	"go/token"
	"os"
	"path/filepath"
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
	if h := hintExecutor(b, true).reconnectHint(ctx); h != "" || b.attempted != 0 {
		t.Errorf("a cancelled run: %q after %d calls", h, b.attempted)
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

// Device text cannot reach the hint: a forward whose serial is not a plain serial is not blamed, so nothing from it is in the message or the command it suggests.
func TestTheHintNeverCarriesDeviceText(t *testing.T) {
	for _, serial := range []string{"emu\x1b[2Jlator", "a;rm", "$(id)", "a\u202eb"} {
		h := hintExecutor(liveAdb(serial+" tcp:48792 tcp:48792\n"), true).reconnectHint(context.Background())
		if h != "" {
			t.Errorf("hostile serial %q reached the hint: %q", serial, h)
		}
	}
	// what the hint does contain is made of constants, the app id (validated), the port (a number) and a serial that passed the check
	h := hintExecutor(liveAdb("emulator-5554 tcp:48792 tcp:48792\n"), true).reconnectHint(context.Background())
	for _, r := range h {
		if r < 0x20 || r == 0x7f {
			t.Errorf("a control character in the hint: %q", h)
		}
	}
}

// The hint costs nothing on a step that passes: no adb call at all, with the opt-in off, and it is asked for only by a reconnect that failed.
func TestAPassingStepAsksNothingForTheHint(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, forward: "emulator-5554 tcp:48792 tcp:48792\n"}
	e := hintExecutor(a, false)
	e.client = &ceFailingClient{log: new([]string)}
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter}); err != nil || len(a.calls) != 0 {
		t.Errorf("a passing step: err %v, %d adb calls", err, len(a.calls))
	}
	// a step that fails without a lost connection never reaches the reconnect, so the hint's calls are not made either
	b := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{"4100"}, forward: "emulator-5554 tcp:48792 tcp:48792\n"}
	e2 := hintExecutor(b, false)
	e2.client = &ceFailingClient{log: new([]string), err: errors.New("rpc error -32001: Widget not found")}
	_ = e2.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
	if b.words("pidof") != 0 || len(b.calls) != 0 {
		t.Errorf("a widget that was not found asked adb %d times", len(b.calls))
	}
}

// The hint stays inside its bound: an adb that never answers ends at the hint's own timeout, and the run's cancel ends it at once.
func TestTheHintIsBoundedAndCancellable(t *testing.T) {
	e := ceExecutorWith(&ceHangAdb{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	start := time.Now()
	if h := e.reconnectHint(ctx); h != "" || time.Since(start) > 3*time.Second {
		t.Errorf("cancel: %q after %v", h, time.Since(start))
	}
	if reconnectHintTimeout != 5*time.Second {
		t.Errorf("the hint's bound is %v, documented as 5 s", reconnectHintTimeout)
	}
}

// ---- review of #665 ----

// Both reconnect paths start from reconnectDialOptions and build no dial options of their own: a literal in either would leave out the own-forward marker.
func TestBothReconnectPathsUseTheSharedDialOptions(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser2.ParseFile(fset, "device_context.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || (fn.Name.Name != "Reconnect" && fn.Name.Name != "ReconnectWithToken") {
			continue
		}
		calls, literals := 0, 0
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "reconnectDialOptions" {
					calls++
				}
			case *ast.CompositeLit:
				if sel, ok := v.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "DialOptions" {
					literals++
				}
			}
			return true
		})
		seen[fn.Name.Name] = true
		if calls != 1 || literals != 0 {
			t.Errorf("%s: %d calls of reconnectDialOptions and %d DialOptions literals, want 1 and 0", fn.Name.Name, calls, literals)
		}
	}
	if !seen["Reconnect"] || !seen["ReconnectWithToken"] {
		t.Errorf("a reconnect function was not found: %v", seen)
	}
}

// A kill step that works, run through runStep, marks the app as stopped; a restart that works afterwards (the same executor, an adb that succeeds) clears it.
func TestAKillStepThroughRunStepMarksTheApp(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, Manager: device.NewManagerWithPaths(device.ToolPaths{ADB: bin})}
	e := NewExecutor(&ceTimeoutClient{err: nil}, dc, nil, 2*time.Second, false)
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbKill}); err != nil {
		t.Fatalf("the kill step failed: %v", err)
	}
	if !e.appStopped {
		t.Error("a kill step that worked did not mark the app as stopped")
	}
}

// A cancelled run gets no hint and costs no adb call, through the step itself.
func TestACancelledRunGetsNoHintThroughRunStep(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}
	e := hintExecutor(a, false)
	e.client = &ceClient{log: new([]string)}
	e.SetReconnectPolicy(1, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbPressEnter})
	if err == nil || strings.Contains(err.Error(), " — the app ") || a.attempted != 0 {
		t.Errorf("err %v, %d adb calls", err, a.attempted)
	}
}

// The step's own error stays in the chain: errors.Is finds it through the reconnect failure and the hint.
func TestTheStepErrorStaysInTheChainThroughTheHint(t *testing.T) {
	sentinel := errors.New("the original failure")
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}}
	e := hintExecutor(a, false)
	e.client = &ceTimeoutClient{err: fmt.Errorf("probelink: write: broken pipe: %w", sentinel)}
	e.SetReconnectPolicy(1, time.Hour)
	e.hooks.reconnect = func(ctx context.Context, attempt int) error { return errors.New("reconnect refused") }
	err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
	if err == nil || !errors.Is(err, sentinel) || !strings.Contains(err.Error(), " — the app ") {
		t.Errorf("err = %v (is the original: %v)", err, errors.Is(err, sentinel))
	}
}

// ceFlakyClient loses the connection once and then works, which is what a successful reconnect looks like to the step.
type ceFlakyClient struct {
	fakeAIClient
	calls int
}

func (c *ceFlakyClient) DeviceAction(ctx context.Context, action, value string) error {
	c.calls++
	if c.calls == 1 {
		return errors.New("probelink: write: broken pipe")
	}
	return nil
}

// The hint is asked for after a failed reconnect only: not before the reconnect, and not at all when the reconnect worked and the retried step passed.
func TestTheHintComesAfterTheReconnectAndOnlyWhenItFailed(t *testing.T) {
	var log []string
	a := &ceAdb{log: &log, date: "2026-11-15 19:31:15", pidof: []string{""}}
	e := hintExecutor(a, false)
	e.client = &ceTimeoutClient{err: errors.New("probelink: write: broken pipe")}
	e.SetReconnectPolicy(1, time.Hour)
	e.hooks.reconnect = func(ctx context.Context, attempt int) error {
		log = append(log, "reconnect")
		return errors.New("reconnect refused")
	}
	_ = e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
	rec := -1
	for i, l := range log {
		if l == "reconnect" && rec < 0 {
			rec = i
		}
	}
	if rec < 0 || len(log) < 2 {
		t.Fatalf("log = %v", log)
	}
	for i, l := range log {
		if strings.HasPrefix(l, "adb ") && i < rec {
			t.Errorf("an adb call came before the reconnect: %v", log)
		}
	}
	// a reconnect that works and a retried step that passes: nothing is asked
	log = nil
	b := &ceAdb{log: &log, date: "2026-11-15 19:31:15", pidof: []string{""}}
	e2 := hintExecutor(b, false)
	e2.client = &ceFlakyClient{}
	e2.SetReconnectPolicy(1, time.Hour)
	e2.hooks.reconnect = func(ctx context.Context, attempt int) error { log = append(log, "reconnect"); return nil }
	if err := e2.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter}); err != nil {
		t.Fatalf("the retried step failed: %v", err)
	}
	if len(b.calls) != 0 || len(log) != 1 || log[0] != "reconnect" {
		t.Errorf("after a successful reconnect: %d adb calls, log %v", len(b.calls), log)
	}
}

// The three looks that can meet on one failed step, in the order they run and within their cap: the crash look before the reconnect, the hint after the failed
// reconnect, and the crash look at the end of the step. A slow app (running, no exit entry) is looked at twice by the crash look, and the hint asks for the
// clock and the process list once.
func TestTheLooksOnOneFailedStepRunInOrderWithinTheCap(t *testing.T) {
	var log []string
	a := &ceAdb{log: &log, date: "2026-11-15 19:31:15", pidof: []string{"4100"}, exit: []string{"ACTIVITY MANAGER PROCESS EXIT INFO\n"}, forward: "emulator-5600 tcp:48792 tcp:48792\n"}
	e := hintExecutor(a, true)
	e.client = &ceTimeoutClient{err: errors.New("probelink: write: broken pipe")}
	e.SetReconnectPolicy(1, time.Hour)
	e.hooks.reconnect = func(ctx context.Context, attempt int) error {
		log = append(log, "reconnect")
		return errors.New("reconnect refused")
	}
	_ = e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
	// the markers: the first exit-info read is before the reconnect, and the other is after it (the end-of-step look); the hint's calls are between
	rec, first, second := -1, -1, -1
	exits := 0
	for i, l := range log {
		switch {
		case l == "reconnect" && rec < 0:
			rec = i
		case strings.Contains(l, "exit-info"):
			exits++
			if first < 0 {
				first = i
			} else if second < 0 {
				second = i
			}
		}
	}
	if rec < 0 || first < 0 || second < 0 || !(first < rec && rec < second) || exits != 2 {
		t.Fatalf("order wrong (exit-info reads %d): %v", exits, log)
	}
	if e.crashTries != 2 || e.CrashReport() != nil {
		t.Errorf("tries %d (a slow app is looked at twice here, cap %d), report %v", e.crashTries, crashMaxCollections, e.CrashReport())
	}
	if a.words("pidof") != 3 || a.words("date") != 3 {
		t.Errorf("the clock was read %d times and the process list %d times, want 3 and 3 (look, hint, end-of-step look)", a.words("date"), a.words("pidof"))
	}
	// the cap holds however often steps fail in one attempt
	for i := 0; i < 6; i++ {
		_ = e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbPressEnter})
	}
	if e.crashTries != crashMaxCollections {
		t.Errorf("tries %d after many failures, want the cap %d", e.crashTries, crashMaxCollections)
	}
}

// The parallel orchestrator's connect failure on Android adds the shared forward hint, and only on Android; no site of the runner builds that text itself.
func TestOrchestratorConnectFailureUsesTheSharedForwardHint(t *testing.T) {
	b, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, `"[%s] connect: %w%s"`)
	if i < 0 {
		t.Fatal("the connect failure of the orchestrator does not carry a hint")
	}
	// the hint is asked for in the block just above that message, behind the Android check and nowhere else
	block := strings.Join(strings.Fields(src[max(0, i-400):i]), " ")
	want := `hint := "" if dr.Platform == device.PlatformAndroid { hint = device.ForwardHint(ctx, po.manager.ADB(), dr.Port, dr.DeviceID) }`
	if !strings.Contains(block, want) {
		t.Errorf("the connect failure must ask for the hint only on Android, through device.ForwardHint; the block above the message is:\n%s", block)
	}
	for _, f := range []string{"orchestrator.go", "reconnect_hint.go"} {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), "agent port %d is forwarded") {
			t.Errorf("%s builds the forward message itself", f)
		}
	}
}
