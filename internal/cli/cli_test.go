package cli

import (
	"bytes"
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
