package main

import "testing"

func TestEventToProbeScriptLine(t *testing.T) {
	sel := func(kind, text string) map[string]interface{} {
		return map[string]interface{}{"kind": kind, "text": text}
	}
	cases := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{"tap by text", map[string]interface{}{"action": "tap", "selector": sel("text", "Sign In")}, `  tap "Sign In"`},
		{"tap by key", map[string]interface{}{"action": "tap", "selector": sel("id", "loginButton")}, `  tap #loginButton`},
		{"tap text containing backslashes", map[string]interface{}{"action": "tap", "selector": sel("text", `path\file`)}, `  tap "path\\file"`},
		{"tap text containing quotes", map[string]interface{}{"action": "tap", "selector": sel("text", `Say "hello"`)}, `  tap "Say \"hello\""`},
		{"key already has a hash", map[string]interface{}{"action": "tap", "selector": sel("id", "#loginButton")}, `  tap #loginButton`},
		{"tap by type", map[string]interface{}{"action": "tap", "selector": sel("type", "Switch")}, `  tap the Switch`},
		{"tap ID containing spaces", map[string]interface{}{"action": "tap", "selector": sel("id", "login button")}, `  tap #login button`},
		{"tap type containing spaces", map[string]interface{}{"action": "tap", "selector": sel("type", "Custom Widget")}, `  tap the Custom Widget`},
		{"tap with only text", map[string]interface{}{"action": "tap", "text": "OK"}, `  tap "OK"`},
		{"tap without a selector", map[string]interface{}{"action": "tap"}, `  tap the element`},
		{"tap with ID fallback", map[string]interface{}{"action": "tap", "id": "loginButton"}, `  tap #loginButton`},
		{"empty text selector falls back to ID", map[string]interface{}{"action": "tap", "selector": sel("text", ""), "id": "loginButton"}, `  tap #loginButton`},
		{"empty text selector without fallback", map[string]interface{}{"action": "tap", "selector": sel("text", "")}, `  tap the element`},
		{"type into a field", map[string]interface{}{"action": "type", "text": "a@b.co", "selector": sel("id", "email")}, `  type "a@b.co" into the #email field`},
		{"type with no target", map[string]interface{}{"action": "type", "text": "hi"}, `  type "hi"`},
		{"type text containing tab", map[string]interface{}{"action": "type", "text": "hello\tworld"}, `  type "hello\tworld"`},
		{"type text containing newline", map[string]interface{}{"action": "type", "text": "hello\nworld"}, `  type "hello\nworld"`},
		{"swipe", map[string]interface{}{"action": "swipe", "direction": "left"}, `  swipe left`},
		{"scroll", map[string]interface{}{"action": "scroll", "direction": "down"}, `  scroll down`},
		{"swipe without direction", map[string]interface{}{"action": "swipe"}, `  swipe `},
		{"scroll without direction", map[string]interface{}{"action": "scroll"}, `  scroll `},
		{"long press", map[string]interface{}{"action": "long_press", "selector": sel("text", "Item")}, `  long press on "Item"`},
		{"long press by ID", map[string]interface{}{"action": "long_press", "selector": sel("id", "item")}, `  long press on #item`},
		{"navigate", map[string]interface{}{"action": "navigate"}, `  go back`},
		{"see", map[string]interface{}{"action": "see", "selector": sel("text", "Done")}, `  see "Done"`},
		{"see by type", map[string]interface{}{"action": "see", "selector": sel("type", "Switch")}, `  see the Switch`},
		{"see with ID fallback", map[string]interface{}{"action": "see", "id": "status"}, `  see #status`},
		{"unknown action becomes a comment", map[string]interface{}{"action": "pinch"}, `  # pinch`},
		{"unknown action containing newline", map[string]interface{}{"action": "pinch\n  tap #unexpected"}, "  # pinch\n  tap #unexpected"},
		{"no action is skipped", map[string]interface{}{}, ``},
	}
	for _, c := range cases {
		if got := eventToProbeScriptLine(c.params); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Text from the screen can hold line breaks; a recorded step must stay on one line.
func TestRecordedTextStaysOnOneLine(t *testing.T) {
	got := eventToProbeScriptLine(map[string]interface{}{"action": "tap", "selector": map[string]interface{}{"kind": "text", "text": "  Line one\r\nline two \n"}})
	if got != `  tap "Line one line two"` {
		t.Errorf("got %q", got)
	}
	if sanitizeRecordedText("\n  x \r") != "x" {
		t.Error("sanitizeRecordedText must trim and drop line breaks")
	}
}
