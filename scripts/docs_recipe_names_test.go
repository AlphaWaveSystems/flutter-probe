package scripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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
	exts := map[string]bool{".md": true, ".mdx": true, ".probe": true}
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

// fencedBlocks returns the text of each fenced code block in a Markdown file.
func fencedBlocks(text string) []string {
	var blocks []string
	var cur []string
	in := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if in {
				blocks = append(blocks, strings.Join(cur, "\n"))
				cur = nil
			}
			in = !in
			continue
		}
		if in {
			cur = append(cur, line)
		}
	}
	return blocks
}

func stepLine(s parser.Step) int {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0
	}
	if f := v.FieldByName("Line"); f.IsValid() && f.Kind() == reflect.Int {
		return int(f.Int())
	}
	return 0
}

// splitLines returns the lines of a snippet that the parser reads as a step followed by a stray recipe call: two steps on one
// source line, the second a recipe call. That is what a call to a recipe named with a keyword becomes (`log in as "u" with "p"` is
// the log step and a call named `as <arg> with <arg>`).
func splitLines(steps []parser.Step) []int {
	var lines []int
	for i := 1; i < len(steps); i++ {
		if _, ok := steps[i].(parser.RecipeCall); !ok {
			continue
		}
		if l := stepLine(steps[i]); l > 0 && l == stepLine(steps[i-1]) {
			lines = append(lines, l)
		}
	}
	return lines
}

func programSteps(prog *parser.Program) [][]parser.Step {
	var all [][]parser.Step
	for _, tst := range prog.Tests {
		all = append(all, tst.Body)
	}
	for _, h := range prog.Hooks {
		all = append(all, h.Body)
	}
	for _, r := range prog.Recipes {
		all = append(all, r.Body)
	}
	return all
}

// A call to a recipe named with a keyword fails even when the recipe is declared on another page (the Hooks page called
// `log in as` and declared nothing). So the snippets are parsed too: a snippet that parses must not hold a line that is
// read as a step plus a stray recipe call. A snippet that does not parse (an excerpt, a fragment) is skipped.
func TestDocumentedSnippetsHaveNoSplitLines(t *testing.T) {
	scanned, parsed := 0, 0
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
		if filepath.Ext(p) != ".md" || d.Name() == "CHANGELOG.md" {
			return nil
		}
		if strings.Contains(filepath.ToSlash(p), "docs/proposals/") {
			return nil // a proposal shows syntax that does not exist yet
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		scanned++
		for _, block := range fencedBlocks(string(b)) {
			if !strings.Contains(block, "test \"") && !strings.Contains(block, "recipe \"") && !strings.Contains(block, "before ") {
				continue
			}
			prog, err := parser.ParseFile(block)
			if err != nil {
				continue
			}
			parsed++
			for _, steps := range programSteps(prog) {
				for _, l := range splitLines(steps) {
					line := strings.Split(block, "\n")[l-1]
					problems = append(problems, filepath.ToSlash(strings.TrimPrefix(p, recipeDocsRoot+string(filepath.Separator)))+": "+strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 50 || parsed < 20 {
		t.Fatalf("scanned %d files and parsed %d snippets: the walk is not reaching the docs", scanned, parsed)
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Errorf("a documented step line is read as a step plus a stray recipe call: %s", p)
	}
}
