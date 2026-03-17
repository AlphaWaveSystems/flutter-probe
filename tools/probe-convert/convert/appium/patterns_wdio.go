package appium

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alphawavesystems/probe-convert/convert"
)

// WebdriverIO / Appium JS patterns.
var (
	jsClickSelector = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.click\(\)`)
	jsSetValue      = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.setValue\(['"](.*?)['"]\)`)
	jsAddValue      = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.addValue\(['"](.*?)['"]\)`)
	jsClearValue    = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.clearValue\(\)`)
	jsIsDisplayed   = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.isDisplayed\(\)`)
	jsWaitForExist  = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.waitForExist\(`)
	jsWaitDisplay   = regexp.MustCompile(`\$\(['"]([~#]?)([^'"]+)['"]\)\.waitForDisplayed\(`)
	jsItBlock       = regexp.MustCompile(`it\(['"](.*?)['"]`)
	jsDescribe      = regexp.MustCompile(`describe\(['"](.*?)['"]`)
	jsBeforeBlock   = regexp.MustCompile(`before(?:All|Each)\(`)
	jsAfterBlock    = regexp.MustCompile(`after(?:All|Each)\(`)
	jsSleep         = regexp.MustCompile(`(?:browser\.)?pause\((\d+)\)`)
	jsBack          = regexp.MustCompile(`(?:browser|driver)\.back\(\)`)
	jsScreenshot    = regexp.MustCompile(`(?:browser|driver)\.(?:saveScreenshot|takeScreenshot)\(['"](.*?)['"]\)`)
)

func convertJS(src, path string) (string, []convert.Warning) {
	w := convert.NewProbeWriter()
	var warnings []convert.Warning

	w.Comment("Converted from Appium JS")
	w.BlankLine()

	lines := strings.Split(src, "\n")
	inBlock := false

	for lineNum, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
			continue
		}

		// Skip imports.
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "const ") ||
			strings.HasPrefix(line, "require(") || strings.HasPrefix(line, "let ") ||
			strings.HasPrefix(line, "var ") {
			continue
		}

		// Skip closing braces.
		if line == "});" || line == "})," || line == "}" || line == "})" {
			continue
		}

		// describe.
		if m := jsDescribe.FindStringSubmatch(line); m != nil {
			w.Comment(m[1])
			w.BlankLine()
			continue
		}

		// it block.
		if m := jsItBlock.FindStringSubmatch(line); m != nil {
			w.BlankLine()
			w.Test(m[1])
			inBlock = true
			continue
		}

		// before.
		if jsBeforeBlock.MatchString(line) {
			w.BeforeEach()
			inBlock = true
			continue
		}

		// after.
		if jsAfterBlock.MatchString(line) {
			w.AfterEach()
			inBlock = true
			continue
		}

		if !inBlock {
			continue
		}

		if step := matchJSLine(line); step != "" {
			w.Step(step)
			continue
		}

		if isJSNoise(line) {
			continue
		}

		if hasJSAPI(line) {
			w.TODO(line)
			warnings = append(warnings, convert.Warning{
				Line:     lineNum + 1,
				Severity: convert.Warn,
				Message:  fmt.Sprintf("unrecognized Appium JS call: %s", truncate(line, 80)),
			})
		}
	}

	return w.String(), warnings
}

func matchJSLine(line string) string {
	// $('~x').click() or $('#x').click()
	if m := jsClickSelector.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("tap on %s", jsLocator(m[1], m[2]))
	}

	// .setValue()
	if m := jsSetValue.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into %s", m[3], jsLocator(m[1], m[2]))
	}

	// .addValue()
	if m := jsAddValue.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into %s", m[3], jsLocator(m[1], m[2]))
	}

	// .clearValue()
	if m := jsClearValue.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("clear %s", jsLocator(m[1], m[2]))
	}

	// .waitForExist()
	if m := jsWaitForExist.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait until %s appears", jsLocator(m[1], m[2]))
	}

	// .waitForDisplayed()
	if m := jsWaitDisplay.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait until %s appears", jsLocator(m[1], m[2]))
	}

	// pause/sleep
	if m := jsSleep.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait %s seconds", msToSec(m[1]))
	}

	// back
	if jsBack.MatchString(line) {
		return "go back"
	}

	// screenshot
	if m := jsScreenshot.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("take a screenshot called %q", m[1])
	}

	return ""
}

func jsLocator(prefix, value string) string {
	switch prefix {
	case "~":
		return fmt.Sprintf("%q", value)
	case "#":
		return "#" + value
	default:
		return fmt.Sprintf("%q", value)
	}
}

func isJSNoise(line string) bool {
	noises := []string{"await", "async", "console.", "return", "expect("}
	trimmed := strings.TrimSpace(line)
	for _, n := range noises {
		if strings.HasPrefix(trimmed, n) && !strings.Contains(trimmed, "$('") && !strings.Contains(trimmed, "$(\"") {
			return true
		}
	}
	return false
}

func hasJSAPI(line string) bool {
	apis := []string{"$('", "$(\"", ".click()", ".setValue(", ".clearValue(", ".waitForExist(", ".waitForDisplayed("}
	for _, a := range apis {
		if strings.Contains(line, a) {
			return true
		}
	}
	return false
}
