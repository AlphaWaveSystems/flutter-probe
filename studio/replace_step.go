package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// "Use the text on screen" (docs/proposals/studio-replace-line-step.md): a failed `see "X"` or `tap "X"` that timed out, when the screen showed exactly one
// text that is X written another way (case, spaces, normalisation), gets the same step with that text. The replacement is computed here from the failed
// step and what the agent reported on screen, never from the window; CheckReplaceStep decides, at the moment of the click, whether it may go into the editor.

// maxReplacementRunes is the longest text the snapshot keeps whole (runner.maxVisibleItemLen): a longer item is "cut" and the snapshot is then not proof of anything.
const maxReplacementRunes = 40

// maxSnapshotTexts is the cap the agent has used for its lists; a list that reaches it may have left something out whatever it says (a second guard: the first
// is the agent's own truncated flag, and an agent that does not send it gives no offer).
const maxSnapshotTexts = 25

// foldedText is a text as the replacement rule compares it: normalised (NFC), lower case, edge spaces trimmed and every run of white space one space. It is
// deliberately narrower than the agent's loose fold (which also folds accents, typographic dashes and apostrophes and full-width forms): a narrower fold
// offers less, and an offer that is wrong costs more than one that is missing. The two are not to be aligned.
func foldedText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFC.String(s))), " ")
}

// replacementProblem says why n cannot be the text of a replacement step ("" when it can). n comes from the app under test, so it is distrusted: it must be
// one plain line that a quoted selector holds exactly as it is, and it must not be a glyph.
func replacementProblem(n string) string {
	if n == "" || strings.TrimSpace(n) != n {
		return "the text is empty or has spaces at its edges"
	}
	if utf8.RuneCountInString(n) > maxReplacementRunes {
		return "the text is longer than a snapshot keeps whole"
	}
	for _, r := range n {
		if unicode.Is(unicode.Co, r) || unicode.Is(unicode.Cs, r) || unicode.Is(unicode.Cn, r) || r == 0xFFFC || r == 0xFFFD {
			// an Icon's glyph is a private-use character, which the agent's text matcher matches although the snapshot leaves it out: a text that holds one
			// can match a widget the snapshot does not show. Asked first, so it is its own rule and not only a case of "not plain".
			return "the text has a glyph character (private use, object replacement or unassigned)"
		}
	}
	if t, why := plainSuggestionLine(n); why != "" || t != n {
		return "the text is not one plain line"
	}
	hasWord := false
	for _, r := range n {
		switch {
		case strings.ContainsRune(`"\<>${};#`, r):
			return "the text has a character that a step line would read as something else"
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			hasWord = true
		}
	}
	if !hasWord {
		return "the text has no letter or digit"
	}
	if u, err := strconv.Unquote(`"` + n + `"`); err != nil || u != n {
		return "a quoted selector would not hold the text as it is"
	}
	if sysdialog.ScrubKnown(n) != n {
		return "the text holds a known secret"
	}
	return ""
}

// replacementFor is the text that replaces failed in a see or tap, or "" when the rules give none. matchMode is the run's text matching ("" and "exact" are
// the default: a case-sensitive substring). It gives a text only when everything it needs to be sure of is in the snapshot:
//   - the snapshot is complete: the agent said so with all of its keys, nothing was masked or cut, no list reached its cap;
//   - exactly one visible text is failed written another way, and it is not the typed value of a field;
//   - exactly one visible text, of any list, contains that text as a substring: the new step matches one widget, the one on screen.
func replacementFor(failed string, v runner.VisibleItems, matchMode string) string {
	if matchMode != "" && matchMode != "exact" {
		return "" // a looser match folds more than this rule does: what the new text would match cannot be counted here
	}
	if !v.Detail || v.Truncated || v.Masked || v.Cut > 0 || v.Max < 1 {
		return ""
	}
	for _, list := range [][]string{v.Texts, v.Editable, v.Rich} {
		if len(list) >= v.Max || len(list) >= maxSnapshotTexts {
			return "" // the list may be a cut one
		}
	}
	want := foldedText(failed)
	n := ""
	for _, t := range v.Texts {
		if t != failed && foldedText(t) == want {
			if n != "" && n != t {
				return "" // two texts: which one is meant is not for a rule to say
			}
			n = t
		}
	}
	if n == "" || replacementProblem(n) != "" {
		return ""
	}
	for _, e := range v.Editable {
		if e == n {
			return "" // the typed value of a field: a step would be sent to the field
		}
	}
	holders := 0
	seen := map[string]bool{}
	for _, list := range [][]string{v.Texts, v.Editable, v.Rich} {
		for _, t := range list {
			if !seen[t] && strings.Contains(t, n) {
				seen[t] = true
				holders++
			}
		}
	}
	if holders != 1 {
		return "" // another text holds it too (Sign In and Sign In with Google): the step would match both and act on the first
	}
	return n
}

// ReplaceStepCheck is Studio's answer to "may the on-screen text replace the text of this failed step, now?". When OK, the window replaces line Line (1-based,
// the whole line) with Text, as one edit, and does nothing else.
type ReplaceStepCheck struct {
	OK     bool   `json:"ok"`
	Line   int    `json:"line,omitempty"`
	Text   string `json:"text,omitempty"` // the new line, with the indentation of the one it replaces
	Reason string `json:"reason,omitempty"`
}

// CheckReplaceStep says whether the replacement the rules made for the failed step on line of file may take its place in openFile, whose text the editor holds as
// content. It reads nothing from disk and writes nothing; the window applies the edit. The line is built here from the text the run read, never from the window.
func (a *App) CheckReplaceStep(file string, line int, openFile, content string) ReplaceStepCheck {
	refuse := func(why string) ReplaceStepCheck { return ReplaceStepCheck{Reason: why} }
	if a.track.snapshot().Running { // first, as Insert before does: a run seen only as events must stop an edit in flight too
		return refuse("a run is in progress: wait until it has finished")
	}
	a.mu.Lock()
	ws := a.workspace
	a.mu.Unlock()
	step, ok := a.failedTriageStep(ws, file, line)
	switch {
	case !ok:
		return refuse("this is not a failed step of the last run")
	case step.Depth > 0:
		return refuse("this step runs inside a recipe: change the text there by hand")
	case step.Triage == nil || step.Triage.Replacement == "":
		return refuse("the rules have no replacement for this step")
	}
	n := step.Triage.Replacement
	if why := replacementProblem(n); why != "" {
		return refuse(why)
	}
	m := selectorStep.FindStringSubmatch(step.Description)
	if m == nil || !safeSelector(m[2]) {
		return refuse("the failed step is not a see or tap on a text a quoted selector holds as it is")
	}
	verb, sel := m[1], m[2]
	if n == sel || foldedText(n) != foldedText(sel) {
		return refuse("the replacement is not the failed text written another way")
	}
	if !sameFile(ws, step.File, openFile) {
		return refuse("the open file is not the file the failed step ran from")
	}
	kept, held := a.sources.keptText(ws, step.File)
	if !held {
		return refuse("the text the run read is not held any more (a newer run began): run again")
	}
	runText := normalizedSource(kept)
	if runText != normalizedSource(content) {
		return refuse("the file changed since the run (or the editor holds it with other characters than the run read): save, run again")
	}
	lines := strings.Split(runText, "\n")
	if line < 1 || line > len(lines) {
		return refuse("the file has no line " + fmt.Sprint(line))
	}
	pv, pt, count, err := stepTarget(runText, line)
	if err != nil {
		return refuse("the file does not parse as it is, so a replacement cannot be checked")
	}
	if count != 1 || pv != verb || pt != sel {
		return refuse("line " + fmt.Sprint(line) + " does not hold the step that failed (or holds more than one step)")
	}
	current := strings.TrimRight(lines[line-1], "\r")
	if strings.TrimSpace(current) != verb+` "`+sel+`"` {
		return refuse("the line holds more than the step (a flag or a comment): change the text by hand")
	}
	indent := current[:len(current)-len(strings.TrimLeft(current, " \t"))]
	newLine := indent + verb + ` "` + n + `"`
	if why := bringsNothingNew(current, newLine); why != "" {
		return refuse(why)
	}
	before, err := stepsOf(runText)
	if err != nil {
		return refuse("the file does not parse as it is, so a replacement cannot be checked")
	}
	replaced := append([]string(nil), lines...)
	replaced[line-1] = newLine
	afterText := strings.Join(replaced, "\n")
	after, err := stepsOf(afterText)
	if err != nil {
		return refuse("the file with the new line does not parse")
	}
	if why := sameSteps(before, after, line); why != "" {
		return refuse(why)
	}
	// the profile check above ignores the selector text, so what the parser reads is compared with what was built, exactly
	if av, at, c, err := stepTarget(afterText, line); err != nil || c != 1 || av != verb || at != n {
		return refuse("the parser does not read the new line as the same kind of step with the new text")
	}
	return ReplaceStepCheck{OK: true, Line: line, Text: newLine}
}

// matchModeOf is the text matching of a connection's run: "" when it is the default.
func matchModeOf(conn *connection) string {
	if conn == nil || conn.cfg == nil {
		return ""
	}
	return conn.cfg.Defaults.Match
}

// withReplacement gives a triaged step the replacement the screen supports, if there is one, and takes the wait fix away then.
func withReplacement(t *StepTriage, ev runner.StepEvent, matchMode string) {
	if ev.SelectorIsID || t.Fix == "" { // an id gets no replacement; a step the rules gave no fix for (not "not found") is not one a spelling explains
		return
	}
	m := selectorStep.FindStringSubmatch(ev.Description)
	if m == nil {
		return
	}
	if n := replacementFor(m[2], ev.Visible, matchMode); n != "" {
		t.Replacement = n
		t.Fix = ""
		t.Reason = fmt.Sprintf("%s found no %q on screen after %.1f s; the screen shows %q, the same text written another way.", m[1], m[2], ev.Duration.Seconds(), n)
	}
}
