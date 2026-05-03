package main

import (
	"fmt"
	"strings"
)

// eventToProbeScriptLine converts a single recorded agent event into a
// ProbeScript step string (with leading 2-space indent), or "" to skip.
func eventToProbeScriptLine(params map[string]interface{}) string {
	action, _ := params["action"].(string)
	switch action {
	case "tap":
		return fmt.Sprintf("  tap %s", selectorFromRecordedParams(params))
	case "type":
		text, _ := params["text"].(string)
		sel := selectorFromRecordedParams(params)
		if sel != "the element" {
			return fmt.Sprintf("  type %q into the %s field", text, sel)
		}
		return fmt.Sprintf("  type %q", text)
	case "swipe":
		dir, _ := params["direction"].(string)
		return fmt.Sprintf("  swipe %s", dir)
	case "scroll":
		dir, _ := params["direction"].(string)
		return fmt.Sprintf("  scroll %s", dir)
	case "long_press":
		return fmt.Sprintf("  long press on %s", selectorFromRecordedParams(params))
	case "navigate":
		return "  go back"
	case "see":
		return fmt.Sprintf("  see %s", selectorFromRecordedParams(params))
	default:
		if action != "" {
			return fmt.Sprintf("  # %s", action)
		}
		return ""
	}
}

func selectorFromRecordedParams(params map[string]interface{}) string {
	if sel, ok := params["selector"].(map[string]interface{}); ok {
		kind, _ := sel["kind"].(string)
		text, _ := sel["text"].(string)
		text = sanitizeRecordedText(text)
		switch kind {
		case "id":
			if !strings.HasPrefix(text, "#") {
				return "#" + text
			}
			return text
		case "text":
			if text != "" {
				return fmt.Sprintf("%q", text)
			}
		case "type":
			if text != "" {
				return "the " + text
			}
		}
	}
	if text, ok := params["text"].(string); ok && text != "" {
		return fmt.Sprintf("%q", sanitizeRecordedText(text))
	}
	if id, ok := params["id"].(string); ok && id != "" {
		return "#" + sanitizeRecordedText(id)
	}
	return "the element"
}

func sanitizeRecordedText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}
