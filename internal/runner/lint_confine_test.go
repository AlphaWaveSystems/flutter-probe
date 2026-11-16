package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// confinedWorkspace lays out a workspace and a folder outside it under one temporary folder. The outside folder holds text that
// a lint fenced to the workspace must never show. It returns the workspace and the outside folder.
func confinedWorkspace(t *testing.T) (ws, outside string) {
	t.Helper()
	base := t.TempDir()
	ws = filepath.Join(base, "ws")
	outside = filepath.Join(base, "outside")
	for _, dir := range []string{
		filepath.Join(ws, "tests"),
		filepath.Join(ws, "recipes"),
		filepath.Join(ws, "parts"),
		filepath.Join(outside, "recipes"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(outside, "secret.probe"):           "recipe \"open the vault\"\n  tap \"SECRETTOKEN\"\n",
		filepath.Join(outside, "secret.csv"):             "guessedheader\nSECRETTOKEN\n",
		filepath.Join(outside, "recipes", "vault.probe"): "recipe \"open the vault\"\n  tap \"SECRETTOKEN\"\n",
		filepath.Join(ws, "recipes", "open_menu.probe"):  "recipe \"open menu\"\n  tap \"Menu\"\n",
		filepath.Join(ws, "parts", "broken.probe"):       "recipe \"broken\"\n  swipe right on \"Menu\" from 2 percent SECRETTOKEN\n",
		filepath.Join(ws, "tests", "data.csv"):           "guessedheader\nvalue\n",
	}
	for p, s := range files {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return ws, outside
}

// confinedLeaks are the strings no finding of a fenced lint may contain: the secret text, the outside folder and the workspace
// as absolute paths, and the names of the outside file.
func confinedLeaks(ws, outside string) []string {
	return []string{"SECRETTOKEN", outside, ws, filepath.Dir(ws), "secret.probe"}
}

// assertNoLeak fails when a finding of rep contains any of the leaks.
func assertNoLeak(t *testing.T, rep LintReport, leaks ...string) {
	t.Helper()
	for _, f := range append(append([]LintFinding{}, rep.Program...), rep.References...) {
		for _, leak := range leaks {
			if strings.Contains(f.Message, leak) {
				t.Errorf("finding shows %q: line %d %q", leak, f.Line, f.Message)
			}
		}
	}
}

// findingsWith returns the findings whose message is exactly message.
func findingsWith(fs []LintFinding, message string) []LintFinding {
	var out []LintFinding
	for _, f := range fs {
		if f.Message == message {
			out = append(out, f)
		}
	}
	return out
}

// A `use` target outside the workspace is not read: its recipe is not defined, and the report carries one warning for the target
// with fixed text. Without the fence the same use is read and nothing is reported.
func TestLintWithRootRefusesUseOutsideTheWorkspace(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../../outside/secret.probe\"\n\ntest \"t\"\n  open the app\n  open the vault\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	refused := findingsWith(fenced.References, refuseUseTarget)
	if len(refused) != 1 || refused[0].Line != 1 || !refused[0].Warning {
		t.Errorf("want one warning for the use target at line 1, got %+v", fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)

	open, err := LintSource(&config.Config{}, path, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.References) != 0 {
		t.Errorf("without the fence the use is read and the recipe defined; got %+v", open.References)
	}

	// LintFile takes the same option, and reads the linted file itself.
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fromFile, err := LintFile(&config.Config{}, path, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	if len(findingsWith(fromFile.References, refuseUseTarget)) != 1 {
		t.Errorf("LintFile with the fence should refuse the use target too, got %+v", fromFile.References)
	}
}

// A `use` inside a recipe file that is inside the workspace, pointing outside, is refused too: its finding has no line in the
// linted file.
func TestLintWithRootRefusesNestedUseOutsideTheWorkspace(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	chain := "use \"../../outside/secret.probe\"\n\nrecipe \"chain\"\n  tap \"Menu\"\n"
	if err := os.WriteFile(filepath.Join(ws, "parts", "chain.probe"), []byte(chain), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../parts/chain.probe\"\n\ntest \"t\"\n  open the app\n  open the vault\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	refused := findingsWith(fenced.References, refuseUseTarget)
	if len(refused) != 1 || refused[0].Line != 0 {
		t.Errorf("want one warning for the nested use with no line, got %+v", fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)
}

// A CSV source outside the workspace is not read, so a placeholder only that file would define stays undefined, and the report
// names the refused source with fixed text. An absolute source inside the workspace is read as before.
func TestLintWithRootRefusesCSVOutsideTheWorkspace(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	csv := filepath.Join(outside, "secret.csv")
	content := "test \"t\"\n  open the app\n  tap \"<guessedheader>\"\n\nwith examples from \"" + csv + "\"\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	refused := findingsWith(fenced.References, refuseCSVSource)
	if len(refused) != 1 || refused[0].Line != 5 || !refused[0].Warning {
		t.Errorf("want one warning for the CSV source at line 5, got %+v", fenced.References)
	}
	if len(findingsWith(fenced.References, "<guessedheader> is not defined: no example column, recipe parameter or stored value has that name")) != 1 {
		t.Errorf("the placeholder should stay undefined, got %+v", fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)

	// Without the fence the absolute CSV is read and its header defines the placeholder.
	open, err := LintSource(&config.Config{}, path, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.References) != 0 {
		t.Errorf("without the fence the CSV header defines the placeholder; got %+v", open.References)
	}

	// A CSV inside the workspace is read under the fence.
	inside := "test \"t\"\n  open the app\n  tap \"<guessedheader>\"\n\nwith examples from \"data.csv\"\n"
	readInside, err := LintSource(&config.Config{}, path, inside, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	if len(readInside.References) != 0 {
		t.Errorf("a CSV inside the workspace should be read, got %+v", readInside.References)
	}
}

// A recipes folder outside the workspace is skipped: one warning at line 0, and the recipe calls the folder would define are not
// reported as unknown steps. Both an absolute folder and one reached with ../ from the working directory are refused. Without the
// fence the folder is read.
func TestLintWithRootSkipsTheRecipesFolderOutsideTheWorkspace(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "test \"t\"\n  open the app\n  open the vault\n  open the vault\n"
	cases := []struct {
		name    string
		recipes string
		chdir   string
	}{
		{"absolute folder", filepath.Join(outside, "recipes"), ""},
		{"folder reached with ../ from the tests folder", "../../outside/recipes", filepath.Join(ws, "tests")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.chdir != "" {
				t.Chdir(c.chdir)
			}
			cfg := &config.Config{Recipes: c.recipes}
			fenced, err := LintSource(cfg, path, content, LintWithRoot(ws))
			if err != nil {
				t.Fatal(err)
			}
			refused := findingsWith(fenced.References, refuseRecipesFolder)
			if len(refused) != 1 || refused[0].Line != 0 || !refused[0].Warning {
				t.Errorf("want one warning for the recipes folder at line 0, got %+v", fenced.References)
			}
			for _, f := range fenced.References {
				if !f.Warning {
					t.Errorf("no unknown-step error while the folder is skipped; got line %d %q", f.Line, f.Message)
				}
			}
			assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)

			open, err := LintSource(cfg, path, content)
			if err != nil {
				t.Fatal(err)
			}
			if len(open.References) != 0 {
				t.Errorf("without the fence the folder is read and the recipe defined; got %+v", open.References)
			}
		})
	}
}

// A recipes folder inside the workspace is read under the fence, as before.
func TestLintWithRootReadsTheRecipesFolderInside(t *testing.T) {
	ws, _ := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "test \"t\"\n  open the app\n  open menu\n"
	fenced, err := LintSource(&config.Config{Recipes: filepath.Join(ws, "recipes")}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	if len(fenced.References) != 0 {
		t.Errorf("the recipe in the workspace folder should resolve, got %+v", fenced.References)
	}
}

// A symlink inside the workspace that points outside is refused as a use target: it is not read, and the report names no file.
func TestLintWithRootRefusesSymlinkPointingOut(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	link := filepath.Join(ws, "tests", "link.probe")
	if err := os.Symlink(filepath.Join(outside, "secret.probe"), link); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"link.probe\"\n\ntest \"t\"\n  open the app\n  open the vault\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	refused := findingsWith(fenced.References, refuseUseTarget)
	if len(refused) != 1 || refused[0].Line != 1 {
		t.Errorf("want one warning for the symlink at line 1, got %+v", fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)
}

// A file inside the workspace that does not parse is named by its relative path and the parser's line. The finding carries none
// of the file's text, although the parser's own message quotes it. Without the fence the same parse error comes back as an error.
func TestLintWithRootNamesABrokenFileWithoutItsText(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../parts/broken.probe\"\n\ntest \"t\"\n  open the app\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatalf("a broken file inside the workspace should be a finding, not an error: %v", err)
	}
	broken := findingsWith(fenced.References, "parts/broken.probe does not parse: first error at its line 2")
	if len(broken) != 1 || broken[0].Line != 1 || broken[0].Warning {
		t.Errorf("want one error for the broken file at the use line, got %+v", fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)

	if _, err := LintSource(&config.Config{}, path, content); err == nil || !strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Errorf("without the fence the parse error is returned, and the parser quotes the file: err = %v", err)
	}
}

// A use target inside the workspace that is missing is named relatively, as it was in the CLI's message, with no absolute path.
func TestLintWithRootNamesAMissingTargetRelatively(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../recipes/none.probe\"\n\ntest \"t\"\n  open the app\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	want := "use \"../recipes/none.probe\": no file at recipes/none.probe"
	if len(findingsWith(fenced.References, want)) != 1 {
		t.Errorf("want the missing target named relatively %q, got %+v", want, fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)
}

// A use target inside the workspace that exists is read under the fence exactly as without it.
func TestLintWithRootReadsAUseInside(t *testing.T) {
	ws, _ := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../recipes/open_menu.probe\"\n\ntest \"t\"\n  open the app\n  open menu\n"
	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	open, err := LintSource(&config.Config{}, path, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(fenced.References) != 0 || len(open.References) != 0 {
		t.Errorf("a use inside the workspace should resolve in both modes: fenced %+v, open %+v", fenced.References, open.References)
	}
}

// An empty root is an error, so a caller cannot lint unfenced by accident.
func TestLintWithRootEmptyIsAnError(t *testing.T) {
	ws, _ := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	if _, err := LintSource(&config.Config{}, path, "test \"t\"\n  open the app\n", LintWithRoot("")); err == nil {
		t.Fatal("an empty root should be an error")
	}
}

// While a use target is refused, an unresolved recipe call is not reported: the refused file may define it, and the lint cannot read
// the file to tell. This is the trade-off: a call that nothing defines is not reported while a recipe source is refused, because the
// lint cannot tell it from one the refused source defines. Without the fence the use is read, the vault resolves, and the gizmo is the
// one unknown step.
func TestLintWithRootSuppressesCallsWhileASourceIsRefused(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../../outside/secret.probe\"\n\ntest \"t\"\n  open the app\n  open the vault\n  frobnicate the gizmo\n"

	fenced, err := LintSource(&config.Config{}, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fenced.References {
		if !f.Warning {
			t.Errorf("no unknown-step error while a source is refused; got line %d %q", f.Line, f.Message)
		}
	}
	if len(findingsWith(fenced.References, refuseUseTarget)) != 1 {
		t.Errorf("want one warning for the refused use target, got %+v", fenced.References)
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)

	open, err := LintSource(&config.Config{}, path, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.References) != 1 || open.References[0].Line != 6 || open.References[0].Warning || !strings.Contains(open.References[0].Message, `unknown step "frobnicate the gizmo"`) {
		t.Errorf("without the fence, want the one unknown-step error at line 6, got %+v", open.References)
	}
}

// With no refused source, a call that nothing defines is an unknown-step error with the fence on, exactly as without it.
func TestLintWithRootKeepsUnknownStepWhenNothingIsRefused(t *testing.T) {
	ws, _ := confinedWorkspace(t)
	path := filepath.Join(ws, "tests", "login.probe")
	content := "use \"../recipes/open_menu.probe\"\n\ntest \"t\"\n  open the app\n  open menu\n  frobnicate the gizmo\n"
	cases := []struct {
		label string
		opts  []LintOption
	}{
		{"fenced", []LintOption{LintWithRoot(ws)}},
		{"unfenced", nil},
	}
	for _, c := range cases {
		rep, err := LintSource(&config.Config{}, path, content, c.opts...)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.References) != 1 || rep.References[0].Line != 6 || rep.References[0].Warning || !strings.Contains(rep.References[0].Message, `unknown step "frobnicate the gizmo"`) {
			t.Errorf("%s: want the one unknown-step error at line 6, got %+v", c.label, rep.References)
		}
	}
}

// A recipe file in the recipes folder that resolves outside the workspace is not read: one warning for it, and the calls it could
// define are not reported. Without the fence the file is read through the link and the call resolves.
func TestLintWithRootRefusesARecipeFileOutsideTheFolder(t *testing.T) {
	ws, outside := confinedWorkspace(t)
	if err := os.Symlink(filepath.Join(outside, "secret.probe"), filepath.Join(ws, "recipes", "vault_link.probe")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	path := filepath.Join(ws, "tests", "login.probe")
	content := "test \"t\"\n  open the app\n  open the vault\n"
	cfg := &config.Config{Recipes: filepath.Join(ws, "recipes")}

	fenced, err := LintSource(cfg, path, content, LintWithRoot(ws))
	if err != nil {
		t.Fatal(err)
	}
	refused := findingsWith(fenced.References, refuseRecipeFile)
	if len(refused) != 1 || refused[0].Line != 0 || !refused[0].Warning {
		t.Errorf("want one warning for the recipe file at line 0, got %+v", fenced.References)
	}
	for _, f := range fenced.References {
		if !f.Warning {
			t.Errorf("no unknown-step error while a source is refused; got line %d %q", f.Line, f.Message)
		}
	}
	assertNoLeak(t, fenced, confinedLeaks(ws, outside)...)

	open, err := LintSource(cfg, path, content)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.References) != 0 {
		t.Errorf("without the fence the link is read and the recipe defined; got %+v", open.References)
	}
}
