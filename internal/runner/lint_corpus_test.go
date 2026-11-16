package runner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// lintCorpusDir is the synthetic corpus: one file per lint rule and boundary, each with a comment that says what it should
// trigger. recipes/ is the recipes folder the second configuration reads.
const lintCorpusDir = "testdata/lintcorpus"

// lintCorpusGolden is what LintFile reports for every file of the corpus, under both configurations. Regenerate it after a
// deliberate change to a message with UPDATE_LINT_GOLDEN=1 go test ./internal/runner -run TestLintCorpusGolden.
const lintCorpusGolden = "testdata/lintcorpus/golden.txt"

// renderLintReport writes one file's report as text: the program warnings, then the reference findings with their lines, or
// the error that stopped the lint. It is the same content probe lint prints, without the marks and the colours.
func renderLintReport(b *strings.Builder, label, path string, report LintReport, err error) {
	fmt.Fprintf(b, "### %s %s\n", label, path)
	if err != nil {
		fmt.Fprintf(b, "error: %v\n", err)
		return
	}
	for _, w := range report.Program {
		fmt.Fprintf(b, "program warning: %s\n", w)
	}
	for _, f := range report.References {
		kind := "error"
		if f.Warning {
			kind = "warning"
		}
		fmt.Fprintf(b, "reference %s: line %d: %s\n", kind, f.Line, f.Message)
	}
}

// lintCorpusFiles lists the .probe files of the corpus, in order, the recipes folder included.
func lintCorpusFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(lintCorpusDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".probe") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

// TestLintCorpusGolden runs LintFile over every file of the corpus, with no recipes folder and with the corpus recipes folder,
// and compares the report with the golden file. Every message of the program rules and of the reference rules appears in it.
func TestLintCorpusGolden(t *testing.T) {
	configs := []struct {
		label string
		cfg   *config.Config
	}{
		{"none", &config.Config{}},
		{"rec", &config.Config{Recipes: lintCorpusDir + "/recipes"}},
	}
	var b strings.Builder
	for _, c := range configs {
		for _, path := range lintCorpusFiles(t) {
			report, err := LintFile(c.cfg, path)
			renderLintReport(&b, c.label, path, report, err)
		}
	}
	got := b.String()
	if os.Getenv("UPDATE_LINT_GOLDEN") != "" {
		if err := os.WriteFile(lintCorpusGolden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(lintCorpusGolden)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with UPDATE_LINT_GOLDEN=1)", lintCorpusGolden, err)
	}
	if got != string(want) {
		gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
			var g, w string
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if g != w {
				t.Fatalf("LintFile output differs from %s at line %d:\n  want: %s\n  got:  %s\nregenerate with UPDATE_LINT_GOLDEN=1 if the change is deliberate", lintCorpusGolden, i+1, w, g)
			}
		}
	}
}
