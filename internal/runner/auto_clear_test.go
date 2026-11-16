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
