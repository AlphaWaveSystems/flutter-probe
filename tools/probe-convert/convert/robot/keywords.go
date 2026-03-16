package robot

import (
	"fmt"
	"regexp"
	"strings"
)

// mapKeyword converts a Robot Framework keyword call to a ProbeScript step.
// Returns the step line and whether it was recognized.
func mapKeyword(keyword string, args []string) (string, bool) {
	kw := strings.ToLower(strings.TrimSpace(keyword))

	switch kw {
	case "click element":
		if len(args) > 0 {
			return fmt.Sprintf("tap on %s", parseLocator(args[0])), true
		}
	case "click text":
		if len(args) > 0 {
			return fmt.Sprintf("tap on %q", args[0]), true
		}
	case "click button":
		if len(args) > 0 {
			return fmt.Sprintf("tap on %q", args[0]), true
		}
	case "input text":
		if len(args) >= 2 {
			return fmt.Sprintf("type %q into %s", args[1], parseLocator(args[0])), true
		}
	case "input value":
		if len(args) >= 2 {
			return fmt.Sprintf("type %q into %s", args[1], parseLocator(args[0])), true
		}
	case "clear text":
		if len(args) > 0 {
			return fmt.Sprintf("clear %s", parseLocator(args[0])), true
		}
	case "page should contain text":
		if len(args) > 0 {
			return fmt.Sprintf("see %q", args[0]), true
		}
	case "page should contain element":
		if len(args) > 0 {
			return fmt.Sprintf("see %s", parseLocator(args[0])), true
		}
	case "page should not contain text":
		if len(args) > 0 {
			return fmt.Sprintf("don't see %q", args[0]), true
		}
	case "page should not contain element":
		if len(args) > 0 {
			return fmt.Sprintf("don't see %s", parseLocator(args[0])), true
		}
	case "element should be visible":
		if len(args) > 0 {
			return fmt.Sprintf("see %s", parseLocator(args[0])), true
		}
	case "element should not be visible":
		if len(args) > 0 {
			return fmt.Sprintf("don't see %s", parseLocator(args[0])), true
		}
	case "wait until element is visible":
		if len(args) > 0 {
			return fmt.Sprintf("wait until %s appears", parseLocator(args[0])), true
		}
	case "wait until page contains":
		if len(args) > 0 {
			return fmt.Sprintf("wait until %q appears", args[0]), true
		}
	case "wait until page contains element":
		if len(args) > 0 {
			return fmt.Sprintf("wait until %s appears", parseLocator(args[0])), true
		}
	case "capture page screenshot":
		return "take a screenshot", true
	case "sleep":
		if len(args) > 0 {
			return fmt.Sprintf("wait %s", parseDuration(args[0])), true
		}
	case "go back":
		return "go back", true
	case "swipe":
		if len(args) >= 5 {
			return fmt.Sprintf("swipe %s", guessDirection(args)), true
		}
		return "swipe down", true
	case "scroll down":
		return "scroll down", true
	case "scroll up":
		return "scroll up", true
	case "open application", "launch application":
		return "open the app", true
	case "close application":
		return "close the app", true
	case "reset application":
		return "clear app data", true
	case "hide keyboard":
		return "close keyboard", true
	case "long press":
		if len(args) > 0 {
			return fmt.Sprintf("long press on %s", parseLocator(args[0])), true
		}
	case "tap":
		if len(args) > 0 {
			return fmt.Sprintf("tap on %s", parseLocator(args[0])), true
		}
	case "log":
		if len(args) > 0 {
			return fmt.Sprintf("# log: %s", args[0]), true
		}
	}
	return "", false
}

var xpathTextRe = regexp.MustCompile(`@(?:text|content-desc|label)=["']([^"']+)["']`)

// parseLocator converts a Robot Framework locator to ProbeScript selector.
func parseLocator(loc string) string {
	loc = strings.TrimSpace(loc)

	switch {
	case strings.HasPrefix(loc, "id="):
		return "#" + strings.TrimPrefix(loc, "id=")
	case strings.HasPrefix(loc, "text="):
		return fmt.Sprintf("%q", strings.TrimPrefix(loc, "text="))
	case strings.HasPrefix(loc, "accessibility_id="):
		return fmt.Sprintf("%q", strings.TrimPrefix(loc, "accessibility_id="))
	case strings.HasPrefix(loc, "xpath="):
		if m := xpathTextRe.FindStringSubmatch(loc); m != nil {
			return fmt.Sprintf("%q", m[1])
		}
		return fmt.Sprintf("# xpath: %s", loc)
	default:
		// If it looks like an ID (no spaces, no quotes), treat as text.
		if !strings.Contains(loc, " ") && !strings.HasPrefix(loc, "\"") {
			return fmt.Sprintf("%q", loc)
		}
		return fmt.Sprintf("%q", loc)
	}
}

// parseDuration converts "5s", "5", "5 seconds", etc. to "N seconds".
func parseDuration(d string) string {
	d = strings.TrimSpace(d)
	d = strings.TrimSuffix(d, "s")
	d = strings.TrimSuffix(d, " second")
	d = strings.TrimSpace(d)
	return d + " seconds"
}

// guessDirection guesses swipe direction from start/end coordinates.
func guessDirection(args []string) string {
	// args: [locator, startX, startY, endX, endY] or [startX, startY, endX, endY]
	// simplified: just return "down" as default
	return "down"
}
