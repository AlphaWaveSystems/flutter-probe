package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// lintFiles writes files into a fresh directory, then lints the file called main and returns its findings.
func lintFiles(t *testing.T, files map[string]string, main string) []LintFinding {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	findings, err := LintReferences(&config.Config{}, filepath.Join(dir, main))
	if err != nil {
		t.Fatalf("LintReferences: %v", err)
	}
	return findings
}

// lintSource lints one file called flows.probe.
func lintSource(t *testing.T, src string) []LintFinding {
	t.Helper()
	return lintFiles(t, map[string]string{"flows.probe": src}, "flows.probe")
}

// requireFinding fails unless findings holds one finding on line with the given severity whose message contains substr.
func requireFinding(t *testing.T, findings []LintFinding, line int, warning bool, substr string) {
	t.Helper()
	for _, f := range findings {
		if f.Line == line && f.Warning == warning && strings.Contains(f.Message, substr) {
			return
		}
	}
	t.Errorf("no finding on line %d (warning=%t) containing %q; got %+v", line, warning, substr, findings)
}

func requireNoFindings(t *testing.T, findings []LintFinding) {
	t.Helper()
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %+v", findings)
	}
}

func TestLintReferencesFlagsUnknownSteps(t *testing.T) {
	findings := lintSource(t, "test \"checkout\"\n  tap \"Buy\"\n  frobnicate the cart\n")
	requireFinding(t, findings, 3, false, "unknown step")
	if len(findings) != 1 {
		t.Errorf("expected only the unknown step, got %+v", findings)
	}
}

// A use statement inside a test is an ordinary call to a recipe named use, which no run can resolve.
func TestLintReferencesExplainsAUseInsideATest(t *testing.T) {
	findings := lintSource(t, "test \"nav\"\n  use \"lib.probe\"\n  tap \"Go\"\n")
	requireFinding(t, findings, 2, false, "must come before the first test")
}

// go back takes no arguments, so the words after it on its line are read as a separate step. The message says so, and
// the call is not reported as an unknown step on its own.
func TestLintReferencesExplainsWordsAfterGoBack(t *testing.T) {
	findings := lintSource(t, "test \"nav\"\n  go back to main menu\n")
	requireFinding(t, findings, 2, false, "go back takes no arguments")
	if len(findings) != 1 {
		t.Errorf("expected only the go back explanation, got %+v", findings)
	}
}

func TestLintReferencesAcceptsRecipesDefinedInTheFile(t *testing.T) {
	src := "recipe \"sign in\" (email)\n  type \"<email>\" into the \"Email\" field\n\ntest \"login\"\n  sign in \"a@b.c\"\n"
	requireNoFindings(t, lintSource(t, src))
}

func TestLintReferencesAcceptsRecipesFromTheRecipesFolder(t *testing.T) {
	recipes := t.TempDir()
	if err := os.WriteFile(filepath.Join(recipes, "shared.probe"), []byte("recipe \"open menu\"\n  tap \"Menu\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "flows.probe")
	if err := os.WriteFile(path, []byte("test \"nav\"\n  open menu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := LintReferences(&config.Config{Recipes: recipes}, path)
	if err != nil {
		t.Fatal(err)
	}
	requireNoFindings(t, findings)
}

func TestLintReferencesWarnsAboutUseOfAMissingFile(t *testing.T) {
	findings := lintSource(t, "use \"missing.probe\"\n\ntest \"x\"\n  tap \"Go\"\n")
	requireFinding(t, findings, 1, true, "no file at")
}

// A missing use file fails only the calls that need its recipes: they are errors, and the use is just a warning.
func TestLintReferencesErrorsOnACallThatNeedsAMissingUse(t *testing.T) {
	findings := lintSource(t, "use \"missing.probe\"\n\ntest \"x\"\n  open menu\n")
	requireFinding(t, findings, 1, true, "no file at")
	requireFinding(t, findings, 4, false, "unknown step")
}

func TestLintReferencesAcceptsRecipesFromAUseStatement(t *testing.T) {
	files := map[string]string{
		"lib.probe":   "recipe \"open menu\"\n  tap \"Menu\"\n",
		"flows.probe": "use \"lib.probe\"\n\ntest \"nav\"\n  open menu\n",
	}
	requireNoFindings(t, lintFiles(t, files, "flows.probe"))
}

func TestLintReferencesWarnsAboutAPlaceholderNothingDefines(t *testing.T) {
	findings := lintSource(t, "test \"login\"\n  type \"<email>\" into the \"Email\" field\n")
	requireFinding(t, findings, 2, true, "<email> is not defined")
	if len(findings) != 1 {
		t.Errorf("expected only the placeholder warning, got %+v", findings)
	}
}

func TestLintReferencesAcceptsPlaceholdersThatAValueDefines(t *testing.T) {
	cases := map[string]string{
		"example column": "test \"login\"\n  type \"<email>\" into the \"Email\" field\n\nwith examples:\n  email\n  \"a@b.c\"\n",
		"stored value":   "test \"login\"\n  store \"abc\" as token\n  type \"<token>\" into the \"Field\"\n",
		"recipe param":   "recipe \"sign in\" (email)\n  type \"<email>\" into the \"Email\" field\n\ntest \"login\"\n  sign in \"a@b.c\"\n",
		"runtime value":  "test \"status\"\n  see \"<response.status>\"\n",
		"random value":   "test \"gen\"\n  see \"<random.int>\"\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireNoFindings(t, lintSource(t, src))
		})
	}
}

func TestLintReferencesFlagsASelectorThatCanNeverMatch(t *testing.T) {
	cases := []struct {
		name string
		step string
		want string
	}{
		{"ordinal without text", "tap 2nd", "an ordinal selector needs a number and text"},
		{"relational without anchor", "tap \"Save\" below", "the below selector has no anchor text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := lintSource(t, "test \"x\"\n  "+c.step+"\n")
			requireFinding(t, findings, 2, false, c.want)
		})
	}
}

func TestLintReferencesAcceptsWorkingSelectors(t *testing.T) {
	src := "test \"x\"\n  tap 2nd \"Buy\"\n  tap \"Save\" below \"Cancel\"\n  tap \"Buy\" in \"Cart\"\n  tap #buy_button\n"
	requireNoFindings(t, lintSource(t, src))
}

// walkSteps reaches the steps nested in conditionals, loops, retries and timed blocks, not only the top level.
func TestWalkStepsVisitsNestedBodies(t *testing.T) {
	inner := parser.ActionStep{Verb: parser.VerbStore, Text: "x", Name: "v", Line: 3}
	steps := []parser.Step{
		parser.ConditionalStep{Then: []parser.Step{inner}, Line: 1},
		parser.LoopStep{Body: []parser.Step{parser.WaitStep{Line: 4}}, Line: 2},
	}
	var lines []int
	walkSteps(steps, func(s parser.Step) { lines = append(lines, stepLine(s)) })
	want := []int{1, 3, 2, 4}
	if len(lines) != len(want) {
		t.Fatalf("visited lines %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("visited lines %v, want %v", lines, want)
			break
		}
	}
}

func TestLintReferencesReportsAParseErrorWithoutFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flows.probe")
	if err := os.WriteFile(path, []byte("test \"unterminated\n  tap \"broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := LintReferences(&config.Config{}, path)
	if err == nil {
		t.Fatalf("expected the parse error, got findings %+v", findings)
	}
}
