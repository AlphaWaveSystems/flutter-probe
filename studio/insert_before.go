package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// "Insert before the step" (docs/proposals/studio-insert-before-step.md): the rule-based fix of a failed see or tap is a line to add before the step.
// CheckInsertBefore says whether it may be added to the text the editor holds. It reads nothing from disk and writes nothing; the window applies the
// edit. The anchor is the file as the run read it: the run reads each file through a hook that keeps the bytes it returned, and an insertion is allowed
// only while the editor holds that same text, so the line number of the failed step is exact.

// maxKeptSource is the largest file whose text is kept for a run.
const maxKeptSource = 1 << 20

// runSources keeps the bytes the runner parsed, by path, for the last run. It is safe for the goroutines of a run.
type runSources struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (s *runSources) reset() {
	s.mu.Lock()
	s.files = nil
	s.mu.Unlock()
}

// readAndKeep is the run's ReadFile hook: it reads the disk and keeps what it returned (a file over maxKeptSource is returned to the run but not kept, so no
// insertion is offered for it).
func (s *runSources) readAndKeep(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > maxKeptSource {
		return b, err
	}
	s.mu.Lock()
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	s.files[filepath.Clean(path)] = append([]byte(nil), b...)
	s.mu.Unlock()
	return b, err
}

// keptText is the kept text of the file, whichever way its path is written (a relative one is relative to the workspace).
func (s *runSources) keptText(ws, path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.files {
		if sameFile(ws, k, path) {
			return string(v), true
		}
	}
	return "", false
}

// InsertBeforeCheck is the answer of CheckInsertBefore: ok with the line to insert before and the text of the new line (with the indentation of the failed
// step's own line, no line ending), or the reason it is refused.
type InsertBeforeCheck struct {
	OK     bool   `json:"ok"`
	Line   int    `json:"line,omitempty"`
	Text   string `json:"text,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// normalizedSource is a text as the line numbers see it: no byte-order mark, every line ending (CRLF, a lone CR) one LF. The editor does not keep a
// file's bytes, so the comparison with the run's text is made on this form; it changes no line number.
func normalizedSource(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// failedTriageStep is the failed step of the last run at file:line that the rules gave a fix for.
func (a *App) failedTriageStep(ws, file string, line int) (RunStep, bool) {
	st := a.track.snapshot()
	for i := len(st.Steps) - 1; i >= 0; i-- {
		s := st.Steps[i]
		if s.Status == "failed" && s.Line == line && sameFile(ws, s.File, file) {
			return s, true
		}
	}
	return RunStep{}, false
}

// stepTarget is the verb and the quoted text of the see or tap on the given line, found in a parsed file: new code, stepsOf keeps only the kind.
func stepTarget(src string, line int) (verb, text string, count int, err error) {
	defer func() {
		if recover() != nil {
			verb, text, count, err = "", "", 0, fmt.Errorf("the parser stopped")
		}
	}()
	prog, err := parser.ParseFile(src)
	if err != nil {
		return "", "", 0, err
	}
	var walk func(steps []parser.Step)
	walk = func(steps []parser.Step) {
		for _, s := range steps {
			if s.GetLine() == line {
				count++
				switch st := s.(type) {
				case parser.ActionStep:
					if st.Verb == parser.VerbTap && st.Sel != nil && st.Sel.Kind == parser.SelectorText {
						verb, text = "tap", st.Sel.Text
					}
				case parser.AssertStep:
					if !st.Negated && st.Sel.Kind == parser.SelectorText {
						verb, text = "see", st.Sel.Text
					}
				}
			}
			walk(childSteps(s))
		}
	}
	for _, t := range prog.Tests {
		walk(t.Body)
	}
	for _, r := range prog.Recipes {
		walk(r.Body)
	}
	for _, h := range prog.Hooks {
		walk(h.Body)
	}
	for _, c := range prog.CompositeTests {
		walk(c.Body)
	}
	return verb, text, count, nil
}

// safeSelector says whether the quoted text of a step description names the selector as it is: no backslash, quote, control, hidden or non-printable
// character. The runner writes a step description with %q, so such a selector would be shown escaped and a wait built from the description would name
// another selector; these get no button.
func safeSelector(sel string) bool {
	if sel == "" {
		return false
	}
	for _, r := range sel {
		if r == '\\' || r == '"' || hiddenRune(r) || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// CheckInsertBefore says whether the rule-based fix of the failed step on line of file may be inserted above it in openFile, whose text the editor holds
// as content.
func (a *App) CheckInsertBefore(file string, line int, openFile, content string) InsertBeforeCheck {
	refuse := func(why string) InsertBeforeCheck { return InsertBeforeCheck{Reason: why} }
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	step, ok := a.failedTriageStep(ws, file, line)
	switch {
	case !ok:
		return refuse("this is not a failed step of the last run")
	case step.Depth > 0:
		return refuse("this step runs inside a recipe: add the wait in the recipe by hand")
	case step.Triage == nil || step.Triage.Fix == "":
		return refuse("the rules have no fix for this step")
	}
	m := selectorStep.FindStringSubmatch(step.Description)
	if m == nil || !safeSelector(m[2]) {
		return refuse("the selector has a character a quoted text cannot hold as it is: add the wait by hand")
	}
	verb, sel := m[1], m[2]
	fix := `wait until "` + sel + `" appears` // made here from the step (the selector is safe: no escape), never taken from the window
	if fix != step.Triage.Fix {
		return refuse("the suggested line is not the one the rules made for this step")
	}
	if !sameFile(ws, step.File, openFile) {
		return refuse("the open file is not the file the failed step ran from")
	}
	kept, held := a.sources.keptText(ws, step.File)
	if !held {
		return refuse("the text the run read is not held any more (a newer run began): run again")
	}
	runText, editorText := normalizedSource(kept), normalizedSource(content)
	if runText != editorText {
		return refuse("the file changed since the run (or the editor holds it with other characters than the run read): save, run again")
	}
	text, why := plainSuggestionLine(fix)
	if why != "" {
		return refuse(why)
	}
	lines := strings.Split(runText, "\n")
	if line < 1 || line > len(lines) {
		return refuse("the file has no line " + fmt.Sprint(line))
	}
	pv, pt, count, err := stepTarget(runText, line)
	if err != nil {
		return refuse("the file does not parse as it is, so an insertion cannot be checked")
	}
	if count != 1 || pv != verb || pt != sel {
		return refuse("line " + fmt.Sprint(line) + " does not hold the step that failed (or holds more than one step)")
	}
	before, err := stepsOf(runText)
	if err != nil {
		return refuse("the file does not parse as it is, so an insertion cannot be checked")
	}
	var at *stepInfo
	for i := range before {
		if before[i].line == line {
			at = &before[i]
		}
	}
	if at == nil || !at.allowed {
		return refuse("the step on this line is not one a fix may be placed before")
	}
	current := strings.TrimRight(lines[line-1], "\r")
	indent := current[:len(current)-len(strings.TrimLeft(current, " \t"))]
	if line >= 2 && strings.TrimSpace(lines[line-2]) == text {
		return refuse("the line above already is this wait")
	}
	newLine := indent + text
	inserted := append(append(append([]string(nil), lines[:line-1]...), newLine), lines[line-1:]...)
	after, err := stepsOf(strings.Join(inserted, "\n"))
	if err != nil {
		return refuse("the file with the new line does not parse")
	}
	if why := sameStepsAfterInsert(before, after, line); why != "" {
		return refuse(why)
	}
	return InsertBeforeCheck{OK: true, Line: line, Text: newLine}
}

// sameStepsAfterInsert says why after is not before with exactly one new wait step at line n and every step from line n on moved down one line ("" when
// it is). sameSteps cannot be used: it compares line by line, and an insertion shifts every following line.
func sameStepsAfterInsert(before, after []stepInfo, n int) string {
	if len(after) != len(before)+1 {
		return "the new line would not be exactly one step"
	}
	type key struct {
		line      int
		kind      string
		container bool
		profile   string
	}
	want := map[key]int{}
	for _, s := range before {
		l := s.line
		if l >= n {
			l++
		}
		want[key{l, s.kind, s.container, s.profile}]++
	}
	extra := 0
	for _, s := range after {
		k := key{s.line, s.kind, s.container, s.profile}
		if want[k] > 0 {
			want[k]--
			continue
		}
		if s.line == n && s.kind == "parser.WaitStep" && s.allowed {
			extra++
			continue
		}
		return "the new line would change another step"
	}
	if extra != 1 {
		return "the new line would not be exactly one wait step"
	}
	return ""
}
