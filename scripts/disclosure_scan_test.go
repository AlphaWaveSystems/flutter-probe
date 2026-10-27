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

// newEmptyRepo returns a repository with one commit that holds base.txt.
func newEmptyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", ".")
	runGit(t, dir, "config", "user.email", "test@example.invalid")
	runGit(t, dir, "config", "user.name", "test")
	writeFile(t, filepath.Join(dir, "base.txt"), "base\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// commitAll stages everything in dir and commits it with msg.
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", msg)
}

// TestDisclosureScanIgnoresDeletedNames checks that deleting a file whose name
// matches is not a match, because the guard is about what a change adds. Adding
// the same file still fails.
func TestDisclosureScanIgnoresDeletedNames(t *testing.T) {
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, "docs", "evidence", syntheticTerm+"-folder", "readme.txt"), "x\n")
	commitAll(t, dir, "add")
	runGit(t, dir, "rm", "-q", "--", "docs/evidence/"+syntheticTerm+"-folder/readme.txt")
	commitAll(t, dir, "delete")
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
	// The invalid pattern contains the term, so an output that printed the pattern would leak it.
	invalid := writeFile(t, filepath.Join(t.TempDir(), "invalid.txt"), syntheticTerm+"(\n")
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

// TestDisclosureScanReadsQuotedNamesAndBinaryFiles adds three files whose names git
// quotes (a tab, a double quote, a non-ASCII letter), each holding the term, then a
// binary file holding the term behind a NUL byte, then a binary file under the tree
// path. Every one of them must be reported, and the output must name none of them.
func TestDisclosureScanReadsQuotedNamesAndBinaryFiles(t *testing.T) {
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, "tab\tname.txt"), "a "+syntheticTerm+" line\n")
	writeFile(t, filepath.Join(dir, `quote"name.txt`), "a "+syntheticTerm+" line\n")
	writeFile(t, filepath.Join(dir, "caf\u00e9.txt"), "a "+syntheticTerm+" line\n")
	commitAll(t, dir, "quoted")
	writeFile(t, filepath.Join(dir, "binary.dat"), "x\x00"+syntheticTerm+"\x00\n")
	commitAll(t, dir, "binary")
	writeFile(t, filepath.Join(dir, "docs", "evidence", "record.dat"), "y\x00"+syntheticTerm+"\n")
	commitAll(t, dir, "tree binary")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	out, code := scan(t, dir, "HEAD~3...HEAD~2", pats)
	if code != 1 || strings.Count(out, "added line 1") != 3 {
		t.Errorf("three quoted names: exit status %d, want three added-line findings:\n%s", code, out)
	}
	out, code = scan(t, dir, "HEAD~2...HEAD~1", pats)
	if code != 1 || !strings.Contains(out, "changed file #1, added line 1") {
		t.Errorf("binary file: exit status %d, want an added-line finding:\n%s", code, out)
	}
	out, code = scan(t, dir, "HEAD...HEAD", pats, "--tree", "docs/evidence")
	if code != 1 || !strings.Contains(out, "tree file #1 has a match in its text") {
		t.Errorf("binary tree file: exit status %d, want a tree text finding:\n%s", code, out)
	}
	for _, leak := range []string{syntheticTerm, "tab", "quote", "caf", "binary.dat", "record.dat"} {
		if strings.Contains(out, leak) {
			t.Errorf("output names %q:\n%s", leak, out)
		}
	}
}

// TestDisclosureScanReportsLineBreakNames checks that a file whose name holds a line
// break is reported by its number, and that the name appears nowhere in the output.
func TestDisclosureScanReportsLineBreakNames(t *testing.T) {
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, "line\nbreak.txt"), "clean\n")
	commitAll(t, dir, "line break")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	out, code := scan(t, dir, "HEAD~1...HEAD", pats)
	if code != 1 || !strings.Contains(out, "changed file #1: its name holds a line break") {
		t.Errorf("line break name: exit status %d, want the line break finding:\n%s", code, out)
	}
	if strings.Contains(out, "break.txt") {
		t.Errorf("output names the file:\n%s", out)
	}
}

// TestDisclosureScanExtraErrorNamesNoPath checks that an --extra file that cannot be
// read fails the scan without printing its path.
func TestDisclosureScanExtraErrorNamesNoPath(t *testing.T) {
	dir := newEmptyRepo(t)
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")
	missing := filepath.Join(t.TempDir(), "secret-title.txt")

	out, code := scan(t, dir, "HEAD...HEAD", pats, "--extra", missing)
	if code != 2 || !strings.Contains(out, "extra file unreadable") {
		t.Errorf("unreadable extra file: exit status %d, want 2 with the unreadable message:\n%s", code, out)
	}
	for _, leak := range []string{"secret-title", "No such", missing} {
		if strings.Contains(out, leak) {
			t.Errorf("output names %q:\n%s", leak, out)
		}
	}
}
