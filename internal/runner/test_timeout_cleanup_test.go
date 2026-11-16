package runner

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedRecorder stands in for the device's video recorder. Stop returns path, or, when hang is set, does not return until release
// is closed. started counts the Start calls.
type scriptedRecorder struct {
	path    string
	hang    bool
	release chan struct{}
	started atomic.Int32
}

func (s *scriptedRecorder) Start(context.Context, string) error {
	s.started.Add(1)
	return nil
}

func (s *scriptedRecorder) Stop(context.Context) (string, error) {
	if s.hang {
		<-s.release
	}
	return s.path, nil
}

// withVideo makes the runner record through rec, as a device run does, and stubs the restart step so no device is needed.
func withVideo(r *Runner, rec videoRecorder) {
	r.opts.VideoEnabled = true
	r.deviceCtx = &DeviceContext{}
	r.newVideo = func() videoRecorder { return rec }
	r.restartHook = func(context.Context) error { return nil }
}

// Once the limit ends the attempt, the cleanup gives its recording stop the same budget as the screenshot and the hooks. A stop that
// never returns is given up at the budget, and the attempt returns without that video.
func TestVideoStopIsBoundedByTheCleanupBudget(t *testing.T) {
	shortGrace(t, 300*time.Millisecond)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	rec := &scriptedRecorder{path: "/tmp/hung.mp4", hang: true, release: make(chan struct{})}
	defer close(rec.release)
	withVideo(r, rec)
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	start := time.Now()
	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)
	elapsed := time.Since(start)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the recording stop must give up at the cleanup budget; the attempt took %v", elapsed)
	}
	for _, a := range res.Artifacts {
		if strings.Contains(a, "hung.mp4") {
			t.Errorf("a stop that never returned left an artifact: %q", a)
		}
	}
	if got := rec.started.Load(); got != 1 {
		t.Errorf("the recording was started %d times, want 1", got)
	}
}

// A recording stop that returns inside the budget is reported like any other: its file is an artifact of the attempt.
func TestVideoStopIsReportedWhenItReturnsInsideTheBudget(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	withVideo(r, &scriptedRecorder{path: "/tmp/slow_video.mp4"})
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	found := false
	for _, a := range res.Artifacts {
		if strings.HasSuffix(a, "slow_video.mp4") {
			found = true
		}
	}
	if !found {
		t.Errorf("the video the stop returned must be an artifact of the attempt, got %v", res.Artifacts)
	}
}

// When the cleanup is abandoned (a screenshot that does not return in time), the app is still restarted for the next test. The restart
// is not skipped by the abandonment: the next test needs the app relaunched either way.
func TestAbandonedCleanupStillRestartsTheApp(t *testing.T) {
	shortGrace(t, 300*time.Millisecond)
	client := newSlowClient()
	client.shotDelay = 2 * time.Second // returns after the grace budget, so the cleanup is abandoned
	r := newTimeoutRunner(client, time.Second)
	var restarts atomic.Int32
	r.restartHook = func(context.Context) error {
		restarts.Add(1)
		return nil
	}
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if got := restarts.Load(); got != 1 {
		t.Errorf("an abandoned cleanup must still restart the app once, got %d restarts", got)
	}
}

// The cleanup's calls see a live context after the limit: the grace budget is the cleanup's own, not the expired attempt's.
func TestCleanupCallsSeeALiveContextAfterTheLimit(t *testing.T) {
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
	live := append([]bool(nil), client.probeLive...)
	client.mu.Unlock()
	if len(live) != 1 || !live[0] {
		t.Errorf("the after-each call must run on a live context after the limit, got %v", live)
	}
}

// Ctrl-C during the cleanup ends it at once and skips the restart: the run's cancel reaches the cleanup's budget, and a run that was
// cancelled is not restarted.
func TestCancellingTheRunDuringCleanupSkipsTheRestart(t *testing.T) {
	shortGrace(t, 30*time.Second)
	client := newSlowClient()
	client.shotDelay = 10 * time.Second // the screenshot does not return in the test's time
	r := newTimeoutRunner(client, time.Second)
	var restarts atomic.Int32
	r.restartHook = func(context.Context) error {
		restarts.Add(1)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(1500*time.Millisecond, cancel) // after the limit, while the screenshot is still out
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	start := time.Now()
	res := r.runSingleTest(ctx, prog, slow, "slow.probe", nil, -1)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("the cleanup must end when the run is cancelled; it took %v", elapsed)
	}
	if got := restarts.Load(); got != 0 {
		t.Errorf("a cancelled run must not be restarted, got %d restarts", got)
	}
	var te *testTimeoutError
	if !errors.As(res.Error, &te) || te.restart == nil {
		t.Errorf("want the skipped restart recorded on the timeout, got %v", res.Error)
	}
}

// Every expiry is followed by a restart, and the restart runs on the run's live context, not on the expired attempt's.
func TestEveryExpiryIsFollowedByARestartOnALiveContext(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	var mu sync.Mutex
	var live []bool
	r.restartHook = func(ctx context.Context) error {
		mu.Lock()
		live = append(live, ctx.Err() == nil)
		mu.Unlock()
		return nil
	}
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	for i := 0; i < 2; i++ {
		if res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1); !res.TimedOut {
			t.Fatalf("expiry %d: want a timed-out attempt, got %+v", i+1, res)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(live) != 2 || !live[0] || !live[1] {
		t.Errorf("want two restarts, each on a live context; got %v", live)
	}
}

// A restart that fails is recorded on the timeout, and the next test still runs.
func TestNextTestRunsAfterAFailedRestart(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return errors.New("the agent did not come back") }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)
	if !strings.Contains(res.Error.Error(), "restart after the timeout failed: the agent did not come back") {
		t.Errorf("the failed restart must be named on the timeout, got %v", res.Error)
	}

	r.restartHook = nil
	nextProg, nextTest := parseOneTest(t, "test \"next\"\n  tap \"Fine\"\n")
	nres := r.runSingleTest(context.Background(), nextProg, nextTest, "next.probe", nil, -1)
	if !nres.Passed || nres.TimedOut {
		t.Errorf("the next test must still run and pass after a failed restart; got passed=%t timedOut=%t err=%v", nres.Passed, nres.TimedOut, nres.Error)
	}
}

// A retry gets its own budget: an attempt that ran to the limit does not cut the next attempt short.
func TestEachRetryGetsAFreshBudget(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.cfg.Defaults.RetryFailedTests = 1
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if res.Attempts != 2 || len(res.AttemptLog) != 2 {
		t.Fatalf("want two attempts, got attempts=%d log=%d", res.Attempts, len(res.AttemptLog))
	}
	for i, a := range res.AttemptLog {
		if a.Duration < 900*time.Millisecond {
			t.Errorf("attempt %d lasted %v; each attempt must get the whole limit", i+1, a.Duration)
		}
	}
}

// Each data row gets its own budget: a row that runs to the limit does not cut the next row short. The rows are run with their own
// variables, as a data-driven test runs them.
func TestEachDataRowGetsAFreshBudget(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, row := parseOneTest(t, "test \"row\"\n  tap \"<Target>\"\n")

	slow := r.runSingleTest(context.Background(), prog, row, "row.probe", map[string]string{"Target": "Slow"}, 0)
	fine := r.runSingleTest(context.Background(), prog, row, "row.probe", map[string]string{"Target": "Fine"}, 1)

	if !slow.TimedOut {
		t.Errorf("the first row must hit the limit, got %+v", slow)
	}
	if !fine.Passed || fine.TimedOut {
		t.Errorf("the second row must get a fresh budget and pass; got passed=%t timedOut=%t err=%v", fine.Passed, fine.TimedOut, fine.Error)
	}
}

// A before-each hook that runs past the limit ends the attempt on the limit, and the body does not run.
func TestBeforeEachIsBoundedByTheLimit(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, body := parseOneTest(t, "before each\n  tap \"Slow\"\n\ntest \"t\"\n  tap \"Fine\"\n")

	res := r.runSingleTest(context.Background(), prog, body, "t.probe", nil, -1)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	for _, tap := range client.tapped() {
		if tap == "Fine" {
			t.Errorf("the body ran after the before-each hook hit the limit")
		}
	}
	if res.Error == nil || !strings.Contains(res.Error.Error(), `tap "Slow"`) {
		t.Errorf("the timeout must name the before-each step it was on, got %v", res.Error)
	}
}

// A reconnect that finishes after its attempt's guard was released is dropped: the runner keeps its client, and the dropped client is
// closed.
func TestReconnectAfterTheGuardIsReleasedIsDropped(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	guard := newCleanupGuard()
	exec := r.newAttemptExecutor(guard)
	late := newSlowClient()
	guard.release()

	exec.onReconnect(late)

	if r.client != client {
		t.Errorf("a reconnect after the guard was released replaced the runner's client")
	}
	late.mu.Lock()
	closed := late.closed
	late.mu.Unlock()
	if !closed {
		t.Errorf("the dropped client must be closed")
	}
}

// The timeout's text names the step it was on and keeps a real error the attempt ended with. A cancellation is the limit's own, so it is
// not repeated, and a failed restart is named after it.
func TestTimeoutMessageNamesTheStepAndKeepsARealError(t *testing.T) {
	cases := []struct {
		name string
		err  *testTimeoutError
		want string
	}{
		{
			name: "a step error is kept",
			err:  &testTimeoutError{limit: 90 * time.Second, step: `line 3: tap "Save"`, cause: errors.New(`no element "Save"`)},
			want: `test exceeded 90s (line 3: tap "Save"): no element "Save"`,
		},
		{
			name: "a cancellation is not repeated",
			err:  &testTimeoutError{limit: time.Second, step: `line 2: tap "Slow"`, cause: context.DeadlineExceeded},
			want: `test exceeded 1s (line 2: tap "Slow")`,
		},
		{
			name: "a failed restart is named",
			err:  &testTimeoutError{limit: time.Second, step: `line 2: tap "Slow"`, cause: context.Canceled, restart: errors.New("no agent")},
			want: `test exceeded 1s (line 2: tap "Slow") (restart after the timeout failed: no agent)`,
		},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("%s: Error() = %q, want %q", c.name, got, c.want)
		}
	}
}

// The stream carries time_limit_seconds only for a result that ran under the limit, as the report does, so a run without the limit
// streams exactly what it did before.
func TestStreamResultOmitsTheLimitWhenOff(t *testing.T) {
	var buf bytes.Buffer
	rep := NewReporter(FormatJSON, &buf, false)
	rep.SetStreaming(true)
	rep.StreamResult(TestResult{TestName: "a", File: "a.probe", Passed: true})
	rep.StreamResult(TestResult{TestName: "b", File: "b.probe", TimedOut: true, TimeLimit: 90 * time.Second})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two stream events, got %q", buf.String())
	}
	if strings.Contains(lines[0], "time_limit_seconds") || strings.Contains(lines[0], "timed_out") {
		t.Errorf("a stream event without the limit must not carry the new fields: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"time_limit_seconds":90`) || !strings.Contains(lines[1], `"timed_out":true`) {
		t.Errorf("a stream event under the limit must carry the fields: %s", lines[1])
	}
}
