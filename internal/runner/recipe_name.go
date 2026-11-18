package runner

import (
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// Why a recipe's name cannot be called.
const (
	RecipeNameKeyword = "keyword" // the name starts with a ProbeScript keyword: the call is that keyword's step plus a stray call
	RecipeNameSkipped = "skipped" // the parser drops a leading filler word from the call, so the call is looked up without it
	RecipeNameCase    = "case"    // the name has capital letters: a call is looked up in lower case, so it never finds the recipe
)

// RecipeCallProblem says whether a call written with a recipe's name reaches the recipe. It writes the call as a person does (the
// name, then one quoted argument per parameter), has the parser read it, and resolves the result against the recipe the way a run
// does (resolveCall), so it cannot disagree with `probe test`. It returns "" and "" when the call reaches the recipe, or when the
// name is not plain words and so is not simulated. Otherwise word is the first word of the name and why is RecipeNameKeyword,
// RecipeNameSkipped or RecipeNameCase.
func RecipeCallProblem(name string, params int) (word, why string) {
	rc, how, first := parser.SimulateRecipeCall(name, params)
	switch how {
	case parser.CallNotSimulated:
		return "", ""
	case parser.CallIsOtherStep:
		return first, RecipeNameKeyword
	case parser.CallIsNothing:
		return first, RecipeNameSkipped
	}
	if reaches(name, params, rc) {
		return "", ""
	}
	if lower := strings.ToLower(name); lower != name {
		if lrc, lhow, _ := parser.SimulateRecipeCall(lower, params); lhow == parser.CallIsRecipeCall && reaches(lower, params, lrc) {
			return first, RecipeNameCase
		}
	}
	return first, RecipeNameSkipped
}

// reaches reports whether the call resolves to a recipe declared with this name and parameter count.
func reaches(name string, params int, rc parser.RecipeCall) bool {
	def := parser.RecipeDef{Name: name, Params: make([]string, params)}
	_, _, _, found := resolveCall(map[string]parser.RecipeDef{name: def}, rc)
	return found
}
