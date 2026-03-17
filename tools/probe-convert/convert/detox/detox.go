// Package detox converts Detox JS/TS test files to ProbeScript.
package detox

import (
	"fmt"
	"strings"

	"github.com/alphawavesystems/probe-convert/convert"
)

// Converter handles Detox → ProbeScript conversion.
type Converter struct{}

// New creates a new Detox converter.
func New() *Converter { return &Converter{} }

func (c *Converter) Format() convert.Format    { return convert.FormatDetox }
func (c *Converter) Extensions() []string       { return []string{".js", ".ts"} }

func (c *Converter) Convert(source []byte, path string) (*convert.Result, error) {
	probe, warnings := convertDetox(string(source), path)
	return &convert.Result{
		InputPath: path,
		ProbeCode: probe,
		Warnings:  warnings,
	}, nil
}

func convertDetox(src, path string) (string, []convert.Warning) {
	w := convert.NewProbeWriter()
	var warnings []convert.Warning

	w.Comment("Converted from Detox")
	w.BlankLine()

	// Pre-process: join continuation lines (lines starting with .).
	lines := strings.Split(src, "\n")
	var joined []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".") && len(joined) > 0 {
			joined[len(joined)-1] += trimmed
		} else {
			joined = append(joined, line)
		}
	}

	inBlock := false
	braceDepth := 0

	for lineNum, rawLine := range joined {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		// Skip imports and requires.
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "const ") ||
			strings.HasPrefix(line, "require(") || strings.HasPrefix(line, "let ") ||
			strings.HasPrefix(line, "var ") || line == "});" || line == "})," ||
			line == "}" || line == "})" || line == "{" || line == "}))" {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			continue
		}

		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

		// describe block.
		if m := describeBlock.FindStringSubmatch(line); m != nil {
			w.Comment(m[1])
			w.BlankLine()
			continue
		}

		// it block → test.
		if m := itBlock.FindStringSubmatch(line); m != nil {
			w.BlankLine()
			w.Test(m[1])
			inBlock = true
			continue
		}

		// beforeAll/beforeEach.
		if beforeBlock.MatchString(line) {
			w.BeforeEach()
			inBlock = true
			continue
		}

		// afterAll/afterEach.
		if afterBlock.MatchString(line) {
			w.AfterEach()
			inBlock = true
			continue
		}

		if !inBlock {
			continue
		}

		// Match Detox API patterns.
		if step := matchDetoxLine(line); step != "" {
			w.Step(step)
			continue
		}

		// Skip common JS noise.
		if isJSNoise(line) {
			continue
		}

		// Unmatched line with test-like API calls.
		if hasTestAPI(line) {
			w.TODO(line)
			warnings = append(warnings, convert.Warning{
				Line:     lineNum + 1,
				Severity: convert.Warn,
				Message:  fmt.Sprintf("unrecognized Detox API call: %s", truncate(line, 80)),
			})
		}
	}

	return w.String(), warnings
}

func matchDetoxLine(line string) string {
	// Device operations.
	if deviceLaunch.MatchString(line) {
		return "open the app"
	}
	if deviceReload.MatchString(line) {
		return "restart the app"
	}
	if m := deviceScreenshot.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("take a screenshot called %q", m[1])
	}

	// Wait operations (check before expect to avoid false matches).
	if m := waitForTextVisible.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait until %q appears", m[1])
	}
	if m := waitForIDVisible.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait until #%s appears", m[1])
	}

	// Expect operations.
	if m := expectTextNotVisible.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("don't see %q", m[1])
	}
	if m := expectIDNotVisible.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("don't see #%s", m[1])
	}
	if m := expectTextVisible.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("see %q", m[1])
	}
	if m := expectIDVisible.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("see #%s", m[1])
	}
	if m := expectIDHaveText.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("see %q", m[2])
	}

	// Type operations (before tap to match longer patterns first).
	if m := elemByIDType.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into #%s", m[2], m[1])
	}
	if m := elemByTextType.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into %q", m[2], m[1])
	}
	if m := elemByIDReplace.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into #%s", m[2], m[1])
	}
	if m := elemByIDClear.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("clear #%s", m[1])
	}

	// Long press operations.
	if m := elemByIDLongPress.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("long press on #%s", m[1])
	}
	if m := elemByTextLongPress.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("long press on %q", m[1])
	}

	// Swipe operations.
	if m := elemByIDSwipe.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("swipe %s on #%s", m[2], m[1])
	}

	// Scroll operations.
	if m := elemByIDScroll.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("scroll %s on #%s", m[2], m[1])
	}

	// Tap operations.
	if m := elemByIDTap.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("tap on #%s", m[1])
	}
	if m := elemByTextTap.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("tap on %q", m[1])
	}
	if m := elemByLabelTap.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("tap on %q", m[1])
	}

	return ""
}

func isJSNoise(line string) bool {
	noise := []string{
		"await", "async", "//", "/*", "*/", "try {", "catch", "finally",
		"console.log", "console.warn", "console.error",
	}
	trimmed := strings.TrimSpace(line)
	for _, n := range noise {
		if strings.HasPrefix(trimmed, n) {
			return true
		}
	}
	// Pure await with semicolon is often wrapping an API call we already matched.
	if strings.HasPrefix(trimmed, "await ") && !strings.Contains(trimmed, "element(") &&
		!strings.Contains(trimmed, "expect(") && !strings.Contains(trimmed, "waitFor(") &&
		!strings.Contains(trimmed, "device.") {
		return true
	}
	return false
}

func hasTestAPI(line string) bool {
	apis := []string{"element(", "expect(", "waitFor(", "device.", ".tap()", ".typeText(",
		".toBeVisible()", ".toExist()", ".swipe(", ".scroll("}
	for _, a := range apis {
		if strings.Contains(line, a) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
