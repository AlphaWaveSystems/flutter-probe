package runner

import (
	"sort"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// EnvRef is a ${NAME} reference in a step of a file, and the line of the step.
type EnvRef struct {
	Name string
	Line int
}

// UnsetEnvRefs parses content and returns the ${NAME} references in the strings of its steps (tests, hooks, recipes and composite tests) whose
// variable is not set, so a caller can say that a run will use the text as written (the way expandEnv treats an unset variable). It reads the
// step strings the parser holds, so a reference in a comment, which the parser drops, is not one. isSet only says whether a name exists: the
// function never sees or returns a value. Each name is reported once per line, in line order.
func UnsetEnvRefs(content string, isSet func(name string) bool) ([]EnvRef, error) {
	prog, err := parser.ParseFile(content)
	if err != nil {
		return nil, err
	}
	var bodies [][]parser.Step
	for _, t := range prog.Tests {
		bodies = append(bodies, t.Body)
	}
	for _, h := range prog.Hooks {
		bodies = append(bodies, h.Body)
	}
	for _, rec := range prog.Recipes {
		bodies = append(bodies, rec.Body)
	}
	for _, c := range prog.CompositeTests {
		bodies = append(bodies, c.Body)
	}
	seen := map[EnvRef]bool{}
	var out []EnvRef
	for _, body := range bodies {
		walkSteps(body, func(s parser.Step) {
			texts, _ := scanStep(s)
			for _, text := range texts {
				for _, m := range envRef.FindAllString(text, -1) {
					ref := EnvRef{Name: m[2 : len(m)-1], Line: stepLine(s)}
					if seen[ref] || isSet(ref.Name) {
						continue
					}
					seen[ref] = true
					out = append(out, ref)
				}
			}
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out, nil
}
