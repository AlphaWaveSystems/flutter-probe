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
	dc := &DeviceContext{AppID: "a.b", Serial: "s", AllowClearData: true, Confirm: func(string) bool { asks++; return false }}
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
