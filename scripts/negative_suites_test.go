package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// probe-tests/negative holds the variants of a suite whose every test must FAIL. `probe test <dir>` picks up every file that ends in .probe, so none of them may:
// they carry the extension .probe.expect-fail and are copied to a .probe file to be run (the header of each says how). Each test of a variant says what it expects.
func TestNegativeSuitesAreNeverPickedUpByADirectoryRun(t *testing.T) {
	files, err := filepath.Glob("../native-test-apps/*/probe-tests/negative/*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no negative suites yet")
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".probe") {
			t.Errorf("%s ends in .probe: a directory run would pick it up and its tests, which must fail, would fail the run. Name it .probe.expect-fail", f)
		}
		if !strings.HasSuffix(f, ".probe.expect-fail") {
			t.Errorf("%s is not a .probe.expect-fail file", f)
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		tests := 0
		for n, line := range lines {
			if !strings.HasPrefix(line, "test \"") {
				continue
			}
			tests++
			if n == 0 || !strings.HasPrefix(lines[n-1], "# Expected: ") {
				t.Errorf("%s:%d: %s: the line before a test must be its \"# Expected: ...\" comment: every test must say how it is expected to fail", f, n+1, strings.TrimSpace(line))
			}
		}
		if tests == 0 {
			t.Errorf("%s has no test", f)
		}
	}
}
