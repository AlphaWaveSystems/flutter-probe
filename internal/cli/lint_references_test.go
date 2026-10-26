package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// probe lint fails on a reference a run cannot resolve, and only warns about a placeholder no value defines.
func TestLintCommandFailsOnUnknownStepsButNotOnWarnings(t *testing.T) {
	dir := t.TempDir()
	warn := filepath.Join(dir, "warn.probe")
	if err := os.WriteFile(warn, []byte("test \"x\"\n  type \"<email>\" into the \"Email\" field\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.probe")
	if err := os.WriteFile(bad, []byte("test \"x\"\n  frobnicate the cart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := lintCmd.RunE(lintCmd, []string{warn}); err != nil {
		t.Errorf("a placeholder warning must not fail lint: %v", err)
	}
	if err := lintCmd.RunE(lintCmd, []string{bad}); err == nil {
		t.Error("an unknown step must fail lint")
	}
}
