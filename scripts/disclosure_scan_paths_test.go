package scripts

import (
	"path/filepath"
	"strings"
	"testing"
)

// checkNoLeak fails the test when out names any of the given strings.
func checkNoLeak(t *testing.T, out string, leaks ...string) {
	t.Helper()
	for _, leak := range leaks {
		if strings.Contains(out, leak) {
			t.Errorf("output names %q:\n%s", leak, out)
		}
	}
}

// TestDisclosureScanFindsTermsSplitByNul checks a term that a NUL byte splits in two.
// The changed-file path and the tree path both drop NUL bytes before they match, so the
// term is found on each path, and the output names neither file.
func TestDisclosureScanFindsTermsSplitByNul(t *testing.T) {
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, "split.dat"), "x quor\x00vex y\n")
	writeFile(t, filepath.Join(dir, "docs", "evidence", "split-tree.dat"), "y quor\x00vex\n")
	commitAll(t, dir, "split")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	out, code := scan(t, dir, "HEAD~1...HEAD", pats)
	if code != 1 || strings.Count(out, "added line 1") != 2 {
		t.Errorf("split term on the changed-file path: exit status %d, want two added-line findings:\n%s", code, out)
	}
	checkNoLeak(t, out, syntheticTerm, "split", "docs/")

	out, code = scan(t, dir, "HEAD...HEAD", pats, "--tree", "docs/evidence")
	if code != 1 || !strings.Contains(out, "tree file #1 has a match in its text") {
		t.Errorf("split term on the tree path: exit status %d, want a tree text finding:\n%s", code, out)
	}
	checkNoLeak(t, out, syntheticTerm, "split", "docs/")
}

// TestDisclosureScanReadsMagicPrefixedNames adds two files whose names begin with git's
// pathspec magic. Their names are read literally, so the added line of each is found.
// Read as pathspecs, the names would match other paths or none, and the lines would be
// missed.
func TestDisclosureScanReadsMagicPrefixedNames(t *testing.T) {
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, ":(top)"+syntheticTerm+"-top.txt"), "a "+syntheticTerm+" line\n")
	writeFile(t, filepath.Join(dir, ":(glob)"+syntheticTerm+"-glob.txt"), "a "+syntheticTerm+" line\n")
	commitAll(t, dir, "magic names")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	out, code := scan(t, dir, "HEAD~1...HEAD", pats)
	if code != 1 || strings.Count(out, "added line 1") != 2 {
		t.Errorf("magic-prefixed names: exit status %d, want two added-line findings:\n%s", code, out)
	}
	checkNoLeak(t, out, syntheticTerm, "top", "glob")
}
