package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// hookHeadWarnings returns the findings of rep that are the warning about a word after a hook header.
func hookHeadWarnings(rep LintReport) []LintFinding {
	var out []LintFinding
	for _, f := range rep.Program {
		if strings.HasPrefix(f.Message, "unexpected word(s) after the hook header are ignored") {
			out = append(out, f)
		}
	}
	return out
}

// A word after a hook header is accepted and ignored by the parser, and probe lint warns at the header's line, naming the word.
func TestLintWarnsAWordAfterAHookHeader(t *testing.T) {
	src := "before each tests\n  open the app\n\ntest \"t\"\n  open the app\n"
	rep, err := LintSource(&config.Config{}, "t.probe", src)
	if err != nil {
		t.Fatalf("the parser accepts a word after a hook header, got %v", err)
	}
	got := hookHeadWarnings(rep)
	if len(got) != 1 || got[0].Line != 1 || !got[0].Warning || got[0].Message != "unexpected word(s) after the hook header are ignored: tests; the steps under this header are NOT part of the hook, and they do not run" {
		t.Fatalf("want one warning at line 1 naming the word, got %+v", got)
	}
}

// The warning says what happens to the steps under the header. A test keyword among the words makes them run as a test; any other words drop
// them, so they do not run. Either way they are not the hook's, and the warning says so.
func TestLintHookHeaderWarningSaysWhatHappensToTheSteps(t *testing.T) {
	cases := []struct {
		head string
		want string
	}{
		{"on failure test", "unexpected word(s) after the hook header are ignored: test; the steps under this header are NOT part of the hook: they run as a test"},
		{"on failure foo test", "unexpected word(s) after the hook header are ignored: foo test; the steps under this header are NOT part of the hook: they run as a test"},
		{"before each foo", "unexpected word(s) after the hook header are ignored: foo; the steps under this header are NOT part of the hook, and they do not run"},
		{"before all tests foo", "unexpected word(s) after the hook header are ignored: foo; the steps under this header are NOT part of the hook, and they do not run"},
	}
	for _, c := range cases {
		rep, err := LintSource(&config.Config{}, "t.probe", c.head+"\n  open the app\n\ntest \"t\"\n  open the app\n")
		if err != nil {
			t.Fatalf("%q: %v", c.head, err)
		}
		got := hookHeadWarnings(rep)
		if len(got) != 1 || got[0].Message != c.want {
			t.Errorf("%q: want one warning %q, got %+v", c.head, c.want, got)
		}
	}
}

// A header with no word after it gets no such warning.
func TestLintNoHookHeaderWarningForAWholeHeader(t *testing.T) {
	for _, head := range []string{"before all tests", "after all tests", "before each test", "after each test", "before each", "on failure"} {
		rep, err := LintSource(&config.Config{}, "t.probe", head+"\n  open the app\n\ntest \"t\"\n  open the app\n")
		if err != nil {
			t.Fatalf("%q: %v", head, err)
		}
		if got := hookHeadWarnings(rep); len(got) != 0 {
			t.Errorf("%q: a whole header gets no warning, got %+v", head, got)
		}
	}
}
