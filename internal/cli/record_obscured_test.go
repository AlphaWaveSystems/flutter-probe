package cli

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// The value of an obscured field never reaches a recording. The agent sends no text for it, so the event becomes a comment
// that names the lookup and tells the person to add the type step by hand, and the recorder prints a warning line.
func TestObscuredTypeEventIsNotRecorded(t *testing.T) {
	const secret = "quorvex-secret-123"
	selector := map[string]interface{}{"kind": "text", "text": "Password"}
	obscured := recordedEvent{Method: probelink.NotifyRecordedEvent, At: time.Now(), Params: map[string]interface{}{
		"action": "type", "text": "", "obscured": true, "selector": selector,
	}}
	// Defence in depth: an event that carried the value with the obscured flag must not write it either.
	leaky := recordedEvent{Method: probelink.NotifyRecordedEvent, At: time.Now(), Params: map[string]interface{}{
		"action": "type", "text": secret, "obscured": true, "selector": selector,
	}}
	// A plain field still records its type step.
	plain := recordedEvent{Method: probelink.NotifyRecordedEvent, At: time.Now(), Params: map[string]interface{}{
		"action": "type", "text": "hello", "selector": selector,
	}}
	events := []recordedEvent{obscured, leaky, plain}

	probe := generateProbeScript(events)
	if strings.Contains(probe, secret) {
		t.Fatalf("the .probe file carries the obscured field's value:\n%s", probe)
	}
	var typeSteps []string
	for _, line := range strings.Split(probe, "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "type ") {
			typeSteps = append(typeSteps, l)
		}
	}
	if got, want := strings.Join(typeSteps, "\n"), `type "hello" into the "Password" field`; got != want {
		t.Fatalf("type steps = %q, want only the plain field's step %q:\n%s", got, want, probe)
	}
	for _, want := range []string{`# type into "Password" was not recorded`, `Add type "${VAR}" into "Password" by hand.`} {
		if !strings.Contains(probe, want) {
			t.Errorf("the .probe file is missing %q:\n%s", want, probe)
		}
	}

	out := captureStdoutForTest(t, func() {
		for _, ev := range events {
			printRecordedEvent(ev)
		}
	})
	if strings.Contains(out, secret) || strings.Contains(out, `type ""`) {
		t.Fatalf("the recording output carries the obscured field's value or an empty type:\n%s", out)
	}
	if !strings.Contains(out, `type into "Password" not recorded`) {
		t.Fatalf("the recording output should warn that the obscured value was not recorded:\n%s", out)
	}
}

// captureStdoutForTest runs fn with os.Stdout redirected to a pipe, and returns what fn wrote to it.
func captureStdoutForTest(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	old := os.Stdout
	os.Stdout = w
	out := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	func() {
		defer func() {
			os.Stdout = old
			w.Close()
		}()
		fn()
	}()
	return <-out
}
