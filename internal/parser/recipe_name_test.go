package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestRecipeCallProblem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params int
		want   string // the first word when the call is not read as a recipe call, "" when it is
	}{
		{"sign in as", 2, ""},
		{"log in as", 2, "log"}, // log is a keyword: the call is the log step plus a stray call
		{"log in", 0, "log"},
		{"login as", 2, ""},
		{"go to the checkout", 0, ""},
		{"the login", 1, ""}, // a filler word at the start is skipped and the call still resolves
		{"add item to cart", 1, ""},
		{"home page", 0, ""},
		{"take a break", 0, "take"},
		{"pause the app", 0, "pause"},
		{"tap the logo", 1, "tap"},
		{"go back", 0, "go"},
		{"open the menu", 0, ""},
		{"restart everything", 0, "restart"},
		{"press the home key", 0, ""},
		{"2nd attempt", 0, ""},
		// The simulated call carries one quoted argument per parameter, so the parameter count matters. Only a filler word on
		// its own shows it: `to` alone is skipped as a filler (no call), `to "x"` is a call.
		{"to", 0, "to"},
		{"to", 1, ""},
		{"the", 0, "the"},
		{"the", 2, ""},
		{"", 0, ""},             // no words: not simulated
		{`say "hi"`, 0, ""},     // not plain words: not simulated
		{"fill <email>", 1, ""}, // a placeholder: not simulated
	} {
		if got := parser.RecipeCallProblem(tc.name, tc.params); got != tc.want {
			t.Errorf("RecipeCallProblem(%q, %d) = %q, want %q", tc.name, tc.params, got, tc.want)
		}
	}
}
