package cli

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// `probe lint` prefixes a program-level warning with `line N:` (the finding carries the line), but not a message that already says
// `, line N:` (the count and zero-state warnings), which would print the line twice.
func TestProgramFindingText(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    runner.LintFinding
		want string
	}{
		{"a finding with a line", runner.LintFinding{Line: 7, Warning: true, Message: `recipe "Sign In": write the name in lower case`}, `line 7: recipe "Sign In": write the name in lower case`},
		{"a finding with no line", runner.LintFinding{Warning: true, Message: "something about the file"}, "something about the file"},
		{"a message that names its line", runner.LintFinding{Line: 4, Warning: true, Message: `test "count text", line 4: "0 items" matches any longer text`}, `test "count text", line 4: "0 items" matches any longer text`},
		{"a hook message that names its line", runner.LintFinding{Line: 4, Warning: true, Message: `before_each hook, line 4: "None" matches any longer text`}, `before_each hook, line 4: "None" matches any longer text`},
		{"a message that names another line", runner.LintFinding{Line: 4, Warning: true, Message: "see line 9 for the other one"}, "line 4: see line 9 for the other one"},
	} {
		if got := programFindingText(tc.f); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The real findings: the ones the program checks produce for a file with a no-step test and a count check.
func TestProgramFindingsPrintOneLinePrefix(t *testing.T) {
	src := "test \"empty\"\n\ntest \"count text\"\n  see \"0 items\"\n"
	report, err := runner.LintSource(&config.Config{}, "t.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Program) < 2 {
		t.Fatalf("want at least two program findings, got %+v", report.Program)
	}
	for _, f := range report.Program {
		text := programFindingText(f)
		if strings.Count(text, "line ") > 1 {
			t.Errorf("the line is said twice: %q", text)
		}
		if !strings.Contains(text, "line ") {
			t.Errorf("no line in %q", text)
		}
	}
}
