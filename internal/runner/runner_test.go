package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flutterprobe/probe/internal/parser"
	"github.com/flutterprobe/probe/internal/runner"
)

// ---- Reporter tests (no network required) ----

func TestReporter_Terminal(t *testing.T) {
	results := []runner.TestResult{
		{TestName: "test 1", File: "tests/a.probe", Passed: true},
		{TestName: "test 2", File: "tests/a.probe", Passed: false, Error: errStub("assertion failed")},
		{TestName: "test 3", File: "tests/b.probe", Passed: true},
	}

	rep := runner.NewReporter(runner.FormatTerminal, os.Stderr, false)
	if err := rep.Report(results); err != nil {
		t.Fatalf("terminal report: %v", err)
	}
}

func TestReporter_JUnit(t *testing.T) {
	results := []runner.TestResult{
		{TestName: "passes", File: "tests/a.probe", Passed: true},
		{TestName: "fails", File: "tests/a.probe", Passed: false, Error: errStub("boom")},
		{TestName: "skipped", File: "tests/b.probe", Skipped: true},
	}

	rep := runner.NewReporter(runner.FormatJUnit, os.Stderr, false)
	if err := rep.Report(results); err != nil {
		t.Fatalf("junit report: %v", err)
	}
}

func TestReporter_JSON(t *testing.T) {
	results := []runner.TestResult{
		{TestName: "test", File: "tests/a.probe", Passed: true},
	}
	rep := runner.NewReporter(runner.FormatJSON, os.Stderr, false)
	if err := rep.Report(results); err != nil {
		t.Fatalf("json report: %v", err)
	}
}

func TestReporter_Summary(t *testing.T) {
	results := []runner.TestResult{
		{Passed: true},
		{Passed: true},
		{Passed: false, Error: errStub("fail")},
		{Skipped: true},
	}
	s := runner.Summary(results)
	if s == "" {
		t.Error("summary is empty")
	}
	// Should mention "2 passed"
	if s[:7] != "2 passe" {
		t.Errorf("summary: %q", s)
	}
}

func TestAllPassed(t *testing.T) {
	if !runner.AllPassed([]runner.TestResult{{Passed: true}, {Skipped: true}}) {
		t.Error("expected AllPassed to return true")
	}
	if runner.AllPassed([]runner.TestResult{{Passed: true}, {Passed: false, Error: errStub("x")}}) {
		t.Error("expected AllPassed to return false")
	}
}

// ---- CollectFiles tests ----

func TestCollectFiles_Dir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.probe"), `test "a"\n  open the app\n`)
	writeFile(t, filepath.Join(dir, "b.probe"), `test "b"\n  open the app\n`)
	writeFile(t, filepath.Join(dir, "not_a_test.txt"), "ignore me")

	files, err := runner.CollectFiles([]string{dir})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 .probe files, got %d", len(files))
	}
}

func TestCollectFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.probe")
	writeFile(t, path, `test "t"\n  open the app\n`)

	files, err := runner.CollectFiles([]string{path})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Errorf("expected [%s], got %v", path, files)
	}
}

func TestCollectFiles_Nested(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	writeFile(t, filepath.Join(dir, "a.probe"), "")
	writeFile(t, filepath.Join(sub, "b.probe"), "")

	files, err := runner.CollectFiles([]string{dir})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2, got %d: %v", len(files), files)
	}
}

// ---- Parse integration: parse real .probe files ----

func TestParseIntegration_LoginFlow(t *testing.T) {
	src := `test "a user can sign in with valid credentials"
  open the app
  wait until "Sign In" appears
  tap on "Sign In"
  type "user@example.com" into the "Email" field
  type "mypassword" into the "Password" field
  tap on "Continue"
  see "Dashboard"
`
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Tests) != 1 {
		t.Errorf("tests: %d", len(prog.Tests))
	}
	if len(prog.Tests[0].Body) < 6 {
		t.Errorf("steps: %d", len(prog.Tests[0].Body))
	}
}

func TestParseIntegration_DataDriven(t *testing.T) {
	src := `test "login validation"
  open the app
  type <email> into the "Email" field
  tap "Continue"
  see <expected>

with examples:
  email              expected
  "valid@test.com"   "Dashboard"
  ""                 "Email is required"
`
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ex := prog.Tests[0].Examples
	if ex == nil {
		t.Fatal("examples block is nil")
	}
	if len(ex.Rows) != 2 {
		t.Errorf("rows: %d", len(ex.Rows))
	}
}

func TestParseIntegration_RecipeFile(t *testing.T) {
	src := `recipe "log in as" (email, password)
  open the app
  tap "Sign In"
  type <email> into the "Email" field
  type <password> into the "Password" field
  tap "Continue"
  see "Dashboard"
`
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(prog.Recipes) != 1 {
		t.Fatalf("recipes: %d", len(prog.Recipes))
	}
	r := prog.Recipes[0]
	if len(r.Params) != 2 {
		t.Errorf("params: %d", len(r.Params))
	}
}

// ---- Helpers ----

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type testErr string

func errStub(msg string) error { return testErr(msg) }
func (e testErr) Error() string { return string(e) }
