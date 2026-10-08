package runner

import (
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// isNotFoundError reports whether err says the step's target was not on screen
// (as opposed to being on screen in the wrong state, or a connection problem).
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Widget not found") ||
		(strings.Contains(msg, "Expected to see") && strings.Contains(msg, "was not found"))
}

// implicitWaitable reports whether a step should be retried while its target is
// missing: actions on a selector and a plain (non-negated, non-native) `see`.
// Steps that already tolerate absence (`if visible`, `optional`) and explicit waits
// are left alone.
func implicitWaitable(step parser.Step) bool {
	switch s := step.(type) {
	case parser.ActionStep:
		if s.Sel == nil || s.IfVisible || s.Optional {
			return false
		}
		switch s.Verb {
		case parser.VerbTap, parser.VerbType, parser.VerbLongPress, parser.VerbDoubleTap, parser.VerbClear, parser.VerbDrag:
			return true
		}
	case parser.AssertStep:
		return !s.Negated && !s.Native && !s.Optional && s.Count == 0
	}
	return false
}
