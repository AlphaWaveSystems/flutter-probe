package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// "Clear data before each test" asks once for the run: a yes holds for every test, a no is the error of every test, and nobody to ask names --yes.
func TestClearBeforeTestAsksOncePerRun(t *testing.T) {
	asks, msgs := 0, []string{}
	dc := &DeviceContext{AppID: "com.example.app", Serial: "emulator-5554", Confirm: func(m string) bool { asks++; msgs = append(msgs, m); return true }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	exec := &Executor{} // no device context: the wipe itself is the cloud-mode no-op, so the question is what is under test
	for i := 0; i < 3; i++ {
		if err := r.clearBeforeTest(context.Background(), exec); err != nil {
			t.Fatalf("test %d: %v", i, err)
		}
	}
	if asks != 1 {
		t.Errorf("asked %d times for 3 tests, want once per run", asks)
	}
	for _, want := range []string{"com.example.app", "emulator-5554", "BEFORE EVERY TEST"} {
		if !strings.Contains(msgs[0], want) {
			t.Errorf("the question lacks %q: %q", want, msgs[0])
		}
	}
}

func TestClearBeforeTestNoStopsEveryTest(t *testing.T) {
	asks := 0
	dc := &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return false }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	for i := 0; i < 3; i++ {
		err := r.clearBeforeTest(context.Background(), &Executor{})
		if err == nil || !strings.Contains(err.Error(), "clear data before each test: cancelled by user") {
			t.Fatalf("test %d: %v", i, err)
		}
	}
	if asks != 1 {
		t.Errorf("asked %d times, want once", asks)
	}
}

func TestClearBeforeTestWithNobodyToAskNamesYes(t *testing.T) {
	r := &Runner{cfg: &config.Config{}, deviceCtx: &DeviceContext{AppID: "a.b", Serial: "s"}}
	err := r.clearBeforeTest(context.Background(), &Executor{})
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "clear data before each test") {
		t.Errorf("err = %v", err)
	}
}

func TestClearBeforeTestWithYesNeverAsks(t *testing.T) {
	asks := 0
	dc := &DeviceContext{AppID: "a.b", Serial: "s", AllowClearData: true, ExplicitYes: true, Confirm: func(string) bool { asks++; return false }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	if err := r.clearBeforeTest(context.Background(), &Executor{}); err != nil || asks != 0 {
		t.Errorf("err = %v, asked %d", err, asks)
	}
}

// The answer to the per-run question does not become permission for a `clear app data` step inside a test: that step still asks as it always has.
func TestTheRunQuestionDoesNotAnswerAClearAppDataStep(t *testing.T) {
	asks := 0
	dc := &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return true }}
	r := &Runner{cfg: &config.Config{}, deviceCtx: dc}
	if err := r.clearBeforeTest(context.Background(), &Executor{}); err != nil {
		t.Fatal(err)
	}
	if dc.AllowClearData {
		t.Fatal("the run question turned on AllowClearData")
	}
	if err := dc.AskClearData(false); err != nil || asks != 2 {
		t.Errorf("a clear app data step after it: err %v, asked %d (want a second question)", err, asks)
	}
}

// Through the real attempt: when the run's question is answered no, the test body does not run at all (nothing was tapped), the result names the
// setting, and with the setting off the same test runs and no question is asked.
func TestAnAttemptDoesNotRunItsBodyWhenTheClearWasRefused(t *testing.T) {
	asks := 0
	client := newSlowClient()
	r := newTimeoutRunner(client, 0)
	r.cfg.Defaults.ClearDataBeforeEachTest = true
	r.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return false }}
	prog, tdef := parseOneTest(t, "test \"t\"\n  tap \"Fine\"\n")
	res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	if res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "clear data before each test: cancelled by user") {
		t.Fatalf("result = %+v", res)
	}
	client.mu.Lock()
	tapped := len(client.taps)
	client.mu.Unlock()
	if tapped != 0 {
		t.Error("the test body ran although the clear was refused")
	}
	// the same run: a second test is refused with the same answer, without a second question
	res2 := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	if res2.Passed || asks != 1 {
		t.Errorf("second test passed=%v, asks=%d (want a no and one question)", res2.Passed, asks)
	}

	off := newTimeoutRunner(newSlowClient(), 0)
	off.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { t.Error("asked with the setting off"); return false }}
	if r := off.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1); !r.Passed {
		t.Errorf("with the setting off the test did not run: %+v", r)
	}
}

// A before-each hook is part of the test's start: after a refused clear it does not run either.
func TestABeforeEachHookDoesNotRunAfterARefusedClear(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, 0)
	r.cfg.Defaults.ClearDataBeforeEachTest = true
	r.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { return false }}
	prog, tdef := parseOneTest(t, "before each\n  tap \"Hook\"\n\ntest \"t\"\n  tap \"Body\"\n")
	res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	client.mu.Lock()
	taps := append([]string(nil), client.taps...)
	client.mu.Unlock()
	if res.Passed || len(taps) != 0 {
		t.Errorf("passed=%v, taps=%v: neither the hook nor the body may run after a refused clear", res.Passed, taps)
	}
}

// A parallel lane has AllowClearData set by the orchestrator, no confirmation and no --yes: "clear data before each test" refuses there, whatever
// else is set. A project's probe.yaml must not wipe every attached device through that flag.
func TestALaneCannotAnswerClearBeforeEachTest(t *testing.T) {
	lane := &DeviceContext{AppID: "com.example.app", Serial: "R58M123ABC", AllowClearData: true} // what ParallelOrchestrator.deviceContext builds
	r := &Runner{cfg: &config.Config{}, deviceCtx: lane}
	err := r.clearBeforeTest(context.Background(), &Executor{})
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "parallel") || !strings.Contains(err.Error(), "R58M123ABC") {
		t.Fatalf("a lane was allowed to clear before each test: %v", err)
	}
	// through the real attempt: nothing runs, neither a hook nor the body
	client := newSlowClient()
	run := newTimeoutRunner(client, 0)
	run.cfg.Defaults.ClearDataBeforeEachTest = true
	run.deviceCtx = lane
	prog, tdef := parseOneTest(t, "before each\n  tap \"Hook\"\n\ntest \"t\"\n  tap \"Body\"\n")
	res := run.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	client.mu.Lock()
	taps := append([]string(nil), client.taps...)
	client.mu.Unlock()
	if res.Passed || len(taps) != 0 {
		t.Errorf("passed=%v taps=%v: a lane wiped or ran after a refusal", res.Passed, taps)
	}
	// the person's answer to the question still answers it for a lane that has one, and --yes answers it
	asked := 0
	withConfirm := &DeviceContext{AppID: "a.b", Serial: "s", AllowClearData: true, Confirm: func(string) bool { asked++; return true }}
	if err := (&Runner{cfg: &config.Config{}, deviceCtx: withConfirm}).clearBeforeTest(context.Background(), &Executor{}); err != nil || asked != 1 {
		t.Errorf("a lane with a confirmation: %v asked %d", err, asked)
	}
}

// A `clear app data` step in a test asks as it always has, through the real executor and device context (the run question's answer, tested above at
// the device context, is not permission for it).
func TestAClearAppDataStepStillAsksThroughTheExecutor(t *testing.T) {
	asks := 0
	r := newTimeoutRunner(newSlowClient(), 0)
	r.deviceCtx = &DeviceContext{AppID: "a.b", Serial: "s", Confirm: func(string) bool { asks++; return false }}
	prog, tdef := parseOneTest(t, "test \"t\"\n  clear app data\n")
	res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1)
	if res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "clear app data: cancelled by user") || asks != 1 {
		t.Errorf("the step did not ask: passed=%v err=%v asks=%d", res.Passed, res.Error, asks)
	}
	// AllowClearData (what a lane has) answers the step's own question as it always did; it does not answer the per-test one (see TestALane...)
	lane := &DeviceContext{AppID: "a.b", Serial: "s", AllowClearData: true}
	if err := lane.AskClearData(false); err != nil {
		t.Errorf("a clear app data step on a lane: %v", err)
	}
}

// The question text is one line each, whatever the id or serial hold.
func TestTheRunQuestionIsCleanedToOneLine(t *testing.T) {
	var msg string
	dc := &DeviceContext{AppID: "a.b\nFAKE\x1b[2J", Serial: "s\u202e\nX", Confirm: func(m string) bool { msg = m; return false }}
	_ = dc.AskClearData(true)
	if strings.Contains(msg, "\x1b") || strings.Contains(msg, "\u202e") || strings.Contains(msg, "a.b\nFAKE") {
		t.Errorf("the question carries a control character or a split: %q", msg)
	}
}

// With the setting on and no local device (cloud, dry run) the run says once that it does nothing, and runs the test.
func TestTheSettingWithNoDeviceSaysSoOnce(t *testing.T) {
	r := newTimeoutRunner(newSlowClient(), 0)
	r.cfg.Defaults.ClearDataBeforeEachTest = true
	r.deviceCtx = nil
	prog, tdef := parseOneTest(t, "test \"t\"\n  tap \"Fine\"\n")
	out := captureStdout(t, func() {
		for i := 0; i < 3; i++ {
			if res := r.runSingleTest(context.Background(), prog, tdef, "t.probe", nil, -1); !res.Passed {
				t.Errorf("the test did not run: %+v", res)
			}
		}
	})
	if strings.Count(out, "clear-before-each-test has no effect here") != 1 {
		t.Errorf("the notice appeared %d times: %q", strings.Count(out, "clear-before-each-test has no effect here"), out)
	}
}
