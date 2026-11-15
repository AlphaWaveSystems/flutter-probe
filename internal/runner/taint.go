package runner

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Taint: a variable whose value came from a page (a WebView, once the read step exists) must not steer the runner. One mechanism carries it
// (docs/proposals/webview-read-and-taint.md):
//
//   - e.tainted holds the names of the tainted variables; every write to a variable goes through setVar, which sets or clears the flag.
//   - resolveT expands a text like resolve always has, and also says which tainted variables ended up in it. A tainted value is inserted as it is,
//     in one pass: it is never searched again for <name> placeholders, ${ENV} references or l10n keys, so a page text cannot read another variable.
//   - forSink is the only way a step that sends a string out of the UI (a shell, a file, an address, a request, a baseline key) takes that string from
//     the script: it refuses a text that holds a tainted value.
//
// Nothing in production code marks a variable as tainted yet, so with no tainted variable resolveT is exactly the expansion it was before.

// setVar sets a variable and records whether its value is tainted. Every write to e.vars goes through here (a test checks that), so a
// clean assignment always clears the flag the name had.
func (e *Executor) setVar(name, value string, tainted bool) {
	e.vars[name] = value
	if tainted {
		if e.tainted == nil {
			e.tainted = map[string]bool{}
		}
		e.tainted[name] = true
		return
	}
	delete(e.tainted, name)
}

// taintedNames is the sorted list of tainted variables that still exist.
func (e *Executor) taintedNames() []string {
	if len(e.tainted) == 0 {
		return nil
	}
	var names []string
	for n := range e.tainted {
		if _, ok := e.vars[n]; ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// resolveT expands variables, ${ENV} values, generators and l10n keys like resolve, and returns the names of the tainted variables whose value is
// part of the result.
func (e *Executor) resolveT(s string) (string, []string) { return e.expandT(s, true) }

// expandT is the one expansion. With withEnv false only variables and generators are expanded (the step lines).
func (e *Executor) expandT(s string, withEnv bool) (string, []string) {
	names := e.taintedNames()
	if len(names) == 0 {
		out := e.expandVars(s, nil)
		if withEnv {
			out = e.finishResolve(out)
		}
		return out, nil
	}
	// Tainted values travel as tokens until the end, so nothing that follows can read them or change them. A token holds a NUL: a script text
	// that happens to contain one has it removed first.
	s = strings.ReplaceAll(s, "\x00", "")
	skip := make(map[string]bool, len(names))
	token := make(map[string]string, len(names))
	for i, n := range names {
		skip[n] = true
		token[n] = "\x00T" + strconv.Itoa(i) + "\x00"
	}
	protect := func(text string) string {
		for _, n := range names {
			if strings.Contains(text, "<"+n+">") {
				text = strings.ReplaceAll(text, "<"+n+">", token[n])
			}
		}
		return text
	}
	// the variables that are not tainted are expanded first (a tainted one is skipped, so its placeholder is left as written); then every placeholder of a
	// tainted variable, whether the script wrote it or another variable's value produced it, becomes a token
	text := protect(e.expandVars(s, skip))
	if withEnv {
		text = e.finishResolve(text)
	}
	var used []string
	for _, n := range names {
		if strings.Contains(text, token[n]) {
			used = append(used, n)
			text = strings.ReplaceAll(text, token[n], e.vars[n])
		}
	}
	return text, used
}

// forSink expands a text for a step that sends it out of the UI. It refuses the text when a tainted value is part of it: the error names the
// variables (script text), never their values, and the caller sends nothing.
func (e *Executor) forSink(step, s string) (string, error) {
	out, by := e.resolveT(s)
	if len(by) > 0 {
		return "", taintedError(step, by)
	}
	return out, nil
}

func taintedError(step string, names []string) error {
	list := "<" + strings.Join(names, ">, <") + ">"
	return fmt.Errorf("%s: %s holds text read from a page, so it cannot be used to choose a link, a file, a command or an address; "+
		"a value read from a page can be typed, looked for (see, wait until) and used in selectors", step, list)
}
