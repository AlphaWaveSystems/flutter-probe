package scripts

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// utf8Locale returns a UTF-8 locale this machine has, or "" when it has none.
func utf8Locale() string {
	out, err := exec.Command("locale", "-a").Output()
	if err != nil {
		return ""
	}
	have := make(map[string]bool)
	for _, name := range strings.Fields(string(out)) {
		have[name] = true
	}
	for _, name := range []string{"C.UTF-8", "C.utf8", "en_US.UTF-8", "en_US.utf8"} {
		if have[name] {
			return name
		}
	}
	return ""
}

// TestDisclosureScanReadsInvalidUTF8UnderUTF8Locale runs the scanner with a UTF-8 locale in its environment. A file
// that is not valid UTF-8 must still be scanned and reported, and must not stop the run with status 2.
func TestDisclosureScanReadsInvalidUTF8UnderUTF8Locale(t *testing.T) {
	locale := utf8Locale()
	if locale == "" {
		t.Skip("no UTF-8 locale on this machine")
	}
	t.Setenv("LC_ALL", locale)
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, "invalid.txt"), "\xff\xfe a "+syntheticTerm+" line\n")
	commitAll(t, dir, "invalid utf-8")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	out, code := scan(t, dir, "HEAD~1...HEAD", pats)
	if code != 1 || !strings.Contains(out, "changed file #1, added line 1") {
		t.Errorf("invalid UTF-8 under %s: exit status %d, want an added-line finding:\n%s", locale, code, out)
	}
	checkNoLeak(t, out, syntheticTerm, "invalid")
}

// TestDisclosureScanFindsWholeTermAfterNul checks a NUL byte placed right before a whole term. The NUL is removed
// before matching, so the term is found.
func TestDisclosureScanFindsWholeTermAfterNul(t *testing.T) {
	dir := newEmptyRepo(t)
	writeFile(t, filepath.Join(dir, "nul-before.dat"), "x\x00"+syntheticTerm+" line\n")
	commitAll(t, dir, "nul before")
	pats := writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")

	out, code := scan(t, dir, "HEAD~1...HEAD", pats)
	if code != 1 || !strings.Contains(out, "changed file #1, added line 1") {
		t.Errorf("NUL before a term: exit status %d, want an added-line finding:\n%s", code, out)
	}
	checkNoLeak(t, out, syntheticTerm, "nul-before")
}
