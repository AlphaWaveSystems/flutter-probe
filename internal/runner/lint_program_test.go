package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// lintTextWarnings returns the count and zero-state warnings that lintProgram gives for one step inside a test.
func lintTextWarnings(t *testing.T, step string) []string {
	t.Helper()
	prog, err := parser.ParseFile("test \"t\"\n  " + step + "\n")
	if err != nil {
		t.Fatalf("parse %q: %v", step, err)
	}
	var got []string
	for _, w := range lintProgram(prog) {
		if strings.Contains(w, "matches any longer text") {
			got = append(got, w)
		}
	}
	return got
}

func TestLintFlagsCountAndZeroStateTextChecks(t *testing.T) {
	cases := []struct {
		name string
		step string
		want int // warnings for the step
	}{
		{"zero count", `see "0 items"`, 1},
		{"count with one word", `see "3 items"`, 1},
		{"bare number", `see "12"`, 1},
		{"count word in capitals", `see "3 Items"`, 1},
		{"zero state phrase", `see "No items found"`, 1},
		{"zero state alone", `see "None"`, 1},
		{"wait until appears", `wait until "0 ml" appears`, 1},
		{"wait until without appears", `wait until "0 ml"`, 1},
		{"anchored check", `see "0 items" matching "^0 items$"`, 0},
		{"anchored wait", `wait until "0 ml" appears matching "^0 ml$"`, 0},
		{"unanchored pattern still warns", `see "0 items" matching "0 items"`, 1},
		{"pattern anchored at one end still warns", `see "0 items" matching "^0 items"`, 1},
		{"negated check", `don't see "0 items"`, 0},
		{"native check", `see native "0 items"`, 0},
		{"ai check", `see "0 items" with ai`, 0},
		{"ordinary label", `see "Sign in"`, 0},
		{"word that starts with no", `see "Notifications"`, 0},
		{"longer sentence", `see "No results for your search"`, 0},
		{"decimal is not matched", `see "12.5"`, 0},
		{"id selector", `see #counter_text`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lintTextWarnings(t, c.step); len(got) != c.want {
				t.Errorf("%s: %d warning(s), want %d: %q", c.step, len(got), c.want, got)
			}
		})
	}
}

func TestLintTextWarningNamesTheLineAndTheAnchoredPattern(t *testing.T) {
	got := lintTextWarnings(t, `see "0 items"`)
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %q", len(got), got)
	}
	for _, want := range []string{`test "t", line 2`, `"0 items"`, `matching "^0 items$"`} {
		if !strings.Contains(got[0], want) {
			t.Errorf("warning %q does not contain %q", got[0], want)
		}
	}
}

// lintProgram warns on a count or zero-state text check that a substring match passes by accident. It does not warn on an
// anchored check, a negated check, or a sentence that only starts with a zero-state word.
func TestLintProgramFlagsUnanchoredCountAndZeroStateChecks(t *testing.T) {
	cases := []struct {
		name, src string
		warn      bool
	}{
		{"zero state", "test \"x\"\n  see \"No items\"\n", true},
		{"count", "test \"x\"\n  see \"3 items\"\n", true},
		{"anchored zero state", "test \"x\"\n  see \"No items\" matching \"^No items$\"\n", false},
		{"negated count", "test \"x\"\n  don't see \"3 items\"\n", false},
		{"longer sentence", "test \"x\"\n  see \"Nothing is wrong with this page\"\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := parser.ParseFile(c.src)
			if err != nil {
				t.Fatal(err)
			}
			warns := lintProgram(prog)
			if got := len(warns) > 0; got != c.warn {
				t.Errorf("lintProgram warned = %t, want %t: %q", got, c.warn, warns)
			}
		})
	}
}
