package parser

import (
	"regexp"
	"strings"
)

// plainRecipeName is a name made of words only: letters, digits, spaces, hyphens and apostrophes. A name with anything else (a
// quote, a placeholder, a symbol) is not simulated, because the line built from it would not be the call a person writes.
var plainRecipeName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} '-]*$`)

// RecipeCallProblem reports whether a call to a recipe with this name and this many parameters would be read as a call to it.
// It returns "" when it is, and otherwise the first word of the name, which the step parser takes for something else.
//
// A call is the name written as a step, followed by one quoted argument per parameter. A name that starts with a ProbeScript
// keyword (`log in as`, `home page`, `take a break`) is read as that keyword's step and a stray call after it, so the recipe
// parses but no call can reach it. A name whose simulated call does not parse is reported the same way. A name that is not
// plain words, or has no words, returns "": it is not simulated.
func RecipeCallProblem(name string, params int) string {
	name = strings.TrimSpace(name)
	if !plainRecipeName.MatchString(name) {
		return ""
	}
	if params < 0 {
		params = 0
	}
	src := "test \"t\"\n  " + name + strings.Repeat(` "x"`, params) + "\n"
	prog, err := ParseFile(src)
	if err == nil && len(prog.Tests) == 1 && len(prog.Tests[0].Body) == 1 {
		if _, ok := prog.Tests[0].Body[0].(RecipeCall); ok {
			return ""
		}
	}
	return strings.Fields(name)[0]
}
