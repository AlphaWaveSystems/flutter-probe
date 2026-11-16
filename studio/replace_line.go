package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
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
//   - the file parses as it is, and parses with the line replaced, into exactly the same steps at exactly the same lines, and the step on
//     that line keeps its profile (see fixProfile): the same verb, the same flags (`optional`, `with ai`, ...) and the same shape, from a
//     closed list of what a selector fix may be (tap, long press, double tap, type into, see, don't see, wait until appears or
//     disappears). A step that is not on that list is never replaced and never produced, and neither is one that holds others;
//   - the suggestion brings nothing the line did not have: no comment, no `;`, no `${ENV}` or `<variable>` the original did not hold.

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
	if why := bringsNothingNew(current, text); why != "" {
		return refuse(why)
	}
	indent := current[:len(current)-len(strings.TrimLeft(current, " \t"))]
	newLine := indent + text
	before, err := stepsOf(content)
	if err != nil {
		return refuse("the file does not parse as it is, so a replacement cannot be checked")
	}
	after, err := stepsOf(withLineReplaced(lines, r.line, current, newLine))
	if err != nil {
		return refuse("the suggested line does not parse as a step")
	}
	if why := sameSteps(before, after, r.line); why != "" {
		return refuse(why)
	}
	return ReplaceLineCheck{OK: true, Line: r.line, Text: newLine}
}

// withLineReplaced is the file with line n (1-based) replaced, the line ending of that line kept (a CRLF file stays a CRLF file).
func withLineReplaced(lines []string, n int, current, newLine string) string {
	replaced := append([]string(nil), lines...)
	replaced[n-1] = newLine + strings.TrimPrefix(lines[n-1], current)
	return strings.Join(replaced, "\n")
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
	profile   string // see fixProfile
	allowed   bool   // a selector fix may be, and may replace, a step of this profile
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
			profile, allowed := fixProfile(s)
			out = append(out, stepInfo{kind: kind, line: s.GetLine(), container: len(kids) > 0, profile: profile, allowed: allowed})
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

// sameSteps says why after is not before with only the step on line replaced by a step of the same profile ("" when it is).
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
	if len(b[line]) != 1 {
		return "that line holds more than one step (a step under a device in a composite test is wrapped in another), so it is not replaced from a suggestion"
	}
	var old, now stepInfo
	for _, st := range before {
		if st.line == line {
			old = st
		}
	}
	if old.container {
		return "that step holds other steps, so it is not replaced from a suggestion"
	}
	if !old.allowed {
		return "that kind of step is not replaced from a suggestion (only tap, long press, double tap, type into, see, don't see and wait until appears or disappears are)"
	}
	if len(before) != len(after) {
		return "the suggestion is not exactly one step"
	}
	if !reflect.DeepEqual(b, a) {
		return "the suggestion is not a step of the same kind as the one it replaces"
	}
	for _, st := range after {
		if st.line == line {
			now = st
		}
	}
	if now.profile != old.profile { // a step that is not allowed has no profile
		return "the suggestion changes what the step does (its verb or a flag such as optional), not only what it looks for"
	}
	return ""
}

// fixVerbs are the actions a selector fix may be, and may replace. Every action verb the parser has is classified in the test
// (TestEveryParserVerbIsClassified): a new verb is refused until it is added here on purpose.
var fixVerbs = map[parser.ActionVerb]bool{
	parser.VerbTap: true, parser.VerbLongPress: true, parser.VerbDoubleTap: true, parser.VerbType: true,
}

// fixWaits are the waits a selector fix may be: the ones that look for something on the screen.
var fixWaits = map[parser.WaitKind]bool{parser.WaitAppears: true, parser.WaitDisappears: true, parser.WaitAny: true, parser.WaitSelector: true}

// fixProfile describes a step by what it does, not by what it looks for: its verb and every flag and shape that changes its effect, and
// not the selector, the text or the label a selector fix is there to change. Two steps with the same profile do the same thing to a
// different target. allowed is false for a step that is not on the closed list: any other action (open link, clear app data, kill,
// restart, store, native and webview actions, add media ...), `see ... with ai`, a system dialog step, a block, a recipe call, a call, a
// mock, a dart block, and everything the parser may add.
func fixProfile(s parser.Step) (string, bool) {
	switch st := s.(type) {
	case parser.ActionStep:
		if !fixVerbs[st.Verb] || st.WebView != nil || st.DeepLink || st.To != nil || st.Until != nil || st.Direction != "" || st.DirectionWord != "" || st.Percent != 0 || st.Name != "" {
			return "", false
		}
		// The target's shape (a step with a target, or without) is part of the profile, and so is the text a `type` types: only the
		// selector may change, never what is typed, and `type "bob" into "Email"` is neither `type "bob"` nor `type "pw" into "Email"`.
		return fmt.Sprintf("action:%s:optional=%t:ifvisible=%t:target=%t:typed=%q", st.Verb, st.Optional, st.IfVisible, st.Sel != nil, st.Text), true
	case parser.AssertStep:
		if st.WithAI || st.Native || st.WebView != nil {
			return "", false
		}
		return fmt.Sprintf("assert:negated=%t:count=%t:check=%d:checkval=%q:pattern=%t:optional=%t:any=%t",
			st.Negated, st.Count != 0, st.Check, st.CheckVal, st.Pattern != "", st.Optional, len(st.Any) > 0), true
	case parser.WaitStep:
		if !fixWaits[st.Kind] || st.WebView != nil || st.Duration != 0 {
			return "", false
		}
		return fmt.Sprintf("wait:%d:pattern=%t:any=%t", st.Kind, st.Pattern != "", len(st.Any) > 0), true
	}
	return "", false
}

// interpolations are the forms the runner fills in when a step runs: ${ENV} and <variable> or <generator(...)>.
var interpolations = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}|<[A-Za-z_][A-Za-z0-9_.\-]*(?:\([^)]*\))?>`)

// bringsNothingNew says why the suggestion adds something the line did not have, or "": a comment, a `;`, a ${ENV} or <variable> form.
func bringsNothingNew(original, suggestion string) string {
	if hasComment(suggestion) {
		return "the suggestion has a comment"
	}
	if hasComment(original) {
		return "the line has a comment, which a replacement would drop: edit it by hand"
	}
	if strings.Contains(suggestion, ";") && !strings.Contains(original, ";") {
		return "the suggestion has a ; the line did not have"
	}
	have := map[string]int{}
	for _, f := range interpolations.FindAllString(original, -1) {
		have[f]++
	}
	for _, f := range interpolations.FindAllString(suggestion, -1) {
		if have[f] == 0 {
			return "the suggestion has " + f + ", which the runner fills in when the step runs and the line did not have"
		}
		have[f]--
	}
	return ""
}

// hasComment says whether the line has a comment: a # outside double quotes that is followed by a space or the end of the line (a `#key`
// selector is not one).
func hasComment(line string) bool {
	inString := false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\\' && inString:
			i++
		case c == '"':
			inString = !inString
		case c == '#' && !inString && (i+1 == len(line) || line[i+1] == ' ' || line[i+1] == '\t'):
			return true
		}
	}
	return false
}
