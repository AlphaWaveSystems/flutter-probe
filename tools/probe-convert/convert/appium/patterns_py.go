package appium

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/flutterprobe/probe-convert/convert"
)

// Python Appium patterns.
var (
	pyFindClick     = regexp.MustCompile(`find_element\((?:By\.|AppiumBy\.)?(\w+),\s*['"](.*?)['"]\)\.click\(\)`)
	pyFindSendKeys  = regexp.MustCompile(`find_element\((?:By\.|AppiumBy\.)?(\w+),\s*['"](.*?)['"]\)\.send_keys\(['"](.*?)['"]\)`)
	pyFindClear     = regexp.MustCompile(`find_element\((?:By\.|AppiumBy\.)?(\w+),\s*['"](.*?)['"]\)\.clear\(\)`)
	pyWait          = regexp.MustCompile(`WebDriverWait.*?\.until\(.*?(?:visibility|presence).*?(?:By\.|AppiumBy\.)?(\w+),\s*['"](.*?)['"]`)
	pyAssertDisplay = regexp.MustCompile(`(?:assert|assertTrue).*?(?:is_displayed|isDisplayed)`)
	pySleep         = regexp.MustCompile(`(?:time\.)?sleep\((\d+)\)`)
	pyTestDef       = regexp.MustCompile(`def (test_\w+)\(self\)`)
	pySetUp         = regexp.MustCompile(`def setUp\(self\)`)
	pyTearDown      = regexp.MustCompile(`def tearDown\(self\)`)
	pyClassDef      = regexp.MustCompile(`class (\w+)`)
	pyBack          = regexp.MustCompile(`\.back\(\)`)
	pyScreenshot    = regexp.MustCompile(`(?:get_screenshot|save_screenshot)\(['"](.*?)['"]\)`)
)

func convertPython(src, path string) (string, []convert.Warning) {
	w := convert.NewProbeWriter()
	var warnings []convert.Warning

	w.Comment("Converted from Appium Python")
	w.BlankLine()

	// Pre-process: join continuation lines (lines after opening parens).
	rawLines := strings.Split(src, "\n")
	var lines []string
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if len(lines) > 0 && strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "(") {
			lines[len(lines)-1] += " " + trimmed
		} else if len(lines) > 0 && strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), ",") && strings.HasPrefix(trimmed, "EC.") {
			lines[len(lines)-1] += " " + trimmed
		} else {
			lines = append(lines, line)
		}
	}

	inBlock := false

	for lineNum, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip imports and setup boilerplate.
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
			continue
		}

		// Class definition.
		if m := pyClassDef.FindStringSubmatch(line); m != nil {
			w.Comment(fmt.Sprintf("Test class: %s", m[1]))
			continue
		}

		// setUp → before each.
		if pySetUp.MatchString(line) {
			w.BeforeEach()
			inBlock = true
			continue
		}

		// tearDown → after each.
		if pyTearDown.MatchString(line) {
			w.AfterEach()
			inBlock = true
			continue
		}

		// test method → test.
		if m := pyTestDef.FindStringSubmatch(line); m != nil {
			name := strings.TrimPrefix(m[1], "test_")
			name = strings.ReplaceAll(name, "_", " ")
			w.BlankLine()
			w.Test(name)
			inBlock = true
			continue
		}

		if !inBlock {
			continue
		}

		// Match patterns.
		if step := matchPythonLine(line); step != "" {
			w.Step(step)
			continue
		}

		// Skip common Python noise.
		if isPythonNoise(line) {
			continue
		}

		// Unmatched line with Appium API.
		if hasPythonAPI(line) {
			w.TODO(line)
			warnings = append(warnings, convert.Warning{
				Line:     lineNum + 1,
				Severity: convert.Warn,
				Message:  fmt.Sprintf("unrecognized Appium call: %s", truncate(line, 80)),
			})
		}
	}

	return w.String(), warnings
}

func matchPythonLine(line string) string {
	// find_element(...).click()
	if m := pyFindClick.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("tap on %s", pyLocator(m[1], m[2]))
	}

	// find_element(...).send_keys(...)
	if m := pyFindSendKeys.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into %s", m[3], pyLocator(m[1], m[2]))
	}

	// find_element(...).clear()
	if m := pyFindClear.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("clear %s", pyLocator(m[1], m[2]))
	}

	// WebDriverWait...until(visibility/presence)
	if m := pyWait.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait until %s appears", pyLocator(m[1], m[2]))
	}

	// sleep(N)
	if m := pySleep.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait %s seconds", m[1])
	}

	// .back()
	if pyBack.MatchString(line) {
		return "go back"
	}

	// screenshot
	if m := pyScreenshot.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("take a screenshot called %q", m[1])
	}

	return ""
}

func pyLocator(strategy, value string) string {
	switch strings.ToUpper(strategy) {
	case "ID":
		return "#" + value
	case "ACCESSIBILITY_ID":
		return fmt.Sprintf("%q", value)
	case "XPATH":
		if text := extractXPathText(value); text != "" {
			return fmt.Sprintf("%q", text)
		}
		return fmt.Sprintf("# xpath: %s", value)
	case "NAME":
		return fmt.Sprintf("%q", value)
	case "CLASS_NAME":
		return fmt.Sprintf("# class: %s", value)
	default:
		return fmt.Sprintf("%q", value)
	}
}

var xpathTextExtract = regexp.MustCompile(`@(?:text|content-desc|label|name)=["']([^"']+)["']`)

func extractXPathText(xpath string) string {
	if m := xpathTextExtract.FindStringSubmatch(xpath); m != nil {
		return m[1]
	}
	return ""
}

func isPythonNoise(line string) bool {
	noises := []string{"self.driver", "self.assertEqual", "self.assert", "print(", "pass", "return", "super()"}
	for _, n := range noises {
		if strings.HasPrefix(line, n) && !strings.Contains(line, "find_element") &&
			!strings.Contains(line, "click()") && !strings.Contains(line, "send_keys") {
			return true
		}
	}
	return false
}

func hasPythonAPI(line string) bool {
	apis := []string{"find_element", ".click()", ".send_keys(", ".clear()", "WebDriverWait",
		".back()", "get_screenshot", "save_screenshot"}
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
