package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func crashReport(summary string) *runner.CrashReport {
	return &runner.CrashReport{Gone: true, Kind: device.ExitUnknown, Evidence: device.CrashEvidence{Summary: summary}}
}

func lostConnection() error {
	return errors.New("probelink: write: write tcp 127.0.0.1:1->127.0.0.1:2: use of closed network connection")
}

// The three things Studio takes from the engine's result, and nothing else.
func TestRunResultForCopiesTheCrashLineTheTimeoutAndTheHint(t *testing.T) {
	a := &App{}
	a.SetWorkspace("/work/space")
	conn := &connection{platform: device.PlatformAndroid, cfg: &config.Config{}}
	res := runner.TestResult{TestName: "t", File: "/work/space/tests/a.probe", Error: errors.New("boom"), Crash: crashReport("java.lang.OutOfMemoryError at /work/space/lib/x.dart\nsecond line\u202e")}
	rr := a.runResultFor(res, conn)
	if !strings.HasPrefix(rr.Crash, "the app died during this test: ") || !strings.Contains(rr.Crash, "OutOfMemoryError") {
		t.Errorf("crash line = %q", rr.Crash)
	}
	// the last guard: one line, no control or invisible character, no workspace path
	if strings.ContainsAny(rr.Crash, "\n\u202e") || strings.Contains(rr.Crash, "/work/space") {
		t.Errorf("the crash line was not cleaned: %q", rr.Crash)
	}
	if rr.TimedOut || rr.TimeLimitSeconds != 0 || rr.CrashHint {
		t.Errorf("a plain failure got timeout or hint fields: %+v", rr)
	}
	timed := a.runResultFor(runner.TestResult{TestName: "t", Error: errors.New("test exceeded 90s"), TimedOut: true, TimeLimit: 90 * time.Second}, conn)
	if !timed.TimedOut || timed.TimeLimitSeconds != 90 {
		t.Errorf("timeout fields = %+v", timed)
	}
	pass := a.runResultFor(runner.TestResult{TestName: "t", Passed: true}, conn)
	if pass.Crash != "" || pass.TimedOut || pass.CrashHint {
		t.Errorf("a passing test carries failure fields: %+v", pass)
	}
	if a.runResultFor(res, nil).CrashHint {
		t.Error("a hint without a connection")
	}
}

// The hint says how to find out why the app died, only when it would help: a lost-connection failure on Android with evidence off and none attached.
func TestCrashHintOnlyForALostConnectionOnAndroidWithEvidenceOff(t *testing.T) {
	lost := runner.TestResult{Error: lostConnection()}
	for _, tc := range []struct {
		name     string
		res      runner.TestResult
		platform device.Platform
		evidence bool
		want     bool
	}{
		{"lost connection, android, evidence off", lost, device.PlatformAndroid, false, true},
		{"evidence is on", lost, device.PlatformAndroid, true, false},
		{"ios has no crash evidence", lost, device.PlatformIOS, false, false},
		{"an ordinary failure", runner.TestResult{Error: errors.New("Expected to see \"x\"")}, device.PlatformAndroid, false, false},
		{"no error", runner.TestResult{}, device.PlatformAndroid, false, false},
		{"it passed", runner.TestResult{Passed: true, Error: lostConnection()}, device.PlatformAndroid, false, false},
		{"it was skipped", runner.TestResult{Skipped: true, Error: lostConnection()}, device.PlatformAndroid, false, false},
		{"evidence came with it", runner.TestResult{Error: lostConnection(), Crash: crashReport("x")}, device.PlatformAndroid, false, false},
	} {
		if got := wantsCrashHint(tc.res, tc.platform, tc.evidence); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The crash line and the timeout are for the window only: nothing of them goes into what an Explain, a Suggest a fix or the chat sends to a provider.
func TestCrashAndTimeoutFieldsAreNotInWhatTheProviderGets(t *testing.T) {
	const marker = "CRASHSECRET-9f2c"
	a := &App{}
	a.track.begin("/work/space/tests/a.probe", RunPlan{})
	a.track.result(RunResult{Name: "t", File: "/work/space/tests/a.probe", Error: "step failed", Crash: "the app died during this test: " + marker, TimedOut: true, TimeLimitSeconds: 77, CrashHint: true})
	st := a.track.snapshot()
	if got := summarizeRun(st); strings.Contains(got, marker) || strings.Contains(got, "77") || !strings.Contains(got, "step failed") {
		t.Errorf("the chat's run summary = %q", got)
	}
	failures, _, _, _ := a.explainFailures("")
	if len(failures) != 1 {
		t.Fatalf("failures = %+v", failures)
	}
	for _, f := range failures {
		all := f.Test + f.File + f.Error
		if strings.Contains(all, marker) || strings.Contains(all, "77") || f.Error != "step failed" {
			t.Errorf("what Explain would send = %+v", f)
		}
	}
}

// A real run puts the timeout fields on the result the window gets (executeRun uses runResultFor, not the bare mapping).
func TestExecuteRunSendsTheTimeoutFieldsOfAResult(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"slow\"\n  tap \"Go\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 10 * time.Second
	cfg.Defaults.TestTimeout = 300 * time.Millisecond
	conn := &connection{client: &slowFake{}, cfg: cfg, platform: device.PlatformAndroid}
	a.conn = conn
	events, unsub := a.hub.subscribe()
	defer unsub()
	results, err := a.executeRun(context.Background(), file, runSelection{}, RunPlan{})
	var got *RunResult
	var names []string
	for len(events) > 0 && got == nil {
		ev := <-events
		names = append(names, ev.Name)
		if ev.Name == "run:result" {
			r := ev.Data[0].(RunResult)
			got = &r
		}
	}
	if got == nil {
		t.Fatalf("no run:result event (events %v, results %+v, err %v)", names, results, err)
	}
	if got.Passed || !got.TimedOut || got.TimeLimitSeconds < 0.29 || got.TimeLimitSeconds > 0.31 {
		t.Errorf("result = %+v, want a timed-out failure with a 0.3 s limit", *got)
	}
}

// slowFake answers every call after two seconds (or when the call's context ends): a step that outlives the test's time limit.
type slowFake struct{ runFake }

func (f *slowFake) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-time.After(2 * time.Second):
		return json.RawMessage(`{}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *slowFake) Tap(ctx context.Context, _ probelink.SelectorParam) error {
	select {
	case <-time.After(2 * time.Second):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
