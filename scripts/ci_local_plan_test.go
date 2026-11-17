package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// planIn runs `ci-local.sh --plan <args>` in a throwaway repository that holds a copy of the script, a base commit and one
// commit that touches the given files, and returns what the plan printed.
func planIn(t *testing.T, touched []string, args ...string) string {
	t.Helper()
	script, err := os.ReadFile("ci-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	git := func(a ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, a...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("scripts/ci-local.sh", string(script))
	write("README.md", "base\n")
	git("add", ".")
	git("commit", "-q", "-m", "base")
	base := git("rev-parse", "HEAD")
	for _, f := range touched {
		write(f, "changed\n")
	}
	git("add", ".")
	git("commit", "-q", "-m", "change")
	cmd := exec.Command("bash", append([]string{"scripts/ci-local.sh", "--plan", "--base", base}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ci-local --plan: %v\n%s", err, out)
	}
	return string(out)
}

// The Studio frontend check protects the generated dictionary, and the VS Code extension tests the generated keywords and snippets,
// all made from the ProbeScript docs. The hosted workflows only run for studio/ and vscode/, so a change to the docs must pull both
// checks into a default run, and nothing else may.
func TestCILocalAddsTheStudioFrontendForDictionaryInputs(t *testing.T) {
	for _, f := range []string{
		"website/src/content/docs/probescript/grammar.md",
		"website/src/content/docs/probescript/dictionary.md",
		"studio/frontend/scripts/gen-probescript-dictionary.mjs",
		"studio/frontend/src/probescript/dictionary.generated.ts",
	} {
		out := planIn(t, []string{f})
		if !strings.Contains(out, "Studio frontend\n") || !strings.Contains(out, "VS Code extension tests\n") || !strings.Contains(out, "the change touches "+f) {
			t.Errorf("%s: plan = %q", f, out)
		}
		if strings.Count(out, "\n") != 8 { // five required checks, the frontend, the extension, the note
			t.Errorf("%s: unexpected plan size: %q", f, out)
		}
	}
}

func TestCILocalDoesNotAddTheStudioFrontendForOtherChanges(t *testing.T) {
	for _, f := range []string{
		"internal/parser/parser.go",
		"website/src/content/docs/tools/cli-reference.md",
		"website/src/content/docs/probescript/image.png",
		"studio/frontend/src/other/x.ts",
	} {
		if out := planIn(t, []string{f}); strings.Contains(out, "Studio frontend") || strings.Contains(out, "VS Code extension tests") {
			t.Errorf("%s: plan = %q", f, out)
		}
	}
	// an explicit selection is left as it is
	if out := planIn(t, []string{"website/src/content/docs/probescript/grammar.md"}, "--job", "Build"); out != "Build\n" {
		t.Errorf("--job: plan = %q", out)
	}
	// --all already has the check once, with no note
	out := planIn(t, []string{"website/src/content/docs/probescript/grammar.md"}, "--all")
	if strings.Count(out, "Studio frontend\n") != 1 || strings.Count(out, "VS Code extension tests\n") != 1 || strings.Contains(out, "added") {
		t.Errorf("--all: plan = %q", out)
	}
}
