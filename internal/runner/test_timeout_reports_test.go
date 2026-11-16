package runner

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A literal `wait N seconds` at or above the test's limit fails at once, with its own message: the limit could only end the wait, so the
// wait does not run.
func TestWaitAtOrAboveTheLimitFailsAtOnce(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, time.Second, false)
	exec.testLimit = 2 * time.Second
	_, w := parseOneTest(t, "test \"t\"\n  wait 2 seconds\n")

	start := time.Now()
	err := exec.RunBody(context.Background(), w.Body)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "wait 2 seconds is at or above test_timeout (2s)") {
		t.Fatalf("want the wait refused with its message, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("the refused wait took %v; it must fail at once", elapsed)
	}
	if timeoutError(err) != nil {
		t.Errorf("the refusal is a step failure, not the limit's timeout: %v", err)
	}
}

// A wait below the limit waits, as it always did.
func TestWaitBelowTheLimitStillWaits(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, time.Second, false)
	exec.testLimit = 2 * time.Second
	_, w := parseOneTest(t, "test \"t\"\n  wait 1 seconds\n")

	start := time.Now()
	if err := exec.RunBody(context.Background(), w.Body); err != nil {
		t.Fatalf("a wait below the limit must run: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("the wait ended after %v; it must last its second", elapsed)
	}
}

// Through a run, the refused wait is an ordinary failure: the attempt is not a timeout, carries no limit error, and names the step.
func TestWaitAtTheLimitThroughARunIsAPlainFailure(t *testing.T) {
	client := newSlowClient()
	r := newTimeoutRunner(client, time.Second)
	prog, w := parseOneTest(t, "test \"t\"\n  wait 1 seconds\n")

	res := r.runSingleTest(context.Background(), prog, w, "t.probe", nil, -1)

	if res.Passed || res.TimedOut {
		t.Fatalf("want a plain failure, got passed=%t timedOut=%t err=%v", res.Passed, res.TimedOut, res.Error)
	}
	if timeoutError(res.Error) != nil || !strings.Contains(res.Error.Error(), "at or above test_timeout") {
		t.Errorf("the failure must name the wait and the limit, got %v", res.Error)
	}
}

// Without a limit a wait of any length is a wait, as before.
func TestWaitRuleIsOffWithoutALimit(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, time.Second, false)
	_, w := parseOneTest(t, "test \"t\"\n  wait 1 seconds\n")
	if err := exec.RunBody(context.Background(), w.Body); err != nil {
		t.Errorf("without a limit the wait must run: %v", err)
	}
}

// A test that ran past its limit is a JUnit <failure> whose type is test_timeout, and whose message names the limit; the text keeps the
// step and the cause. An infra error whose text says so stays an <error>, as the existing rule has it.
func TestJUnitMarksATimeoutAsTestTimeout(t *testing.T) {
	timedOut := TestResult{
		TestName: "slow", File: "slow.probe", Passed: false, TimedOut: true, TimeLimit: 15 * time.Second,
		Error: &testTimeoutError{limit: 15 * time.Second, step: `line 4: wait 20 seconds`, cause: errors.New("no element")},
	}
	infra := TestResult{
		TestName: "dropped", File: "dropped.probe", Passed: false, TimedOut: true, TimeLimit: 15 * time.Second,
		Error: &testTimeoutError{limit: 15 * time.Second, step: `line 4: tap "Save"`, cause: errors.New("probelink: write: broken pipe")},
	}
	plain := TestResult{TestName: "plain", File: "plain.probe", Passed: false, Error: errors.New("expected Done")}

	var buf bytes.Buffer
	rep := NewReporter(FormatJUnit, &buf, false)
	rep.SetJUnitOptions(JUnitOptions{Base: "/work"})
	if err := rep.Report([]TestResult{timedOut, infra, plain}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if !strings.Contains(out, `<failure type="test_timeout" message="test exceeded 15s">`) {
		t.Errorf("a timed-out test must be a test_timeout failure naming the limit:\n%s", out)
	}
	if !strings.Contains(out, `<error type="error" message="test exceeded 15s`) {
		t.Errorf("an infra error inside a timeout must stay an <error>:\n%s", out)
	}
	if !strings.Contains(out, `<failure message="expected Done">`) {
		t.Errorf("an assertion must stay a plain <failure>:\n%s", out)
	}
}
