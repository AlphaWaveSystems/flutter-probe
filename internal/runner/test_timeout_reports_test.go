package runner

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"
)

// junitFailureInfo is the failure of one JUnit test case.
type junitFailureInfo struct {
	Type, Message, Text string
}

// junitFailures decodes a JUnit report and returns each test case's failure by test name; a passing case maps to nil.
func junitFailures(t *testing.T, out string) map[string]*junitFailureInfo {
	t.Helper()
	var doc struct {
		Suites []struct {
			Cases []struct {
				Name    string `xml:"name,attr"`
				Failure *struct {
					Type    string `xml:"type,attr"`
					Message string `xml:"message,attr"`
					Text    string `xml:",chardata"`
				} `xml:"failure"`
			} `xml:"testcase"`
		} `xml:"testsuite"`
	}
	if err := xml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]*junitFailureInfo{}
	for _, s := range doc.Suites {
		for _, c := range s.Cases {
			if c.Failure == nil {
				got[c.Name] = nil
				continue
			}
			got[c.Name] = &junitFailureInfo{Type: c.Failure.Type, Message: c.Failure.Message, Text: c.Failure.Text}
		}
	}
	return got
}

// A run without a limit writes no timeout type: a failed assertion is a plain failure, and the report never says test_timeout or time_limit.
func TestJUnitLimitOffHasNoTimeoutType(t *testing.T) {
	var buf bytes.Buffer
	rep := NewReporter(FormatJUnit, &buf, false)
	rep.SetJUnitOptions(JUnitOptions{Base: "/work"})
	if err := rep.Report([]TestResult{
		{TestName: "plain", File: "plain.probe", Passed: false, Error: errors.New("expected Done")},
		{TestName: "ok", File: "ok.probe", Passed: true},
	}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "test_timeout") || strings.Contains(out, "time_limit") {
		t.Errorf("a run without a limit must not write the timeout fields:\n%s", out)
	}
	failures := junitFailures(t, out)
	if f := failures["plain"]; f == nil || f.Type != "" || f.Message != "expected Done" {
		t.Errorf("a failed assertion is a plain failure without a type, got %+v", f)
	}
	if failures["ok"] != nil {
		t.Errorf("a passing test has no failure")
	}
}

// The failure of a timed-out test keeps its step and its cause in the text, and names the limit in the message.
func TestJUnitTimeoutFailureTextKeepsStepAndCause(t *testing.T) {
	var buf bytes.Buffer
	rep := NewReporter(FormatJUnit, &buf, false)
	rep.SetJUnitOptions(JUnitOptions{Base: "/work"})
	if err := rep.Report([]TestResult{{
		TestName: "slow", File: "slow.probe", Passed: false, TimedOut: true, TimeLimit: 15 * time.Second,
		Error: &testTimeoutError{limit: 15 * time.Second, step: "line 4: wait 20 seconds", cause: errors.New("no element \"Save\"")},
	}}); err != nil {
		t.Fatal(err)
	}
	f := junitFailures(t, buf.String())["slow"]
	if f == nil || f.Type != "test_timeout" || f.Message != "test exceeded 15s" {
		t.Fatalf("want a test_timeout failure naming the limit, got %+v", f)
	}
	if !strings.Contains(f.Text, "line 4: wait 20 seconds") || !strings.Contains(f.Text, `no element "Save"`) {
		t.Errorf("the failure text must keep the step and the cause, got %q", f.Text)
	}
}

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

// A wait below the limit waits, as it always did. The step timeout is far above the wait (see waitStepTimeout).
func TestWaitBelowTheLimitStillWaits(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, waitStepTimeout, false)
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

// waitStepTimeout is the step timeout of the tests whose 1 second wait must run to its end. The step timeout is the step's own deadline, not
// the test limit these tests are about; a step timeout equal to the wait made the wait race it under load.
const waitStepTimeout = 30 * time.Second

// Without a limit a wait of any length is a wait, as before. The step timeout is far above the wait (see waitStepTimeout).
func TestWaitRuleIsOffWithoutALimit(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, waitStepTimeout, false)
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
