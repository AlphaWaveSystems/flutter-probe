package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
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
	{"sleep", false},
	{"file", true}, {"shasum", true}, {"sha256sum", true},
}

// The stand-in for go, make, staticcheck and the tools of the optional checks.
// `make build` and `make build-convert` write the files the Build check tests for,
// and a command with `-o FILE` writes that file.
const fakeTool = `#!%s
name=$(basename "$0")
line="$name $*"
printf '%%s\n' "$line" >> "$FAKE_LOG"
case "$line" in
  "make build")
    if [ -z "${FAKE_NO_PROBE:-}" ]; then mkdir -p bin; : > bin/probe; chmod +x bin/probe; fi
    if [ -n "${FAKE_WORKSUM:-}" ]; then echo "written by the build" > go.work.sum; fi
    if [ -n "${FAKE_TOUCH_TRACKED:-}" ]; then echo "changed by the build" > tracked.txt; fi
    if [ -n "${FAKE_NEW_FILE:-}" ]; then echo "made by the build" > made_by_the_build.txt; fi
    if [ -n "${FAKE_TOUCH_SUB:-}" ]; then echo "changed by the build" > sub/f.txt; fi ;;
  "make build-convert") mkdir -p bin; : > bin/probe-convert; chmod +x bin/probe-convert ;;
  "go env GOFLAGS") printf '%%s\n' "${FAKE_GO_ENV_GOFLAGS:-}" ;;
  "flutter --version") echo "Flutter 3.29.0" ;;
  "node --version") echo "v26.1.0" ;;
esac
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ] && [ "$a" != "/dev/null" ]; then
    case "$a" in *"${FAKE_NO_OUTPUT:-no such file}"*) ;; *) : > "$a" ;; esac
  fi
  prev=$a
done
if [ -n "${FAKE_SLEEP:-}" ] && [ "$line" = "$FAKE_SLEEP" ]; then
  : > "$FAKE_LOG.started"
  sleep 3
fi
if [ -n "${FAKE_FAIL:-}" ] && [ "$line" = "$FAKE_FAIL" ]; then
  echo "stand-in: failing $line"
  exit 1
fi
exit 0
`

type ciLocalEnv struct {
	repo, bin, deny, head, bash string
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
	e.bash = bash
	for _, tool := range []string{"go", "make", "staticcheck"} {
		e.addTool(t, tool)
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

func (e *ciLocalEnv) addTool(t *testing.T, name string) {
	t.Helper()
	text := strings.Replace(fakeTool, "%s", e.bash, 1)
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

// command prepares a run of the script from dir (the repository when dir is empty)
// with the given extra environment and arguments. The second value is the file the
// stand-ins write their command lines to.
func (e *ciLocalEnv) command(t *testing.T, dir string, env []string, args ...string) (*exec.Cmd, *bytes.Buffer, string) {
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
	out := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd, out, log
}

// result reads what a finished run left: its output, exit status and the calls.
func result(t *testing.T, err error, out *bytes.Buffer, log string) ciLocalRun {
	t.Helper()
	r := ciLocalRun{}
	if err != nil {
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

// run runs the script to its end.
func (e *ciLocalEnv) run(t *testing.T, dir string, env []string, args ...string) ciLocalRun {
	t.Helper()
	cmd, out, log := e.command(t, dir, env, args...)
	return result(t, cmd.Run(), out, log)
}

func (e *ciLocalEnv) withDenylist() []string {
	return []string{"PROBE_DISCLOSURE_DENYLIST=" + e.deny}
}

// expect checks the exit status and the whole last line, and the rule that PASS and
// exit status 0 only ever come together, with all five checks.
func (r ciLocalRun) expect(t *testing.T, e *ciLocalEnv, code int, verdict string) {
	t.Helper()
	r.expectBase(t, e, code, verdict, "")
}

// expectBase is expect for a run with --base: the last line names that base.
func (r ciLocalRun) expectBase(t *testing.T, e *ciLocalEnv, code int, verdict, base string) {
	t.Helper()
	want := "ci-local: " + verdict + " at " + e.head
	if base != "" {
		want += " (base " + base + ")"
	}
	if r.last != want || r.code != code {
		t.Errorf("last line %q, exit status %d\nwant      %q, exit status %d\n%s", r.last, r.code, want, code, r.out)
	}
	fullPass := "ci-local: PASS (5 of 5 required checks) at " + e.head
	if base != "" {
		fullPass += " (base " + base + ")"
	}
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

	e.run(t, "", e.withDenylist(), "--base", "no-such-ref").
		expectBase(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Disclosure guard)", "no-such-ref")

	// A base the match is not in the range of: the commit before HEAD is clean, and
	// the last line names the base.
	e.git(t, "branch", "before", "HEAD~1")
	writeFile(t, filepath.Join(e.repo, "more.txt"), "a clean line\n")
	e.git(t, "add", "more.txt")
	e.git(t, "commit", "-q", "-m", "more")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))
	e.run(t, "", e.withDenylist(), "--base", "HEAD~1").expectBase(t, e, 0, "PASS (5 of 5 required checks)", "HEAD~1")
	e.run(t, "", e.withDenylist(), "--base", "before").
		expectBase(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Disclosure guard)", "before")
}

// A base that is HEAD, or that already contains HEAD, leaves nothing to scan. The
// Disclosure guard is then not run, and the run is not a PASS. This is also the
// result on the main branch right after a merge.
func TestCILocalEmptyRangeIsNotAPass(t *testing.T) {
	e := newCILocalEnv(t)
	verdict := "FAIL (4 of 5 required checks passed; not passed: Disclosure guard (not run))"
	notRun := regexp.MustCompile(`(?m)^NOT RUN \(empty range: \S+ contains HEAD\)  Disclosure guard  \d+s$`)

	r := e.run(t, "", e.withDenylist(), "--base", "HEAD")
	r.expectBase(t, e, 1, verdict, "HEAD")
	if !notRun.MatchString(r.out) {
		t.Errorf("no NOT RUN line for the empty range:\n%s", r.out)
	}

	// origin/main is HEAD: the default base, so the last line names no base.
	e.git(t, "update-ref", "refs/remotes/origin/main", "HEAD")
	r = e.run(t, "", e.withDenylist())
	r.expect(t, e, 1, verdict)
	if !notRun.MatchString(r.out) {
		t.Errorf("no NOT RUN line for the empty range:\n%s", r.out)
	}

	// The base is ahead of HEAD.
	e.git(t, "branch", "ahead")
	e.git(t, "reset", "-q", "--hard", "HEAD~1")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))
	e.run(t, "", e.withDenylist(), "--base", "ahead").expectBase(t, e, 1, verdict, "ahead")
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
	dirty := "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)"
	cases := []struct {
		name  string
		setup func(t *testing.T, e *ciLocalEnv)
		tree  string
	}{
		{"an unstaged change", func(t *testing.T, e *ciLocalEnv) {
			writeFile(t, filepath.Join(e.repo, "tracked.txt"), "changed\n")
		}, "1 changed, 0 untracked, 0 hidden"},
		{"a staged change only", func(t *testing.T, e *ciLocalEnv) {
			writeFile(t, filepath.Join(e.repo, "tracked.txt"), "changed\n")
			e.git(t, "add", "tracked.txt")
		}, "1 changed, 0 untracked, 0 hidden"},
		{"an untracked source file", func(t *testing.T, e *ciLocalEnv) {
			writeFile(t, filepath.Join(e.repo, "extra_test.go"), "package extra\n")
		}, "0 changed, 1 untracked, 0 hidden"},
		{"a change hidden with --assume-unchanged", func(t *testing.T, e *ciLocalEnv) {
			e.git(t, "update-index", "--assume-unchanged", "tracked.txt")
			writeFile(t, filepath.Join(e.repo, "tracked.txt"), "changed\n")
		}, "0 changed, 0 untracked, 1 hidden"},
		{"a change hidden with --skip-worktree", func(t *testing.T, e *ciLocalEnv) {
			e.git(t, "update-index", "--skip-worktree", "tracked.txt")
			writeFile(t, filepath.Join(e.repo, "tracked.txt"), "changed\n")
		}, "0 changed, 0 untracked, 1 hidden"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newCILocalEnv(t)
			c.setup(t, e)
			r := e.run(t, "", e.withDenylist())
			r.expect(t, e, 4, dirty)
			if !strings.Contains(r.out, "- Tree: DIRTY TREE ("+c.tree) {
				t.Errorf("the result does not say %q:\n%s", c.tree, r.out)
			}
		})
	}

	// A failing check outranks the dirty tree.
	e := newCILocalEnv(t)
	writeFile(t, filepath.Join(e.repo, "tracked.txt"), "changed\n")
	e.run(t, "", append(e.withDenylist(), "FAKE_FAIL=go vet ./...")).
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")

	// A file git ignores does not make the tree dirty.
	e = newCILocalEnv(t)
	writeFile(t, filepath.Join(e.repo, "coverage.out"), "x\n")
	writeFile(t, filepath.Join(e.repo, "bin", "old"), "x\n")
	r := e.run(t, "", e.withDenylist())
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	if !strings.Contains(r.out, "- Tree: clean") {
		t.Errorf("the tree is not reported as clean:\n%s", r.out)
	}
}

// The Go tools write go.work.sum during a build of the workspace. The file the run
// itself wrote is removed and noted; it does not make this run or the next one dirty.
func TestCILocalRemovesTheGoWorkSumItWrote(t *testing.T) {
	e := newCILocalEnv(t)
	sum := filepath.Join(e.repo, "go.work.sum")
	env := append(e.withDenylist(), "FAKE_WORKSUM=1")

	for i := 0; i < 2; i++ {
		r := e.run(t, "", env)
		r.expect(t, e, 0, "PASS (5 of 5 required checks)")
		if !strings.Contains(r.out, "the Go tools wrote go.work.sum during the run") {
			t.Errorf("run %d: no note about go.work.sum:\n%s", i+1, r.out)
		}
		if _, err := os.Stat(sum); err == nil {
			t.Fatalf("run %d left go.work.sum behind", i+1)
		}
	}

	// A go.work.sum that was there before the run is the developer's file: the tree is
	// dirty, and the file is not removed.
	writeFile(t, sum, "mine\n")
	e.run(t, "", env).expect(t, e, 4, "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)")
	if _, err := os.Stat(sum); err != nil {
		t.Errorf("the run removed a go.work.sum it did not write: %v", err)
	}

	// A tracked go.work.sum is never removed. The build changes it, so the run is a
	// DIRTY TREE, with a note.
	dirty := "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)"
	writeFile(t, sum, "mine\n") // the run before rewrote it
	e.git(t, "add", "go.work.sum")
	e.git(t, "commit", "-q", "-m", "sum")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))
	r := e.run(t, "", env)
	r.expect(t, e, 4, dirty)
	if _, err := os.Stat(sum); err != nil {
		t.Errorf("the run removed a tracked go.work.sum: %v", err)
	}
	if !strings.Contains(r.out, "the run itself changed or created 1 file(s)") {
		t.Errorf("no note about the file the run changed:\n%s", r.out)
	}

	// A tracked go.work.sum that is missing from the tree (a deletion, so a dirty tree)
	// and that the build writes again is still not removed.
	e.git(t, "checkout", "-q", "--", "go.work.sum")
	if err := os.Remove(sum); err != nil {
		t.Fatal(err)
	}
	e.run(t, "", env).expect(t, e, 4, dirty)
	if _, err := os.Stat(sum); err != nil {
		t.Errorf("the run removed a tracked go.work.sum that it had written: %v", err)
	}
}

// The go.work.sum a run wrote is also removed when the run fails or is interrupted:
// the removal is in the exit handler, not only on the way to the final block.
func TestCILocalRemovesTheGoWorkSumOfAFailedOrInterruptedRun(t *testing.T) {
	e := newCILocalEnv(t)
	sum := filepath.Join(e.repo, "go.work.sum")

	e.run(t, "", append(e.withDenylist(), "FAKE_WORKSUM=1", "FAKE_FAIL=go vet ./...")).
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")
	if _, err := os.Stat(sum); err == nil {
		t.Error("a failed run left go.work.sum behind")
	}

	cmd, out, log := e.command(t, "", append(e.withDenylist(), "FAKE_WORKSUM=1", "FAKE_SLEEP=go vet ./..."))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		if _, err := os.Stat(log + ".started"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(sum); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("the build did not write go.work.sum before the signal: %v\n%s", err, out.String())
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	r := result(t, cmd.Wait(), out, log)
	r.expect(t, e, 130, "FAIL (interrupted)")
	if _, err := os.Stat(sum); err == nil {
		t.Error("an interrupted run left go.work.sum behind")
	}
}

// `git status` follows the user's configuration; the script must not.
func TestCILocalGitConfigCannotHideAnUntrackedFile(t *testing.T) {
	e := newCILocalEnv(t)
	e.git(t, "config", "status.showUntrackedFiles", "no")
	writeFile(t, filepath.Join(e.repo, "extra_test.go"), "package extra\n")
	e.run(t, "", e.withDenylist()).
		expect(t, e, 4, "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)")
}

// A check that changes a tracked file leaves a tree that is not the commit.
func TestCILocalCheckThatChangesATrackedFileIsDirty(t *testing.T) {
	e := newCILocalEnv(t)
	r := e.run(t, "", append(e.withDenylist(), "FAKE_TOUCH_TRACKED=1"))
	r.expect(t, e, 4, "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)")
	if !strings.Contains(r.out, "- Tree: DIRTY TREE (a check changed 1 tracked file(s))") {
		t.Errorf("the result does not say that a check changed a tracked file:\n%s", r.out)
	}
}

// Optional checks that were asked for and skipped do not make a PASS.
func TestCILocalSkippedOptionalChecksAreNotAPass(t *testing.T) {
	e := newCILocalEnv(t) // no flutter, dart, node or npm
	all := "PARTIAL (5 of 5 required checks passed; requested optional checks skipped: Dart agent, Studio frontend, Studio Go tests, VS Code extension tests, Website build)"
	r := e.run(t, "", e.withDenylist(), "--all")
	r.expect(t, e, 5, all)
	if !strings.Contains(r.out, "Counts: 5 passed, 0 failed, 5 skipped, 0 not run (10 selected, 5 required)") {
		t.Errorf("unexpected counts:\n%s", r.out)
	}
	e.run(t, "", e.withDenylist(), "--all", "--allow-skip").expect(t, e, 5, all)
	e.run(t, "", nil, "--job", "frontend").
		expect(t, e, 5, "PARTIAL (0 of 5 required checks passed; 5 not selected; requested optional checks skipped: Studio frontend)")
	// A failure outranks it.
	e.run(t, "", append(e.withDenylist(), "FAKE_FAIL=go vet ./..."), "--all").
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")
}

// A --base with a control character could write its own last line.
func TestCILocalRefusesABaseWithAControlCharacter(t *testing.T) {
	e := newCILocalEnv(t)
	for _, base := range []string{
		"x\nci-local: PASS (5 of 5 required checks) at " + e.head, "x\ry", "x\x1b[2Ky",
		"x\u2028y", "x\u2029y", "x\u0085y", "x\u202ey", "x\u200fy", "x y", "",
	} {
		r := e.run(t, "", e.withDenylist(), "--base", base)
		if r.code != 2 || len(r.calls) != 0 || strings.Contains(r.out, "PASS (5 of 5") {
			t.Errorf("--base %q: exit status %d, calls %q; want 2 and nothing run\n%s", base, r.code, r.calls, r.out)
		}
	}
}

// A --report path that the repository tracks, or go.work.sum, or a link, is refused
// before anything is truncated or run.
func TestCILocalRefusesAReportThatWouldDestroyAFile(t *testing.T) {
	e := newCILocalEnv(t)
	link := filepath.Join(t.TempDir(), "link.md")
	if err := os.Symlink(filepath.Join(e.repo, "tracked.txt"), link); err != nil {
		t.Fatal(err)
	}
	hard := filepath.Join(t.TempDir(), "hard.md") // another name for the tracked file
	if err := os.Link(filepath.Join(e.repo, "tracked.txt"), hard); err != nil {
		t.Skipf("hard links are not available: %v", err)
	}
	gitFiles := map[string][]byte{}
	for _, f := range []string{"config", "HEAD"} {
		data, err := os.ReadFile(filepath.Join(e.repo, ".git", f))
		if err != nil {
			t.Fatal(err)
		}
		gitFiles[f] = data
	}
	for _, path := range []string{
		filepath.Join(e.repo, "tracked.txt"),
		filepath.Join(e.repo, "scripts", "..", "tracked.txt"),
		filepath.Join(e.repo, "go.work.sum"),
		filepath.Join(e.repo, "GO.WORK.SUM"),
		filepath.Join(e.repo, ".git", "config"),
		filepath.Join(e.repo, ".git", "HEAD"),
		filepath.Join(e.repo, ".git", "info", "exclude"),
		link,
		hard,
	} {
		r := e.run(t, "", e.withDenylist(), "--report", path)
		if r.code != 2 || len(r.calls) != 0 || strings.Contains(r.out, "PASS") {
			t.Errorf("--report %s: exit status %d, calls %q; want 2 and nothing run\n%s", path, r.code, r.calls, r.out)
		}
	}
	if data, err := os.ReadFile(filepath.Join(e.repo, "tracked.txt")); err != nil || string(data) != "one\n" {
		t.Errorf("tracked.txt was changed: %q (%v)", data, err)
	}
	for f, want := range gitFiles {
		if got, err := os.ReadFile(filepath.Join(e.repo, ".git", f)); err != nil || !bytes.Equal(got, want) {
			t.Errorf(".git/%s was changed: %q (%v)", f, got, err)
		}
	}
	// A report path inside the repository that git ignores is fine.
	e.run(t, "", e.withDenylist(), "--report", filepath.Join(e.repo, "coverage.out")).
		expect(t, e, 0, "PASS (5 of 5 required checks)")
}

// An exported shell function cannot stand in for a tool.
func TestCILocalRemovesExportedShellFunctions(t *testing.T) {
	e := newCILocalEnv(t)
	env := append(e.withDenylist(), "BASH_FUNC_go%%=() { return 0; }", "FAKE_FAIL=go vet ./...")
	r := e.run(t, "", env)
	r.expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")
	if !strings.Contains(r.out, "1 exported shell function(s) were in the environment and were removed") {
		t.Errorf("no note about the exported function:\n%s", r.out)
	}
}

// A failing `git status` does not mean a clean tree.
func TestCILocalStopsWhenGitStatusFails(t *testing.T) {
	e := newCILocalEnv(t)
	writeFile(t, filepath.Join(e.repo, ".git", "index"), "not an index\n")
	r := e.run(t, "", e.withDenylist())
	if r.code != 2 || len(r.calls) != 0 || strings.Contains(r.out, "PASS") {
		t.Errorf("exit status %d, calls %q; want 2, no calls and no PASS\n%s", r.code, r.calls, r.out)
	}
}

func TestCILocalFailedOptionalCheckFailsTheRun(t *testing.T) {
	e := newCILocalEnv(t)
	for _, tool := range []string{"flutter", "dart", "node", "npm"} {
		e.addTool(t, tool)
	}
	for _, dir := range []string{"probe_agent", "studio/frontend", "vscode", "website"} {
		writeFile(t, filepath.Join(e.repo, dir, "keep.txt"), "x\n")
	}
	e.git(t, "add", "-A")
	e.git(t, "commit", "-q", "-m", "directories of the optional checks")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))

	r := e.run(t, "", e.withDenylist(), "--all")
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	if !strings.Contains(r.out, "Counts: 10 passed, 0 failed, 0 skipped, 0 not run (10 selected, 5 required)") {
		t.Errorf("unexpected counts:\n%s", r.out)
	}

	r = e.run(t, "", append(e.withDenylist(), "FAKE_FAIL=npm test"), "--all")
	r.expect(t, e, 1, "FAIL (5 of 5 required checks passed; not passed: VS Code extension tests)")
	if !strings.Contains(r.out, "Counts: 9 passed, 1 failed, 0 skipped, 0 not run (10 selected, 5 required)") {
		t.Errorf("unexpected counts:\n%s", r.out)
	}
}

// The report holds the same result as the terminal, also for a run that fails, and a
// report that cannot be written stops the run before the first check.
func TestCILocalReport(t *testing.T) {
	e := newCILocalEnv(t)
	report := filepath.Join(t.TempDir(), "report.md")
	writeFile(t, report, "ci-local: PASS (5 of 5 required checks) at an older commit\n")

	r := e.run(t, "", append(e.withDenylist(), "FAKE_FAIL=go vet ./..."), "--report", report)
	r.expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	block := r.out[strings.Index(r.out, "### ci-local result"):]
	if string(data) != block {
		t.Errorf("the report is not the result the terminal shows.\nreport:\n%s\nterminal:\n%s", data, block)
	}

	for _, path := range []string{filepath.Join(t.TempDir(), "no-such-directory", "report.md"), t.TempDir()} {
		r = e.run(t, "", e.withDenylist(), "--report", path)
		if r.code != 2 || len(r.calls) != 0 || strings.Contains(r.out, "PASS") {
			t.Errorf("--report %s: exit status %d, calls %q; want 2, no calls and no PASS\n%s", path, r.code, r.calls, r.out)
		}
	}
}

// A run that is stopped with a signal ends with FAIL and exit status 130, and runs no
// further step. bash handles the signal when the command it waits for ends.
func TestCILocalInterruptedRunFails(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT} {
		t.Run(sig.String(), func(t *testing.T) {
			e := newCILocalEnv(t)
			report := filepath.Join(t.TempDir(), "report.md")
			cmd, out, log := e.command(t, "", append(e.withDenylist(), "FAKE_SLEEP=go vet ./..."), "--report", report)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			started := false
			for i := 0; i < 600 && !started; i++ {
				if _, err := os.Stat(log + ".started"); err == nil {
					started = true
				} else {
					time.Sleep(50 * time.Millisecond)
				}
			}
			if !started {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("the run never reached the Lint check:\n%s", out.String())
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			r := result(t, cmd.Wait(), out, log)
			r.expect(t, e, 130, "FAIL (interrupted)")
			for _, later := range []string{"go install honnef.co/go/tools/cmd/staticcheck@latest", "staticcheck ./..."} {
				if count(r.calls, later) != 0 {
					t.Errorf("%q ran after the signal; calls: %q", later, r.calls)
				}
			}
			data, err := os.ReadFile(report)
			if err != nil || strings.TrimSpace(string(data)) != r.last {
				t.Errorf("the report of an interrupted run is %q (%v), want the last line %q", data, err, r.last)
			}
		})
	}
}

// Settings that change what the commands do.
func TestCILocalGoSettings(t *testing.T) {
	e := newCILocalEnv(t)
	partial := "PARTIAL (5 of 5 required checks passed; GOFLAGS is set)"

	r := e.run(t, "", append(e.withDenylist(), "GOFLAGS=-run=NoSuchTest", "GOTOOLCHAIN=go1.99.0"))
	r.expect(t, e, 5, partial)
	for _, line := range []string{"- GOFLAGS: `-run=NoSuchTest` (workflows: not set)", "- GOTOOLCHAIN: `go1.99.0`"} {
		if !strings.Contains(r.out, line) {
			t.Errorf("the result does not hold %q:\n%s", line, r.out)
		}
	}

	// GOFLAGS from the go env file, not from the environment.
	r = e.run(t, "", append(e.withDenylist(), "FAKE_GO_ENV_GOFLAGS=-mod=mod"))
	r.expect(t, e, 5, partial)
	if !strings.Contains(r.out, "- GOFLAGS: `-mod=mod` (workflows: not set)") {
		t.Errorf("the result does not hold the GOFLAGS of `go env`:\n%s", r.out)
	}

	// A failure outranks it.
	e.run(t, "", append(e.withDenylist(), "GOFLAGS=-mod=mod", "FAKE_FAIL=go vet ./...")).
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Lint)")

	// Without the settings the result names neither.
	r = e.run(t, "", e.withDenylist())
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	if strings.Contains(r.out, "GOFLAGS") || strings.Contains(r.out, "GOTOOLCHAIN") {
		t.Errorf("the result names a setting that is not set:\n%s", r.out)
	}
}

// A start-up file named by BASH_ENV must not reach the steps: this one replaces
// `test`, which would hide the missing bin/probe.
func TestCILocalStepsDoNotReadBashEnv(t *testing.T) {
	e := newCILocalEnv(t)
	rc := writeFile(t, filepath.Join(t.TempDir(), "rc.sh"), "test() { return 0; }\n")
	e.run(t, "", append(e.withDenylist(), "FAKE_NO_PROBE=1", "BASH_ENV="+rc, "ENV="+rc)).
		expect(t, e, 1, "FAIL (4 of 5 required checks passed; not passed: Build)")
}

// --release-dry-run builds and runs no checks: it never says PASS, and it refuses the
// options of a run of the checks.
func TestCILocalReleaseDryRun(t *testing.T) {
	e := newCILocalEnv(t)
	last := func(text string) string { return "release-dry-run: " + text + " at " + e.head }

	r := e.run(t, "", nil, "--release-dry-run")
	if r.code != 0 || r.last != last("BUILT 8 binaries")+" (no checks run)" {
		t.Errorf("last line %q, exit status %d\n%s", r.last, r.code, r.out)
	}
	if strings.Contains(r.out, "PASS") {
		t.Errorf("a dry run of the release build says PASS:\n%s", r.out)
	}
	for _, call := range r.calls {
		if !strings.HasPrefix(call, "go build ") && !strings.HasPrefix(call, "go env ") {
			t.Errorf("the dry run called %q", call)
		}
	}

	for _, args := range [][]string{
		{"--all"}, {"--job", "lint"}, {"--allow-skip"}, {"--report", filepath.Join(t.TempDir(), "r.md")},
		{"--pr-text", filepath.Join(e.repo, "tracked.txt")}, {"--base", "HEAD"},
	} {
		for _, order := range [][]string{append([]string{"--release-dry-run"}, args...), append(append([]string{}, args...), "--release-dry-run")} {
			r := e.run(t, "", nil, order...)
			if r.code != 2 || len(r.calls) != 0 || strings.Contains(r.out, "PASS") || strings.Contains(r.out, "BUILT") {
				t.Errorf("%q: exit status %d, calls %q; want 2 and nothing run\n%s", order, r.code, r.calls, r.out)
			}
		}
	}

	// One binary is missing.
	r = e.run(t, "", []string{"FAKE_NO_OUTPUT=probe-mcp-linux-amd64"}, "--release-dry-run")
	if r.code != 1 || !strings.HasPrefix(r.last, "release-dry-run: FAIL (7 of 8 binaries built;") {
		t.Errorf("a missing binary: last line %q, exit status %d\n%s", r.last, r.code, r.out)
	}

	// A dirty tree: tracked change, or an untracked file.
	for _, name := range []string{"tracked.txt", "new.go"} {
		e := newCILocalEnv(t)
		writeFile(t, filepath.Join(e.repo, name), "changed\n")
		r := e.run(t, "", nil, "--release-dry-run")
		want := "release-dry-run: FAIL (DIRTY TREE; the files were built from a tree that is not the commit) at " + e.head
		if r.code != 4 || r.last != want {
			t.Errorf("%s changed: last line %q, exit status %d\nwant %q, exit status 4\n%s", name, r.last, r.code, want, r.out)
		}
	}
}

// --help holds the exit statuses.
func TestCILocalHelpHoldsTheExitStatuses(t *testing.T) {
	e := newCILocalEnv(t)
	r := e.run(t, "", nil, "--help")
	if r.code != 0 {
		t.Fatalf("--help: exit status %d\n%s", r.code, r.out)
	}
	for _, line := range []string{
		"    0  PASS: all five required checks ran and passed on a clean tree",
		"    4  FAIL: nothing failed, but the tree was not the commit",
		"    5  PARTIAL:",
		"  130  FAIL (interrupted)",
		"    0  BUILT: the eight binaries were built from a clean tree",
		"overriding a shell builtin (compgen, unset)",
		"inside .git",
	} {
		if !strings.Contains(r.out, line) {
			t.Errorf("--help does not hold %q:\n%s", line, r.out)
		}
	}
	if strings.Contains(r.out, "end of help") || strings.Contains(r.out, "#") {
		t.Errorf("--help prints more than the help text:\n%s", r.out)
	}
}

// GIT_* variables can point git somewhere else: GIT_WORK_TREE at a clean clone made
// every tree look clean.
func TestCILocalIgnoresGitVariables(t *testing.T) {
	e := newCILocalEnv(t)
	clone := filepath.Join(t.TempDir(), "clone")
	cmd := exec.Command("git", "clone", "-q", e.repo, clone)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(e.repo, "extra_test.go"), "package extra\n")
	dirty := "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)"

	for _, env := range [][]string{
		{"GIT_WORK_TREE=" + clone},
		{"GIT_DIR=" + filepath.Join(clone, ".git")},
		{"GIT_DIR=" + filepath.Join(clone, ".git"), "GIT_WORK_TREE=" + clone},
		{"GIT_INDEX_FILE=" + filepath.Join(clone, ".git", "index")},
	} {
		r := e.run(t, "", append(e.withDenylist(), env...))
		r.expect(t, e, 4, dirty)
		if !strings.Contains(r.out, "GIT_* variable(s) were in the environment and were removed") {
			t.Errorf("%q: no note about the removed variables:\n%s", env, r.out)
		}
	}
}

// A submodule that is configured to be ignored, and that is not at its commit, is a
// dirty tree; so is a submodule a check changes.
func TestCILocalSubmoduleConfigCannotHideAChange(t *testing.T) {
	e := newCILocalEnv(t)
	sub := t.TempDir()
	runIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "protocol.file.allow=always"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	runIn(sub, "init", "-q", ".")
	writeFile(t, filepath.Join(sub, "f.txt"), "one\n")
	runIn(sub, "add", "f.txt")
	runIn(sub, "commit", "-q", "-m", "sub")
	runIn(e.repo, "submodule", "add", "-q", sub, "sub")
	runIn(e.repo, "commit", "-q", "-m", "add a submodule")
	e.head = strings.TrimSpace(e.git(t, "rev-parse", "HEAD"))
	e.git(t, "config", "submodule.sub.ignore", "all")
	e.git(t, "config", "diff.ignoreSubmodules", "all")
	dirty := "FAIL (DIRTY TREE; 5 of 5 required checks passed on a tree that is not the commit)"

	// Clean submodule: a pass.
	e.run(t, "", e.withDenylist()).expect(t, e, 0, "PASS (5 of 5 required checks)")

	// A check changes a file of the submodule.
	r := e.run(t, "", append(e.withDenylist(), "FAKE_TOUCH_SUB=1"))
	r.expect(t, e, 4, dirty)
	if !strings.Contains(r.out, "a check changed 1 tracked file(s)") {
		t.Errorf("the result does not say that a check changed the submodule:\n%s", r.out)
	}
	runIn(filepath.Join(e.repo, "sub"), "checkout", "-q", "--", "f.txt")

	// The submodule was changed before the run.
	writeFile(t, filepath.Join(e.repo, "sub", "f.txt"), "two\n")
	r = e.run(t, "", e.withDenylist())
	r.expect(t, e, 4, dirty)
	if !strings.Contains(r.out, "- Tree: DIRTY TREE (1 changed, 0 untracked, 0 hidden") {
		t.Errorf("the change to the submodule was not seen before the checks:\n%s", r.out)
	}
}

// A report path is refused inside any .git directory, and inside the git directory of
// this repository when that is not called .git.
func TestCILocalRefusesAReportInsideAGitDirectory(t *testing.T) {
	e := newCILocalEnv(t)

	nested := filepath.Join(e.repo, "nested", ".git")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	gitdir := filepath.Join(other, "gitdata")
	if err := os.Rename(filepath.Join(e.repo, ".git"), gitdir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(e.repo, ".git"), "gitdir: "+gitdir+"\n")
	config, err := os.ReadFile(filepath.Join(gitdir, "config"))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{filepath.Join(nested, "HEAD"), filepath.Join(gitdir, "config"), filepath.Join(gitdir, "info", "exclude")} {
		r := e.run(t, "", e.withDenylist(), "--report", path)
		if r.code != 2 || len(r.calls) != 0 || strings.Contains(r.out, "PASS") {
			t.Errorf("--report %s: exit status %d, calls %q; want 2 and nothing run\n%s", path, r.code, r.calls, r.out)
		}
	}
	if got, err := os.ReadFile(filepath.Join(gitdir, "config")); err != nil || !bytes.Equal(got, config) {
		t.Errorf("the git configuration was changed: %q (%v)", got, err)
	}
}

// A file the run creates is noted even when the user's configuration hides untracked
// files.
func TestCILocalNotesAnUntrackedFileTheRunCreatedDespiteGitConfig(t *testing.T) {
	e := newCILocalEnv(t)
	e.git(t, "config", "status.showUntrackedFiles", "no")
	r := e.run(t, "", append(e.withDenylist(), "FAKE_NEW_FILE=1"))
	r.expect(t, e, 0, "PASS (5 of 5 required checks)")
	if !strings.Contains(r.out, "the run itself changed or created 1 file(s)") {
		t.Errorf("no note about the file the run created:\n%s", r.out)
	}
}
