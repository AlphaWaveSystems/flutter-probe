package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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
	// Stale says a newer request came while this one waited: nothing was checked, and the window ignores the answer.
	Stale bool `json:"stale,omitempty"`
}

const maxProblemText = 400

// maxLintBytes is the largest text Studio lints: above it the answer is one line, not a pause while the editor types.
const maxLintBytes = 1 << 20

// lintMu keeps the lint of Studio to one call at a time. A call takes a ticket first; one that finds a newer ticket when its turn comes does
// not run (the editor already has newer text), so a burst of requests never queues stale lints behind the running one.
var (
	lintMu  sync.Mutex
	lintSeq atomic.Uint64
)

// LintProblems checks the editor's text of an open file with the same rules `probe lint` uses (runner.LintSource), against the probe.yaml
// of the open workspace. path must be a .probe file inside the workspace (the check ReadFile makes) and only resolves the recipes and data
// files the text names; content is the text to check, saved or not. It reads files in the workspace and writes nothing.
func (a *App) LintProblems(path, content string) (LintProblemsResult, error) {
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	root := ws
	if root == "" {
		if roots := a.automationRoots(); len(roots) > 0 {
			root = roots[0]
		}
	}
	// A fence around the whole file system fences nothing: with a root of / (or a drive) the lint says so and reads nothing.
	if isFilesystemRoot(root) {
		return LintProblemsResult{Problems: []LintProblem{}, Unavailable: "Not checked: the open workspace is the root of the file system, so the lint cannot tell inside from outside. Open your project folder."}, nil
	}
	if hasDotDotSegment(path) {
		return LintProblemsResult{}, fmt.Errorf("path traversal rejected: %s", path)
	}
	real, err := probeFileWithinRoots(a.automationRoots(), path)
	if err != nil {
		return LintProblemsResult{}, err
	}
	if len(content) > maxLintBytes {
		return LintProblemsResult{Problems: []LintProblem{}, Unavailable: "Not checked: the file is too large (over 1 MiB)"}, nil
	}
	cfg := a.workspaceConfig()
	// The recipes folder is given relative to the workspace, like the CLI run from there; a relative one would otherwise resolve against
	// whatever folder Studio was started in.
	if ws != "" && cfg.Recipes != "" && !filepath.IsAbs(cfg.Recipes) {
		cfg.Recipes = filepath.Join(ws, cfg.Recipes)
	}
	ticket := lintSeq.Add(1)
	lintMu.Lock()
	if lintSeq.Load() != ticket {
		lintMu.Unlock()
		return LintProblemsResult{Problems: []LintProblem{}, Stale: true}, nil
	}
	// The lint is fenced to the workspace: the editor's text and the probe.yaml of an opened folder are not trusted to name files outside it.
	rep, err := runner.LintSource(cfg, real, content, runner.LintWithRoot(root))
	lintMu.Unlock()
	if err != nil {
		return LintProblemsResult{Problems: []LintProblem{}, Unavailable: lintUnavailable(err, root)}, nil
	}
	return lintToProblems(rep, root), nil
}

// lintUnavailable is the one line shown when the text could not be checked.
func lintUnavailable(err error, root string) string {
	return "Not checked: " + oneLine(withoutPaths(err.Error(), root))
}

// lintToProblems turns a report into the rows the window shows, every text made safe (no absolute path, one line, bounded).
func lintToProblems(rep runner.LintReport, root string) LintProblemsResult {
	out := LintProblemsResult{Problems: []LintProblem{}}
	for _, f := range rep.References {
		out.Problems = append(out.Problems, LintProblem{Line: f.Line, Warning: f.Warning, Message: oneLine(withoutPaths(f.Message, root)), Source: "reference"})
	}
	for _, f := range rep.Program {
		out.Problems = append(out.Problems, LintProblem{Line: f.Line, Warning: f.Warning, Message: oneLine(withoutPaths(f.Message, root)), Source: "program"})
	}
	return out
}

// isFilesystemRoot says whether p is the top of a file system: "/" or a drive root such as C:\. An empty path is not one (no workspace is
// a different case: there is nothing to lint inside of).
func isFilesystemRoot(p string) bool {
	if p == "" {
		return false
	}
	clean := filepath.Clean(p)
	return filepath.Dir(clean) == clean
}

// absPath matches a path that starts at the root of the file system; a finding shows no such path, whatever the engine put in it.
var absPath = regexp.MustCompile(`(^|[\s"'(=])((?:[A-Za-z]:)?[/\\][^\s"'<>|]+)`)

// fileURL matches a file:// URL, which is an absolute path in another spelling.
var fileURL = regexp.MustCompile(`(?i)file://(/[^\s"'<>|]*)`)

// withoutPaths is a last guard on text shown in the window: the workspace folder is dropped from it and any other absolute path becomes
// "<path>". The engine's findings name files relative to the workspace and never by a path outside it; this keeps a message from the parser
// or the file system from showing one.
func withoutPaths(s, root string) string {
	if root != "" {
		s = strings.ReplaceAll(s, strings.TrimRight(root, "/")+"/", "")
		if real, err := filepath.EvalSymlinks(root); err == nil {
			s = strings.ReplaceAll(s, strings.TrimRight(real, "/")+"/", "")
		}
	}
	s = fileURL.ReplaceAllString(s, "file://<path>")
	return absPath.ReplaceAllString(s, "${1}<path>")
}

// oneLine makes text from a finding safe to show: one line, no control or invisible characters, a bounded length.
func oneLine(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out = append(out, ' ')
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029':
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
