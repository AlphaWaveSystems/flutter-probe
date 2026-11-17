package parser_test

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// `see #pw is filled` and `is empty` say whether a field holds a character, and nothing about the text. "filled" and "empty" are words only right
// after the selector: they are not reserved, so a variable or a recipe with that name keeps working (store_webview_compat_test.go checks it).
func TestSeeFilledAndEmptyParse(t *testing.T) {
	cases := []struct {
		src     string
		check   parser.StateCheck
		negated bool
		sel     string
	}{
		{`see #pw is filled`, parser.StateFilled, false, "pw"},
		{`see #pw is empty`, parser.StateEmpty, false, "pw"},
		{`see #pw filled`, parser.StateFilled, false, "pw"},
		{`see "Email" is filled`, parser.StateFilled, false, "Email"},
		{`see the "Pw" field is empty`, parser.StateEmpty, false, "Pw"},
		{`see #pw IS FILLED`, parser.StateFilled, false, "pw"},
		{`don't see #pw is filled`, parser.StateFilled, true, "pw"},
		{`dont see #pw is empty`, parser.StateEmpty, true, "pw"},
		{`see #pw is filled optional`, parser.StateFilled, false, "pw"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			prog := mustParse(t, "test \"t\"\n  "+c.src+"\n")
			a, ok := prog.Tests[0].Body[0].(parser.AssertStep)
			if !ok {
				t.Fatalf("step is %T, want an assertion", prog.Tests[0].Body[0])
			}
			if a.Check != c.check || a.Negated != c.negated {
				t.Errorf("Check %v Negated %v; want %v %v", a.Check, a.Negated, c.check, c.negated)
			}
			if a.CheckVal != "" || a.Pattern != "" {
				t.Errorf("CheckVal %q Pattern %q; a presence check carries no text", a.CheckVal, a.Pattern)
			}
			if !strings.EqualFold(strings.TrimPrefix(a.Sel.Text, "#"), c.sel) {
				t.Errorf("selector text %q; want %q", a.Sel.Text, c.sel)
			}
		})
	}
}

// The check never reads the text, so it cannot be combined with a pattern, and only one state is allowed.
func TestSeeFilledRefusesWhatWouldReadTheText(t *testing.T) {
	for _, src := range []string{
		`see #pw is filled matching "^a"`,
		`see #pw is empty matching ".*"`,
		`don't see #pw is filled matching "x"`,
	} {
		t.Run(src, func(t *testing.T) {
			_, err := parser.ParseFile("test \"t\"\n  " + src + "\n")
			if err == nil || !strings.Contains(err.Error(), "never read its text") {
				t.Errorf("Parse(%q) error %v; want a refusal that says the check never reads the text", src, err)
			}
		})
	}
}

// Words that are not at the state position keep their meaning: a variable called filled or empty is stored as before.
func TestFilledAndEmptyAreNotReserved(t *testing.T) {
	prog := mustParse(t, "test \"t\"\n  store \"x\" as filled\n  store \"\" as empty\n  see \"<empty>\"\n")
	steps := prog.Tests[0].Body
	if len(steps) != 3 {
		t.Fatalf("steps %d; want 3", len(steps))
	}
	for i, want := range []string{"filled", "empty"} {
		if a, ok := steps[i].(parser.ActionStep); !ok || a.Verb != parser.VerbStore || a.Name != want {
			t.Errorf("step %d = %+v; want store ... as %s", i, steps[i], want)
		}
	}
	if a, ok := steps[2].(parser.AssertStep); !ok || a.Check != parser.StateNone {
		t.Errorf("step 2 = %+v; want a plain see", steps[2])
	}
}
