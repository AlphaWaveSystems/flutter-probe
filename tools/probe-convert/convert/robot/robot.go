// Package robot converts Robot Framework .robot files to ProbeScript.
package robot

import (
	"fmt"
	"strings"

	"github.com/flutterprobe/probe-convert/convert"
)

// Converter handles Robot Framework → ProbeScript conversion.
type Converter struct{}

// New creates a new Robot converter.
func New() *Converter { return &Converter{} }

func (c *Converter) Format() convert.Format    { return convert.FormatRobot }
func (c *Converter) Extensions() []string       { return []string{".robot"} }

func (c *Converter) Convert(source []byte, path string) (*convert.Result, error) {
	probe, warnings := convertRobot(string(source), path)
	return &convert.Result{
		InputPath: path,
		ProbeCode: probe,
		Warnings:  warnings,
	}, nil
}

type section int

const (
	sectionNone section = iota
	sectionSettings
	sectionVariables
	sectionTestCases
	sectionKeywords
)

func convertRobot(src, path string) (string, []convert.Warning) {
	w := convert.NewProbeWriter()
	var warnings []convert.Warning
	lines := strings.Split(src, "\n")

	w.Comment("Converted from Robot Framework")
	w.BlankLine()

	currentSection := sectionNone
	inBlock := false // inside a test case or keyword body

	for lineNum, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)

		// Skip empty lines and comments.
		if trimmed == "" {
			if inBlock {
				// Blank line may end a block in some contexts, but we rely on indentation.
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Section headers.
		if strings.HasPrefix(trimmed, "***") {
			upper := strings.ToUpper(trimmed)
			switch {
			case strings.Contains(upper, "SETTING"):
				currentSection = sectionSettings
			case strings.Contains(upper, "VARIABLE"):
				currentSection = sectionVariables
			case strings.Contains(upper, "TEST CASE"), strings.Contains(upper, "TASK"):
				currentSection = sectionTestCases
			case strings.Contains(upper, "KEYWORD"):
				currentSection = sectionKeywords
			}
			inBlock = false
			continue
		}

		// Settings section.
		if currentSection == sectionSettings {
			parts := splitRobotCells(trimmed)
			if len(parts) >= 2 {
				key := strings.ToLower(parts[0])
				switch {
				case key == "suite setup" || key == "test setup":
					w.BeforeEach()
					step, ok := mapKeyword(parts[1], parts[2:])
					if ok {
						w.Step(step)
					} else {
						w.Step(substituteVars(parts[1]))
					}
					w.BlankLine()
					w.SetDepth(0)
				case key == "suite teardown" || key == "test teardown":
					w.AfterEach()
					step, ok := mapKeyword(parts[1], parts[2:])
					if ok {
						w.Step(step)
					} else {
						w.Step(substituteVars(parts[1]))
					}
					w.BlankLine()
					w.SetDepth(0)
				}
			}
			continue
		}

		// Variables section — skip (we handle ${VAR} inline).
		if currentSection == sectionVariables {
			continue
		}

		// Test Cases / Keywords sections.
		if currentSection == sectionTestCases || currentSection == sectionKeywords {
			isIndented := rawLine[0] == ' ' || rawLine[0] == '\t'

			if !isIndented {
				// New test case or keyword name.
				inBlock = true
				name := strings.TrimSpace(trimmed)
				name = substituteVars(name)
				w.BlankLine()
				if currentSection == sectionTestCases {
					w.Test(name)
				} else {
					w.Recipe(name, nil)
				}
				continue
			}

			// Indented = body line of current test/keyword.
			if !inBlock {
				continue
			}

			parts := splitRobotCells(trimmed)
			if len(parts) == 0 {
				continue
			}

			first := parts[0]
			rest := parts[1:]

			// Handle special settings.
			switch strings.ToLower(first) {
			case "[tags]":
				var tags []string
				for _, t := range rest {
					tags = append(tags, t)
				}
				w.Tags(tags)
				continue
			case "[arguments]":
				// Re-declare recipe with params.
				var params []string
				for _, a := range rest {
					params = append(params, cleanVarName(a))
				}
				// We can't easily re-emit the recipe header, so add a comment.
				if len(params) > 0 {
					w.Comment(fmt.Sprintf("arguments: %s", strings.Join(params, ", ")))
				}
				continue
			case "[template]":
				if len(rest) > 0 {
					w.Comment(fmt.Sprintf("template: %s", rest[0]))
				}
				continue
			case "[setup]":
				if len(rest) > 0 {
					step, ok := mapKeyword(rest[0], rest[1:])
					if ok {
						w.Step(step)
					}
				}
				continue
			case "[teardown]":
				continue
			case "[documentation]":
				if len(rest) > 0 {
					w.Comment(strings.Join(rest, " "))
				}
				continue
			case "[return]":
				continue
			}

			// Regular keyword call.
			keyword := first
			args := substituteVarsSlice(rest)
			step, ok := mapKeyword(keyword, args)
			if ok {
				w.Step(substituteVars(step))
			} else {
				// Unknown keyword — emit as recipe call or TODO.
				if isLikelyKeywordCall(keyword) {
					call := substituteVars(keyword)
					if len(args) > 0 {
						call += " with " + strings.Join(quoteArgs(args), " and ")
					}
					w.Step(call)
				} else {
					w.TODO(substituteVars(trimmed))
					warnings = append(warnings, convert.Warning{
						Line:     lineNum + 1,
						Severity: convert.Warn,
						Message:  fmt.Sprintf("unrecognized keyword: %s", keyword),
					})
				}
			}
		}
	}

	return w.String(), warnings
}

// splitRobotCells splits a Robot Framework line by 2+ spaces or tab.
func splitRobotCells(line string) []string {
	// Robot uses 2+ spaces or \t as cell separators.
	var parts []string
	// First normalize tabs to double-space.
	line = strings.ReplaceAll(line, "\t", "  ")
	for _, p := range strings.Split(line, "  ") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// substituteVars replaces ${var} with <var> for ProbeScript.
func substituteVars(s string) string {
	result := s
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		varName := result[start+2 : start+end]
		result = result[:start] + "<" + strings.ToLower(varName) + ">" + result[start+end+1:]
	}
	return result
}

func substituteVarsSlice(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = substituteVars(s)
	}
	return out
}

// cleanVarName strips ${} from a variable reference.
func cleanVarName(v string) string {
	v = strings.TrimPrefix(v, "${")
	v = strings.TrimSuffix(v, "}")
	return strings.ToLower(v)
}

// isLikelyKeywordCall returns true if the string looks like a custom keyword call
// (starts with uppercase letter, contains spaces).
func isLikelyKeywordCall(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] >= 'A' && s[0] <= 'Z'
}

func quoteArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "<") || strings.HasPrefix(a, "#") {
			out = append(out, a)
		} else {
			out = append(out, fmt.Sprintf("%q", a))
		}
	}
	return out
}
