// Package gherkin converts Cucumber/Gherkin .feature files to ProbeScript.
package gherkin

import (
	"fmt"
	"strings"

	"github.com/flutterprobe/probe-convert/convert"
)

// Converter handles Gherkin → ProbeScript conversion.
type Converter struct{}

// New creates a new Gherkin converter.
func New() *Converter { return &Converter{} }

func (c *Converter) Format() convert.Format    { return convert.FormatGherkin }
func (c *Converter) Extensions() []string       { return []string{".feature"} }

func (c *Converter) Convert(source []byte, path string) (*convert.Result, error) {
	probe, warnings := convertFeature(string(source), path)
	return &convert.Result{
		InputPath: path,
		ProbeCode: probe,
		Warnings:  warnings,
	}, nil
}

type parserState int

const (
	stateTop parserState = iota
	stateBackground
	stateScenario
	stateExamples
)

func convertFeature(src, path string) (string, []convert.Warning) {
	w := convert.NewProbeWriter()
	var warnings []convert.Warning
	var pendingTags []string

	state := stateTop
	lines := strings.Split(src, "\n")
	_ = path

	var exampleHeaders []string
	var exampleRows [][]string

	flushExamples := func() {
		if len(exampleHeaders) > 0 && len(exampleRows) > 0 {
			w.WithExamples(exampleHeaders, exampleRows)
		}
		exampleHeaders = nil
		exampleRows = nil
	}

	for lineNum, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Tag collection.
		if strings.HasPrefix(line, "@") {
			tags := strings.Fields(line)
			for _, t := range tags {
				if strings.HasPrefix(t, "@") {
					pendingTags = append(pendingTags, t)
				}
			}
			continue
		}

		// Feature declaration.
		if strings.HasPrefix(line, "Feature:") {
			featureName := strings.TrimSpace(strings.TrimPrefix(line, "Feature:"))
			w.Comment(fmt.Sprintf("Converted from Gherkin — Feature: %s", featureName))
			w.BlankLine()
			continue
		}

		// Background.
		if strings.HasPrefix(line, "Background:") {
			flushExamples()
			state = stateBackground
			w.BeforeEach()
			continue
		}

		// Scenario Outline.
		if strings.HasPrefix(line, "Scenario Outline:") || strings.HasPrefix(line, "Scenario Template:") {
			flushExamples()
			state = stateScenario
			name := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			w.BlankLine()
			w.Test(name)
			if len(pendingTags) > 0 {
				w.Tags(pendingTags)
				pendingTags = nil
			}
			continue
		}

		// Scenario.
		if strings.HasPrefix(line, "Scenario:") {
			flushExamples()
			state = stateScenario
			name := strings.TrimSpace(strings.TrimPrefix(line, "Scenario:"))
			w.BlankLine()
			w.Test(name)
			if len(pendingTags) > 0 {
				w.Tags(pendingTags)
				pendingTags = nil
			}
			continue
		}

		// Examples table.
		if strings.HasPrefix(line, "Examples:") || strings.HasPrefix(line, "Scenarios:") {
			state = stateExamples
			continue
		}

		// Table row in examples.
		if state == stateExamples && strings.HasPrefix(line, "|") {
			cells := parseTableRow(line)
			if exampleHeaders == nil {
				exampleHeaders = cells
			} else {
				exampleRows = append(exampleRows, cells)
			}
			continue
		}

		// Step lines.
		if state == stateBackground || state == stateScenario {
			stepText := stripGherkinKeyword(line)
			probeLine, matched := matchStep(stepText)
			if matched {
				// Handle screenshot with name.
				if probeLine == "take a screenshot" {
					w.Step(probeLine)
				} else {
					w.Step(probeLine)
				}
			} else {
				w.TODO(stepText)
				warnings = append(warnings, convert.Warning{
					Line:     lineNum + 1,
					Severity: convert.Warn,
					Message:  fmt.Sprintf("unmatched step: %s", stepText),
				})
			}
			continue
		}
	}

	flushExamples()
	return w.String(), warnings
}

// stripGherkinKeyword removes Given/When/Then/And/But/* prefix from a step.
func stripGherkinKeyword(line string) string {
	prefixes := []string{"Given ", "When ", "Then ", "And ", "But ", "* "}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return strings.TrimSpace(strings.TrimPrefix(line, p))
		}
	}
	return line
}

// parseTableRow splits a | delimited row into trimmed cells.
func parseTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}
