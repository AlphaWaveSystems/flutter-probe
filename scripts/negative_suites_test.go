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
		if p := restartBlockProblem(f, string(b)); p != "" {
			t.Error(p)
		}
	}
}

// The suites of a Flutter fixture family (widgets/*.probe) and their negative variants restart the app before each test, so that no test depends on what the
// one before it left on screen. A file that has lost the block (a header cut in the middle of a comment did it once: the negative tests ran without a restart, 105 ms
// each) proves less than it says.
func TestFamilySuitesRestartTheAppBeforeEachTest(t *testing.T) {
	files, err := filepath.Glob("../native-test-apps/*/probe-tests/widgets/*.probe")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no family suites")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if p := restartBlockProblem(f, string(b)); p != "" {
			t.Error(p)
		}
	}
}

// restartBlockProblem returns what is wrong when content has no `before each test` block of its own (a line that is exactly that) whose first step is
// `restart the app`, or "" when it has one. A comment that only mentions the words does not count.
func restartBlockProblem(name, content string) string {
	lines := strings.Split(content, "\n")
	for n, line := range lines {
		if line != "before each test" {
			continue
		}
		for _, next := range lines[n+1:] {
			t := strings.TrimSpace(next)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if t == "restart the app" {
				return ""
			}
			return fmt.Sprintf("%s: the first step of `before each test` is %q, not `restart the app`", name, t)
		}
	}
	return fmt.Sprintf("%s: no `before each test` block (a line that is exactly that) with `restart the app` as its first step: the tests would run without a restart", name)
}

func TestRestartBlockProblem(t *testing.T) {
	cases := []struct {
		name, content string
		bad           bool
	}{
		{"the block", "before each test\n  restart the app\n\ntest \"a\"\n", false},
		{"comments and blanks before the step", "before each test\n\n  # why\n  restart the app\n  wait until \"x\" appears\n", false},
		{"no block", "test \"a\"\n  see \"x\"\n", true},
		{"the words only in a comment (the slip)", "# A failure at the `wait until` of `before each test\n  restart the app\n  wait until \"x\" appears\n", true},
		{"another first step", "before each test\n  wait until \"x\" appears\n  restart the app\n", true},
		{"block with no step", "before each test\n", true},
		{"indented block line", "  before each test\n  restart the app\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := restartBlockProblem("f", c.content) != ""; got != c.bad {
				t.Errorf("problem = %v, want %v", got, c.bad)
			}
		})
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
