// Package scriptgen turns the events the agent records into ProbeScript steps. It is the one mapping behind `probe record` and Studio's
// recorder, so a step the two show for the same event is the same step. It is pure: no I/O, no clock.
//
// A recorded event is the params of the agent's notification: an "action" (tap, type, swipe, scroll, long_press, navigate, see), a nested
// "selector" ({kind: id|text|type, text: ...}) or flat "text" / "id", a "direction" for swipe and scroll, and "obscured" for a field whose
// text is hidden.
package scriptgen

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// MaxNameRunes is the longest an unknown action's name, or a swipe or scroll direction, may be in a recorded step: longer is cut to this many runes, the last of them an
// ellipsis that marks the cut.
const MaxNameRunes = 48

// GapPause is the idle time between two events above which a recording gets a wait step.
const GapPause = 2 * time.Second

// Step is the ProbeScript line (with its two-space indent, no newline) for one recorded event, or "" when the event has no action.
//
// An unknown action becomes a comment that names only the action, sanitized: no other value of the event is ever written into a comment. The
// typed text of an obscured field is never written anywhere (see Obscured).
func Step(params map[string]interface{}) string {
	line := stepLine(params)
	if line == "" || strings.HasPrefix(line, "  #") || readsAsOneStep(line) {
		return line
	}
	// The line is built from rules about what the lexer reads (see Selector), and a word of the grammar in a name can still make it read as more
	// than one step (found by FuzzStep: a type named SAve). So the line is read back with the real parser, and a line that is not one step is not
	// written.
	action, _ := params["action"].(string)
	return fmt.Sprintf("  # %s was not recorded: it could not be written as one step. Add the step by hand.", action)
}

// readsAsOneStep says whether the real parser reads the line as exactly one step.
func readsAsOneStep(line string) bool {
	prog, err := parser.ParseFile("test \"t\"\n" + line + "\n")
	return err == nil && len(prog.Tests) == 1 && len(prog.Tests[0].Body) == 1
}

func stepLine(params map[string]interface{}) string {
	params = Scrub(params)
	action, _ := params["action"].(string)
	switch action {
	case "tap":
		return fmt.Sprintf("  tap %s", Selector(params))
	case "type":
		sel := TypeTarget(params)
		if Obscured(params) {
			// The value of an obscured field is never recorded, so no type step is made for it. The comment names the lookup and tells the
			// person what to add by hand.
			return fmt.Sprintf("  # type into %s was not recorded: the value of an obscured field is not saved. Add type \"${VAR}\" into %s by hand.", sel, sel)
		}
		text, _ := params["text"].(string)
		quoted, ok := Quote(text)
		if !ok {
			// The text has a character a script line cannot hold (a control character, a line separator): no step is made for it, and the
			// comment says what to do. The text itself is not in the comment.
			if sel != "the element" {
				return fmt.Sprintf("  # type into %s was not recorded: the text has a character a script cannot hold. Add the step by hand.", sel)
			}
			return "  # type was not recorded: the text has a character a script cannot hold. Add the step by hand."
		}
		// `into the <lookup> field` takes a quoted text or a #id. A widget type (`the TextField`) is not a lookup a type step can name, so
		// the text is typed into the focused field, as when the event named nothing.
		if sel != "the element" && !strings.HasPrefix(sel, "the ") {
			return fmt.Sprintf("  type %s into the %s field", quoted, sel)
		}
		return "  type " + quoted
	case "swipe", "scroll":
		dir, _ := params["direction"].(string)
		if d, ok := direction(dir); ok {
			return fmt.Sprintf("  %s %s", action, d)
		}
		// Anything else is not a direction, and a line that carried it could say more than the event did (a word with a space or a quote in it
		// is more steps on the same line). The comment names the action only.
		return fmt.Sprintf("  # %s was not recorded: its direction is not up, down, left or right. Add the step by hand.", action)
	case "long_press":
		return fmt.Sprintf("  long press on %s", Selector(params))
	case "navigate":
		return "  go back"
	case "see":
		return fmt.Sprintf("  see %s", Selector(params))
	default:
		if name := cleanName(action); name != "" {
			return "  # " + name
		}
		return ""
	}
}

// Scrub is the event as it may be shown: for an obscured event the flat text is dropped, for every action, so it can never become a selector
// (`tap "<value>"`) any more than a typed value. Anything that prints or records an event takes it through Scrub first (Step does).
func Scrub(params map[string]interface{}) map[string]interface{} {
	if Obscured(params) {
		return withoutText(params)
	}
	return params
}

// GapStep is the wait step for an idle gap between two events: "  wait N seconds" (whole seconds) for a gap longer than GapPause, "" for a
// gap of GapPause or less, and for a negative one.
func GapStep(gap time.Duration) string {
	if gap <= GapPause {
		return ""
	}
	return fmt.Sprintf("  wait %d seconds", int64(gap/time.Second))
}

// Obscured says whether a recorded type event came from an obscured field. The agent sends no text for it, and whatever text an event carries
// with this flag is never written.
func Obscured(params map[string]interface{}) bool {
	obscured, _ := params["obscured"].(bool)
	return obscured
}

// TypeTarget is the selector of the field a type event typed into: Selector, but never from the typed text. Without this an event that names
// no target would produce `type "hi" into the "hi" field`, and the text of an obscured field would be written as its selector.
func TypeTarget(params map[string]interface{}) string {
	return Selector(withoutText(params))
}

// withoutText is the event without its flat "text" (the nested selector's own text is a lookup, not a value, and stays).
func withoutText(params map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		if k != "text" {
			out[k] = v
		}
	}
	return out
}

// Selector is the best selector for an event: the agent's nested selector when it names something, else the flat "text" or "id", else "the
// element". A nested selector without text is not a selector.
func Selector(params map[string]interface{}) string {
	if sel, ok := params["selector"].(map[string]interface{}); ok {
		kind, _ := sel["kind"].(string)
		text, _ := sel["text"].(string)
		text = Sanitize(text)
		if text != "" {
			switch kind {
			case "id":
				if idToken(strings.TrimPrefix(text, "#")) {
					return "#" + strings.TrimPrefix(text, "#")
				}
			case "text":
				if quoted, ok := Quote(text); ok {
					return quoted
				}
			case "type":
				if typeName(text) {
					return "the " + text
				}
			}
		}
	}
	if text, ok := params["text"].(string); ok {
		if clean := Sanitize(text); clean != "" {
			if quoted, ok := Quote(clean); ok {
				return quoted
			}
		}
	}
	if id, ok := params["id"].(string); ok {
		if clean := Sanitize(id); clean != "" && idToken(clean) {
			return "#" + clean
		}
	}
	return "the element"
}

// direction is the direction of a swipe or scroll as the script says it: up, down, left or right (the agent's word, trimmed, in any case), or
// not a direction.
func direction(s string) (string, bool) {
	switch d := strings.ToLower(strings.TrimSpace(s)); d {
	case "up", "down", "left", "right":
		return d, true
	}
	return "", false
}

// idToken says whether an id can follow a # in a line: the lexer reads a #id as a letter or underscore and then letters, digits and underscores,
// and stops at anything else (a hyphen, a dot, a space, a quote), so anything else would leave the rest of the id as more words on the line.
func idToken(s string) bool {
	for i, r := range s {
		if !unicode.IsLetter(r) && r != '_' && !(i > 0 && unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}

// typeName says whether a widget type can follow "the" in a line: a class name (a capital or underscore, then letters, digits or underscores; a dollar sign is the lexer's variable mark).
func typeName(s string) bool {
	for i, r := range s {
		if i == 0 && !unicode.IsUpper(r) && r != '_' {
			return false // a class name starts with a capital: a lower-case word such as `field` is a word of the grammar, and `tap the field` is two steps
		}
		if !unicode.IsLetter(r) && r != '_' && !(i > 0 && unicode.IsDigit(r)) {
			return false // a name holds letters, digits and underscores: "the 0" is an ordinal, not a type
		}
	}
	return s != ""
}

// holdable says whether text can be written into a script line as it is (an id, a widget type): valid UTF-8 and no control character or line
// separator. A lookup that is not holdable is not written; Selector falls back to the next one.
func holdable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unholdable(r) {
			return false
		}
	}
	return true
}

// unholdable is a character a script line cannot hold: a control character (the escapes for tab and newline are Quote's), or a line or
// paragraph separator.
func unholdable(r rune) bool { return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' }

// cleanName makes a word the agent sent that is not text from the screen (an unknown action, a direction) safe for a line: the engine's one-line
// cleaning (line and paragraph separators, control, invisible and bidirectional characters become one space, runs of space collapse, trimmed) and
// at most MaxNameRunes runes (a cut ends with an ellipsis).
func cleanName(s string) string { return device.CleanOneLine(s, MaxNameRunes) }

// Quote writes s as a ProbeScript string literal, or reports false when a script line cannot hold it. The lexer reads \" \\ \n and \t as escapes
// (and any other backslash pair as the second character alone), and everything else literally, so only those four are escaped: a no-break space, a
// soft hyphen, a zero-width or bidirectional character, an emoji are written as they are and match at replay. (Go's %q wrote \u00a0, \x00 and
// \r, which the lexer reads as "u00a0", "x00" and "r", so a label with a no-break space never matched.) What cannot be held is a control character
// other than tab and newline, a line or paragraph separator, and text that is not valid UTF-8.
func Quote(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case unholdable(r):
			return "", false
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), true
}

// Sanitize makes text from the screen safe for a ProbeScript line: a line break (LF or CRLF) becomes a space, and spaces, tabs and carriage
// returns around the text are trimmed. Nothing else changes, so a no-break space at an edge is kept, and a carriage return inside the text stays
// and makes the lookup unholdable (it is refused downstream, not deleted).
func Sanitize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Trim(s, " \t\r")
}

// placeholderShape matches what the runner expands when a step runs: ${NAME} (the environment) and <name> or <random.name> (a variable).
var placeholderShape = regexp.MustCompile(`\$\{|<[A-Za-z_][A-Za-z0-9_.]*>`)

// PlaceholderNote is the comment line that follows a step whose text has a placeholder the runner would expand at replay.
const PlaceholderNote = "  # the text above contains a placeholder (${...} or <...>): the runner expands it, so the step may not mean what was recorded"

// Lines is Step plus what a reader of the script should know about it: a step whose text has a ${...} or <name> placeholder is followed by a
// comment saying the runner expands it at replay (there is no escape for it). The comment never carries the text.
func Lines(params map[string]interface{}) []string {
	line := Step(params)
	if line == "" {
		return nil
	}
	if !strings.HasPrefix(line, "  #") && placeholderShape.MatchString(line) {
		return []string{line, PlaceholderNote}
	}
	return []string{line}
}
