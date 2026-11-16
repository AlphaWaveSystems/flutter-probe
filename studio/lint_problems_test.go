package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
	got := oneLine("a\nb\tc‮d​e\x00f g")
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
			if err != nil || (i%2 == 0) != (len(res.Problems) > 0) {
				t.Errorf("call %d: %+v, %v", i, res, err)
			}
		}(i)
	}
	wg.Wait()
}
