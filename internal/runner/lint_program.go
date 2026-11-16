package runner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// hookBodyFate says what the parser does with the steps under a hook header that has ignored words. A test keyword among the words starts a
// test with no name, so the steps run as a test. Any other words are skipped with the steps, so the steps are not run at all. Either way
// they are not the hook's.
func hookBodyFate(h parser.HookDef) string {
	if h.ExtraRunsAsTest {
		return "the steps under this header are NOT part of the hook: they run as a test"
	}
	return "the steps under this header are NOT part of the hook, and they do not run"
}

// lintProgram runs static checks on a parsed program and returns warnings. It needs only the parse, so it is the part of
// probe lint that reads no file but the one it was given. Each finding carries the line the parser has for it: the test's line
// for a test with no steps and for a dart share, the step's line for a count check.
func lintProgram(prog *parser.Program) []LintFinding {
	var warns []LintFinding

	// Check: each test has at least one step
	for _, t := range prog.Tests {
		if len(t.Body) == 0 {
			warns = append(warns, LintFinding{Line: t.Line, Warning: true, Message: fmt.Sprintf("test %q has no steps", t.Name)})
		}
		// Check: dart usage > 20% of steps
		dartCount := countDartBlocks(t.Body)
		total := len(t.Body)
		if total > 0 && dartCount*100/total > 20 {
			warns = append(warns, LintFinding{Line: t.Line, Warning: true, Message: fmt.Sprintf(
				"test %q: %.0f%% dart blocks (consider ProbeScript instead)",
				t.Name, float64(dartCount*100)/float64(total),
			)})
		}
	}

	// Check: a word after a hook header is ignored by the parser. The indented body under such a header is not the hook's either (the parser
	// drops it, or reads it as a test); the warning says which.
	for _, h := range prog.Hooks {
		if len(h.Extra) > 0 {
			warns = append(warns, LintFinding{Line: h.Line, Warning: true, Message: fmt.Sprintf(
				"unexpected word(s) after the hook header are ignored: %s; %s", strings.Join(h.Extra, " "), hookBodyFate(h))})
		}
	}

	// Check: recipes referenced by `use` that don't exist
	// (static check — file existence not verified here)

	// Check: a count or zero-state text check that a substring match passes by accident
	for _, t := range prog.Tests {
		warns = append(warns, substringCountWarnings(fmt.Sprintf("test %q", t.Name), t.Body)...)
	}
	for _, h := range prog.Hooks {
		warns = append(warns, substringCountWarnings(fmt.Sprintf("%s hook", h.Kind), h.Body)...)
	}
	for _, r := range prog.Recipes {
		warns = append(warns, substringCountWarnings(fmt.Sprintf("recipe %q", r.Name), r.Body)...)
	}

	return warns
}

// countOrZeroText matches a bare count or a count with one word ("0", "3 items"), and zeroStateText a zero state
// ("None", "No items", "Nothing here"). Longer sentences and decimals are not matched, to keep the warning conservative.
var (
	countOrZeroText = regexp.MustCompile(`(?i)^[0-9]+( [a-z]+)?$`)
	zeroStateText   = regexp.MustCompile(`(?i)^(no|none|nothing|empty|zero)( [a-z]+){0,2}$`)
)

// substringCountWarnings flags a `see` or `wait until ... appears` on a count or zero-state text unless its pattern is
// anchored at both ends. A substring match passes "0 items" on "10 items", so the check should be anchored. A negated
// check is not flagged, because its substring match fails loudly rather than passing by accident. A pattern anchored at
// both ends already compares the whole text; any other pattern is still a substring match, so it is flagged.
func substringCountWarnings(owner string, steps []parser.Step) []LintFinding {
	var warns []LintFinding
	for _, s := range steps {
		switch st := s.(type) {
		case parser.AssertStep:
			if st.WebView != nil {
				if !st.Negated && st.WebView.Kind == parser.WebViewText {
					warns = appendWebViewCountWarning(warns, owner, st.Line, st.WebView.Value)
				}
				continue
			}
			if st.Negated || st.Native || st.WithAI || anchoredPattern(st.Pattern) || len(st.Any) > 0 || st.Sel.Kind != parser.SelectorText {
				continue
			}
			warns = appendSubstringCountWarning(warns, owner, st.Line, st.Sel.Text)
		case parser.WaitStep:
			if st.Kind == parser.WaitWebView && st.WebView.Kind == parser.WebViewText {
				warns = appendWebViewCountWarning(warns, owner, st.Line, st.WebView.Value)
			}
			if st.Kind == parser.WaitAppears && !anchoredPattern(st.Pattern) {
				warns = appendSubstringCountWarning(warns, owner, st.Line, st.Target)
			}
		case parser.ConditionalStep:
			warns = append(warns, substringCountWarnings(owner, st.Then)...)
			warns = append(warns, substringCountWarnings(owner, st.Else)...)
		case parser.LoopStep:
			warns = append(warns, substringCountWarnings(owner, st.Body)...)
		case parser.RetryStep:
			warns = append(warns, substringCountWarnings(owner, st.Body)...)
		case parser.TimedStep:
			warns = append(warns, substringCountWarnings(owner, []parser.Step{st.Inner})...)
		}
	}
	return warns
}

// anchoredPattern reports whether a matching pattern is anchored at both ends, so it compares the whole text.
func anchoredPattern(pattern string) bool {
	return strings.HasPrefix(pattern, "^") && strings.HasSuffix(pattern, "$")
}

// appendWebViewCountWarning is appendSubstringCountWarning for a text in a WebView. The text is matched exactly first, then as a part of
// an element's text, and a WebView step has no `matching` form to anchor it, so the warning says what to do instead.
func appendWebViewCountWarning(warns []LintFinding, owner string, line int, text string) []LintFinding {
	text = strings.TrimSpace(text)
	if !countOrZeroText.MatchString(text) && !zeroStateText.MatchString(text) {
		return warns
	}
	return append(warns, LintFinding{Line: line, Warning: true, Message: fmt.Sprintf(
		"%s, line %d: %q in a WebView can match any longer element text that contains it; check the element with css:\"...\" instead of its count text",
		owner, line, text)})
}

func appendSubstringCountWarning(warns []LintFinding, owner string, line int, text string) []LintFinding {
	text = strings.TrimSpace(text)
	if !countOrZeroText.MatchString(text) && !zeroStateText.MatchString(text) {
		return warns
	}
	return append(warns, LintFinding{Line: line, Warning: true, Message: fmt.Sprintf(
		"%s, line %d: %q matches any longer text that contains it; for an exact count or zero-state check add matching \"^%s$\"",
		owner, line, text, text)})
}

func countDartBlocks(steps []parser.Step) int {
	n := 0
	for _, s := range steps {
		if _, ok := s.(parser.DartBlock); ok {
			n++
		}
	}
	return n
}
