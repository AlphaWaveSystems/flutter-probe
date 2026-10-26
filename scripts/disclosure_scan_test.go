package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tests run scripts/disclosure-scan.sh against a small git repository built in
// a temporary directory, with a synthetic pattern. They check the exit status and
// the output. The output must never contain a file path, the matched text or the
// pattern: CI logs of a public repository are public.

const syntheticTerm = "quorvex"

// writeFile writes text to name, creating its folders, and returns name.
func writeFile(t *testing.T, name, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

// newScanRepo builds three commits: a base, two files under docs/evidence (one of
// them holds the term in its text), and a folder whose name holds the term.
func newScanRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "-q", ".")
	git("config", "user.email", "test@example.invalid")
	git("config", "user.name", "test")
	commit := func(msg string) {
		t.Helper()
		git("add", "-A")
		git("commit", "-q", "-m", msg)
	}
	writeFile(t, filepath.Join(dir, "base.txt"), "base\n")
	commit("base")
	writeFile(t, filepath.Join(dir, "docs", "evidence", "one", "notes.txt"), "the "+syntheticTerm+" value\n")
	writeFile(t, filepath.Join(dir, "docs", "evidence", "one", "other.txt"), "clean\n")
	commit("two")
	writeFile(t, filepath.Join(dir, "docs", "evidence", syntheticTerm+"-folder", "readme.txt"), "x\n")
	commit("three")
	return dir
}

// scan runs the scanner in repo and returns its combined output and exit status.
func scan(t *testing.T, repo string, args ...string) (string, int) {
	t.Helper()
	script, err := filepath.Abs("disclosure-scan.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{script, "."}, args...)...)
	cmd.Dir = repo
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	code := 0
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = exitErr.ExitCode()
	}
	return out.String(), code
}

// runGit runs git in dir and fails the test if git fails.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// TestDisclosureScanIgnoresDeletedNames checks that deleting a file whose name
// matches is not a match, because the guard is about what a change adds. Adding
// the same file still fails.
func TestDisclosureScanIgnoresDeletedNames(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", ".")
	runGit(t, dir, "config", "user.email", "test@example.invalid")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, filepath.Join(dir, "base.txt"), "base\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	writeFile(t, filepath.Join(dir, "docs", "evidence", syntheticTerm+"-folder", "readme.txt"), "x\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "add")
	runGit(t, dir, "rm", "-q", "--", "docs/evidence/"+syntheticTerm+"-folder/readme.txt")
	runGit(t, dir, "commit", "-q", "-m", "delete")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	if out, code := scan(t, dir, "HEAD~1...HEAD", pats); code != 0 || !strings.Contains(out, "disclosure scan: clean") {
		t.Errorf("deleting a matching file: exit status %d, want clean:\n%s", code, out)
	}
	if out, code := scan(t, dir, "HEAD~2...HEAD~1", pats); code != 1 || !strings.Contains(out, "changed file #1 has a matching name") {
		t.Errorf("adding a matching file: exit status %d, want a name match:\n%s", code, out)
	}
}

func TestDisclosureScanReportsPositionsOnly(t *testing.T) {
	repo := newScanRepo(t)
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")
	absent := writeFile(t, filepath.Join(t.TempDir(), "absent.txt"), "zzzabsent\n")
	invalid := writeFile(t, filepath.Join(t.TempDir(), "invalid.txt"), "(\n")
	extra := writeFile(t, filepath.Join(t.TempDir(), "extra.txt"), "a line with "+syntheticTerm+"\n")
	cases := []struct {
		name     string
		args     []string
		wantExit int
		want     []string
	}{
		{"changed file name", []string{"HEAD~1...HEAD", pats}, 1,
			[]string{"changed file #1 has a matching name"}},
		{"added line", []string{"HEAD~2...HEAD~1", pats}, 1,
			[]string{"changed file #1, added line 1"}},
		{"extra text", []string{"HEAD...HEAD", pats, "--extra", extra}, 1,
			[]string{"the extra text"}},
		{"tree name and text", []string{"HEAD...HEAD", pats, "--tree", "docs/evidence"}, 1,
			[]string{"tree file #3 has a matching name", "tree file #1 has a match in its text"}},
		{"tree clean", []string{"HEAD...HEAD", absent, "--tree", "docs/evidence"}, 0,
			[]string{"disclosure scan: clean"}},
		{"tree path absent", []string{"HEAD...HEAD", absent, "--tree", "docs/nothere"}, 0,
			[]string{"disclosure scan: clean"}},
		{"unknown revision", []string{"HEAD...nosuchrev", pats, "--tree", "docs/evidence"}, 2,
			[]string{"unknown revision"}},
		{"invalid pattern", []string{"HEAD...HEAD", invalid, "--tree", "docs/evidence"}, 2,
			[]string{"invalid pattern"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := scan(t, repo, tc.args...)
			if code != tc.wantExit {
				t.Errorf("exit status = %d, want %d:\n%s", code, tc.wantExit, out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
			for _, leak := range []string{syntheticTerm, "docs/", "notes.txt", "readme.txt", "-folder"} {
				if strings.Contains(out, leak) {
					t.Errorf("output names %q:\n%s", leak, out)
				}
			}
		})
	}
}
