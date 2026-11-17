package scripts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// probe-tests/negative holds the variants of a suite whose every test must FAIL. `probe test <dir>` picks up every file that ends in .probe, so none of them may:
// they carry the extension .probe.expect-fail and are copied to a .probe file to be run (the header of each says how). Each test of a variant says what it expects.
func TestNegativeSuitesAreNeverPickedUpByADirectoryRun(t *testing.T) {
	files, err := filepath.Glob("../native-test-apps/*/probe-tests/negative/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no negative suites yet")
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".probe") {
			t.Errorf("%s ends in .probe: a directory run would pick it up and its tests, which must fail, would fail the run. Name it .probe.expect-fail", f)
		}
		if !strings.HasSuffix(f, ".probe.expect-fail") {
			t.Errorf("%s is not a .probe.expect-fail file", f)
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		problems, tests := negativeSuiteProblems(f, string(b))
		for _, p := range problems {
			t.Error(p)
		}
		if tests == 0 {
			t.Errorf("%s has no test", f)
		}
	}
}

// negativeSuiteProblems checks one negative suite: every `test "` line must have, right before it, an "# Expected: <text>" line whose text is not empty (a blank one
// says nothing about how the test is expected to fail). It returns what is wrong and how many tests the file has.
func negativeSuiteProblems(name, content string) (problems []string, tests int) {
	lines := strings.Split(content, "\n")
	for n, line := range lines {
		if !strings.HasPrefix(line, "test \"") {
			continue
		}
		tests++
		if n == 0 || !strings.HasPrefix(lines[n-1], "# Expected: ") || strings.TrimSpace(strings.TrimPrefix(lines[n-1], "# Expected: ")) == "" {
			problems = append(problems, fmt.Sprintf("%s:%d: %s: the line before a test must be its \"# Expected: ...\" comment, with text: every test must say how it is expected to fail", name, n+1, strings.TrimSpace(line)))
		}
	}
	return problems, tests
}

func TestNegativeSuiteProblems(t *testing.T) {
	cases := []struct {
		name, content string
		problems      int
	}{
		{"a described test is fine", "# Expected: see \"x\": not found\ntest \"N1\"\n  see \"x\"\n", 0},
		{"no comment", "test \"N1\"\n  see \"x\"\n", 1},
		{"comment not directly before the test", "# Expected: see \"x\": not found\n\ntest \"N1\"\n", 1},
		{"comment without its text", "# Expected:\ntest \"N1\"\n", 1},
		{"comment with only spaces after it", "# Expected:    \ntest \"N1\"\n", 1},
		{"a different comment", "# Note: this fails\ntest \"N1\"\n", 1},
		{"two tests, the second undescribed", "# Expected: a\ntest \"N1\"\n  see \"x\"\ntest \"N2\"\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems, _ := negativeSuiteProblems("f.probe.expect-fail", c.content)
			if len(problems) != c.problems {
				t.Errorf("got %d problem(s) %q, want %d", len(problems), problems, c.problems)
			}
		})
	}
	if _, tests := negativeSuiteProblems("f", "# Expected: a\ntest \"N1\"\n# Expected: b\ntest \"N2\"\n"); tests != 2 {
		t.Errorf("counted %d tests, want 2", tests)
	}
}
