package scripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// A recipe whose name starts with a ProbeScript keyword (`recipe "log in as"`) parses, but a call to it is read as that
// keyword's step and a stray call, so no test can call it. The Recipes, Hooks and tutorial pages and the `probe init`
// scaffold used that name. This test reads every recipe declaration in the docs and READMEs and runs it through the
// parser's own check (parser.RecipeCallProblem), so a new example cannot bring the problem back.

const recipeDocsRoot = ".."

var recipeDecl = regexp.MustCompile(`(?m)^\s*recipe\s+"([^"\n]+)"\s*(?:\(([^)\n]*)\))?\s*$`)

func TestDocumentedRecipesCanBeCalled(t *testing.T) {
	exts := map[string]bool{".md": true, ".mdx": true}
	scanned, decls := 0, 0
	var problems []string
	err := filepath.WalkDir(recipeDocsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules", "research":
				return filepath.SkipDir
			}
			return nil
		}
		if !exts[filepath.Ext(p)] || d.Name() == "CHANGELOG.md" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		scanned++
		for _, m := range recipeDecl.FindAllStringSubmatch(string(b), -1) {
			decls++
			params := 0
			if strings.TrimSpace(m[2]) != "" {
				params = len(strings.Split(m[2], ","))
			}
			if word := parser.RecipeCallProblem(m[1], params); word != "" {
				problems = append(problems, filepath.ToSlash(strings.TrimPrefix(p, recipeDocsRoot+string(filepath.Separator)))+": recipe \""+m[1]+"\" starts with the keyword "+word)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 || decls < 5 {
		t.Fatalf("scanned %d files and %d recipe declarations: the walk is not reaching the docs", scanned, decls)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("a documented recipe cannot be called: %s", p)
	}
}
