package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The tests for the verdict of scripts/ci-local.sh: its last line and its exit status.
//
// Each test runs a copy of the script in a small git repository built in a temporary
// directory. PATH holds only a directory this test builds: links to the shell
// utilities the script needs, and stand-ins for `go`, `make` and `staticcheck` that
// log their arguments and exit 0, or 1 for the one command line a test names. The
// script itself has no test mode, so nothing a test does here can be reached from a
// normal run.
//
// The rule under test: the last line says PASS, and the exit status is 0, only when
// all five required checks ran and passed on a clean tree.

// utilities the script, the scanner and the stand-ins call. The ones marked optional
// are not on every system.
var ciLocalUtilities = []struct {
	name     string
	optional bool
}{
	{"bash", false}, {"git", false}, {"sed", false}, {"tr", false}, {"date", false},
	{"mktemp", false}, {"grep", false}, {"awk", false}, {"cut", false}, {"rm", false},
	{"mkdir", false}, {"cat", false}, {"wc", false}, {"dirname", false}, {"basename", false},
	{"chmod", false}, {"sort", false}, {"ls", false}, {"env", false}, {"diff", false},
	{"file", true}, {"shasum", true}, {"sha256sum", true},
}

// The stand-in for go, make and staticcheck. `make build` and `make build-convert`
// write the files the Build check tests for.
const fakeTool = `#!%s
name=$(basename "$0")
line="$name $*"
printf '%%s\n' "$line" >> "$FAKE_LOG"
case "$line" in
  "make build") if [ -z "${FAKE_NO_PROBE:-}" ]; then mkdir -p bin; : > bin/probe; chmod +x bin/probe; fi ;;
  "make build-convert") mkdir -p bin; : > bin/probe-convert; chmod +x bin/probe-convert ;;
esac
if [ -n "${FAKE_FAIL:-}" ] && [ "$line" = "$FAKE_FAIL" ]; then
  echo "stand-in: failing $line"
  exit 1
fi
exit 0
`

type ciLocalEnv struct {
	repo, bin, deny, head string
}

// newCILocalEnv builds the repository (a base commit that origin/main names, and one
// commit on top), the tool directory and a pattern file with a synthetic term.
func newCILocalEnv(t *testing.T) *ciLocalEnv {
	t.Helper()
	e := &ciLocalEnv{repo: t.TempDir(), bin: t.TempDir()}

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	for _, u := range ciLocalUtilities {
		p, err := exec.LookPath(u.name)
		if err != nil {
			if u.optional {
				continue
			}
			t.Skipf("%s is not installed", u.name)
		}
		if err := os.Symlink(p, filepath.Join(e.bin, u.name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"go", "make", "staticcheck"} {
		e.addTool(t, tool, bash)
	}

	for _, name := range []string{"ci-local.sh", "disclosure-scan.sh"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(e.repo, "scripts", name), string(data))
	}
	writeFile(t, filepath.Join(e.repo, ".gitignore"), "bin/\ncoverage.out\n")
	writeFile(t, filepath.Join(e.repo, "tools", "probe-convert", "keep.txt"), "x\n")
	writeFile(t, filepath.Join(e.repo, "tracked.txt"), "one\n")
	e.git(t, "init", "-q", ".")
	e.git(t, "config", "user.email", "test@example.invalid")
	e.git(t, "config", "user.name", "test")
	e.git(t, "add", "-A")
	e.git(t, "commit", "-q", "-m", "base")
	e.git(t, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFile(t, filepath.Join(e.repo, "change.txt"), "a clean change\n")
	e.git(t, "add", "-A")
	e.git(t, "commit", "-q", "-m", "change")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))

	e.deny = writeFile(t, filepath.Join(t.TempDir(), "patterns.txt"), syntheticTerm+"\n")
	return e
}

func (e *ciLocalEnv) addTool(t *testing.T, name, bash string) {
	t.Helper()
	text := strings.Replace(fakeTool, "%s", bash, 1)
	text = strings.ReplaceAll(text, "%%", "%")
	if err := os.WriteFile(filepath.Join(e.bin, name), []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (e *ciLocalEnv) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = e.repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

type ciLocalRun struct {
	out, last string
	code      int
	calls     []string // the command lines the stand-ins received
}

// run starts the script from dir (the repository when dir is empty) with the given
// extra environment and arguments.
func (e *ciLocalEnv) run(t *testing.T, dir string, env []string, args ...string) ciLocalRun {
	t.Helper()
	if dir == "" {
		dir = e.repo
	}
	log := filepath.Join(t.TempDir(), "calls.log")
	cmd := exec.Command(filepath.Join(e.bin, "bash"), append([]string{filepath.Join(e.repo, "scripts", "ci-local.sh")}, args...)...)
	cmd.Dir = dir
	cmd.Env = append([]string{
		"PATH=" + e.bin,
		"HOME=" + t.TempDir(),
		"TMPDIR=" + t.TempDir(),
		"FAKE_LOG=" + log,
		"GIT_CONFIG_NOSYSTEM=1",
	}, env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	r := ciLocalRun{}
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("ci-local.sh: %v\n%s", err, out.String())
		}
		r.code = ee.ExitCode()
	}
	r.out = out.String()
	lines := strings.Split(strings.TrimRight(r.out, "\n"), "\n")
	r.last = lines[len(lines)-1]
	if data, err := os.ReadFile(log); err == nil {
		r.calls = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}
	return r
}

func (e *ciLocalEnv) withDenylist() []string {
	return []string{"PROBE_DISCLOSURE_DENYLIST=" + e.deny}
}

// expect checks the exit status and the whole last line, and the rule that PASS and
// exit status 0 only ever come together, with all five checks.
func (r ciLocalRun) expect(t *testing.T, e *ciLocalEnv, code int, verdict string) {
	t.Helper()
	want := "ci-local: " + verdict + " at " + e.head
	if r.last != want || r.code != code {
		t.Errorf("last line %q, exit status %d\nwant      %q, exit status %d\n%s", r.last, r.code, want, code, r.out)
	}
	fullPass := "ci-local: PASS (5 of 5 required checks) at " + e.head
	if said := strings.Contains(r.last, "PASS"); said != (r.code == 0) || (said && r.last != fullPass) {
		t.Errorf("PASS and exit status 0 belong together and only to a full run: last line %q, exit status %d", r.last, r.code)
	}
}

func count(calls []string, line string) int {
	n := 0
	for _, c := range calls {
		if c == line {
			n++
		}
	}
	return n
}

func TestCILocalPassesWhenAllFiveChecksPass(t *testing.T) {
	e := newCILocalEnv(t)
	r := e.run(t, "", e.withDenylist())
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	if !strings.Contains(r.out, "Counts: 5 passed, 0 failed, 0 skipped, 0 not run (5 selected, 5 required)") {
		t.Errorf("unexpected counts:\n%s", r.out)
	}
	for _, name := range requiredChecks {
		if !regexp.MustCompile(`(?m)^PASS  ` + regexp.QuoteMeta(name) + `  \d+s$`).MatchString(r.out) {
			t.Errorf("no PASS line for %q:\n%s", name, r.out)
		}
	}
	// The commands of the workflow jobs reached the tools.
	for _, call := range []string{"go test -v -race -coverprofile=coverage.out ./...", "go vet ./...", "staticcheck ./...", "make test-convert-integration"} {
		if count(r.calls, call) == 0 {
			t.Errorf("the run never called %q; calls: %q", call, r.calls)
		}
	}
	if !strings.Contains(r.out, "no --pr-text file was given, so the pull request title and body were not scanned") {
		t.Errorf("a scan without --pr-text must say that the title and body were not scanned:\n%s", r.out)
	}
}

func TestCILocalFailsWhenAStepFails(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		verdict string
		notRun  string // a command after the failing one, which must not run
	}{
		{"the only step of a check", "FAKE_FAIL=go test -v -race -coverprofile=coverage.out ./...",
			"FAIL (4 of 5 required checks passed; not passed: Go unit tests)", ""},
		{"the first of several steps", "FAKE_FAIL=go vet ./...",
			"FAIL (4 of 5 required checks passed; not passed: Lint)", "staticcheck ./..."},
		{"a middle step", "FAKE_FAIL=make test-convert",
			"FAIL (4 of 5 required checks passed; not passed: probe-convert tests)", "make test-convert-integration"},
		{"the last step", "FAKE_FAIL=make test-convert-integration",
			"FAIL (4 of 5 required checks passed; not passed: probe-convert tests)", ""},
		// `test -x bin/probe` is the first of two lines of one step; the second line
		// succeeds. A workflow step stops at the first failing line.
		{"a line that is not the last line of a step", "FAKE_NO_PROBE=1",
			"FAIL (4 of 5 required checks passed; not passed: Build)", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newCILocalEnv(t)
			r := e.run(t, "", append(e.withDenylist(), c.env))
			r.expect(t, e, 1, c.verdict)
			if c.notRun != "" && count(r.calls, c.notRun) != 0 {
				t.Errorf("%q ran after the failing step; calls: %q", c.notRun, r.calls)
			}
		})
	}
}

func TestCILocalFailsWhenTwoChecksFail(t *testing.T) {
	e := newCILocalEnv(t)
	r := e.run(t, "", append(e.withDenylist(), "FAKE_FAIL=make build"))
	r.expect(t, e, 1, "FAIL (3 of 5 required checks passed; not passed: Build, probe-convert tests)")
}

func TestCILocalDoesNotPassWithoutADenylist(t *testing.T) {
	e := newCILocalEnv(t)
	verdict := "FAIL (4 of 5 required checks passed; not passed: Disclosure guard (not run))"

	r := e.run(t, "", nil)
	r.expect(t, e, 1, verdict)
	if !regexp.MustCompile(`(?m)^NOT RUN \(no denylist\)  Disclosure guard  \d+s$`).MatchString(r.out) {
		t.Errorf("no NOT RUN line for the Disclosure guard:\n%s", r.out)
	}

	empty := writeFile(t, filepath.Join(t.TempDir(), "empty.txt"), "")
	e.run(t, "", []string{"PROBE_DISCLOSURE_DENYLIST=" + empty}).expect(t, e, 1, verdict)
	e.run(t, "", []string{"PROBE_DISCLOSURE_DENYLIST=" + filepath.Join(t.TempDir(), "missing.txt")}).expect(t, e, 1, verdict)
}

func TestCILocalDisclosureGuardFindsAMatch(t *testing.T) {
	e := newCILocalEnv(t)
	writeFile(t, filepath.Join(e.repo, "note.txt"), "the "+syntheticTerm+" value\n")
	e.git(t, "add", "note.txt")
	e.git(t, "commit", "-q", "-m", "note")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))

	r := e.run(t, "", e.withDenylist())
	r.expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Disclosure guard)")
	if strings.Contains(r.out, syntheticTerm) {
		t.Errorf("the output holds the matched text:\n%s", r.out)
	}

	// The match is in the range from the default base only.
	e.run(t, "", e.withDenylist(), "--base", "HEAD").expect(t, e, 0, "PASS (5 of 5 required checks)")
	e.run(t, "", e.withDenylist(), "--base", "no-such-ref").expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Disclosure guard)")
}

func TestCILocalScansThePullRequestText(t *testing.T) {
	e := newCILocalEnv(t)
	texts := t.TempDir()
	writeFile(t, filepath.Join(texts, "title.txt"), "a clean title\n")
	writeFile(t, filepath.Join(texts, "body.txt"), "the "+syntheticTerm+" value\n")

	r := e.run(t, "", e.withDenylist(), "--pr-text", filepath.Join(texts, "title.txt"))
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	if strings.Contains(r.out, "were not scanned") {
		t.Errorf("the title was scanned, but the result says it was not:\n%s", r.out)
	}
	e.run(t, "", e.withDenylist(), "--pr-text", filepath.Join(texts, "title.txt"), "--pr-text", filepath.Join(texts, "body.txt")).
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Disclosure guard)")
}

// Paths on the command line and in PROBE_DISCLOSURE_DENYLIST are relative to the
// directory the script is started from, not to the repository root.
func TestCILocalResolvesRelativePathsFromTheCallersDirectory(t *testing.T) {
	e := newCILocalEnv(t)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "body.txt"), "the "+syntheticTerm+" value\n")
	writeFile(t, filepath.Join(work, "title.txt"), "a clean title\n")
	writeFile(t, filepath.Join(work, "patterns.txt"), syntheticTerm+"\n")
	env := []string{"PROBE_DISCLOSURE_DENYLIST=patterns.txt"}

	r := e.run(t, work, env, "--pr-text", "title.txt", "--report", "report.md")
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	report, err := os.ReadFile(filepath.Join(work, "report.md"))
	if err != nil {
		t.Fatalf("the report was not written in the caller's directory: %v", err)
	}
	if !strings.HasSuffix(strings.TrimRight(string(report), "\n"), r.last) {
		t.Errorf("the report does not end with the last line %q:\n%s", r.last, report)
	}
	if _, err := os.Stat(filepath.Join(e.repo, "report.md")); err == nil {
		t.Error("the report was written in the repository root")
	}

	e.run(t, work, env, "--pr-text", "body.txt").
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Disclosure guard)")

	if r := e.run(t, work, env, "--pr-text", "missing.txt"); r.code != 2 {
		t.Errorf("a missing --pr-text file: exit status %d, want 2\n%s", r.code, r.out)
	}
}

func TestCILocalMissingTool(t *testing.T) {
	e := newCILocalEnv(t)
	if err := os.Remove(filepath.Join(e.bin, "go")); err != nil {
		t.Fatal(err)
	}

	r := e.run(t, "", e.withDenylist())
	r.expect(t, e, 3, "FAIL (1 of 5 required checks passed; 4 skipped)")
	if !regexp.MustCompile(`(?m)^SKIPPED \(tool go not installed\)  Lint  \d+s$`).MatchString(r.out) {
		t.Errorf("no SKIPPED line for Lint:\n%s", r.out)
	}
	if len(r.calls) != 0 {
		t.Errorf("a skipped check ran commands: %q", r.calls)
	}

	// An allowed skip is not a pass.
	e.run(t, "", e.withDenylist(), "--allow-skip").
		expect(t, e, 5, "PARTIAL (1 of 5 required checks passed; 4 skipped with --allow-skip)")

	// A failure outranks a skip, allowed or not.
	e.run(t, "", nil, "--allow-skip").
		expect(t, e, 1, "FAIL (0 of 5 required checks passed; not passed: Disclosure guard (not run))")
}

func TestCILocalSubsetIsPartial(t *testing.T) {
	e := newCILocalEnv(t)

	e.run(t, "", nil, "--job", "lint").
		expect(t, e, 5, "PARTIAL (1 of 5 required checks passed; 4 not selected)")
	e.run(t, "", nil, "--job", "Lint", "--job", "unit-tests").
		expect(t, e, 5, "PARTIAL (2 of 5 required checks passed; 3 not selected)")
	e.run(t, "", []string{"FAKE_FAIL=go vet ./..."}, "--job", "lint").
		expect(t, e, 1, "FAIL (0 of 5 required checks passed; not passed: Lint)")

	// All five, named one by one, are a full run.
	e.run(t, "", e.withDenylist(), "--job", "build", "--job", "unit-tests", "--job", "convert-tests", "--job", "lint", "--job", "guard").
		expect(t, e, 0, "PASS (5 of 5 required checks)")

	if r := e.run(t, "", nil, "--job", "no-such-check"); r.code != 2 {
		t.Errorf("an unknown check: exit status %d, want 2\n%s", r.code, r.out)
	}
}

func TestCILocalRepeatedJobCountsOnce(t *testing.T) {
	e := newCILocalEnv(t)
	r := e.run(t, "", nil, "--job", "lint", "--job", "lint", "--job", "Lint", "--job", "lint", "--job", "lint")
	r.expect(t, e, 5, "PARTIAL (1 of 5 required checks passed; 4 not selected)")
	if n := count(r.calls, "go vet ./..."); n != 2 { // once in the root, once in tools/probe-convert
		t.Errorf("Lint ran its commands %d times, want once per directory (2): %q", n, r.calls)
	}
	if !strings.Contains(r.out, "Counts: 1 passed, 0 failed, 0 skipped, 0 not run (1 selected, 5 required)") {
		t.Errorf("unexpected counts:\n%s", r.out)
	}
}

func TestCILocalDirtyTreeFails(t *testing.T) {
	e := newCILocalEnv(t)
	writeFile(t, filepath.Join(e.repo, "tracked.txt"), "changed\n")

	r := e.run(t, "", e.withDenylist())
	r.expect(t, e, 4, "FAIL (DIRTY TREE; 5 of 5 required checks passed on uncommitted changes)")
	if !strings.Contains(r.out, "- Tree: DIRTY TREE") {
		t.Errorf("the result does not mark the tree:\n%s", r.out)
	}

	// A failing check outranks the dirty tree.
	e.run(t, "", append(e.withDenylist(), "FAKE_FAIL=go vet ./...")).
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")

	// An untracked file is counted in a note and does not fail the run.
	e2 := newCILocalEnv(t)
	writeFile(t, filepath.Join(e2.repo, "untracked.txt"), "x\n")
	r = e2.run(t, "", e2.withDenylist())
	r.expect(t, e2, 0, "PASS (5 of 5 required checks)")
	if !strings.Contains(r.out, "1 untracked file(s)") {
		t.Errorf("no note about the untracked file:\n%s", r.out)
	}
}
