package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func lintWorkspace(t *testing.T) (*App, string) {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "probe.yaml"), []byte("project:\n  app: com.example.app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(ws, "tests", "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  log \"saved\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(ws)
	return a, file
}

// The unsaved text is what is checked: the file on disk is clean, the text has a test with no steps and a missing recipe.
func TestLintProblemsChecksTheEditorTextNotTheFile(t *testing.T) {
	a, file := lintWorkspace(t)
	res, err := a.LintProblems(file, "use \"recipes/missing.probe\"\n\ntest \"empty\"\n")
	if err != nil {
		t.Fatal(err)
	}
	var program, reference int
	for _, p := range res.Problems {
		switch p.Source {
		case "program":
			program++
			if p.Line != 3 || !p.Warning || !strings.Contains(p.Message, "has no steps") {
				t.Errorf("program finding = %+v", p)
			}
		case "reference":
			reference++
			if p.Line != 1 || !p.Warning || !strings.Contains(p.Message, "missing.probe") {
				t.Errorf("reference finding = %+v, want a warning on line 1 naming the recipe", p)
			}
		}
	}
	if program != 1 || reference != 1 {
		t.Errorf("findings = %+v, want one program warning and one reference warning", res.Problems)
	}
	clean, err := a.LintProblems(file, "test \"t\"\n  log \"x\"\n")
	if err != nil || len(clean.Problems) != 0 || clean.Unavailable != "" {
		t.Errorf("clean text gave %+v, %v", clean, err)
	}
}

// A path outside the workspace, with .., or that is not a .probe file is refused before anything is read.
func TestLintProblemsRefusesPathsOutsideTheWorkspace(t *testing.T) {
	a, file := lintWorkspace(t)
	outside := filepath.Join(t.TempDir(), "x.probe")
	if err := os.WriteFile(outside, []byte("test \"t\"\n  log \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(file), "link.probe")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	notProbe := filepath.Join(filepath.Dir(file), "notes.txt")
	_ = os.WriteFile(notProbe, []byte("x"), 0o600)
	for name, p := range map[string]string{
		"outside the workspace": outside,
		"a link out of it":      link,
		"a dot-dot segment":     filepath.Dir(file) + "/../tests/a.probe",
		"not a .probe file":     notProbe,
		"a missing file":        filepath.Join(filepath.Dir(file), "gone.probe"),
		"a relative path":       "tests/a.probe",
	} {
		if _, err := a.LintProblems(p, "test \"t\"\n  log \"x\"\n"); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// Text that does not parse yet (the person is typing) is not a failure: one line says it was not checked.
func TestLintProblemsSaysWhenTheTextCannotBeChecked(t *testing.T) {
	a, file := lintWorkspace(t)
	res, err := a.LintProblems(file, "test \"t\"\n  tap \"unterminated\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Unavailable, "Not checked: ") || len(res.Problems) != 0 {
		t.Errorf("result = %+v", res)
	}
}

// The recipes folder is read against the workspace, not the folder Studio was started in: a recipe in <workspace>/tests/recipes resolves.
func TestLintProblemsResolvesRecipesAgainstTheWorkspace(t *testing.T) {
	a, file := lintWorkspace(t)
	rec := filepath.Join(filepath.Dir(file), "recipes")
	if err := os.MkdirAll(rec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rec, "open menu.probe"), []byte("recipe \"open menu\"\n  tap \"Menu\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	here, _ := os.Getwd()
	defer os.Chdir(here)
	_ = os.Chdir(t.TempDir()) // the process folder has no recipes
	res, err := a.LintProblems(file, "test \"t\"\n  open menu\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Problems {
		if p.Source == "reference" && !p.Warning {
			t.Errorf("a recipe of the workspace did not resolve: %+v", p)
		}
	}
}

func TestOneLineMakesFindingTextSafe(t *testing.T) {
	got := oneLine("a\nb\tc\u202ed\u200be\x00f g")
	if got != "a b cdefg" {
		t.Errorf("oneLine = %q", got)
	}
	long := oneLine(strings.Repeat("x", 1000))
	if len([]rune(long)) != maxProblemText+1 || !strings.HasSuffix(long, "…") {
		t.Errorf("a long message is cut at %d characters with an ellipsis, got %d", maxProblemText, len([]rune(long)))
	}
}

// Studio calls it from several goroutines at once (the editor asks again on every pause): run with -race.
func TestLintProblemsUnderConcurrentCalls(t *testing.T) {
	a, file := lintWorkspace(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			text := "test \"t\"\n  log \"x\"\n"
			if i%2 == 0 {
				text = "use \"recipes/missing.probe\"\n\ntest \"e\"\n"
			}
			res, err := a.LintProblems(file, text)
			if res.Stale {
				return // a newer call replaced it before it ran
			}
			if err != nil || (i%2 == 0) != (len(res.Problems) > 0) {
				t.Errorf("call %d: %+v, %v", i, res, err)
			}
		}(i)
	}
	wg.Wait()
}

// A text over 1 MiB is not linted: one line says so, at once.
func TestLintProblemsSkipsAHugeText(t *testing.T) {
	a, file := lintWorkspace(t)
	res, err := a.LintProblems(file, "test \"t\"\n  log \""+strings.Repeat("x", maxLintBytes)+"\"\n")
	if err != nil || !strings.Contains(res.Unavailable, "too large") || len(res.Problems) != 0 {
		t.Errorf("result = %+v, %v", res, err)
	}
	ok, err := a.LintProblems(file, "test \"t\"\n  log \"x\"\n")
	if err != nil || ok.Unavailable != "" {
		t.Errorf("a normal text was refused: %+v, %v", ok, err)
	}
}

// A burst of requests queued behind a running lint: only the newest runs; the others come back stale without checking anything.
func TestLintProblemsCoalescesAQueueOfStaleRequests(t *testing.T) {
	a, file := lintWorkspace(t)
	const n = 6
	start := lintSeq.Load()
	lintMu.Lock() // a lint is "running"
	results := make([]LintProblemsResult, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = a.LintProblems(file, "use \"recipes/missing.probe\"\n\ntest \"e\"\n")
		}(i)
		for lintSeq.Load() < start+uint64(i+1) { // this request has its ticket before the next one starts
			time.Sleep(time.Millisecond)
		}
	}
	lintMu.Unlock()
	wg.Wait()
	stale, ran := 0, 0
	for _, r := range results {
		if r.Stale {
			stale++
			if len(r.Problems) != 0 {
				t.Error("a stale request returned findings")
			}
		} else {
			ran++
			if len(r.Problems) == 0 {
				t.Error("the newest request did not run")
			}
		}
	}
	if ran != 1 || stale != n-1 {
		t.Errorf("%d ran, %d stale; want 1 and %d", ran, stale, n-1)
	}
}

// ---- the lint never discloses a file outside the workspace ----

const secretWord = "SECRETTOKEN"

// leakFixture is a workspace and, beside it, a folder holding files that must never be read or named: a recipe file with a secret in it and
// one that does not parse, and a CSV whose header is the secret.
type leakFixture struct {
	a       *App
	ws      string
	file    string
	outside string
}

func newLeakFixture(t *testing.T) leakFixture {
	t.Helper()
	a, file := lintWorkspace(t)
	ws := filepath.Dir(filepath.Dir(file))
	outside := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(outside, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("secretfile.probe", "recipe \"open "+secretWord+"\"\n  tap \""+secretWord+"\"\n")
	write("brokenfile.probe", "test \""+secretWord+"\"\n  tap \""+secretWord+"\n  ??? "+secretWord+"\n")
	write("secretdata.csv", secretWord+"COL,other\n1,2\n")
	if err := os.MkdirAll(filepath.Join(outside, "recipes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "recipes", "inside.probe"), []byte("recipe \"open "+secretWord+"\"\n  tap \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return leakFixture{a: a, ws: ws, file: file, outside: outside}
}

// assertNoLeak fails if anything the window would show names the secret, a folder outside the workspace, or an outside file.
func (f leakFixture) assertNoLeak(t *testing.T, name string, res LintProblemsResult, err error) {
	t.Helper()
	var shown []string
	if err != nil {
		shown = append(shown, err.Error())
	}
	shown = append(shown, res.Unavailable)
	for _, p := range res.Problems {
		shown = append(shown, p.Message)
	}
	all := strings.Join(shown, "\n")
	for _, bad := range []string{secretWord, f.outside, filepath.Base(f.outside), "secretfile", "brokenfile", "secretdata", f.ws, filepath.Dir(f.ws)} {
		if strings.Contains(all, bad) {
			t.Errorf("%s: what the window would show names %q:\n%s", name, bad, all)
		}
	}
}

func (f leakFixture) setRecipes(t *testing.T, folder string) {
	t.Helper()
	yaml := "project:\n  app: com.example.app\nrecipes_folder: " + folder + "\n"
	if err := os.WriteFile(filepath.Join(f.ws, "probe.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLintProblemsNeverShowsAFileOutsideTheWorkspace(t *testing.T) {
	f := newLeakFixture(t)
	rel, _ := filepath.Rel(filepath.Join(f.ws, "tests"), filepath.Join(f.outside, "secretfile.probe"))
	relBroken, _ := filepath.Rel(filepath.Join(f.ws, "tests"), filepath.Join(f.outside, "brokenfile.probe"))
	danglingLink := filepath.Join(f.ws, "tests", "dangling.probe")
	_ = os.Symlink(filepath.Join(f.outside, "does-not-exist.probe"), danglingLink)
	outLink := filepath.Join(f.ws, "tests", "out.probe")
	_ = os.Symlink(filepath.Join(f.outside, "secretfile.probe"), outLink)
	csvLink := filepath.Join(f.ws, "tests", "data.csv")
	_ = os.Symlink(filepath.Join(f.outside, "secretdata.csv"), csvLink)
	body := "\ntest \"t\"\n  open the app\n  tap \"<guessed>\"\n"
	cases := map[string]string{
		"use with ../ out of the workspace": "use \"" + rel + "\"\n" + body,
		"use of a file that does not parse": "use \"" + relBroken + "\"\n" + body,
		"use through a link to outside":     "use \"out.probe\"\n" + body,
		"use through a dangling link":       "use \"dangling.probe\"\n" + body,
		"absolute CSV source":               body + "\nwith examples from \"" + filepath.Join(f.outside, "secretdata.csv") + "\"\n",
		"CSV through a link to outside":     body + "\nwith examples from \"data.csv\"\n",
		"CSV with ../ out of the workspace": body + "\nwith examples from \"../../" + filepath.Base(f.outside) + "/secretdata.csv\"\n",
	}
	for name, text := range cases {
		res, err := f.a.LintProblems(f.file, text)
		f.assertNoLeak(t, name, res, err)
	}
	// A path the person typed into the text may come back in a finding (the text is theirs); the outside file's contents never do.
	typed := filepath.Join(f.outside, "secretfile.probe")
	typedRes, typedErr := f.a.LintProblems(f.file, "use \""+typed+"\"\n"+body)
	if typedErr != nil {
		t.Fatal(typedErr)
	}
	for _, p := range typedRes.Problems {
		if strings.Contains(p.Message, secretWord) {
			t.Errorf("a typed absolute path made a finding show the file's content: %q", p.Message)
		}
	}
	// the recipes folder comes from probe.yaml, which the opened folder owns
	for name, folder := range map[string]string{
		"recipes_folder absolute":      f.outside,
		"recipes_folder with ../":      "../" + filepath.Base(f.outside),
		"recipes_folder absolute /":    f.outside + "/recipes",
		"recipes_folder dot-dot twice": "../../" + filepath.Base(f.outside) + "/recipes",
	} {
		f.setRecipes(t, folder)
		res, err := f.a.LintProblems(f.file, "test \"t\"\n  open "+secretWord+"\n")
		f.assertNoLeak(t, name, res, err)
		res, err = f.a.LintProblems(f.file, "use \"recipes/inside.probe\"\n"+body)
		f.assertNoLeak(t, name+" (use)", res, err)
	}
}

// An outside recipes folder gives one warning at line 0 that says so, and the lint goes on: the Problems list is not switched off, and the
// recipe calls that follow are not each reported as unknown steps.
func TestLintProblemsReportsAnOutsideRecipesFolderOnceAndGoesOn(t *testing.T) {
	f := newLeakFixture(t)
	f.setRecipes(t, f.outside)
	res, err := f.a.LintProblems(f.file, "test \"empty\"\n\ntest \"t\"\n  open the menu\n  open the cart\n  open settings\n")
	if err != nil || res.Unavailable != "" {
		t.Fatalf("the lint was switched off: %+v, %v", res, err)
	}
	var outside, noSteps, unknown int
	for _, p := range res.Problems {
		switch {
		case strings.Contains(p.Message, "recipes folder is outside the workspace"):
			outside++
			if p.Line != 0 || !p.Warning {
				t.Errorf("the outside-folder finding = %+v, want a warning at line 0", p)
			}
		case strings.Contains(p.Message, "has no steps"):
			noSteps++
		case strings.Contains(strings.ToLower(p.Message), "unknown step"):
			unknown++
		}
	}
	if outside != 1 || noSteps != 1 || unknown != 0 {
		t.Errorf("findings = %+v; want one outside-folder warning, the no-steps warning, and no unknown-step errors", res.Problems)
	}
}

// A refused CSV is not an oracle for its header: a guess that is right and one that is wrong give the same findings.
func TestLintProblemsGivesNoOracleForAnOutsideCSVHeader(t *testing.T) {
	f := newLeakFixture(t)
	csv := filepath.Join(f.outside, "secretdata.csv")
	ask := func(guess string) []string {
		text := "test \"t\"\n  open the app\n  tap \"<" + guess + ">\"\n\nwith examples from \"" + csv + "\"\n"
		res, err := f.a.LintProblems(f.file, text)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range res.Problems {
			out = append(out, strings.ReplaceAll(p.Message, guess, "GUESS"))
		}
		return out
	}
	right, wrong := ask(secretWord+"COL"), ask("nosuchcolumn")
	if strings.Join(right, "|") != strings.Join(wrong, "|") {
		t.Errorf("a right guess and a wrong guess differ:\n  right: %q\n  wrong: %q", right, wrong)
	}
}

func TestWithoutPathsDropsTheWorkspaceAndAnyOtherAbsolutePath(t *testing.T) {
	got := withoutPaths("read /work/space/tests/a.probe and /etc/passwd or C:\\Users\\x\\y.csv: denied", "/work/space")
	if strings.Contains(got, "/work/space") || strings.Contains(got, "/etc/passwd") || strings.Contains(got, "Users") {
		t.Errorf("withoutPaths = %q", got)
	}
	if !strings.Contains(got, "tests/a.probe") {
		t.Errorf("a path inside the workspace should stay as a relative name: %q", got)
	}
}

// With the workspace set only through PROBE_STUDIO_WORKSPACE (a headless Studio), the fence is that folder.
func TestLintProblemsFencesToTheEnvWorkspaceWhenNoneWasOpened(t *testing.T) {
	f := newLeakFixture(t)
	t.Setenv("PROBE_STUDIO_WORKSPACE", f.ws)
	a := &App{} // no SetWorkspace: only the environment names the folder
	res, err := a.LintProblems(f.file, "use \"../../"+filepath.Base(f.outside)+"/secretfile.probe\"\n\ntest \"t\"\n  log \"x\"\n")
	if err != nil {
		t.Fatal(err)
	}
	f.assertNoLeak(t, "env workspace", res, nil)
	var refused bool
	for _, p := range res.Problems {
		if strings.Contains(p.Message, "outside the workspace") {
			refused = true
		}
	}
	if !refused {
		t.Errorf("the outside use target was not refused: %+v", res.Problems)
	}
}

// A workspace of / (or a drive) fences nothing: the lint says so in one line and reads nothing. A home folder is fine.
func TestLintProblemsRefusesAFilesystemRootWorkspace(t *testing.T) {
	for _, root := range []string{"/", "//"} {
		a := &App{}
		a.SetWorkspace(root)
		res, err := a.LintProblems("/anything.probe", "test \"t\"\n  log \"x\"\n")
		if err != nil || !strings.Contains(res.Unavailable, "root of the file system") || len(res.Problems) != 0 {
			t.Errorf("root %q: %+v, %v", root, res, err)
		}
	}
	home := t.TempDir() // a folder under the root, like a home folder, is a fine workspace
	a := &App{}
	a.SetWorkspace(home)
	if err := os.WriteFile(filepath.Join(home, "a.probe"), []byte("test \"t\"\n  log \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := a.LintProblems(filepath.Join(home, "a.probe"), "test \"t\"\n  log \"x\"\n")
	if err != nil || res.Unavailable != "" {
		t.Errorf("a normal workspace was refused: %+v, %v", res, err)
	}
}

// A drive root is a file system root only where drives exist: this case runs on Windows (on other systems C:\\ is a relative name).
func TestIsFilesystemRootDrive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive roots exist on Windows only")
	}
	for p, want := range map[string]bool{`C:\`: true, `C:\Users`: false} {
		if got := isFilesystemRoot(p); got != want {
			t.Errorf("isFilesystemRoot(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestIsFilesystemRoot(t *testing.T) {
	for p, want := range map[string]bool{"/": true, "//": true, "": false, "/Users": false, "/Users/me": false, "/Users/me/": false, "relative": false} {
		if got := isFilesystemRoot(p); got != want {
			t.Errorf("isFilesystemRoot(%q) = %v, want %v", p, got, want)
		}
	}
}

// The size cap is exactly 1 MiB: a text of that many bytes is checked, one byte more is not.
func TestLintProblemsSizeCapBoundary(t *testing.T) {
	a, file := lintWorkspace(t)
	base := "test \"t\"\n  log \"x\"\n# "
	atCap := base + strings.Repeat("x", maxLintBytes-len(base))
	if len(atCap) != maxLintBytes {
		t.Fatalf("fixture is %d bytes", len(atCap))
	}
	res, err := a.LintProblems(file, atCap)
	if err != nil || res.Unavailable != "" {
		t.Errorf("exactly 1 MiB was refused: %+v, %v", res.Unavailable, err)
	}
	res, err = a.LintProblems(file, atCap+"x")
	if err != nil || !strings.Contains(res.Unavailable, "too large") {
		t.Errorf("1 MiB + 1 byte was checked: %+v, %v", res, err)
	}
}

// What the window shows for an error and for the findings of a report has no absolute path, whatever the engine put in the text.
func TestLintTextsShownHaveNoAbsolutePath(t *testing.T) {
	root := "/work/space"
	got := lintUnavailable(errorString("open /work/space/tests/recipes: denied; also /etc/shadow"), root)
	if strings.Contains(got, "/work/space") || strings.Contains(got, "/etc/shadow") || !strings.HasPrefix(got, "Not checked: ") {
		t.Errorf("the Not checked line = %q", got)
	}
	rep := runner.LintReport{
		References: []runner.LintFinding{{Line: 2, Message: "no file at /work/space/tests/x.probe or /secret/dir/y.csv"}},
		Program:    []runner.LintFinding{{Line: 3, Warning: true, Message: "test \"t\" at /secret/dir/z.probe has no steps"}},
	}
	res := lintToProblems(rep, root)
	if len(res.Problems) != 2 || res.Problems[0].Source != "reference" || res.Problems[1].Source != "program" {
		t.Fatalf("problems = %+v", res.Problems)
	}
	for _, p := range res.Problems {
		if strings.Contains(p.Message, "/secret") || strings.Contains(p.Message, "/work/space") {
			t.Errorf("a finding shows a path: %q", p.Message)
		}
	}
	if !strings.Contains(res.Problems[0].Message, "tests/x.probe") {
		t.Errorf("a path inside the workspace should become a relative name: %q", res.Problems[0].Message)
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

// Only real file system paths are replaced: URLs, relative names and plain slashes stay as they are.
func TestWithoutPathsKeepsURLsAndRelativeNames(t *testing.T) {
	keep := []string{
		"see https://example.com/a/b for details",
		"tap \"https://example.com/a/b?x=1\"",
		"open the cart/checkout page",
		"use \"recipes/open menu.probe\"",
		"1/2 and a/b and tests/a.probe",
		"ratio 3:/4",
	}
	for _, s := range keep {
		if got := withoutPaths(s, "/work/space"); got != s {
			t.Errorf("withoutPaths(%q) = %q, want it unchanged", s, got)
		}
	}
	for _, c := range []struct{ in, mustNot string }{
		{"file:///etc/passwd", "/etc/passwd"},
		{"FILE:///Users/me/x.csv", "/Users/me"},
		{"read /etc/passwd now", "/etc/passwd"},
		{"x=/tmp/secret.txt", "/tmp/secret.txt"},
		{"(\"/var/log/x\")", "/var/log"},
		{"C:\\Users\\me\\y.csv failed", "Users"},
		{"path: /home/me/.ssh/id_rsa", "/home/me"},
	} {
		if got := withoutPaths(c.in, "/work/space"); strings.Contains(got, c.mustNot) {
			t.Errorf("withoutPaths(%q) = %q still shows %q", c.in, got, c.mustNot)
		}
	}
}

// A ${NAME} that is not set in Studio's environment is a warning on its step's line: names only, and only existence is asked.
func TestLintProblemsWarnsAboutUnsetEnvironmentVariables(t *testing.T) {
	asked := map[string]int{}
	old := envIsSet
	envIsSet = func(name string) bool { asked[name]++; return name == "SET_ONE" }
	defer func() { envIsSet = old }()
	a, file := lintWorkspace(t)
	text := "test \"login\"\n  open the app\n  type \"${USER_EMAIL}\" into the \"email\" field\n  type \"${SET_ONE}\" into the \"x\" field\n  # tap \"${IN_A_COMMENT}\"\n"
	res, err := a.LintProblems(file, text)
	if err != nil {
		t.Fatal(err)
	}
	var env []LintProblem
	for _, p := range res.Problems {
		if p.Source == "environment" {
			env = append(env, p)
		}
	}
	if len(env) != 1 || env[0].Line != 3 || !env[0].Warning {
		t.Fatalf("environment problems = %+v, want one warning on line 3", env)
	}
	m := env[0].Message
	if !strings.Contains(m, "${USER_EMAIL} is not set in Studio's environment") || !strings.Contains(m, "start Studio from a terminal where USER_EMAIL is set") {
		t.Errorf("message = %q", m)
	}
	if asked["IN_A_COMMENT"] != 0 {
		t.Error("a variable in a comment was looked up")
	}
}

// Only a name is ever looked up and shown: the real environment holds a value, and it is in no finding.
func TestLintProblemsNeverShowsAnEnvironmentValue(t *testing.T) {
	const secret = "VALUE-must-not-appear-7c1e"
	t.Setenv("STUDIO_TEST_SET_VAR", secret)
	a, file := lintWorkspace(t)
	text := "test \"t\"\n  type \"${STUDIO_TEST_SET_VAR}\" into the \"a\" field\n  type \"${STUDIO_TEST_UNSET_VAR_93}\" into the \"b\" field\n"
	res, err := a.LintProblems(file, text)
	if err != nil {
		t.Fatal(err)
	}
	var env int
	for _, p := range res.Problems {
		if strings.Contains(p.Message, secret) {
			t.Errorf("a finding shows an environment value: %q", p.Message)
		}
		if p.Source == "environment" {
			env++
			if !strings.Contains(p.Message, "STUDIO_TEST_UNSET_VAR_93") || strings.Contains(p.Message, "STUDIO_TEST_SET_VAR}") {
				t.Errorf("message = %q", p.Message)
			}
		}
	}
	if env != 1 {
		t.Errorf("%d environment warnings, want one (the set variable has none)", env)
	}
}

func TestUnsetEnvProblemsAreCappedAndSilentOnATextThatDoesNotParse(t *testing.T) {
	old := envIsSet
	envIsSet = func(string) bool { return false }
	defer func() { envIsSet = old }()
	var b strings.Builder
	b.WriteString("test \"t\"\n")
	for i := 0; i < 40; i++ {
		b.WriteString("  tap \"${V" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + "}\"\n")
	}
	if got := unsetEnvProblems(b.String()); len(got) != maxEnvProblems {
		t.Errorf("%d warnings, want the cap of %d", len(got), maxEnvProblems)
	}
	if got := unsetEnvProblems("test \"t\"\n  tap \"unterminated ${X}\n"); got != nil {
		t.Errorf("a text that does not parse gave %+v", got)
	}
}
