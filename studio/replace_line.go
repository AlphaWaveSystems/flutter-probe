package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// Replace the line: a suggested fix can be put into the editor in place of the failed step's line. Studio decides whether it may, here, from
// what it remembered when it asked for the suggestion and from the file as the window holds it now; the window only applies what Studio
// says. Nothing is saved and nothing is run: the edit is one undo step in the editor, and the person saves and runs it as any edit.
//
// It is refused unless every one of these holds, and it says which did not:
//   - the suggestion is a single line of plain characters (no line break, control, hidden, bidirectional or other-width space character)
//     and the cleaning of the answer removed nothing from it;
//   - the open file is the file the failed step ran from, and its line for the step still has the text it had when the suggestion was asked
//     for (so nothing moved, and nothing was typed there since);
//   - the file parses as it is, and parses with the line replaced, into exactly the same steps at exactly the same lines, the step on that
//     line being of the same kind (a `tap` is replaced by a `tap`, never by a second step, a comment, a recipe call, a block or a step of
//     another kind), and not a step that holds others.

const (
	fixResultTTL = 30 * time.Minute
	fixMaxResult = 4
)

// fixResult is a suggestion Studio handed out, with the place it is for. The window gets only the id.
type fixResult struct {
	file, original, suggestion string
	line                       int
	changed                    bool
	created                    time.Time
}

// rememberFix records a suggestion that can be put in the editor, and returns its id ("" when there is no place for it).
func (a *App) rememberFix(p fixStored, suggestion string, changed bool) string {
	if p.line < 1 || p.original == "" {
		return ""
	}
	f := &a.fix
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.results == nil {
		f.results = map[string]fixResult{}
	}
	now := f.clock()
	for id, r := range f.results {
		if now.Sub(r.created) > fixResultTTL {
			delete(f.results, id)
		}
	}
	for len(f.results) >= fixMaxResult {
		oldest := ""
		for id, r := range f.results {
			if oldest == "" || r.created.Before(f.results[oldest].created) {
				oldest = id
			}
		}
		delete(f.results, oldest)
	}
	id := newFixID()
	f.results[id] = fixResult{file: p.file, original: p.original, suggestion: suggestion, line: p.line, changed: changed, created: now}
	return id
}

// failedStepPlace is the run's file and line for the last failed step and the text of that line in content, when the window's file still
// shows that step there: the text, trimmed, is the step's description.
func failedStepPlace(a *App, content string) (file string, line int, original string) {
	step, ok := lastFailedStep(a.track.snapshot())
	if !ok || step.Line < 1 || step.File == "" {
		return "", 0, ""
	}
	lines := strings.Split(content, "\n")
	if step.Line > len(lines) {
		return "", 0, ""
	}
	text := strings.TrimRight(lines[step.Line-1], "\r")
	if strings.TrimSpace(text) == "" || strings.TrimSpace(text) != strings.TrimSpace(step.Description) {
		return "", 0, ""
	}
	return step.File, step.Line, text
}

// ReplaceLineCheck is Studio's answer to "may this suggestion replace that line, now?". When OK, the window replaces line Line (1-based,
// the whole line) with Text, as one edit, and does nothing else.
type ReplaceLineCheck struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"` // when not OK: what did not hold, in a sentence
	Line   int    `json:"line"`
	Text   string `json:"text"` // the new line, with the indentation of the one it replaces
}

// CheckReplaceLine says whether the suggestion with this id may replace the failed step's line in the file the window holds now
// (openFile is its path, content its text). It reads nothing from disk and writes nothing.
func (a *App) CheckReplaceLine(id, openFile, content string) ReplaceLineCheck {
	refuse := func(why string) ReplaceLineCheck { return ReplaceLineCheck{Reason: why} }
	f := &a.fix
	f.mu.Lock()
	r, ok := f.results[id]
	expired := ok && f.clock().Sub(r.created) > fixResultTTL
	f.mu.Unlock()
	switch {
	case !ok || expired:
		return refuse("this suggestion is no longer available: ask for a new one")
	case r.changed:
		return refuse("text was removed from this suggestion, so it is not what the model wrote: edit the line by hand")
	}
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	if !sameFile(ws, r.file, openFile) {
		return refuse("the open file is not the file the failed step ran from")
	}
	text, why := plainSuggestionLine(r.suggestion)
	if why != "" {
		return refuse(why)
	}
	lines := strings.Split(content, "\n")
	if r.line < 1 || r.line > len(lines) {
		return refuse("the file has no line " + fmt.Sprint(r.line) + " now")
	}
	current := strings.TrimRight(lines[r.line-1], "\r")
	if current != r.original {
		return refuse("line " + fmt.Sprint(r.line) + " is not what it was when the suggestion was asked for: ask for a new one")
	}
	indent := current[:len(current)-len(strings.TrimLeft(current, " \t"))]
	newLine := indent + text
	before, err := stepsOf(content)
	if err != nil {
		return refuse("the file does not parse as it is, so a replacement cannot be checked")
	}
	replaced := append([]string(nil), lines...)
	replaced[r.line-1] = newLine + strings.TrimPrefix(lines[r.line-1], current)
	after, err := stepsOf(strings.Join(replaced, "\n"))
	if err != nil {
		return refuse("the suggested line does not parse as a step")
	}
	if why := sameSteps(before, after, r.line); why != "" {
		return refuse(why)
	}
	return ReplaceLineCheck{OK: true, Line: r.line, Text: newLine}
}

// plainSuggestionLine returns the suggestion as one line of plain characters, or why it is not.
func plainSuggestionLine(s string) (string, string) {
	t := strings.TrimSpace(s)
	switch {
	case t == "":
		return "", "the suggestion is empty"
	case utf8.RuneCountInString(t) > fixMaxLine:
		return "", "the suggestion is too long to be one step"
	}
	for _, r := range t {
		switch {
		case r == ' ':
		case hiddenRune(r): // a control, a line or paragraph separator, a bidi control, a joiner, a zero-width character
			return "", "the suggestion has a line break, a control or a hidden character"
		case unicode.IsSpace(r):
			return "", "the suggestion has a space that is not a plain one"
		}
	}
	return t, ""
}

// sameFile says whether two paths name the same file, a relative one being relative to the workspace.
func sameFile(ws, a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	abs := func(p string) string {
		if !filepath.IsAbs(p) && ws != "" {
			p = filepath.Join(ws, p)
		}
		return filepath.Clean(p)
	}
	return abs(a) == abs(b)
}

// stepInfo is one step of a parsed file: its kind and its line. container says it holds other steps.
type stepInfo struct {
	kind      string
	line      int
	container bool
}

var stepIface = reflect.TypeOf((*parser.Step)(nil)).Elem()

// stepsOf parses src and lists every step in it, nested ones too.
func stepsOf(src string) (out []stepInfo, err error) {
	defer func() {
		if recover() != nil {
			out, err = nil, fmt.Errorf("the parser stopped")
		}
	}()
	prog, err := parser.ParseFile(src)
	if err != nil {
		return nil, err
	}
	var walk func(steps []parser.Step)
	walk = func(steps []parser.Step) {
		for _, s := range steps {
			kids := childSteps(s)
			kind := fmt.Sprintf("%T", s)
			out = append(out, stepInfo{kind: kind, line: s.GetLine(), container: len(kids) > 0 && !strings.HasSuffix(kind, "DeviceStep")})
			walk(kids)
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
	return out, nil
}

// childSteps are the steps a step holds, found by type, so a new kind of block is seen without being named here.
func childSteps(s parser.Step) []parser.Step {
	v := reflect.ValueOf(s)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	var kids []parser.Step
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		switch {
		case f.Kind() == reflect.Slice && f.Type().Elem() == stepIface:
			for j := 0; j < f.Len(); j++ {
				if st, ok := f.Index(j).Interface().(parser.Step); ok {
					kids = append(kids, st)
				}
			}
		case f.Type() == stepIface && !f.IsNil():
			if st, ok := f.Interface().(parser.Step); ok {
				kids = append(kids, st)
			}
		}
	}
	return kids
}

// sameSteps says why after is not before with only the step on line replaced by a step of its own kind ("" when it is).
func sameSteps(before, after []stepInfo, line int) string {
	key := func(s []stepInfo) map[int][]string {
		m := map[int][]string{}
		for _, st := range s {
			m[st.line] = append(m[st.line], st.kind)
		}
		return m
	}
	b, a := key(before), key(after)
	if len(b[line]) == 0 {
		return "that line is not a step"
	}
	for _, st := range before {
		if st.line == line && st.container {
			return "that step holds other steps, so it is not replaced from a suggestion"
		}
	}
	if len(before) != len(after) {
		return "the suggestion is not exactly one step"
	}
	if !reflect.DeepEqual(b, a) {
		return "the suggestion is not a step of the same kind as the one it replaces"
	}
	return ""
}
