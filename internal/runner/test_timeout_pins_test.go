package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// lockedBuffer collects what a cleanup prints, from its own goroutine, without a race.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// The reconnect gate: a step whose context a limit ended makes no reconnect. A connection error on such a step is returned as it is,
// and the client is not closed for a re-dial.
func TestReconnectIsSkippedOnceTheLimitEndsTheStep(t *testing.T) {
	client := newSlowClient()
	exec := NewExecutor(client, &DeviceContext{}, func(probelink.ProbeClient) {}, time.Second, false)
	_, drop := parseOneTest(t, "test \"drop\"\n  tap \"Drop\"\n")
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second)) // the limit has ended the step's context
	defer cancel()

	err := exec.RunBody(ctx, drop.Body)

	if got := client.tapped(); len(got) != 1 || got[0] != "Drop" {
		t.Fatalf("the step must reach the client before the gate is judged, taps = %v", got)
	}
	if err == nil || strings.Contains(err.Error(), "auto-reconnect") {
		t.Errorf("a step the limit ended must not reconnect; got %v", err)
	}
	client.mu.Lock()
	closed := client.closed
	client.mu.Unlock()
	if closed {
		t.Errorf("the reconnect closed the client: the gate must stop it before the re-dial")
	}
}

// Without the limit a Ctrl-C (a cancel, not a deadline) reconnects as it always did, so a run without the limit behaves as before.
func TestReconnectStillRunsOnACancel(t *testing.T) {
	client := newSlowClient()
	exec := NewExecutor(client, &DeviceContext{}, func(probelink.ProbeClient) {}, time.Second, false)
	_, drop := parseOneTest(t, "test \"drop\"\n  tap \"Drop\"\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Ctrl-C

	err := exec.RunBody(ctx, drop.Body)

	if err == nil || !strings.Contains(err.Error(), "auto-reconnect attempt 1 failed") {
		t.Errorf("a cancel must reconnect as before, so the failed re-dial is named; got %v", err)
	}
	client.mu.Lock()
	closed := client.closed
	client.mu.Unlock()
	if !closed {
		t.Errorf("a cancel must reach the re-dial, which closes the client first")
	}
}

// The step-timeout annotation asks the agent for the screen. A limit that ended the step must not make that request, since the snapshot
// could outlive the cleanup budget.
func TestAnnotateIsSkippedOnceTheLimitEndsTheStep(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	client.mu.Lock()
	calls := client.summaryCalls
	client.mu.Unlock()
	if calls != 0 {
		t.Errorf("the screen was asked for %d times after the limit; a limit must not annotate", calls)
	}
}

// Without a limit a step timeout is still annotated: the snapshot is asked for, as it always was.
func TestAnnotateStillRunsOnAStepTimeout(t *testing.T) {
	client := newSlowClient()
	exec := NewExecutor(client, nil, nil, 300*time.Millisecond, false)
	_, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	err := exec.RunBody(context.Background(), slow.Body)

	if err == nil {
		t.Fatal("the step must time out")
	}
	client.mu.Lock()
	calls := client.summaryCalls
	client.mu.Unlock()
	if calls != 1 {
		t.Errorf("a step timeout must still annotate with the screen, asked %d times", calls)
	}
}

// The after-each hooks run on the cleanup's budget, so their calls carry a deadline a little less than the budget away. A context
// without one would let a hung call hold the cleanup.
func TestAfterEachRunsOnTheCleanupBudget(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n\nafter each\n  tap \"Probe\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	client.mu.Lock()
	remain := append([]time.Duration(nil), client.probeRemain...)
	client.mu.Unlock()
	if len(remain) != 1 || remain[0] <= 0 || remain[0] > 5*time.Second {
		t.Errorf("the after-each call must carry the cleanup budget as its deadline; it had %v left", remain)
	}
}

// A cleanup that was abandoned at its budget prints nothing after that: the screenshot that returns late is dropped by the guard, even
// though the restart that follows it takes longer than the screenshot.
func TestGuardIsReleasedWhenTheCleanupIsAbandoned(t *testing.T) {
	shortGrace(t, 200*time.Millisecond)
	client := newSlowClient()
	client.shotDelay = 500 * time.Millisecond // returns after the budget, so the cleanup is abandoned first
	client.shotDone = make(chan struct{})
	out := &lockedBuffer{}
	cleanupOut = out // stays for the rest of the run: a test's late cleanup must not write into a test that has already ended
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error {
		time.Sleep(time.Second) // the restart runs while the late screenshot is still on its way back
		return nil
	}
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)
	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	<-client.shotDone
	time.Sleep(100 * time.Millisecond) // let the late screenshot's print happen, if it is going to
	if got := out.String(); strings.Contains(got, "failure screenshot saved") {
		t.Errorf("an abandoned cleanup printed after its budget:\n%s", got)
	}
}

// A `before each` hook that runs past the limit ends the attempt at the limit, not when its call would have returned.
func TestBeforeEachEndsAtTheLimit(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, body := parseOneTest(t, "before each\n  tap \"Slow\"\n\ntest \"t\"\n  tap \"Fine\"\n")

	start := time.Now()
	res := r.runSingleTest(context.Background(), prog, body, "t.probe", nil, -1)
	elapsed := time.Since(start)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if elapsed < 900*time.Millisecond || elapsed > 2500*time.Millisecond {
		t.Errorf("the attempt took %v; the before-each hook must end at the 1s limit", elapsed)
	}
}

// A run cancelled before the limit is not a timeout: the attempt reports the cancellation, carries none of the limit's error, and does not
// restart the app.
func TestRunCancelIsNotATimeout(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	var restarts atomic.Int32
	r.restartHook = func(context.Context) error {
		restarts.Add(1)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(300*time.Millisecond, cancel)
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(ctx, prog, slow, "slow.probe", nil, -1)

	if res.TimedOut {
		t.Errorf("a run cancelled before the limit is not a timeout: %v", res.Error)
	}
	var te *testTimeoutError
	if errors.As(res.Error, &te) {
		t.Errorf("a cancelled attempt must not carry the limit's error: %v", res.Error)
	}
	if got := restarts.Load(); got != 0 {
		t.Errorf("a cancelled run must not restart the app on an expiry, got %d restarts", got)
	}
}

// The restart after an expiry is the real restart step: it reaches adb and the client with a live context. The adb here is a script
// that logs its arguments, so the test sees the force-stop the restart issues.
func TestRestartAfterAnExpiryReachesAdbOnALiveContext(t *testing.T) {
	shortGrace(t, 5*time.Second)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "adb.log")
	bin := filepath.Join(dir, "adb")
	script := "#!/bin/sh\necho \"$@\" >> '" + logPath + "'\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.deviceCtx = &DeviceContext{
		Manager:          device.NewManagerWithPaths(device.ToolPaths{ADB: bin}),
		Serial:           "emulator-5590",
		Platform:         device.PlatformAndroid,
		AppID:            "com.example.app",
		RestartDelay:     time.Millisecond,
		ReconnectDelay:   time.Millisecond,
		TokenReadTimeout: 50 * time.Millisecond,
		DialTimeout:      100 * time.Millisecond,
	}
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logged), "am force-stop com.example.app") {
		t.Errorf("the restart must reach adb on a live context; adb saw %q", logged)
	}
	client.mu.Lock()
	live := append([]bool(nil), client.tokenLive...)
	client.mu.Unlock()
	if len(live) == 0 || !live[0] {
		t.Errorf("the restart's token call must run on a live context, got %v", live)
	}
	var te *testTimeoutError
	if errors.As(res.Error, &te) && te.restart != nil && strings.Contains(te.restart.Error(), "context") {
		t.Errorf("the restart failed on its context, not on the device: %v", te.restart)
	}
}

// The perf window the attempt opened is closed by the cleanup, inside its budget: the cleanup calls the client's perf stop.
func TestPerfWindowIsClosedByTheCleanup(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, perf := parseOneTest(t, "test \"perf\"\n  start measuring \"window\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, perf, "perf.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	client.mu.Lock()
	starts, stops := client.perfStarts, client.perfStops
	client.mu.Unlock()
	if starts != 1 {
		t.Fatalf("the window must open on the client, starts = %d", starts)
	}
	if stops != 1 {
		t.Errorf("the cleanup must close the attempt's perf window, got %d stops", stops)
	}
}

// A scrcpy stop whose deadline ends kills the process and returns: an abandoned stop does not outlive its budget. The process ignores
// SIGINT, so only the kill ends it.
func TestScrcpyStopEndsTheProcessAtItsDeadline(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' INT; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Skipf("sh is not available: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the shell install its trap: a SIGINT that arrives first would end it
	vr := &VideoRecorder{backend: "scrcpy", cmd: cmd, localPath: filepath.Join(t.TempDir(), "missing.mp4")}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := vr.Stop(ctx)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "did not stop") {
		t.Errorf("a stop at its deadline must say so; got %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the stop took %v; it must return at its deadline", elapsed)
	}
	if cmd.ProcessState == nil {
		t.Errorf("the process must be dead when the stop returns")
	}
}

// A scrcpy stop whose context is cancelled (Ctrl-C) waits for the file, as it always did: the process exits on SIGINT and the stop goes on
// to check the file, and does not report a deadline.
func TestScrcpyStopWaitsOnACancel(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("sleep is not available: %v", err)
	}
	vr := &VideoRecorder{backend: "scrcpy", cmd: cmd, localPath: filepath.Join(t.TempDir(), "missing.mp4")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := vr.Stop(ctx)

	if err != nil && strings.Contains(err.Error(), "did not stop") {
		t.Errorf("a cancel must not be reported as a deadline: %v", err)
	}
	if cmd.ProcessState == nil {
		t.Errorf("the stop must have waited for the process to exit")
	}
}
