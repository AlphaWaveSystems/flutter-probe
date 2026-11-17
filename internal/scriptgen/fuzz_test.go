package scriptgen

import (
	"strings"
	"testing"
	"unicode"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// FuzzStep checks the recorder's mapping from an event to a ProbeScript line on whatever the agent sends. `go test` runs the seed corpus below and
// the corpus saved under testdata/fuzz/FuzzStep (a few milliseconds); to look for more, run it for as long as you like:
//
//	go test ./internal/scriptgen -run '^$' -fuzz '^FuzzStep$' -fuzztime 2m
//
// A failing input is saved under testdata/fuzz/FuzzStep and runs with every plain `go test` from then on: commit it with the fix.
//
// For every event: no panic; at most two lines (the step and the placeholder note), none with a line break, a control character or a separator in
// it, so a recording never writes a second line from one event; and every line the script gets reads back through the real parser as at most one
// step. For a type step, and a tap, long press or see with a text or an id lookup, what the parser reads is exactly what the event held.
func FuzzStep(f *testing.F) {
	for _, s := range []struct {
		action, kind                 uint8
		selText, flat, id, dir, text string
		obscured                     bool
	}{
		{0, 1, "Sign In", "", "", "", "", false},
		{0, 0, "loginButton", "", "", "", "", false},
		{1, 0, "field", "", "", "", `Say "hi"`, false},
		{1, 1, "Name", "", "", "", "line one\nline two\ttabbed", false},
		{1, 1, "Name", "", "", "", `back\slash and \n and trailing\`, false},
		{1, 1, "Name", "", "", "", "‮​  bidi, zero width, no-break", false},
		{1, 1, "Name", "", "", "", "💳 é世\x00\x1b", false},
		{1, 1, "Name", "", "", "", "a b", false},
		{1, 1, "Name", "", "", "", "\xff\xfe", false},
		{1, 0, "Password", "", "", "", "hunter2", true},
		{0, 2, "Switch", "", "", "", "", false},
		{0, 2, `Foo" bar`, "", "", "", "", false},
		{0, 0, "has space and \"quote\"", "", "", "", "", false},
		{0, 0, "#already", "", "", "", "", false},
		{0, 3, "", "flat text", "", "", "", false},
		{0, 3, "", "", "flatId", "", "", false},
		{2, 3, "", "", "", "up", "", false},
		{2, 3, "", "", "", "up\nsteps: 1\n  tap \"x\"", "", false},
		{3, 3, "", "", "", strings.Repeat("d", 5000), "", false},
		{4, 1, "Save", "", "", "", "", false},
		{5, 3, "", "", "", "", "", false},
		{6, 1, "Total", "", "", "", "", false},
		{7, 3, "", "", "", "", "", false},
		{8, 3, "x\ny‮" + strings.Repeat("z", 200), "", "", "", "", false},
		{1, 1, "Name", "", "", "", strings.Repeat("long text ", 20000), false},
		{0, 1, "${HOME} and <random.email>", "", "", "", "", false},
		{1, 1, "Name", "", "", "", "${SECRET} <name>", false},
	} {
		f.Add(s.action, s.kind, s.selText, s.flat, s.id, s.dir, s.text, s.obscured)
	}
	actions := []string{"tap", "type", "swipe", "scroll", "long_press", "navigate", "see", "", "mystery\naction"}
	kinds := []string{"id", "text", "type", ""}
	f.Fuzz(func(t *testing.T, action, kind uint8, selText, flat, id, dir, text string, obscured bool) {
		params := map[string]interface{}{"action": actions[int(action)%len(actions)]}
		k := kinds[int(kind)%len(kinds)]
		if k != "" || selText != "" {
			params["selector"] = map[string]interface{}{"kind": k, "text": selText}
		}
		if flat != "" {
			params["text"] = flat
		}
		if params["action"] == "type" {
			params["text"] = text
		}
		if id != "" {
			params["id"] = id
		}
		if dir != "" {
			params["direction"] = dir
		}
		if obscured {
			params["obscured"] = true
		}
		lines := Lines(params)
		if len(lines) > 2 || (len(lines) == 2 && lines[1] != PlaceholderNote) {
			t.Fatalf("one event wrote %d lines: %q", len(lines), lines)
		}
		for _, line := range lines {
			for _, r := range line {
				if unicode.IsControl(r) || r == ' ' || r == ' ' {
					t.Fatalf("a line holds the control or separator character %U: %q", r, line)
				}
			}
			prog, err := parser.ParseFile("test \"t\"\n" + line + "\n")
			if err != nil {
				t.Fatalf("a line the recorder wrote does not parse: %q: %v", line, err)
			}
			if len(prog.Tests) != 1 || len(prog.Tests[0].Body) > 1 {
				t.Fatalf("one line read back as %d steps: %q", len(prog.Tests[0].Body), line)
			}
		}
		if len(lines) == 0 || strings.HasPrefix(strings.TrimSpace(lines[0]), "#") {
			return
		}
		prog, _ := parser.ParseFile("test \"t\"\n" + lines[0] + "\n")
		if len(prog.Tests[0].Body) != 1 {
			return
		}
		act, ok := prog.Tests[0].Body[0].(parser.ActionStep)
		if !ok {
			return
		}
		switch params["action"] {
		case "type":
			if Obscured(params) {
				t.Fatalf("an obscured field wrote a step: %q", lines[0])
			}
			if act.Text != text {
				t.Fatalf("typed %q, the line %q reads back %q", text, lines[0], act.Text)
			}
		case "tap", "long_press":
			// the lookup the line holds is the lookup of the event, whichever of its fields supplied it
			if want := Selector(Scrub(params)); act.Sel != nil && strings.HasPrefix(want, `"`) {
				if q, _ := Quote(act.Sel.Text); q != want {
					t.Fatalf("the lookup %s reads back as %q: %q", want, act.Sel.Text, lines[0])
				}
			}
		}
	})
}
