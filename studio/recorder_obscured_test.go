package main

import (
	"strings"
	"testing"
)

// An obscured field's type event carries no text. Its line must be a comment that names the lookup, never a type step.
func TestObscuredTypeEventWritesNoTypeStep(t *testing.T) {
	line := eventToProbeScriptLine(map[string]interface{}{
		"action":   "type",
		"text":     "",
		"obscured": true,
		"selector": map[string]interface{}{"kind": "id", "text": "#password"},
	})
	want := "  # type into #password was not recorded"
	if !strings.HasPrefix(line, want) {
		t.Fatalf("obscured type event: got %q, want a comment starting %q", line, want)
	}
}

// A plain field's type event still writes its text.
func TestPlainTypeEventKeepsItsText(t *testing.T) {
	line := eventToProbeScriptLine(map[string]interface{}{
		"action":   "type",
		"text":     "dev@example.invalid",
		"selector": map[string]interface{}{"kind": "id", "text": "#email"},
	})
	want := `  type "dev@example.invalid" into the #email field`
	if line != want {
		t.Fatalf("plain type event: got %q, want %q", line, want)
	}
}
