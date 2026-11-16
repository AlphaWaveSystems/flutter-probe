package runner

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// lintConfigs are the two configurations the corpus is linted under: no recipes folder, and the corpus recipes folder.
func lintConfigs() []struct {
	label string
	cfg   *config.Config
} {
	return []struct {
		label string
		cfg   *config.Config
	}{
		{"none", &config.Config{}},
		{"rec", &config.Config{Recipes: lintCorpusDir + "/recipes"}},
	}
}

// LintSource on a file's own text gives the report LintFile gives, for every file of the corpus under both configurations.
func TestLintSourceMatchesLintFile(t *testing.T) {
	for _, c := range lintConfigs() {
		for _, path := range lintCorpusFiles(t) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fromFile, errFile := LintFile(c.cfg, path)
			fromSource, errSource := LintSource(c.cfg, path, string(src))
			if (errFile == nil) != (errSource == nil) {
				t.Fatalf("%s [%s]: LintFile err = %v, LintSource err = %v", path, c.label, errFile, errSource)
			}
			if !reflect.DeepEqual(fromFile, fromSource) {
				t.Errorf("%s [%s]: LintSource differs from LintFile:\n  file:   %+v\n  source: %+v", path, c.label, fromFile, fromSource)
			}
		}
	}
}

// LintSource lints the text it is given, not the file at path: the editor's unsaved text is what Studio shows. The path still
// resolves the use targets relative to the file's folder.
func TestLintSourceLintsTheTextNotTheFile(t *testing.T) {
	path := lintCorpusDir + "/ref_use_present.probe"
	clean, err := LintSource(&config.Config{}, path, "use \"recipes/open_menu.probe\"\n\ntest \"t\"\n  open menu\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(clean.References) != 0 || len(clean.Program) != 0 {
		t.Errorf("use resolved against the file's folder should give no findings, got %+v %+v", clean.Program, clean.References)
	}
	broken, err := LintSource(&config.Config{}, path, "test \"t\"\n  frobnicate the gizmo\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(broken.References) != 1 || broken.References[0].Warning {
		t.Errorf("the editor text has an unknown step; want one reference error, got %+v", broken.References)
	}
	disk, err := LintFile(&config.Config{}, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(disk.References) != 0 {
		t.Errorf("the file on disk is clean; LintFile should not see the editor text, got %+v", disk.References)
	}
}

// Program findings carry the line the parser has: the test's line for a test with no steps and for a dart share, the step's
// line for a count check.
func TestLintProgramFindingsCarryTheirLines(t *testing.T) {
	src := "test \"no steps\"\n\ntest \"dart\"\n  open the app\n  tap \"A\"\n  run dart:\n    print('x')\ntest \"count\"\n  open the app\n  see \"0 items\"\n"
	report, err := LintSource(&config.Config{}, "inline.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	type line struct {
		line    int
		message string
	}
	var got []line
	for _, p := range report.Program {
		got = append(got, line{p.Line, p.Message})
	}
	want := []line{
		{1, `test "no steps" has no steps`},
		{3, `test "dart": 33% dart blocks (consider ProbeScript instead)`},
		{10, `test "count", line 10: "0 items" matches any longer text that contains it; for an exact count or zero-state check add matching "^0 items$"`},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d program findings, want %d: %+v", len(got), len(want), report.Program)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d: got line %d %q, want line %d %q", i, got[i].line, got[i].message, want[i].line, want[i].message)
		}
	}
}

// LintSource holds no package state, so concurrent calls on different texts must each return the report of their own text.
// Run with -race.
func TestLintSourceConcurrentCalls(t *testing.T) {
	const calls = 8
	sources := make([]string, calls)
	for i := range sources {
		sources[i] = fmt.Sprintf("test \"t%d\"\n  open the app\n  see \"%d items\"\n  frobnicate step %d\n", i, i, i)
	}
	want := make([]LintReport, calls)
	for i, src := range sources {
		r, err := LintSource(&config.Config{}, "concurrent.probe", src)
		if err != nil {
			t.Fatal(err)
		}
		want[i] = r
	}
	got := make([]LintReport, calls)
	errs := make([]error, calls)
	var wg sync.WaitGroup
	for i := range sources {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i], errs[i] = LintSource(&config.Config{}, "concurrent.probe", sources[i])
		}(i)
	}
	wg.Wait()
	for i := range sources {
		if errs[i] != nil {
			t.Fatalf("call %d: %v", i, errs[i])
		}
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("call %d: concurrent report differs from the sequential one:\n  got:  %+v\n  want: %+v", i, got[i], want[i])
		}
	}
}
