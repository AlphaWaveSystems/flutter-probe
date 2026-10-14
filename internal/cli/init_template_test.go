package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// What `probe init` scaffolds is the first thing a new user runs: it must parse, or the first `probe test` fails.
func TestInitScaffoldParses(t *testing.T) {
	for name, src := range map[string]string{"tests/example.probe": sampleTestFile, "tests/recipes/login.probe": sampleRecipeFile} {
		if _, err := parser.ParseFile(src); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
		}
	}
}

// The scaffold must pass the same lint checks as a user's own files, so a new user does not start with warnings.
func TestInitScaffoldLintsClean(t *testing.T) {
	for name, src := range map[string]string{"tests/example.probe": sampleTestFile, "tests/recipes/login.probe": sampleRecipeFile} {
		prog, err := parser.ParseFile(src)
		if err != nil {
			t.Fatalf("%s does not parse: %v", name, err)
		}
		if warns := lintProgram(prog); len(warns) > 0 {
			t.Errorf("%s has lint warnings: %v", name, warns)
		}
	}
}

// probe init in an empty project writes the files the first run needs, and the sample it writes is the one checked above.
func TestInitWritesTheFilesTheFirstRunNeeds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("probe init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "probe.yaml")); err != nil {
		t.Errorf("probe.yaml not written: %v", err)
	}
	for name, want := range map[string]string{"tests/example.probe": sampleTestFile, "tests/recipes/login.probe": sampleRecipeFile} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s not written: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s differs from the scaffold constant", name)
		}
	}
}

// The first run must pass on any app, so the sample test checks no text: it only opens the app.
func TestInitSampleTestChecksNoText(t *testing.T) {
	prog, err := parser.ParseFile(sampleTestFile)
	if err != nil {
		t.Fatalf("sample test does not parse: %v", err)
	}
	if len(prog.Tests) != 1 {
		t.Fatalf("sample test file has %d tests, want 1", len(prog.Tests))
	}
	for _, st := range prog.Tests[0].Body {
		switch st.(type) {
		case parser.AssertStep, parser.WaitStep:
			t.Errorf("sample test checks text: found %T, want only the open step", st)
		}
	}
}
