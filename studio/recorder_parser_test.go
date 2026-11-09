package main

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestRecordedTapParsesAsOneStep(t *testing.T) {
	params := map[string]interface{}{
		"action": "tap",
		"selector": map[string]interface{}{
			"kind": "text",
			"text": "Login",
		},
	}

	line := eventToProbeScriptLine(params)
	script := "test \"recorded action\"\n" + line + "\n"

	prog, err := parser.ParseFile(script)
	if err != nil {
		t.Fatalf("generated ProbeScript failed to parse: %v", err)
	}

	if len(prog.Tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(prog.Tests))
	}

	if len(prog.Tests[0].Body) != 1 {
		t.Fatalf("expected 1 step, got %d", len(prog.Tests[0].Body))
	}

	action, ok := prog.Tests[0].Body[0].(parser.ActionStep)
	if !ok {
		t.Fatalf("expected ActionStep, got %T", prog.Tests[0].Body[0])
	}

	if action.Verb != parser.VerbTap {
		t.Errorf("expected tap action, got %v", action.Verb)
	}
}

func TestRecordedActionsParseAsOneStep(t *testing.T) {
	sel := func(kind, text string) map[string]interface{} {
		return map[string]interface{}{"kind": kind, "text": text}
	}

	cases := []struct {
		name   string
		params map[string]interface{}
	}{
		{"tap with quoted text", map[string]interface{}{"action": "tap", "selector": sel("text", `Say "hello"`)}},
		{"tap with backslash", map[string]interface{}{"action": "tap", "selector": sel("text", `path\file`)}},
		{"type into field", map[string]interface{}{"action": "type", "text": "hello", "selector": sel("id", "email")}},
		{"swipe left", map[string]interface{}{"action": "swipe", "direction": "left"}},
		{"scroll down", map[string]interface{}{"action": "scroll", "direction": "down"}},
		{"long press", map[string]interface{}{"action": "long_press", "selector": sel("text", "Item")}},
		{"navigate", map[string]interface{}{"action": "navigate"}},
		{"see", map[string]interface{}{"action": "see", "selector": sel("text", "Done")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := eventToProbeScriptLine(tc.params)
			script := "test \"recorded action\"\n" + line + "\n"

			prog, err := parser.ParseFile(script)
			if err != nil {
				t.Fatalf("failed to parse %q: %v", line, err)
			}

			if len(prog.Tests) != 1 {
				t.Fatalf("expected 1 test, got %d", len(prog.Tests))
			}
			if len(prog.Tests[0].Body) != 1 {
				t.Fatalf("expected 1 step, got %d", len(prog.Tests[0].Body))
			}
			switch tc.params["action"] {
			case "see":
				if _, ok := prog.Tests[0].Body[0].(parser.AssertStep); !ok {
					t.Fatalf("expected AssertStep, got %T", prog.Tests[0].Body[0])
				}
			default:
				action, ok := prog.Tests[0].Body[0].(parser.ActionStep)
				if !ok {
					t.Fatalf("expected ActionStep, got %T", prog.Tests[0].Body[0])
				}

				expectedVerbs := map[string]parser.ActionVerb{
					"tap":        parser.VerbTap,
					"type":       parser.VerbType,
					"swipe":      parser.VerbSwipe,
					"scroll":     parser.VerbScroll,
					"long_press": parser.VerbLongPress,
					"navigate":   parser.VerbGoBack,
				}

				if action.Verb != expectedVerbs[tc.params["action"].(string)] {
					t.Errorf("unexpected action verb: %v", action.Verb)
				}
			}
		})
	}
}

func TestMalformedRecordedScriptIsRejected(t *testing.T) {
	script := "test \"recorded action\"\n  tap \"unterminated\n"

	if _, err := parser.ParseFile(script); err == nil {
		t.Fatal("expected malformed ProbeScript to be rejected")
	}
}
