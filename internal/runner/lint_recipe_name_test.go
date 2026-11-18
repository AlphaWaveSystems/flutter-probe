package runner

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func recipeNameWarnings(t *testing.T, src string) []LintFinding {
	t.Helper()
	prog, err := parser.ParseFile(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var got []LintFinding
	for _, w := range lintProgram(prog) {
		if strings.Contains(w.Message, "is a ProbeScript keyword") {
			got = append(got, w)
		}
	}
	return got
}

// `recipe "log in as"` parses, but `log in as "u" with "p"` is the log step and a stray call, so nothing can call it. The scaffold
// of `probe init` and three docs pages used that name.
func TestLintWarnsARecipeNamedWithAKeyword(t *testing.T) {
	got := recipeNameWarnings(t, "recipe \"log in as\" (email, password)\n  open the app\n")
	if len(got) != 1 {
		t.Fatalf("want one warning, got %v", got)
	}
	w := got[0]
	if !w.Warning || w.Line != 1 {
		t.Errorf("want a warning on line 1, got %+v", w)
	}
	for _, want := range []string{`recipe "log in as"`, `"log"`, "sign in as"} {
		if !strings.Contains(w.Message, want) {
			t.Errorf("message %q lacks %q", w.Message, want)
		}
	}
}

func TestLintDoesNotWarnARecipeNamedWithoutAKeyword(t *testing.T) {
	for _, name := range []string{"sign in as", "login as", "go to the checkout", "the login", "add item to cart"} {
		if got := recipeNameWarnings(t, "recipe \""+name+"\" (a)\n  open the app\n"); len(got) != 0 {
			t.Errorf("recipe %q: unexpected warning %v", name, got)
		}
	}
}

func TestLintReportsTheLineOfEachKeywordRecipe(t *testing.T) {
	src := "recipe \"sign in as\" (a)\n  open the app\n\nrecipe \"take a break\"\n  open the app\n\nrecipe \"tap the logo\"\n  open the app\n"
	got := recipeNameWarnings(t, src)
	if len(got) != 2 || got[0].Line != 4 || got[1].Line != 7 {
		t.Fatalf("want warnings on lines 4 and 7, got %+v", got)
	}
}

// The call that is simulated has one argument per parameter: a recipe named like a filler word is a call only when it has a parameter.
func TestLintUsesTheParameterCountOfTheRecipe(t *testing.T) {
	if got := recipeNameWarnings(t, "recipe \"to\" (x)\n  open the app\n"); len(got) != 0 {
		t.Errorf("recipe \"to\" (x): unexpected warning %v", got)
	}
	if got := recipeNameWarnings(t, "recipe \"to\"\n  open the app\n"); len(got) != 1 {
		t.Errorf("recipe \"to\" with no parameters: want one warning, got %v", got)
	}
}
