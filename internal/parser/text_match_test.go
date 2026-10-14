package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// The anchored form is the exact check for text: a text selector still matches a substring, and the regular
// expression that follows `matching` must match the whole text of a matched widget. The parser keeps the pattern on
// the step for the agent to apply.
func TestAnchoredTextMatchingParses(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		text    string
		pattern string
	}{
		{"see with an anchored pattern", `see "0 ml" matching "^0 ml$"`, "0 ml", "^0 ml$"},
		{"wait until appears with an anchored pattern", `wait until "0 ml" appears matching "^0 ml$"`, "0 ml", "^0 ml$"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog := mustParse(t, "test \"t\"\n  "+c.src+"\n")
			switch s := prog.Tests[0].Body[0].(type) {
			case parser.AssertStep:
				if s.Sel.Kind != parser.SelectorText || s.Sel.Text != c.text || s.Pattern != c.pattern {
					t.Errorf("AssertStep text %q pattern %q, want %q %q", s.Sel.Text, s.Pattern, c.text, c.pattern)
				}
			case parser.WaitStep:
				if s.Kind != parser.WaitAppears || s.Target != c.text || s.Pattern != c.pattern {
					t.Errorf("WaitStep target %q pattern %q, want %q %q", s.Target, s.Pattern, c.text, c.pattern)
				}
			default:
				t.Fatalf("step is %T, want an assertion or a wait", s)
			}
		})
	}
}
