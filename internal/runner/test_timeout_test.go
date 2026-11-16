package runner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// slowClient is an agent that answers every tap and screenshot the test asks for. A tap on "Slow" waits for its context and
// returns when it ends; a tap on "Hang" never returns (it ignores its context) until the test releases it; a screenshot records
// whether its context was still live when it started. Only the methods the tests use are overridden.
type slowClient struct {
	probelink.ProbeClient
	mu        sync.Mutex
	taps      []string
	shotLive  []bool
	shotDelay time.Duration // a screenshot returns after this long, whatever its context says
	release   chan struct{}
	probeLive []bool // a tap on "Probe" records whether its context was still live when it was called
	closed    bool   // Close was called on this client

	slowRemain    []time.Duration // a tap on "Slow" records how long its context had left (0 when it had no deadline)
	probeRemain   []time.Duration // a tap on "Probe" records the same
	tokenLive     []bool          // SetNextToken records whether its context was still live
	summaryCalls  int             // VisibleSummary calls: the step-timeout annotation asks the agent for the screen
	perfStarts    int
	perfStops     int
	shotDone      chan struct{} // closed when a screenshot returns, if set
}

// remainingOf is how long until ctx's deadline, or 0 when it has none.
func remainingOf(ctx context.Context) time.Duration {
	if d, ok := ctx.Deadline(); ok {
		return time.Until(d)
	}
	return 0
}

func newSlowClient() *slowClient {
	return &slowClient{release: make(chan struct{})}
}

func (c *slowClient) Connected() bool { return true }

// HTTPClear is called at the start of every attempt, so it must not reach the nil embedded interface.
func (c *slowClient) HTTPClear(ctx context.Context, mocks bool) error { return nil }

func (c *slowClient) Tap(ctx context.Context, sel probelink.SelectorParam) error {
	c.mu.Lock()
	c.taps = append(c.taps, sel.Text)
	c.mu.Unlock()
	switch sel.Text {
	case "Slow":
		c.mu.Lock()
		c.slowRemain = append(c.slowRemain, remainingOf(ctx))
		c.mu.Unlock()
		// With no deadline on ctx this tap ends after 3 seconds, with an error: a test that expects the limit to cut it fails, not hangs.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			return errors.New("no limit cut this tap")
		}
	case "Drop":
		return errors.New("probelink: write: broken pipe")
	case "Hang":
		<-c.release
		return nil
	case "Probe":
		c.mu.Lock()
		c.probeLive = append(c.probeLive, ctx.Err() == nil)
		c.probeRemain = append(c.probeRemain, remainingOf(ctx))
		c.mu.Unlock()
	}
	return nil
}

// SetNextToken records whether its context was live: a restart calls it first, with the context the restart runs on.
func (c *slowClient) SetNextToken(ctx context.Context, token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokenLive = append(c.tokenLive, ctx.Err() == nil)
	return nil
}

// VisibleSummary is what the step-timeout annotation asks for: the test records the call and answers with nothing.
func (c *slowClient) VisibleSummary(ctx context.Context) (probelink.VisibleSummaryResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summaryCalls++
	return probelink.VisibleSummaryResult{}, nil
}

// HTTPLog answers with an error: the perf window then has no request count to start from, and the window still opens.
func (c *slowClient) HTTPLog(ctx context.Context, p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	return probelink.HTTPLogResult{}, errors.New("no http log in this test")
}

// PerfStart and PerfStop count the perf window's calls.
func (c *slowClient) PerfStart(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perfStarts++
	return nil
}

func (c *slowClient) PerfStop(ctx context.Context) (probelink.PerfSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perfStops++
	return probelink.PerfSnapshot{}, nil
}

// Close records that the client was closed: a reconnect that the guard drops closes the client it was given.
func (c *slowClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *slowClient) Screenshot(ctx context.Context, name string) (string, error) {
	c.mu.Lock()
	c.shotLive = append(c.shotLive, ctx.Err() == nil)
	c.mu.Unlock()
	if c.shotDelay > 0 {
		time.Sleep(c.shotDelay)
	}
	if c.shotDone != nil {
		close(c.shotDone)
	}
	return "/tmp/" + name + ".png", nil
}

func (c *slowClient) tapped() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.taps...)
}

// newTimeoutRunner is a runner that talks to client with the test limit set, and no device context, so the restart step is
// dispatched and skipped, and its StepStarted event is what a test can observe.
func newTimeoutRunner(client probelink.ProbeClient, limit time.Duration) *Runner {
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 30 * time.Second
	cfg.Defaults.TestTimeout = limit
	return New(cfg, client, nil, RunOptions{Timeout: 30 * time.Second})
}

// shortGrace shortens the grace budget for one test and restores it afterwards.
func shortGrace(t *testing.T, d time.Duration) {
	t.Helper()
	saved := testTimeoutGrace
	testTimeoutGrace = d
	t.Cleanup(func() { testTimeoutGrace = saved })
}

func parseOneTest(t *testing.T, src string) (*parser.Program, parser.TestDef) {
	t.Helper()
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Tests) != 1 {
		t.Fatalf("want one test, got %d", len(prog.Tests))
	}
	return prog, prog.Tests[0]
}

// An attempt that runs past its limit fails with the named reason; its after-each hook and screenshot run on a live context;
// the app is restarted; and the next test starts clean.
func TestTestTimeoutNamesTheLimitAndCleansUp(t *testing.T) {
	shortGrace(t, 5*time.Second)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	var restarts int
	var mu sync.Mutex
	r.onStep = func(ev StepEvent) {
		if ev.Status == StepStarted && strings.Contains(ev.Description, "restart") {
			mu.Lock()
			restarts++
			mu.Unlock()
		}
	}
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n\nafter each\n  tap \"Cleanup\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	if res.Passed || !res.TimedOut {
		t.Fatalf("want a failed, timed-out attempt; got passed=%t timedOut=%t err=%v", res.Passed, res.TimedOut, res.Error)
	}
	if res.TimeLimit != time.Second {
		t.Errorf("TimeLimit = %v, want 1s", res.TimeLimit)
	}
	if got := res.Error.Error(); got != `test exceeded 1s (line 2: tap "Slow")` {
		t.Errorf("error = %q, want the limit and the step it was on", got)
	}
	var te *testTimeoutError
	if !errors.As(res.Error, &te) {
		t.Fatalf("error %T is not a testTimeoutError", res.Error)
	}
	if !errors.Is(res.Error, context.Canceled) && !errors.Is(res.Error, context.DeadlineExceeded) {
		t.Errorf("the named error must keep the step's cancellation as its cause, got %v", te.cause)
	}
	if got := client.tapped(); len(got) != 2 || got[1] != "Cleanup" {
		t.Errorf("taps = %v, want the slow tap then the after-each Cleanup tap", got)
	}
	client.mu.Lock()
	shotLive := append([]bool(nil), client.shotLive...)
	client.mu.Unlock()
	if len(shotLive) != 1 || !shotLive[0] {
		t.Errorf("the failure screenshot must run on a live context, got %v", shotLive)
	}
	mu.Lock()
	gotRestarts := restarts
	mu.Unlock()
	if gotRestarts != 1 {
		t.Errorf("restart steps dispatched = %d, want 1 after the timeout", gotRestarts)
	}

	next := "test \"next\"\n  tap \"Fine\"\n"
	nextProg, nextTest := parseOneTest(t, next)
	nres := r.runSingleTest(context.Background(), nextProg, nextTest, "next.probe", nil, -1)
	if !nres.Passed || nres.TimedOut || nres.Error != nil {
		t.Errorf("the next test must start clean and pass; got passed=%t timedOut=%t err=%v", nres.Passed, nres.TimedOut, nres.Error)
	}
}

// With no limit an attempt is the same as before: no timeout, no restart, and the result fields stay zero.
func TestNoLimitLeavesTheAttemptAlone(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, 0)
	prog, pass := parseOneTest(t, "test \"fine\"\n  tap \"Fine\"\n")
	res := r.runSingleTest(context.Background(), prog, pass, "fine.probe", nil, -1)
	if !res.Passed || res.TimedOut || res.TimeLimit != 0 {
		t.Errorf("with the limit off: passed=%t timedOut=%t limit=%v", res.Passed, res.TimedOut, res.TimeLimit)
	}
}

// The grace phase has a total budget even when a call never returns: a hung screenshot and a hung after-each hook still give
// up at the budget, and the attempt returns.
func TestGracePhaseIsBoundedWhenACallNeverReturns(t *testing.T) {
	shortGrace(t, 300*time.Millisecond)
	client := newSlowClient()
	defer close(client.release)
	r := newTimeoutRunner(client, time.Second)
	prog, slow := parseOneTest(t, "test \"hung\"\n  tap \"Slow\"\n\nafter each\n  tap \"Hang\"\n")

	start := time.Now()
	res := r.runSingleTest(context.Background(), prog, slow, "hung.probe", nil, -1)
	elapsed := time.Since(start)

	if !res.TimedOut || res.Error == nil {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	// the limit (1s) plus the grace budget (300ms) plus slack; without the bound this would never return
	if elapsed > 5*time.Second {
		t.Errorf("the attempt took %v; the grace phase must give up at its budget", elapsed)
	}
}

// A call that returns after the grace phase gave up cannot write into the attempt's result or into the next test: the artifacts
// the attempt reports are the ones it had when it gave up, and the next test's own artifacts are its own. Run with -race.
func TestLateScreenshotDoesNotWriteAfterTheGraceBudget(t *testing.T) {
	shortGrace(t, 200*time.Millisecond)
	client := newSlowClient()
	client.shotDelay = 1500 * time.Millisecond // returns well after the grace budget
	r := newTimeoutRunner(client, time.Second)
	prog, slow := parseOneTest(t, "test \"late\"\n  tap \"Slow\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "late.probe", nil, -1)
	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	for _, a := range res.Artifacts {
		if strings.Contains(a, "failure_") {
			t.Errorf("the attempt reported the late screenshot %q; it was abandoned at the grace budget", a)
		}
	}

	// the next test runs while the late screenshot is still on its way back
	nextProg, nextTest := parseOneTest(t, "test \"next\"\n  tap \"Fine\"\n")
	nres := r.runSingleTest(context.Background(), nextProg, nextTest, "next.probe", nil, -1)
	if !nres.Passed {
		t.Errorf("next test failed: %v", nres.Error)
	}
	for _, a := range nres.Artifacts {
		if strings.Contains(a, "failure_slow") {
			t.Errorf("the next test carries the earlier test's screenshot: %q", a)
		}
	}
	time.Sleep(2 * time.Second) // let the late screenshot return; -race reports any write it makes into shared state
}

// A composite test has one budget for all its device blocks: the block that runs past it fails on the limit, and the block that
// finished in time passes and is not marked.
func TestCompositeTimeoutIsOneBudgetForAllDevices(t *testing.T) {
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 30 * time.Second
	cfg.Defaults.TestTimeout = time.Second
	slow := newSlowClient()
	fast := newSlowClient()
	devices := []CompositeDevice{{Alias: "A", Client: slow}, {Alias: "B", Client: fast}}
	cr := NewCompositeRunner(cfg, devices, RunOptions{Timeout: 30 * time.Second})

	_, test := parseOneCompositeTest(t, "composite test \"two devices\"\n  devices:\n    A: iPhone 15 Simulator\n    B: Pixel 9 Emulator\n  A:\n    tap \"Slow\"\n  B:\n    tap \"Fine\"\n")
	res := cr.RunCompositeTest(context.Background(), test, "composite.probe")

	if !res.TimedOut || res.TimeLimit != time.Second {
		t.Fatalf("want a timed-out composite test with a 1s limit, got timedOut=%t limit=%v", res.TimedOut, res.TimeLimit)
	}
	a := res.DeviceResults["A"]
	var te *testTimeoutError
	if a.Passed || !errors.As(a.Error, &te) {
		t.Errorf("device A must fail on the limit, got passed=%t error=%v", a.Passed, a.Error)
	}
	if b := res.DeviceResults["B"]; !b.Passed || b.Error != nil {
		t.Errorf("device B finished in time and must pass, got passed=%t error=%v", b.Passed, b.Error)
	}
	if res.Passed {
		t.Errorf("the composite test must fail when a device failed")
	}
}

func parseOneCompositeTest(t *testing.T, src string) (*parser.Program, parser.CompositeTestDef) {
	t.Helper()
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.CompositeTests) != 1 {
		t.Fatalf("want one composite test, got %d", len(prog.CompositeTests))
	}
	return prog, prog.CompositeTests[0]
}

// The validation accepts zero (off) and any positive limit of at least one second, and rejects a negative value or a shorter
// positive one.
func TestValidateTestTimeout(t *testing.T) {
	cases := []struct {
		d       time.Duration
		wantErr bool
	}{
		{0, false},
		{time.Second, false},
		{90 * time.Second, false},
		{500 * time.Millisecond, true},
		{-time.Second, true},
	}
	for _, c := range cases {
		if err := ValidateTestTimeout(c.d); (err != nil) != c.wantErr {
			t.Errorf("ValidateTestTimeout(%v) = %v, want error %t", c.d, err, c.wantErr)
		}
	}
}

// The JSON result carries timed_out and time_limit_seconds only when the limit is set, so a run without it is unchanged.
func TestJSONCarriesTheLimitOnlyWhenSet(t *testing.T) {
	r := NewReporter(FormatJSON, nil, false)
	off := r.buildJSONReport([]TestResult{{TestName: "a", File: "a.probe", Passed: true}})
	offBytes, _ := json.Marshal(off)
	if strings.Contains(string(offBytes), "timed_out") || strings.Contains(string(offBytes), "time_limit_seconds") {
		t.Errorf("a run without the limit must not carry the new fields: %s", offBytes)
	}
	on := r.buildJSONReport([]TestResult{{TestName: "b", File: "b.probe", Passed: false, TimedOut: true, TimeLimit: 90 * time.Second}})
	onBytes, _ := json.Marshal(on)
	for _, want := range []string{`"timed_out":true`, `"time_limit_seconds":90`} {
		if !strings.Contains(string(onBytes), want) {
			t.Errorf("JSON with the limit must contain %s: %s", want, onBytes)
		}
	}
}
