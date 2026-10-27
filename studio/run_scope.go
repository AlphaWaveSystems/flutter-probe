package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// A run can cover part of a file: the test at a line, the tests with a tag, or the tests that failed last time. The
// scope is resolved against the file's text when the run starts, so a line or a name always refers to what the file
// holds now.

// RunScope narrows a run to part of a file. Set at most one field; the zero value runs the whole file.
type RunScope struct {
	// Line runs the test that contains this 1-based line: the test whose header is nearest above it.
	Line int
	// Tag runs the tests that carry this tag. A leading "@" is optional.
	Tag string
	// FailedOnly re-runs the tests that failed the last time this file ran in this Studio session.
	FailedOnly bool
}

// check reports a scope that cannot run: a line before the first line, a tag without a name, or more than one kind.
func (s RunScope) check() error {
	kinds := 0
	if s.Line != 0 {
		if s.Line < 0 {
			return fmt.Errorf("the line must be 1 or more")
		}
		kinds++
	}
	if s.Tag != "" {
		if tagName(s.Tag) == "" {
			return fmt.Errorf("the tag needs a name, such as smoke")
		}
		kinds++
	}
	if s.FailedOnly {
		kinds++
	}
	if kinds > 1 {
		return fmt.Errorf("choose one run scope: a line, a tag or the failed tests")
	}
	return nil
}

// tagName is a tag as the file writes it: without surrounding space and without a leading "@".
func tagName(tag string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tag), "@"))
}

// runSelection says which tests of a file run. A nil field does not narrow the run.
type runSelection struct {
	names []string // only the tests with these names
	tags  []string // only the tests with one of these tags
}

// selectsTest reports whether the selection runs t.
func (s runSelection) selectsTest(t parser.TestDef) bool {
	switch {
	case s.names != nil:
		return slices.Contains(s.names, t.Name)
	case s.tags != nil:
		return slices.ContainsFunc(t.Tags, func(tag string) bool { return slices.Contains(s.tags, tag) })
	}
	return true
}

// selection resolves the scope against the parsed file. failed holds what earlier runs of this session found.
func (s RunScope) selection(path string, prog *parser.Program, failed *failureLedger) (runSelection, error) {
	switch {
	case s.Line != 0:
		name, err := testAtLine(prog, s.Line)
		if err != nil {
			return runSelection{}, err
		}
		if err := requireUniqueName(prog, name); err != nil {
			return runSelection{}, err
		}
		return runSelection{names: []string{name}}, nil
	case tagName(s.Tag) != "":
		tag := tagName(s.Tag)
		if !anyTestHasTag(prog, tag) {
			return runSelection{}, fmt.Errorf("no test in this file has the tag @%s", tag)
		}
		return runSelection{tags: []string{tag}}, nil
	case s.FailedOnly:
		return failedSelection(path, prog, failed)
	}
	return runSelection{}, nil
}

// failedSelection re-runs the tests that failed the last time path ran and that the file still has.
func failedSelection(path string, prog *parser.Program, failed *failureLedger) (runSelection, error) {
	failedNames, ran := failed.failedNames(path)
	if !ran {
		return runSelection{}, fmt.Errorf("this file has not run in Studio yet, so there is no failed test to re-run")
	}
	var names []string
	for _, name := range failedNames {
		if testsNamed(prog, name) == 0 {
			continue // renamed or removed since it failed
		}
		if err := requireUniqueName(prog, name); err != nil {
			return runSelection{}, err
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return runSelection{}, fmt.Errorf("no test in this file failed the last time it ran")
	}
	return runSelection{names: names}, nil
}

// testAtLine names the test that contains line: the test whose header is nearest above it. Blank lines after a test
// belong to it. A line above the first header, or inside a use, a recipe or a hook, is not in a test.
func testAtLine(prog *parser.Program, line int) (string, error) {
	type header struct {
		line   int
		name   string
		isTest bool
	}
	var headers []header
	for _, u := range prog.Uses {
		headers = append(headers, header{line: u.Line})
	}
	for _, r := range prog.Recipes {
		headers = append(headers, header{line: r.Line})
	}
	for _, h := range prog.Hooks {
		headers = append(headers, header{line: h.Line})
	}
	for _, t := range prog.Tests {
		headers = append(headers, header{line: t.Line, name: t.Name, isTest: true})
	}
	for _, c := range prog.CompositeTests {
		headers = append(headers, header{line: c.Line, name: c.Name, isTest: true})
	}
	sort.SliceStable(headers, func(i, j int) bool { return headers[i].line < headers[j].line })
	var nearest *header
	for i := range headers {
		if headers[i].line > line {
			break
		}
		nearest = &headers[i]
	}
	if nearest == nil || !nearest.isTest {
		return "", fmt.Errorf("line %d is not inside a test", line)
	}
	return nearest.name, nil
}

// testsNamed counts the tests and composite tests of prog called name.
func testsNamed(prog *parser.Program, name string) int {
	n := 0
	for _, t := range prog.Tests {
		if t.Name == name {
			n++
		}
	}
	for _, c := range prog.CompositeTests {
		if c.Name == name {
			n++
		}
	}
	return n
}

// requireUniqueName refuses a scope that names a test the file shares with another one: running it would run both.
func requireUniqueName(prog *parser.Program, name string) error {
	if testsNamed(prog, name) > 1 {
		return fmt.Errorf("two tests in this file are named %q, so a run of one would run both; rename one of them", name)
	}
	return nil
}

// anyTestHasTag reports whether a test of prog carries tag.
func anyTestHasTag(prog *parser.Program, tag string) bool {
	for _, t := range prog.Tests {
		if slices.Contains(t.Tags, tag) {
			return true
		}
	}
	return false
}

// planFor lists the tests of prog that sel runs, with their step counts, for the toolbar's "step N of M".
func planFor(prog *parser.Program, sel runSelection) RunPlan {
	var plan RunPlan
	if prog == nil {
		return plan
	}
	for _, t := range prog.Tests {
		if sel.selectsTest(t) {
			plan.Tests = append(plan.Tests, RunPlanTest{Name: t.Name, Line: t.Line, Steps: len(t.Body)})
		}
	}
	return plan
}

// parseRunFile reads and parses path the way the runner does, so the scope and the plan see the file the run sees.
func parseRunFile(path string) (*parser.Program, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	prog, err := parser.ParseFile(string(src))
	if err != nil {
		return nil, err
	}
	if prog == nil {
		return nil, fmt.Errorf("cannot read %s", path)
	}
	return prog, nil
}

// resolveRunScope turns scope into the tests a run of path executes and the plan to show for them. A run without a
// scope always goes ahead, so a file that does not parse is left to the runner to report, as it was before scopes.
func (a *App) resolveRunScope(path string, scope RunScope) (runSelection, RunPlan, error) {
	if err := scope.check(); err != nil {
		return runSelection{}, RunPlan{}, err
	}
	prog, err := parseRunFile(path)
	if err != nil {
		if scope == (RunScope{}) {
			return runSelection{}, RunPlan{}, nil
		}
		return runSelection{}, RunPlan{}, err
	}
	sel, err := scope.selection(path, prog, &a.failures)
	if err != nil {
		return runSelection{}, RunPlan{}, err
	}
	return sel, planFor(prog, sel), nil
}

// failureLedger remembers, per file, which tests failed the last time they ran in this Studio session. A run replaces
// the state of the tests it ran and leaves the rest alone, so running one test does not forget the others.
type failureLedger struct {
	mu    sync.Mutex
	files map[string]map[string]bool // file -> test name -> failed last time
}

// record stores the outcome of every test that ran in path. A data-driven test counts as failed when any row did.
func (l *failureLedger) record(path string, results []runner.TestResult) {
	if len(results) == 0 {
		return
	}
	outcome := map[string]bool{}
	for _, res := range results {
		name := res.DefName()
		outcome[name] = outcome[name] || (!res.Passed && !res.Skipped)
	}
	key := filepath.Clean(path)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.files == nil {
		l.files = map[string]map[string]bool{}
	}
	if l.files[key] == nil {
		l.files[key] = map[string]bool{}
	}
	for name, failed := range outcome {
		l.files[key][name] = failed
	}
}

// failedNames returns the tests that failed the last time path ran, sorted, and whether path has run in this session.
func (l *failureLedger) failedNames(path string) ([]string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tests, ran := l.files[filepath.Clean(path)]
	var names []string
	for name, failed := range tests {
		if failed {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, ran
}
