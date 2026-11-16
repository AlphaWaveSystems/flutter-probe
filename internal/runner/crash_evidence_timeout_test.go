package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// ceTimeoutClient fails every device action with the error it was given and counts how often the connection is closed, which a reconnect attempt does first.
type ceTimeoutClient struct {
	fakeAIClient
	err    error
	closes int
}

func (c *ceTimeoutClient) DeviceAction(ctx context.Context, action, value string) error { return c.err }
func (c *ceTimeoutClient) Close() error                                                 { c.closes++; return nil }

const (
	ceSignaled = "          timestamp=2026-11-15 19:31:09.870 pid=3567 realUid=10207 packageUid=10207 definingUid=10207 user=0\n          process=dev.probe.webviewfixture reason=2 (SIGNALED) subreason=0 status=9\n"
)

func ceSignalEntry(ts, reason string) string {
	return "        ApplicationExitInfo #0:\n" + strings.Replace(strings.Replace(ceSignaled, "2026-11-15 19:31:09.870", ts, 1), "reason=2", "reason="+reason, 1) + "          importance=100 description=x state=empty\n"
}

func timeoutErr() error {
	return errors.New("rpc error -32000: step timed out after 30s: context deadline exceeded")
}

func runTimeoutStep(t *testing.T, on bool, a *ceAdb, err error, step parser.Step) (*Executor, *ceTimeoutClient, error) {
	t.Helper()
	e := ceExecutor(a, on)
	c := &ceTimeoutClient{err: err}
	e.client = c
	e.SetReconnectPolicy(1, time.Hour)
	e.timeout = 80 * time.Millisecond
	return e, c, e.runStep(context.Background(), step)
}

var pressEnter = parser.ActionStep{Verb: parser.VerbPressEnter}

// The app was killed during a wait: the step fails with a timeout, not a lost connection, and the evidence is still found.
func TestAKilledAppIsFoundAfterATimeout(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
	e, c, err := runTimeoutStep(t, true, a, timeoutErr(), pressEnter)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("the step's own error changed: %v", err)
	}
	r := e.CrashReport()
	if r == nil || r.Kind != device.ExitSignaled || !r.Gone || r.Evidence.Exit == nil || r.Evidence.Exit.PID != 3567 || r.Evidence.Exit.Status != 9 {
		t.Fatalf("report = %+v", r)
	}
	if !strings.HasPrefix(r.Line(), "the app died during this test: signaled (pid 3567)") {
		t.Errorf("line = %q", r.Line())
	}
	if c.closes != 0 {
		t.Errorf("a timeout started %d reconnect attempts", c.closes)
	}
}

// Whatever the step failed with, the opt-in changes neither the error nor the reconnect behaviour: only the evidence is added.
func TestTheOptInChangesNoOutcome(t *testing.T) {
	for name, stepErr := range map[string]error{
		"a timeout":         timeoutErr(),
		"a missing widget":  errors.New("rpc error -32001: Widget not found: text(\"Login\")"),
		"a lost connection": errors.New("probelink: write: broken pipe"),
	} {
		dead := func() *ceAdb {
			return &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
		}
		_, c1, err1 := runTimeoutStep(t, false, dead(), stepErr, pressEnter)
		e2, c2, err2 := runTimeoutStep(t, true, dead(), stepErr, pressEnter)
		if err1 == nil || err2 == nil || err1.Error() != err2.Error() {
			t.Errorf("%s: error off %q, on %q", name, err1, err2)
		}
		if c1.closes != c2.closes {
			t.Errorf("%s: reconnect attempts off %d, on %d", name, c1.closes, c2.closes)
		}
		if e2.CrashReport() == nil {
			t.Errorf("%s: no evidence with the opt-in on", name)
		}
	}
}

// A slow app that is still running, with no new exit entry, reports nothing, and costs only the reads.
func TestASlowAppIsNotReportedAfterATimeout(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{"3567"}, exit: []string{"ACTIVITY MANAGER PROCESS EXIT INFO\n"}}
	e, _, err := runTimeoutStep(t, true, a, timeoutErr(), pressEnter)
	if err == nil || e.CrashReport() != nil {
		t.Fatalf("err %v, report %v", err, e.CrashReport())
	}
	if len(a.calls) != 3 || a.words("logcat") != 0 {
		t.Errorf("%d adb calls, want date, exit-info and pidof", len(a.calls))
	}
}

// The test's own force-stop is not a death, whether the app is running again or not.
func TestATestsOwnForceStopIsNotADeath(t *testing.T) {
	for name, pids := range map[string][]string{"app running again": {"4100"}, "app not running": {""}} {
		a := &ceAdb{date: "2026-11-15 19:31:15", pidof: pids, exit: []string{ceSignalEntry("2026-11-15 19:31:12.000", "10")}}
		e := ceExecutor(a, true)
		e.appStopped = true // the test's own kill step
		e.client = &ceTimeoutClient{err: timeoutErr()}
		e.SetReconnectPolicy(1, time.Hour)
		e.timeout = 80 * time.Millisecond
		err := e.runStep(context.Background(), pressEnter)
		if err == nil || e.CrashReport() != nil {
			t.Errorf("%s: err %v, report %+v", name, err, e.CrashReport())
		}
	}
}

// Any step error is looked at once, not only timeouts: a widget that is not found because the app is gone is explained too.
func TestAnyStepErrorGetsOneLook(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
	e, _, _ := runTimeoutStep(t, true, a, errors.New("rpc error -32001: Widget not found: text(\"Login\")"), pressEnter)
	if e.CrashReport() == nil || e.CrashReport().Kind != device.ExitSignaled {
		t.Errorf("report = %+v", e.CrashReport())
	}
}

// With the opt-in off, or when nothing failed, or when the failure does not end the step, there is no adb call at all.
func TestNoLookWithoutAFailureOrTheOptIn(t *testing.T) {
	dead := func() *ceAdb {
		return &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
	}
	a := dead()
	if _, _, err := runTimeoutStep(t, false, a, timeoutErr(), pressEnter); err == nil || len(a.calls) != 0 {
		t.Errorf("opt-in off: err %v, %d adb calls", err, len(a.calls))
	}
	a = dead()
	if _, _, err := runTimeoutStep(t, true, a, nil, pressEnter); err != nil || len(a.calls) != 0 {
		t.Errorf("a passing step: err %v, %d adb calls", err, len(a.calls))
	}
	a = dead()
	opt := parser.ActionStep{Verb: parser.VerbPressEnter, Optional: true}
	if _, _, err := runTimeoutStep(t, true, a, timeoutErr(), opt); err != nil || len(a.calls) != 0 {
		t.Errorf("an optional step that failed and went on: err %v, %d adb calls", err, len(a.calls))
	}
	// a cancelled run (Ctrl-C) is not a crash to explain
	a = dead()
	e := ceExecutor(a, true)
	e.client = &ceTimeoutClient{err: timeoutErr()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = e.runStep(ctx, pressEnter)
	if len(a.calls) != 0 || e.CrashReport() != nil {
		t.Errorf("a cancelled run made %d adb calls", len(a.calls))
	}
}

// One failed step ends the attempt, so the look happens once; a second failing step in the same attempt (a hook, say) looks at most crashMaxCollections times.
func TestTheLookIsBoundedPerAttempt(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{"3567"}, exit: []string{"x\n"}}
	e := ceExecutor(a, true)
	e.client = &ceTimeoutClient{err: timeoutErr()}
	for i := 0; i < 6; i++ {
		_ = e.runStep(context.Background(), pressEnter)
	}
	if a.words("date") != crashMaxCollections {
		t.Errorf("%d looks, want %d", a.words("date"), crashMaxCollections)
	}
}

// The look needs a live context, and says nothing when it is given an expired one: a caller that ends an attempt on a timeout passes a fresh bounded context.
func TestAnExpiredContextGetsNothingAndAFreshOneWorks(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
	e := ceExecutor(a, true)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	e.collectCrashOnce(expired)
	if e.CrashReport() != nil {
		t.Fatalf("an expired context found evidence: %+v", e.CrashReport())
	}
	// the same with an adb that honours the context while it waits
	h := &ceHangAdb{}
	e2 := ceExecutorWith(h)
	start := time.Now()
	e2.collectCrashOnce(expired)
	if time.Since(start) > 3*time.Second || e2.CrashReport() != nil {
		t.Errorf("expired context: took %v, report %v", time.Since(start), e2.CrashReport())
	}
	// a fresh bounded context, derived from the run's, works on the same executor
	fresh, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	e.collectCrashOnce(fresh)
	if e.CrashReport() == nil || e.CrashReport().Kind != device.ExitSignaled {
		t.Errorf("a fresh context found nothing: %+v", e.CrashReport())
	}
}

// failingAdbExecutor is an executor whose device manager runs an adb that always fails, so a restart, kill or clear step fails, while the crash look has its own
// recording adb.
func failingAdbExecutor(t *testing.T, crash *ceAdb) *Executor {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho failure >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dc := &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: true, CrashADB: crash, CrashPoll: time.Millisecond,
		Manager: device.NewManagerWithPaths(device.ToolPaths{ADB: bin}), AllowClearData: true, RestartDelay: time.Millisecond}
	e := NewExecutor(&ceTimeoutClient{err: nil}, dc, nil, 2*time.Second, false)
	e.startedAt = time.Now()
	return e
}

// A restart, kill or clear step that fails is never looked at: it stops or restarts the app on purpose, so an app with no process is expected there.
func TestAFailingLifecycleStepMakesNoCrashLook(t *testing.T) {
	for name, verb := range map[string]parser.ActionVerb{"restart": parser.VerbRestart, "kill": parser.VerbKill, "clear app data": parser.VerbClearAppData} {
		a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
		e := failingAdbExecutor(t, a)
		err := e.runStep(context.Background(), parser.ActionStep{Verb: verb})
		if err == nil {
			t.Fatalf("%s: the step did not fail (the premise of the test)", name)
		}
		if len(a.calls) != 0 || e.CrashReport() != nil {
			t.Errorf("%s: %d crash-look adb calls after a failing lifecycle step", name, len(a.calls))
		}
	}
	// the control: the same executor and a failing step that is not a lifecycle step does look
	a := &ceAdb{date: "2026-11-15 19:31:15", pidof: []string{""}, exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}}
	e := failingAdbExecutor(t, a)
	e.client = &ceTimeoutClient{err: timeoutErr()}
	if err := e.runStep(context.Background(), pressEnter); err == nil || a.words("date") == 0 {
		t.Errorf("control: err %v, %d looks", err, a.words("date"))
	}
}

// The look before a reconnect sees the app as it was when the connection was lost: a reconnect can relaunch the app, and then the process list says it is running.
func TestTheLookBeforeAReconnectSeesTheAppAsItWas(t *testing.T) {
	var log []string
	a := &ceAdb{log: &log, date: "2026-11-15 19:31:15", exit: []string{ceSignalEntry("2026-11-15 19:31:09.870", "2")}, pidof: []string{""}, pidAfterClose: "4100"}
	e := ceExecutor(a, true)
	e.client = &ceClient{log: &log}
	e.SetReconnectPolicy(1, time.Hour)
	e.timeout = 80 * time.Millisecond
	_ = e.runStep(context.Background(), pressEnter)
	r := e.CrashReport()
	if r == nil || !r.Gone || r.Kind != device.ExitSignaled {
		t.Fatalf("the app was reported as it was after the reconnect: %+v", r)
	}
	closeAt := -1
	for i, l := range log {
		if l == "client close" && closeAt < 0 {
			closeAt = i
		}
	}
	for i, l := range log {
		if strings.HasPrefix(l, "adb ") && i > closeAt && closeAt >= 0 && strings.Contains(l, "pidof") {
			t.Errorf("a pidof was asked after the reconnect started: %v", log)
		}
	}
}
