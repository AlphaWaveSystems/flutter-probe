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
		if strings.HasPrefix(w.Message, "recipe ") && (strings.Contains(w.Message, "is a ProbeScript keyword") || strings.Contains(w.Message, "drops a leading") || strings.Contains(w.Message, "capital letters")) {
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

// A recipe whose name starts with a filler word the parser drops from a step is not reached by a call either: `to "x"` is looked up as
// "<arg>", `on the home screen` as "home screen". The warning says so, and does not call the word a keyword.
func TestLintWarnsARecipeNamedWithADroppedFiller(t *testing.T) {
	for _, src := range []string{
		"recipe \"on the home screen\"\n  open the app\n",
		"recipe \"is signed in\"\n  open the app\n",
		"recipe \"from the menu\"\n  open the app\n",
		"recipe \"in the cart\" (a)\n  open the app\n",
		"recipe \"to\" (x)\n  open the app\n",
	} {
		got := recipeNameWarnings(t, src)
		if len(got) != 1 || !got[0].Warning || got[0].Line != 1 {
			t.Fatalf("%q: want one warning on line 1, got %+v", src, got)
		}
		if !strings.Contains(got[0].Message, "drops a leading") || strings.Contains(got[0].Message, "keyword") {
			t.Errorf("%q: message %q should name the dropped word, not a keyword", src, got[0].Message)
		}
	}
}

// A call is looked up in lower case, so a recipe whose name has capital letters is never found. This was seen with the CLI: a recipe
// "Sign In" called as `sign in "x"` is an unknown step.
func TestLintWarnsARecipeNamedWithCapitals(t *testing.T) {
	got := recipeNameWarnings(t, "recipe \"Sign In\" (x)\n  open the app\n")
	if len(got) != 1 || !strings.Contains(got[0].Message, `"sign in"`) || !strings.Contains(got[0].Message, "lower case") {
		t.Fatalf("want one lower-case warning, got %+v", got)
	}
}

// The simulated call has one argument per parameter, so a recipe named with fillers only is reached when it has parameters and
// not when it has none.
func TestLintUsesTheParameterCountOfTheRecipe(t *testing.T) {
	if got := recipeNameWarnings(t, "recipe \"the\" (a, b)\n  open the app\n"); len(got) != 0 {
		t.Errorf("recipe \"the\" (a, b): unexpected warning %v", got)
	}
	if got := recipeNameWarnings(t, "recipe \"the\"\n  open the app\n"); len(got) != 1 {
		t.Errorf("recipe \"the\" with no parameters: want one warning, got %v", got)
	}
}
