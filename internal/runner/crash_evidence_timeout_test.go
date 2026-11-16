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
		e, _, err := runTimeoutStep(t, true, a, timeoutErr(), pressEnter)
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
