package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
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

// lintOutput runs probe lint on paths and returns what the command printed to standard output, and its error. The report
// is written with fmt.Print, so the test swaps os.Stdout for a pipe while the command runs.
func lintOutput(t *testing.T, paths ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	captured := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		captured <- b.String()
	}()
	runErr := lintCmd.RunE(lintCmd, paths)
	os.Stdout = saved
	w.Close()
	out := <-captured
	r.Close()
	return out, runErr
}

// A file's errors print before its warnings, a file with errors gets the failure mark, and the command counts the files
// that have errors.
func TestLintCommandPrintsErrorsBeforeWarningsAndCountsFiles(t *testing.T) {
	dir := t.TempDir()
	mixed := filepath.Join(dir, "mixed.probe")
	if err := os.WriteFile(mixed, []byte("test \"x\"\n  type \"<email>\" into the \"Email\" field\n  frobnicate the cart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.probe")
	if err := os.WriteFile(bad, []byte("test \"x\"\n  frobnicate the cart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := lintOutput(t, mixed, bad)
	if err == nil || err.Error() != "2 file(s) have errors" {
		t.Fatalf("err = %v, want \"2 file(s) have errors\"", err)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("a file with errors needs the failure mark:\n%s", out)
	}
	errAt := strings.Index(out, "line 3: unknown step")
	warnAt := strings.Index(out, "line 2: <email> is not defined")
	if errAt < 0 || warnAt < 0 || errAt > warnAt {
		t.Errorf("errors must print before warnings:\n%s", out)
	}
}

// A warning alone does not fail the command, and the run ends with All files valid. A zero-state text check with no
// anchor is reported through the same path.
func TestLintCommandWarnsOnAZeroStateCheckAndStillPasses(t *testing.T) {
	dir := t.TempDir()
	zero := filepath.Join(dir, "zero.probe")
	if err := os.WriteFile(zero, []byte("test \"x\"\n  see \"No items\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := lintOutput(t, zero)
	if err != nil {
		t.Fatalf("a warning must not fail lint: %v", err)
	}
	if !strings.Contains(out, "⚠") || !strings.Contains(out, "matches any longer text") {
		t.Errorf("want the warning mark and the zero-state warning:\n%s", out)
	}
	if !strings.Contains(out, "All files valid.") {
		t.Errorf("a run with only warnings ends with All files valid.:\n%s", out)
	}
}

// lintProgram warns on a count or zero-state text check that a substring match passes by accident. It does not warn on an
// anchored check, a negated check, or a sentence that only starts with a zero-state word.
func TestLintProgramFlagsUnanchoredCountAndZeroStateChecks(t *testing.T) {
	cases := []struct {
		name, src string
		warn      bool
	}{
		{"zero state", "test \"x\"\n  see \"No items\"\n", true},
		{"count", "test \"x\"\n  see \"3 items\"\n", true},
		{"anchored zero state", "test \"x\"\n  see \"No items\" matching \"^No items$\"\n", false},
		{"negated count", "test \"x\"\n  don't see \"3 items\"\n", false},
		{"longer sentence", "test \"x\"\n  see \"Nothing is wrong with this page\"\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := parser.ParseFile(c.src)
			if err != nil {
				t.Fatal(err)
			}
			warns := lintProgram(prog)
			if got := len(warns) > 0; got != c.warn {
				t.Errorf("lintProgram warned = %t, want %t: %q", got, c.warn, warns)
			}
		})
	}
}

// A composite test's device blocks are linted: a bad step in one fails probe lint, and a valid block passes.
func TestLintCommandChecksCompositeDeviceBlocks(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.probe")
	if err := os.WriteFile(good, []byte("composite test \"login\"\n  A:\n    tap \"Login\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.probe")
	if err := os.WriteFile(bad, []byte("composite test \"login\"\n  A:\n    bogus thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := lintOutput(t, good); err != nil {
		t.Errorf("a valid composite test must pass lint: %v", err)
	}
	if _, err := lintOutput(t, bad); err == nil {
		t.Error("an unknown step in a device block must fail lint")
	}
}
