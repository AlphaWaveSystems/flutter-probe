package cli

import (
	"os"
	"path/filepath"
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
		{"any matching pattern silences the warning", `see "0 items" matching "0 items"`, 0},
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

// A warning never changes lint's exit status: only a file that cannot be read or parsed is an error.
func TestLintWarningsDoNotFailTheCommand(t *testing.T) {
	dir := t.TempDir()
	src := "test \"cart\"\n  see \"0 items\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cart.probe"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lintCmd.RunE(lintCmd, []string{dir}); err != nil {
		t.Errorf("lint returned %v for a warning-only file; warnings must not change the exit status", err)
	}
}
