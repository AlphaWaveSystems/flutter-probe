package runner_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// TestReporter_StreamResult verifies StreamResult emits one JSON line per
// call when streaming is enabled and is a no-op otherwise.
func TestReporter_StreamResult(t *testing.T) {
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJSON, &buf, false)
	rep.SetStreaming(true)

	rep.StreamResult(runner.TestResult{
		TestName: "login",
		File:     "tests/login.probe",
		Passed:   true,
		Duration: 230 * time.Millisecond,
		Row:      -1,
	})
	rep.StreamResult(runner.TestResult{
		TestName: "checkout",
		File:     "tests/checkout.probe",
		Passed:   false,
		Duration: 1200 * time.Millisecond,
		Error:    errStub("widget not found"),
		Row:      -1,
	})

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 ndjson lines, got %d:\n%s", len(lines), buf.String())
	}

	for i, line := range lines {
		var evt struct {
			Type   string                 `json:"type"`
			Result map[string]interface{} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Fatalf("line %d not valid JSON: %v\n%s", i, err, line)
		}
		if evt.Type != "test_result" {
			t.Errorf("line %d: type = %q, want test_result", i, evt.Type)
		}
		if _, ok := evt.Result["name"]; !ok {
			t.Errorf("line %d: missing result.name", i)
		}
	}
}

// TestReporter_StreamResult_DisabledForNonJSON verifies streaming is silently
// disabled when the format is not JSON, and StreamResult is a no-op.
func TestReporter_StreamResult_DisabledForNonJSON(t *testing.T) {
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatTerminal, &buf, false)
	rep.SetStreaming(true) // ignored — not JSON

	rep.StreamResult(runner.TestResult{TestName: "x", Passed: true, Row: -1})

	if buf.Len() != 0 {
		t.Errorf("expected no output for non-JSON streaming, got: %q", buf.String())
	}
}

// TestReporter_StreamResult_NotEnabled verifies StreamResult on a JSON
// reporter without SetStreaming(true) is a no-op.
func TestReporter_StreamResult_NotEnabled(t *testing.T) {
	var buf bytes.Buffer
	rep := runner.NewReporter(runner.FormatJSON, &buf, false)
	// no SetStreaming

	rep.StreamResult(runner.TestResult{TestName: "x", Passed: true, Row: -1})

	if buf.Len() != 0 {
		t.Errorf("expected no output without SetStreaming, got: %q", buf.String())
	}
}
