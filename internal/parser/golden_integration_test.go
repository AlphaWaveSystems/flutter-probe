package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// TestGoldenIntegration_DartEmittedFiles verifies that every `.probe.golden`
// file produced by the Dart `flutter_probe_gen` builder is a syntactically
// valid ProbeScript file according to the Go-side parser.
//
// This is a cross-language integration test: if the Dart-side emitter
// generates output the Go parser can't accept, this test fails — catching
// real bugs in the emitter that wouldn't show up in Dart-only golden tests.
func TestGoldenIntegration_DartEmittedFiles(t *testing.T) {
	// Locate the golden directory relative to the repo root. The internal/
	// parser tests run from internal/parser/, so we walk up one directory.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	goldenDir := filepath.Join(repoRoot, "probe_gen", "test", "fixtures")

	if _, err := os.Stat(goldenDir); os.IsNotExist(err) {
		t.Skipf("golden directory %s not present (probe_gen package not present in this checkout)", goldenDir)
	}

	matches, err := filepath.Glob(filepath.Join(goldenDir, "*.probe.golden"))
	if err != nil {
		t.Fatalf("globbing goldens: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no .probe.golden files found in %s", goldenDir)
	}

	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			prog, err := parser.ParseFile(string(data))
			if err != nil {
				t.Fatalf("parser rejected golden %s:\n%v\n--- contents ---\n%s",
					filepath.Base(path), err, string(data))
			}
			// Sanity: every golden defines at least one test, composite test,
			// recipe, or hook.
			if len(prog.Tests) == 0 && len(prog.CompositeTests) == 0 &&
				len(prog.Recipes) == 0 && len(prog.Hooks) == 0 {
				t.Errorf("golden %s parsed but produced no tests/composite tests/recipes/hooks",
					filepath.Base(path))
			}
		})
	}

	// Final smoke: the golden directory exists and contains expected files.
	if !strings.Contains(strings.Join(matches, "|"), "login_screen.probe.golden") {
		t.Error("expected login_screen.probe.golden in fixtures")
	}
}
