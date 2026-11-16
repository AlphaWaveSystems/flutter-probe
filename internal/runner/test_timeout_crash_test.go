package runner

import (
	"context"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
)

// timeoutCrashDevice is the device a run with --crash-evidence has, reading its evidence through the recording adb.
func timeoutCrashDevice(a *ceAdb, crashEvidence bool) *DeviceContext {
	return &DeviceContext{Serial: ceSerial, Platform: device.PlatformAndroid, AppID: cePkg, CrashEvidence: crashEvidence, CrashADB: a, CrashPoll: time.Millisecond}
}

// An attempt that ran past its limit gets one look at the crash evidence, on a live context. The recording adb fails any call on an expired
// context, as the real adb does, so a report is only possible if the look ran live. The look finds the app that died during the test: the
// device clock is just after its exit entry, which is inside the attempt's window.
func TestTimeoutLooksAtTheCrashEvidenceOnce(t *testing.T) {
	shortGrace(t, 5*time.Second)
	a := &ceAdb{date: "2026-11-15 19:31:10", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.deviceCtx = timeoutCrashDevice(a, true)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if res.Crash == nil {
		t.Fatalf("the look must find the death of the app that the limit ended, got no crash report")
	}
	if res.Crash.Kind != device.ExitCrash {
		t.Errorf("crash kind = %v, want the Java crash", res.Crash.Kind)
	}
	if n := a.words("date"); n != 1 {
		t.Errorf("the look must run once per attempt, got %d device clock reads", n)
	}
}

// Without --crash-evidence the limit adds no call to the device: the flag decides, as it does for every other run.
func TestTimeoutMakesNoCrashCallWithoutTheFlag(t *testing.T) {
	shortGrace(t, 5*time.Second)
	a := &ceAdb{date: "2026-11-15 19:31:10", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.deviceCtx = timeoutCrashDevice(a, false)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if res.Crash != nil || len(a.calls) != 0 {
		t.Errorf("without the flag the limit must not reach the device: crash=%v device calls=%d", res.Crash, len(a.calls))
	}
}

// A run that was cancelled makes no call to the device: the look's context derives from the run's, so the cancel ends it before the first
// call, and the look reports nothing.
func TestCancelledRunMakesNoCrashCall(t *testing.T) {
	a := &ceAdb{date: "2026-11-15 19:31:10", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}}
	e := NewExecutor(nil, timeoutCrashDevice(a, true), nil, time.Second, false)
	e.startedAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	e.collectCrashAfterTimeout(ctx)

	if len(a.calls) != 0 {
		t.Errorf("a cancelled run must make no device call, made %d", len(a.calls))
	}
	if e.CrashReport() != nil {
		t.Errorf("a cancelled look must report nothing")
	}
}

// The look runs before the cleanup, so its evidence survives a cleanup that is abandoned at its budget: the attempt keeps what it found.
func TestAbandonedCleanupKeepsTheCrashEvidence(t *testing.T) {
	shortGrace(t, 200*time.Millisecond)
	a := &ceAdb{date: "2026-11-15 19:31:10", pidof: []string{""}, exit: []string{ceSample(t, "exit-java.txt")}, buf: ceSample(t, "crashbuf-java.txt")}
	client := newSlowClient()
	client.shotDelay = 2 * time.Second // the screenshot outlasts the budget, so the cleanup is abandoned
	r := newTimeoutRunner(client, time.Second)
	r.deviceCtx = timeoutCrashDevice(a, true)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if res.Crash == nil {
		t.Errorf("the evidence found before an abandoned cleanup must be kept on the result")
	}
}
