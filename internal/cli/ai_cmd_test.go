package cli

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func sampleResults() []runner.TestResult {
	return []runner.TestResult{
		{TestName: "ok", Passed: true},
		{TestName: "boom", Passed: false, Error: errors.New(`line 9: tap "Save": Widget not found`)},
		{TestName: "skipped", Skipped: true},
	}
}

// The core promise of FP-14: triage is advisory and can never alter a run.
func TestMaybeAITriage_DeadEndpointLeavesResultsUntouchedAndReturns(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{Provider: "local", Model: "m", Endpoint: "http://127.0.0.1:1/v1", Timeout: 500 * time.Millisecond}}
	results := sampleResults()
	before := append([]runner.TestResult(nil), results...)
	dir := t.TempDir()

	var out bytes.Buffer
	start := time.Now()
	maybeAITriage(context.Background(), &out, cfg, results, dir)

	if !reflect.DeepEqual(before, results) {
		t.Fatal("triage must not modify results")
	}
	if runner.AllPassed(results) {
		t.Fatal("triage must not turn a failed run into a passed one")
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("a dead endpoint must fail fast, took %s", time.Since(start))
	}
	if !strings.Contains(out.String(), "AI triage unavailable") {
		t.Errorf("expected a one-line note, got %q", out.String())
	}
}

func TestMaybeAITriage_NoAIConfigIsASkipNote(t *testing.T) {
	var out bytes.Buffer
	maybeAITriage(context.Background(), &out, &config.Config{}, sampleResults(), t.TempDir())
	if !strings.Contains(out.String(), "tests are unaffected") {
		t.Errorf("got %q", out.String())
	}
}

func TestMaybeAITriage_AllPassedDoesNothingEvenWithBrokenConfig(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{Provider: "nonsense"}}
	var out bytes.Buffer
	maybeAITriage(context.Background(), &out, cfg, []runner.TestResult{{TestName: "ok", Passed: true}}, t.TempDir())
	if out.Len() != 0 {
		t.Errorf("a green run must print nothing, got %q", out.String())
	}
}

func TestMaybeAITriage_WritesAdviceFromALocalModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Likely a timing issue; try wait for idle."}}]}`))
	}))
	defer srv.Close()
	cfg := &config.Config{AI: config.AIConfig{Provider: "local", Model: "m", Endpoint: srv.URL, Timeout: 5 * time.Second}}
	dir := t.TempDir()

	var out bytes.Buffer
	maybeAITriage(context.Background(), &out, cfg, sampleResults(), dir)

	md, err := os.ReadFile(filepath.Join(dir, "triage.md"))
	if err != nil {
		t.Fatalf("triage.md not written: %v\n%s", err, out.String())
	}
	if !strings.Contains(string(md), "## boom") || !strings.Contains(string(md), "wait for idle") {
		t.Errorf("unexpected triage.md:\n%s", md)
	}
}

func TestFailuresFromJSONReport(t *testing.T) {
	data := []byte(`{"results":[
	  {"name":"a","file":"a.probe","passed":true},
	  {"name":"b","file":"b.probe","passed":false,"error":"line 3: boom"},
	  {"name":"c","file":"c.probe","passed":false,"skipped":true}]}`)
	got, err := failuresFromJSONReport(data)
	if err != nil || len(got) != 1 || got[0].Test != "b" || got[0].Error != "line 3: boom" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if _, err := failuresFromJSONReport([]byte("not json")); err == nil {
		t.Error("garbage input must be an error")
	}
}
