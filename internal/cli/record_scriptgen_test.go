package cli

import (
	"strings"
	"testing"
	"time"
)

// The recorded script is written by the engine's shared mapping (internal/scriptgen), the same one Studio's recorder uses: the header, the test
// and `open the app` are this command's, the gap waits and the steps are the mapping's.
func TestGenerateProbeScriptUsesTheSharedMapping(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []recordedEvent{
		{At: at, Params: map[string]interface{}{"action": "tap", "selector": map[string]interface{}{"kind": "text", "text": "Sign In"}}},
		{At: at.Add(2 * time.Second), Params: map[string]interface{}{"action": "tap", "text": "Two seconds is no pause"}},
		{At: at.Add(5 * time.Second), Params: map[string]interface{}{"action": "type", "text": "hello", "id": "email"}},
		{At: at.Add(5100 * time.Millisecond), Params: map[string]interface{}{"action": "swipe", "direction": "left"}},
		{At: at.Add(6 * time.Second), Params: map[string]interface{}{"action": "pinch", "text": "SECRET"}},
		{At: at.Add(7 * time.Second), Params: map[string]interface{}{"action": "navigate"}},
	}
	got := generateProbeScript(events)
	body := got[strings.Index(got, "test \"recorded flow\""):]
	want := "test \"recorded flow\"\n  open the app\n  tap \"Sign In\"\n  tap \"Two seconds is no pause\"\n  wait 3 seconds\n  type \"hello\" into the #email field\n  swipe left\n  # pinch\n  go back\n"
	if body != want {
		t.Errorf("got:\n%s\nwant:\n%s", body, want)
	}
	if !strings.HasPrefix(got, "# Recorded on ") {
		t.Errorf("no header: %q", got)
	}
}

// A selector without a text key used to panic the recorder (an unchecked type assertion); it now records the step without that selector.
func TestAnEventWithASelectorWithoutTextDoesNotPanic(t *testing.T) {
	events := []recordedEvent{
		{At: time.Now(), Params: map[string]interface{}{"action": "tap", "selector": map[string]interface{}{"kind": "id"}}},
		{At: time.Now(), Params: map[string]interface{}{"action": "long_press", "selector": map[string]interface{}{"kind": "text"}, "text": "Item"}},
		{At: time.Now(), Params: map[string]interface{}{"action": "type", "text": "hi", "selector": map[string]interface{}{}}},
	}
	got := generateProbeScript(events)
	for _, want := range []string{"  tap the element\n", "  long press on \"Item\"\n", "  type \"hi\"\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	printRecordedEvent(events[0]) // the live line takes the same path
}
