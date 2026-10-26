package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// scopeFixture has a recipe, two tests (one tagged @smoke, one @payments, which has two steps) and a composite test.
// The line numbers in the tests below count from its first line.
const scopeFixture = `recipe "sign in"
  wait 1 seconds

test "opens the app"
  @smoke
  wait 1 seconds

test "checkout"
  @payments
  wait 1 seconds
  wait 2 seconds

composite test "two devices"
  devices
    A: iPhone 15 Simulator
    B: Pixel 9 Emulator

  A:
    tap "Login"

  B:
    tap "Login"
`

// writeScopeFile writes src to a .probe file in a fresh directory and returns its path.
func writeScopeFile(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flows.probe")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTestAtLineFindsTheTestThatContainsTheLine(t *testing.T) {
	prog, err := parser.ParseFile(scopeFixture)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		line int
		want string
	}{
		{4, "opens the app"}, // the header
		{5, "opens the app"}, // its tag line
		{7, "opens the app"}, // the blank line after it belongs to it
		{8, "checkout"},      // the next header
		{11, "checkout"},     // its last step
		{13, "two devices"},  // a composite test's header
		{22, "two devices"},  // a composite test's last line
		{999, "two devices"}, // past the end of the file
	}
	for _, c := range cases {
		got, err := testAtLine(prog, c.line)
		if err != nil || got != c.want {
			t.Errorf("line %d: got %q, %v; want %q", c.line, got, err, c.want)
		}
	}
	// Lines 1 to 3 are the recipe, which is not a test.
	for _, line := range []int{1, 2, 3} {
		if got, err := testAtLine(prog, line); err == nil {
			t.Errorf("line %d is in a recipe but resolved to test %q", line, got)
		}
	}
}

func TestRunScopeCheckRefusesAmbiguousOrEmptyScopes(t *testing.T) {
	cases := []struct {
		name  string
		scope RunScope
		ok    bool
	}{
		{"the whole file", RunScope{}, true},
		{"a line", RunScope{Line: 4}, true},
		{"a tag", RunScope{Tag: "@smoke"}, true},
		{"the failed tests", RunScope{FailedOnly: true}, true},
		{"a line and a tag", RunScope{Line: 4, Tag: "smoke"}, false},
		{"a tag and the failed tests", RunScope{Tag: "smoke", FailedOnly: true}, false},
		{"a negative line", RunScope{Line: -1}, false},
		{"a tag with no name", RunScope{Tag: "@"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.scope.check(); (err == nil) != c.ok {
				t.Errorf("check() = %v, want ok=%v", err, c.ok)
			}
		})
	}
}

func TestResolveRunScopeSelectsAndPlansTheRequestedTests(t *testing.T) {
	path := writeScopeFile(t, scopeFixture)
	a := &App{}

	sel, plan, err := a.resolveRunScope(path, RunScope{Line: 8})
	if err != nil || !slices.Equal(sel.names, []string{"checkout"}) {
		t.Fatalf("line 8: selection %+v, %v; want checkout", sel, err)
	}
	if len(plan.Tests) != 1 || plan.Tests[0].Name != "checkout" || plan.Tests[0].Steps != 2 {
		t.Errorf("line 8: plan %+v; want checkout with 2 steps", plan.Tests)
	}

	sel, plan, err = a.resolveRunScope(path, RunScope{Tag: "@smoke"})
	if err != nil || !slices.Equal(sel.tags, []string{"smoke"}) {
		t.Fatalf("tag: selection %+v, %v; want the smoke tag", sel, err)
	}
	if len(plan.Tests) != 1 || plan.Tests[0].Name != "opens the app" {
		t.Errorf("tag: plan %+v; want only opens the app", plan.Tests)
	}

	sel, plan, err = a.resolveRunScope(path, RunScope{})
	if err != nil || sel.names != nil || sel.tags != nil {
		t.Errorf("whole file: selection %+v, %v; want no narrowing", sel, err)
	}
	if len(plan.Tests) != 2 {
		t.Errorf("whole file: plan %+v; want both tests", plan.Tests)
	}
}

func TestResolveRunScopeExplainsWhatItCannotRun(t *testing.T) {
	path := writeScopeFile(t, scopeFixture)
	a := &App{}
	cases := []struct {
		name  string
		scope RunScope
		want  string
	}{
		{"a line in a recipe", RunScope{Line: 2}, "is not inside a test"},
		{"a tag no test has", RunScope{Tag: "nope"}, "no test in this file has the tag @nope"},
		{"failed tests before any run", RunScope{FailedOnly: true}, "has not run in Studio yet"},
		{"two kinds of scope", RunScope{Line: 4, Tag: "smoke"}, "choose one run scope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := a.resolveRunScope(path, c.scope)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err %v; want it to contain %q", err, c.want)
			}
		})
	}
}

// A failed-only run re-runs what failed the last time the file ran, and a run of some tests leaves the others'
// outcomes alone.
func TestFailedOnlyKeepsOutcomesOfTestsThatDidNotRun(t *testing.T) {
	path := writeScopeFile(t, scopeFixture)
	a := &App{}
	failedNames := func() []string {
		t.Helper()
		sel, _, err := a.resolveRunScope(path, RunScope{FailedOnly: true})
		if err != nil {
			return nil
		}
		return sel.names
	}

	// A full run: one row of opens the app fails, checkout fails.
	a.failures.record(path, []runner.TestResult{
		{TestName: "opens the app [row 1]", Row: 0, Passed: true},
		{TestName: "opens the app [row 2]", Row: 1, Passed: false, Error: fmt.Errorf("boom")},
		{TestName: "checkout", Row: -1, Passed: false, Error: fmt.Errorf("boom")},
	})
	if got := failedNames(); !slices.Equal(got, []string{"checkout", "opens the app"}) {
		t.Fatalf("after the full run: failed %v; want checkout and opens the app", got)
	}

	// Only checkout runs again and passes; opens the app did not run, so it stays failed.
	a.failures.record(path, []runner.TestResult{{TestName: "checkout", Row: -1, Passed: true}})
	if got := failedNames(); !slices.Equal(got, []string{"opens the app"}) {
		t.Fatalf("after checkout passed: failed %v; want opens the app", got)
	}

	// A skipped test is not a failure.
	a.failures.record(path, []runner.TestResult{{TestName: "opens the app", Row: -1, Skipped: true}})
	sel, _, err := a.resolveRunScope(path, RunScope{FailedOnly: true})
	if err == nil || !strings.Contains(err.Error(), "failed the last time it ran") {
		t.Errorf("after the skip: selection %+v, %v; want the no-failures refusal", sel, err)
	}
}

// Two tests with one name cannot be told apart by name, so a scope that names one is refused; a tag still runs both,
// as the tag filter always has.
func TestScopesRefuseTwoTestsWithOneName(t *testing.T) {
	src := "test \"same\"\n  wait 1 seconds\n\ntest \"same\"\n  wait 2 seconds\n"
	path := writeScopeFile(t, src)
	a := &App{}
	if _, _, err := a.resolveRunScope(path, RunScope{Line: 1}); err == nil || !strings.Contains(err.Error(), "two tests in this file are named") {
		t.Errorf("a line in one of two same-named tests: err %v; want the ambiguity refusal", err)
	}
	a.failures.record(path, []runner.TestResult{{TestName: "same", Row: -1, Passed: false}})
	if _, _, err := a.resolveRunScope(path, RunScope{FailedOnly: true}); err == nil || !strings.Contains(err.Error(), "two tests in this file are named") {
		t.Errorf("failed-only with a shared name: err %v; want the ambiguity refusal", err)
	}

	tagged := writeScopeFile(t, "test \"same\"\n  @smoke\n  wait 1 seconds\n\ntest \"same\"\n  @smoke\n  wait 2 seconds\n")
	if _, plan, err := a.resolveRunScope(tagged, RunScope{Tag: "smoke"}); err != nil || len(plan.Tests) != 2 {
		t.Errorf("a tag over two same-named tests: plan %+v, err %v; want both", plan.Tests, err)
	}
}

// An unscoped run keeps the old behaviour for a file that does not parse: the runner reports it. A scoped run cannot
// pick tests out of such a file, so it fails before anything starts.
func TestUnscopedRunLeavesAFileThatDoesNotParseToTheRunner(t *testing.T) {
	path := writeScopeFile(t, "test \"unterminated\n  tap \"broken\n")
	a := &App{}
	sel, plan, err := a.resolveRunScope(path, RunScope{})
	if err != nil || sel.names != nil || sel.tags != nil || len(plan.Tests) != 0 {
		t.Errorf("unscoped run: selection %+v, plan %+v, err %v; want nothing and no error", sel, plan.Tests, err)
	}
	if _, _, err := a.resolveRunScope(path, RunScope{Line: 1}); err == nil {
		t.Error("a scoped run of a file that does not parse must fail before it starts")
	}
}

// A scope that cannot run is returned by runFileAsync itself; the run never starts in the background.
func TestRunFileAsyncReturnsAScopeErrorAtOnce(t *testing.T) {
	path := writeScopeFile(t, scopeFixture)
	a := &App{conn: &connection{}}
	if err := a.runFileAsync(path, RunScope{FailedOnly: true}); err == nil || !strings.Contains(err.Error(), "has not run in Studio yet") {
		t.Errorf("runFileAsync with no earlier run: err %v; want the not-run-yet refusal", err)
	}
}

// run_file passes its optional line, tag and failedOnly fields on to the run as its scope.
func TestRunFileSendsItsScope(t *testing.T) {
	s, fb, _ := startTest(t)
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	fb.roots = []string{dir}
	file := filepath.Join(dir, "x.probe")
	if err := os.WriteFile(file, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		params map[string]any
		want   RunScope
	}{
		{map[string]any{"path": file}, RunScope{}},
		{map[string]any{"path": file, "line": 4}, RunScope{Line: 4}},
		{map[string]any{"path": file, "tag": "smoke"}, RunScope{Tag: "smoke"}},
		{map[string]any{"path": file, "failedOnly": true}, RunScope{FailedOnly: true}},
	}
	for _, c := range cases {
		if _, out := rpc(t, s, s.token, "run_file", c.params, nil); out["error"] != nil {
			t.Fatalf("run_file %v: %s", c.params, out["error"])
		}
		if fb.scope != c.want {
			t.Errorf("run_file %v: scope %+v, want %+v", c.params, fb.scope, c.want)
		}
	}
}
