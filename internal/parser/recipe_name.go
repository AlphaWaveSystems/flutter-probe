package parser

import (
	"regexp"
	"strings"
)

// plainRecipeName is a name made of words only: letters, digits, spaces, hyphens and apostrophes. A name with anything else (a
// quote, a placeholder, a symbol) is not simulated, because the line built from it would not be the call a person writes.
var plainRecipeName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} '-]*$`)

// What the parser makes of the call written with a recipe's name.
type CallReading int

const (
	CallNotSimulated CallReading = iota // the name is not plain words: nothing was parsed
	CallIsRecipeCall                    // one recipe call
	CallIsOtherStep                     // something else: a keyword's step (plus a stray call), or a parse error
	CallIsNothing                       // no step at all: the parser dropped every word as a filler
)

// SimulateRecipeCall parses the call a person writes to run a recipe with this name and this many parameters: the name written
// as a step, followed by one quoted argument per parameter. It returns what the parser made of that line, and the call when it is
// one recipe call. A name that starts with a ProbeScript keyword, such as `log in as`, becomes that keyword's step plus a stray
// call (CallIsOtherStep). first is the first word of the name.
//
// The call it returns has the name the parser gives it, which is not always the recipe's name: the parser drops a filler word at
// the start of a step (`to`, `in`, `on`, `is`, `from`, ...) and lower-cases the words. Whether that call reaches the recipe is the
// runner's lookup; runner.RecipeCallProblem asks it.
func SimulateRecipeCall(name string, params int) (call RecipeCall, how CallReading, first string) {
	name = strings.TrimSpace(name)
	if !plainRecipeName.MatchString(name) {
		return RecipeCall{}, CallNotSimulated, ""
	}
	if params < 0 {
		params = 0
	}
	first = strings.Fields(name)[0]
	src := "test \"t\"\n  " + name + strings.Repeat(` "x"`, params) + "\n"
	prog, err := ParseFile(src)
	if err != nil || len(prog.Tests) != 1 {
		return RecipeCall{}, CallIsOtherStep, first
	}
	switch body := prog.Tests[0].Body; {
	case len(body) == 0:
		return RecipeCall{}, CallIsNothing, first
	case len(body) == 1:
		if rc, ok := body[0].(RecipeCall); ok {
			return rc, CallIsRecipeCall, first
		}
	}
	return RecipeCall{}, CallIsOtherStep, first
}
