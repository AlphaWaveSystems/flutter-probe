package cli

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestStatusOK(t *testing.T) {
	var buf bytes.Buffer
	statusOK(&buf, "Connected to %s", "device-1")
	out := buf.String()
	if out == "" {
		t.Error("statusOK produced no output")
	}
	if !bytes.Contains([]byte(out), []byte("Connected to device-1")) {
		t.Errorf("statusOK output missing message: %q", out)
	}
}

func TestStatusWarn(t *testing.T) {
	var buf bytes.Buffer
	statusWarn(&buf, "timeout exceeded")
	out := buf.String()
	if out == "" {
		t.Error("statusWarn produced no output")
	}
	if !bytes.Contains([]byte(out), []byte("timeout exceeded")) {
		t.Errorf("statusWarn output missing message: %q", out)
	}
}

// TestReportIfNotTestFailure covers the fix for PT-14: testCmd sets
// SilenceErrors:true (to avoid a duplicate line when tests merely fail —
// the runner already reported those in detail), but that used to silently
// swallow every OTHER error too (token read, connect, handshake failures),
// leaving `probe test` with zero diagnostic output beyond whatever progress
// lines were printed before the failure.
func TestReportIfNotTestFailure(t *testing.T) {
	t.Run("nil error prints nothing", func(t *testing.T) {
		var buf bytes.Buffer
		reportIfNotTestFailure(&buf, nil)
		if buf.String() != "" {
			t.Errorf("expected no output for nil error, got %q", buf.String())
		}
	})

	t.Run("errTestFailed prints nothing (runner already reported it)", func(t *testing.T) {
		var buf bytes.Buffer
		reportIfNotTestFailure(&buf, errTestFailed)
		if buf.String() != "" {
			t.Errorf("expected no output for errTestFailed, got %q", buf.String())
		}
	})

	t.Run("wrapped errTestFailed still prints nothing", func(t *testing.T) {
		var buf bytes.Buffer
		reportIfNotTestFailure(&buf, fmt.Errorf("running tests: %w", errTestFailed))
		if buf.String() != "" {
			t.Errorf("expected no output for a wrapped errTestFailed, got %q", buf.String())
		}
	})

	t.Run("any other error is printed", func(t *testing.T) {
		var buf bytes.Buffer
		err := errors.New("agent token: android: probe token not found within 30s — is the app running with probe_agent?")
		reportIfNotTestFailure(&buf, err)
		out := buf.String()
		if out == "" {
			t.Fatal("expected the error message to be printed, got no output")
		}
		if !bytes.Contains([]byte(out), []byte(err.Error())) {
			t.Errorf("output %q does not contain the error message %q", out, err.Error())
		}
	})
}

func TestMessageConstants(t *testing.T) {
	// Verify key message constants are non-empty
	messages := []struct {
		name string
		val  string
	}{
		{"msgNoProbeFiles", msgNoProbeFiles},
		{"msgWaitingForTokenIOS", msgWaitingForTokenIOS},
		{"msgWaitingForToken", msgWaitingForToken},
	}
	for _, m := range messages {
		if m.val == "" {
			t.Errorf("%s is empty", m.name)
		}
	}
}
