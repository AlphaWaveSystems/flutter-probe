package appium

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/alphawavesystems/probe-convert/convert"
)

// Java Appium patterns.
var (
	javaFindClick    = regexp.MustCompile(`findElement\((?:By|AppiumBy)\.(\w+)\(['"](.*?)['"]\)\)\.click\(\)`)
	javaFindSendKeys = regexp.MustCompile(`findElement\((?:By|AppiumBy)\.(\w+)\(['"](.*?)['"]\)\)\.sendKeys\(['"](.*?)['"]\)`)
	javaFindClear    = regexp.MustCompile(`findElement\((?:By|AppiumBy)\.(\w+)\(['"](.*?)['"]\)\)\.clear\(\)`)
	javaWait         = regexp.MustCompile(`(?:WebDriverWait|wait).*?until\(.*?(?:visibilityOf|presenceOf).*?(?:By|AppiumBy)\.(\w+)\(['"](.*?)['"]`)
	javaSleep        = regexp.MustCompile(`Thread\.sleep\((\d+)\)`)
	javaTestMethod   = regexp.MustCompile(`(?:@Test\s+)?(?:public\s+)?void\s+(\w+)\s*\(`)
	javaBefore       = regexp.MustCompile(`@(?:Before|BeforeEach|BeforeAll)`)
	javaAfter        = regexp.MustCompile(`@(?:After|AfterEach|AfterAll)`)
	javaClassDef     = regexp.MustCompile(`class\s+(\w+)`)
	javaBack         = regexp.MustCompile(`\.navigate\(\)\.back\(\)|\.back\(\)`)
	javaScreenshot   = regexp.MustCompile(`getScreenshotAs|takeScreenshot`)
)

func convertJava(src, path string) (string, []convert.Warning) {
	w := convert.NewProbeWriter()
	var warnings []convert.Warning

	w.Comment("Converted from Appium Java")
	w.BlankLine()

	lines := strings.Split(src, "\n")
	inBlock := false
	nextIsBefore := false
	nextIsAfter := false

	for lineNum, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
			continue
		}

		// Skip imports and package.
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "package ") {
			continue
		}

		// Class definition.
		if m := javaClassDef.FindStringSubmatch(line); m != nil {
			w.Comment(fmt.Sprintf("Test class: %s", m[1]))
			continue
		}

		// @Before annotation.
		if javaBefore.MatchString(line) {
			nextIsBefore = true
			continue
		}
		if javaAfter.MatchString(line) {
			nextIsAfter = true
			continue
		}

		// Method definition (possibly preceded by @Before/@After/@Test).
		if m := javaTestMethod.FindStringSubmatch(line); m != nil {
			methodName := m[1]
			if nextIsBefore {
				w.BeforeEach()
				nextIsBefore = false
				inBlock = true
				continue
			}
			if nextIsAfter {
				w.AfterEach()
				nextIsAfter = false
				inBlock = true
				continue
			}
			// Skip non-test methods (no @Test and not test-prefixed).
			if !strings.HasPrefix(strings.ToLower(methodName), "test") {
				continue
			}
			name := camelToSpaces(strings.TrimPrefix(methodName, "test"))
			w.BlankLine()
			w.Test(name)
			inBlock = true
			continue
		}

		// Reset annotation flags on non-method lines.
		if nextIsBefore || nextIsAfter {
			if !strings.HasPrefix(line, "@") && !strings.Contains(line, "void ") {
				nextIsBefore = false
				nextIsAfter = false
			}
		}

		if !inBlock {
			continue
		}

		// Match patterns.
		if step := matchJavaLine(line); step != "" {
			w.Step(step)
			continue
		}

		if isJavaNoise(line) {
			continue
		}

		if hasJavaAPI(line) {
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

func matchJavaLine(line string) string {
	if m := javaFindClick.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("tap on %s", javaLocator(m[1], m[2]))
	}
	if m := javaFindSendKeys.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("type %q into %s", m[3], javaLocator(m[1], m[2]))
	}
	if m := javaFindClear.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("clear %s", javaLocator(m[1], m[2]))
	}
	if m := javaWait.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf("wait until %s appears", javaLocator(m[1], m[2]))
	}
	if m := javaSleep.FindStringSubmatch(line); m != nil {
		// Java sleep is in milliseconds.
		return fmt.Sprintf("wait %s seconds", msToSec(m[1]))
	}
	if javaBack.MatchString(line) {
		return "go back"
	}
	if javaScreenshot.MatchString(line) {
		return "take a screenshot"
	}
	return ""
}

func javaLocator(strategy, value string) string {
	switch strings.ToLower(strategy) {
	case "id":
		return "#" + value
	case "accessibilityid":
		return fmt.Sprintf("%q", value)
	case "xpath":
		if text := extractXPathText(value); text != "" {
			return fmt.Sprintf("%q", text)
		}
		return fmt.Sprintf("# xpath: %s", value)
	case "name":
		return fmt.Sprintf("%q", value)
	default:
		return fmt.Sprintf("%q", value)
	}
}

// camelToSpaces converts "LoginWithEmail" to "login with email".
func camelToSpaces(s string) string {
	if s == "" {
		return s
	}
	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune(' ')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

func msToSec(ms string) string {
	// Simple: strip trailing zeros.
	if len(ms) > 3 {
		return ms[:len(ms)-3]
	}
	return "1"
}

func isJavaNoise(line string) bool {
	noises := []string{"driver.", "Assert.", "System.out", "return", "}", "{", "try", "catch", "finally", "@"}
	for _, n := range noises {
		if strings.HasPrefix(line, n) && !strings.Contains(line, "findElement") &&
			!strings.Contains(line, "click()") && !strings.Contains(line, "sendKeys") {
			return true
		}
	}
	return false
}

func hasJavaAPI(line string) bool {
	apis := []string{"findElement", ".click()", ".sendKeys(", ".clear()", "WebDriverWait",
		".back()", "getScreenshotAs", "takeScreenshot"}
	for _, a := range apis {
		if strings.Contains(line, a) {
			return true
		}
	}
	return false
}
