package runner

import "testing"

// A call written with a recipe's name must reach the recipe. These are the names whose calls do not, and the reason, checked
// against what `probe lint` of a declaration plus a call reports ("unknown step ...").
func TestRecipeCallProblem(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params int
		word   string
		why    string
	}{
		// reaches the recipe
		{"sign in as", 2, "", ""},
		{"login as", 2, "", ""},
		{"go to the checkout", 0, "", ""},
		{"add item to cart", 1, "", ""},
		{"the login", 1, "", ""}, // the filler is dropped from the call and from the name when they are compared
		{"and then continue", 0, "", ""},
		{"with the user", 1, "", ""},
		{"step 2 of onboarding", 0, "", ""},
		{"home page", 0, "", ""},
		{"open the menu", 0, "", ""},
		// starts with a keyword: the call is that keyword's step plus a stray call
		{"log in as", 2, "log", RecipeNameKeyword},
		{"take a break", 0, "take", RecipeNameKeyword},
		{"pause the app", 0, "pause", RecipeNameKeyword},
		{"tap the logo", 1, "tap", RecipeNameKeyword},
		{"go back to main menu", 0, "go", RecipeNameKeyword},
		// starts with a filler the parser drops from a step: the call is looked up without it (unknown step "home screen")
		{"on the home screen", 0, "on", RecipeNameSkipped},
		{"is signed in", 0, "is", RecipeNameSkipped},
		{"from the menu", 0, "from", RecipeNameSkipped},
		{"in the cart", 1, "in", RecipeNameSkipped},
		{"to", 1, "to", RecipeNameSkipped}, // `to "x"` is looked up as "<arg>"
		{"to", 0, "to", RecipeNameSkipped},
		{"a", 1, "a", RecipeNameSkipped},
		{"into the form", 0, "into", RecipeNameSkipped},
		{"the", 0, "the", RecipeNameSkipped}, // every word is dropped: the call is no step at all
		// The call carries one argument per parameter, so the count matters: with two parameters `the "x" "x"` is a call that resolves.
		{"the", 2, "", ""},
		{"the", 1, "", ""},
		// capital letters: a call is lower-cased, a recipe name is not
		{"Sign In", 0, "Sign", RecipeNameCase},
		{"Sign in as", 2, "Sign", RecipeNameCase},
		// not simulated
		{"", 0, "", ""},
		{`say "hi"`, 0, "", ""},
		{"fill <email>", 1, "", ""},
	} {
		word, why := RecipeCallProblem(tc.name, tc.params)
		if word != tc.word || why != tc.why {
			t.Errorf("RecipeCallProblem(%q, %d) = (%q, %q), want (%q, %q)", tc.name, tc.params, word, why, tc.word, tc.why)
		}
	}
}
