package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// captureCleanup points cleanupOut at a buffer until the test ends, and returns it. The tests that use it run no cleanup goroutine they
// leave behind, so restoring the writer when they end is safe.
func captureCleanup(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	saved := cleanupOut
	cleanupOut = buf
	t.Cleanup(func() { cleanupOut = saved })
	return buf
}

// A wait in an after-each hook of a test that passes runs to its end. The hook is cleanup, outside the test's budget, so the limit does not
// refuse it, and the test takes the time the wait takes.
func TestAfterEachWaitRunsToItsEndAndIsNotRefused(t *testing.T) {
	buf := captureCleanup(t)
	client := newSlowClient()
	r := newTimeoutRunner(client, 2*time.Second)
	prog, pass := parseOneTest(t, "test \"t\"\n  tap \"Fine\"\n\nafter each\n  wait 3 seconds\n")

	start := time.Now()
	res := r.runSingleTest(context.Background(), prog, pass, "t.probe", nil, -1)
	elapsed := time.Since(start)

	if !res.Passed || res.TimedOut {
		t.Fatalf("the test must pass, got passed=%t timedOut=%t err=%v", res.Passed, res.TimedOut, res.Error)
	}
	if elapsed < 3*time.Second {
		t.Errorf("the after-each wait ended after %v; it must run its 3 seconds", elapsed)
	}
	if strings.Contains(buf.String(), "at or above") {
		t.Errorf("the cleanup must not refuse its wait:\n%s", buf.String())
	}
}

// A wait in an on-failure hook of a test that ran into its limit runs to its end inside the cleanup budget, and is not refused.
func TestOnFailureWaitRunsInsideTheCleanupBudget(t *testing.T) {
	shortGrace(t, 10*time.Second)
	buf := captureCleanup(t)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n\non failure\n  wait 3 seconds\n")

	start := time.Now()
	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)
	elapsed := time.Since(start)

	if !res.TimedOut {
		t.Fatalf("want a timed-out attempt, got %+v", res)
	}
	if elapsed < time.Second+3*time.Second {
		t.Errorf("the attempt took %v; the on-failure wait must run its 3 seconds after the 1s limit", elapsed)
	}
	if strings.Contains(buf.String(), "at or above") {
		t.Errorf("the cleanup must not refuse its wait:\n%s", buf.String())
	}
}

// A cleanup hook whose step fails does not change the result, but its error is shown: it is never dropped silently.
func TestCleanupHookErrorIsShownAndTheResultStands(t *testing.T) {
	buf := captureCleanup(t)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	prog, pass := parseOneTest(t, "test \"t\"\n  tap \"Fine\"\n\nafter each\n  tap \"Refuse\"\n")

	res := r.runSingleTest(context.Background(), prog, pass, "t.probe", nil, -1)

	if !res.Passed {
		t.Errorf("a failed after-each step does not fail the test, got %v", res.Error)
	}
	if out := buf.String(); !strings.Contains(out, "after each hook") || !strings.Contains(out, "the agent refused the tap") {
		t.Errorf("the hook's error must be shown, got %q", out)
	}
}

// A timeout is still a timeout when a cleanup hook fails: the hook's error is shown and the attempt keeps the limit's error.
func TestTimeoutKeepsItsErrorWhenAHookFails(t *testing.T) {
	shortGrace(t, 5*time.Second)
	buf := captureCleanup(t)
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	r.restartHook = func(context.Context) error { return nil }
	prog, slow := parseOneTest(t, "test \"slow\"\n  tap \"Slow\"\n\nafter each\n  tap \"Refuse\"\n")

	res := r.runSingleTest(context.Background(), prog, slow, "slow.probe", nil, -1)

	var te *testTimeoutError
	if !res.TimedOut || !errors.As(res.Error, &te) {
		t.Errorf("the attempt must keep the limit's error, got timedOut=%t err=%v", res.TimedOut, res.Error)
	}
	if !strings.Contains(buf.String(), "the agent refused the tap") {
		t.Errorf("the failed hook must still be shown, got %q", buf.String())
	}
}
