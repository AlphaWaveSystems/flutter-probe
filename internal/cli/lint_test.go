package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A warning never changes lint's exit status: only a file that cannot be read or parsed is an error.
func TestLintWarningsDoNotFailTheCommand(t *testing.T) {
	dir := t.TempDir()
	src := "test \"cart\"\n  see \"0 items\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cart.probe"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lintCmd.RunE(lintCmd, []string{dir}); err != nil {
		t.Errorf("lint returned %v for a warning-only file; warnings must not change the exit status", err)
	}
}
