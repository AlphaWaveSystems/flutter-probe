// Package scriptgen turns the events the agent records into ProbeScript steps. It is the one mapping behind `probe record` and Studio's
// recorder, so a step the two show for the same event is the same step. It is pure: no I/O, no clock.
//
// A recorded event is the params of the agent's notification: an "action" (tap, type, swipe, scroll, long_press, navigate, see), a nested
// "selector" ({kind: id|text|type, text: ...}) or flat "text" / "id", a "direction" for swipe and scroll, and "obscured" for a field whose
// text is hidden.
package scriptgen

import (
	"fmt"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
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
		if sel != "the element" {
			return fmt.Sprintf("  type %q into the %s field", text, sel)
		}
		return fmt.Sprintf("  type %q", text)
	case "swipe":
		dir, _ := params["direction"].(string)
		return fmt.Sprintf("  swipe %s", cleanName(dir))
	case "scroll":
		dir, _ := params["direction"].(string)
		return fmt.Sprintf("  scroll %s", cleanName(dir))
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
				if strings.HasPrefix(text, "#") {
					return text
				}
				return "#" + text
			case "text":
				return fmt.Sprintf("%q", text)
			case "type":
				return "the " + text
			}
		}
	}
	if text, ok := params["text"].(string); ok && text != "" {
		return fmt.Sprintf("%q", Sanitize(text))
	}
	if id, ok := params["id"].(string); ok && id != "" {
		return "#" + Sanitize(id)
	}
	return "the element"
}

// cleanName makes a word the agent sent that is not text from the screen (an unknown action, a direction) safe for a line: the engine's one-line
// cleaning (line and paragraph separators, control, invisible and bidirectional characters become one space, runs of space collapse, trimmed) and
// at most MaxNameRunes runes (a cut ends with an ellipsis).
func cleanName(s string) string { return device.CleanOneLine(s, MaxNameRunes) }

// Sanitize makes text from the screen safe for a ProbeScript line: no line breaks (they would end the step), trimmed.
func Sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}
