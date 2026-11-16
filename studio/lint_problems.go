package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"unicode"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// LintProblem is one finding of the project's lint rules for the open file: a reference that does not resolve (a recipe, a data file, a
// placeholder) or a rule of the program (a test with no steps, a count check). Line is 1-based, or 0 where the rule has none.
type LintProblem struct {
	Line    int    `json:"line"`
	Warning bool   `json:"warning"`
	Message string `json:"message"`
	Source  string `json:"source"` // "reference" or "program"
}

// LintProblemsResult is what the Problems list shows. Unavailable says, in one line, why the file could not be checked (text that does not
// parse yet, a recipes folder that cannot be read); it is not a finding and it never stops a run.
type LintProblemsResult struct {
	Problems    []LintProblem `json:"problems"`
	Unavailable string        `json:"unavailable,omitempty"`
}

const maxProblemText = 400

// lintMu keeps the lint of Studio to one call at a time: the editor asks again on every pause in typing.
var lintMu sync.Mutex

// LintProblems checks the editor's text of an open file with the same rules `probe lint` uses (runner.LintSource), against the probe.yaml
// of the open workspace. path must be a .probe file inside the workspace (the check ReadFile makes) and only resolves the recipes and data
// files the text names; content is the text to check, saved or not. It reads files in the workspace and writes nothing.
func (a *App) LintProblems(path, content string) (LintProblemsResult, error) {
	if hasDotDotSegment(path) {
		return LintProblemsResult{}, fmt.Errorf("path traversal rejected: %s", path)
	}
	real, err := probeFileWithinRoots(a.automationRoots(), path)
	if err != nil {
		return LintProblemsResult{}, err
	}
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	cfg := a.workspaceConfig()
	// The recipes folder is given relative to the workspace, like the CLI run from there; a relative one would otherwise resolve against
	// whatever folder Studio was started in.
	if ws != "" && cfg.Recipes != "" && !filepath.IsAbs(cfg.Recipes) {
		cfg.Recipes = filepath.Join(ws, cfg.Recipes)
	}
	lintMu.Lock()
	rep, err := runner.LintSource(cfg, real, content)
	lintMu.Unlock()
	if err != nil {
		return LintProblemsResult{Problems: []LintProblem{}, Unavailable: "Not checked: " + oneLine(err.Error())}, nil
	}
	out := LintProblemsResult{Problems: []LintProblem{}}
	for _, f := range rep.References {
		out.Problems = append(out.Problems, LintProblem{Line: f.Line, Warning: f.Warning, Message: oneLine(f.Message), Source: "reference"})
	}
	for _, f := range rep.Program {
		out.Problems = append(out.Problems, LintProblem{Line: f.Line, Warning: f.Warning, Message: oneLine(f.Message), Source: "program"})
	}
	return out, nil
}

// oneLine makes text from a finding safe to show: one line, no control or invisible characters, a bounded length.
func oneLine(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out = append(out, ' ')
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ':
			continue
		default:
			out = append(out, r)
		}
		if len(out) >= maxProblemText {
			out = append(out, '…')
			break
		}
	}
	return string(out)
}
